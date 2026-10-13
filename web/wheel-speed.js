// The mouse-wheel zoom speed of a graph page (docs/design.md 58). The
// default is the tenant config's theme.graph.wheelZoomSpeed (docs/design.md
// 37), from GET .../config; a viewer's own value for one page lives only in
// this browser, in localStorage under portal:<tenant>:<page>:wheelSpeed,
// never on the server. Every Cytoscape graph a page draws (graph.js, so
// Graph/Nodes and Explore's query result, and the Canvas) registers with
// track, which puts the Wheel control at the right of the page's toolbar.
// The control changes every graph on the page in place: Cytoscape reads its
// renderer's wheelSensitivity on each wheel event, so nothing is redrawn.

// The product's wheel step (Cytoscape's wheelSensitivity) at speed 1.
const WHEEL_STEP = 0.2;
// The schema's range for theme.graph.wheelZoomSpeed, held here again.
const MIN = 0.1;
const MAX = 5;

// page is the page being drawn: set by use, read by every graph on it.
const page = { tenant: '', id: '', config: null, toolbar: null, override: null, control: null };
const drawn = new Set();

// configured is the merged config's theme.graph.wheelZoomSpeed, 1 when
// absent; the schema holds it to 0.1..5 at boot, and the page clamps too.
export function configured(config) {
  const v = Number(config && config.theme && config.theme.graph && config.theme.graph.wheelZoomSpeed);
  return Number.isFinite(v) ? Math.min(MAX, Math.max(MIN, v)) : 1;
}

// storageKey is where a page's own speed is kept in this browser.
export function storageKey(tenant, id) {
  return `portal:${tenant}:${id}:wheelSpeed`;
}

// valid is v as a speed in range, or null.
function valid(v) {
  if (v === null || v === undefined || v === '') return null;
  const n = Number(v);
  return Number.isFinite(n) && n >= MIN && n <= MAX ? n : null;
}

// stored reads the page's override; absent, invalid or no storage is null.
function stored() {
  if (!page.id) return null;
  try {
    return valid(localStorage.getItem(storageKey(page.tenant, page.id)));
  } catch {
    return null;
  }
}

// use starts a page: id is its path (graph/nodes), '' for a page that is not
// a menu page, which keeps the config's speed and shows no control.
export function use({ tenant, id, config, toolbar }) {
  Object.assign(page, { tenant, id: id || '', config, toolbar, control: null });
  page.override = stored();
}

// speed is the page's speed: its override, else the config's.
export function speed() {
  return page.override ?? configured(page.config);
}

// sensitivity is Cytoscape's wheelSensitivity at the page's speed.
export function sensitivity() {
  return Number((WHEEL_STEP * speed()).toFixed(4));
}

// track registers a drawn graph: cy takes the page's speed, host records it
// as data-wheel-sensitivity for the tests, and the toolbar gets the control.
// It returns the untrack its renderer's destroy calls.
export function track(cy, host) {
  const d = { cy, host };
  drawn.add(d);
  apply(d);
  mountControl();
  return () => drawn.delete(d);
}

function apply(d) {
  const w = sensitivity();
  const r = d.cy.renderer && d.cy.renderer();
  if (r) r.wheelSensitivity = w;
  if (d.host) d.host.dataset.wheelSensitivity = String(w);
}

// set changes the page's speed (null: back to the config's), keeps or
// clears it in this browser, and applies it to every graph on the page.
function set(v) {
  page.override = v;
  try {
    const key = storageKey(page.tenant, page.id);
    if (v === null) localStorage.removeItem(key);
    else localStorage.setItem(key, String(v));
  } catch {
    // No storage (a private window, blocked site data): the speed holds
    // for this page view only.
  }
  for (const d of drawn) apply(d);
  if (page.control) show(page.control);
}

function mountControl() {
  if (!page.toolbar || !page.id) return;
  if (page.control && page.control.isConnected) return;
  page.control = control();
  page.toolbar.appendChild(page.control);
}

// control is the toolbar's Wheel: a slider over the schema's range, the
// value, and a reset to the config's speed.
function control() {
  const box = document.createElement('span');
  box.className = 'pt-wheel';
  box.title = 'Mouse-wheel zoom speed on this page, kept in this browser';
  const id = `pt-wheel-${page.id.replace(/[^a-z0-9-]/g, '-')}`;
  const label = document.createElement('label');
  label.htmlFor = id;
  label.textContent = 'Wheel';
  const input = document.createElement('input');
  Object.assign(input, { type: 'range', id, min: String(MIN), max: String(MAX), step: '0.1' });
  input.addEventListener('input', () => set(valid(input.value)));
  const out = document.createElement('output');
  out.htmlFor = id;
  const reset = document.createElement('button');
  reset.type = 'button';
  reset.className = 'gr-btn pt-wheel-reset';
  reset.textContent = '↺';
  reset.addEventListener('click', () => set(null));
  box.append(label, input, out, reset);
  show(box);
  return box;
}

// show writes the page's speed into the control.
function show(box) {
  const s = speed();
  const def = configured(page.config);
  const [, input, out, reset] = box.children;
  input.value = String(s);
  input.style.setProperty('--gr-fill', `${((s - MIN) / (MAX - MIN)) * 100}%`);
  out.textContent = `${s.toFixed(1)}×`;
  box.dataset.wheelSpeed = String(s);
  box.dataset.default = String(def);
  box.dataset.from = page.override === null ? 'config' : 'browser';
  reset.disabled = page.override === null;
  reset.title = `Reset to the tenant default, ${def.toFixed(1)}×`;
  reset.setAttribute('aria-label', reset.title);
}
