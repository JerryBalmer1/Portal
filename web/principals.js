// Identity/Users: the tenant's principals of type user, from GET
// /api/tenants/{tenant}/principals (an envelope of Principal nodes), one row
// each (name, role, created, created by), and an Add form (name and role)
// that only an admin may send, and only for a role up to its own
// (attenuation); the portal checks both again (docs/design.md 38). A refused
// add is a toast. The banner's search filters the rows; the row whose id
// follows # is marked, where an action's principal links (docs/design.md
// 50). Identity/Agents is agents.js since 0.8.0 (docs/design.md 43).
import { toast, guard, refusal } from '/static/toast.js';

const ROLES = ['reader', 'contributor', 'admin'];

// mount draws into el. ctx: {client, page, toolbar, bus, who}.
export function mount(el, ctx) {
  const type = 'user';
  const root = document.createElement('div');
  root.className = 'pt-stack';
  root.dataset.principals = type;
  el.appendChild(root);
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  const list = document.createElement('div');
  list.className = 'pt-card pt-table-card';
  let search = '';
  let rows = [];
  const apply = () => {
    let shown = 0;
    for (const r of rows) {
      r.row.hidden = !!search && !r.text.includes(search);
      if (!r.row.hidden) shown++;
    }
    count.textContent = shown === rows.length ? `${rows.length} ${type}s` : `${shown} of ${rows.length} ${type}s`;
  };
  const onSearch = (ev) => { search = String(ev.detail || '').toLowerCase(); apply(); };
  ctx.bus.addEventListener('search', onSearch);
  const h = { root, off: () => ctx.bus.removeEventListener('search', onSearch) };

  const load = () => ctx.client.listPrincipals().then((env) => {
    const names = new Map(env.nodes.map((n) => [n.id, n.name]));
    const mine = env.nodes.filter((n) => n.properties?.type === type);
    rows = [];
    if (!mine.length) {
      list.className = 'pt-card pt-empty-state';
      list.dataset.kind = 'Principal';
      const t = document.createElement('h2');
      const p = document.createElement('p');
      t.textContent = 'No users yet';
      p.textContent = `Make the first admin with New-PortalToken -Name <name> -Bootstrap (tenants/${ctx.client.tenant}/principals.yaml).`;
      list.replaceChildren(t, p);
      count.textContent = `0 ${type}s`;
      return;
    }
    list.className = 'pt-card pt-table-card';
    const t = document.createElement('table');
    t.className = 'pt-table';
    const head = t.createTHead().insertRow();
    for (const name of ['name', 'role', 'created', 'created by']) {
      const th = document.createElement('th');
      th.textContent = name;
      head.appendChild(th);
    }
    const body = t.createTBody();
    for (const n of mine) {
      const row = body.insertRow();
      row.dataset.principal = n.id;
      const by = n.properties.createdBy ? names.get(n.properties.createdBy) || n.properties.createdBy : 'bootstrap';
      for (const v of [n.name, n.properties.role, n.properties.created, by]) row.insertCell().textContent = v;
      rows.push({ row, text: `${n.name} ${n.properties.role} ${by}`.toLowerCase() });
    }
    list.replaceChildren(t);
    apply();
    const target = decodeURIComponent(location.hash.slice(1));
    const hit = target && rows.find((r) => r.row.dataset.principal === target);
    if (hit) {
      hit.row.classList.add('pt-target');
      hit.row.setAttribute('aria-current', 'true');
    }
  }).catch((err) => {
    list.className = 'pt-card pt-error-card';
    list.textContent = `${err.code}: ${err.message}`;
  });

  root.appendChild(addForm(ctx, load));
  root.appendChild(list);
  load();
  return h;
}

export function destroy(h) {
  h.off();
  h.root.remove();
}

// addForm is Add user: a name, a role no higher than the viewer's, and the
// button, disabled unless the viewer is an admin.
function addForm(ctx, reload) {
  const f = document.createElement('form');
  f.className = 'pt-card pt-add-form';
  f.dataset.form = 'add-user';
  const title = document.createElement('h3');
  title.textContent = 'Add user';
  const name = document.createElement('input');
  name.type = 'text';
  name.name = 'name';
  name.required = true;
  name.pattern = '[a-z0-9][a-z0-9._\\-]{0,63}';
  name.placeholder = 'name';
  name.autocomplete = 'off';
  name.setAttribute('aria-label', 'name');
  name.className = 'gr-input';
  const role = document.createElement('select');
  role.name = 'role';
  role.setAttribute('aria-label', 'role');
  const own = ROLES.indexOf(ctx.who?.role);
  for (const [i, r] of ROLES.entries()) {
    const o = document.createElement('option');
    o.value = r;
    o.textContent = r;
    // Attenuation: never above the viewer's own role.
    if (own >= 0 && i > own) o.disabled = true;
    role.appendChild(o);
  }
  const go = document.createElement('button');
  go.type = 'submit';
  go.className = 'gr-btn';
  go.textContent = 'Add';
  guard(go, refusal(ctx.who, 'admin'));
  const status = document.createElement('span');
  status.className = 'pt-status';
  status.setAttribute('aria-live', 'polite');
  f.append(title, name, role, go, status);
  f.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    if (go.disabled) return;
    go.disabled = true;
    status.textContent = '';
    try {
      const n = await ctx.client.postPrincipal({ name: name.value.trim(), role: role.value, type: 'user' });
      status.textContent = `Added ${n.name}. Issue its token with New-PortalToken -Name ${n.name}.`;
      name.value = '';
      await reload();
    } catch (err) {
      toast(err);
    } finally {
      go.disabled = false;
    }
  });
  return f;
}
