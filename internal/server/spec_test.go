package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// The harness is GraphRenderer's (internal/server/spec_test.go): every call
// is validated against the spec, request and response, status included.

const (
	specPath    = "../../api/openapi.yaml"
	clientPath  = "../../web/client.js"
	fixtureDir  = "../../tests/fixtures/layers/filesystem"
	runsDir     = "../../tests/fixtures/layers/agent-runs"
	invalidPath = "../../tests/fixtures/invalid.graph.json"
)

// Operations the page loads directly rather than through client.js, and
// mountLayerUnscoped, gone since 0.7.0 (410).
var pageLoaded = map[string]bool{"getIndex": true, "getStatic": true, "getShellPage": true, "getShellItem": true, "getLayerPage": true,
	"getAgentsPage": true, "getAgentPage": true, "getEmittersPage": true, "getEmitterPage": true, "mountLayerUnscoped": true}

func init() {
	text := func(r io.Reader, _ http.Header, _ *openapi3.SchemaRef, _ openapi3filter.EncodingFn) (any, error) {
		b, err := io.ReadAll(r)
		return string(b), err
	}
	for _, ct := range []string{"text/html", "text/javascript", "text/css"} {
		openapi3filter.RegisterBodyDecoder(ct, text)
	}
}

type harness struct {
	doc    *openapi3.T
	router routers.Router
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	r, err := legacy.NewRouter(doc)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{doc: doc, router: r}
}

// specOps returns "METHOD /path" -> operationId for every operation.
func (h *harness) specOps() map[string]string {
	out := map[string]string{}
	for path, item := range h.doc.Paths.Map() {
		for method, op := range item.Operations() {
			out[method+" "+path] = op.OperationID
		}
	}
	return out
}

// testToken is the bearer token of the test tenant's first admin,
// "tester"; do sends it unless told otherwise. The tenant also holds one
// token per other role and a legacy token from before 0.5.0, which names
// no principal (docs/design.md 38).
const (
	testToken    = "pt-test-token"
	tester       = "tester"
	contribToken = "pt-contrib-token"
	readerToken  = "pt-reader-token"
	legacyToken  = "pt-legacy-token"
	ghSecret     = "gh-secret"
)

// testPrincipals is the test tenant's principals.yaml: tester (admin, the
// first, createdBy null), contrib (contributor) and reed (reader).
const testPrincipals = `principals:
  - { id: 'ui:principal:tester', name: tester, type: user, role: admin, created: '2026-10-10T00:00:00.000Z', createdBy: null }
  - { id: 'ui:principal:contrib', name: contrib, type: user, role: contributor, created: '2026-10-10T00:00:01.000Z', createdBy: 'ui:principal:tester' }
  - { id: 'ui:principal:reed', name: reed, type: user, role: reader, created: '2026-10-10T00:00:02.000Z', createdBy: 'ui:principal:tester' }
`

// tokenID is how the API names a token: its hash's first 12 digits.
func tokenID(token string) string { return tenant.HashToken(token)[:tenant.TokenIDLength] }

// testEnv is the environment the GitHub caller reads: one connection's
// token is set, one is not.
var testEnv = map[string]string{"PT_GH": ghSecret, "PT_GH_BAD": "wrong", "PORTAL_NUGET_KEY": nugetKey}

// fakeGitHub answers GET /user/repos like GitHub: 101 repos over two pages
// for the right token, 401 otherwise.
func fakeGitHub(t *testing.T) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/user/repos" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+ghSecret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
			return
		}
		var repos []map[string]any
		from, to := 0, 100
		if r.URL.Query().Get("page") == "2" {
			from, to = 100, 101
		}
		for i := from; i < to; i++ {
			repos = append(repos, map[string]any{"full_name": fmt.Sprintf("octo/repo-%03d", i), "private": i%2 == 0,
				"default_branch": "main", "html_url": "https://github.com/octo/repo", "updated_at": "2026-10-01T00:00:00Z"})
		}
		_ = json.NewEncoder(w).Encode(repos)
	}))
	t.Cleanup(s.Close)
	return s
}

// newTenant writes a tenant directory (empty overlay, the test principals
// and their tokens, three github connections onto gh) and loads it.
func newTenant(t *testing.T, p *store.Portal, gh string) *tenant.Tenant {
	t.Helper()
	tt, _ := newTenantDir(t, p, gh)
	return tt
}

// newTenantDir is newTenant, also returning the tenant's directory.
func newTenantDir(t *testing.T, p *store.Portal, gh string) (*tenant.Tenant, string) {
	t.Helper()
	dir := t.TempDir()
	td := filepath.Join(dir, "default")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"portal.yaml":     "{}\n",
		"principals.yaml": testPrincipals,
		"tokens.yaml": "tokens:\n" +
			"  - { principal: 'ui:principal:tester', sha256: " + tenant.HashToken(testToken) + ", created: '2026-10-10T00:00:00.000Z' }\n" +
			"  - { principal: 'ui:principal:contrib', sha256: " + tenant.HashToken(contribToken) + ", created: '2026-10-10T00:00:01.000Z' }\n" +
			"  - { principal: 'ui:principal:reed', sha256: " + tenant.HashToken(readerToken) + ", created: '2026-10-10T00:00:02.000Z' }\n" +
			"  - { name: old, sha256: " + tenant.HashToken(legacyToken) + " }\n",
		"connections.yaml": "connections:\n" +
			"  - { name: gh, type: github, url: " + gh + ", token_env: PT_GH, created_by: 'ui:principal:tester' }\n" +
			"  - { name: gh-unset, type: github, url: " + gh + ", token_env: PT_GH_UNSET }\n" +
			"  - { name: gh-bad, type: github, url: " + gh + ", token_env: PT_GH_BAD }\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(td, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tt, err := tenant.Load(tenant.Options{Dir: dir, Base: portal.BaseYAML, Schema: portal.ConfigSchema,
		Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope})
	if err != nil {
		t.Fatal(err)
	}
	return tt, td
}

// serve wraps p for the test tenant, with the fake GitHub and testEnv.
func serve(t *testing.T, p *store.Portal) *httptest.Server {
	t.Helper()
	gh := fakeGitHub(t)
	h := New(p, newTenant(t, p, gh.URL), Options{GitHub: tenant.GitHub{Getenv: func(k string) string { return testEnv[k] }}, Feeds: fakeFeeds(t)})
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s
}

// newPortal starts a server on a fresh artifacts directory with the
// FileSystem fixture mounted as "filesystem".
func newPortal(t *testing.T, production bool) (*httptest.Server, *store.Portal, string) {
	t.Helper()
	dir := t.TempDir()
	p, err := store.New(store.Options{Artifacts: dir, Production: production})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.MountAtStart(store.Mount{Name: "filesystem", Path: fixtureDir}); err != nil {
		t.Fatal(err)
	}
	return serve(t, p), p, dir
}

// noRedirect is a client that hands back a redirect instead of following it.
var noRedirect = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// do sends one request with the test token and validates the response
// against the spec, including that the status code is documented for the
// operation.
func (h *harness) do(t *testing.T, base, method, target, body string, want int) []byte {
	t.Helper()
	return h.doAs(t, testToken, base, method, target, body, want)
}

// doAs is do with another token; "" sends no Authorization header.
func (h *harness) doAs(t *testing.T, token, base, method, target, body string, want int) []byte {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, base+target, rdr)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := noRedirect.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != want {
		t.Errorf("%s %s: status %d, want %d: %s", method, target, res.StatusCode, want, data)
	}
	// kin-openapi's router matches on the decoded path, so an id holding an
	// escaped slash (a Ref) is escaped once more for the validation copy.
	vreq, _ := http.NewRequest(method, strings.ReplaceAll(target, "%2F", "%252F"), bytes.NewReader([]byte(body)))
	if body != "" {
		vreq.Header.Set("Content-Type", "application/json")
	}
	route, params, err := h.router.FindRoute(vreq)
	if err != nil {
		t.Errorf("%s %s: not in spec: %v", method, target, err)
		return data
	}
	in := &openapi3filter.RequestValidationInput{Request: vreq, PathParams: params, Route: route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}}
	if want < 400 {
		if err := openapi3filter.ValidateRequest(context.Background(), in); err != nil {
			t.Errorf("%s %s: request invalid per spec: %v", method, target, err)
		}
	}
	out := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: in,
		Status:                 res.StatusCode,
		Header:                 res.Header,
		Options:                &openapi3filter.Options{IncludeResponseStatus: true},
	}
	out.SetBodyBytes(data)
	if err := openapi3filter.ValidateResponse(context.Background(), out); err != nil {
		t.Errorf("%s %s: response invalid per spec: %v", method, target, err)
	}
	return data
}

type call struct {
	method, target, body string
	want                 int
}

func snapStamp(t *testing.T, h *harness, base string) string {
	t.Helper()
	var s struct{ Stamp string }
	if err := json.Unmarshal(h.do(t, base, "POST", "/api/tenants/default/snap", "", 201), &s); err != nil {
		t.Fatal(err)
	}
	return s.Stamp
}

func TestEveryOperationAgainstSpec(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	invalid, err := os.ReadFile(invalidPath)
	if err != nil {
		t.Fatal(err)
	}
	fixture, _ := filepath.Abs(fixtureDir)
	fixtureJSON, _ := json.Marshal(fixture)
	runs, _ := filepath.Abs(runsDir)
	runsJSON, _ := json.Marshal(runs)
	envBody := `{"name":"inline","envelope":{"schema":"graph/1","module":"M","layer":"2026-10-09T00:00:00Z","nodes":[{"id":"a","kind":"K"}],"edges":[]}}`
	ref := url.PathEscape("ui:ref:filesystem/fs:src")

	a := snapStamp(t, h, s.URL)
	calls := []call{
		{"GET", "/", "", 302},
		{"GET", "/default/overview/summary", "", 200},
		{"GET", "/default/workflows/all/wf-1", "", 200},
		{"GET", "/default/nope/summary", "", 404},
		{"GET", "/other/overview/summary", "", 404},
		{"GET", "/default/overview/nope/x", "", 404},
		{"GET", "/api/tenants/default", "", 200},
		{"GET", "/api/tenants/other", "", 404},
		{"GET", "/api/tenants/default/dev-token", "", 404},
		{"GET", "/api/tenants/other/dev-token", "", 404},
		{"GET", "/api/tenants/default/config", "", 200},
		{"GET", "/api/tenants/other/config", "", 404},
		{"GET", "/api/tenants/default/menu", "", 200},
		{"GET", "/api/tenants/other/menu", "", 404},
		{"GET", "/api/tenants/default/auth/whoami", "", 200},
		{"GET", "/api/tenants/other/auth/whoami", "", 404},
		{"GET", "/api/tenants/default/connections", "", 200},
		{"GET", "/api/tenants/other/connections", "", 404},
		{"GET", "/api/tenants/default/connections/gh/repos", "", 200},
		{"GET", "/api/tenants/default/connections/gh-unset/repos", "", 422},
		{"GET", "/api/tenants/default/connections/gh-bad/repos", "", 502},
		{"GET", "/api/tenants/default/connections/nope/repos", "", 404},
		{"GET", "/api/tenants/default/principals", "", 200},
		{"GET", "/api/tenants/other/principals", "", 404},
		{"POST", "/api/tenants/default/principals", `{"name":"newbie","role":"reader"}`, 201},
		{"POST", "/api/tenants/default/principals", `{"name":"newbie","role":"reader"}`, 409},
		{"POST", "/api/tenants/default/principals", `{"name":"x","role":"owner"}`, 422},
		{"POST", "/api/tenants/default/principals", `{"name":"bot","role":"reader","type":"agent"}`, 422},
		{"POST", "/api/tenants/default/principals", `{"name":"x","role":"reader","extra":1}`, 400},
		{"POST", "/api/tenants/other/principals", `{"name":"x","role":"reader"}`, 404},
		{"GET", "/api/tenants/default/tokens", "", 200},
		{"GET", "/api/tenants/other/tokens", "", 404},
		{"DELETE", "/api/tenants/default/tokens/" + tokenID(legacyToken), "", 200},
		{"DELETE", "/api/tenants/default/tokens/" + tokenID(legacyToken), "", 404},
		{"GET", "/api/tenants/default/feeds", "", 200},
		{"GET", "/api/tenants/other/feeds", "", 404},
		{"GET", "/api/tenants/default/nodes?kind=Category", "", 200},
		{"GET", "/api/tenants/default/nodes", "", 400},
		{"GET", "/api/tenants/other/nodes?kind=Category", "", 404},
		{"POST", "/api/layers", `{"name":"fs3","path":` + string(fixtureJSON) + `}`, 410},
		{"POST", "/api/layers", `{"name":"Bad Name","path":"x"}`, 410},
		{"GET", "/default/layers/filesystem", "", 200},
		{"GET", "/default/layers/nope", "", 404},
		{"GET", "/other/layers/filesystem", "", 404},
		{"GET", "/default/agents", "", 200},
		{"GET", "/other/agents", "", 404},
		{"GET", "/api/tenants/default/agents", "", 200},
		{"GET", "/api/tenants/other/agents", "", 404},
		{"POST", "/api/tenants/default/layers", `{"name":"agent-runs","path":` + string(runsJSON) + `}`, 201},
		{"GET", "/default/agents/tester", "", 200},
		{"GET", "/default/agents/reed", "", 404},
		{"GET", "/other/agents/tester", "", 404},
		{"GET", "/api/tenants/default/agents/tester/runs", "", 200},
		{"GET", "/api/tenants/default/agents/reed/runs", "", 404},
		{"GET", "/api/tenants/other/agents/tester/runs", "", 404},
		{"GET", "/default/emitters", "", 200},
		{"GET", "/other/emitters", "", 404},
		{"GET", "/default/emitters/Graph.Emitter.FileSystem", "", 200},
		{"GET", "/default/emitters/sources", "", 200},
		{"GET", "/default/emitters/Graph.Emitter.Terraform", "", 404},
		{"GET", "/other/emitters/Graph.Emitter.FileSystem", "", 404},
		{"GET", "/api/tenants/default/emitters", "", 200},
		{"GET", "/api/tenants/other/emitters", "", 404},
		{"GET", "/api/tenants/default/emitters/Graph.Emitter.Agent", "", 200},
		{"GET", "/api/tenants/default/emitters/Graph.Emitter.Terraform", "", 404},
		{"GET", "/api/tenants/other/emitters/Graph.Emitter.Agent", "", 404},
		{"GET", "/static/portal.js", "", 200},
		{"GET", "/static/base.css", "", 200},
		{"GET", "/static/cytoscape.min.js", "", 200},
		{"GET", "/static/LICENSE", "", 404},
		{"GET", "/static/nope.js", "", 404},
		{"GET", "/api/tenants/default/screen", "", 200},
		{"GET", "/api/tenants/default/ontology", "", 200},
		{"GET", "/api/tenants/default/layers", "", 200},
		{"POST", "/api/tenants/default/layers", `{"name":"fs2","path":` + string(fixtureJSON) + `}`, 201},
		{"POST", "/api/tenants/default/layers", envBody, 201},
		{"POST", "/api/tenants/default/layers", `{"name":"bad","envelope":` + string(invalid) + `}`, 422},
		{"POST", "/api/tenants/default/layers", `{"name":"Bad Name","path":"x"}`, 400},
		{"POST", "/api/tenants/default/layers", `{"name":"none","path":"does/not/exist"}`, 404},
		{"GET", "/api/tenants/default/layers/filesystem/nodes", "", 200},
		{"GET", "/api/tenants/default/layers/filesystem/nodes?kind=File", "", 200},
		{"GET", "/api/tenants/default/layers/filesystem/nodes?from=fs:src&kind=Directory", "", 200},
		{"GET", "/api/tenants/default/layers/filesystem/nodes?from=fs:nope", "", 404},
		{"GET", "/api/tenants/default/layers/nope/nodes", "", 404},
		{"POST", "/api/tenants/default/nodes", `{"id":"ui:table","kind":"Widget","name":"files","properties":{"type":"Table","source":"filesystem?kind=File","bounds":{"x":0,"y":9,"w":6,"h":4}}}`, 201},
		{"POST", "/api/tenants/default/nodes", `{"id":"ui:table","kind":"Widget"}`, 409},
		{"POST", "/api/tenants/default/nodes", `not json`, 400},
		{"POST", "/api/tenants/default/nodes", `{"id":"ui:x","kind":"Widget","properties":{"color":"red"}}`, 422},
		{"POST", "/api/tenants/default/nodes", `{"id":"ui:ref:filesystem/fs:src","kind":"Ref","properties":{"layer":"filesystem","node":"fs:src"}}`, 201},
		{"POST", "/api/tenants/default/edges", `{"kind":"contains","from":"ui:root","to":"ui:table"}`, 201},
		{"POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:table","to":"ui:category:Table"}`, 201},
		{"POST", "/api/tenants/default/edges", `{"kind":"displays","from":"ui:table","to":"ui:ref:filesystem/fs:src"}`, 201},
		{"POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:table","to":"ui:category:Table"}`, 409},
		{"POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:table","to":"ui:category:Nope"}`, 422},
		{"POST", "/api/tenants/default/edges", `[]`, 400},
		{"PATCH", "/api/tenants/default/nodes/ui:table", `{"properties":{"bounds":{"x":6,"y":9,"w":6,"h":4}},"state":{"collapsed":true}}`, 200},
		{"PATCH", "/api/tenants/default/nodes/ui:table", `{"name":"renamed"}`, 400},
		{"PATCH", "/api/tenants/default/nodes/ui:table", `{"properties":{"selected":true}}`, 422},
		{"PATCH", "/api/tenants/default/nodes/ui:nope", `{"properties":{}}`, 404},
		{"DELETE", "/api/tenants/default/nodes/" + ref, "", 200},
		{"DELETE", "/api/tenants/default/nodes/ui:nope", "", 404},
		{"POST", "/api/tenants/default/snap", "", 201},
		{"POST", "/api/tenants/default/snap", `{"note":"table moved"}`, 201},
		{"POST", "/api/tenants/default/snap", `{"note":1}`, 400},
		{"POST", "/api/tenants/default/snap", `{"approve":true}`, 400},
		{"GET", "/api/tenants/default/diff?a=" + a + "&b=" + a, "", 200},
		{"GET", "/api/tenants/default/diff?a=" + a + "&b=2000-01-01T00-00-00.000Z", "", 404},
		{"GET", "/api/tenants/default/diff?a=../../etc&b=" + a, "", 400},
		{"GET", "/api/tenants/default/diff?a=" + a, "", 400},
	}
	hit := map[string]bool{}
	for _, c := range calls {
		h.do(t, s.URL, c.method, c.target, c.body, c.want)
		vreq, _ := http.NewRequest(c.method, strings.ReplaceAll(c.target, "%2F", "%252F"), nil)
		if route, _, err := h.router.FindRoute(vreq); err == nil {
			hit[c.method+" "+route.Path] = true
		}
	}
	for _, c := range pushCalls(t) {
		h.doForm(t, c.token, s.URL, c.target, c.parts, c.want)
		hit["POST /api/tenants/{tenant}/feeds/{feed}/push"] = true
	}
	for op := range h.specOps() {
		if !hit[op] {
			t.Errorf("spec operation %s is not exercised", op)
		}
	}
}

// Every registered route is in the spec, and every spec operation is
// registered.
func TestRouteParity(t *testing.T) {
	h := newHarness(t)
	p, err := store.New(store.Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(p, newTenant(t, p, "https://api.github.com"), Options{})
	routes, ok := handler.(chi.Routes)
	if !ok {
		t.Fatalf("handler is %T, not chi.Routes", handler)
	}
	registered := map[string]bool{}
	_ = chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		registered[method+" "+route] = true
		return nil
	})
	spec := h.specOps()
	for r := range registered {
		if _, ok := spec[r]; !ok {
			t.Errorf("registers %s, which the spec does not have", r)
		}
	}
	for r := range spec {
		if !registered[r] {
			t.Errorf("spec operation %s is not registered", r)
		}
	}
}

// client.js ROUTES must name spec operations by operationId with the spec's
// method and path, and every spec operation is in ROUTES or page-loaded.
func TestClientRoutesMatchSpec(t *testing.T) {
	h := newHarness(t)
	src, err := os.ReadFile(clientPath)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`(\w+):\s*\{\s*method:\s*'(\w+)',\s*path:\s*'([^']+)'\s*\}`)
	matches := re.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("no ROUTES entries found in client.js")
	}
	byID := map[string]string{}
	for route, id := range h.specOps() {
		byID[id] = route
	}
	inClient := map[string]bool{}
	for _, m := range matches {
		inClient[m[1]] = true
		if got := m[2] + " " + m[3]; byID[m[1]] != got {
			t.Errorf("client.js %s = %s; spec has %q", m[1], got, byID[m[1]])
		}
	}
	var missing []string
	for id := range byID {
		if !inClient[id] && !pageLoaded[id] {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("spec operations neither in client.js ROUTES nor page-loaded: %v", missing)
	}
}

func problemCodes(t *testing.T, body []byte) []string {
	t.Helper()
	var e struct {
		Problems []graph.Problem `json:"problems"`
	}
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range e.Problems {
		out = append(out, p.Code)
	}
	return out
}

func has(codes []string, code string) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

func TestDisplayPropertiesOnAPlainNodeAreRefused(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	for _, prop := range []string{"color", "shape", "icon", "collapsed", "selected", "scroll", "display"} {
		body := `{"id":"ui:w","kind":"Widget","properties":{"` + prop + `":true}}`
		if codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/nodes", body, 422)); !has(codes, "display") {
			t.Errorf("properties.%s: codes %v, want display", prop, codes)
		}
	}
	// A top-level display key is Graph.Node's own refusal.
	codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:w","kind":"Widget","display":{"color":"red"}}`, 422))
	if !has(codes, "schema") {
		t.Errorf("top-level display: codes %v, want schema", codes)
	}
	// The same properties are accepted on a State node.
	h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:state:x","kind":"State","properties":{"collapsed":true,"selected":false,"scroll":{"x":0,"y":10}}}`, 201)
}

func TestUnknownCategoryIsARefused(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:w","kind":"Widget"}`, 201)
	codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:w","to":"ui:category:Nope"}`, 422))
	if !has(codes, "unknown-category") {
		t.Errorf("is-a to a missing Category: codes %v", codes)
	}
	codes = problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:w","to":"ui:root"}`, 422))
	if !has(codes, "unknown-category") {
		t.Errorf("is-a to a Container: codes %v", codes)
	}
	codes = problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:w2","kind":"Widget","properties":{"type":"Nope"}}`, 422))
	if !has(codes, "unknown-category") {
		t.Errorf("Widget type naming no Category: codes %v", codes)
	}
	// A new panel type is a Category node and an is-a edge, nothing more.
	h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:category:Tree","kind":"Category","name":"Tree","properties":{"label":"Tree"}}`, 201)
	h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:category:Tree","to":"ui:category:Canvas"}`, 201)
	h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:w","to":"ui:category:Tree"}`, 201)
}

func TestPortalOntologyChecks(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	for body, code := range map[string]string{
		`{"id":"w","kind":"Widget"}`:                                                      "id-prefix",
		`{"id":"ui:w","kind":"Panel"}`:                                                    "unknown-kind",
		`{"id":"ui:w","kind":"Widget","properties":{"colour":1}}`:                         "unknown-property",
		`{"id":"ui:w","kind":"Widget","properties":{"bounds":{"x":0,"y":0,"w":0,"h":1}}}`: "property-type",
		`{"id":"ui:l","kind":"Layout","properties":{"mode":"tabs"}}`:                      "property-type",
		`{"id":"ui:l","kind":"Layout"}`:                                                   "property-type",
		`{"id":"ui:c","kind":"Category","name":"C","properties":{"description":"d"}}`:     "kind",
		// Graph.Node 0.2.0 passes a Category with no properties at all; the portal does not (docs/design.md 10).
		`{"id":"ui:c","kind":"Category","name":"C"}`:                                      "property-type",
		`{"id":"ui:r","kind":"Ref","properties":{"layer":"filesystem","node":"fs:nope"}}`: "unknown-ref",
		`{"id":"ui:w","kind":"Widget","extra":1}`:                                         "unknown-field",
	} {
		if codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/nodes", body, 422)); !has(codes, code) {
			t.Errorf("%s: codes %v, want %s", body, codes, code)
		}
	}
	for body, code := range map[string]string{
		`{"kind":"holds","from":"ui:root","to":"ui:canvas"}`:                                    "unknown-edge-kind",
		`{"kind":"contains","from":"ui:canvas","to":"ui:root"}`:                                 "edge-endpoints",
		`{"kind":"contains","from":"ui:root","to":"ui:nope"}`:                                   "dangling-edge",
		`{"id":"wrong","kind":"contains","from":"ui:root","to":"ui:canvas"}`:                    "edgeid",
		`{"id":"ui:root|ui:nope|contains","kind":"contains","from":"ui:root","to":"ui:canvas"}`: "edge-id",
	} {
		if codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/edges", body, 422)); !has(codes, code) {
			t.Errorf("%s: codes %v, want %s", body, codes, code)
		}
	}
}

func TestProductionHidesDesignerAndOnlyDesigner(t *testing.T) {
	h := newHarness(t)
	dev, _, _ := newPortal(t, false)
	prod, _, _ := newPortal(t, true)
	var full, shown graph.Envelope
	_ = json.Unmarshal(h.do(t, dev.URL, "GET", "/api/tenants/default/screen", "", 200), &full)
	_ = json.Unmarshal(h.do(t, prod.URL, "GET", "/api/tenants/default/screen", "", 200), &shown)
	kept := map[string]bool{}
	for _, n := range shown.Nodes {
		kept[n.ID] = true
	}
	var hidden []string
	for _, n := range full.Nodes {
		if !kept[n.ID] {
			hidden = append(hidden, n.ID)
		}
	}
	// State nodes stay, the section States too (docs/design.md 3, 28).
	if !slices.Equal(hidden, []string{"ui:designer"}) {
		t.Errorf("production hid %v, want exactly [ui:designer]", hidden)
	}
	for _, e := range shown.Edges {
		if e.From == "ui:designer" || e.To == "ui:designer" {
			t.Errorf("edge %s touches a hidden node", e.ID)
		}
	}
	raw, _ := json.Marshal(shown)
	if ps := graph.ValidateBytes(raw); len(ps) > 0 {
		t.Errorf("production screen does not validate: %v", ps)
	}
}

func TestSnapRoundTripsThroughGraphNodeValidate(t *testing.T) {
	h := newHarness(t)
	s, _, dir := newPortal(t, false)
	stamp := snapStamp(t, h, s.URL)
	path := filepath.Join(dir, "snaps", stamp+".graph.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if ps := graph.ValidateBytes(data); len(ps) > 0 {
		t.Errorf("snap fails graphnode validate: %v", ps)
	}
	if _, err := os.Stat(filepath.Join(dir, "snaps", "ontology.yaml")); err != nil {
		t.Errorf("ontology.yaml not beside the snap: %v", err)
	}
	var screen, snap graph.Envelope
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/screen", "", 200), &screen)
	_ = json.Unmarshal(data, &snap)
	a, _ := json.Marshal(screen.Nodes)
	b, _ := json.Marshal(snap.Nodes)
	if !bytes.Equal(a, b) {
		t.Errorf("snap nodes differ from the screen's")
	}
}

func TestDiff(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	a := snapStamp(t, h, s.URL)
	type result struct {
		Added, Removed struct{ Nodes, Edges []any }
		Changed        struct{ Nodes, Edges []store.Change }
		Merged         json.RawMessage
	}
	var same result
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/diff?a="+a+"&b="+a, "", 200), &same)
	if n := len(same.Added.Nodes) + len(same.Added.Edges) + len(same.Removed.Nodes) + len(same.Removed.Edges) + len(same.Changed.Nodes) + len(same.Changed.Edges); n != 0 {
		t.Errorf("diff of identical snaps has %d entries", n)
	}

	h.do(t, s.URL, "PATCH", "/api/tenants/default/nodes/ui:canvas", `{"properties":{"bounds":{"x":0,"y":0,"w":6,"h":9}}}`, 200)
	b := snapStamp(t, h, s.URL)
	var one result
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/diff?a="+a+"&b="+b, "", 200), &one)
	if len(one.Changed.Nodes) != 1 || one.Changed.Nodes[0].ID != "ui:canvas" || len(one.Added.Nodes)+len(one.Removed.Nodes)+len(one.Changed.Edges) != 0 {
		t.Errorf("one PATCH of bounds: %+v", one)
	}

	h.do(t, s.URL, "DELETE", "/api/tenants/default/nodes/ui:designer", "", 200)
	c := snapStamp(t, h, s.URL)
	var gone result
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/diff?a="+b+"&b="+c, "", 200), &gone)
	// Its contains and is-a edges, and a has-state edge per section; the
	// section States stay, as any node's State does when the node goes.
	if len(gone.Removed.Nodes) != 1 || len(gone.Removed.Edges) != 2+len(store.DesignerSections) {
		t.Errorf("delete of ui:designer: removed %d nodes, %d edges", len(gone.Removed.Nodes), len(gone.Removed.Edges))
	}
	// Design 11: the merge keeps what b dropped, marked absent, and validates.
	if !bytes.Contains(gone.Merged, []byte(`"kind":"absent"`)) {
		t.Errorf("merged envelope marks nothing absent")
	}
	if ps := graph.ValidateBytes(gone.Merged); len(ps) > 0 {
		t.Errorf("merged envelope does not validate: %v", ps)
	}
}

func TestLayerFailingValidationIsRefusedWithCodes(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	path, _ := filepath.Abs(invalidPath)
	pj, _ := json.Marshal(path)
	codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"bad","path":`+string(pj)+`}`, 422))
	// The codes graphnode validate reports on Graph.Node's invalid fixture.
	for _, want := range []string{"schema", "duplicate-node", "dangling-edge", "edge-id", "edgeid", "kind", "stamp"} {
		if !has(codes, want) {
			t.Errorf("refused layer: codes %v, missing %s", codes, want)
		}
	}
	var layers []struct{ Name string }
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/layers", "", 200), &layers)
	for _, l := range layers {
		if l.Name == "bad" {
			t.Error("a refused layer was mounted")
		}
	}
}

// A mount answers 201 with Location at the layer's page, which serves the
// shell; the pre-0.4.0 POST /api/layers is 410, names the tenant route and
// mounts nothing.
func TestMountLocationIsTheLayerPageAndTheOldRouteIsGone(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	fixture, _ := filepath.Abs(fixtureDir)
	pj, _ := json.Marshal(fixture)
	req, _ := http.NewRequest("POST", s.URL+"/api/tenants/default/layers", strings.NewReader(`{"name":"fs2","path":`+string(pj)+`}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 201 {
		t.Fatalf("mount: %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if loc != "/default/layers/fs2" {
		t.Fatalf("Location %q, want /default/layers/fs2", loc)
	}
	if page := string(h.doAs(t, "", s.URL, "GET", loc, "", 200)); !strings.Contains(page, `<script type="module" src="/static/portal.js">`) {
		t.Errorf("%s is not the shell", loc)
	}
	var e struct{ Code, Message string }
	_ = json.Unmarshal(h.do(t, s.URL, "POST", "/api/layers", `{"name":"fs3","path":`+string(pj)+`}`, 410), &e)
	if e.Code != "gone" || !strings.Contains(e.Message, "POST /api/tenants/default/layers") {
		t.Errorf("410 body %+v does not name the tenant route", e)
	}
	var layers []struct{ Name string }
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/layers", "", 200), &layers)
	for _, l := range layers {
		if l.Name == "fs3" {
			t.Error("POST /api/layers mounted fs3")
		}
	}
	h.doAs(t, "", s.URL, "GET", "/default/layers/fs3", "", 404)
}

func TestLayerOntologyAndQuery(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	var set struct {
		Portal map[string]any
		Layers map[string]map[string]any
	}
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/ontology", "", 200), &set)
	if set.Portal["module"] != "Portal" {
		t.Errorf("portal ontology module %v", set.Portal["module"])
	}
	kinds, _ := set.Layers["filesystem"]["kinds"].(map[string]any)
	dir, _ := kinds["Directory"].(map[string]any)
	if disp, _ := dir["display"].(map[string]any); disp["color"] == nil {
		t.Errorf("filesystem Directory has no display colour: %v", dir)
	}
	var env graph.Envelope
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/layers/filesystem/nodes?from=fs:src&kind=Directory", "", 200), &env)
	for _, n := range env.Nodes {
		if n.Kind != "Directory" || !strings.HasPrefix(n.ID, "fs:src") {
			t.Errorf("query kept %s (%s)", n.ID, n.Kind)
		}
	}
	if len(env.Nodes) == 0 {
		t.Error("query from fs:src found nothing")
	}
}

func TestApproveReportsTheDiffAgainstThePreviousSnap(t *testing.T) {
	h := newHarness(t)
	s, _, dir := newPortal(t, false)
	type approval struct {
		Stamp, Note, Previous string
		Diff                  *struct {
			Added, Removed, Changed struct{ Nodes, Edges []string }
		}
	}
	var first approval
	_ = json.Unmarshal(h.do(t, s.URL, "POST", "/api/tenants/default/snap", `{"note":"seed"}`, 201), &first)
	if first.Previous != "" || first.Diff != nil || first.Note != "seed" {
		t.Errorf("first approval: %+v", first)
	}
	h.do(t, s.URL, "PATCH", "/api/tenants/default/nodes/"+store.SeedTheme, `{"properties":{"opacity":0.3}}`, 200)
	var second approval
	_ = json.Unmarshal(h.do(t, s.URL, "POST", "/api/tenants/default/snap", `{"note":"lighter glass"}`, 201), &second)
	if second.Previous != first.Stamp || second.Diff == nil {
		t.Fatalf("second approval: %+v", second)
	}
	d := second.Diff
	if len(d.Changed.Nodes) != 1 || d.Changed.Nodes[0] != store.SeedTheme ||
		len(d.Added.Nodes)+len(d.Added.Edges)+len(d.Removed.Nodes)+len(d.Removed.Edges)+len(d.Changed.Edges) != 0 {
		t.Errorf("diff is not exactly the Theme: %+v", d)
	}
	data, err := os.ReadFile(filepath.Join(dir, "snaps", second.Stamp+".graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"kind": "approved"`)) || !bytes.Contains(data, []byte(`lighter glass`)) {
		t.Errorf("the snap does not carry its approval note")
	}
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Errorf("snap fails graphnode validate: %v", ps)
	}
}

func TestScreenReportsWhatItBootedFrom(t *testing.T) {
	newHarness(t)
	s, _, dir := newPortal(t, false)
	boot := func(url string) string {
		req, _ := http.NewRequest("GET", url+"/api/tenants/default/screen", nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.Header.Get("X-Portal-Boot")
	}
	if got := boot(s.URL); got != store.BootSeed {
		t.Errorf("fresh portal booted from %q", got)
	}
	req, _ := http.NewRequest("POST", s.URL+"/api/tenants/default/snap", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var snap struct{ Stamp string }
	_ = json.NewDecoder(res.Body).Decode(&snap)
	res.Body.Close()
	p, err := store.New(store.Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	again := serve(t, p)
	if got := boot(again.URL); got != snap.Stamp {
		t.Errorf("restart booted from %q, want %q", got, snap.Stamp)
	}
}

// A Category added through POST, refining Canvas, is on GET /api/screen with
// its refines (is-a) edge: what the Designer re-reads after every add.
func TestRefinedCategoryIsOnTheScreenWithItsRefinesEdge(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:category:Tree","kind":"Category","name":"Tree","properties":{"label":"Tree"}}`, 201)
	h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:category:Tree","to":"ui:category:Canvas"}`, 201)
	var env graph.Envelope
	if err := json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/screen", "", 200), &env); err != nil {
		t.Fatal(err)
	}
	i, ok := env.NodeIndex()["ui:category:Tree"]
	if !ok || env.Nodes[i].Kind != "Category" || env.Nodes[i].Name != "Tree" {
		t.Fatal("GET /api/screen lacks the Category Tree")
	}
	found := false
	for _, e := range env.Edges {
		found = found || (e.Kind == "is-a" && e.From == "ui:category:Tree" && e.To == "ui:category:Canvas")
	}
	if !found {
		t.Error("GET /api/screen lacks ui:category:Tree is-a ui:category:Canvas")
	}
}

// A menu entry is a node add and two edge adds through the existing POSTs;
// an approval snaps it and a restart boots it back (docs/design.md 25).
func TestMenuEntryAddedThroughPostSurvivesApproveAndRestart(t *testing.T) {
	h := newHarness(t)
	s, _, dir := newPortal(t, false)
	h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:menu:designer","kind":"Container","name":"Designer","properties":{"order":1}}`, 201)
	h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"is-a","from":"ui:menu:designer","to":"ui:category:Menu"}`, 201)
	h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"opens","from":"ui:menu:designer","to":"ui:designer"}`, 201)
	codes := problemCodes(t, h.do(t, s.URL, "POST", "/api/tenants/default/edges", `{"kind":"opens","from":"ui:menu:designer","to":"ui:layout:root"}`, 422))
	if !has(codes, "edge-endpoints") {
		t.Errorf("opens to a Layout: codes %v, want edge-endpoints", codes)
	}
	snapStamp(t, h, s.URL)
	p, err := store.New(store.Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := p.Screen()
	want := map[string]bool{
		"ui:menu:graph|ui:category:Menu|is-a": true, "ui:menu:graph|ui:canvas|opens": true,
		"ui:menu:designer|ui:category:Menu|is-a": true, "ui:menu:designer|ui:designer|opens": true,
	}
	for _, e := range env.Edges {
		delete(want, e.ID)
	}
	if len(want) > 0 {
		t.Errorf("after restart the screen lacks %v", want)
	}
}

// The served Designer draws the seven sections Go seeds, in the same order,
// with the same Title Case headings and the same defaults.
func TestServedDesignerHasTheSevenTitleCaseSections(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	page := string(h.do(t, s.URL, "GET", "/static/designer.js", "", 200))
	re := regexp.MustCompile(`\{ slug: '([^']+)', title: '([^']+)', open: (true|false) \}`)
	var got []store.DesignerSection
	for _, m := range re.FindAllStringSubmatch(page, -1) {
		got = append(got, store.DesignerSection{Slug: m[1], Title: m[2], Open: m[3] == "true"})
	}
	if !slices.Equal(got, store.DesignerSections) {
		t.Errorf("designer.js SECTIONS %v, want %v", got, store.DesignerSections)
	}
	titles := []string{"Approve", "Kinds (ontology.yaml)", "Categories", "Add Panel", "Add Container", "Add Category", "Add Menu Entry"}
	for i, sec := range got {
		if i >= len(titles) || sec.Title != titles[i] {
			t.Errorf("section %d is %q, want %q", i, sec.Title, titles[i])
		}
	}
	// Buttons say their section's title: no lower-cased label survives.
	for _, old := range []string{"title.toLowerCase()", "'Add panel'", "'Add container'", "'Add menu entry'"} {
		if strings.Contains(page, old) {
			t.Errorf("designer.js still has %s", old)
		}
	}
}

// POST /api/layers by path puts the Mount node on GET /api/screen, and an
// approved snap mounts the layer again at boot (docs/design.md 33).
func TestPathMountSurvivesApproveAndRestart(t *testing.T) {
	h := newHarness(t)
	s, _, dir := newPortal(t, false)
	path, _ := filepath.Abs(fixtureDir)
	pj, _ := json.Marshal(path)
	h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"agent-runs","path":`+string(pj)+`}`, 201)
	var env graph.Envelope
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/screen", "", 200), &env)
	i, ok := env.NodeIndex()[store.MountID("agent-runs")]
	if !ok || env.Nodes[i].Properties["path"] != path {
		t.Fatalf("screen has no ui:mount:agent-runs at %s", path)
	}
	snapStamp(t, h, s.URL)
	p, err := store.New(store.Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, failed := p.Remount(); len(failed) != 0 {
		t.Fatal(failed)
	}
	r := serve(t, p)
	var layers []struct {
		Name   string
		Counts struct{ Nodes int }
	}
	_ = json.Unmarshal(h.do(t, r.URL, "GET", "/api/tenants/default/layers", "", 200), &layers)
	if len(layers) != 1 || layers[0].Name != "agent-runs" || layers[0].Counts.Nodes == 0 {
		t.Errorf("after restart the layers are %+v", layers)
	}
}
