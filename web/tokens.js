// Settings/Tokens: the tenant's bearer tokens from GET
// /api/tenants/{tenant}/tokens, one row each (principal, the first 12 hex
// digits of the token's SHA-256, created), never a token. Each has Revoke,
// which only an admin may send (DELETE .../tokens/{id}); it asks once more
// before it sends. A refused revoke is a toast. A token from before 0.5.0
// names no principal and is marked legacy: it acts as admin until rotated
// (docs/design.md 38).
import { toast, guard, refusal } from '/static/toast.js';

// mount draws into el. ctx: {client, toolbar, bus, who, reauth}.
export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-card pt-table-card';
  el.appendChild(wrap);
  const count = document.createElement('span');
  count.className = 'pt-count';
  ctx.toolbar.appendChild(count);
  const deny = refusal(ctx.who, 'admin');

  const load = () => ctx.client.listTokens().then((list) => {
    count.textContent = `${list.length} tokens`;
    if (!list.length) {
      wrap.className = 'pt-card pt-empty-state';
      const h = document.createElement('h2');
      h.textContent = 'No tokens yet';
      const p = document.createElement('p');
      p.textContent = 'New-PortalToken -Name <principal> issues one; -Bootstrap makes the first admin.';
      wrap.replaceChildren(h, p);
      return;
    }
    wrap.className = 'pt-card pt-table-card';
    const t = document.createElement('table');
    t.className = 'pt-table';
    const head = t.createTHead().insertRow();
    for (const name of ['principal', 'hash', 'created', '']) {
      const th = document.createElement('th');
      th.textContent = name;
      head.appendChild(th);
    }
    const body = t.createTBody();
    for (const k of list) {
      const row = body.insertRow();
      row.dataset.token = k.id;
      const who = row.insertCell();
      who.textContent = k.name;
      if (k.legacy) {
        const b = document.createElement('span');
        b.className = 'pt-badge';
        b.textContent = 'legacy: admin until rotated';
        who.append(' ', b);
      }
      const hash = document.createElement('code');
      hash.textContent = k.id;
      row.insertCell().appendChild(hash);
      row.insertCell().textContent = k.created || '';
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'gr-btn';
      btn.textContent = 'Revoke';
      guard(btn, deny);
      row.insertCell().appendChild(btn);
      btn.addEventListener('click', async () => {
        if (btn.dataset.confirm !== 'true') {
          btn.dataset.confirm = 'true';
          btn.textContent = `Revoke ${k.id}?`;
          return;
        }
        btn.disabled = true;
        try {
          await ctx.client.revokeToken(k.id);
          await load();
        } catch (err) {
          toast(err);
          btn.disabled = false;
          btn.dataset.confirm = '';
          btn.textContent = 'Revoke';
          // The token revoked may have been the viewer's own.
          if (err.code === 'unauthorized') ctx.reauth();
        }
      });
    }
    wrap.replaceChildren(t);
  }).catch((err) => {
    if (err.code === 'unauthorized') {
      ctx.reauth();
      return;
    }
    wrap.className = 'pt-card pt-error-card';
    wrap.textContent = `${err.code}: ${err.message}`;
  });
  load();
  return { wrap };
}

export function destroy(h) {
  h.wrap.remove();
}
