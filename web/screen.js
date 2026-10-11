// Drawing screen nodes. The page keeps no UI model of its own for what a
// page shows: a page whose view is screen names a screen node (ui:canvas,
// ui:designer), and this draws it from GET /api/tenants/{tenant}/screen:
// Containers recursively by their contains edges and Layout, each Widget
// (and any Container that is-a a Category with a renderer) mounted by its
// Category. A Category without a renderer of its own uses the one it is-a,
// so a new panel type is a Category node and an is-a edge, never new code
// (docs/design.md 1). Moved here from portal.js in 0.4.0, when the shell
// took the page (docs/design.md 36).
import { parseSource } from '/static/client.js';
import * as canvas from '/static/canvas.js';
import * as table from '/static/table.js';
import * as detail from '/static/detail.js';
import * as designer from '/static/designer.js';
import { toast, guard } from '/static/toast.js';

// The base Categories: the only names bound to code.
const RENDERERS = { Canvas: canvas, Table: table, Detail: detail, Designer: designer };
// The Category whose Containers are menu entries (docs/design.md 25).
const MENU = 'Menu';

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
    // state is the node's own State, ui:state:<id without ui:>. A node may
    // hold more (the Designer's per section, ui:state:designer:<slug>), so
    // the first has-state edge is the fallback only for an id of another
    // shape, never a section's (docs/design.md 28).
    state(id) {
      const own = `ui:state:${id.replace(/^ui:/, '')}`;
      const all = targets(id, 'has-state');
      return (all.find((s) => s.id === own) || all.find((s) => !s.id.startsWith(`${own}:`)) || {}).properties || {};
    },
    theme: (id) => (targets(id, 'themed-by')[0] || {}).properties || null,
    category: (id) => targets(id, 'is-a')[0] || null,
    // menu is the screen graph's menu entries: the Containers that is-a
    // Menu, by order, each with the node its opens edge shows. Since 0.4.0
    // the shell's rail comes from the tenant's menu, not these; the
    // Designer still lists and adds them (docs/design.md 36).
    menu() {
      const cat = screen.nodes.find((n) => n.kind === 'Category' && n.name === MENU);
      if (!cat) return [];
      return screen.edges
        .filter((e) => e.kind === 'is-a' && e.to === cat.id && nodes.get(e.from)?.kind === 'Container')
        .map((e) => nodes.get(e.from))
        .sort((a, b) => order(a) - order(b) || a.id.localeCompare(b.id))
        .map((entry) => ({ entry, opens: targets(entry.id, 'opens')[0] || null }));
    },
    isRenderer: (name) => Object.hasOwn(RENDERERS, name),
    isMenuEntry(id) {
      return (out.get(id) || []).some((e) => e.kind === 'is-a' && nodes.get(e.to)?.name === MENU);
    },
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
      return screen.nodes.find((n) => n.kind === 'Container' && !inbound.has(n.id) && !this.isMenuEntry(n.id)) || null;
    },
  };
}

// sourceLayer is the layer a node draws: a Widget's source, else the first
// source among what a Container holds, depth first in drawing order.
export function sourceLayer(ix, node, seen = new Set()) {
  if (!node || seen.has(node.id)) return null;
  seen.add(node.id);
  const src = node.properties && node.properties.source;
  if (src) return parseSource(src).layer;
  for (const k of ix.children(node.id).filter(visible)) {
    const l = sourceLayer(ix, k, seen);
    if (l) return l;
  }
  return null;
}

// draw puts one screen node into el: a Container with what it holds, or a
// Widget, in its glass box. ctx is {client, bus, config, screen, ontology,
// layers, ix, readOnly, refresh} and, for a Designer, sections (the slugs to
// draw).
export function draw(ctx, node, el) {
  const box = node.kind === 'Container' ? drawContainer(ctx, node, null) : drawWidget(ctx, node, null);
  if (box) el.appendChild(box);
  return box;
}

// destroyAll unmounts every renderer draw mounted.
export function destroyAll() {
  for (const m of mounted) if (m.mod.destroy) m.mod.destroy(m.handle);
  mounted = [];
}

function visible(n) {
  return !(n.properties && n.properties.visible === false);
}

function drawContainer(ctx, c, parentMode) {
  if (!visible(c)) return null;
  const { ix } = ctx;
  const layout = ix.layout(c.id);
  const mode = (layout && layout.properties && layout.properties.mode) || 'flow';
  const state = ix.state(c.id);
  const { box, body } = chrome(ctx, c);
  box.classList.add('pt-container');
  if (state.collapsed) box.classList.add('pt-collapsed');
  place(box, c, parentMode);
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
      const el = k.kind === 'Container' ? drawContainer(ctx, k, mode) : drawWidget(ctx, k, mode);
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

// chrome is the card with a head: title, Category, collapse toggle.
// Collapsing writes the node's State through PATCH; display never lives on
// the node itself. While the viewer reads only (ctx.readOnly) the toggle is
// disabled; a refused PATCH is a toast, and the panel keeps what it draws
// (docs/design.md 37).
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
  guard(toggle, ctx.readOnly);
  toggle.addEventListener('click', async () => {
    try {
      await ctx.client.patchNode(n.id, { state: { collapsed: !collapsed } });
    } catch (err) {
      toast(err);
      return;
    }
    await ctx.refresh();
  });
  head.append(title, badge, toggle);
  const body = document.createElement('div');
  body.className = 'gr-box-body';
  box.append(head, body);
  return { box, body, head };
}

// applyTheme sets a Container's Theme as the stylesheet's tokens on its box,
// so it reaches everything inside that has no Theme of its own: color is the
// accent, opacity the card surface, font the font-family.
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
