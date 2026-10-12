// The tenant graph (docs/design.md 53), Graph/Nodes at /<tenant>/graph:
// every node and edge of the mounted layers, from GET
// /api/tenants/{tenant}/graph, drawn with the vendored Cytoscape as the
// Canvas draws a layer (colour and shape from the layers' ontologies, a Kind
// with none grey). ?source=<emitter> keeps the nodes of the layers whose
// Graph.Node source block names that emitter, and the edges between them;
// the page passes it on to the API and shows it as a chip whose X links to
// /<tenant>/graph. The filter is the URL's, never the page's state, so a
// filtered graph is a link (the Catalogue's Show graph is one). Nothing here
// writes.
/* global cytoscape */
import { label, splitAlpha, styleFor, wheelZoomSpeed } from '/static/canvas.js';

const WHEEL_STEP = 0.2;

// mount draws into el. ctx: {client, toolbar, config, navigate, sourceGraphHref}.
export function mount(el, ctx) {
  el.classList.add('pt-fill');
  const source = new URLSearchParams(location.search).get('source') || '';
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-graph';
  el.appendChild(wrap);
  const h = { wrap, cy: null, gone: false, ro: null };
  const count = document.createElement('span');
  count.className = 'pt-count';
  count.id = 'graph-stats';
  ctx.toolbar.appendChild(count);
  if (source) ctx.toolbar.appendChild(filterChip(source, ctx.sourceGraphHref(null), ctx.navigate));
  Promise.all([ctx.client.getTenantGraph({ source }), ctx.client.getOntology()]).then(([g, ontology]) => {
    if (h.gone) return;
    count.textContent = `${g.nodes.length} nodes · ${g.edges.length} edges`;
    count.dataset.nodes = String(g.nodes.length);
    count.dataset.edges = String(g.edges.length);
    if (!g.nodes.length) {
      wrap.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No nodes';
      const p = document.createElement('p');
      p.textContent = source
        ? `No mounted layer's source block names ${source}. A layer belongs to an emitter when its Graph.Node source block (source.emitter) gives the card's name.`
        : 'No layer is mounted (Mount-PortalLayer).';
      wrap.append(t, p);
      return;
    }
    draw(h, ctx, g, merged(ontology));
  }).catch((err) => {
    wrap.classList.add('pt-error-card');
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return h;
}

export function destroy(h) {
  h.gone = true;
  if (h.ro) h.ro.disconnect();
  if (h.cy) h.cy.destroy();
  h.wrap.remove();
}

// filterChip is the active source filter, its X a link to the unfiltered
// graph that routes inside the shell on a plain click.
export function filterChip(source, clearHref, navigate) {
  const chip = document.createElement('span');
  chip.className = 'pt-filter';
  chip.dataset.filter = 'source';
  chip.dataset.value = source;
  chip.append('source: ');
  const v = document.createElement('b');
  v.textContent = source;
  chip.appendChild(v);
  const x = document.createElement('a');
  x.className = 'pt-filter-clear';
  x.href = clearHref;
  x.textContent = '×';
  x.title = 'clear the source filter';
  x.setAttribute('aria-label', `clear the source filter ${source}`);
  x.addEventListener('click', (ev) => {
    if (!navigate || ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.altKey || ev.button !== 0) return;
    ev.preventDefault();
    navigate(clearHref);
  });
  chip.appendChild(x);
  return chip;
}

// merged is every mounted layer's ontology as one, the first Kind wins in
// mount order: the tenant graph's nodes keep their layers' Kinds.
function merged(ontology) {
  const out = { kinds: {}, edgeKinds: {} };
  for (const ont of Object.values((ontology && ontology.layers) || {})) {
    for (const key of ['kinds', 'edgeKinds']) {
      for (const [k, v] of Object.entries((ont && ont[key]) || {})) if (!(k in out[key])) out[key][k] = v;
    }
  }
  return out;
}

function draw(h, ctx, g, ont) {
  const root = document.createElement('div');
  root.className = 'pt-graph-root';
  h.wrap.appendChild(root);
  const elements = g.nodes.map((n) => ({ group: 'nodes', data: { id: n.id, kind: n.kind, label: label(ont, n) } }));
  for (const e of g.edges) elements.push({ group: 'edges', data: { id: e.id, kind: e.kind, source: e.from, target: e.to } });
  h.cy = cytoscape({ container: root, elements, wheelSensitivity: WHEEL_STEP * wheelZoomSpeed(ctx.config), maxZoom: 2.2 });
  const cs = getComputedStyle(root);
  const tok = (name) => splitAlpha(cs.getPropertyValue(name).trim() || cs.color);
  h.cy.style(styleFor(ont, tok('--gr-text'), tok('--gr-text-muted'), tok('--gr-fallback'), cs.fontFamily));
  h.cy.layout({ name: 'cose', animate: false, fit: true }).run();
  const fit = () => {
    if (h.gone || !root.clientWidth || !root.clientHeight) return;
    h.cy.resize();
    h.cy.fit(undefined, 24);
  };
  h.ro = new ResizeObserver(fit);
  h.ro.observe(root);
  requestAnimationFrame(fit);
  root.dataset.drawn = `${g.nodes.length}/${g.edges.length}`;
}
