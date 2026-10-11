package tenant

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
)

func writeTenant(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	td := filepath.Join(dir, Default)
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(td, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func load(t *testing.T, files map[string]string) (*Tenant, error) {
	t.Helper()
	return Load(Options{Dir: writeTenant(t, files), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z"})
}

func menuIDs(c map[string]any) []string {
	var out []string
	for _, m := range c["menu"].([]any) {
		out = append(out, m.(map[string]any)["id"].(string))
	}
	return out
}

// The base alone (the committed empty overlay) is the product as shipped.
func TestBaseOnly(t *testing.T) {
	overlay, err := os.ReadFile("../../tenants/default/portal.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(portal.BaseYAML, overlay, portal.ConfigSchema)
	if err != nil {
		t.Fatal(err)
	}
	if c["product"].(map[string]any)["name"] != "Portal" {
		t.Errorf("product %v", c["product"])
	}
	if ids := menuIDs(c); len(ids) != 13 || ids[0] != "overview" || ids[12] != "settings" {
		t.Errorf("menu ids %v", ids)
	}
	tt, err := load(t, map[string]string{"portal.yaml": ""})
	if err != nil {
		t.Fatal(err)
	}
	if tt.Name != Default || tt.Tokens() != 0 || len(tt.Connections) != 0 {
		t.Errorf("tenant %s, %d tokens, %d connections", tt.Name, tt.Tokens(), len(tt.Connections))
	}
}

// theme.graph.wheelZoomSpeed is 1.0 in the base and a tenant may override
// it within 0.1..5 (docs/design.md 37).
func TestWheelZoomSpeed(t *testing.T) {
	speed := func(c map[string]any) any {
		return c["theme"].(map[string]any)["graph"].(map[string]any)["wheelZoomSpeed"]
	}
	c, err := LoadConfig(portal.BaseYAML, nil, portal.ConfigSchema)
	if err != nil {
		t.Fatal(err)
	}
	if got := speed(c); got != 1.0 {
		t.Errorf("base wheelZoomSpeed %v (%T), want 1.0", got, got)
	}
	for overlay, want := range map[string]float64{
		"theme: { graph: { wheelZoomSpeed: 2.5 } }\n": 2.5,
		"theme: { graph: { wheelZoomSpeed: 0.1 } }\n": 0.1,
		"theme: { graph: { wheelZoomSpeed: 5 } }\n":   5,
	} {
		c, err := LoadConfig(portal.BaseYAML, []byte(overlay), portal.ConfigSchema)
		if err != nil {
			t.Errorf("%q: %v", overlay, err)
			continue
		}
		if got := speed(c); got != want {
			t.Errorf("%q: wheelZoomSpeed %v, want %v", overlay, got, want)
		}
	}
}

// An overlay changes what it names: maps merge, a menu item merges on id,
// a new id is appended, a list without ids is replaced.
func TestBasePlusOverlay(t *testing.T) {
	overlay := `
product: { name: Acme Portal }
menu:
  - id: overview
    label: Home
    children:
      - { id: summary, label: Start }
  - id: billing
    label: Billing
    icon: ledger
    path: billing
    children:
      - { id: invoices, label: Invoices, path: billing/invoices, kind: billing.invoice }
  - id: environments
    children:
      - { id: instances, filter: [host] }
`
	c, err := LoadConfig(portal.BaseYAML, []byte(overlay), portal.ConfigSchema)
	if err != nil {
		t.Fatal(err)
	}
	p := c["product"].(map[string]any)
	if p["name"] != "Acme Portal" || p["help"] == nil {
		t.Errorf("product %v: name replaced, help kept", p)
	}
	ids := menuIDs(c)
	if len(ids) != 14 || ids[13] != "billing" || ids[0] != "overview" {
		t.Errorf("menu ids %v", ids)
	}
	ov := c["menu"].([]any)[0].(map[string]any)
	if ov["label"] != "Home" || ov["icon"] != "home" || ov["path"] != "overview" {
		t.Errorf("overview %v", ov)
	}
	kids := ov["children"].([]any)
	sum := kids[0].(map[string]any)
	if len(kids) != 2 || sum["label"] != "Start" || sum["kind"] != "portal.summary" {
		t.Errorf("overview children %v", kids)
	}
	// The environments graph comes first (docs/design.md 46), then Instances.
	inst := c["menu"].([]any)[5].(map[string]any)["children"].([]any)[1].(map[string]any)
	if f := inst["filter"].([]any); len(f) != 1 || f[0] != "host" || inst["kind"] != "environment.instance" {
		t.Errorf("instances %v", inst)
	}
	// The base is not changed by a merge.
	again, _ := LoadConfig(portal.BaseYAML, []byte("{}"), portal.ConfigSchema)
	if again["product"].(map[string]any)["name"] != "Portal" {
		t.Error("merge changed the base")
	}
}

// A bad overlay stops the load, naming the file and the place.
func TestBadOverlayFailsBoot(t *testing.T) {
	for overlay, want := range map[string]string{
		"colours: dark\n": "'colours' not allowed",
		"theme: dark\n":   "/theme",
		"theme: { graph: { wheelZoomSpeed: 10 } }\n":   "/theme/graph/wheelZoomSpeed",
		"theme: { graph: { wheelZoomSpeed: 0.05 } }\n": "/theme/graph/wheelZoomSpeed",
		"theme: { graph: { wheelZoom: 2 } }\n":         "'wheelZoom' not allowed",
		"product: { name: 1 }\n":                       "/product/name",
		"menu:\n  - { label: No id }\n":                "/menu/0",
		"menu:\n  - { id: overview, view: grid }\n":    "/menu/0/view",
		"menu:\n  - id: extra\n    label: Extra\n    path: elsewhere\n    children: [{ id: p, label: P, path: extra/p, kind: k }]\n": "/menu/13/path",
		"menu:\n  - id: extra\n    label: Extra\n    path: extra\n    children: [{ id: p, label: P, path: extra/p }]\n":              "/menu/13/children/0/kind",
		"menu:\n  - id: layers\n    label: Layers\n    path: layers\n    children: [{ id: p, label: P, path: layers/p, kind: k }]\n": "/menu/13/id: layers is the route of the layer pages",
		"menu:\n  - id: agents\n    label: Agents\n    path: agents\n    children: [{ id: p, label: P, path: agents/p, kind: k }]\n": "/menu/13/id: agents is the route of the agent pages",
		"menu:\n  - id: ledger\n    children: [{ id: portal, label: P, path: ledger/portal, kind: k }]\n":                            "/children/3/id: portal is a ledger source, whose entries' route /<tenant>/ledger/portal this page would take",
		"menu:\n  - id: environments\n    children: [{ id: graph, view: table, kind: environment.graph }]\n":                         "/children/0/view: environments/graph is the environments graph, where every Show graph lands; its view is environments-graph, not \"table\"",
		"menu: [": "yaml",
	} {
		_, err := load(t, map[string]string{"portal.yaml": overlay})
		var ce *ConfigError
		if !errors.As(err, &ce) {
			t.Errorf("%q: error %v, want a ConfigError", overlay, err)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want it to name %s", overlay, err, want)
		}
	}
	if _, err := Load(Options{Dir: t.TempDir(), Base: portal.BaseYAML, Schema: portal.ConfigSchema}); err == nil || !strings.Contains(err.Error(), "portal.yaml") {
		t.Errorf("a tenant without portal.yaml: %v", err)
	}
	if _, err := Load(Options{Name: "Bad Name", Dir: t.TempDir(), Base: portal.BaseYAML, Schema: portal.ConfigSchema}); err == nil {
		t.Error("tenant name with a space accepted")
	}
}

// tokens.yaml and connections.yaml are checked at load too.
func TestTokenAndConnectionFilesAreChecked(t *testing.T) {
	for name, text := range map[string]string{
		"tokens.yaml":      "tokens:\n  - { name: a, sha256: nothex }\n",
		"connections.yaml": "connections:\n  - { name: x, type: gitlab, url: https://x, token_env: X }\n",
	} {
		_, err := load(t, map[string]string{"portal.yaml": "{}", name: text})
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: %v", name, err)
		}
	}
	tt, err := load(t, map[string]string{"portal.yaml": "{}",
		"tokens.yaml":      "tokens:\n  - { name: jerry, sha256: " + HashToken("s3cret") + " }\n",
		"connections.yaml": "connections:\n  - { name: gh, type: github, token_env: GITHUB_TOKEN }\n"})
	if err != nil {
		t.Fatal(err)
	}
	if who, err := tt.Authenticate("Bearer s3cret"); err != nil || who.Name != "jerry" {
		t.Errorf("good token: %v %v", who, err)
	}
	if who, _ := tt.Authenticate(""); !who.Anonymous {
		t.Errorf("no header: %v", who)
	}
	for _, h := range []string{"Bearer nope", "s3cret", "Basic s3cret", "Bearer "} {
		if _, err := tt.Authenticate(h); !errors.Is(err, ErrBadToken) {
			t.Errorf("%q: %v", h, err)
		}
	}
	if c, _ := tt.Connection("gh"); c.URL != GitHubAPI {
		t.Errorf("github url defaults to %s, got %q", GitHubAPI, c.URL)
	}
}

func TestMergeKeyedListsAndScalars(t *testing.T) {
	base := map[string]any{"a": []any{"x", "y"}, "b": []any{map[string]any{"id": "1", "v": 1.0}}, "c": 1.0}
	got := Merge(base, map[string]any{"a": []any{"z"}, "b": []any{map[string]any{"id": "1", "w": 2.0}, map[string]any{"id": "2"}}, "c": nil}).(map[string]any)
	if a := got["a"].([]any); len(a) != 1 || a[0] != "z" {
		t.Errorf("unkeyed list not replaced: %v", a)
	}
	b := got["b"].([]any)
	if len(b) != 2 || b[0].(map[string]any)["v"] != 1.0 || b[0].(map[string]any)["w"] != 2.0 {
		t.Errorf("keyed list: %v", b)
	}
	if got["c"] != nil {
		t.Errorf("null in the overlay replaces: %v", got["c"])
	}
}

const rootAdmin = "  - { id: 'ui:principal:jerry', name: jerry, type: user, role: admin, created: '2026-10-11T00:00:00.000Z', createdBy: null }\n"

// Bootstrap happens once: createdBy is null on the first principal only,
// and that one is an admin (docs/design.md 38). A second root, a root that
// is not first, or a root that is not an admin stops the load.
func TestBootstrapOnceOnly(t *testing.T) {
	for name, text := range map[string]string{
		"second root": rootAdmin +
			"  - { id: 'ui:principal:eve', name: eve, type: user, role: admin, created: '2026-10-11T00:00:01.000Z', createdBy: null }\n",
		"root not first": "  - { id: 'ui:principal:eve', name: eve, type: user, role: reader, created: '2026-10-11T00:00:01.000Z', createdBy: 'ui:principal:jerry' }\n" + rootAdmin,
		"root not admin": "  - { id: 'ui:principal:jerry', name: jerry, type: user, role: contributor, created: '2026-10-11T00:00:00.000Z', createdBy: null }\n",
	} {
		_, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + text})
		if err == nil || !strings.Contains(err.Error(), "principals.yaml") || !strings.Contains(err.Error(), "createdBy") {
			t.Errorf("%s: %v", name, err)
		}
	}
	tt, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + rootAdmin})
	if err != nil {
		t.Fatal(err)
	}
	if ps := tt.Principals(); len(ps) != 1 || ps[0].CreatedBy != "" || ps[0].Role != RoleAdmin {
		t.Errorf("principals %+v", ps)
	}
}

// Attenuation holds in the file as in the API: a principal whose role is
// above its creator's stops the load, and AddPrincipal refuses it with
// ErrForbidden, naming attenuation, before it checks for admin.
func TestAttenuation(t *testing.T) {
	contrib := "  - { id: 'ui:principal:cy', name: cy, type: user, role: contributor, created: '2026-10-11T00:00:01.000Z', createdBy: 'ui:principal:jerry' }\n"
	_, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + rootAdmin + contrib +
		"  - { id: 'ui:principal:max', name: max, type: user, role: admin, created: '2026-10-11T00:00:02.000Z', createdBy: 'ui:principal:cy' }\n"})
	if err == nil || !strings.Contains(err.Error(), "attenuation") {
		t.Errorf("an admin made by a contributor: %v", err)
	}
	tt, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + rootAdmin + contrib})
	if err != nil {
		t.Fatal(err)
	}
	cy, _ := tt.PrincipalByID("ui:principal:cy")
	jerry, _ := tt.PrincipalByID("ui:principal:jerry")
	if _, err := tt.AddPrincipal(cy, "max", "user", RoleAdmin); !errors.Is(err, ErrForbidden) || !strings.Contains(err.Error(), "attenuation") {
		t.Errorf("contributor -> admin: %v", err)
	}
	if _, err := tt.AddPrincipal(cy, "rea", "user", RoleReader); !errors.Is(err, ErrForbidden) || strings.Contains(err.Error(), "attenuation") {
		t.Errorf("contributor -> reader: %v", err)
	}
	for _, role := range Roles {
		if !jerry.MayGrant(role) || cy.MayGrant(role) != (role != RoleAdmin) {
			t.Errorf("MayGrant %s: admin %v contributor %v", role, jerry.MayGrant(role), cy.MayGrant(role))
		}
	}
	if Anonymous.MayGrant(RoleReader) || jerry.MayGrant("owner") {
		t.Error("anonymous grants, or an unknown role is grantable")
	}
	p, err := tt.AddPrincipal(jerry, "max", "", RoleAdmin)
	if err != nil || p.CreatedBy != jerry.ID || p.Type != "user" {
		t.Errorf("admin -> admin: %+v %v", p, err)
	}
	if _, err := tt.AddPrincipal(jerry, "max", "user", RoleReader); !errors.Is(err, ErrExists) {
		t.Errorf("a taken name: %v", err)
	}
}

// A token names its principal, which principals.yaml must hold; a token
// that names none is legacy and acts as admin.
func TestTokensBelongToPrincipals(t *testing.T) {
	_, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + rootAdmin,
		"tokens.yaml": "tokens:\n  - { principal: 'ui:principal:nobody', sha256: " + HashToken("x") + " }\n"})
	if err == nil || !strings.Contains(err.Error(), "ui:principal:nobody") {
		t.Errorf("a token of no principal: %v", err)
	}
	tt, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + rootAdmin,
		"tokens.yaml": "tokens:\n  - { principal: 'ui:principal:jerry', sha256: " + HashToken("j") + " }\n  - { name: ci, sha256: " + HashToken("c") + " }\n"})
	if err != nil {
		t.Fatal(err)
	}
	if who, _ := tt.Authenticate("Bearer j"); who.ID != "ui:principal:jerry" || who.Legacy {
		t.Errorf("principal token: %+v", who)
	}
	if who, _ := tt.Authenticate("Bearer c"); who.Name != "ci" || !who.Legacy || who.Role != RoleAdmin {
		t.Errorf("legacy token: %+v", who)
	}
}

// The committed examples load together as a tenant's files: the example
// admin, a token of it and a connection it made.
func TestIdentityExamplesLoad(t *testing.T) {
	files := map[string]string{"portal.yaml": "{}"}
	for _, f := range []string{"principals", "tokens", "connections"} {
		data, err := os.ReadFile("../../tenants/default/" + f + ".example.yaml")
		if err != nil {
			t.Fatal(err)
		}
		files[f+".yaml"] = string(data)
	}
	tt, err := load(t, files)
	if err != nil {
		t.Fatal(err)
	}
	if ps := tt.Principals(); len(ps) != 1 || ps[0].Role != RoleAdmin || tt.Tokens() != 1 || len(tt.LegacyTokens()) != 0 || tt.Connections[0].CreatedBy != ps[0].ID {
		t.Errorf("examples: %+v, %d tokens, connection %+v", ps, tt.Tokens(), tt.Connections)
	}
}
