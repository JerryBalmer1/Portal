// The environments graph (docs/design.md 46), Environments/Graph at
// /<tenant>/environments/graph. Every environment.* node of the mounted
// layers and what is one edge away (a principal, a run, ...), from GET
// /api/tenants/{tenant}/environments/graph, drawn as SVG so every node is a
// link: /<tenant>/environments/graph#<id>, the selected node, which survives
// a reload and can be pasted. A force layout (repulsion, a link distance by
// edge kind, a pull toward the node's env) runs until it settles. Drag a
// node to pin it; double-click releases it. Selecting a node pins it and its
// neighbours where they are. Colour is the instance's kind (compose,
// container, kubernetes, host), shape the node's Kind (instance, image,
// network, other); a node one edge away is smaller and dimmer. member-of is
// solid, runs-as dashed, hosts dotted. Nodes are grouped by the env they
// carry (dev, qa, prod; none is "unset", so the page shows the gap) in
// hulls, and by group inside env; chips filter on env and tags. Portal reads
// env, group and tags from the nodes and infers none. Right-click (or
// Shift+F10) opens the node menu (nodemenu.js). Positions, pins, the chips
// and the selection are the viewer's pointers (docs/design.md 11): never
// written.
import * as nodemenu from '/static/nodemenu.js';
import { chips } from '/static/nodes.js';

const SVG = 'http://www.w3.org/2000/svg';
const W = 1000;
const H = 640;

export const ENVS = Object.freeze(['dev', 'qa', 'prod']);
export const UNSET = 'unset';
export const INSTANCE_KINDS = Object.freeze(['compose', 'container', 'kubernetes', 'host']);
// The link distance of each edge kind; any other kind gets OTHER_DISTANCE.
export const EDGE_DISTANCE = Object.freeze({ 'member-of': 90, 'runs-as': 170, hosts: 170 });
const OTHER_DISTANCE = 130;

// isEnvironment is nodemenu's: an environment.* Kind under an env: id.
export const isEnvironment = nodemenu.isEnvironment;

// shapeOf is a node's shape by its Kind.
export function shapeOf(n) {
  switch (n.kind) {
    case 'environment.instance': return 'circle';
    case 'environment.image': return 'square';
    case 'environment.network': return 'diamond';
    default: return 'hexagon';
  }
}

// instanceKind is what colours an instance: its properties.kind when it is
// one of INSTANCE_KINDS, else "".
export function instanceKind(n) {
  const k = n.kind === 'environment.instance' ? n.properties?.kind : '';
  return INSTANCE_KINDS.includes(k) ? k : '';
}

export function envOf(n) {
  const v = n.properties?.env;
  return typeof v === 'string' && v.trim() ? v.trim() : UNSET;
}

export function groupOf(n) {
  const v = n.properties?.group;
  return typeof v === 'string' && v.trim() ? v.trim() : '';
}

export function tagsOf(n) {
  const v = n.properties?.tags;
  if (Array.isArray(v)) return v.filter((t) => typeof t === 'string' && t);
  return typeof v === 'string' && v ? [v] : [];
}

// envChips is the env chips' values: dev, qa and prod always, any other env
// the nodes carry, then unset.
export function envChips(list) {
  const extra = [...new Set(list.map(envOf))].filter((e) => !ENVS.includes(e) && e !== UNSET).sort();
  return [...ENVS, ...extra, UNSET];
}

export function shortLabel(n) {
  if (n.kind === 'Principal') return n.name || n.id;
  if (n.kind === 'Run') return `run ${String(n.properties?.startedAt || n.id).slice(0, 19)}`;
  if (n.properties?.service) return String(n.properties.service);
  return String(n.name || n.id).replace(/\s*\(.*\)$/, '');
}

// hull is the convex hull (monotone chain) of points [[x, y], ...].
export function hull(points) {
  const p = [...points].sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  if (p.length < 3) return p;
  const cross = (o, a, b) => (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0]);
  const lower = [];
  for (const q of p) {
    while (lower.length >= 2 && cross(lower[lower.length - 2], lower[lower.length - 1], q) <= 0) lower.pop();
    lower.push(q);
  }
  const upper = [];
  for (const q of p.slice().reverse()) {
    while (upper.length >= 2 && cross(upper[upper.length - 2], upper[upper.length - 1], q) <= 0) upper.pop();
    upper.push(q);
  }
  return lower.slice(0, -1).concat(upper.slice(0, -1));
}

function el(name, attrs = {}, parent) {
  const e = document.createElementNS(SVG, name);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
  if (parent) parent.appendChild(e);
  return e;
}

function shapeEl(shape, r) {
  switch (shape) {
    case 'circle': return el('circle', { r });
    case 'square': return el('rect', { x: -r, y: -r, width: 2 * r, height: 2 * r, rx: 2 });
    case 'diamond': return el('polygon', { points: `0,${-r * 1.25} ${r * 1.25},0 0,${r * 1.25} ${-r * 1.25},0` });
    default: {
      const pts = [];
      for (let i = 0; i < 6; i++) {
        const a = (Math.PI / 3) * i;
        pts.push(`${(r * Math.cos(a)).toFixed(2)},${(r * Math.sin(a)).toFixed(2)}`);
      }
      return el('polygon', { points: pts.join(' ') });
    }
  }
}

// mount draws into el. ctx: {client, bus, page, toolbar, navigate,
// nodeLinks}.
export function mount(root, ctx) {
  root.classList.add('pt-fill');
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-envgraph';
  root.appendChild(wrap);
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  const h = { wrap, off: () => {}, onHash: () => {} };
  ctx.client.getEnvironmentGraph().then((g) => {
    if (!wrap.isConnected) return;
    draw(wrap, ctx, g, count, h);
  }).catch((err) => {
    wrap.classList.add('pt-error-card');
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return h;
}

export function destroy(h) {
  nodemenu.close(false);
  h.off();
  h.wrap.remove();
}

function draw(wrap, ctx, g, count, h) {
  const env = g.nodes.filter(isEnvironment);
  if (!env.length) {
    count.textContent = '0 nodes';
    wrap.className = 'pt-card pt-empty-state';
    wrap.dataset.kind = 'environment.*';
    const t = document.createElement('h2');
    t.textContent = 'No environments yet';
    const p = document.createElement('p');
    p.textContent = 'Nothing of an environment.* Kind under an env: id is in a mounted layer. A layer that emits environment.instance, environment.image or environment.network nodes under env:compose/, env:container/, env:image/ or env:network/ ids fills this page.';
    wrap.replaceChildren(t, p);
    return;
  }
  const byId = new Map();
  const envKeys = envChips(env).filter((e) => env.some((n) => envOf(n) === e));
  const nodes = g.nodes.map((n, i) => {
    const own = isEnvironment(n);
    const s = { n, id: n.id, own, env: own ? envOf(n) : null, group: own ? groupOf(n) : '', tags: own ? tagsOf(n) : [], r: own ? 14 : 9, vx: 0, vy: 0, pin: false, i };
    byId.set(n.id, s);
    return s;
  });
  // Start each env's nodes around its own point on a ring, the neighbours
  // in the middle, so the layout is the same on every load.
  const ring = envKeys.length > 1 ? 200 : 0;
  nodes.forEach((s, i) => {
    const k = s.own ? envKeys.indexOf(s.env) : -1;
    const a0 = k < 0 ? 0 : (2 * Math.PI * k) / envKeys.length;
    const cx = k < 0 ? 0 : ring * Math.cos(a0);
    const cy = k < 0 ? 0 : ring * Math.sin(a0);
    const a = i * 2.39996;
    const d = 30 + 18 * Math.sqrt(i);
    s.x = cx + d * Math.cos(a);
    s.y = cy + d * Math.sin(a);
  });
  const links = g.edges.filter((e) => byId.has(e.from) && byId.has(e.to)).map((e) => ({
    e, s: byId.get(e.from), t: byId.get(e.to), dist: EDGE_DISTANCE[e.kind] || OTHER_DISTANCE,
  }));
  const near = new Map(nodes.map((s) => [s.id, new Set()]));
  for (const l of links) {
    near.get(l.s.id).add(l.t.id);
    near.get(l.t.id).add(l.s.id);
  }
  const unset = env.filter((n) => envOf(n) === UNSET).length;
  count.textContent = `${env.length} nodes · ${nodes.length - env.length} one edge away · ${links.length} edges${unset ? ` · ${unset} without env` : ''}`;
  count.dataset.unset = String(unset);

  // ---- toolbar: env and tag chips, the selection's status ----
  let envFilter = '';
  let tagFilter = '';
  let search = '';
  for (const c of chips(envChips(env), (v) => { envFilter = v; apply(); }, 'filter by env')) {
    c.dataset.chips = 'env';
    ctx.toolbar.appendChild(c);
  }
  const tags = [...new Set(env.flatMap(tagsOf))].sort();
  if (tags.length) {
    for (const c of chips(tags, (v) => { tagFilter = v; apply(); }, 'filter by tag')) {
      c.dataset.chips = 'tags';
      ctx.toolbar.appendChild(c);
    }
  } else {
    const none = document.createElement('span');
    none.className = 'pt-muted';
    none.dataset.chips = 'tags';
    none.textContent = 'no tags';
    ctx.toolbar.appendChild(none);
  }
  const status = document.createElement('span');
  status.className = 'pt-eg-status';
  status.setAttribute('aria-live', 'polite');
  ctx.toolbar.appendChild(status);

  // ---- the drawing ----
  const svg = el('svg', { viewBox: `${-W / 2} ${-H / 2} ${W} ${H}`, class: 'pt-eg', role: 'group', 'aria-label': 'environments graph' });
  const defs = el('defs', {}, svg);
  const marker = el('marker', { id: 'pt-eg-arrow', viewBox: '0 0 10 10', refX: 9, refY: 5, markerWidth: 7, markerHeight: 7, orient: 'auto-start-reverse' }, defs);
  el('path', { d: 'M0,0 L10,5 L0,10 z', class: 'pt-eg-arrow' }, marker);
  const hullLayer = el('g', { class: 'pt-eg-hulls' }, svg);
  const edgeLayer = el('g', { class: 'pt-eg-edges' }, svg);
  const nodeLayer = el('g', { class: 'pt-eg-nodes' }, svg);
  for (const l of links) {
    l.line = el('line', { class: 'pt-eg-edge', 'data-kind': l.e.kind, 'data-from': l.s.id, 'data-to': l.t.id, 'marker-end': 'url(#pt-eg-arrow)' }, edgeLayer);
    el('title', {}, l.line).textContent = `${l.e.kind}: ${l.s.id} → ${l.t.id}`;
  }
  const base = `${location.pathname}${location.search}`;
  const hrefOf = (id) => `${base}#${encodeURIComponent(id)}`;
  for (const s of nodes) {
    const a = el('a', { href: hrefOf(s.id), class: `pt-eg-node${s.own ? '' : ' pt-eg-neighbour'}`, 'data-id': s.id, 'data-kind': s.n.kind, 'data-shape': shapeOf(s.n) }, nodeLayer);
    const ik = instanceKind(s.n);
    if (ik) a.dataset.ikind = ik;
    if (s.own) {
      a.dataset.env = s.env;
      if (s.group) a.dataset.group = s.group;
      if (s.tags.length) a.dataset.tags = s.tags.join(' ');
    }
    a.setAttribute('aria-label', `${s.n.name || s.id}, ${s.n.kind}${ik ? ` ${ik}` : ''}${s.own ? `, env ${s.env}` : ', one edge away'}`);
    const shape = shapeEl(shapeOf(s.n), s.r);
    shape.setAttribute('class', 'pt-eg-shape');
    a.appendChild(shape);
    el('text', { class: 'pt-eg-label', y: s.r + 13, 'text-anchor': 'middle' }, a).textContent = shortLabel(s.n);
    const tip = [`${s.n.name || s.id}`, `${s.n.kind} · ${s.id}`];
    if (s.own) tip.push(`env ${s.env}${s.group ? ` · group ${s.group}` : ''}${s.tags.length ? ` · tags ${s.tags.join(', ')}` : ''}`);
    el('title', {}, a).textContent = tip.join('\n');
    s.a = a;
  }
  wrap.appendChild(svg);
  wrap.appendChild(legend());

  // ---- the layout ----
  let alpha = 1;
  let frame = 0;
  let pending = null;
  const decay = 1 - Math.pow(0.001, 1 / 300);
  const tick = () => {
    for (let i = 0; i < nodes.length; i++) {
      const a = nodes[i];
      for (let j = i + 1; j < nodes.length; j++) {
        const b = nodes[j];
        let dx = b.x - a.x;
        let dy = b.y - a.y;
        let d2 = dx * dx + dy * dy;
        if (d2 < 1) { dx = (j - i) * 0.5; dy = 0.5; d2 = dx * dx + dy * dy; }
        if (d2 > 160000) continue;
        const d = Math.sqrt(d2);
        // Nodes of two envs push apart harder, so their hulls part.
        const k = (a.env && b.env && a.env !== b.env ? 5200 : 2200) * alpha / d2;
        a.vx -= (dx / d) * k; a.vy -= (dy / d) * k;
        b.vx += (dx / d) * k; b.vy += (dy / d) * k;
      }
    }
    for (const l of links) {
      const dx = l.t.x - l.s.x;
      const dy = l.t.y - l.s.y;
      const d = Math.sqrt(dx * dx + dy * dy) || 1;
      const k = ((d - l.dist) / d) * 0.08 * alpha;
      l.s.vx += dx * k; l.s.vy += dy * k;
      l.t.vx -= dx * k; l.t.vy -= dy * k;
    }
    // Each env's nodes are pulled toward their mean, so a hull holds its own.
    const mean = new Map();
    for (const s of nodes) {
      if (!s.env) continue;
      const m = mean.get(s.env) || { x: 0, y: 0, c: 0 };
      m.x += s.x; m.y += s.y; m.c++;
      mean.set(s.env, m);
    }
    for (const s of nodes) {
      const m = s.env && mean.get(s.env);
      if (m) { s.vx += (m.x / m.c - s.x) * 0.02 * alpha; s.vy += (m.y / m.c - s.y) * 0.02 * alpha; }
      s.vx -= s.x * 0.006 * alpha;
      s.vy -= s.y * 0.006 * alpha;
    }
    for (const s of nodes) {
      if (s.pin) { s.x = s.fx; s.y = s.fy; s.vx = 0; s.vy = 0; continue; }
      s.vx *= 0.6; s.vy *= 0.6;
      s.x = Math.max(-W * 2, Math.min(W * 2, s.x + s.vx));
      s.y = Math.max(-H * 2, Math.min(H * 2, s.y + s.vy));
    }
    alpha += (0 - alpha) * decay;
  };
  const render = () => {
    for (const s of nodes) {
      s.a.setAttribute('transform', `translate(${s.x.toFixed(1)},${s.y.toFixed(1)})`);
      s.a.classList.toggle('pt-eg-pinned', s.pin);
      if (s.pin) s.a.dataset.pinned = 'true';
      else delete s.a.dataset.pinned;
    }
    for (const l of links) {
      const dx = l.t.x - l.s.x;
      const dy = l.t.y - l.s.y;
      const d = Math.sqrt(dx * dx + dy * dy) || 1;
      const ux = dx / d;
      const uy = dy / d;
      l.line.setAttribute('x1', (l.s.x + ux * l.s.r).toFixed(1));
      l.line.setAttribute('y1', (l.s.y + uy * l.s.r).toFixed(1));
      l.line.setAttribute('x2', (l.t.x - ux * (l.t.r + 2)).toFixed(1));
      l.line.setAttribute('y2', (l.t.y - uy * (l.t.r + 2)).toFixed(1));
    }
    drawHulls();
    if (!fitted) fit();
  };
  // fit frames the drawing until it first settles, never closer than 1.5
  // times life size; after that the frame stays, so a drag does not move
  // the view under the pointer.
  let fitted = false;
  const fit = () => {
    const shown = nodes.filter((s) => !s.a.hasAttribute('hidden'));
    if (!shown.length) return;
    let x0 = Infinity; let y0 = Infinity; let x1 = -Infinity; let y1 = -Infinity;
    for (const s of shown) {
      x0 = Math.min(x0, s.x - 80); x1 = Math.max(x1, s.x + 80);
      y0 = Math.min(y0, s.y - 60); y1 = Math.max(y1, s.y + 50);
    }
    const box = svg.getBoundingClientRect();
    const minW = Math.max(240, (box.width || W) / 1.5);
    const minH = Math.max(160, (box.height || H) / 1.5);
    const w = Math.max(x1 - x0, minW);
    const h2 = Math.max(y1 - y0, minH);
    const cx = (x0 + x1) / 2;
    const cy = (y0 + y1) / 2;
    svg.setAttribute('viewBox', `${(cx - w / 2).toFixed(1)} ${(cy - h2 / 2).toFixed(1)} ${w.toFixed(1)} ${h2.toFixed(1)}`);
  };
  const run = () => {
    if (frame) return;
    svg.dataset.settled = 'false';
    const step = () => {
      for (let i = 0; i < 4 && alpha > 0.004; i++) tick();
      render();
      if (alpha > 0.004 && wrap.isConnected) {
        frame = requestAnimationFrame(step);
        return;
      }
      frame = 0;
      fitted = true;
      svg.dataset.settled = 'true';
      if (pending) {
        const id = pending;
        pending = null;
        pinAround(id);
      }
    };
    frame = requestAnimationFrame(step);
  };
  // settle lays the graph out before its first paint, up to a budget of
  // 150 ms (a large graph finishes animated), so a load or a reload shows
  // it at rest.
  const settle = () => {
    const until = performance.now() + 150;
    while (alpha > 0.004 && performance.now() < until) tick();
    if (alpha > 0.004) {
      run();
      return;
    }
    render();
    fitted = true;
    svg.dataset.settled = 'true';
  };
  const reheat = (to = 0.3) => {
    alpha = Math.max(alpha, to);
    run();
  };

  // ---- hulls: env, then group inside env ----
  function drawHulls() {
    hullLayer.replaceChildren();
    const sets = new Map();
    for (const s of nodes) {
      if (!s.own || s.a.hasAttribute('hidden')) continue;
      for (const key of [`env:${s.env}`, s.group ? `group:${s.env}\u0000${s.group}` : null]) {
        if (!key) continue;
        if (!sets.has(key)) sets.set(key, []);
        sets.get(key).push(s);
      }
    }
    for (const [key, members] of sets) {
      const isEnv = key.startsWith('env:');
      const pad = isEnv ? 30 : 18;
      const pts = [];
      for (const s of members) {
        for (let k = 0; k < 12; k++) {
          const a = (Math.PI / 6) * k;
          pts.push([s.x + (s.r + pad) * Math.cos(a), s.y + (s.r + pad) * Math.sin(a)]);
        }
      }
      const hp = hull(pts);
      const [envName, groupName] = isEnv ? [key.slice(4), ''] : key.slice(6).split('\u0000');
      const gEl = el('g', { class: isEnv ? 'pt-eg-hull' : 'pt-eg-hull pt-eg-subhull', 'data-env': envName }, hullLayer);
      if (!isEnv) gEl.dataset.group = groupName;
      el('path', { d: `M${hp.map((q) => `${q[0].toFixed(1)},${q[1].toFixed(1)}`).join('L')}Z` }, gEl);
      // env names its hull at the top, a group inside it at the bottom.
      const at = isEnv ? hp.reduce((m, q) => (q[1] < m[1] ? q : m), hp[0]) : hp.reduce((m, q) => (q[1] > m[1] ? q : m), hp[0]);
      const label = el('text', { x: at[0].toFixed(1), y: (isEnv ? at[1] - 5 : at[1] + 12).toFixed(1), 'text-anchor': 'middle' }, gEl);
      label.textContent = isEnv ? `env ${envName}` : groupName;
    }
  }

  // ---- filters: chips hide, search dims ----
  function apply() {
    const shown = new Set();
    for (const s of nodes) {
      if (!s.own) continue;
      if ((!envFilter || s.env === envFilter) && (!tagFilter || s.tags.includes(tagFilter))) shown.add(s.id);
    }
    for (const s of nodes) {
      if (!s.own && [...near.get(s.id)].some((id) => shown.has(id))) shown.add(s.id);
    }
    for (const s of nodes) {
      s.a.toggleAttribute('hidden', !shown.has(s.id));
      const text = `${s.id} ${s.n.name || ''} ${s.n.kind} ${s.env || ''} ${s.group} ${s.tags.join(' ')}`.toLowerCase();
      s.a.classList.toggle('pt-eg-dim', Boolean(search) && !text.includes(search));
    }
    for (const l of links) l.line.toggleAttribute('hidden', !shown.has(l.s.id) || !shown.has(l.t.id));
    svg.dataset.shown = String([...shown].filter((id) => byId.get(id).own).length);
    drawHulls();
  }

  // ---- selection: the node after #, pinned with its neighbours ----
  let selected = null;
  function pinAround(id) {
    for (const nid of [id, ...near.get(id)]) {
      const s = byId.get(nid);
      s.pin = true;
      s.fx = s.x;
      s.fy = s.y;
    }
    render();
  }
  function select(id) {
    selected = byId.has(id) ? id : null;
    for (const s of nodes) {
      const on = s.id === selected;
      s.a.classList.toggle('pt-eg-selected', on);
      if (on) s.a.setAttribute('aria-current', 'true');
      else s.a.removeAttribute('aria-current');
      s.a.classList.toggle('pt-eg-near', Boolean(selected) && near.get(selected).has(s.id));
    }
    svg.dataset.selected = selected || '';
    if (!id) {
      status.textContent = '';
    } else if (!selected) {
      status.textContent = `no node ${id} in the environments graph`;
      status.dataset.missing = id;
    } else {
      delete status.dataset.missing;
      status.textContent = `selected ${id} · ${near.get(id).size} neighbours pinned`;
      if (frame) pending = id;
      else pinAround(id);
    }
  }
  const fromHash = () => {
    try { return decodeURIComponent(location.hash.slice(1)); } catch { return location.hash.slice(1); }
  };
  // Inside this page a node's link selects it in place; anything else
  // routes through the shell.
  const go = (to) => {
    const u = new URL(to, location.href);
    if (`${u.pathname}${u.search}` === base && u.hash) {
      history.pushState(null, '', to);
      select(fromHash());
      return;
    }
    ctx.navigate(to);
  };
  h.onHash = () => select(fromHash());

  // ---- pointer: click selects, drag pins, double-click releases ----
  let drag = null;
  let dragged = false;
  const point = (ev) => {
    const m = svg.getScreenCTM();
    const p = new DOMPoint(ev.clientX, ev.clientY).matrixTransform(m ? m.inverse() : new DOMMatrix());
    return { x: p.x, y: p.y };
  };
  nodeLayer.addEventListener('pointerdown', (ev) => {
    const a = ev.target.closest('a.pt-eg-node');
    if (!a || ev.button !== 0) return;
    drag = { s: byId.get(a.dataset.id), x0: ev.clientX, y0: ev.clientY, id: ev.pointerId, moved: false };
    dragged = false;
  });
  svg.addEventListener('pointermove', (ev) => {
    if (!drag || ev.pointerId !== drag.id) return;
    if (!drag.moved && Math.hypot(ev.clientX - drag.x0, ev.clientY - drag.y0) < 4) return;
    if (!drag.moved) {
      drag.moved = true;
      try { svg.setPointerCapture(ev.pointerId); } catch { /* a pointer the browser no longer tracks */ }
      svg.classList.add('pt-eg-dragging');
    }
    const p = point(ev);
    drag.s.pin = true;
    drag.s.fx = p.x;
    drag.s.fy = p.y;
    reheat(0.3);
  });
  const endDrag = (ev) => {
    if (!drag || ev.pointerId !== drag.id) return;
    dragged = drag.moved;
    try { if (svg.hasPointerCapture(ev.pointerId)) svg.releasePointerCapture(ev.pointerId); } catch { /* as above */ }
    svg.classList.remove('pt-eg-dragging');
    drag = null;
  };
  svg.addEventListener('pointerup', endDrag);
  svg.addEventListener('pointercancel', endDrag);
  nodeLayer.addEventListener('click', (ev) => {
    const a = ev.target.closest('a.pt-eg-node');
    if (!a) return;
    if (dragged) {
      ev.preventDefault();
      dragged = false;
      return;
    }
    if (ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.altKey || ev.button !== 0) return;
    ev.preventDefault();
    go(a.getAttribute('href'));
  });
  nodeLayer.addEventListener('dblclick', (ev) => {
    const a = ev.target.closest('a.pt-eg-node');
    if (!a) return;
    ev.preventDefault();
    const s = byId.get(a.dataset.id);
    s.pin = false;
    reheat(0.3);
  });
  nodeLayer.addEventListener('contextmenu', (ev) => {
    const a = ev.target.closest('a.pt-eg-node');
    if (!a) return;
    ev.preventDefault();
    nodemenu.open(ev, byId.get(a.dataset.id).n, g, ctx.nodeLinks, go, a);
  });

  const onSearch = (ev) => { search = String(ev.detail || '').toLowerCase(); apply(); };
  ctx.bus.addEventListener('search', onSearch);
  h.off = () => {
    ctx.bus.removeEventListener('search', onSearch);
    if (frame) cancelAnimationFrame(frame);
    frame = 0;
  };
  apply();
  render();
  // The layout settles first, so a node selected on load is pinned with
  // its neighbours where the layout leaves them.
  settle();
  select(fromHash());
}

// legend says what colour, shape and line mean.
function legend() {
  const box = document.createElement('div');
  box.className = 'pt-eg-legend';
  const row = (title, items) => {
    const r = document.createElement('div');
    r.className = 'pt-eg-legend-row';
    const t = document.createElement('span');
    t.className = 'pt-muted';
    t.textContent = title;
    r.appendChild(t);
    for (const [label, draw] of items) {
      const item = document.createElement('span');
      item.className = 'pt-eg-key';
      const s = el('svg', { viewBox: '-12 -12 24 24', width: 18, height: 18, 'aria-hidden': 'true', class: 'pt-eg' });
      draw(s);
      item.append(s, document.createTextNode(label));
      r.appendChild(item);
    }
    box.appendChild(r);
  };
  const node = (shape, ikind, neighbour) => (s) => {
    const a = el('g', { class: `pt-eg-node${neighbour ? ' pt-eg-neighbour' : ''}` }, s);
    if (ikind) a.setAttribute('data-ikind', ikind);
    const sh = shapeEl(shape, neighbour ? 6 : 8);
    sh.setAttribute('class', 'pt-eg-shape');
    a.appendChild(sh);
  };
  const line = (kind) => (s) => {
    s.setAttribute('viewBox', '-14 -6 28 12');
    s.setAttribute('width', 28);
    el('line', { x1: -13, y1: 0, x2: 13, y2: 0, class: 'pt-eg-edge', 'data-kind': kind }, s);
  };
  row('colour', INSTANCE_KINDS.map((k) => [k, node('circle', k)]));
  row('shape', [['instance', node('circle', '')], ['image', node('square', '')], ['network', node('diamond', '')], ['other', node('hexagon', '')], ['one edge away', node('hexagon', '', true)]]);
  row('line', [['member-of', line('member-of')], ['runs-as', line('runs-as')], ['hosts', line('hosts')], ['other', line('other')]]);
  return box;
}
