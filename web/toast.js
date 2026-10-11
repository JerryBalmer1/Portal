// Toast: a short message over the page's corner, for a write the portal
// refused. An error body is never drawn inside a panel; a 401 (or any write
// a panel could not make) says so here and the panel keeps drawing what the
// graph holds (docs/design.md 37).

// READ_ONLY is the tooltip on every write control while the principal is
// anonymous.
export const READ_ONLY = 'sign in to write';

// refusal is why the principal who (whoami) may not make a write that needs
// role need (contributor: any write; admin: identity and settings), as the
// tooltip to show, or '' when it may (docs/design.md 38).
export function refusal(who, need = 'contributor') {
  if (!who || who.anonymous) return READ_ONLY;
  if (need === 'admin' ? who.administers : who.writes) return '';
  return need === 'admin' ? `admin only: ${who.name} is ${who.role}` : `${who.name} is ${who.role}, which reads only`;
}

let box = null;

// toast shows err's code, message and problems (an Error object from
// client.js), or text, for a few seconds, newest at the bottom.
export function toast(err, ms = 5000) {
  if (!box) {
    box = document.createElement('div');
    box.className = 'pt-toasts';
    box.setAttribute('role', 'status');
    box.setAttribute('aria-live', 'polite');
    document.body.appendChild(box);
  }
  const t = document.createElement('div');
  t.className = 'pt-toast';
  if (typeof err === 'string') {
    t.textContent = err;
  } else {
    const code = (err && err.code) || 'error';
    t.dataset.code = code;
    const problems = ((err && err.problems) || []).map((p) => `\n${p.code} ${p.path}: ${p.message}`).join('');
    t.textContent = `${code}: ${(err && err.message) || String(err)}${problems}`;
  }
  box.appendChild(t);
  setTimeout(() => t.remove(), ms);
  return t;
}

// guard disables a write control while the viewer may not write, with the
// tooltip readOnly gives when it is a string (refusal), else READ_ONLY; it
// returns the control.
export function guard(el, readOnly) {
  if (readOnly) {
    el.disabled = true;
    el.title = typeof readOnly === 'string' ? readOnly : READ_ONLY;
    el.dataset.readonly = 'true';
  }
  return el;
}
