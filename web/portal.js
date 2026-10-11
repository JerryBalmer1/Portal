// The shell. A top banner (product, breadcrumb, search, mode, marker, help,
// avatar), under it the tenant's mode bar (amber DEV MODE, blue DEMO, none
// for prod; docs/design.md 39), a left rail of services whose pages open in
// place, and a main pane (title, toolbar, content). Everything it draws comes
// from the API: the rail from GET /api/tenants/{tenant}/menu (an envelope of
// Service and Page nodes built from the tenant's config, icons in their
// States), the mode from GET /api/tenants/{tenant}, the product name and help
// from .../config, the avatar from .../auth/whoami,
// and each page from its Page node: a table of nodes of its Kind, a screen
// node drawn by screen.js, the service connections (docs/design.md 36), or
// the tenant's principals and tokens (docs/design.md 38), or its feeds and
// the cards they hold (docs/design.md 40), or the mounted layers, each with
// its own page at /<tenant>/layers/<name> (docs/design.md 42), or the
// agents, principals holding runs, at /<tenant>/agents and each at
// /<tenant>/agents/<name> (docs/design.md 43), or the emitters, at
// /<tenant>/emitters and each at /<tenant>/emitters/<name> (docs/design.md 44),
// or the ledger chains, at /<tenant>/ledger, each chain's entries at
// /<tenant>/ledger/<source> and each entry at /<tenant>/ledger/<source>/<seq>
// (docs/design.md 45). The route is the URL, /<tenant>/<service>/<page>[/<id>],
// /<tenant>/layers/<name>, /<tenant>/agents[/<name>],
// /<tenant>/emitters[/<name>] or /<tenant>/ledger[/<source>[/<seq>]], so a deep link survives a refresh. Which service is expanded, whether the rail is
// collapsed and what the search box holds are the viewer's pointers, like a
// Canvas selection (docs/design.md 11): never written.
import { createClient, getToken, setToken } from '/static/client.js';
import * as screenView from '/static/screen.js';
import * as nodes from '/static/nodes.js';
import * as connections from '/static/connections.js';
import * as principals from '/static/principals.js';
import * as tokens from '/static/tokens.js';
import * as feeds from '/static/feeds.js';
import * as layers from '/static/layers.js';
import * as agents from '/static/agents.js';
import * as emitters from '/static/emitters.js';
import * as ledger from '/static/ledger.js';
import { icon } from '/static/icons.js';
import { refusal } from '/static/toast.js';

// Page renderers by a Page's view; screen is drawn by screen.js.
const VIEWS = { table: nodes, connections, users: principals, agents, tokens, feeds, layers, emitters, ledger };

const tenant = decodeURIComponent(location.pathname.split('/').filter(Boolean)[0] || 'default');
const client = createClient(tenant);
const bus = new EventTarget();
const $ = (id) => document.getElementById(id);
const ui = {
  brand: $('brand'), crumbs: $('crumbs'), search: $('search'), mode: $('mode'), modebar: $('modebar'), marker: $('marker'), help: $('help'), avatar: $('avatar'),
  account: $('account'), rail: $('rail'), flyout: $('flyout'), title: $('title'), toolbar: $('toolbar'), content: $('content'),
};

let config = null;
let menu = null;
let mode = 'prod';
let who = null;
let expanded = null;
let routeService = null;
let current = null;

const byOrder = (a, b) => (Number(a.properties?.order) || 0) - (Number(b.properties?.order) || 0) || a.id.localeCompare(b.id);

// railModel reads the menu envelope: Services by order, each Page's
// children (has-page) by order, and each entry's icon from its State.
export function railModel(env) {
  const all = new Map(env.nodes.map((n) => [n.id, n]));
  const out = (id, kind) => env.edges.filter((e) => e.kind === kind && e.from === id).map((e) => all.get(e.to)).filter(Boolean);
  const kids = (id) => out(id, 'has-page').sort(byOrder);
  const iconOf = (id) => out(id, 'has-state')[0]?.properties?.icon;
  const services = env.nodes.filter((n) => n.kind === 'Service').sort(byOrder);
  const pages = new Map(env.nodes.filter((n) => n.kind === 'Page').map((n) => [n.properties.path, n]));
  return { services, pages, kids, iconOf };
}

function route() {
  const parts = location.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  if (parts[1] === 'layers' && parts[2] && !parts[3]) return layerRoute(parts[2]);
  if (parts[1] === 'agents' && !parts[3]) return agentRoute(parts[2] || null);
  // A page of the emitters service keeps its path; any other name is an emitter's.
  if (parts[1] === 'emitters' && !parts[3] && !(parts[2] && menu.pages.has(`emitters/${parts[2]}`))) return emitterRoute(parts[2] || null);
  // So does a page of the ledger service; any other name is a chain's source.
  if (parts[1] === 'ledger' && !parts[4] && !(parts[2] && menu.pages.has(`ledger/${parts[2]}`))) return ledgerRoute(parts[2] || null, parts[3] || null);
  return { service: parts[1] || '', page: parts[2] || '', id: parts[3] || null, tab: new URLSearchParams(location.search).get('tab') };
}

// layerRoute places a layer page, /<tenant>/layers/<name>, under the Page
// whose view is layers (Graph/Layers), so the rail and the crumbs show
// where it sits; no service has the path layers (the binary refuses one).
function layerRoute(name) {
  const list = [...menu.pages.values()].find((p) => p.properties.view === 'layers');
  const path = list ? list.properties.path : '';
  const cut = path.indexOf('/');
  return { service: path.slice(0, cut), page: path.slice(cut + 1), id: name, layer: name, tab: null };
}

function layerHref(name) {
  return `/${encodeURIComponent(tenant)}/layers/${encodeURIComponent(name)}`;
}

// agentRoute places /<tenant>/agents under the Page whose view is agents
// (Identity/Agents), and /<tenant>/agents/<name> as its item; no service has
// the path agents (the binary refuses one).
function agentRoute(name) {
  const list = [...menu.pages.values()].find((p) => p.properties.view === 'agents');
  const path = list ? list.properties.path : '';
  const cut = path.indexOf('/');
  return { service: path.slice(0, cut), page: path.slice(cut + 1), id: name, agent: name, tab: null };
}

function agentHref(name) {
  return `/${encodeURIComponent(tenant)}/agents${name ? `/${encodeURIComponent(name)}` : ''}`;
}

// emitterRoute places /<tenant>/emitters under the Page whose view is
// emitters (Emitters/Catalogue), and /<tenant>/emitters/<name> as its item.
function emitterRoute(name) {
  const list = [...menu.pages.values()].find((p) => p.properties.view === 'emitters');
  const path = list ? list.properties.path : '';
  const cut = path.indexOf('/');
  return { service: path.slice(0, cut), page: path.slice(cut + 1), id: name, emitter: name, tab: null };
}

function emitterHref(name) {
  return `/${encodeURIComponent(tenant)}/emitters${name ? `/${encodeURIComponent(name)}` : ''}`;
}

// ledgerRoute places /<tenant>/ledger under the Page whose view is ledger
// (Ledger/Entries), /<tenant>/ledger/<source> as its item and
// /<tenant>/ledger/<source>/<seq> below that; ?run= and ?before= filter and
// page a chain's entries.
function ledgerRoute(source, seq) {
  const list = [...menu.pages.values()].find((p) => p.properties.view === 'ledger');
  const path = list ? list.properties.path : '';
  const cut = path.indexOf('/');
  const q = new URLSearchParams(location.search);
  return { service: path.slice(0, cut), page: path.slice(cut + 1), id: source, source, seq, run: q.get('run'), before: q.get('before'), tab: null };
}

function ledgerHref(source, seq) {
  return `/${encodeURIComponent(tenant)}/ledger${source ? `/${encodeURIComponent(source)}` : ''}${seq ? `/${encodeURIComponent(seq)}` : ''}`;
}

// cardHref is a card's row on the Page whose view is feeds (Artifacts/Feeds),
// which marks the row its fragment names.
function cardHref(id) {
  const list = [...menu.pages.values()].find((p) => p.properties.view === 'feeds');
  return list ? `${href(list.properties.path)}#${encodeURIComponent(id)}` : null;
}

// pageHref is where a Page lives: the agents page at /<tenant>/agents, the
// emitters page at /<tenant>/emitters, every other at its path.
function pageHref(page) {
  if (page.properties.view === 'agents') return agentHref(null);
  if (page.properties.view === 'emitters') return emitterHref(null);
  if (page.properties.view === 'ledger') return ledgerHref(null);
  return href(page.properties.path);
}

function href(path, id) {
  return `/${encodeURIComponent(tenant)}/${path}${id ? `/${encodeURIComponent(id)}` : ''}`;
}

function navigate(to) {
  if (to === location.pathname + location.search) return;
  history.pushState(null, '', to);
  render();
}

// link is an <a> that routes inside the shell on a plain click.
function link(to, text, cls) {
  const a = document.createElement('a');
  a.href = to;
  if (cls) a.className = cls;
  if (text !== undefined) a.append(text);
  a.addEventListener('click', (ev) => {
    if (ev.defaultPrevented || ev.ctrlKey || ev.metaKey || ev.shiftKey || ev.altKey || ev.button !== 0) return;
    ev.preventDefault();
    hideFlyout();
    navigate(a.getAttribute('href'));
  });
  return a;
}

// The bars' words; the dev one is the text the brief gives (docs/design.md 39).
export const MODE_BARS = Object.freeze({
  dev: 'DEV MODE — auto-minted admin, local sign-in shortcut. Not for demo or production. See README: Before you demo.',
  demo: 'DEMO',
});

async function load() {
  const [c, m, t] = await Promise.all([client.getConfig(), client.getMenu().then(railModel), client.getTenant()]);
  config = c;
  menu = m;
  mode = t.mode;
}

async function boot() {
  try {
    try {
      await load();
    } catch (err) {
      // A token the portal refuses (revoked, or dev-bootstrap's once the
      // tenant left mode dev) is refused on reads too: drop it and read as
      // anonymous rather than stop at the error card.
      if (err.code !== 'unauthorized' || !getToken()) throw err;
      setToken('');
      await load();
    }
  } catch (err) {
    ui.title.textContent = 'The portal could not load';
    ui.content.replaceChildren(errorCard(err));
    return;
  }
  ui.brand.replaceChildren(link(href(firstPage(null)), config.product.name, 'pt-brand-link'));
  if (config.product.help) {
    ui.help.href = config.product.help;
    ui.help.replaceChildren(icon('help', 16), document.createTextNode('Help'));
    ui.help.hidden = false;
  }
  drawMode();
  await whoami();
  wireBanner();
  wireRail();
  window.addEventListener('popstate', render);
  render();
}

// whoami reads the principal; a token the portal refuses is dropped, so the
// viewer is never stuck behind a stale one.
async function whoami() {
  try {
    who = await client.getWhoami();
  } catch (err) {
    if (err.code === 'unauthorized' && getToken()) {
      setToken('');
      who = await client.getWhoami().catch(() => null);
      return err;
    }
    who = null;
  }
  return null;
}

function firstPage(service) {
  const svc = service || menu.services.find((s) => s.properties.dock !== 'bottom') || menu.services[0];
  const page = svc && menu.kids(svc.id)[0];
  return page ? page.properties.path : '';
}

function render() {
  const r = route();
  const svc = menu.services.find((s) => s.properties.path === r.service) || null;
  // A move to another service expands it; staying keeps what the viewer chose.
  if (svc && svc.id !== routeService) {
    routeService = svc.id;
    expanded = svc.id;
  }
  const page = menu.pages.get(`${r.service}/${r.page}`) || null;
  document.documentElement.dataset.route = `${r.service}/${r.page}${r.id ? '/*' : ''}`;
  drawRail(r);
  drawCrumbs(svc, page, r);
  ui.search.value = '';
  drawPage(svc, page, r);
}

// ---- banner ----

// drawMode shows the tenant's mode: a badge in the banner, and under it a
// bar no one can dismiss, amber for dev and blue for demo; prod shows neither.
function drawMode() {
  document.documentElement.dataset.mode = mode;
  ui.mode.textContent = mode;
  ui.mode.dataset.mode = mode;
  ui.mode.title = `tenant ${tenant} runs in mode ${mode}`;
  ui.mode.hidden = mode === 'prod';
  ui.modebar.textContent = MODE_BARS[mode] || '';
  ui.modebar.dataset.mode = mode;
  ui.modebar.hidden = !MODE_BARS[mode];
}

function drawCrumbs(svc, page, r) {
  const parts = [[href(firstPage(null)), tenant]];
  if (svc) parts.push([href(firstPage(svc)), svc.name]);
  if (page) parts.push([pageHref(page), page.name]);
  if (r.agent) parts.push([agentHref(r.agent), r.agent]);
  else if (r.emitter) parts.push([emitterHref(r.emitter), r.emitter]);
  else if (r.source) {
    parts.push([ledgerHref(r.source), r.source]);
    if (r.seq) parts.push([ledgerHref(r.source, r.seq), r.seq]);
  }
  else if (r.id) parts.push([href(page ? page.properties.path : `${r.service}/${r.page}`, r.id), r.id]);
  ui.crumbs.replaceChildren();
  parts.forEach(([to, text], i) => {
    const li = document.createElement('li');
    if (i === parts.length - 1) {
      const s = document.createElement('span');
      s.textContent = text;
      s.setAttribute('aria-current', 'page');
      li.appendChild(s);
    } else {
      li.appendChild(link(to, text));
    }
    ui.crumbs.appendChild(li);
  });
  const named = r.source && (r.seq ? `${r.source} ${r.seq}` : `${r.source} ledger`);
  document.title = `${r.layer || r.agent || r.emitter || named || (page ? page.name : 'Not found')} · ${config.product.name}`;
}

function drawWho() {
  const name = who ? who.name : 'anonymous';
  ui.marker.replaceChildren(icon('marker', 14), document.createTextNode(` marker: ${who && who.marker ? who.marker : 'none'}`));
  ui.marker.title = 'Who holds the tenant\'s marker';
  const initials = name.split(/[\s._-]+/).filter(Boolean).map((w) => w[0]).join('').slice(0, 2).toUpperCase() || '?';
  const badge = document.createElement('span');
  badge.className = `pt-initials${who && !who.anonymous ? ' pt-signed-in' : ''}`;
  badge.textContent = initials;
  const label = document.createElement('span');
  label.className = 'pt-avatar-name';
  label.textContent = name;
  if (who && !who.anonymous && who.role) {
    const role = document.createElement('span');
    role.className = 'pt-avatar-role';
    role.textContent = ` · ${who.role}`;
    label.appendChild(role);
  }
  ui.avatar.replaceChildren(badge, label);
  ui.avatar.dataset.principal = name;
  ui.avatar.dataset.role = (who && who.role) || '';
  ui.avatar.title = !who || who.anonymous ? 'anonymous: reads only. Click to give a token.' : `${name}: ${who.role}${who.legacy ? ' (legacy token)' : ''}`;
}

function wireBanner() {
  drawWho();
  ui.search.addEventListener('input', () => bus.dispatchEvent(new CustomEvent('search', { detail: ui.search.value })));
  ui.avatar.addEventListener('click', () => {
    if (ui.account.hidden) drawAccount();
    ui.account.hidden = !ui.account.hidden;
    ui.avatar.setAttribute('aria-expanded', String(!ui.account.hidden));
    if (!ui.account.hidden) ui.account.querySelector('input, button')?.focus();
  });
  document.addEventListener('keydown', (ev) => {
    if (ev.key === 'Escape' && !ui.account.hidden) {
      ui.account.hidden = true;
      ui.avatar.setAttribute('aria-expanded', 'false');
      ui.avatar.focus();
    }
  });
}

// drawAccount fills the avatar's panel: give a token to write, or drop it.
// In mode dev it also offers the dev-bootstrap token (docs/design.md 39).
function drawAccount() {
  ui.account.replaceChildren();
  const p = document.createElement('p');
  const status = document.createElement('p');
  status.className = 'pt-status';
  status.setAttribute('aria-live', 'polite');
  const done = async () => {
    const refused = await whoami();
    drawWho();
    drawAccount();
    if (refused) ui.account.querySelector('.pt-status').textContent = `${refused.code}: ${refused.message}`;
    render();
  };
  if (who && !who.anonymous) {
    const can = who.administers ? 'Everything is allowed, identity and settings too.' : who.writes ? 'Writes are allowed, but not identity or settings.' : 'Reads only.';
    p.textContent = `Signed in as ${who.name}, ${who.role}${who.legacy ? ' (a legacy token: rotate it)' : ''}. ${can}`;
    const out = document.createElement('button');
    out.type = 'button';
    out.className = 'gr-btn';
    out.textContent = 'Sign out';
    out.addEventListener('click', () => { setToken(''); done(); });
    ui.account.append(p, out, status);
    if (who.name !== 'dev-bootstrap') devSignIn(done, status);
    return;
  }
  p.textContent = 'Reading as anonymous. Give a token from this tenant\'s tokens.yaml to write.';
  const f = document.createElement('form');
  f.className = 'pt-token-form';
  const input = document.createElement('input');
  input.type = 'password';
  input.name = 'token';
  input.autocomplete = 'off';
  input.className = 'gr-input';
  input.setAttribute('aria-label', 'bearer token');
  input.placeholder = 'bearer token';
  const go = document.createElement('button');
  go.type = 'submit';
  go.className = 'gr-btn';
  go.textContent = 'Sign in';
  f.append(input, go);
  f.addEventListener('submit', (ev) => {
    ev.preventDefault();
    setToken(input.value.trim());
    done();
  });
  ui.account.append(p, f, status);
  devSignIn(done, status);
}

// devSignIn adds "Use dev token" to the panel in mode dev only: it reads the
// token from GET .../dev-token, which the portal serves only in mode dev, to
// 127.0.0.1, after Invoke-Build Dev; anywhere else that call is 404.
function devSignIn(done, status) {
  if (mode !== 'dev') return;
  const box = document.createElement('div');
  box.className = 'pt-dev-signin';
  const note = document.createElement('p');
  note.textContent = 'DEV ONLY: sign in as dev-bootstrap, the admin Invoke-Build Dev minted.';
  const use = document.createElement('button');
  use.type = 'button';
  use.className = 'gr-btn';
  use.dataset.devToken = 'true';
  use.textContent = 'Use dev token';
  use.addEventListener('click', async () => {
    use.disabled = true;
    try {
      const t = await client.getDevToken();
      setToken(t.token);
      await done();
    } catch (err) {
      use.disabled = false;
      status.textContent = err.code === 'not_found' ? 'No dev token: start the portal with Invoke-Build Dev.' : `${err.code}: ${err.message}`;
    }
  });
  box.append(note, use);
  ui.account.append(box);
}

// ---- rail ----

function wireRail() {
  ui.flyout.addEventListener('mouseenter', () => clearTimeout(flyoutTimer));
  ui.flyout.addEventListener('mouseleave', () => hideFlyoutSoon());
  $('rail-toggle').addEventListener('click', () => {
    const collapsed = document.body.classList.toggle('pt-rail-collapsed');
    $('rail-toggle').setAttribute('aria-expanded', String(!collapsed));
    $('rail-toggle').replaceChildren(icon(collapsed ? 'chevron-right' : 'chevron-left'), label(collapsed ? 'Expand' : 'Collapse'));
    $('rail-toggle').title = collapsed ? 'Expand the rail' : 'Collapse the rail to icons';
    drawRail(route());
  });
}

function label(text) {
  const s = document.createElement('span');
  s.className = 'pt-label';
  s.textContent = text;
  return s;
}

function railCollapsed() {
  return document.body.classList.contains('pt-rail-collapsed');
}

function drawRail(r) {
  const top = document.createElement('ul');
  top.className = 'pt-services';
  const bottom = document.createElement('ul');
  bottom.className = 'pt-services pt-docked';
  for (const svc of menu.services) {
    (svc.properties.dock === 'bottom' ? bottom : top).appendChild(drawService(svc, r));
  }
  ui.rail.replaceChildren(top, bottom);
}

function drawService(svc, r) {
  const li = document.createElement('li');
  li.className = 'pt-service';
  li.dataset.service = svc.id;
  const active = svc.properties.path === r.service;
  const open = expanded === svc.id && !railCollapsed();
  li.classList.toggle('pt-active', active);
  li.classList.toggle('pt-open', open);
  const head = document.createElement('button');
  head.type = 'button';
  head.className = 'pt-service-head';
  head.setAttribute('aria-expanded', String(open));
  const chev = icon('chevron-down', 14);
  chev.classList.add('pt-chev');
  head.append(icon(menu.iconOf(svc.id)), label(svc.name), chev);
  head.addEventListener('click', () => {
    if (railCollapsed()) {
      navigate(href(firstPage(svc)));
      return;
    }
    expanded = expanded === svc.id ? null : svc.id;
    hideFlyout();
    drawRail(route());
  });
  li.appendChild(head);
  if (open) {
    const ul = document.createElement('ul');
    ul.className = 'pt-pages';
    for (const page of menu.kids(svc.id)) {
      const item = document.createElement('li');
      const a = link(pageHref(page), page.name, 'pt-page');
      a.dataset.page = page.id;
      if (`${r.service}/${r.page}` === page.properties.path) a.setAttribute('aria-current', 'page');
      item.appendChild(a);
      ul.appendChild(item);
    }
    li.appendChild(ul);
  } else {
    const show = () => showFlyout(li, svc, r);
    li.addEventListener('mouseenter', show);
    li.addEventListener('mouseleave', hideFlyoutSoon);
    head.addEventListener('focus', show);
  }
  return li;
}

let flyoutTimer = 0;

// showFlyout lists a collapsed service's pages beside it, on hover.
function showFlyout(li, svc, r) {
  clearTimeout(flyoutTimer);
  const box = li.getBoundingClientRect();
  const h = document.createElement('div');
  h.className = 'pt-flyout-head';
  h.textContent = svc.name;
  const ul = document.createElement('ul');
  for (const page of menu.kids(svc.id)) {
    const item = document.createElement('li');
    const a = link(pageHref(page), page.name, 'pt-page');
    if (`${r.service}/${r.page}` === page.properties.path) a.setAttribute('aria-current', 'page');
    item.appendChild(a);
    ul.appendChild(item);
  }
  ui.flyout.replaceChildren(h, ul);
  ui.flyout.dataset.service = svc.id;
  ui.flyout.hidden = false;
  const top = Math.min(box.top, window.innerHeight - ui.flyout.offsetHeight - 8);
  Object.assign(ui.flyout.style, { left: `${box.right + 4}px`, top: `${Math.max(56, top)}px` });
}

function hideFlyoutSoon() {
  clearTimeout(flyoutTimer);
  flyoutTimer = setTimeout(hideFlyout, 150);
}

function hideFlyout() {
  clearTimeout(flyoutTimer);
  ui.flyout.hidden = true;
}

// ---- pages ----

function unmount() {
  if (current && current.mod.destroy) current.mod.destroy(current.handle);
  current = null;
  screenView.destroyAll();
}

function drawPage(svc, page, r) {
  unmount();
  ui.toolbar.replaceChildren();
  ui.content.replaceChildren();
  ui.content.className = 'pt-content';
  if (r.layer) {
    ui.title.textContent = r.layer;
    ui.title.dataset.page = `layer:${r.layer}`;
    current = { mod: layers, handle: layers.open(ui.content, { client, bus, toolbar: ui.toolbar, name: r.layer, navigate, emitterHref }) };
    return;
  }
  if (r.agent) {
    ui.title.textContent = r.agent;
    ui.title.dataset.page = `agent:${r.agent}`;
    current = { mod: agents, handle: agents.open(ui.content, { client, bus, toolbar: ui.toolbar, name: r.agent, navigate, layerHref, ledgerHref }) };
    return;
  }
  if (r.emitter) {
    ui.title.textContent = r.emitter;
    ui.title.dataset.page = `emitter:${r.emitter}`;
    current = { mod: emitters, handle: emitters.open(ui.content, { client, bus, toolbar: ui.toolbar, name: r.emitter, navigate, layerHref, cardHref }) };
    return;
  }
  if (r.source) {
    ui.title.textContent = r.seq ? `${r.source} ${r.seq}` : `${r.source} ledger`;
    ui.title.dataset.page = `ledger:${r.source}${r.seq ? `/${r.seq}` : ''}`;
    current = { mod: ledger, handle: ledger.open(ui.content, { client, bus, toolbar: ui.toolbar, source: r.source, seq: r.seq, run: r.run, before: r.before, navigate, ledgerHref, layerHref }) };
    return;
  }
  if (!page) {
    ui.title.textContent = 'Not found';
    ui.content.appendChild(errorCard({ code: 'not_found', message: `no page ${r.service}/${r.page} in tenant ${tenant}'s menu` }));
    return;
  }
  ui.title.textContent = r.id ? r.id : page.name;
  ui.title.dataset.page = page.id;
  if (r.id) {
    drawItem(page, r);
    return;
  }
  const view = page.properties.view || 'table';
  if (view === 'screen') {
    drawScreen(page);
    return;
  }
  const mod = VIEWS[view];
  if (!mod) {
    ui.content.appendChild(errorCard({ code: 'no-view', message: `${page.id} has view ${view}, which no renderer draws` }));
    return;
  }
  current = { mod, handle: mod.mount(ui.content, pageCtx(page)) };
}

function pageCtx(page) {
  return { client, bus, page, who, reauth, toolbar: ui.toolbar, navigate, href: (id) => href(page.properties.path, id), layerHref, agentHref, emitterHref, cardHref, ledgerHref };
}

// reauth reads whoami again (a token the portal refuses is dropped) and
// redraws, after a write that may have revoked the viewer's own token.
async function reauth() {
  await whoami();
  drawWho();
  render();
}

// drawItem is one node of a page: its fields, and the page's tabs (its child
// Pages, a child's own children after it) below, each a table of its Kind.
function drawItem(page, r) {
  const tabs = [];
  const walk = (p, prefix) => {
    for (const k of menu.kids(p.id)) {
      tabs.push({ page: k, label: prefix + k.name });
      walk(k, `${prefix}${k.name} › `);
    }
  };
  walk(page, '');
  const sub = document.createElement('span');
  sub.className = 'pt-count';
  sub.textContent = page.name;
  ui.toolbar.appendChild(sub);
  const card = document.createElement('div');
  card.className = 'pt-card pt-detail';
  ui.content.appendChild(card);
  client.listNodes(page.properties.nodeKind).then((env) => {
    const n = env.nodes.find((x) => x.id === r.id);
    if (!n) {
      card.textContent = `No ${page.properties.nodeKind} ${r.id} on the screen or in a mounted layer.`;
      return;
    }
    const dl = document.createElement('dl');
    const row = (k, v) => {
      const dt = document.createElement('dt');
      dt.textContent = k;
      const dd = document.createElement('dd');
      dd.textContent = typeof v === 'string' ? v : JSON.stringify(v);
      dl.append(dt, dd);
    };
    row('id', n.id);
    row('kind', n.kind);
    if (n.name) row('name', n.name);
    for (const [k, v] of Object.entries(n.properties || {})) row(k, v);
    row('updated', nodes.updatedOf(n));
    card.replaceChildren(dl);
  }).catch((err) => card.replaceChildren(errorCard(err)));
  if (!tabs.length) return;
  const chosen = tabs.find((t) => t.page.properties.path === r.tab) || tabs[0];
  const nav = document.createElement('nav');
  nav.className = 'pt-tabs';
  nav.setAttribute('aria-label', `${page.name} tabs`);
  for (const t of tabs) {
    const a = link(`${href(page.properties.path, r.id)}?tab=${encodeURIComponent(t.page.properties.path)}`, t.label, 'pt-tab');
    if (t === chosen) a.setAttribute('aria-current', 'page');
    nav.appendChild(a);
  }
  ui.content.appendChild(nav);
  const pane = document.createElement('div');
  pane.className = 'pt-tab-pane';
  ui.content.appendChild(pane);
  const tabBar = document.createElement('div');
  tabBar.className = 'pt-toolbar pt-tab-toolbar';
  pane.appendChild(tabBar);
  current = { mod: nodes, handle: nodes.mount(pane, { ...pageCtx(chosen.page), toolbar: tabBar }) };
}

// drawScreen draws the screen node a page names, with the open view's layer
// and the screen graph's own counts in the toolbar (docs/design.md 33).
async function drawScreen(page) {
  const id = page.properties.screen;
  ui.content.classList.add('pt-fill');
  let screen;
  let ontology;
  let layers;
  try {
    [screen, ontology, layers] = await Promise.all([client.getScreen(), client.getOntology(), client.listLayers()]);
  } catch (err) {
    ui.content.appendChild(errorCard(err));
    return;
  }
  if (ui.title.dataset.page !== page.id) return;
  const ix = screenView.index(screen);
  const node = ix.nodes.get(id);
  const name = node && screenView.sourceLayer(ix, node);
  const layer = name && layers.find((l) => l.name === name);
  const stats = document.createElement('span');
  stats.className = 'pt-mode';
  stats.id = 'stats';
  stats.dataset.layer = name || '';
  if (layer) {
    stats.title = `layer ${name}${layer.path ? ` from ${layer.path}` : ''}`;
    stats.textContent = `${name} · ${layer.counts.nodes} nodes · ${layer.counts.edges} edges · ${layer.stamp}`;
  } else {
    stats.title = name ? `no layer ${name} is mounted` : `${id} draws no layer`;
    stats.textContent = name ? `${name} · not mounted` : 'no layer';
  }
  const badge = document.createElement('span');
  badge.className = 'pt-badge';
  badge.id = 'screen-stats';
  badge.title = 'the screen graph: nodes, edges, stamp';
  badge.textContent = `screen ${screen.nodes.length} · ${screen.edges.length} · ${screen.layer}`;
  ui.toolbar.replaceChildren(stats, badge);
  // readOnly: the principal may not write, so every write control is
  // disabled with why (docs/design.md 37, 38).
  const readOnly = refusal(who) || false;
  if (readOnly) {
    const ro = document.createElement('span');
    ro.className = 'pt-readonly';
    ro.textContent = `read-only: ${readOnly}`;
    ui.toolbar.appendChild(ro);
  }
  ui.content.replaceChildren();
  if (!node) {
    ui.content.appendChild(errorCard({ code: 'not_on_screen', message: `${id} is not on the screen${screen.nodes.some((n) => n.kind === 'Category' && n.name === 'Designer') ? ' (PORTAL_MODE=production hides the Designer)' : ''}` }));
    return;
  }
  const ctx = { client, bus, config, screen, ontology, layers, ix, readOnly, sections: page.properties.sections, refresh: () => drawScreen(page) };
  screenView.destroyAll();
  screenView.draw(ctx, node, ui.content);
}

function errorCard(err) {
  const box = document.createElement('div');
  box.className = 'pt-card pt-error-card';
  const pre = document.createElement('pre');
  pre.className = 'gr-error';
  pre.textContent = `${err.code || 'error'}: ${err.message || String(err)}`;
  box.appendChild(pre);
  return box;
}

boot();
