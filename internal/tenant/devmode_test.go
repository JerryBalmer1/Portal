package tenant

import (
	"errors"
	"strings"
	"testing"
)

// devAdmin is the principal Invoke-Build Dev mints: an admin, createdBy null.
const devAdmin = "  - { id: 'ui:principal:dev-bootstrap', name: dev-bootstrap, type: user, role: admin, created: '2026-10-11T00:00:05.000Z', createdBy: null }\n"

// devTokens holds one token each for jerry and dev-bootstrap.
func devTokens() string {
	return "tokens:\n" +
		"  - { principal: 'ui:principal:jerry', sha256: " + HashToken("jerry-token") + ", created: '2026-10-11T00:00:00.000Z' }\n" +
		"  - { principal: 'ui:principal:dev-bootstrap', sha256: " + HashToken("dev-token") + ", created: '2026-10-11T00:00:05.000Z' }\n"
}

// The overlay's mode is dev, demo or prod; absent is prod, and the merged
// config always says which (docs/design.md 39).
func TestModeDefaultsToProdAndIsChecked(t *testing.T) {
	for overlay, want := range map[string]string{"{}": ModeProd, "mode: dev": ModeDev, "mode: demo": ModeDemo, "mode: prod": ModeProd} {
		tt, err := load(t, map[string]string{"portal.yaml": overlay})
		if err != nil {
			t.Fatalf("%s: %v", overlay, err)
		}
		if tt.Mode() != want || tt.Config["mode"] != want {
			t.Errorf("%s: Mode() %s, config mode %v, want %s", overlay, tt.Mode(), tt.Config["mode"], want)
		}
	}
	_, err := load(t, map[string]string{"portal.yaml": "mode: staging"})
	if err == nil || !strings.Contains(err.Error(), "portal.yaml") || !strings.Contains(err.Error(), "/mode") {
		t.Errorf("mode staging: %v", err)
	}
}

// dev-bootstrap may sit anywhere in principals.yaml with createdBy null,
// beside the real first admin, which keeps its own once-only rule.
func TestDevPrincipalBesideTheFirstAdmin(t *testing.T) {
	for name, text := range map[string]string{
		"after the first admin":  rootAdmin + devAdmin,
		"before the first admin": devAdmin + rootAdmin,
		"alone":                  devAdmin,
	} {
		if _, err := load(t, map[string]string{"portal.yaml": "mode: dev", "principals.yaml": "principals:\n" + text}); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for name, text := range map[string]string{
		"dev-bootstrap with a creator": rootAdmin + "  - { id: 'ui:principal:dev-bootstrap', name: dev-bootstrap, type: user, role: admin, created: '2026-10-11T00:00:05.000Z', createdBy: 'ui:principal:jerry' }\n",
		"dev-bootstrap a reader":       "  - { id: 'ui:principal:dev-bootstrap', name: dev-bootstrap, type: user, role: reader, created: '2026-10-11T00:00:05.000Z', createdBy: null }\n",
		"a second real root":           devAdmin + rootAdmin + "  - { id: 'ui:principal:eve', name: eve, type: user, role: admin, created: '2026-10-11T00:00:01.000Z', createdBy: null }\n",
	} {
		if _, err := load(t, map[string]string{"portal.yaml": "mode: dev", "principals.yaml": "principals:\n" + text}); err == nil || !strings.Contains(err.Error(), "principals.yaml") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// dev-bootstrap's tokens authenticate only in mode dev; DevToken names the
// principal only for one of its tokens in mode dev.
func TestDevTokensWorkOnlyInModeDev(t *testing.T) {
	files := map[string]string{"principals.yaml": "principals:\n" + rootAdmin + devAdmin, "tokens.yaml": devTokens()}
	for _, mode := range []string{ModeDev, ModeDemo, ModeProd} {
		files["portal.yaml"] = "mode: " + mode
		tt, err := load(t, files)
		if err != nil {
			t.Fatal(err)
		}
		p, err := tt.Authenticate("Bearer dev-token")
		_, devOK := tt.DevToken("dev-token")
		if mode == ModeDev {
			if err != nil || p.Name != DevPrincipalName || p.Role != RoleAdmin || !devOK {
				t.Errorf("dev: %+v %v %v", p, err, devOK)
			}
		} else if !errors.Is(err, ErrDevOnly) || devOK {
			t.Errorf("%s: dev-bootstrap's token gave %+v %v, DevToken %v", mode, p, err, devOK)
		}
		if me, err := tt.Authenticate("Bearer jerry-token"); err != nil || me.Name != "jerry" {
			t.Errorf("%s: jerry %+v %v", mode, me, err)
		}
		if _, ok := tt.DevToken("jerry-token"); ok {
			t.Errorf("%s: DevToken accepted jerry's token", mode)
		}
		if _, ok := tt.DevToken(""); ok {
			t.Errorf("%s: DevToken accepted no token", mode)
		}
		if tt.DevTokens() != 1 {
			t.Errorf("%s: DevTokens %d", mode, tt.DevTokens())
		}
	}
}

// No admin may add a principal named dev-bootstrap: it would carry a
// creator, which the file check refuses at the next boot.
func TestDevPrincipalNameIsReserved(t *testing.T) {
	tt, err := load(t, map[string]string{"portal.yaml": "{}", "principals.yaml": "principals:\n" + rootAdmin})
	if err != nil {
		t.Fatal(err)
	}
	jerry, _ := tt.PrincipalByID("ui:principal:jerry")
	if _, err := tt.AddPrincipal(jerry, DevPrincipalName, "user", RoleReader); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("add dev-bootstrap: %v", err)
	}
}
