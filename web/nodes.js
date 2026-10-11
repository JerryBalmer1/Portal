// The table page: every node of the page's Kind (its Page node's nodeKind)
// on the screen and in the mounted layers, from GET
// /api/tenants/{tenant}/nodes?kind=, one row each (id, kind, label,
// updated), or an empty state naming the page. A row opens the node's item
// page, /<tenant>/<service>/<page>/<id>. A page with filter offers its
// values as toolbar chips over properties.kind; the banner's search box
// filters the rows on screen. On a page of an environment.* Kind a row's
// right-click (or Shift+F10) opens the node menu (nodemenu.js; docs/design.md
// 46), whose links read the environments graph. Nothing here writes.
import * as nodemenu from '/static/nodemenu.js';

// mount draws into el. ctx: {client, page, toolbar, bus, href, navigate}.
export function mount(el, ctx) {
  const kind = ctx.page.properties.nodeKind;
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  let filter = '';
  let search = '';
  let apply = () => {};
  // Filter chips show before the rows arrive, and with none, so the page
  // says what it offers.
  for (const chip of chips(ctx.page.properties.filter, (v) => { filter = v; apply(); })) ctx.toolbar.appendChild(chip);
  ctx.client.listNodes(kind).then((env) => {
    if (!env.nodes.length) {
      count.textContent = `0 ${kind}`;
      const e = empty(ctx.page.name, kind);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const { t, rows } = table(env.nodes, { href: ctx.href, navigate: ctx.navigate });
    apply = () => {
      let shown = 0;
      for (const r of rows) {
        const ok = (!filter || r.n.properties?.kind === filter) && (!search || r.text.includes(search));
        r.row.hidden = !ok;
        if (ok) shown++;
      }
      count.textContent = shown === rows.length ? `${rows.length} ${kind}` : `${shown} of ${rows.length} ${kind}`;
    };
    const onSearch = (ev) => { search = String(ev.detail || '').toLowerCase(); apply(); };
    ctx.bus.addEventListener('search', onSearch);
    h.off = () => ctx.bus.removeEventListener('search', onSearch);
    if (String(kind).startsWith('environment.') && ctx.nodeLinks) withMenu(t, rows, ctx);
    apply();
    wrap.appendChild(t);
  }).catch((err) => {
    wrap.classList.add('pt-error-card');
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return h;
}

// table is one row per node (id, kind, label, updated), the id a link to
// opts.href(id) inside the shell when href is given; a node with no stamp
// shows opts.updated. It returns the table and, per row, the node and the
// text the search box matches.
export function table(list, opts = {}) {
  const t = document.createElement('table');
  t.className = 'pt-table';
  const head = t.createTHead().insertRow();
  for (const name of ['id', 'kind', 'label', 'updated']) {
    const th = document.createElement('th');
    th.textContent = name;
    head.appendChild(th);
  }
  const body = t.createTBody();
  const rows = list.map((n) => {
    const row = body.insertRow();
    row.dataset.id = n.id;
    const cell = row.insertCell();
    if (opts.href) {
      const link = document.createElement('a');
      link.href = opts.href(n.id);
      link.textContent = n.id;
      link.addEventListener('click', (ev) => {
        if (ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.button !== 0) return;
        ev.preventDefault();
        opts.navigate(link.getAttribute('href'));
      });
      cell.appendChild(link);
    } else {
      cell.textContent = n.id;
    }
    for (const v of [n.kind, labelOf(n), updatedOf(n) || opts.updated || '']) row.insertCell().textContent = v;
    return { row, n, text: `${n.id} ${n.kind} ${labelOf(n)}`.toLowerCase() };
  });
  return { t, rows };
}

export function destroy(h) {
  nodemenu.close(false);
  h.off();
  h.wrap.remove();
}

// withMenu opens the node menu on a row's right-click. The environments
// graph, whose edges say which items link where, is read on the first.
function withMenu(t, rows, ctx) {
  let graph = null;
  t.dataset.menu = 'node';
  t.addEventListener('contextmenu', async (ev) => {
    const row = ev.target.closest('tr[data-id]');
    const r = row && rows.find((x) => x.row === row);
    if (!r) return;
    ev.preventDefault();
    graph ||= ctx.client.getEnvironmentGraph().catch(() => ({ nodes: [], edges: [] }));
    const g = await graph;
    if (!row.isConnected) return;
    const at = { clientX: ev.clientX, clientY: ev.clientY };
    nodemenu.open(at, r.n, g, ctx.nodeLinks, ctx.navigate, row.querySelector('a') || row);
  });
}

// chips is one toggle per filter value, and "all" first; choosing one calls
// pick with the value ('' for all). label names the group for a screen
// reader.
export function chips(values, pick, label = 'filter by kind') {
  if (!Array.isArray(values) || !values.length) return [];
  const group = document.createElement('div');
  group.className = 'pt-chips';
  group.setAttribute('role', 'group');
  group.setAttribute('aria-label', label);
  const all = ['', ...values];
  const buttons = all.map((v) => {
    const b = document.createElement('button');
    b.type = 'button';
    b.className = 'pt-chip';
    b.textContent = v || 'all';
    b.dataset.value = v;
    b.setAttribute('aria-pressed', String(v === ''));
    b.addEventListener('click', () => {
      for (const x of buttons) x.setAttribute('aria-pressed', String(x === b));
      pick(v);
    });
    group.appendChild(b);
    return b;
  });
  return [group];
}

// empty is the empty state: the page's name and the Kind it waits for.
export function empty(name, kind) {
  const box = document.createElement('div');
  box.className = 'pt-card pt-empty-state';
  box.dataset.kind = kind;
  const h = document.createElement('h2');
  h.textContent = `No ${name.toLowerCase()} yet`;
  const p = document.createElement('p');
  p.textContent = `Nothing of kind ${kind} is on the screen or in a mounted layer. A layer that emits ${kind} nodes fills this page.`;
  box.append(h, p);
  return box;
}

export function labelOf(n) {
  return n.name || n.properties?.label || '';
}

// updatedOf is when the node was last seen: its stamp's lastSeen, an
// updated property, else the stamp of the envelope it came from.
export function updatedOf(n) {
  return n.stamp?.lastSeen || n.properties?.updated || n.stamp?.layer || '';
}
