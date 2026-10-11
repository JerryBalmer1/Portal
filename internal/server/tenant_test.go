package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

type whoami struct {
	ID          *string
	Name        string
	Role        *string
	Anonymous   bool
	Legacy      bool
	Writes      bool
	Administers bool
	Tenant      string
	Marker      *string
}

// No token reads and never writes; a token the tenant does not hold is 401
// on a read too; the test token names its principal and writes.
func TestAuthNoTokenReadsBadTokenIs401GoodTokenNamesThePrincipal(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	var anon whoami
	_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200), &anon)
	if anon.Name != "anonymous" || !anon.Anonymous || anon.Writes || anon.Tenant != "default" || anon.Marker != nil {
		t.Errorf("no token: %+v", anon)
	}
	h.doAs(t, "", s.URL, "GET", "/api/tenants/default/screen", "", 200)
	for _, c := range []call{
		{"POST", "/api/tenants/default/nodes", `{"id":"ui:w","kind":"Widget"}`, 401},
		{"PATCH", "/api/tenants/default/nodes/ui:canvas", `{"properties":{}}`, 401},
		{"DELETE", "/api/tenants/default/nodes/ui:canvas", "", 401},
		{"POST", "/api/tenants/default/edges", `{"kind":"contains","from":"ui:root","to":"ui:canvas"}`, 401},
		{"POST", "/api/tenants/default/layers", `{"name":"x","path":"x"}`, 401},
		{"POST", "/api/layers", `{"name":"x","path":"x"}`, 401},
		{"POST", "/api/tenants/default/snap", "", 401},
	} {
		h.doAs(t, "", s.URL, c.method, c.target, c.body, c.want)
	}
	body := h.doAs(t, "not-a-token", s.URL, "GET", "/api/tenants/default/screen", "", 401)
	if !strings.Contains(string(body), `"unauthorized"`) {
		t.Errorf("bad token body: %s", body)
	}
	h.doAs(t, "not-a-token", s.URL, "GET", "/api/tenants/default/auth/whoami", "", 401)
	var me whoami
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200), &me)
	if me.Name != tester || me.Anonymous || !me.Writes || !me.Administers || me.ID == nil || *me.ID != "ui:principal:tester" || *me.Role != "admin" {
		t.Errorf("good token: %+v", me)
	}
	h.do(t, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:w","kind":"Widget"}`, 201)
	// The scheme must be Bearer.
	req, _ := http.NewRequest("GET", s.URL+"/api/tenants/default/screen", nil)
	req.Header.Set("Authorization", "Basic "+testToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("Basic auth: %d, WWW-Authenticate %q", res.StatusCode, res.Header.Get("WWW-Authenticate"))
	}
}

// The connections list names each token's variable and says whether it is
// set, and never carries a secret.
func TestConnectionsHideSecrets(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	body := h.doAs(t, "", s.URL, "GET", "/api/tenants/default/connections", "", 200)
	for _, secret := range testEnv {
		if strings.Contains(string(body), secret) {
			t.Errorf("connections list carries a secret: %s", body)
		}
	}
	var list []map[string]any
	_ = json.Unmarshal(body, &list)
	got := map[string]bool{}
	for _, c := range list {
		got[c["name"].(string)] = c["tokenSet"].(bool)
		if c["type"] != "github" || c["token_env"] == "" {
			t.Errorf("connection %v", c)
		}
		for k := range c {
			if !slices.Contains([]string{"name", "type", "url", "token_env", "tokenSet", "createdBy"}, k) {
				t.Errorf("connection carries %q", k)
			}
		}
		// Each connection records who made it; null in a file from before 0.5.0.
		if by := c["createdBy"]; (c["name"] == "gh" && by != "ui:principal:tester") || (c["name"] != "gh" && by != nil) {
			t.Errorf("connection %v createdBy %v", c["name"], by)
		}
	}
	if !got["gh"] || got["gh-unset"] || !got["gh-bad"] || len(got) != 3 {
		t.Errorf("tokenSet by connection: %v", got)
	}
}

// The repos endpoint pages through GitHub (mocked) and returns every repo as
// a git.repository node in a valid envelope; an unset token is 422, a token
// GitHub refuses is 502 carrying GitHub's message. It spends the tenant's
// secret, so it needs a bearer token though it is a GET.
func TestGitHubReposEndpoint(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	h.doAs(t, "", s.URL, "GET", "/api/tenants/default/connections/gh/repos", "", 401)
	h.doAs(t, "not-a-token", s.URL, "GET", "/api/tenants/default/connections/gh/repos", "", 401)
	data := h.do(t, s.URL, "GET", "/api/tenants/default/connections/gh/repos", "", 200)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Errorf("repos envelope does not validate: %v", ps)
	}
	var env graph.Envelope
	_ = json.Unmarshal(data, &env)
	if len(env.Nodes) != 101 {
		t.Fatalf("%d repos, want 101 over two pages", len(env.Nodes))
	}
	n := env.Nodes[0]
	if n.Kind != "git.repository" || n.ID != "ui:conn:gh/octo/repo-000" || n.Name != "octo/repo-000" || n.Properties["defaultBranch"] != "main" {
		t.Errorf("first repo node %+v", n)
	}
	if strings.Contains(string(data), ghSecret) {
		t.Error("repos envelope carries the token")
	}
	unset := string(h.do(t, s.URL, "GET", "/api/tenants/default/connections/gh-unset/repos", "", 422))
	if !strings.Contains(unset, "PT_GH_UNSET") {
		t.Errorf("unset token: %s", unset)
	}
	bad := string(h.do(t, s.URL, "GET", "/api/tenants/default/connections/gh-bad/repos", "", 502))
	if !strings.Contains(bad, "Bad credentials") || strings.Contains(bad, "wrong") {
		t.Errorf("refused token: %s", bad)
	}
}

// The menu is the brief's tree as Service and Page nodes, in order, with
// Settings docked at the bottom, icons in State nodes, and the workflow
// item tabs (Runs holding Steps) as Pages below All.
func TestMenuShape(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	data := h.doAs(t, "", s.URL, "GET", "/api/tenants/default/menu", "", 200)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Fatalf("menu does not validate: %v", ps)
	}
	var env graph.Envelope
	_ = json.Unmarshal(data, &env)
	idx := env.NodeIndex()
	pages := func(id string) []string {
		var out []struct {
			name  string
			order float64
		}
		for _, e := range env.Edges {
			if e.Kind == "has-page" && e.From == id {
				n := env.Nodes[idx[e.To]]
				o, _ := n.Properties["order"].(float64)
				out = append(out, struct {
					name  string
					order float64
				}{n.Name, o})
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
		var names []string
		for _, p := range out {
			names = append(names, p.name)
		}
		return names
	}
	var services []string
	for _, n := range env.Nodes {
		if n.Kind == "Service" {
			services = append(services, n.Name)
			if _, ok := idx["ui:state:rail:"+strings.TrimPrefix(n.ID, "ui:rail:")]; !ok {
				t.Errorf("%s has no icon State", n.ID)
			}
		}
	}
	want := []string{"Overview", "Graph", "Emitters", "Designer", "Workflows", "Environments", "Compliance", "Policy", "Ledger", "Identity", "Artifacts", "Stack", "Settings"}
	if !slices.Equal(services, want) {
		t.Errorf("services %v, want %v", services, want)
	}
	for id, want := range map[string][]string{
		"ui:rail:overview":           {"Summary", "Dashboards"},
		"ui:rail:graph":              {"Explore", "Kinds", "Taxonomy", "Layers"},
		"ui:rail:emitters":           {"Catalogue", "Sources", "Runs", "Schemas"},
		"ui:rail:designer":           {"Sections", "Themes", "Export"},
		"ui:rail:workflows":          {"All"},
		"ui:rail:workflows:all":      {"Plan", "Runs", "History"},
		"ui:rail:workflows:all:runs": {"Steps"},
		"ui:rail:environments":       {"Graph", "Instances", "Images", "Networks", "Actions"},
		"ui:rail:compliance":         {"Alerts", "Rules", "Findings", "Reports"},
		"ui:rail:policy":             {"Rules", "Decisions"},
		"ui:rail:ledger":             {"Entries", "Verify", "Keys"},
		"ui:rail:identity":           {"Users", "Agents", "Marker holders"},
		"ui:rail:artifacts":          {"Feeds", "Exports", "Bundles"},
		"ui:rail:stack":              {"Services", "Requests", "Events"},
		"ui:rail:settings":           {"Tenants", "Tokens", "Service connections"},
	} {
		if got := pages(id); !slices.Equal(got, want) {
			t.Errorf("%s pages %v, want %v", id, got, want)
		}
	}
	if d := env.Nodes[idx["ui:rail:settings"]].Properties["dock"]; d != "bottom" {
		t.Errorf("Settings dock %v", d)
	}
	inst := env.Nodes[idx["ui:rail:environments:instances"]].Properties["filter"]
	if b, _ := json.Marshal(inst); string(b) != `["container","compose","kubernetes","host"]` {
		t.Errorf("Instances filter %s", b)
	}
	if v := env.Nodes[idx["ui:rail:environments:graph"]].Properties; v["view"] != "environments-graph" || v["path"] != "environments/graph" {
		t.Errorf("Environments/Graph %v", v)
	}
	if v := env.Nodes[idx["ui:rail:compliance:alerts"]].Properties; v["view"] != "alerts" || v["path"] != "compliance/alerts" {
		t.Errorf("Compliance/Alerts %v", v)
	}
	if v := env.Nodes[idx["ui:rail:graph:explore"]].Properties; v["view"] != "screen" || v["screen"] != "ui:canvas" {
		t.Errorf("Graph/Explore %v", v)
	}
	if v := env.Nodes[idx["ui:rail:settings:service-connections"]].Properties; v["view"] != "connections" || v["path"] != "settings/service-connections" {
		t.Errorf("Settings/Service connections %v", v)
	}
}

// GET /nodes?kind= lists a Kind across the screen and the mounted layers,
// each node stamped with when its envelope was written.
func TestNodesOfKindSpansScreenAndLayers(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	var cats, dirs graph.Envelope
	_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/nodes?kind=Category", "", 200), &cats)
	if len(cats.Nodes) != 5 {
		t.Errorf("%d Categories, want the seed's 5", len(cats.Nodes))
	}
	_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/nodes?kind=Directory", "", 200), &dirs)
	if len(dirs.Nodes) == 0 {
		t.Fatal("no Directory from the filesystem layer")
	}
	for _, n := range append(cats.Nodes, dirs.Nodes...) {
		if n.Stamp == nil || n.Stamp.Layer == "" {
			t.Errorf("%s has no stamp.layer", n.ID)
		}
	}
	var none graph.Envelope
	_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/nodes?kind=workflow", "", 200), &none)
	if len(none.Nodes) != 0 {
		t.Errorf("workflow nodes: %d", len(none.Nodes))
	}
}

// / goes to the first page of the first service; a page route serves the
// shell for any page in the menu, and 404 for one that is not.
func TestShellRoutes(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	res, err := noRedirect.Get(s.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if loc := res.Header.Get("Location"); loc != "/default/overview/summary" {
		t.Errorf("/ redirects to %q", loc)
	}
	for _, path := range []string{"/default/overview/summary", "/default/settings/service-connections", "/default/graph/explore", "/default/graph/layers", "/default/workflows/all/some-id", "/default/layers/filesystem",
		"/default/emitters", "/default/emitters/catalogue", "/default/emitters/sources", "/default/emitters/runs/some-id", "/default/emitters/Graph.Emitter.FileSystem"} {
		page := string(h.doAs(t, "", s.URL, "GET", path, "", 200))
		if !strings.Contains(page, `<script type="module" src="/static/portal.js">`) {
			t.Errorf("%s is not the shell", path)
		}
	}
	h.doAs(t, "", s.URL, "GET", "/default/workflows/plan", "", 404)
	h.doAs(t, "", s.URL, "GET", "/default/layers/nope", "", 404)
	h.doAs(t, "", s.URL, "GET", "/default/emitters/nope", "", 404)
}
