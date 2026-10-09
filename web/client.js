// The one JS client for api/openapi.yaml. Hand-written, as in GraphRenderer:
// oapi-codegen v2.8.0 emits Go only. ROUTES keys are the spec's operationIds;
// the Go spec test reads this table and checks every entry against the spec,
// so the client and the spec cannot drift silently.
// Every non-2xx response throws the spec Error object {code, message, problems?}.

export const ROUTES = Object.freeze({
  getScreen: { method: 'GET', path: '/api/screen' },
  postNode: { method: 'POST', path: '/api/nodes' },
  patchNode: { method: 'PATCH', path: '/api/nodes/{id}' },
  deleteNode: { method: 'DELETE', path: '/api/nodes/{id}' },
  postEdge: { method: 'POST', path: '/api/edges' },
  listLayers: { method: 'GET', path: '/api/layers' },
  mountLayer: { method: 'POST', path: '/api/layers' },
  getLayerNodes: { method: 'GET', path: '/api/layers/{name}/nodes' },
  postSnap: { method: 'POST', path: '/api/snap' },
  getDiff: { method: 'GET', path: '/api/diff' },
  getOntology: { method: 'GET', path: '/api/ontology' },
});

export function createClient(base = '') {
  async function call(op, { params, query, body } = {}) {
    const r = ROUTES[op];
    const path = r.path.replace(/\{(\w+)\}/g, (_, k) => encodeURIComponent(params[k]));
    const url = new URL(base + path, window.location.href);
    for (const [k, v] of Object.entries(query || {})) {
      if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, v);
    }
    const init = { method: r.method, headers: { Accept: 'application/json' } };
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    let res;
    try {
      res = await fetch(url, init);
    } catch (e) {
      throw { code: 'network', message: String(e && e.message ? e.message : e) };
    }
    const text = await res.text();
    let data;
    try {
      data = text ? JSON.parse(text) : undefined;
    } catch {
      data = undefined;
    }
    if (!res.ok) {
      if (data && typeof data.code === 'string' && typeof data.message === 'string') throw data;
      throw { code: `http_${res.status}`, message: text || res.statusText };
    }
    return data;
  }

  return {
    getScreen: () => call('getScreen'),
    postNode: (node) => call('postNode', { body: node }),
    patchNode: (id, patch) => call('patchNode', { params: { id }, body: patch }),
    deleteNode: (id) => call('deleteNode', { params: { id } }),
    postEdge: (edge) => call('postEdge', { body: edge }),
    listLayers: () => call('listLayers'),
    mountLayer: (req) => call('mountLayer', { body: req }),
    getLayerNodes: (name, { kind, from } = {}) => call('getLayerNodes', { params: { name }, query: { kind, from } }),
    postSnap: (note) => call('postSnap', { body: note === undefined ? undefined : { note } }),
    getDiff: (a, b) => call('getDiff', { query: { a, b } }),
    getOntology: () => call('getOntology'),
  };
}

// parseSource reads a Widget source: "filesystem" or "filesystem?kind=File&from=fs:src".
export function parseSource(source) {
  const [layer, qs] = String(source || '').split('?', 2);
  const q = new URLSearchParams(qs || '');
  return { layer, kind: q.get('kind') || undefined, from: q.get('from') || undefined };
}
