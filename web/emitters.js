// Emitters/Catalogue, at /<tenant>/emitters: the tenant's emitters from GET
// /api/tenants/{tenant}/emitters (docs/design.md 44). An emitter is a card of
// type emitter in one of the tenant's feeds, under the card's own id; an
// emitter a mounted layer names with no card in any feed is still listed,
// in amber and flagged unknown, with no card link. One row each (name,
// version, feed, card, layers), the name a link to the emitter's page,
// /<tenant>/emitters/<name>, which lists the layers it produced from GET
// .../emitters/{name}, newest first: name (linking to the layer's page),
// nodes, edges, stamp and the principal that mounted it. The card links to
// its row on the feeds page. The banner's search filters the rows. Nothing
// here writes.

// mount draws the list into el. ctx: {client, toolbar, bus, navigate, emitterHref, cardHref}.
export function mount(el, ctx) {
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
      p.textContent = 'An emitter is a card of type emitter pushed to one of the tenant\'s feeds (Publish-PortalCard), or the module a mounted layer\'s ontology names (Mount-PortalLayer).';
      e.append(t, p);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-emitters';
    head(t, ['name', 'version', 'feed', 'card', 'layers']);
    const body = t.createTBody();
    const rows = [];
    for (const em of ems) {
      const name = nameOf(em);
      const p = em.properties;
      const row = body.insertRow();
      row.dataset.emitter = name;
      const cell = row.insertCell();
      cell.appendChild(go(ctx, ctx.emitterHref(name), name));
      if (p.unknown) {
        row.className = 'pt-unknown';
        cell.append(' ', unknownBadge());
        for (let i = 0; i < 3; i++) row.insertCell().textContent = '';
      } else {
        row.insertCell().textContent = p.version;
        row.insertCell().textContent = p.feed;
        row.insertCell().appendChild(cardLink(ctx, em));
      }
      row.insertCell().textContent = p.layers;
      rows.push({ row, text: `${name} ${p.version || 'unknown'} ${p.feed || ''}`.toLowerCase() });
    }
    const unknown = ems.filter((em) => em.properties.unknown).length;
    h.off = searchable(ctx.bus, rows, count, 'emitters', unknown ? ` · ${unknown} unknown` : '');
    wrap.appendChild(t);
  }).catch((err) => fail(wrap, err));
  return h;
}

// open draws one emitter's layers into el. ctx: {client, toolbar, bus, name, navigate, layerHref, cardHref}.
export function open(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'emitter-stats';
  const card = document.createElement('span');
  card.className = 'pt-emitter-card';
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.append(stats, card, count);
  ctx.client.getEmitter(ctx.name).then((env) => {
    const me = env.nodes.find((n) => n.id === env.root);
    const byId = new Map(env.nodes.map((n) => [n.id, n]));
    const layers = env.edges.filter((e) => e.kind === 'has-layer' && e.from === env.root).map((e) => byId.get(e.to)).filter(Boolean);
    stats.dataset.emitter = ctx.name;
    stats.textContent = `${layers.length} layers`;
    if (me.properties.unknown) {
      card.appendChild(unknownBadge());
    } else {
      card.append(`${me.properties.version} in ${me.properties.feed} · `, cardLink(ctx, me));
    }
    if (!layers.length) {
      count.textContent = '0 layers';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No layers mounted';
      const p = document.createElement('p');
      p.textContent = `No mounted layer's ontology names ${ctx.name}.`;
      e.append(t, p);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-emitter-layers';
    head(t, ['name', 'nodes', 'edges', 'stamp', 'principal']);
    const body = t.createTBody();
    const rows = [];
    for (const l of layers) {
      const p = l.properties;
      const row = body.insertRow();
      row.dataset.layer = l.name;
      row.title = p.version ? `${p.module} ${p.version}` : p.module;
      row.insertCell().appendChild(go(ctx, ctx.layerHref(l.name), l.name));
      const who = p.mountedBy ? p.mountedBy.replace(/^ui:principal:/, '') : '';
      for (const v of [p.nodes, p.edges, p.stamp, who]) row.insertCell().textContent = v;
      rows.push({ row, text: `${l.name} ${p.stamp} ${who}`.toLowerCase() });
    }
    h.off = searchable(ctx.bus, rows, count, 'layers', '');
    wrap.appendChild(t);
  }).catch((err) => fail(wrap, err));
  return h;
}

export function destroy(h) {
  h.off();
  h.wrap.remove();
}

// emittersOf is the envelope's emitters, Card and Emitter nodes, in
// envelope order (by name, as the API serves them).
function emittersOf(env) {
  return env.nodes.filter((n) => n.kind === 'Card' || n.kind === 'Emitter');
}

function nameOf(em) {
  return em.properties.emitter || em.name;
}

function unknownBadge() {
  const b = document.createElement('span');
  b.className = 'pt-unknown-badge';
  b.textContent = 'unknown';
  b.title = 'No feed of the tenant holds a card of this emitter.';
  return b;
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

function searchable(bus, rows, count, noun, tail) {
  let search = '';
  const apply = () => {
    let shown = 0;
    for (const r of rows) {
      r.row.hidden = !!search && !r.text.includes(search);
      if (!r.row.hidden) shown++;
    }
    count.textContent = (shown === rows.length ? `${rows.length} ${noun}` : `${shown} of ${rows.length} ${noun}`) + tail;
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
