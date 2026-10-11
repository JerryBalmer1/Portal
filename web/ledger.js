// Ledger/Entries, at /<tenant>/ledger: the tenant's ledger chains from GET
// /api/tenants/{tenant}/ledger (docs/design.md 45). The agent chain is read
// from a mounted ledger layer, the portal chain is the tenant's own push
// ledger. One row each (source, verify, entries, first, last, stamp, from),
// the verify green when the chain is valid and red with the seq it breaks at
// when not; the source links to its entries, /<tenant>/ledger/<source>,
// newest first from GET .../ledger/{source}, one page at a time, each run id
// linking to the runs layer's page with the Run's id after # (the layer
// page marks that row), and each seq to the entry's page,
// /<tenant>/ledger/<source>/<seq>, from GET .../ledger/{source}/{seq}. A
// ?run= on the entries' URL keeps only that run's entries; ?before= pages.
// An entry's page has an alerts row linking each alert about the entry
// (docs/design.md 47). An action request's page links its result and an
// action result's links its request, the result's output in a pre block
// (docs/design.md 50). Nothing here writes.
import { aboutLinks } from '/static/alerts.js';

// mount draws the chains into el. ctx: {client, toolbar, bus, navigate, ledgerHref, layerHref}.
export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  ctx.client.listLedgers().then((env) => {
    const chains = env.nodes.filter((n) => n.kind === 'Ledger');
    const t = document.createElement('table');
    t.className = 'pt-table pt-ledgers';
    head(t, ['source', 'verify', 'entries', 'first', 'last', 'stamp', 'from']);
    const body = t.createTBody();
    const rows = [];
    for (const c of chains) {
      const p = c.properties;
      const row = body.insertRow();
      row.dataset.source = p.source;
      row.insertCell().appendChild(go(ctx, ctx.ledgerHref(p.source), p.source));
      row.insertCell().appendChild(verdict(p));
      for (const v of [p.count, p.first ?? '', p.last ?? '', p.stamp || '']) row.insertCell().textContent = v;
      const from = row.insertCell();
      if (p.layer) from.append('layer ', go(ctx, ctx.layerHref(p.layer), p.layer));
      else from.textContent = p.unchained ? `ledger.jsonl · ${p.unchained} before the chain` : 'ledger.jsonl';
      rows.push({ row, text: `${p.source} ${p.valid ? 'valid' : 'broken'} ${p.layer || ''}`.toLowerCase() });
    }
    const broken = chains.filter((c) => !c.properties.valid).length;
    h.off = searchable(ctx.bus, rows, count, 'chains', broken ? ` · ${broken} broken` : '');
    wrap.appendChild(t);
  }).catch((err) => fail(wrap, err));
  return h;
}

// open draws one chain's entries, or one entry when ctx.seq is set, into el.
// ctx: {client, toolbar, bus, source, seq, run, before, navigate, ledgerHref, layerHref, alertHref, graphHref, principalHref}.
export function open(el, ctx) {
  return ctx.seq ? entry(el, ctx) : entries(el, ctx);
}

function entries(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, more: null, off: () => {} };
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'ledger-stats';
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.append(stats, count);
  ctx.client.getLedger(ctx.source, { run: ctx.run, before: ctx.before }).then((env) => {
    const me = env.nodes.find((n) => n.id === env.root);
    const p = me.properties;
    stats.dataset.source = ctx.source;
    // With a run filter, the run's entries; the verify is the whole chain's.
    stats.append(`${p.run ? p.entries : p.count} entries · `, verdict(p));
    if (p.run) stats.append(` · run ${p.run}`);
    const byId = new Map(env.nodes.map((n) => [n.id, n]));
    const list = env.edges.filter((e) => e.kind === 'has-entry' && e.from === env.root).map((e) => byId.get(e.to)).filter(Boolean);
    const records = new Map(env.edges.filter((e) => e.kind === 'records').map((e) => [e.from, e.to]));
    if (!list.length) {
      count.textContent = '0 entries';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No entries';
      const q = document.createElement('p');
      q.textContent = p.run ? `No entry of the ${ctx.source} ledger belongs to ${p.run}.` : `The ${ctx.source} ledger is empty.`;
      e.append(t, q);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const agent = ctx.source === 'agent';
    const t = document.createElement('table');
    t.className = 'pt-table pt-ledger-entries';
    head(t, agent ? ['seq', 'stamp', 'event', 'run', 'hash'] : ['seq', 'stamp', 'result', 'card', 'by', 'hash']);
    const body = t.createTBody();
    const rows = [];
    for (const n of list) {
      const q = n.properties;
      const row = body.insertRow();
      row.dataset.seq = String(q.seq);
      if (!p.valid && q.seq === p.breakAt) {
        row.className = 'pt-break';
        row.title = `the chain breaks here: ${p.reason}`;
      }
      row.insertCell().appendChild(go(ctx, ctx.ledgerHref(ctx.source, q.seq), String(q.seq)));
      row.insertCell().textContent = q.stamp;
      if (agent) {
        row.insertCell().textContent = q.event || '';
        row.insertCell().appendChild(runLink(ctx, q, records.get(n.id)));
      } else if (q.event === 'action.request') {
        row.dataset.event = q.event;
        row.insertCell().textContent = `${q.action} · ${q.outcome}`;
        row.insertCell().textContent = q.subject;
        row.insertCell().textContent = q.by;
      } else {
        row.insertCell().textContent = q.unchained ? `${q.result} (unchained)` : q.result;
        row.insertCell().textContent = q.card ? `${q.card}${q.version ? `@${q.version}` : ''}` : '';
        row.insertCell().textContent = q.by;
      }
      const hc = row.insertCell();
      hc.className = 'pt-hash';
      hc.textContent = short(q.hash);
      hc.title = q.hash;
      rows.push({ row, text: `${q.seq} ${q.stamp} ${q.event || q.result || ''} ${q.run || q.card || q.subject || ''} ${q.action || ''}`.toLowerCase() });
    }
    h.off = searchable(ctx.bus, rows, count, 'entries', p.entries > rows.length ? ` · ${p.entries} in all` : '');
    wrap.appendChild(t);
    if (p.next) {
      const more = document.createElement('p');
      more.className = 'pt-more';
      const qs = new URLSearchParams();
      if (p.run) qs.set('run', p.run);
      qs.set('before', String(p.next));
      more.appendChild(go(ctx, `${ctx.ledgerHref(ctx.source)}?${qs}`, 'older entries'));
      el.appendChild(more);
      h.more = more;
    }
  }).catch((err) => fail(wrap, err));
  return h;
}

function entry(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-ledger-entry';
  el.appendChild(wrap);
  const h = { wrap, more: null, off: () => {} };
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'ledger-stats';
  ctx.toolbar.append(stats);
  ctx.client.getLedgerEntry(ctx.source, ctx.seq).then((env) => {
    const n = env.nodes.find((x) => x.id === env.root);
    const chain = env.nodes.find((x) => x.kind === 'Ledger');
    const q = n.properties;
    const before = env.edges.find((e) => e.kind === 'prev' && e.from === n.id);
    const rec = env.edges.find((e) => e.kind === 'records' && e.from === n.id);
    stats.dataset.source = ctx.source;
    stats.append(`${ctx.source} ledger · `, verdict(chain.properties));
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
    field('id', n.id);
    field('seq', String(q.seq));
    field('stamp', q.stamp);
    for (const k of ['event', 'result', 'card', 'version', 'feed', 'by', 'message']) field(k, q[k]);
    const waits = actionFields(ctx, n, env, field);
    if (q.run) field('run', runLink(ctx, q, rec && rec.to));
    field('hash', q.hash);
    if (q.prev) {
      const prev = document.createElement('span');
      prev.append(q.prev);
      if (before) prev.append(' · ', go(ctx, ctx.ledgerHref(ctx.source, q.seq - 1), `entry ${q.seq - 1}`));
      field('prev', prev);
    }
    if (q.unchained) field('prev', 'none: written before 0.10.0, before the chain');
    if (q.written) field('written', `the line says seq ${q.written}`);
    const p = chain.properties;
    if (!p.valid && q.seq === p.breakAt) {
      wrap.classList.add('pt-break');
      field('verify', `the chain breaks here: ${p.reason}`);
    }
    wrap.appendChild(dl);
    return Promise.all([waits, aboutLinks(ctx, n.id).then((links) => { if (links) field('alerts', links); })]);
  }).catch((err) => fail(wrap, err));
  return h;
}

// actionFields adds an action's own fields to an entry's page: for a
// request, what was asked, of what, by whom, its outcome and its result
// (linking the agent chain's entry); for a result, the request it answers
// (linking Portal's entry), its status, times, exit and the output in a
// pre block, and where the Runner wrote it. Resolves once the principal's
// link is drawn.
function actionFields(ctx, n, env, field) {
  const q = n.properties;
  if (q.event === 'action.request') {
    field('action', q.action);
    const graph = ctx.graphHref && ctx.graphHref(q.subject);
    field('subject', graph ? go(ctx, graph, q.subject) : q.subject);
    field('line id', q.lineId);
    field('status', q.status);
    const out = document.createElement('span');
    out.className = 'pt-outcome';
    out.dataset.outcome = q.outcome;
    out.textContent = q.outcome;
    field('outcome', out);
    if (q.resultSeq) field('answered by', go(ctx, ctx.ledgerHref('agent', q.resultSeq), q.resultId));
    else field('answered by', 'nothing yet: no action.result in the ledger layer names ' + q.lineId);
    return ctx.client.listPrincipals().then((people) => {
      const p = people.nodes.find((x) => x.id === q.principal);
      const to = p && ctx.principalHref ? ctx.principalHref(p.id, p.properties?.type, p.name) : null;
      field('principal', to ? go(ctx, to, q.principal) : q.principal);
    }).catch(() => field('principal', q.principal));
  }
  if (q.event === 'action.result') {
    const back = env.edges.find((e) => e.kind === 'answers' && e.from === n.id);
    field('answers', q.requestSeq && back ? go(ctx, ctx.ledgerHref('portal', q.requestSeq), `${q.request} (portal ${q.requestSeq})`)
      : `${q.request || 'nothing'}: no such request in the tenant's ledger`);
    const st = document.createElement('span');
    st.className = 'pt-outcome';
    st.dataset.outcome = q.status;
    st.textContent = q.status;
    field('status', st);
    for (const k of ['started', 'finished']) field(k, q[k]);
    if (q.exit !== undefined) field('exit', String(q.exit));
    if (q.output !== undefined) {
      const pre = document.createElement('pre');
      pre.className = 'pt-output';
      pre.textContent = q.output;
      field('output', pre);
    }
    for (const s of n.source || []) field('written by', [s.tool, s.path && `${s.path}${s.line ? `:${s.line}` : ''}`].filter(Boolean).join(' · '));
  }
  return Promise.resolve();
}

export function destroy(h) {
  h.off();
  h.wrap.remove();
  if (h.more) h.more.remove();
}

// verdict is a chain's verify: valid in green, or broken at its seq in red.
function verdict(p) {
  const s = document.createElement('span');
  s.className = p.valid ? 'pt-valid' : 'pt-broken';
  s.dataset.valid = String(p.valid);
  s.textContent = p.valid ? 'valid' : `broken at ${p.breakAt}`;
  if (!p.valid) s.title = p.reason;
  return s;
}

// runLink is an entry's run id, linking to the runs layer's page with the
// Run's (or the Step's) id after #; plain text when no runs layer holds it.
function runLink(ctx, q, target) {
  if (!q.run) return document.createTextNode('');
  if (!q.runLayer) return document.createTextNode(q.run);
  const a = go(ctx, `${ctx.layerHref(q.runLayer)}#${encodeURIComponent(target || q.run)}`, q.run);
  a.className = 'pt-run-link';
  a.dataset.run = target || q.run;
  return a;
}

function short(hash) {
  return hash ? hash.slice(0, 12) : '';
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
