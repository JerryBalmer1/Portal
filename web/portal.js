// The portal page. It has no UI model of its own: it fetches the screen graph
// (GET /api/screen), draws Containers recursively from the root by their
// contains edges and Layout, and mounts each Widget (and any Container that
// is-a a Category with a renderer) by its Category. A Category without a
// renderer of its own uses the one it is-a, so a new panel type is a Category
// node and an is-a edge, never new code (docs/design.md 1).
import { createClient } from '/static/client.js';
import * as canvas from '/static/canvas.js';
import * as table from '/static/table.js';
import * as detail from '/static/detail.js';
import * as designer from '/static/designer.js';

// The base Categories: the only names bound to code.
const RENDERERS = { Canvas: canvas, Table: table, Detail: detail, Designer: designer };

const client = createClient();
const bus = new EventTarget();
const screenEl = document.getElementById('screen');
const statsEl = document.getElementById('stats');
let mounted = [];

// index builds the lookups the page draws from.
export function index(screen) {
  const nodes = new Map(screen.nodes.map((n) => [n.id, n]));
  const out = new Map();
  const inbound = new Set();
  for (const e of screen.edges) {
    if (!out.has(e.from)) out.set(e.from, []);
    out.get(e.from).push(e);
    if (e.kind === 'contains') inbound.add(e.to);
  }
  const targets = (id, kind) => (out.get(id) || []).filter((e) => e.kind === kind).map((e) => nodes.get(e.to)).filter(Boolean);
  const order = (n) => (n.properties && Number.isFinite(Number(n.properties.order)) ? Number(n.properties.order) : 0);
  return {
    nodes,
    targets,
    children: (id) => targets(id, 'contains').sort((a, b) => order(a) - order(b) || a.id.localeCompare(b.id)),
    layout: (id) => targets(id, 'laid-out-by')[0] || null,
    state: (id) => (targets(id, 'has-state')[0] || {}).properties || {},
    theme: (id) => (targets(id, 'themed-by')[0] || {}).properties || null,
    category: (id) => targets(id, 'is-a')[0] || null,
    // base walks is-a from a node until a Category bound to a renderer.
    base(id) {
      const seen = new Set();
      let cat = this.category(id);
      while (cat && !seen.has(cat.id)) {
        if (RENDERERS[cat.name]) return cat.name;
        seen.add(cat.id);
        cat = this.category(cat.id);
      }
      return null;
    },
    root() {
      if (screen.root && nodes.get(screen.root)?.kind === 'Container') return nodes.get(screen.root);
      return screen.nodes.find((n) => n.kind === 'Container' && !inbound.has(n.id)) || null;
    },
  };
}

async function refresh() {
  try {
    const [screen, ontology] = await Promise.all([client.getScreen(), client.getOntology()]);
    draw(screen, ontology);
  } catch (err) {
    showError(screenEl, err);
  }
}

function draw(screen, ontology) {
  for (const m of mounted) if (m.mod.destroy) m.mod.destroy(m.handle);
  mounted = [];
  screenEl.replaceChildren();
  statsEl.textContent = `${screen.nodes.length} nodes · ${screen.edges.length} edges · ${screen.layer}`;
  const ix = index(screen);
  const root = ix.root();
  if (!root) {
    const p = document.createElement('p');
    p.className = 'pt-empty';
    p.textContent = 'The screen graph is empty: POST a Container to /api/nodes.';
    screenEl.appendChild(p);
    return;
  }
  const ctx = { client, bus, screen, ontology, ix, refresh };
  const el = drawContainer(ctx, root, null, true);
  if (el) screenEl.appendChild(el);
}

function visible(n) {
  return !(n.properties && n.properties.visible === false);
}

function drawContainer(ctx, c, parentMode, isRoot) {
  if (!visible(c)) return null;
  const { ix } = ctx;
  const layout = ix.layout(c.id);
  const mode = (layout && layout.properties && layout.properties.mode) || 'flow';
  const state = ix.state(c.id);
  let box;
  let body;
  if (isRoot) {
    box = document.createElement('div');
    box.className = 'pt-container pt-root';
    body = box;
  } else {
    ({ box, body } = chrome(ctx, c));
    box.classList.add('pt-container');
    if (state.collapsed) box.classList.add('pt-collapsed');
    place(box, c, parentMode);
  }
  box.dataset.id = c.id;
  applyTheme(box, ix.theme(c.id));
  const base = ix.base(c.id);
  if (base) mountRenderer(ctx, base, c, body);
  const kids = ix.children(c.id).filter(visible);
  if (kids.length) {
    const inner = document.createElement('div');
    inner.className = `pt-${mode}`;
    if (mode === 'grid') {
      const cols = Number(layout.properties.columns) || 12;
      inner.style.gridTemplateColumns = `repeat(${cols}, minmax(0, 1fr))`;
    }
    for (const k of kids) {
      const el = k.kind === 'Container' ? drawContainer(ctx, k, mode, false) : drawWidget(ctx, k, mode);
      if (el) inner.appendChild(el);
    }
    body.appendChild(inner);
  }
  return box;
}

function drawWidget(ctx, w, mode) {
  const { box, body } = chrome(ctx, w);
  box.classList.add('pt-widget');
  box.dataset.id = w.id;
  if (ctx.ix.state(w.id).collapsed) box.classList.add('pt-collapsed');
  place(box, w, mode);
  const base = ctx.ix.base(w.id);
  if (!base) {
    const cat = ctx.ix.category(w.id);
    showError(body, { code: 'no-renderer', message: cat ? `Category ${cat.name} is-a no base Category (${Object.keys(RENDERERS).join(', ')})` : `${w.id} has no is-a edge to a Category` });
  } else {
    mountRenderer(ctx, base, w, body);
  }
  return box;
}

// chrome is the glass box with a head: title, Category, collapse toggle.
// Collapsing writes the node's State through PATCH; display never lives on
// the node itself.
function chrome(ctx, n) {
  const box = document.createElement('div');
  box.className = 'gr-box';
  const head = document.createElement('div');
  head.className = 'gr-box-head';
  const title = document.createElement('span');
  title.className = 'gr-box-title';
  title.textContent = n.name || n.id;
  const cat = ctx.ix.category(n.id);
  const badge = document.createElement('span');
  badge.className = 'pt-badge';
  badge.textContent = cat ? cat.name : n.kind;
  const toggle = document.createElement('button');
  toggle.className = 'gr-btn';
  const collapsed = !!ctx.ix.state(n.id).collapsed;
  toggle.textContent = collapsed ? 'expand' : 'collapse';
  toggle.addEventListener('click', async () => {
    try {
      await ctx.client.patchNode(n.id, { state: { collapsed: !collapsed } });
      await ctx.refresh();
    } catch (err) {
      showError(body, err);
    }
  });
  head.append(title, badge, toggle);
  const body = document.createElement('div');
  body.className = 'gr-box-body';
  box.append(head, body);
  return { box, body, head };
}

// applyTheme sets a Container's Theme as the stylesheet's tokens on its box,
// so it reaches everything inside that has no Theme of its own: color is the
// accent, opacity the glass surface, font the font-family.
function applyTheme(el, t) {
  if (!t) return;
  if (t.color) el.style.setProperty('--gr-accent', t.color);
  if (Number.isFinite(Number(t.opacity))) el.style.setProperty('--gr-surface', `rgba(255,255,255,${Number(t.opacity)})`);
  if (t.font) el.style.fontFamily = t.font;
}

// place applies bounds: grid cells under grid, pixels under canvas.
function place(el, n, mode) {
  const b = n.properties && n.properties.bounds;
  if (!b) return;
  if (mode === 'grid') {
    el.style.gridColumn = `${b.x + 1} / span ${b.w}`;
    el.style.gridRow = `${b.y + 1} / span ${b.h}`;
  } else if (mode === 'canvas') {
    Object.assign(el.style, { left: `${b.x}px`, top: `${b.y}px`, width: `${b.w}px`, height: `${b.h}px` });
  }
}

function mountRenderer(ctx, base, node, el) {
  const mod = RENDERERS[base];
  const host = document.createElement('div');
  host.className = 'pt-host';
  el.appendChild(host);
  try {
    mounted.push({ mod, handle: mod.mount(host, { ...ctx, node }) });
  } catch (err) {
    showError(host, err);
  }
}

export function showError(el, err) {
  const pre = document.createElement('pre');
  pre.className = 'gr-error';
  pre.textContent = JSON.stringify(err && err.code ? err : { code: 'error', message: String(err) }, null, 2);
  el.appendChild(pre);
}

refresh();
