package server

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// newIdentityPortal is newPortal, also returning the tenant's directory,
// where principals.yaml and tokens.yaml are written.
func newIdentityPortal(t *testing.T) (*httptest.Server, *store.Portal, string) {
	t.Helper()
	p, err := store.New(store.Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	tt, dir := newTenantDir(t, p, "https://api.github.com")
	s := httptest.NewServer(New(p, tt, Options{}))
	t.Cleanup(s.Close)
	return s, p, dir
}

// The role matrix (docs/design.md 38): reader reads; contributor reads and
// writes the screen, but not identity; admin does all three. Anonymous
// reads, and a write without a token stays 401.
func TestRoleMatrix(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newIdentityPortal(t)
	type want struct{ read, write, identity, revoke int }
	for _, c := range []struct {
		role, token string
		want        want
	}{
		{"anonymous", "", want{200, 401, 401, 401}},
		{"reader", readerToken, want{200, 403, 403, 403}},
		{"contributor", contribToken, want{200, 201, 403, 403}},
		{"admin", testToken, want{200, 201, 201, 200}},
	} {
		t.Run(c.role, func(t *testing.T) {
			h.doAs(t, c.token, s.URL, "GET", "/api/tenants/default/screen", "", c.want.read)
			h.doAs(t, c.token, s.URL, "GET", "/api/tenants/default/principals", "", c.want.read)
			h.doAs(t, c.token, s.URL, "GET", "/api/tenants/default/tokens", "", c.want.read)
			h.doAs(t, c.token, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:w-`+c.role+`","kind":"Widget"}`, c.want.write)
			h.doAs(t, c.token, s.URL, "POST", "/api/tenants/default/snap", "", c.want.write)
			h.doAs(t, c.token, s.URL, "POST", "/api/tenants/default/principals", `{"name":"by-`+c.role+`","role":"reader"}`, c.want.identity)
			h.doAs(t, c.token, s.URL, "DELETE", "/api/tenants/default/tokens/"+tokenID(legacyToken), "", c.want.revoke)
		})
	}
	// A 403 says which role refused, in the spec's Error body.
	body := string(h.doAs(t, readerToken, s.URL, "PATCH", "/api/tenants/default/nodes/ui:canvas", `{"properties":{}}`, 403))
	if !strings.Contains(body, `"forbidden"`) || !strings.Contains(body, "reader") {
		t.Errorf("reader's PATCH: %s", body)
	}
	var me whoami
	_ = json.Unmarshal(h.doAs(t, readerToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200), &me)
	if me.Writes || me.Administers || me.Role == nil || *me.Role != "reader" {
		t.Errorf("reader's whoami %+v", me)
	}
	_ = json.Unmarshal(h.doAs(t, contribToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200), &me)
	if !me.Writes || me.Administers || *me.Role != "contributor" {
		t.Errorf("contributor's whoami %+v", me)
	}
}

// A token from before 0.5.0 names no principal: it acts as admin until it
// is rotated, but cannot create a principal (there is no id to record as
// createdBy), and the boot names it.
func TestLegacyTokenIsAdminUntilRotated(t *testing.T) {
	h := newHarness(t)
	s, _, dir := newIdentityPortal(t)
	var me whoami
	_ = json.Unmarshal(h.doAs(t, legacyToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200), &me)
	if me.Name != "old" || !me.Legacy || me.ID != nil || me.Role == nil || *me.Role != "admin" || !me.Administers {
		t.Errorf("legacy whoami %+v", me)
	}
	h.doAs(t, legacyToken, s.URL, "POST", "/api/tenants/default/nodes", `{"id":"ui:w","kind":"Widget"}`, 201)
	body := string(h.doAs(t, legacyToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"x","role":"reader"}`, 403))
	if !strings.Contains(body, "-Bootstrap") {
		t.Errorf("legacy add: %s", body)
	}
	tt, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-11T00:00:00.000Z"})
	if err != nil {
		t.Fatal(err)
	}
	if got := tt.LegacyTokens(); len(got) != 1 || got[0] != "old" {
		t.Errorf("legacy tokens %v", got)
	}
}

// Attenuation, server-side: no principal creates one of a role above its
// own. A contributor asking for an admin is refused for attenuation before
// it is refused for not being an admin; an admin may create an admin.
func TestAttenuationRefusal(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newIdentityPortal(t)
	up := string(h.doAs(t, contribToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"boss","role":"admin"}`, 403))
	if !strings.Contains(up, "attenuation") {
		t.Errorf("contributor creating an admin: %s", up)
	}
	same := string(h.doAs(t, contribToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"peer","role":"reader"}`, 403))
	if strings.Contains(same, "attenuation") || !strings.Contains(same, "needs role admin") {
		t.Errorf("contributor creating a reader: %s", same)
	}
	h.doAs(t, readerToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"boss","role":"admin"}`, 403)
	h.do(t, s.URL, "POST", "/api/tenants/default/principals", `{"name":"boss","role":"admin"}`, 201)
}

// An added principal and a revoked token reach the files at once, and a
// reload sees them: the new principal (createdBy its admin, a valid
// Principal node), and no revoked token, which is refused from then on.
func TestAddAndRevokePersist(t *testing.T) {
	h := newHarness(t)
	s, p, dir := newIdentityPortal(t)
	var n graph.Node
	_ = json.Unmarshal(h.do(t, s.URL, "POST", "/api/tenants/default/principals", `{"name":"newbie","role":"contributor"}`, 201), &n)
	if n.ID != "ui:principal:newbie" || n.Kind != "Principal" || n.Properties["createdBy"] != "ui:principal:tester" ||
		n.Properties["role"] != "contributor" || n.Properties["type"] != "user" {
		t.Errorf("added %+v", n)
	}
	h.do(t, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200)
	h.doAs(t, contribToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200)
	gone := h.do(t, s.URL, "DELETE", "/api/tenants/default/tokens/"+tokenID(contribToken), "", 200)
	if !strings.Contains(string(gone), `"name":"contrib"`) {
		t.Errorf("revoked %s", gone)
	}
	h.doAs(t, contribToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 401)

	data := h.doAs(t, "", s.URL, "GET", "/api/tenants/default/principals", "", 200)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Errorf("principals envelope: %v", ps)
	}
	var env graph.Envelope
	_ = json.Unmarshal(data, &env)
	if ps := errorsOf(p.CheckEnvelope(&env)); len(ps) > 0 {
		t.Errorf("principals envelope against the portal ontology: %v", ps)
	}
	if len(env.Nodes) != 4 || env.Nodes[0].Properties["createdBy"] != nil {
		t.Errorf("principals %+v", env.Nodes)
	}

	tokens, _ := os.ReadFile(filepath.Join(dir, "tokens.yaml"))
	if strings.Contains(string(tokens), tenant.HashToken(contribToken)) || !strings.Contains(string(tokens), tenant.HashToken(readerToken)) {
		t.Errorf("tokens.yaml after revoke:\n%s", tokens)
	}
	tt, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-11T00:00:00.000Z"})
	if err != nil {
		t.Fatal(err)
	}
	if q, ok := tt.PrincipalByID("ui:principal:newbie"); !ok || q.CreatedBy != "ui:principal:tester" || q.Role != "contributor" {
		t.Errorf("reloaded newbie %+v %v", q, ok)
	}
	if _, err := tt.Authenticate("Bearer " + contribToken); err == nil {
		t.Error("a revoked token authenticates after a reload")
	}
	if tt.Tokens() != 3 {
		t.Errorf("%d tokens after one revoke of four", tt.Tokens())
	}
}

func errorsOf(ps []graph.Problem) []string {
	var out []string
	for _, p := range ps {
		if p.Severity != graph.SeverityWarning {
			out = append(out, fmt.Sprint(p))
		}
	}
	return out
}
