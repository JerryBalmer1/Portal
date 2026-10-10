// Designer: a Container like any other, drawn by this renderer because it
// is-a the Designer Category. It lists the portal ontology's Kinds and every
// Category on screen as a tree by refines (is-a) depth, and adds panels,
// Containers, Categories and menu entries through the same API every client
// uses, and approves the screen as a snap. Every list and dropdown is built
// from the screen the page has just read from GET /api/screen; after each add
// the page re-reads it and mounts the Designer again (docs/design.md 26).
// PORTAL_MODE=production leaves it out of GET /api/screen, so it is never
// drawn there. Each section is a collapsible block whose open or closed is a
// State node, ui:state:designer:<slug> for the seed's (docs/design.md 28).

// SECTIONS are the Designer's blocks in order: slug, heading, open by
// default. internal/store's DesignerSections seeds the same seven, and a Go
// test reads this list off the served page.
export const SECTIONS = Object.freeze([
  { slug: 'approve', title: 'Approve', open: true },
  { slug: 'kinds', title: 'Kinds (ontology.yaml)', open: false },
  { slug: 'categories', title: 'Categories', open: true },
  { slug: 'add-panel', title: 'Add Panel', open: false },
  { slug: 'add-container', title: 'Add Container', open: false },
  { slug: 'add-category', title: 'Add Category', open: false },
  { slug: 'add-menu-entry', title: 'Add Menu Entry', open: false },
]);

const slug = (s) => String(s).trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-designer';
  el.appendChild(wrap);
  const { screen, ontology, ix } = ctx;
  const categories = screen.nodes.filter((n) => n.kind === 'Category');
  // Menu entries are not drawn on screen: no panel goes in one and none
  // opens another.
  const containers = screen.nodes.filter((n) => n.kind === 'Container' && !ix.isMenuEntry(n.id));
  const menuCat = categories.find((c) => c.name === 'Menu');
  const status = document.createElement('div');
  status.className = 'pt-status';

  // Kinds from the portal ontology.
  const kinds = document.createElement('div');
  const ul = document.createElement('ul');
  for (const [name, def] of Object.entries(ontology.portal.kinds || {})) {
    const li = document.createElement('li');
    li.textContent = name;
    li.title = def.description || '';
    ul.appendChild(li);
  }
  kinds.appendChild(ul);
  const cats = document.createElement('div');
  const cl = document.createElement('ul');
  cl.className = 'pt-categories';
  for (const { cat: c, depth } of categoryTree(categories, ix)) {
    const li = document.createElement('li');
    li.dataset.id = c.id;
    li.dataset.depth = depth;
    li.style.marginLeft = `${depth * 1.2}em`;
    const parent = ix.category(c.id);
    if (depth) {
      const d = document.createElement('span');
      d.className = 'pt-depth';
      d.textContent = '└ ';
      li.appendChild(d);
    }
    li.append(parent ? `${c.name} (refines ${parent.name})` : c.name);
    li.title = (c.properties && c.properties.description) || '';
    cl.appendChild(li);
  }
  cats.appendChild(cl);

  // Writes: node first, then its edges; any refusal deletes what this form
  // added, so a half-built panel never stays on screen.
  const write = async (node, edges) => {
    status.textContent = '';
    await ctx.client.postNode(node);
    try {
      for (const e of edges) await ctx.client.postEdge(e);
    } catch (err) {
      await ctx.client.deleteNode(node.id).catch(() => {});
      throw err;
    }
  };
  const run = (fn) => async (ev) => {
    ev.preventDefault();
    try {
      await fn(new FormData(ev.target));
      await ctx.refresh();
    } catch (err) {
      status.textContent = `${err.message}${(err.problems || []).map((p) => `\n${p.code} ${p.path}: ${p.message}`).join('')}`;
    }
  };
  const bounds = (f) => ({ x: Number(f.get('x')), y: Number(f.get('y')), w: Number(f.get('w')), h: Number(f.get('h')) });
  const containerOptions = containers.map((c) => [c.id, c.name || c.id]);
  const tree = categoryTree(categories, ix);
  const categoryOptions = tree.map(({ cat: c, depth }) => [c.id, `${'· '.repeat(depth)}${c.name}`]);
  // A panel's Category must reach a renderer up its is-a chain.
  const panelOptions = tree.filter(({ cat: c }) => drawable(c, ix)).map(({ cat: c, depth }) => [c.id, `${'· '.repeat(depth)}${c.name}`]);
  const shownOptions = screen.nodes
    .filter((n) => (n.kind === 'Container' || n.kind === 'Widget') && !ix.isMenuEntry(n.id) && n.id !== screen.root)
    .map((n) => [n.id, `${n.name || n.id} (${n.kind})`]);

  const panel = form('Add Panel', [
    ['name', 'text', 'tree'],
    ['category', panelOptions],
    ['parent', containerOptions],
    ['source', 'text', 'filesystem'],
    ['x', 'number', 0], ['y', 'number', 9], ['w', 'number', 6], ['h', 'number', 6],
  ], run(async (f) => {
    const cat = ix.nodes.get(f.get('category'));
    const id = `ui:${slug(f.get('name'))}`;
    await write(
      { id, kind: 'Widget', name: f.get('name'), properties: { type: cat.name, source: f.get('source'), bounds: bounds(f) } },
      [{ kind: 'contains', from: f.get('parent'), to: id }, { kind: 'is-a', from: id, to: cat.id }],
    );
  }));

  const container = form('Add Container', [
    ['name', 'text', 'group'],
    ['parent', containerOptions],
    ['mode', [['grid', 'grid'], ['flow', 'flow'], ['canvas', 'canvas']]],
    ['columns', 'number', 12],
    ['x', 'number', 0], ['y', 'number', 9], ['w', 'number', 12], ['h', 'number', 6],
  ], run(async (f) => {
    const id = `ui:${slug(f.get('name'))}`;
    const layout = `ui:layout:${slug(f.get('name'))}`;
    const props = { mode: f.get('mode') };
    if (f.get('mode') === 'grid') props.columns = Number(f.get('columns'));
    await write({ id: layout, kind: 'Layout', name: `${f.get('name')} ${f.get('mode')}`, properties: props }, []);
    try {
      await write(
        { id, kind: 'Container', name: f.get('name'), properties: { parent: f.get('parent'), bounds: bounds(f) } },
        [{ kind: 'contains', from: f.get('parent'), to: id }, { kind: 'laid-out-by', from: id, to: layout }],
      );
    } catch (err) {
      await ctx.client.deleteNode(layout).catch(() => {});
      throw err;
    }
  }));

  const category = form('Add Category', [
    ['name', 'text', 'Tree'],
    ['refines', categoryOptions],
    ['description', 'text', ''],
  ], run(async (f) => {
    const name = f.get('name').trim();
    const id = `ui:category:${name}`;
    const props = { label: name };
    if (f.get('description')) props.description = f.get('description');
    await write({ id, kind: 'Category', name, properties: props }, [{ kind: 'is-a', from: id, to: f.get('refines') }]);
  }));

  // Approve writes the screen as a snap; the next start boots from it. The
  // screen does not change, so the page is not redrawn and the result stays.
  const approve = form('Approve', [['note', 'text', '']], async (ev) => {
    ev.preventDefault();
    try {
      status.textContent = approval(await ctx.client.postSnap(new FormData(ev.target).get('note')));
    } catch (err) {
      status.textContent = `${err.message}${(err.problems || []).map((p) => `\n${p.code} ${p.path}: ${p.message}`).join('')}`;
    }
  });

  // A menu entry is a Container that is-a Menu and opens what it shows: one
  // node add and two edge adds through the same POSTs (docs/design.md 25).
  const entries = ix.menu();
  const menu = form('Add Menu Entry', [
    ['name', 'text', 'Designer'],
    ['opens', shownOptions],
  ], run(async (f) => {
    if (!menuCat) throw { code: 'no-menu', message: 'no Category named Menu on screen: POST ui:category:Menu first' };
    const name = f.get('name').trim();
    const id = `ui:menu:${slug(name)}`;
    const order = entries.reduce((m, { entry }) => Math.max(m, Number(entry.properties?.order) || 0), -1) + 1;
    await write(
      { id, kind: 'Container', name, properties: { order } },
      [{ kind: 'is-a', from: id, to: menuCat.id }, { kind: 'opens', from: id, to: f.get('opens') }],
    );
  }));

  const bodies = { approve, kinds, categories: cats, 'add-panel': panel, 'add-container': container, 'add-category': category, 'add-menu-entry': menu };
  // A page may draw some sections only (Designer/Export: approve), by the
  // slugs its Page node's sections property lists; absent draws all.
  const shown = SECTIONS.filter((s) => !Array.isArray(ctx.sections) || ctx.sections.includes(s.slug));
  wrap.append(...shown.map((s) => section(ctx, s, bodies[s.slug], status)), status);
  return { wrap };
}

// section wraps one block in a head (heading left, the panels' collapse
// control right) and a body that collapsing hides. Toggling writes the
// section's State node: PATCH its collapsed when it is on screen, else POST
// it and its has-state edge. The block then follows the written State in
// place rather than redrawing the page, so a Canvas is not laid out again.
function section(ctx, s, body, status) {
  const sid = `ui:state:${ctx.node.id.replace(/^ui:/, '')}:${s.slug}`;
  const node = ctx.ix.nodes.get(sid);
  let exists = !!node;
  let collapsed = node && typeof node.properties?.collapsed === 'boolean' ? node.properties.collapsed : !s.open;
  const box = document.createElement('section');
  box.className = 'pt-section';
  box.dataset.section = s.slug;
  const head = document.createElement('div');
  head.className = 'pt-section-head';
  const h = document.createElement('h4');
  h.textContent = s.title;
  const toggle = document.createElement('button');
  toggle.type = 'button';
  toggle.className = 'gr-btn';
  body.classList.add('pt-section-body');
  const paint = () => {
    box.classList.toggle('pt-collapsed', collapsed);
    toggle.textContent = collapsed ? 'expand' : 'collapse';
    toggle.setAttribute('aria-expanded', String(!collapsed));
  };
  toggle.addEventListener('click', async () => {
    const next = !collapsed;
    try {
      if (exists) {
        await ctx.client.patchNode(sid, { properties: { collapsed: next } });
      } else {
        await ctx.client.postNode({ id: sid, kind: 'State', name: `state of ${ctx.node.id} · ${s.title}`, properties: { collapsed: next } });
        exists = true;
        await ctx.client.postEdge({ kind: 'has-state', from: ctx.node.id, to: sid });
      }
      collapsed = next;
      paint();
    } catch (err) {
      status.textContent = `${err.message}${(err.problems || []).map((p) => `\n${p.code} ${p.path}: ${p.message}`).join('')}`;
    }
  });
  paint();
  head.append(h, toggle);
  box.append(head, body);
  return box;
}

// approval says what a snap holds against the one before it.
export function approval(s) {
  const head = `approved ${s.stamp}${s.note ? ` · ${s.note}` : ''}`;
  if (!s.diff) return `${head}\nfirst snap: nothing to compare`;
  const part = (verb, ids) => {
    const all = [...ids.nodes, ...ids.edges];
    return all.length ? `\n${verb} ${ids.nodes.length} nodes, ${ids.edges.length} edges: ${all.join(', ')}` : '';
  };
  const body = part('added', s.diff.added) + part('removed', s.diff.removed) + part('changed', s.diff.changed);
  return `${head}\nagainst ${s.previous}:${body || ' no change'}`;
}

export function destroy(h) { h.wrap.remove(); }

// categoryTree orders Categories as a tree by refines (is-a): each top-level
// Category, then what refines it, depth-first by name. A Category in an is-a
// cycle, or refining one that is gone, is listed at the top level.
export function categoryTree(categories, ix) {
  const ids = new Set(categories.map((c) => c.id));
  const kids = new Map();
  const top = [];
  for (const c of categories) {
    const parent = ix.category(c.id);
    if (parent && ids.has(parent.id) && parent.id !== c.id) {
      if (!kids.has(parent.id)) kids.set(parent.id, []);
      kids.get(parent.id).push(c);
    } else {
      top.push(c);
    }
  }
  const byName = (a, b) => a.name.localeCompare(b.name);
  const out = [];
  const seen = new Set();
  const walk = (c, depth) => {
    if (seen.has(c.id)) return;
    seen.add(c.id);
    out.push({ cat: c, depth });
    for (const k of (kids.get(c.id) || []).sort(byName)) walk(k, depth + 1);
  };
  for (const c of top.sort(byName)) walk(c, 0);
  for (const c of categories.slice().sort(byName)) walk(c, 0);
  return out;
}

// drawable says whether a Category is a base renderer or refines one.
function drawable(c, ix) {
  const seen = new Set();
  for (let cat = c; cat && !seen.has(cat.id); cat = ix.category(cat.id)) {
    if (ix.isRenderer(cat.name)) return true;
    seen.add(cat.id);
  }
  return false;
}

// form builds a small labelled form; a field is [name, type, default] or
// [name, [[value, label], ...]] for a select. Its button says the section's
// title; the heading is the section's.
function form(title, fields, onSubmit) {
  const f = document.createElement('form');
  f.className = 'gr-form';
  for (const [name, type, def] of fields) {
    const row = document.createElement('label');
    row.className = 'gr-form-row';
    row.append(name);
    let input;
    if (Array.isArray(type)) {
      input = document.createElement('select');
      for (const [value, label] of type) input.add(new Option(label, value));
    } else {
      input = document.createElement('input');
      input.type = type;
      input.value = def;
    }
    input.name = name;
    input.className = 'gr-input';
    row.appendChild(input);
    f.appendChild(row);
  }
  const btn = document.createElement('button');
  btn.className = 'gr-btn';
  btn.type = 'submit';
  btn.textContent = title;
  f.appendChild(btn);
  f.addEventListener('submit', onSubmit);
  return f;
}
