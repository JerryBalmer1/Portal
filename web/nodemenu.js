// The node menu (docs/design.md 46): what right-click, Shift+F10 or the
// context-menu key opens on a node of the environments graph or a row of an
// environment page, and, with its own items, on a row of the alerts page
// (docs/design.md 47). Five items, each a link: Details (the node's item page),
// Show graph (/<tenant>/environments/graph#<id>), Runs, Ledger and Query
// from here. An item
// the node has no edge for is greyed, with why in its title, never hidden.
// What each links to is read from the environments graph's edges
// (GET .../environments/graph), never stored. Arrow keys, Home and End move
// through it, Enter follows, Escape and Tab close it. On a container, and
// only there, three more items ask for an action (docs/design.md 50): Logs,
// Stop and Restart, buttons, since they go nowhere; each asks through
// links.act, greyed with why while the viewer may not write. Since 0.16.0
// every node of every graph page has Query from here (docs/design.md 54):
// Graph/Explore's query bar from that node, /<tenant>/graph/explore?from=<id>;
// a Cytoscape graph (Graph/Nodes, Explore's Canvases and its result) opens a
// menu of that item alone on a right-click (wireCanvas).

let openMenu = null;

// ACTIONS are the container's action items, in menu order; stop and
// restart are confirmed first, logs is not.
export const ACTIONS = [
  { label: 'Logs', action: 'logs', confirm: false },
  { label: 'Stop', action: 'stop', confirm: true },
  { label: 'Restart', action: 'restart', confirm: true },
];

// ENV_ID is the four env: forms Graph.Node 0.3.3 registers to
// Claude.Agent.Orchestrator (docs/design.md 48): env:compose/<project>,
// env:container/<project>/<service>, env:image/<reference>,
// env:network/<name>.
const ENV_ID = /^env:(compose\/[^/\\ ]+|container\/[^/\\ ]+\/[^/\\ ]+|image\/[^\\ ]+|network\/[^/\\ ]+)$/;

// isEnvironment is whether n is an environment node, as the API reads one:
// a Kind starting environment. under an env: id. A bare id is none.
export function isEnvironment(n) {
  return String(n.kind || '').startsWith('environment.') && ENV_ID.test(String(n.id || ''));
}

// isContainer is whether the menu offers actions on n: an environment
// node of Kind environment.instance whose properties.kind is container.
// The API takes only an env:container/ id as a subject, which every such
// node has.
export function isContainer(n) {
  return isEnvironment(n) && n.kind === 'environment.instance' && n.properties?.kind === 'container';
}

// detailsHref is where a node is shown, as the API serves it: a Run's or
// a Decision's row on its runs layer's page (/<tenant>/layers/<layer>#<id>),
// a principal's agent page when a has-run edge of g says it holds a run, a
// ledger entry's page (/<tenant>/ledger/<source>/<seq>), an alert's page,
// else the item page of the Page listing its Kind; null when there is none.
// The menu's Details and the alerts page's subject and run links are this.
export function detailsHref(n, g, links) {
  const p = n.properties || {};
  if (n.kind === 'Run' || n.kind === 'Decision') return p.layer && links.layerHref(p.layer) ? `${links.layerHref(p.layer)}#${encodeURIComponent(n.id)}` : null;
  if (n.kind === 'Principal') return (g.edges || []).some((e) => e.kind === 'has-run' && e.from === n.id) ? links.agentHref(n.name) : null;
  if (n.kind === 'LedgerEntry') return p.source && p.seq !== undefined ? links.ledgerHref(p.source, p.seq) : null;
  if (n.kind === 'Alert') return links.alertHref ? links.alertHref(p.fingerprint) : null;
  return links.kindHref(n.kind, n.id);
}

// items is the menu for node n of graph g (an envelope; nodes and edges),
// each {label, href, why}: href null when greyed, why saying what is
// missing. links: {kindHref(kind, id), graphHref(id), layerHref(name),
// agentHref(name), ledgerHref(source, seq), alertHref(fingerprint)}, each
// null when the menu has no page for it, and act(action, n, confirm) and
// refusal(), which ask for an action and say why the viewer may not. A
// container also gets ACTIONS, each {label, action, confirm, why}, why ''
// unless refusal() says otherwise.
export function items(n, g, links) {
  const byId = new Map((g.nodes || []).map((x) => [x.id, x]));
  const runs = n.kind === 'Run' ? [n] : (g.edges || [])
    .filter((e) => e.kind === 'hosts' && e.from === n.id)
    .map((e) => byId.get(e.to))
    .filter((x) => x && x.kind === 'Run')
    .sort((a, b) => String(b.properties?.startedAt || '').localeCompare(String(a.properties?.startedAt || '')));
  const run = runs[0];
  // A principal's page is its agent page, which lists only the principals
  // holding a run: its has-run edge says it does.
  const details = detailsHref(n, g, links);
  let runsHref = null;
  if (n.kind === 'Principal') runsHref = details;
  else if (run) runsHref = detailsHref(run, g, links);
  const list = [
    { label: 'Details', href: details, why: n.kind === 'Principal' ? 'no has-run edge: it holds no mounted run' : `no page lists ${n.kind}` },
    { label: 'Show graph', href: links.graphHref(n.id), why: 'the menu has no environments graph' },
    { label: 'Runs', href: runsHref, why: n.kind === 'Principal' ? 'no has-run edge: it holds no mounted run' : 'no hosts edge to a mounted run' },
    ledgerItem(runs, links, run ? 'the agent ledger records nothing of this run' : 'no hosts edge to a mounted run'),
    queryItem(n.id, links),
  ];
  if (isContainer(n) && links.act) {
    const why = links.refusal ? links.refusal() : '';
    for (const a of ACTIONS) list.push({ ...a, why });
  }
  return list;
}

// ledgerItem is Ledger: the agent chain's entries of the first of runs
// that counts any, /<tenant>/ledger/agent?run=<id>, else greyed with why.
export function ledgerItem(runs, links, why) {
  const counted = runs.find((r) => Number(r.properties?.ledger) > 0);
  const href = counted && links.ledgerHref('agent') ? `${links.ledgerHref('agent')}?run=${encodeURIComponent(counted.id)}` : null;
  return { label: 'Ledger', href, why };
}

// queryItem is Query from here: the query bar's page from id, greyed with
// why when the menu has no page with a query bar.
export function queryItem(id, links) {
  return { label: 'Query from here', href: links.queryHref ? links.queryHref(id) : null, why: 'the menu has no page with a query bar' };
}

// wireCanvas opens a menu of Query from here on a right-click on a node of
// cy, drawn in root, whose own context menu it keeps closed.
export function wireCanvas(cy, root, links, navigate) {
  if (!links) return;
  root.addEventListener('contextmenu', (ev) => ev.preventDefault());
  cy.on('cxttap', 'node', (ev) => {
    const d = ev.target.data();
    const n = { id: d.id, kind: d.kind, name: d.label };
    open(ev.originalEvent, n, null, links, navigate, root, [queryItem(n.id, links)]);
  });
}

// open draws the menu for n at the event's point (or beside from, the
// element the menu was opened on, when the keyboard opened it) and focuses
// its first item. navigate(href) routes inside the shell; focus goes back
// to from on close. list is the items, items(n, g, links) when absent.
export function open(ev, n, g, links, navigate, from, list = items(n, g, links)) {
  close();
  const menu = document.createElement('div');
  menu.className = 'pt-nodemenu';
  menu.setAttribute('role', 'menu');
  menu.setAttribute('aria-label', `${n.name || n.id} (${n.kind})`);
  menu.dataset.id = n.id;
  const head = document.createElement('div');
  head.className = 'pt-nodemenu-head';
  head.textContent = n.name || n.id;
  head.title = n.id;
  menu.appendChild(head);
  const entries = list.map((it) => {
    if (it.action) return actionItem(menu, it, n, links);
    const a = document.createElement('a');
    a.className = 'pt-nodemenu-item';
    a.setAttribute('role', 'menuitem');
    a.tabIndex = -1;
    a.textContent = it.label;
    a.dataset.item = it.label.toLowerCase().replace(/\s+/g, '-');
    if (it.href) {
      a.href = it.href;
      a.addEventListener('click', (e) => {
        if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey || e.button !== 0) return;
        e.preventDefault();
        close(false);
        navigate(it.href);
      });
    } else {
      a.setAttribute('aria-disabled', 'true');
      a.title = it.why;
    }
    menu.appendChild(a);
    return a;
  });
  document.body.appendChild(menu);
  const keyboard = !ev || (ev.clientX === 0 && ev.clientY === 0);
  let x = ev ? ev.clientX : 0;
  let y = ev ? ev.clientY : 0;
  if (keyboard && from) {
    const r = from.getBoundingClientRect();
    x = r.left + 8;
    y = r.bottom;
  }
  const w = menu.offsetWidth;
  const h = menu.offsetHeight;
  menu.style.left = `${Math.max(4, Math.min(x, window.innerWidth - w - 4))}px`;
  menu.style.top = `${Math.max(4, Math.min(y, window.innerHeight - h - 4))}px`;
  const focusAt = (i) => entries[(i + entries.length) % entries.length].focus();
  menu.addEventListener('keydown', (e) => {
    const i = entries.indexOf(document.activeElement);
    if (e.key === 'ArrowDown') { e.preventDefault(); focusAt(i + 1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); focusAt(i - 1); }
    else if (e.key === 'Home') { e.preventDefault(); focusAt(0); }
    else if (e.key === 'End') { e.preventDefault(); focusAt(entries.length - 1); }
    else if (e.key === 'Escape') { e.preventDefault(); close(); }
    else if (e.key === 'Tab') close(false);
  });
  const outside = (e) => { if (!menu.contains(e.target)) close(false); };
  document.addEventListener('pointerdown', outside, true);
  window.addEventListener('blur', onBlur);
  openMenu = { menu, from, off: () => { document.removeEventListener('pointerdown', outside, true); window.removeEventListener('blur', onBlur); } };
  focusAt(0);
  return menu;
}

// actionItem is one of ACTIONS as a button: it asks through links.act,
// or is greyed with why. The first one follows a separator.
function actionItem(menu, it, n, links) {
  if (!menu.querySelector('.pt-nodemenu-sep')) {
    const sep = document.createElement('div');
    sep.className = 'pt-nodemenu-sep';
    sep.setAttribute('role', 'separator');
    menu.appendChild(sep);
  }
  const b = document.createElement('button');
  b.type = 'button';
  b.className = 'pt-nodemenu-item pt-nodemenu-action';
  b.setAttribute('role', 'menuitem');
  b.tabIndex = -1;
  b.textContent = it.label;
  b.dataset.item = it.action;
  b.dataset.action = it.action;
  if (it.why) {
    b.setAttribute('aria-disabled', 'true');
    b.title = it.why;
  } else {
    b.title = `ask the Runner to ${it.action} ${n.id}${it.confirm ? ' (asks you first)' : ''}`;
    b.addEventListener('click', () => {
      close(false);
      links.act(it.action, n, it.confirm);
    });
  }
  menu.appendChild(b);
  return b;
}

function onBlur() {
  close(false);
}

// close removes the open menu, if any; refocus puts focus back on the
// element it was opened on.
export function close(refocus = true) {
  if (!openMenu) return;
  const { menu, from, off } = openMenu;
  openMenu = null;
  off();
  menu.remove();
  if (refocus && from && from.isConnected) from.focus();
}
