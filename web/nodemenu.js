// The node menu (docs/design.md 46): what right-click, Shift+F10 or the
// context-menu key opens on a node of the environments graph or a row of an
// environment page, and, with its own items, on a row of the alerts page
// (docs/design.md 47). Four items, each a link: Details (the node's item page),
// Show graph (/<tenant>/environments/graph#<id>), Runs and Ledger. An item
// the node has no edge for is greyed, with why in its title, never hidden.
// What each links to is read from the environments graph's edges
// (GET .../environments/graph), never stored. Arrow keys, Home and End move
// through it, Enter follows, Escape and Tab close it.

let openMenu = null;

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
// null when the menu has no page for it.
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
  return [
    { label: 'Details', href: details, why: n.kind === 'Principal' ? 'no has-run edge: it holds no mounted run' : `no page lists ${n.kind}` },
    { label: 'Show graph', href: links.graphHref(n.id), why: 'the menu has no environments graph' },
    { label: 'Runs', href: runsHref, why: n.kind === 'Principal' ? 'no has-run edge: it holds no mounted run' : 'no hosts edge to a mounted run' },
    ledgerItem(runs, links, run ? 'the agent ledger records nothing of this run' : 'no hosts edge to a mounted run'),
  ];
}

// ledgerItem is Ledger: the agent chain's entries of the first of runs
// that counts any, /<tenant>/ledger/agent?run=<id>, else greyed with why.
export function ledgerItem(runs, links, why) {
  const counted = runs.find((r) => Number(r.properties?.ledger) > 0);
  const href = counted && links.ledgerHref('agent') ? `${links.ledgerHref('agent')}?run=${encodeURIComponent(counted.id)}` : null;
  return { label: 'Ledger', href, why };
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
