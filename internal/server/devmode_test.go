package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

const devToken = "pt-dev-token"

// newModePortal serves a tenant whose overlay is mode (empty: no mode key)
// and whose files hold the test principals plus dev-bootstrap, with a token
// each. passToken is what cmd/portal would pass from PORTAL_TOKEN. It returns
// the server, its handler (for requests from elsewhere than loopback) and
// the tenant's directory.
func newModePortal(t *testing.T, mode, passToken string) (*httptest.Server, http.Handler, string) {
	t.Helper()
	p, err := store.New(store.Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	td := filepath.Join(dir, "default")
	if err := os.MkdirAll(td, 0o755); err != nil {
		t.Fatal(err)
	}
	overlay := "{}\n"
	if mode != "" {
		overlay = "mode: " + mode + "\n"
	}
	files := map[string]string{
		"portal.yaml": overlay,
		"principals.yaml": testPrincipals +
			"  - { id: 'ui:principal:dev-bootstrap', name: dev-bootstrap, type: user, role: admin, created: '2026-10-11T00:00:00.000Z', createdBy: null }\n",
		"tokens.yaml": "tokens:\n" +
			"  - { principal: 'ui:principal:tester', sha256: " + tenant.HashToken(testToken) + " }\n" +
			"  - { principal: 'ui:principal:reed', sha256: " + tenant.HashToken(readerToken) + " }\n" +
			"  - { principal: 'ui:principal:dev-bootstrap', sha256: " + tenant.HashToken(devToken) + " }\n",
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(td, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tt, err := tenant.Load(tenant.Options{Dir: dir, Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope})
	if err != nil {
		t.Fatal(err)
	}
	h := New(p, tt, Options{DevToken: passToken})
	s := httptest.NewServer(h)
	t.Cleanup(s.Close)
	return s, h, td
}

type devTokenBody struct{ Token, Principal, Role string }

// In mode dev, on loopback, with Invoke-Build Dev's token passed, the
// endpoint hands it out, and the token signs in as dev-bootstrap, admin.
func TestDevTokenInModeDev(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newModePortal(t, "dev", devToken)
	var info struct{ Name, Mode string }
	_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default", "", 200), &info)
	if info.Name != "default" || info.Mode != "dev" {
		t.Errorf("GET /api/tenants/default: %+v", info)
	}
	var got devTokenBody
	_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/dev-token", "", 200), &got)
	if got.Token != devToken || got.Principal != "ui:principal:dev-bootstrap" || got.Role != "admin" {
		t.Errorf("dev-token: %+v", got)
	}
	var me whoami
	_ = json.Unmarshal(h.doAs(t, got.Token, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200), &me)
	if me.Name != "dev-bootstrap" || me.Role == nil || *me.Role != "admin" || !me.Administers {
		t.Errorf("whoami with the dev token: %+v", me)
	}
	// Revoked, it is never handed out again.
	h.doAs(t, testToken, s.URL, "DELETE", "/api/tenants/default/tokens/"+tokenID(devToken), "", 200)
	h.doAs(t, "", s.URL, "GET", "/api/tenants/default/dev-token", "", 404)
}

// The endpoint is 404 for a caller or a Host that is not loopback, and
// when the binary was given no token, or one that is not dev-bootstrap's.
func TestDevTokenIs404OffLoopbackAndForAnyOtherToken(t *testing.T) {
	h := newHarness(t)
	_, handler, _ := newModePortal(t, "dev", devToken)
	for _, c := range []struct {
		name, remote, host string
		want               int
	}{
		{"loopback", "127.0.0.1:50000", "127.0.0.1:8200", 200},
		{"loopback ipv6", "[::1]:50000", "localhost:8200", 200},
		{"a LAN caller", "192.168.1.20:50000", "127.0.0.1:8200", 404},
		{"a public caller", "203.0.113.9:50000", "127.0.0.1:8200", 404},
		{"a rebound Host", "127.0.0.1:50000", "evil.example:8200", 404},
		{"a LAN Host", "127.0.0.1:50000", "192.168.1.20:8200", 404},
	} {
		req := httptest.NewRequest("GET", "/api/tenants/default/dev-token", nil)
		req.RemoteAddr, req.Host = c.remote, c.host
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: %d, want %d: %s", c.name, rec.Code, c.want, rec.Body)
		}
		if c.want == 200 && !strings.Contains(rec.Body.String(), devToken) {
			t.Errorf("%s: %s", c.name, rec.Body)
		}
		if c.want == 404 && strings.Contains(rec.Body.String(), devToken) {
			t.Errorf("%s: a 404 carried the token", c.name)
		}
	}
	for name, pass := range map[string]string{"no token": "", "the admin's token": testToken, "a reader's token": readerToken, "an unknown token": "nope"} {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newModePortal(t, "dev", pass)
			h.doAs(t, "", s.URL, "GET", "/api/tenants/default/dev-token", "", 404)
		})
	}
}

// Outside mode dev the endpoint is 404 even with the dev token passed, and
// dev-bootstrap's token is refused (401, naming the mode); the tenant says
// its mode, prod when the overlay names none.
func TestDevTokenIs404OutsideModeDev(t *testing.T) {
	h := newHarness(t)
	for mode, want := range map[string]string{"demo": "demo", "prod": "prod", "": "prod"} {
		s, _, _ := newModePortal(t, mode, devToken)
		var info struct{ Name, Mode string }
		_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default", "", 200), &info)
		if info.Mode != want {
			t.Errorf("mode %q: GET /api/tenants/default says %q", mode, info.Mode)
		}
		var cfg map[string]any
		_ = json.Unmarshal(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/config", "", 200), &cfg)
		if cfg["mode"] != want {
			t.Errorf("mode %q: config mode %v", mode, cfg["mode"])
		}
		body := string(h.doAs(t, "", s.URL, "GET", "/api/tenants/default/dev-token", "", 404))
		if strings.Contains(body, devToken) {
			t.Errorf("mode %q: a 404 carried the token", mode)
		}
		body = string(h.doAs(t, devToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 401))
		if !strings.Contains(body, "dev-bootstrap") || !strings.Contains(body, "mode is dev; it is "+want) {
			t.Errorf("mode %q: dev token on whoami: %s", mode, body)
		}
		h.doAs(t, testToken, s.URL, "GET", "/api/tenants/default/auth/whoami", "", 200)
	}
}

// A reader cannot add a principal; an admin signed in with the dev token can,
// and nobody can add one named dev-bootstrap.
func TestDevAdminAddsAndReaderCannot(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newModePortal(t, "dev", devToken)
	h.doAs(t, readerToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"ann","role":"reader"}`, 403)
	h.doAs(t, devToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"ann","role":"reader"}`, 201)
	h.doAs(t, devToken, s.URL, "POST", "/api/tenants/default/principals", `{"name":"dev-bootstrap","role":"reader"}`, 422)
}
