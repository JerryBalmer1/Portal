// Graph/Explore's query bar and what it draws (docs/design.md 54). The bar
// sits at the top of the Page whose query is true; its fields are the URL's
// query string, never the page's state, so a query is a link:
// /<tenant>/graph/explore?from=<ids>&follow=<kinds>&direction=in&depth=2&until=<Kind>.
// With from or kind in the URL, the page draws GET .../query's result with
// the tenant graph's renderer (graph.js) and lists its paths under it, each
// id a button that selects and centres its node; without, the page's screen
// (Explore's Canvases) is drawn as before. kind, where, source and limit,
// which the bar has no field for, ride along as they came. Nothing here
// writes.
import { draw, merged } from '/static/graph.js';

// PARAMS are the query's parameters in the URL's order.
export const PARAMS = ['from', 'kind', 'follow', 'direction', 'depth', 'until', 'where', 'source', 'limit'];

// FIELDS are the bar's inputs; every other parameter is carried hidden.
const FIELDS = [
  { name: 'from', label: 'from', placeholder: 'node ids, comma-separated' },
  { name: 'follow', label: 'follow', placeholder: 'edge kinds; every kind' },
  { name: 'direction', label: 'direction' },
  { name: 'depth', label: 'depth', placeholder: '3', type: 'number' },
  { name: 'until', label: 'until', placeholder: 'a Kind' },
];
const DIRECTIONS = [['', 'out (default)'], ['out', 'out'], ['in', 'in'], ['both', 'both']];
// PATHS_SHOWN caps the paths listed; the count says how many there are.
const PATHS_SHOWN = 500;

// asked is whether q holds a query: from or kind.
export function asked(q) {
  return !!(q.get('from') || q.get('kind'));
}

// search is the query string for q's values: PARAMS order, empty ones left
// out, where repeated. Anything else in q is dropped.
export function search(q) {
  const out = new URLSearchParams();
  for (const k of PARAMS) {
    for (const v of q.getAll(k)) if (v.trim() !== '') out.append(k, v.trim());
  }
  return out.toString();
}

// bar is the form for q; go(search) runs the query its fields give.
export function bar(q, go) {
  const form = document.createElement('form');
  form.className = 'pt-card pt-query';
  form.dataset.form = 'query';
  form.setAttribute('role', 'search');
  form.setAttribute('aria-label', 'Query the tenant graph');
  for (const f of FIELDS) {
    const wrap = document.createElement('label');
    wrap.className = `pt-query-field pt-query-${f.name}`;
    const t = document.createElement('span');
    t.textContent = f.label;
    let input;
    if (f.name === 'direction') {
      input = document.createElement('select');
      for (const [v, text] of DIRECTIONS) {
        const o = document.createElement('option');
        o.value = v;
        o.textContent = text;
        input.appendChild(o);
      }
    } else {
      input = document.createElement('input');
      input.type = f.type || 'text';
      input.placeholder = f.placeholder;
      input.autocomplete = 'off';
      input.spellcheck = false;
      if (f.type === 'number') Object.assign(input, { min: 0, max: 10, step: 1 });
    }
    input.className = 'gr-input';
    input.name = f.name;
    input.value = q.get(f.name) || '';
    wrap.append(t, input);
    form.appendChild(wrap);
  }
  for (const k of PARAMS.filter((p) => !FIELDS.some((f) => f.name === p))) {
    for (const v of q.getAll(k)) {
      const h = document.createElement('input');
      h.type = 'hidden';
      h.name = k;
      h.value = v;
      form.appendChild(h);
    }
  }
  const run = document.createElement('button');
  run.type = 'submit';
  run.className = 'gr-btn';
  run.textContent = 'Query';
  form.appendChild(run);
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    go(search(new URLSearchParams(new FormData(form))));
  });
  return form;
}

// mount draws the result of q into el. ctx: the page context and q.
export function mount(el, ctx) {
  const q = ctx.q;
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-graph';
  const paths = document.createElement('div');
  paths.className = 'pt-card pt-paths';
  el.append(wrap, paths);
  const h = { wrap, paths, cy: null, gone: false, ro: null };
  const count = document.createElement('span');
  count.className = 'pt-count';
  count.id = 'query-stats';
  ctx.toolbar.appendChild(count);
  const ask = {};
  for (const k of PARAMS) ask[k] = k === 'where' ? q.getAll(k) : q.get(k) || undefined;
  Promise.all([ctx.client.queryTenantGraph(ask), ctx.client.getOntology()]).then(([g, ontology]) => {
    if (h.gone) return;
    count.textContent = `${g.nodes.length} nodes · ${g.edges.length} edges · ${g.paths.length} paths`;
    Object.assign(count.dataset, { nodes: String(g.nodes.length), edges: String(g.edges.length), paths: String(g.paths.length) });
    if (g.truncated) {
      const t = document.createElement('span');
      t.className = 'pt-filter pt-truncated';
      t.dataset.hop = String(g.truncatedAt);
      t.textContent = `truncated at hop ${g.truncatedAt}: limit ${g.query.limit} nodes`;
      ctx.toolbar.appendChild(t);
    }
    if (!g.nodes.length) {
      wrap.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No nodes';
      const p = document.createElement('p');
      p.textContent = q.get('kind') && !q.get('from')
        ? `No mounted node of Kind ${q.get('kind')} passes the query.`
        : 'No start passes the where filters.';
      wrap.append(t, p);
      paths.remove();
      return;
    }
    draw(h, ctx, g, merged(ontology));
    drawPaths(h, g.paths);
  }).catch((err) => {
    wrap.className = 'pt-card pt-error-card';
    const pre = document.createElement('pre');
    pre.className = 'gr-error';
    pre.textContent = `${err.code}: ${err.message}`;
    wrap.replaceChildren(pre);
    paths.remove();
  });
  return h;
}

export function destroy(h) {
  h.gone = true;
  if (h.ro) h.ro.disconnect();
  if (h.cy) h.cy.destroy();
  h.wrap.remove();
  h.paths.remove();
}

// drawPaths lists the paths as id chains; an id selects and centres its node.
function drawPaths(h, list) {
  const head = document.createElement('h2');
  head.textContent = list.length > PATHS_SHOWN ? `Paths (first ${PATHS_SHOWN} of ${list.length})` : 'Paths';
  const ol = document.createElement('ol');
  for (const p of list.slice(0, PATHS_SHOWN)) {
    const li = document.createElement('li');
    li.dataset.path = p.join('>');
    p.forEach((id, i) => {
      if (i) li.append(' → ');
      const b = document.createElement('button');
      b.type = 'button';
      b.className = 'pt-path-id';
      b.dataset.id = id;
      b.textContent = id;
      b.title = `select ${id} in the graph`;
      b.addEventListener('click', () => {
        if (!h.cy) return;
        const n = h.cy.getElementById(id);
        h.cy.elements().unselect();
        n.select();
        h.cy.animate({ center: { eles: n }, duration: 200 });
      });
      li.appendChild(b);
    });
    ol.appendChild(li);
  }
  h.paths.append(head, ol);
}
