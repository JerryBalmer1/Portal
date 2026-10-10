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
	inst := c["menu"].([]any)[5].(map[string]any)["children"].([]any)[0].(map[string]any)
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
		"theme: dark\n":                             "'theme' not allowed",
		"product: { name: 1 }\n":                    "/product/name",
		"menu:\n  - { label: No id }\n":             "/menu/0",
		"menu:\n  - { id: overview, view: grid }\n": "/menu/0/view",
		"menu:\n  - id: extra\n    label: Extra\n    path: elsewhere\n    children: [{ id: p, label: P, path: extra/p, kind: k }]\n": "/menu/13/path",
		"menu:\n  - id: extra\n    label: Extra\n    path: extra\n    children: [{ id: p, label: P, path: extra/p }]\n":              "/menu/13/children/0/kind",
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
