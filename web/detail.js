// Detail: one data node. Shows the node last selected on a Canvas or Table
// (in the Widget's source layer, or any layer when it has no source), else
// the node its displays edge points at through a Ref.
import { parseSource } from '/static/client.js';

export function mount(el, ctx) {
  const box = document.createElement('div');
  box.className = 'pt-scroll pt-detail';
  el.appendChild(box);
  const own = parseSource(ctx.node.properties && ctx.node.properties.source).layer;
  const show = async (layer, id) => {
    try {
      const g = await ctx.client.getLayerNodes(layer, { from: id });
      const n = g.nodes.find((x) => x.id === id);
      const dl = document.createElement('dl');
      const row = (k, v) => {
        const dt = document.createElement('dt');
        dt.textContent = k;
        const dd = document.createElement('dd');
        dd.textContent = typeof v === 'string' ? v : JSON.stringify(v);
        dl.append(dt, dd);
      };
      row('layer', layer);
      row('id', n.id);
      row('kind', n.kind);
      if (n.name) row('name', n.name);
      for (const [k, v] of Object.entries(n.properties || {})) row(k, v);
      row('reachable', `${g.nodes.length - 1} nodes below`);
      box.replaceChildren(dl);
    } catch (err) {
      box.textContent = JSON.stringify(err);
    }
  };
  const onSelect = (ev) => {
    if (!own || ev.detail.layer === own) show(ev.detail.layer, ev.detail.id);
  };
  ctx.bus.addEventListener('select', onSelect);
  const ref = ctx.ix.targets(ctx.node.id, 'displays')[0];
  if (ref && ref.properties) show(ref.properties.layer, ref.properties.node);
  else box.textContent = 'Select a node on a Canvas or Table.';
  return { box, off: () => ctx.bus.removeEventListener('select', onSelect) };
}

export function destroy(h) {
  h.off();
  h.box.remove();
}
