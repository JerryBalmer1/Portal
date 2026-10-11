package tenant

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"
)

// Roles, lowest first (docs/design.md 38). A reader reads; a contributor
// writes everything but identity and settings; an admin writes all.
const (
	RoleReader      = "reader"
	RoleContributor = "contributor"
	RoleAdmin       = "admin"
)

// Roles and PrincipalTypes are the values the files and the API accept.
var (
	Roles          = []string{RoleReader, RoleContributor, RoleAdmin}
	PrincipalTypes = []string{"user", "agent"}
)

// Rank orders roles: reader 1, contributor 2, admin 3; anything else 0.
func Rank(role string) int {
	for i, r := range Roles {
		if r == role {
			return i + 1
		}
	}
	return 0
}

// Principal is who a request is, and one entry of principals.yaml. A token
// from before 0.5.0 that names no principal is Legacy: its token's name,
// role admin, no id. Anonymous has no role.
type Principal struct {
	ID        string
	Name      string
	Type      string
	Role      string
	Created   string
	CreatedBy string // "" only for the first admin (null in the file)
	Anonymous bool
	Legacy    bool
}

// Anonymous is the principal of a request with no Authorization header. It
// reads and never writes.
var Anonymous = Principal{Name: "anonymous", Anonymous: true}

// Writes is whether the principal may write at all: contributor or admin.
func (p Principal) Writes() bool { return !p.Anonymous && Rank(p.Role) >= Rank(RoleContributor) }

// Administers is whether the principal may write identity and settings.
func (p Principal) Administers() bool { return !p.Anonymous && Rank(p.Role) >= Rank(RoleAdmin) }

// MayGrant is the attenuation rule: a principal creates principals of a
// role equal to or below its own, never above.
func (p Principal) MayGrant(role string) bool {
	return !p.Anonymous && Rank(role) > 0 && Rank(role) <= Rank(p.Role)
}

// ErrBadToken: an Authorization header that is not a bearer token this
// tenant holds.
var ErrBadToken = errors.New("bad token")

// ErrDevOnly: a token of the dev-bootstrap principal, presented while the
// tenant's mode is not dev (docs/design.md 39).
var ErrDevOnly = errors.New("dev-bootstrap's tokens work only in mode dev")

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// HashToken is how tokens.yaml stores a token: SHA-256, lower-case hex.
func HashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// Authenticate reads an Authorization header value. Empty is Anonymous;
// "Bearer <token>" whose hash this tenant holds is that token's principal
// (an admin named after the token when it names no principal); a token of
// dev-bootstrap outside mode dev is ErrDevOnly; anything else is
// ErrBadToken.
func (t *Tenant) Authenticate(header string) (Principal, error) {
	if header == "" {
		return Anonymous, nil
	}
	scheme, raw, ok := strings.Cut(header, " ")
	raw = strings.TrimSpace(raw)
	if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
		return Principal{}, ErrBadToken
	}
	sum := []byte(HashToken(raw))
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, k := range t.tokens {
		if subtle.ConstantTimeCompare(sum, []byte(k.SHA256)) == 1 {
			if k.Principal == "" {
				return Principal{Name: k.Name, Role: RoleAdmin, Legacy: true}, nil
			}
			for _, p := range t.principals {
				if p.ID == k.Principal {
					if p.Name == DevPrincipalName && t.Mode() != ModeDev {
						return Principal{}, ErrDevOnly
					}
					return p, nil
				}
			}
		}
	}
	return Principal{}, ErrBadToken
}

// DevToken is the dev-bootstrap principal when token is one of its tokens
// and the tenant's mode is dev; false otherwise. GET .../dev-token serves a
// token only when this holds at the time of the request, so a revoked dev
// token is never handed out (docs/design.md 39).
func (t *Tenant) DevToken(token string) (Principal, bool) {
	if token == "" || t.Mode() != ModeDev {
		return Principal{}, false
	}
	p, err := t.Authenticate("Bearer " + token)
	if err != nil || p.Name != DevPrincipalName {
		return Principal{}, false
	}
	return p, true
}

// DevTokens is how many tokens belong to dev-bootstrap: the binary warns
// at boot when a tenant not in mode dev still holds any.
func (t *Tenant) DevTokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	n := 0
	for _, k := range t.tokens {
		if k.Principal == PrincipalPrefix+DevPrincipalName {
			n++
		}
	}
	return n
}

// Tokens is how many tokens the tenant holds.
func (t *Tenant) Tokens() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tokens)
}
