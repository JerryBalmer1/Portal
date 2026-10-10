// Canvas: draws a Widget's source (a mounted layer query) with Cytoscape.
// Lifted from GraphRenderer's pkg/widgets/canvas/assets/widget.js: SHAPES,
// the tinted-glass style, splitAlpha and restyle-on-paint are its code. What
// changed: data comes from GET /api/layers/{name}/nodes, display from that
// layer's ontology in GET /api/ontology (a Kind with no display colour is
// grey, --gr-fallback), labels are Graph.Node templates ({name},
// {properties.x}), tapping a node selects it for Detail widgets, and
// Graph.Node 0.2.0 display (circle and square, size, edge style) maps onto
// Cytoscape.
/* global cytoscape */
import { parseSource } from '/static/client.js';

const SHAPES = new Set([
  'ellipse', 'triangle', 'round-triangle', 'rectangle', 'round-rectangle',
  'bottom-round-rectangle', 'cut-rectangle', 'barrel', 'rhomboid', 'diamond',
  'round-diamond', 'pentagon', 'round-pentagon', 'hexagon', 'round-hexagon',
  'concave-hexagon', 'heptagon', 'round-heptagon', 'octagon', 'round-octagon',
  'star', 'tag', 'round-tag', 'vee',
]);
// Graph.Node 0.2.0's display shapes are circle, square, diamond and hexagon;
// the first two are not Cytoscape names.
const SHAPE_ALIASES = { circle: 'ellipse', square: 'round-rectangle' };
const EDGE_STYLES = new Set(['solid', 'dashed', 'dotted']);

export function mount(el, ctx) {
  const root = document.createElement('div');
  root.className = 'pt-canvas-root';
  root.style.position = 'absolute';
  root.style.inset = '0';
  el.appendChild(root);
  const src = parseSource(ctx.node.properties && ctx.node.properties.source);
  const ont = (ctx.ontology.layers && ctx.ontology.layers[src.layer]) || { kinds: {}, edgeKinds: {} };
  const h = { root, cy: null, gone: false, ro: null, timer: 0 };
  // Cytoscape measures its container once, when made; on first load that
  // can be before the panel has a size, and the graph is drawn a few pixels
  // wide. fit measures again and fits whenever the container has a size:
  // once on the next frame, then from a ResizeObserver on the panel body,
  // debounced, which also fires when a collapsed panel expands
  // (docs/design.md 29). data-fit records the drawn graph's width against
  // the panel's, for the smoke.
  const body = el.closest('.gr-box-body') || el;
  const fit = () => {
    if (h.gone || !h.cy || !root.clientWidth || !root.clientHeight) return;
    h.cy.resize();
    h.cy.fit(undefined, 24);
    const panel = el.closest('.gr-box') || el;
    const drawn = h.cy.elements().renderedBoundingBox();
    const box = panel.getBoundingClientRect();
    el.dataset.fit = `${Math.round(drawn.w)}/${Math.round(box.width)}`;
    // A page-wide panel is fitted by its height first (docs/design.md 36),
    // so the height is recorded too.
    el.dataset.fitHeight = `${Math.round(drawn.h)}/${Math.round(box.height)}`;
  };
  h.ro = new ResizeObserver(() => {
    clearTimeout(h.timer);
    h.timer = setTimeout(fit, 100);
  });
  h.ro.observe(body);
  ctx.client.getLayerNodes(src.layer, { kind: src.kind, from: src.from }).then((graph) => {
    if (h.gone) return;
    h.cy = cytoscape({ container: root, elements: elements(ont, graph), wheelSensitivity: 0.2, maxZoom: 2.2 });
    restyle(h, ont);
    h.cy.layout({ name: 'cose', animate: false, fit: true }).run();
    requestAnimationFrame(fit);
    h.cy.on('tap', 'node', (ev) => {
      ctx.bus.dispatchEvent(new CustomEvent('select', { detail: { layer: src.layer, id: ev.target.id() } }));
    });
    // Rendered colour, for the smoke check and the report: n/m nodes took a
    // colour from the layer ontology; the rest are grey.
    const kinds = ont.kinds || {};
    const coloured = graph.nodes.filter((n) => kinds[n.kind] && kinds[n.kind].display && kinds[n.kind].display.color).length;
    el.dataset.coloured = `${coloured}/${graph.nodes.length}`;
    el.closest('.gr-box')?.querySelector('.pt-badge')?.append(` · ${src.layer} · ${coloured ? 'colour' : 'grey'} ${coloured}/${graph.nodes.length}`);
  }).catch((err) => {
    const pre = document.createElement('pre');
    pre.className = 'gr-error';
    pre.textContent = JSON.stringify(err, null, 2);
    el.appendChild(pre);
  });
  return h;
}

export function destroy(h) {
  h.gone = true;
  h.ro.disconnect();
  clearTimeout(h.timer);
  if (h.cy) h.cy.destroy();
  h.root.remove();
}

function elements(ont, graph) {
  const out = graph.nodes.map((n) => ({ group: 'nodes', data: { id: n.id, kind: n.kind, label: label(ont, n) } }));
  for (const e of graph.edges) out.push({ group: 'edges', data: { id: e.id, kind: e.kind, source: e.from, target: e.to } });
  return out;
}

// label fills a Graph.Node display label template: {name} and {properties.x}.
export function label(ont, n) {
  const d = (ont.kinds && ont.kinds[n.kind] && ont.kinds[n.kind].display) || {};
  if (!d.label) return n.name || n.id;
  return d.label.replace(/\{([^}]+)\}/g, (_, key) => {
    const v = key === 'name' ? n.name : key.startsWith('properties.') ? (n.properties || {})[key.slice(11)] : n[key];
    return v == null ? '' : String(v);
  });
}

function restyle(h, ont) {
  const cs = getComputedStyle(h.root);
  const tok = (name) => splitAlpha(cs.getPropertyValue(name).trim() || cs.color);
  h.cy.style(styleFor(ont, tok('--gr-text'), tok('--gr-text-muted'), tok('--gr-fallback'), cs.fontFamily));
}

// Cytoscape ignores the alpha of an rgba() colour, so it travels separately
// as text-opacity, line-opacity and the like.
function splitAlpha(c) {
  const m = /^rgba\(\s*([^,]+),\s*([^,]+),\s*([^,]+),\s*([^)]+)\)$/i.exec(c);
  return m ? { rgb: `rgb(${m[1]},${m[2]},${m[3]})`, alpha: Number(m[4]) } : { rgb: c, alpha: 1 };
}

function styleFor(ont, text, muted, grey, font) {
  const s = [
    {
      selector: 'node',
      style: {
        label: 'data(label)', color: text.rgb, 'text-opacity': text.alpha, 'font-family': font, 'font-size': 10, 'font-weight': 500,
        'text-valign': 'bottom', 'text-margin-y': 6, width: 26, height: 26,
        'background-color': grey.rgb, 'background-opacity': 0.22,
        'border-width': 1.5, 'border-color': grey.rgb, 'border-opacity': 0.9,
      },
    },
    { selector: 'edge', style: { width: 1.2, 'curve-style': 'bezier', 'target-arrow-shape': 'triangle', 'arrow-scale': 0.8, 'line-color': muted.rgb, 'target-arrow-color': muted.rgb, 'line-opacity': muted.alpha * 0.7 } },
  ];
  for (const [kind, def] of Object.entries(ont.kinds || {})) {
    const d = def.display || {};
    const st = {};
    if (d.color) Object.assign(st, { 'background-color': d.color, 'background-opacity': 0.22, 'border-color': d.color, 'border-opacity': 0.9 });
    const shape = SHAPE_ALIASES[d.shape] || d.shape;
    if (shape && SHAPES.has(shape)) st.shape = shape;
    if (Number.isInteger(d.size) && d.size >= 1) { st.width = 18 + 8 * d.size; st.height = st.width; }
    if (d.collapsed) { st.width = 12; st.height = 12; }
    s.push({ selector: `node[kind = ${JSON.stringify(kind)}]`, style: st });
  }
  for (const [kind, def] of Object.entries(ont.edgeKinds || {})) {
    const d = def.display || {};
    const st = {};
    if (d.color) Object.assign(st, { 'line-color': d.color, 'target-arrow-color': d.color, 'line-opacity': 0.55 });
    if (EDGE_STYLES.has(d.style)) st['line-style'] = d.style;
    if (Object.keys(st).length) s.push({ selector: `edge[kind = ${JSON.stringify(kind)}]`, style: st });
  }
  return s;
}
