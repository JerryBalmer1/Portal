// Identity/Agents, at /<tenant>/agents: the tenant's agents from GET
// /api/tenants/{tenant}/agents. An agent is a principal (docs/design.md 43):
// one of type agent, or one whose token mounted a layer of
// Graph.Emitter.Agent runs; runs mounted with no principal's token are
// dev-bootstrap's. One row each (name, type, role, runs, denied, last run),
// the name a link to the agent's page, /<tenant>/agents/<name>, which lists
// its runs from GET .../agents/{name}/runs, newest first: stamps, steps,
// decisions by effect with the deny count in red, the layer linking to its
// page, and each decision linking to its node on that page
// (/<tenant>/layers/<layer>#<id>). While a ledger layer is mounted, each run
// also counts its ledger entries, linking to them on the agent chain's page,
// /<tenant>/ledger/agent?run=<id> (docs/design.md 45). The banner's search
// filters the rows. Nothing here writes.

// mount draws the list into el. ctx: {client, toolbar, bus, navigate, agentHref}.
export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  ctx.client.listAgents().then((env) => {
    const agents = env.nodes.filter((n) => n.kind === 'Principal');
    if (!agents.length) {
      count.textContent = '0 agents';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      e.dataset.kind = 'Principal';
      const t = document.createElement('h2');
      t.textContent = 'No agents yet';
      const p = document.createElement('p');
      p.textContent = 'An agent is a principal holding runs: mount a Graph.Emitter.Agent runs layer (Mount-PortalLayer) and its runs list under the principal whose token mounted it.';
      e.append(t, p);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const runs = runsOf(env);
    const t = document.createElement('table');
    t.className = 'pt-table pt-agents';
    head(t, ['name', 'type', 'role', 'runs', 'denied', 'last run']);
    const body = t.createTBody();
    const rows = [];
    for (const a of agents) {
      const mine = runs.get(a.id) || [];
      const denied = mine.reduce((s, r) => s + (r.properties.deny || 0), 0);
      const row = body.insertRow();
      row.dataset.agent = a.name;
      row.insertCell().appendChild(go(ctx, ctx.agentHref(a.name), a.name));
      for (const v of [a.properties.type, a.properties.role, mine.length]) row.insertCell().textContent = v;
      row.insertCell().appendChild(denyCount(denied));
      row.insertCell().textContent = mine.length ? mine[0].properties.startedAt : '';
      rows.push({ row, text: `${a.name} ${a.properties.type} ${a.properties.role}`.toLowerCase() });
    }
    h.off = searchable(ctx.bus, rows, count, 'agents');
    wrap.appendChild(t);
  }).catch((err) => fail(wrap, err));
  return h;
}

// open draws one agent's runs into el. ctx: {client, toolbar, bus, name, navigate, layerHref, ledgerHref}.
export function open(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const stats = document.createElement('span');
  stats.className = 'pt-badge';
  stats.id = 'agent-stats';
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.append(stats, count);
  ctx.client.listAgentRuns(ctx.name).then((env) => {
    const me = env.nodes.find((n) => n.id === env.root);
    const runs = runsOf(env).get(env.root) || [];
    const decisions = new Map();
    for (const e of env.edges) if (e.kind === 'has-decision') decisions.set(e.from, [...(decisions.get(e.from) || []), e.to]);
    const byId = new Map(env.nodes.map((n) => [n.id, n]));
    const denied = runs.reduce((s, r) => s + r.properties.deny, 0);
    stats.dataset.agent = ctx.name;
    stats.title = me ? `${me.properties.type} · ${me.properties.role}` : '';
    stats.textContent = `${runs.length} runs · ${denied} denied`;
    if (!runs.length) {
      count.textContent = '0 runs';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No runs yet';
      const p = document.createElement('p');
      p.textContent = `No mounted Graph.Emitter.Agent layer holds a run of ${ctx.name}'s.`;
      e.append(t, p);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-runs';
    // The ledger column only while a ledger layer is mounted: the API counts then.
    const ledgered = runs.some((r) => r.properties.ledger !== undefined);
    const cols = ['started', 'finished', 'steps', 'allow', 'deny', 'witness', 'exit', 'layer', ...(ledgered ? ['ledger'] : [])];
    head(t, cols);
    const body = t.createTBody();
    const rows = [];
    for (const r of runs) {
      const p = r.properties;
      const row = body.insertRow();
      row.dataset.run = r.id;
      row.title = r.name || r.id;
      for (const v of [p.startedAt, p.stopped ? p.finishedAt || '' : 'not stopped', p.steps, p.allow]) row.insertCell().textContent = v;
      row.insertCell().appendChild(denyCount(p.deny));
      for (const v of [p.witness, p.exit ?? '']) row.insertCell().textContent = v;
      row.insertCell().appendChild(go(ctx, ctx.layerHref(p.layer), p.layer));
      if (ledgered) {
        const a = go(ctx, `${ctx.ledgerHref('agent')}?run=${encodeURIComponent(r.id)}`, String(p.ledger ?? 0));
        a.className = 'pt-ledger-count';
        a.dataset.run = r.id;
        row.insertCell().appendChild(a);
      }
      const ds = (decisions.get(r.id) || []).map((id) => byId.get(id)).filter(Boolean);
      const sub = body.insertRow();
      sub.className = 'pt-decisions';
      sub.dataset.decisionsOf = r.id;
      const cell = sub.insertCell();
      cell.colSpan = cols.length;
      for (const d of ds) {
        const a = go(ctx, `${ctx.layerHref(p.layer)}#${encodeURIComponent(d.id)}`, `step ${d.properties.step} ${d.properties.phase} ${d.properties.effect}`);
        a.className = `pt-decision pt-${d.properties.effect}`;
        a.dataset.decision = d.id;
        a.title = [d.name, d.properties.reason].filter(Boolean).join(' · ');
        cell.appendChild(a);
      }
      if (!ds.length) cell.textContent = 'no decisions';
      const text = `${p.startedAt} ${p.layer} ${ds.map((d) => `${d.properties.effect} ${d.properties.rule}`).join(' ')}`.toLowerCase();
      rows.push({ row, text, also: sub });
    }
    h.off = searchable(ctx.bus, rows, count, 'runs');
    wrap.appendChild(t);
  }).catch((err) => fail(wrap, err));
  return h;
}

export function destroy(h) {
  h.off();
  h.wrap.remove();
}

// runsOf maps each principal's id to its Run nodes, in envelope order
// (newest first, as the API serves them).
function runsOf(env) {
  const byId = new Map(env.nodes.map((n) => [n.id, n]));
  const out = new Map();
  for (const e of env.edges) {
    if (e.kind === 'has-run' && byId.has(e.to)) out.set(e.from, [...(out.get(e.from) || []), byId.get(e.to)]);
  }
  return out;
}

function head(t, names) {
  const row = t.createTHead().insertRow();
  for (const name of names) {
    const th = document.createElement('th');
    th.textContent = name;
    row.appendChild(th);
  }
}

// denyCount is a count, red when it is not zero.
function denyCount(n) {
  const s = document.createElement('span');
  s.className = n ? 'pt-deny' : 'pt-zero';
  s.dataset.deny = String(n);
  s.textContent = String(n);
  return s;
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

function searchable(bus, rows, count, noun) {
  let search = '';
  const apply = () => {
    let shown = 0;
    for (const r of rows) {
      r.row.hidden = !!search && !r.text.includes(search);
      if (r.also) r.also.hidden = r.row.hidden;
      if (!r.row.hidden) shown++;
    }
    count.textContent = shown === rows.length ? `${rows.length} ${noun}` : `${shown} of ${rows.length} ${noun}`;
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
