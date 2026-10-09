// Designer: a Container like any other, drawn by this renderer because it
// is-a the Designer Category. It lists the portal ontology's Kinds and the
// Categories on screen, and adds panels, Containers and Categories through
// the same API every client uses, and approves the screen as a snap.
// PORTAL_MODE=production leaves it out of GET /api/screen, so it is never
// drawn there.

const slug = (s) => String(s).trim().toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');

export function mount(el, ctx) {
  const wrap = document.createElement('div');
  wrap.className = 'pt-designer';
  el.appendChild(wrap);
  const { screen, ontology, ix } = ctx;
  const categories = screen.nodes.filter((n) => n.kind === 'Category');
  const containers = screen.nodes.filter((n) => n.kind === 'Container');
  const status = document.createElement('div');
  status.className = 'pt-status';

  // Kinds from the portal ontology.
  const kinds = document.createElement('div');
  kinds.innerHTML = '<h4>Kinds (ontology.yaml)</h4>';
  const ul = document.createElement('ul');
  for (const [name, def] of Object.entries(ontology.portal.kinds || {})) {
    const li = document.createElement('li');
    li.textContent = name;
    li.title = def.description || '';
    ul.appendChild(li);
  }
  kinds.appendChild(ul);
  const cats = document.createElement('div');
  cats.innerHTML = '<h4>Categories</h4>';
  const cl = document.createElement('ul');
  for (const c of categories) {
    const li = document.createElement('li');
    const parent = ix.category(c.id);
    li.textContent = parent ? `${c.name} is-a ${parent.name}` : c.name;
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
  const categoryOptions = categories.map((c) => [c.id, c.name]);

  const panel = form('Add panel', [
    ['name', 'text', 'tree'],
    ['category', categoryOptions],
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

  const container = form('Add container', [
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

  wrap.append(approve, kinds, cats, panel, container, category, status);
  return { wrap };
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

// form builds a small labelled form; a field is [name, type, default] or
// [name, [[value, label], ...]] for a select.
function form(title, fields, onSubmit) {
  const f = document.createElement('form');
  f.className = 'gr-form';
  const h = document.createElement('h4');
  h.textContent = title;
  f.appendChild(h);
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
  btn.textContent = title.toLowerCase();
  f.appendChild(btn);
  f.addEventListener('submit', onSubmit);
  return f;
}
