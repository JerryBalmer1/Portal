// Table: the nodes of a Widget's source query, one row each (id, Kind,
// name). Clicking a row selects the node for Detail widgets.
import { parseSource } from '/static/client.js';

export function mount(el, ctx) {
  const src = parseSource(ctx.node.properties && ctx.node.properties.source);
  const wrap = document.createElement('div');
  wrap.className = 'pt-scroll';
  el.appendChild(wrap);
  ctx.client.getLayerNodes(src.layer, { kind: src.kind, from: src.from }).then((graph) => {
    const t = document.createElement('table');
    t.className = 'pt-table';
    const head = t.createTHead().insertRow();
    for (const h of ['id', 'kind', 'name']) {
      const th = document.createElement('th');
      th.textContent = h;
      head.appendChild(th);
    }
    const body = t.createTBody();
    for (const n of graph.nodes) {
      const row = body.insertRow();
      for (const v of [n.id, n.kind, n.name || '']) row.insertCell().textContent = v;
      row.style.cursor = 'pointer';
      row.addEventListener('click', () => ctx.bus.dispatchEvent(new CustomEvent('select', { detail: { layer: src.layer, id: n.id } })));
    }
    wrap.appendChild(t);
  }).catch((err) => { wrap.textContent = JSON.stringify(err); });
  return { wrap };
}

export function destroy(h) { h.wrap.remove(); }
