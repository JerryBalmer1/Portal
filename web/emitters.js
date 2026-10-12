// Emitters/Catalogue, at /<tenant>/emitters: the roll call from GET
// /api/tenants/{tenant}/emitters (docs/design.md 44, 49). One row per name a
// card of type emitter or a mounted layer's source block gives, and one per
// module of layers with no source block: name (a link to its page), status,
// the card's version and the source.version its layers carry, the card's
// facets, the card (a link to its row on the feeds page), how many layers and
// how many nodes the tenant graph filtered to its name holds (docs/design.md
// 53). Right-click (or Shift+F10) on a row opens its menu (nodemenu.js): Show
// graph, /<tenant>/graph?source=<name>, and Open card, its page below; each
// greyed with why where it leads nowhere. Status and facet chips filter the rows and keep the filter in the URL
// (?status=, ?facet=), so a filtered list is a link; the banner's search
// filters them too. Each emitter's page, /<tenant>/emitters/<name>, from GET
// .../emitters/{name}, is its source: built from the card (module, version,
// repo, url, emits) with the layers that name it, newest first, each with its
// source block's version, target and stamp, and Show graph beside the
// version. Nothing here writes.
import { chips } from '/static/nodes.js';
import * as nodemenu from '/static/nodemenu.js';

// STATUSES are the status chips, in the order the roll call reads.
export const STATUSES = Object.freeze(['ok', 'version-mismatch', 'no-layers', 'no-card', 'unstamped']);

// WHY says what each status means, as the badge's title.
const WHY = {
  ok: 'A card, and every layer naming it in its source block carries the card\'s version.',
  'version-mismatch': 'A layer naming this card in its source block carries another version.',
  'no-layers': 'A card no mounted layer names in its source block.',
  'no-card': 'Mounted layers name this emitter in their source block; no feed of the tenant holds its card.',
  unstamped: 'The module of mounted layers with no source block. Never matched to a card.',
};

// mount draws the roll call into el. ctx: {client, toolbar, bus, navigate, emitterHref, cardHref, sourceGraphHref}.
export function mount(el, ctx) {
  const q = new URLSearchParams(location.search);
  const filter = { status: q.get('status') || '', facet: q.get('facet') || '' };
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  ctx.client.listEmitters().then((env) => {
    const ems = emittersOf(env);
    if (!ems.length) {
      count.textContent = '0 emitters';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      e.dataset.kind = 'Card';
      const t = document.createElement('h2');
      t.textContent = 'No emitters yet';
      const p = document.createElement('p');
      p.textContent = 'An emitter is a card of type emitter pushed to one of the tenant\'s feeds (Publish-PortalCard). A mounted layer belongs to the emitter its Graph.Node source block names (Mount-PortalLayer).';
      e.append(t, p);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-emitters';
    head(t, ['name', 'status', 'version', 'layer versions', 'facets', 'card', 'layers', 'nodes']);
    const body = t.createTBody();
    const rows = [];
    for (const em of ems) {
      const name = nameOf(em);
      const p = em.properties;
      const fs = facetsOf(em);
      const row = body.insertRow();
      row.dataset.emitter = name;
      row.dataset.status = p.status;
      row.insertCell().appendChild(go(ctx, ctx.emitterHref(name), name));
      row.insertCell().appendChild(statusBadge(p.status));
      row.insertCell().textContent = em.kind === 'Card' ? p.version : '';
      const lv = row.insertCell();
      lv.textContent = (p.layerVersions || []).join(', ');
      if (p.status === 'version-mismatch') lv.className = 'pt-mismatch';
      row.insertCell().textContent = fs.join(', ');
      const card = row.insertCell();
      if (em.kind === 'Card') card.appendChild(cardLink(ctx, em));
      row.insertCell().textContent = p.layers;
      row.insertCell().textContent = Number(p.nodes) || 0;
      rows.push({ row, em, status: p.status, facets: fs, text: `${name} ${p.status} ${p.version || ''} ${(p.layerVersions || []).join(' ')} ${fs.join(' ')} ${p.module || ''}`.toLowerCase() });
    }
    // Chips: every status, then the facets the cards emit.
    const facets = [...new Set(rows.flatMap((r) => r.facets))].sort();
    const groups = [];
    const group = (key, values, label) => {
      const [g] = chips(values, (v) => { filter[key] = v; keepInURL(filter); apply(); }, label);
      if (!g) return;
      g.dataset.chips = key;
      for (const b of g.querySelectorAll('button')) b.setAttribute('aria-pressed', String(b.dataset.value === filter[key]));
      ctx.toolbar.appendChild(g);
      groups.push(g);
    };
    group('status', STATUSES, 'filter by status');
    group('facet', facets, 'filter by facet');
    let search = '';
    const apply = () => {
      let shown = 0;
      for (const r of rows) {
        r.row.hidden = (!!filter.status && r.status !== filter.status) || (!!filter.facet && !r.facets.includes(filter.facet)) || (!!search && !r.text.includes(search));
        if (!r.row.hidden) shown++;
      }
      const tally = STATUSES.map((s) => [s, rows.filter((r) => r.status === s).length]).filter(([, n]) => n).map(([s, n]) => `${n} ${s}`).join(' · ');
      count.textContent = `${shown === rows.length ? rows.length : `${shown} of ${rows.length}`} emitters · ${tally}`;
    };
    const onSearch = (ev) => { search = String(ev.detail || '').toLowerCase(); apply(); };
    ctx.bus.addEventListener('search', onSearch);
    h.off = () => { ctx.bus.removeEventListener('search', onSearch); for (const g of groups) g.remove(); };
    apply();
    withMenu(t, rows, ctx);
    wrap.appendChild(t);
  }).catch((err) => fail(wrap, err));
  return h;
}

// menuItems is a roll-call row's menu, each {label, href, why}: Show graph,
// the tenant graph filtered to the emitter's name, greyed for an unstamped
// module, whose layers no source filter matches; Open card, the emitter's
// page built from its card, greyed where no feed holds one.
export function menuItems(em, links) {
  const name = nameOf(em);
  const unstamped = em.properties.status === 'unstamped';
  const graph = unstamped ? null : links.sourceGraphHref(name);
  return [
    { label: 'Show graph', href: graph, why: unstamped ? 'unstamped: its layers carry no source block, so no source filter matches them' : 'the menu has no tenant graph' },
    { label: 'Open card', href: em.kind === 'Card' ? links.emitterHref(name) : null, why: unstamped ? 'unstamped: no card is matched to a module' : 'no feed of the tenant holds a card of this name' },
  ];
}

// withMenu opens a row's menu on right-click (or Shift+F10).
function withMenu(t, rows, ctx) {
  t.dataset.menu = 'emitter';
  t.addEventListener('contextmenu', (ev) => {
    const row = ev.target.closest('tr[data-emitter]');
    const r = row && rows.find((x) => x.row === row);
    if (!r) return;
    ev.preventDefault();
    const at = { clientX: ev.clientX, clientY: ev.clientY };
    nodemenu.open(at, r.em, { nodes: [], edges: [] }, {}, ctx.navigate, row.querySelector('a') || row, menuItems(r.em, ctx));
  });
}

// open draws one emitter's source into el: its card and its layers. ctx: {client, toolbar, bus, name, navigate, layerHref, cardHref, sourceGraphHref}.
export function open(el, ctx) {
  const box = document.createElement('div');
  box.className = 'pt-tab-pane';
  el.appendChild(box);
  const h = { wrap: box, off: () => {} };
  const status = document.createElement('span');
  status.className = 'pt-emitter-card';
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'emitter-stats';
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.append(status, stats, count);
  ctx.client.getEmitter(ctx.name).then((env) => {
    const me = env.nodes.find((n) => n.id === env.root);
    const p = me.properties;
    const byId = new Map(env.nodes.map((n) => [n.id, n]));
    const layers = env.edges.filter((e) => e.kind === 'has-layer' && e.from === env.root).map((e) => byId.get(e.to)).filter(Boolean);
    stats.dataset.emitter = ctx.name;
    stats.textContent = `${layers.length} layers`;
    status.appendChild(statusBadge(p.status));
    box.appendChild(source(ctx, me));
    const wrap = document.createElement('div');
    wrap.className = 'pt-card pt-table-card';
    box.appendChild(wrap);
    if (!layers.length) {
      count.textContent = '0 layers';
      wrap.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No layers mounted';
      const s = document.createElement('p');
      s.textContent = `No mounted layer's source block names ${ctx.name}.`;
      wrap.append(t, s);
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-emitter-layers';
    head(t, ['name', 'source version', 'target', 'stamp', 'nodes', 'edges', 'principal']);
    const body = t.createTBody();
    const rows = [];
    for (const l of layers) {
      const lp = l.properties;
      const src = lp.source || {};
      const row = body.insertRow();
      row.dataset.layer = l.name;
      row.title = lp.version ? `${lp.module} ${lp.version}` : lp.module;
      row.insertCell().appendChild(go(ctx, ctx.layerHref(l.name), l.name));
      const v = row.insertCell();
      v.textContent = src.version || '';
      if (me.kind === 'Card' && src.version && src.version !== p.version) {
        row.classList.add('pt-mismatch');
        v.className = 'pt-mismatch';
        v.title = `The card is ${p.version}.`;
      }
      const who = lp.mountedBy ? lp.mountedBy.replace(/^ui:principal:/, '') : '';
      for (const x of [src.target || '', src.stamp || lp.stamp, lp.nodes, lp.edges, who]) row.insertCell().textContent = x;
      rows.push({ row, text: `${l.name} ${src.version || ''} ${src.target || ''} ${src.stamp || lp.stamp} ${who}`.toLowerCase() });
    }
    h.off = searchable(ctx.bus, rows, count, 'layers');
    wrap.appendChild(t);
  }).catch((err) => fail(box, err));
  return h;
}

export function destroy(h) {
  nodemenu.close(false);
  h.off();
  h.wrap.remove();
}

// source is the emitter's card as a definition list, its emits as a table:
// module, version with Show graph beside it, repo (a link to url), url (Open
// in source), the card (a link to its row on the feeds page) and status. A
// no-card name has no card and says so, Show graph beside its name; an
// unstamped module has neither.
function source(ctx, me) {
  const p = me.properties;
  const card = document.createElement('div');
  card.className = 'pt-card pt-source';
  card.dataset.emitter = nameOf(me);
  const dl = document.createElement('dl');
  const row = (label, value) => {
    const dt = document.createElement('dt');
    dt.textContent = label;
    const dd = document.createElement('dd');
    dd.dataset.field = label;
    if (value instanceof Node) dd.appendChild(value); else dd.textContent = value ?? '';
    dl.append(dt, dd);
    return dd;
  };
  if (me.kind !== 'Card') {
    const unstamped = p.status === 'unstamped';
    const named = row(unstamped ? 'module' : 'emitter', me.name);
    if (!unstamped) showGraph(ctx, me, named);
    row('status', statusBadge(p.status));
    if (p.layerVersions) row('layer versions', p.layerVersions.join(', '));
    row('card', p.status === 'unstamped' ? 'none: its layers carry no source block, so no card is matched' : 'none: no feed of the tenant holds a card of this name');
    card.appendChild(dl);
    return card;
  }
  row('module', p.module || '');
  showGraph(ctx, me, row('version', p.version));
  if (p.status === 'version-mismatch') {
    const s = document.createElement('span');
    s.className = 'pt-mismatch';
    s.textContent = (p.layerVersions || []).join(', ');
    row('layer versions', s);
  } else if (p.layerVersions) {
    row('layer versions', p.layerVersions.join(', '));
  }
  row('repo', p.repo ? (p.url ? external(p.url, p.repo) : p.repo) : '');
  row('url', p.url ? external(p.url, 'Open in source', 'pt-open-source') : '');
  row('card', cardLink(ctx, me));
  row('status', statusBadge(p.status));
  card.appendChild(dl);
  const emits = Array.isArray(p.emits) ? p.emits : [];
  const h2 = document.createElement('h2');
  h2.textContent = 'emits';
  card.appendChild(h2);
  if (!emits.length) {
    const s = document.createElement('p');
    s.className = 'pt-emitter-card';
    s.textContent = 'The push recorded no emits (a card pushed before 0.13.0).';
    card.appendChild(s);
    return card;
  }
  const t = document.createElement('table');
  t.className = 'pt-table pt-emits';
  head(t, ['facet', 'kinds']);
  const body = t.createTBody();
  for (const e of emits) {
    const r = body.insertRow();
    r.dataset.facet = e.facet;
    r.insertCell().textContent = e.facet;
    const k = r.insertCell();
    for (const kind of e.kinds || []) {
      const s = document.createElement('span');
      s.className = 'pt-kind';
      s.textContent = kind;
      k.appendChild(s);
    }
  }
  card.appendChild(t);
  return card;
}

// showGraph appends Show graph to el: a plain link to the tenant graph
// filtered to the emitter's name, as the roll call's menu has it; nothing
// when the menu has no tenant graph.
function showGraph(ctx, em, el) {
  const name = nameOf(em);
  const to = ctx.sourceGraphHref && ctx.sourceGraphHref(name);
  if (!to) return;
  const a = go(ctx, to, 'Show graph');
  a.className = 'pt-show-graph';
  a.dataset.source = name;
  a.title = `${Number(em.properties.nodes) || 0} nodes in the tenant graph from the layers whose source block names ${name}`;
  el.appendChild(a);
}

// emittersOf is the envelope's roll call, Card and Emitter nodes, in
// envelope order (by name, as the API serves them).
function emittersOf(env) {
  return env.nodes.filter((n) => n.kind === 'Card' || n.kind === 'Emitter');
}

function nameOf(em) {
  return em.properties.emitter || em.name;
}

// facetsOf is the facets an emitter's card emits; none without a card.
function facetsOf(em) {
  return Array.isArray(em.properties.emits) ? em.properties.emits.map((e) => e.facet) : [];
}

// statusBadge is a status as a badge, its meaning as its title.
export function statusBadge(status) {
  const b = document.createElement('span');
  b.className = 'pt-status';
  b.dataset.status = status;
  b.textContent = status;
  b.title = WHY[status] || '';
  return b;
}

// keepInURL puts the chips' filter in the address, so a filtered list is a link.
function keepInURL(filter) {
  const q = new URLSearchParams(location.search);
  for (const [k, v] of Object.entries(filter)) { if (v) q.set(k, v); else q.delete(k); }
  const s = q.toString();
  history.replaceState(history.state, '', location.pathname + (s ? `?${s}` : '') + location.hash);
}

// cardLink is the card's name and version, linking to its row on the feeds
// page; plain text when the menu has no feeds page.
function cardLink(ctx, em) {
  const text = `${em.name}@${em.properties.version}`;
  const to = ctx.cardHref(em.id);
  if (!to) return document.createTextNode(text);
  const a = go(ctx, to, text);
  a.className = 'pt-card-link';
  a.dataset.card = em.id;
  return a;
}

// external is a link out of the portal, in a new tab.
function external(href, text, className) {
  const a = document.createElement('a');
  a.href = href;
  a.textContent = text;
  a.target = '_blank';
  a.rel = 'noopener noreferrer';
  if (className) a.className = className;
  return a;
}

function head(t, names) {
  const row = t.createTHead().insertRow();
  for (const name of names) {
    const th = document.createElement('th');
    th.textContent = name;
    row.appendChild(th);
  }
}

// go is a link that routes inside the shell on a plain click.
function go(ctx, to, text) {
  const a = document.createElement('a');
  a.href = to;
  a.textContent = text;
  a.addEventListener('click', (ev) => {
    if (ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.button !== 0) return;
    ev.preventDefault();
    ctx.navigate(a.getAttribute('href'));
  });
  return a;
}

function searchable(bus, rows, count, noun) {
  let search = '';
  const apply = () => {
    let shown = 0;
    for (const r of rows) {
      r.row.hidden = !!search && !r.text.includes(search);
      if (!r.row.hidden) shown++;
    }
    count.textContent = shown === rows.length ? `${rows.length} ${noun}` : `${shown} of ${rows.length} ${noun}`;
  };
  const onSearch = (ev) => { search = String(ev.detail || '').toLowerCase(); apply(); };
  bus.addEventListener('search', onSearch);
  apply();
  return () => bus.removeEventListener('search', onSearch);
}

function fail(wrap, err) {
  wrap.classList.add('pt-error-card');
  wrap.textContent = `${err.code}: ${err.message}`;
}
