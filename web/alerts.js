// Compliance/Alerts, at /<tenant>/compliance/alerts: the alerts of the
// mounted alert layers (Portal.Agent's agent-alerts layer) from GET
// /api/tenants/{tenant}/alerts, critical first, newest first, one page at a
// time (docs/design.md 47). One row each: severity in its colour, rule,
// summary (a link to the alert's page), subject and run, each a link to its
// own page as the node menu's Details finds it (nodemenu.detailsHref), and
// stamp. Severity, rule and state chips filter through the URL (?severity=,
// ?rule=, ?state=; ?run= and ?about= too), so a filtered list is a link; the
// banner's search filters the rows on the page. A row's right-click (or
// Shift+F10) opens Details, Subject, Run and Ledger, greyed with why where
// no edge leads. Each alert's page, /<tenant>/compliance/alerts/<fingerprint>,
// from GET .../alerts/{fingerprint}, shows its fields and its about and
// raised-in edges as links. The banner's badge (badgeOf) and the "alerts"
// rows of the subjects' pages (aboutLinks) are drawn from here too.
// Nothing here writes; Portal never writes an alert.
import * as nodemenu from '/static/nodemenu.js';
import { chips } from '/static/nodes.js';

// SEVERITIES are the severity chips, in the order the list sorts them.
export const SEVERITIES = Object.freeze(['critical', 'warning', 'info']);

// FILTERS are the query keys the list passes to listAlerts.
const FILTERS = ['severity', 'rule', 'state', 'run', 'about'];

// badgeOf is the banner's badge for the open counts by severity: the top
// severity of critical and warning and its count, or null when only info
// alerts (or none) are open.
export function badgeOf(open) {
  for (const severity of ['critical', 'warning']) {
    const count = Number(open?.[severity]) || 0;
    if (count > 0) return { severity, count };
  }
  return null;
}

// ends maps each alert id of env to its subject and run, the nodes its
// about and raised-in edges reach, when the API serves them.
function ends(env) {
  const byId = new Map(env.nodes.map((n) => [n.id, n]));
  const out = new Map();
  for (const e of env.edges) {
    if (e.kind !== 'about' && e.kind !== 'raised-in') continue;
    const o = out.get(e.from) || {};
    o[e.kind === 'about' ? 'subject' : 'run'] = byId.get(e.to);
    out.set(e.from, o);
  }
  return out;
}

// menuItems is an alert row's menu: Details (its page), Subject, Run and
// Ledger (the run's entries of the agent chain), each greyed with why when
// there is nowhere to go.
export function menuItems(a, end, env, links) {
  const p = a.properties;
  const subject = end.subject ? nodemenu.detailsHref(end.subject, env, links) : null;
  const run = end.run ? nodemenu.detailsHref(end.run, env, links) : null;
  return [
    { label: 'Details', href: links.alertHref(p.fingerprint), why: 'the menu has no alerts page' },
    { label: 'Subject', href: subject, why: !p.about ? 'no about edge' : end.subject ? `no page shows ${end.subject.kind}` : `no mounted layer Portal reads holds ${p.about}` },
    { label: 'Run', href: run, why: !p.run ? 'no raised-in edge' : `no mounted runs layer holds ${p.run}` },
    nodemenu.ledgerItem(end.run ? [end.run] : [], links, end.run ? 'the agent ledger records nothing of this run' : 'no raised-in edge to a mounted run'),
  ];
}

// mount draws the list into el. ctx: {client, toolbar, bus, navigate, alertHref, nodeLinks}.
export function mount(el, ctx) {
  const q = new URLSearchParams(location.search);
  const filter = Object.fromEntries(FILTERS.map((k) => [k, q.get(k) || '']));
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, more: null, off: () => {} };
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  // A chip moves to the list with that filter, from its first page.
  const pick = (key) => (v) => {
    const next = new URLSearchParams();
    for (const k of FILTERS) if (filter[k] && k !== key) next.set(k, filter[k]);
    if (v) next.set(key, v);
    ctx.navigate(ctx.alertHref(null, next));
  };
  const group = (key, values, label) => {
    const [g] = chips(values, pick(key), label);
    if (!g) return;
    g.dataset.chips = key;
    for (const b of g.querySelectorAll('button')) b.setAttribute('aria-pressed', String(b.dataset.value === filter[key]));
    ctx.toolbar.appendChild(g);
  };
  // Severity chips show before the alerts arrive; rule and state chips are
  // the values the alerts carry.
  group('severity', SEVERITIES, 'filter by severity');
  ctx.client.listAlerts({ ...filter, after: q.get('after') || undefined }).then((env) => {
    const root = env.nodes.find((n) => n.id === env.root);
    const p = root.properties;
    group('rule', p.rules, 'filter by rule');
    group('state', p.states, 'filter by state');
    const end = ends(env);
    const list = env.edges.filter((e) => e.kind === 'has-alert' && e.from === env.root).map((e) => env.nodes.find((n) => n.id === e.to)).filter(Boolean);
    if (!list.length) {
      count.textContent = '0 alerts';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      e.dataset.kind = 'alert';
      const t = document.createElement('h2');
      t.textContent = p.count ? 'No alert matches' : 'No alerts yet';
      const s = document.createElement('p');
      s.textContent = p.count ? `None of the ${p.count} alerts matches ${FILTERS.filter((k) => filter[k]).map((k) => `${k} ${filter[k]}`).join(', ')}.`
        : 'Nothing of kind alert is in a mounted layer. Portal.Agent\'s agent-alerts layer fills this page; Portal reads it and never writes an alert.';
      e.append(t, s);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-alerts';
    t.dataset.menu = 'alert';
    head(t, ['severity', 'rule', 'summary', 'subject', 'run', 'stamp']);
    const body = t.createTBody();
    const rows = [];
    for (const a of list) {
      const ap = a.properties;
      const ae = end.get(a.id) || {};
      const row = body.insertRow();
      row.dataset.id = a.id;
      row.dataset.severity = ap.severity;
      if (!ap.open) row.classList.add('pt-closed');
      row.insertCell().appendChild(severity(ap.severity));
      row.insertCell().textContent = ap.rule;
      row.insertCell().appendChild(go(ctx, ctx.alertHref(ap.fingerprint), ap.summary || a.name || a.id));
      row.insertCell().appendChild(endLink(ctx, env, ae.subject, ap.about, 'pt-subject-link'));
      row.insertCell().appendChild(endLink(ctx, env, ae.run, ap.run, 'pt-run-link'));
      row.insertCell().textContent = ap.stamp;
      rows.push({ row, a, end: ae, text: `${ap.severity} ${ap.rule} ${ap.summary} ${ap.state || 'open'} ${ap.about || ''} ${ap.run || ''}`.toLowerCase() });
    }
    // A filtered or paged list says how many match and how many there are.
    let tail = p.alerts > rows.length && p.alerts < p.count ? ` · ${p.alerts} match` : '';
    if (rows.length < p.count) tail += ` · ${p.count} in all`;
    h.off = searchable(ctx.bus, rows, count, tail);
    withMenu(t, rows, env, ctx);
    wrap.appendChild(t);
    if (p.next) {
      const more = document.createElement('p');
      more.className = 'pt-more';
      const next = new URLSearchParams();
      for (const k of FILTERS) if (filter[k]) next.set(k, filter[k]);
      next.set('after', p.next);
      more.appendChild(go(ctx, ctx.alertHref(null, next), 'more alerts'));
      el.appendChild(more);
      h.more = more;
    }
  }).catch((err) => fail(wrap, err));
  return h;
}

// open draws one alert into el. ctx: {client, toolbar, id, navigate, alertHref, layerHref, nodeLinks}.
export function open(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-alert';
  el.appendChild(wrap);
  const h = { wrap, more: null, off: () => {} };
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'alert-stats';
  ctx.toolbar.append(stats);
  ctx.client.getAlert(ctx.id).then((env) => {
    const a = env.nodes.find((n) => n.id === env.root);
    const p = a.properties;
    const e = ends(env).get(a.id) || {};
    wrap.dataset.severity = p.severity;
    stats.append(severity(p.severity), ` · ${p.state || 'open'}`);
    const dl = document.createElement('dl');
    dl.className = 'pt-fields';
    const field = (k, v) => {
      if (v === undefined || v === null || v === '') return;
      const dt = document.createElement('dt');
      dt.textContent = k;
      const dd = document.createElement('dd');
      dd.dataset.field = k;
      dd.append(v);
      dl.append(dt, dd);
    };
    field('id', a.id);
    field('severity', severity(p.severity));
    for (const k of ['rule', 'summary']) field(k, p[k]);
    field('state', p.state || 'open (no state given)');
    for (const k of ['stamp', 'fingerprint']) field(k, p[k]);
    field('layer', go(ctx, ctx.layerHref(p.layer), p.layer));
    field('about', p.about ? endLink(ctx, env, e.subject, p.about, 'pt-subject-link') : 'no about edge');
    field('raised-in', p.run ? endLink(ctx, env, e.run, p.run, 'pt-run-link') : 'no raised-in edge');
    wrap.appendChild(dl);
  }).catch((err) => fail(wrap, err));
  return h;
}

export function destroy(h) {
  nodemenu.close(false);
  h.off();
  h.wrap.remove();
  if (h.more) h.more.remove();
}

// aboutLinks is the "alerts" row of a subject's page: one link per alert
// about id, from GET .../alerts?about=, in its severity's colour, or null
// when no alert is about it (or the menu has no alerts page).
export async function aboutLinks(ctx, id) {
  if (!ctx.alertHref || !ctx.alertHref(null)) return null;
  let env;
  try {
    env = await ctx.client.listAlerts({ about: id });
  } catch {
    return null;
  }
  const list = env.nodes.filter((n) => n.kind === 'Alert');
  if (!list.length) return null;
  const span = document.createElement('span');
  span.className = 'pt-alerts-about';
  span.dataset.about = id;
  list.forEach((a, i) => {
    const p = a.properties;
    const link = go(ctx, ctx.alertHref(p.fingerprint), `${p.severity} ${p.rule}${p.open ? '' : ` (${p.state})`}`);
    link.className = 'pt-alert-link';
    link.dataset.severity = p.severity;
    link.title = p.summary || '';
    if (i) span.append(' · ');
    span.append(link);
  });
  return span;
}

// severity is a severity in its colour.
function severity(s) {
  const span = document.createElement('span');
  span.className = 'pt-sev';
  span.dataset.severity = s;
  span.textContent = s;
  return span;
}

// endLink is an edge's end: a link to its page when the API serves the
// node and a page shows it, else its id as text, saying why in its title.
function endLink(ctx, env, n, id, cls) {
  if (!id) return document.createTextNode('');
  const to = n && nodemenu.detailsHref(n, env, ctx.nodeLinks);
  if (!to) {
    const s = document.createElement('span');
    s.className = 'pt-muted';
    s.textContent = id;
    s.title = n ? `no page shows ${n.kind}` : 'no mounted layer Portal reads holds it';
    return s;
  }
  const a = go(ctx, to, n.name || id);
  a.className = cls;
  a.dataset.id = id;
  a.title = `${n.kind} ${id}`;
  return a;
}

// withMenu opens an alert row's menu on right-click (or Shift+F10).
function withMenu(t, rows, env, ctx) {
  t.addEventListener('contextmenu', (ev) => {
    const row = ev.target.closest('tr[data-id]');
    const r = row && rows.find((x) => x.row === row);
    if (!r) return;
    ev.preventDefault();
    const at = { clientX: ev.clientX, clientY: ev.clientY };
    nodemenu.open(at, r.a, env, ctx.nodeLinks, ctx.navigate, row.querySelector('a') || row, menuItems(r.a, r.end, env, ctx.nodeLinks));
  });
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

function searchable(bus, rows, count, tail) {
  let search = '';
  const apply = () => {
    let shown = 0;
    for (const r of rows) {
      r.row.hidden = !!search && !r.text.includes(search);
      if (!r.row.hidden) shown++;
    }
    count.textContent = (shown === rows.length ? `${rows.length} alerts` : `${shown} of ${rows.length} alerts`) + tail;
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
