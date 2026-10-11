// Settings/Service connections: the tenant's connections from GET
// /api/tenants/{tenant}/connections (no secrets: the variable that holds
// each token, whether the portal has it, and who made the connection), each
// with a Test button that calls GET .../connections/{name}/repos and shows
// how many repositories came back, or the error. Nothing is stored.

export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  ctx.client.listConnections().then((list) => {
    if (!list.length) {
      wrap.className = 'pt-card pt-empty-state';
      const h = document.createElement('h2');
      h.textContent = 'No service connections yet';
      const p = document.createElement('p');
      p.textContent = `Copy tenants/${ctx.client.tenant}/connections.example.yaml to connections.yaml and restart the portal.`;
      wrap.append(h, p);
      return;
    }
    const t = document.createElement('table');
    t.className = 'pt-table';
    const head = t.createTHead().insertRow();
    for (const name of ['name', 'type', 'url', 'token', 'created by', '', 'result']) {
      const th = document.createElement('th');
      th.textContent = name;
      head.appendChild(th);
    }
    const body = t.createTBody();
    for (const c of list) {
      const row = body.insertRow();
      row.dataset.connection = c.name;
      for (const v of [c.name, c.type, c.url]) row.insertCell().textContent = v;
      const token = row.insertCell();
      token.textContent = c.token_env;
      const dot = document.createElement('span');
      dot.className = `pt-dot ${c.tokenSet ? 'pt-ok' : 'pt-bad'}`;
      dot.title = c.tokenSet ? `${c.token_env} is set` : `${c.token_env} is not set in the portal's environment`;
      token.prepend(dot);
      // createdBy is a principal id (docs/design.md 38); null before 0.5.0.
      row.insertCell().textContent = c.createdBy ? c.createdBy.replace(/^ui:principal:/, '') : '';
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'gr-btn';
      btn.textContent = 'Test';
      row.insertCell().appendChild(btn);
      const result = row.insertCell();
      result.className = 'pt-result';
      result.setAttribute('aria-live', 'polite');
      btn.addEventListener('click', async () => {
        btn.disabled = true;
        result.className = 'pt-result';
        result.textContent = 'testing…';
        try {
          const env = await ctx.client.getConnectionRepos(c.name);
          result.classList.add('pt-ok');
          result.textContent = `${env.nodes.length} repositories`;
        } catch (err) {
          result.classList.add('pt-bad');
          result.textContent = `${err.code}: ${err.message}`;
        } finally {
          btn.disabled = false;
        }
      });
    }
    wrap.appendChild(t);
  }).catch((err) => {
    wrap.classList.add('pt-error-card');
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  return { wrap };
}

export function destroy(h) {
  h.wrap.remove();
}
