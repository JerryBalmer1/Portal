// Actions (docs/design.md 50). Portal asks; Portal.Agent's Runner executes;
// the ledger records; the page shows the gap. ask is what the node menu's
// Logs, Stop and Restart do on a container: Stop and Restart are confirmed
// first, then POST /api/tenants/{tenant}/actions appends one action.request
// line to the tenant's ledger as the signed-in principal, and a notice
// links the id it returns, ledger:<seq>, to that line's entry page,
// /<tenant>/ledger/portal/<seq>. Nothing here runs anything.
//
// Environments/Actions, at /<tenant>/actions: the requests from GET
// .../actions, newest first, one row each (action, subject, principal,
// requested, status, result). Status is requested until a later line of
// the tenant's ledger is the Runner's result naming the request, then that
// result's status; request and result are entries of the one chain,
// ledger:<seq>. A row opens the request's entry page, where a result's
// output is shown.
import { toast } from '/static/toast.js';

// ask asks for action on node n. ctx: {client, navigate, ledgerHref}.
// confirm asks the viewer first. Resolves the line as written, or null when
// the viewer said no or the portal refused (a toast says why).
export async function ask(ctx, action, n, confirm) {
  if (confirm && !(await confirmAsk(`${verb(action)} ${n.name || n.id}?`,
    `Portal asks the Runner to ${action} ${n.id}, as you. The request is one line of the tenant's ledger; Portal itself runs nothing.`, verb(action)))) {
    return null;
  }
  try {
    const line = await ctx.client.postAction(action, n.id);
    const box = document.createElement('span');
    box.className = 'pt-asked';
    box.dataset.asked = line.id;
    box.dataset.action = action;
    box.append(`asked: ${action} ${n.id} · `, go(ctx, ctx.ledgerHref('portal', line.seq), line.id));
    toast(box, 20000);
    return line;
  } catch (err) {
    toast(err);
    return null;
  }
}

function verb(action) {
  return action.charAt(0).toUpperCase() + action.slice(1);
}

// confirmAsk is a modal question with two buttons, ok and Cancel, Cancel
// focused; Escape or a click outside is Cancel. Resolves true for ok.
export function confirmAsk(title, text, ok) {
  return new Promise((resolve) => {
    const back = document.createElement('div');
    back.className = 'pt-confirm-backdrop';
    const box = document.createElement('div');
    box.className = 'pt-confirm';
    box.setAttribute('role', 'alertdialog');
    box.setAttribute('aria-modal', 'true');
    box.setAttribute('aria-labelledby', 'pt-confirm-title');
    box.setAttribute('aria-describedby', 'pt-confirm-text');
    const h = document.createElement('h2');
    h.id = 'pt-confirm-title';
    h.textContent = title;
    const p = document.createElement('p');
    p.id = 'pt-confirm-text';
    p.textContent = text;
    const yes = document.createElement('button');
    yes.type = 'button';
    yes.className = 'gr-btn pt-confirm-ok';
    yes.dataset.confirm = 'ok';
    yes.textContent = ok;
    const no = document.createElement('button');
    no.type = 'button';
    no.className = 'gr-btn';
    no.dataset.confirm = 'cancel';
    no.textContent = 'Cancel';
    const row = document.createElement('div');
    row.className = 'pt-confirm-buttons';
    row.append(yes, no);
    box.append(h, p, row);
    back.appendChild(box);
    const back0 = document.activeElement;
    const done = (v) => {
      back.remove();
      if (back0 && back0.isConnected) back0.focus();
      resolve(v);
    };
    yes.addEventListener('click', () => done(true));
    no.addEventListener('click', () => done(false));
    back.addEventListener('pointerdown', (e) => { if (e.target === back) done(false); });
    box.addEventListener('keydown', (e) => {
      if (e.key === 'Escape') { e.preventDefault(); done(false); }
      else if (e.key === 'Tab') { e.preventDefault(); (document.activeElement === yes ? no : yes).focus(); }
    });
    document.body.appendChild(back);
    no.focus();
  });
}

// mount draws the requests into el. ctx: {client, toolbar, bus, navigate,
// ledgerHref, nodeLinks, principalHref}.
export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const h = { wrap, off: () => {} };
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  Promise.all([ctx.client.listActions(), ctx.client.listPrincipals().catch(() => ({ nodes: [] }))]).then(([env, people]) => {
    const byId = new Map(env.nodes.map((n) => [n.id, n]));
    const types = new Map(people.nodes.map((n) => [n.id, n.properties?.type]));
    const list = env.edges.filter((e) => e.kind === 'has-action' && e.from === env.root).map((e) => byId.get(e.to)).filter(Boolean);
    const root = byId.get(env.root).properties;
    if (!list.length) {
      count.textContent = '0 actions';
      const e = document.createElement('div');
      e.className = 'pt-card pt-empty-state';
      const t = document.createElement('h2');
      t.textContent = 'No actions yet';
      const q = document.createElement('p');
      q.textContent = 'Right-click a container on Environments → Graph to ask for its logs, a stop or a restart.';
      e.append(t, q);
      wrap.replaceWith(e);
      h.wrap = e;
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table pt-actions';
    const head = t.createTHead().insertRow();
    for (const name of ['action', 'subject', 'principal', 'requested', 'status', 'result']) {
      const th = document.createElement('th');
      th.textContent = name;
      head.appendChild(th);
    }
    const body = t.createTBody();
    const rows = [];
    for (const n of list) {
      const q = n.properties;
      const entry = ctx.ledgerHref('portal', q.seq);
      const row = body.insertRow();
      row.dataset.seq = String(q.seq);
      row.dataset.outcome = q.outcome;
      row.className = 'pt-action-row';
      row.title = `request ${n.id}: its entry is ${entry}`;
      row.insertCell().appendChild(go(ctx, entry, q.action));
      const graph = ctx.nodeLinks.graphHref(q.subject);
      row.insertCell().appendChild(graph ? go(ctx, graph, q.subject) : document.createTextNode(q.subject));
      const who = ctx.principalHref(q.principal, types.get(q.principal), q.by);
      const pc = row.insertCell();
      if (who) pc.appendChild(go(ctx, who, q.by));
      else {
        pc.textContent = q.by;
        pc.title = `no principal ${q.principal} in principals.yaml`;
      }
      row.insertCell().textContent = q.stamp;
      const st = document.createElement('span');
      st.className = 'pt-outcome';
      st.dataset.outcome = q.outcome;
      st.textContent = q.outcome;
      row.insertCell().appendChild(st);
      const rc = row.insertCell();
      if (q.resultSeq) rc.appendChild(go(ctx, ctx.ledgerHref('portal', q.resultSeq), q.resultId));
      // A click anywhere on the row but a link opens the request's entry.
      row.addEventListener('click', (ev) => {
        if (ev.target.closest('a') || ev.button !== 0) return;
        ctx.navigate(entry);
      });
      rows.push({ row, text: `${q.action} ${q.subject} ${q.by} ${q.outcome} ${q.resultId || ''}`.toLowerCase() });
    }
    const tail = ['requested', 'done', 'failed'].map((k) => ` · ${root[k]} ${k}`).join('');
    h.off = searchable(ctx.bus, rows, count, 'actions', tail);
    wrap.appendChild(t);
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
