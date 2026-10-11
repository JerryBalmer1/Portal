// Artifacts/Feeds: the tenant's feeds from GET /api/tenants/{tenant}/feeds,
// an envelope of Feed nodes and the Card nodes each holds (has-card edges),
// drawn from the graph and nothing else (docs/design.md 40). One card per
// feed: its kind, URL and visibility, then a row per card pushed to it
// (name, version, type, content hash, pushed, by). Read-only in 0.6.0:
// cards are pushed through POST .../feeds/{feed}/push (Publish-PortalCard,
// Portal.Agent), never from the page. A card id after # in the URL marks
// that card's row and scrolls to it: an emitter's card links there
// (docs/design.md 44).

// short is a sha256:<hex> cut to its first 12 hex digits, as tokens are named.
function short(hash) {
  return String(hash || '').replace(/^sha256:/, '').slice(0, 12);
}

// mount draws into el. ctx: {client, toolbar, bus, who, reauth}.
export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-feeds';
  el.appendChild(wrap);
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);

  ctx.client.listFeeds().then((env) => {
    const feeds = env.nodes.filter((n) => n.kind === 'Feed')
      .sort((a, b) => (a.properties.order ?? 0) - (b.properties.order ?? 0));
    const byId = new Map(env.nodes.map((n) => [n.id, n]));
    const held = new Map(feeds.map((f) => [f.id, []]));
    for (const e of env.edges) {
      if (e.kind === 'has-card' && held.has(e.from) && byId.has(e.to)) held.get(e.from).push(byId.get(e.to));
    }
    const cards = [...held.values()].reduce((n, l) => n + l.length, 0);
    count.textContent = `${feeds.length} feeds · ${cards} cards`;
    if (!feeds.length) {
      const empty = document.createElement('div');
      empty.className = 'pt-card pt-empty-state';
      const h = document.createElement('h2');
      h.textContent = 'No feeds';
      const p = document.createElement('p');
      p.textContent = 'Feeds are declared under feeds: in portal.base.yaml or the tenant\'s portal.yaml.';
      empty.append(h, p);
      wrap.replaceChildren(empty);
      return;
    }
    wrap.replaceChildren(...feeds.map((f) => feedCard(f, held.get(f.id))));
    const target = decodeURIComponent(location.hash.slice(1));
    const hit = target && [...wrap.querySelectorAll('tr[data-card]')].find((r) => r.dataset.card === target);
    if (hit) {
      hit.classList.add('pt-target');
      hit.setAttribute('aria-current', 'true');
      hit.scrollIntoView({ block: 'center' });
    }
  }).catch((err) => {
    if (err.code === 'unauthorized') {
      ctx.reauth();
      return;
    }
    wrap.className = 'pt-card pt-error-card';
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return { wrap };
}

function feedCard(f, cards) {
  const card = document.createElement('section');
  card.className = 'pt-card pt-table-card pt-feed';
  card.dataset.feed = f.id;
  const head = document.createElement('header');
  head.className = 'pt-feed-head';
  const h = document.createElement('h2');
  h.textContent = f.name || f.id;
  const kind = document.createElement('span');
  kind.className = 'pt-badge';
  kind.textContent = f.properties.kind;
  const vis = document.createElement('span');
  vis.className = 'pt-badge';
  vis.textContent = f.properties.visibility;
  const url = document.createElement('code');
  url.textContent = f.properties.url;
  head.append(h, kind, vis, url);
  card.appendChild(head);
  if (!cards.length) {
    const p = document.createElement('p');
    p.className = 'pt-feed-empty';
    p.textContent = 'No cards pushed yet.';
    card.appendChild(p);
    return card;
  }
  cards.sort((a, b) => a.name.localeCompare(b.name) || String(b.properties.pushed).localeCompare(String(a.properties.pushed)));
  const t = document.createElement('table');
  t.className = 'pt-table';
  const row0 = t.createTHead().insertRow();
  for (const name of ['card', 'version', 'type', 'hash', 'pushed', 'by']) {
    const th = document.createElement('th');
    th.textContent = name;
    row0.appendChild(th);
  }
  const body = t.createTBody();
  for (const c of cards) {
    const row = body.insertRow();
    row.dataset.card = c.id;
    row.insertCell().textContent = c.name;
    row.insertCell().textContent = c.properties.version;
    row.insertCell().textContent = c.properties.type;
    const hash = document.createElement('code');
    hash.textContent = short(c.properties.hash);
    hash.title = c.properties.hash + (c.properties.digest ? `\nstored as ${c.properties.digest}` : '');
    row.insertCell().appendChild(hash);
    row.insertCell().textContent = c.properties.pushed;
    row.insertCell().textContent = c.properties.pushedBy;
  }
  card.appendChild(t);
  return card;
}

export function destroy(h) {
  h.wrap.remove();
}
