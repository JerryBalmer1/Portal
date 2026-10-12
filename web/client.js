// The one JS client for api/openapi.yaml. Hand-written, as in GraphRenderer:
// oapi-codegen v2.8.0 emits Go only. ROUTES keys are the spec's operationIds;
// the Go spec test reads this table and checks every entry against the spec,
// so the client and the spec cannot drift silently.
// Every API path names the tenant: createClient(tenant) fills {tenant}.
// A bearer token, when the viewer has given one (see setToken), goes on
// every call; without one the portal reads only.
// Every non-2xx response throws the spec Error object {code, message, problems?}.

export const ROUTES = Object.freeze({
  getTenant: { method: 'GET', path: '/api/tenants/{tenant}' },
  getDevToken: { method: 'GET', path: '/api/tenants/{tenant}/dev-token' },
  getConfig: { method: 'GET', path: '/api/tenants/{tenant}/config' },
  getMenu: { method: 'GET', path: '/api/tenants/{tenant}/menu' },
  getWhoami: { method: 'GET', path: '/api/tenants/{tenant}/auth/whoami' },
  listConnections: { method: 'GET', path: '/api/tenants/{tenant}/connections' },
  getConnectionRepos: { method: 'GET', path: '/api/tenants/{tenant}/connections/{name}/repos' },
  listPrincipals: { method: 'GET', path: '/api/tenants/{tenant}/principals' },
  postPrincipal: { method: 'POST', path: '/api/tenants/{tenant}/principals' },
  listTokens: { method: 'GET', path: '/api/tenants/{tenant}/tokens' },
  revokeToken: { method: 'DELETE', path: '/api/tenants/{tenant}/tokens/{id}' },
  listAgents: { method: 'GET', path: '/api/tenants/{tenant}/agents' },
  listAgentRuns: { method: 'GET', path: '/api/tenants/{tenant}/agents/{name}/runs' },
  listEmitters: { method: 'GET', path: '/api/tenants/{tenant}/emitters' },
  getEmitter: { method: 'GET', path: '/api/tenants/{tenant}/emitters/{name}' },
  listLedgers: { method: 'GET', path: '/api/tenants/{tenant}/ledger' },
  getLedger: { method: 'GET', path: '/api/tenants/{tenant}/ledger/{source}' },
  getLedgerEntry: { method: 'GET', path: '/api/tenants/{tenant}/ledger/{source}/{seq}' },
  getEnvironmentGraph: { method: 'GET', path: '/api/tenants/{tenant}/environments/graph' },
  getTenantGraph: { method: 'GET', path: '/api/tenants/{tenant}/graph' },
  listActions: { method: 'GET', path: '/api/tenants/{tenant}/actions' },
  postAction: { method: 'POST', path: '/api/tenants/{tenant}/actions' },
  getAction: { method: 'GET', path: '/api/tenants/{tenant}/actions/{seq}' },
  listAlerts: { method: 'GET', path: '/api/tenants/{tenant}/alerts' },
  getAlert: { method: 'GET', path: '/api/tenants/{tenant}/alerts/{fingerprint}' },
  listFeeds: { method: 'GET', path: '/api/tenants/{tenant}/feeds' },
  pushCard: { method: 'POST', path: '/api/tenants/{tenant}/feeds/{feed}/push' },
  listNodes: { method: 'GET', path: '/api/tenants/{tenant}/nodes' },
  getScreen: { method: 'GET', path: '/api/tenants/{tenant}/screen' },
  postNode: { method: 'POST', path: '/api/tenants/{tenant}/nodes' },
  patchNode: { method: 'PATCH', path: '/api/tenants/{tenant}/nodes/{id}' },
  deleteNode: { method: 'DELETE', path: '/api/tenants/{tenant}/nodes/{id}' },
  postEdge: { method: 'POST', path: '/api/tenants/{tenant}/edges' },
  listLayers: { method: 'GET', path: '/api/tenants/{tenant}/layers' },
  mountLayer: { method: 'POST', path: '/api/tenants/{tenant}/layers' },
  getLayerNodes: { method: 'GET', path: '/api/tenants/{tenant}/layers/{name}/nodes' },
  postSnap: { method: 'POST', path: '/api/tenants/{tenant}/snap' },
  getDiff: { method: 'GET', path: '/api/tenants/{tenant}/diff' },
  getOntology: { method: 'GET', path: '/api/tenants/{tenant}/ontology' },
});

// The token lives for the browser tab only (sessionStorage): it is the
// viewer's credential, not page state, and never part of the graph.
const TOKEN_KEY = 'portal.token';

export function getToken() {
  try {
    return sessionStorage.getItem(TOKEN_KEY) || '';
  } catch {
    return '';
  }
}

export function setToken(token) {
  try {
    if (token) sessionStorage.setItem(TOKEN_KEY, token);
    else sessionStorage.removeItem(TOKEN_KEY);
  } catch {
    // Storage blocked: the token holds until the page is left.
  }
}

export function createClient(tenant, base = '') {
  async function call(op, { params, query, body, form } = {}) {
    const r = ROUTES[op];
    const all = { tenant, ...params };
    const path = r.path.replace(/\{(\w+)\}/g, (_, k) => encodeURIComponent(all[k]));
    const url = new URL(base + path, window.location.href);
    for (const [k, v] of Object.entries(query || {})) {
      if (v !== undefined && v !== null && v !== '') url.searchParams.set(k, v);
    }
    const init = { method: r.method, headers: { Accept: 'application/json' } };
    const token = getToken();
    if (token) init.headers.Authorization = `Bearer ${token}`;
    if (body !== undefined) {
      init.headers['Content-Type'] = 'application/json';
      init.body = JSON.stringify(body);
    }
    // A FormData body sets its own multipart Content-Type and boundary.
    if (form !== undefined) init.body = form;
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
    tenant,
    getTenant: () => call('getTenant'),
    getDevToken: () => call('getDevToken'),
    getConfig: () => call('getConfig'),
    getMenu: () => call('getMenu'),
    getWhoami: () => call('getWhoami'),
    listConnections: () => call('listConnections'),
    getConnectionRepos: (name) => call('getConnectionRepos', { params: { name } }),
    listPrincipals: () => call('listPrincipals'),
    postPrincipal: (req) => call('postPrincipal', { body: req }),
    listTokens: () => call('listTokens'),
    revokeToken: (id) => call('revokeToken', { params: { id } }),
    listAgents: () => call('listAgents'),
    listAgentRuns: (name) => call('listAgentRuns', { params: { name } }),
    listEmitters: () => call('listEmitters'),
    getEmitter: (name) => call('getEmitter', { params: { name } }),
    listLedgers: () => call('listLedgers'),
    getLedger: (source, { limit, before, run } = {}) => call('getLedger', { params: { source }, query: { limit, before, run } }),
    getLedgerEntry: (source, seq) => call('getLedgerEntry', { params: { source, seq } }),
    getEnvironmentGraph: () => call('getEnvironmentGraph'),
    // getTenantGraph({source}): source keeps one emitter's layers, by their source block (docs/design.md 53).
    getTenantGraph: ({ source } = {}) => call('getTenantGraph', { query: { source } }),
    listActions: () => call('listActions'),
    // postAction asks; who asks is the token's principal, never the body's (docs/design.md 50).
    postAction: (action, subject) => call('postAction', { body: { action, subject } }),
    getAction: (seq) => call('getAction', { params: { seq } }),
    listAlerts: ({ limit, after, severity, rule, state, run, about } = {}) => call('listAlerts', { query: { limit, after, severity, rule, state, run, about } }),
    getAlert: (fingerprint) => call('getAlert', { params: { fingerprint } }),
    listFeeds: () => call('listFeeds'),
    // pushCard(feed, manifest, files): manifest a Blob of manifest.yaml, files [{path, blob}]. Nothing on the page
    // pushes in 0.6.0; Portal.Agent and Publish-PortalCard do.
    pushCard: (feed, manifest, files = []) => {
      const form = new FormData();
      form.append('manifest', manifest, 'manifest.yaml');
      for (const f of files) form.append('file', f.blob, f.path);
      return call('pushCard', { params: { feed }, form });
    },
    listNodes: (kind) => call('listNodes', { query: { kind } }),
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
