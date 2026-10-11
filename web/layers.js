// Graph/Layers: the mounted data layers from GET
// /api/tenants/{tenant}/layers, one row each (name, module, nodes, edges,
// stamp, path), the name a link to the layer's own page. The layer page,
// /<tenant>/layers/<name>, where mountLayer's Location points, shows one
// layer: its counts and stamp in the toolbar and its nodes from GET
// .../layers/{name}/nodes as the table pages draw nodes (nodes.js), the
// banner's search box filtering them (docs/design.md 42). A node id after #
// in the URL marks that node's row and scrolls to it: an agent's decision
// links there (docs/design.md 43). Nothing here writes; a layer is mounted
// through the API (Mount-PortalLayer).
import * as nodes from '/static/nodes.js';

// mount draws the list into el. ctx: {client, toolbar, bus, navigate, layerHref}.
export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  ctx.client.listLayers().then((layers) => {
    count.textContent = `${layers.length} layers`;
    if (!layers.length) {
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      const h = document.createElement('h2');
      h.textContent = 'No layers mounted';
      const p = document.createElement('p');
      p.textContent = 'A layer is mounted with POST /api/tenants/<tenant>/layers (Mount-PortalLayer); it is read-only here.';
      e.append(h, p);
      wrap.replaceWith(e);
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-layers';
    const head = t.createTHead().insertRow();
    for (const name of ['name', 'module', 'nodes', 'edges', 'stamp', 'path']) {
      const th = document.createElement('th');
      th.textContent = name;
      head.appendChild(th);
    }
    const body = t.createTBody();
    for (const l of layers) {
      const row = body.insertRow();
      row.dataset.layer = l.name;
      const a = document.createElement('a');
      a.href = ctx.layerHref(l.name);
      a.textContent = l.name;
      a.addEventListener('click', (ev) => {
        if (ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.button !== 0) return;
        ev.preventDefault();
        ctx.navigate(a.getAttribute('href'));
      });
      row.insertCell().appendChild(a);
      for (const v of [l.version ? `${l.module} ${l.version}` : l.module, l.counts.nodes, l.counts.edges, l.stamp, l.path || '']) row.insertCell().textContent = v;
    }
    wrap.appendChild(t);
  }).catch((err) => {
    wrap.classList.add('pt-error-card');
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return { wrap, off: () => {} };
}

// open draws one layer into el. ctx: {client, toolbar, bus, name}.
export function open(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'layer-stats';
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.append(stats, count);
  Promise.all([ctx.client.listLayers(), ctx.client.getLayerNodes(ctx.name)]).then(([layers, env]) => {
    const l = layers.find((x) => x.name === ctx.name);
    if (l) {
      stats.dataset.layer = l.name;
      stats.title = `${l.version ? `${l.module} ${l.version}` : l.module}${l.path ? ` from ${l.path}` : ''}`;
      stats.textContent = `${l.counts.nodes} nodes · ${l.counts.edges} edges · ${l.stamp}`;
    }
    const { t, rows } = nodes.table(env.nodes, { updated: env.layer });
    let search = '';
    const apply = () => {
      const shown = rows.filter((r) => { r.row.hidden = !!search && !r.text.includes(search); return !r.row.hidden; }).length;
      count.textContent = shown === rows.length ? `${rows.length} nodes` : `${shown} of ${rows.length} nodes`;
    };
    const onSearch = (ev) => { search = String(ev.detail || '').toLowerCase(); apply(); };
    ctx.bus.addEventListener('search', onSearch);
    h.off = () => ctx.bus.removeEventListener('search', onSearch);
    apply();
    wrap.appendChild(t);
    const target = decodeURIComponent(location.hash.slice(1));
    const hit = target && rows.find((r) => r.row.dataset.id === target);
    if (hit) {
      hit.row.classList.add('pt-target');
      hit.row.setAttribute('aria-current', 'true');
      hit.row.scrollIntoView({ block: 'center' });
    }
  }).catch((err) => {
    wrap.classList.add('pt-error-card');
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return h;
}

export function destroy(h) {
  h.off();
  h.wrap.remove();
}
