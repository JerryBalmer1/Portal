package tenant

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// The tenant's identity is two files beside portal.yaml, both gitignored:
// principals.yaml (who) and tokens.yaml (the SHA-256 of each bearer token
// and the principal it belongs to). They are the only files the binary
// writes outside -artifacts: an admin's Add principal appends to the first,
// an admin's Revoke removes from the second (docs/design.md 38). Every write
// re-reads both files under the tenant's lock first, so a token New-PortalToken
// minted since boot is kept, and goes live with it.
const (
	PrincipalsFile = "principals.yaml"
	TokensFile     = "tokens.yaml"
	// PrincipalPrefix + name is a principal's id.
	PrincipalPrefix = "ui:principal:"
	// CreatedLayout is how created is written: UTC, milliseconds, Z.
	CreatedLayout = "2006-01-02T15:04:05.000Z"
	// TokenIDLength is how much of a token's SHA-256 names it in the API.
	TokenIDLength = 12
)

// PrincipalName is a principal's name: it is the tail of the id.
var PrincipalName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Errors of an identity write, mapped by the server onto 403, 409, 422 and 404.
var (
	ErrForbidden    = errors.New("forbidden")
	ErrExists       = errors.New("exists")
	ErrInvalid      = errors.New("invalid")
	ErrNoSuchToken  = errors.New("no such token")
	ErrNoPrincipals = errors.New("no principals")
)

type principalEntry struct {
	ID        string  `yaml:"id"`
	Name      string  `yaml:"name"`
	Type      string  `yaml:"type"`
	Role      string  `yaml:"role"`
	Created   string  `yaml:"created"`
	CreatedBy *string `yaml:"createdBy"`
}

// token is one entry of tokens.yaml: Principal names its owner; a token
// from before 0.5.0 has Name instead and acts as admin until rotated.
type token struct {
	Name      string `yaml:"name,omitempty"`
	Principal string `yaml:"principal,omitempty"`
	SHA256    string `yaml:"sha256"`
	Created   string `yaml:"created,omitempty"`
}

// TokenSummary is one token as the API lists it: never the hash entire.
type TokenSummary struct {
	ID        string
	Principal string // the owner's id; "" for a legacy token
	Name      string // the owner's name, or a legacy token's own
	Created   string
	Legacy    bool
}

func readYAML(path string, into any) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(into); err != nil && !errors.Is(err, io.EOF) {
		return true, &ConfigError{File: path, Problems: []string{err.Error()}}
	}
	return true, nil
}

// loadIdentity reads principals.yaml and tokens.yaml from dir and checks
// them together. No file means none of that kind.
func loadIdentity(dir string) ([]Principal, []token, error) {
	ppath := filepath.Join(dir, PrincipalsFile)
	var pdoc struct {
		Principals []principalEntry `yaml:"principals"`
	}
	if _, err := readYAML(ppath, &pdoc); err != nil {
		return nil, nil, err
	}
	var ps []string
	var out []Principal
	byID := map[string]Principal{}
	roots := 0
	for i, e := range pdoc.Principals {
		at := fmt.Sprintf("/principals/%d", i)
		if !PrincipalName.MatchString(e.Name) {
			ps = append(ps, fmt.Sprintf("%s/name: %q must match %s", at, e.Name, PrincipalName))
		}
		if e.ID != PrincipalPrefix+e.Name {
			ps = append(ps, fmt.Sprintf("%s/id: %q, want %q (ui:principal: and the name)", at, e.ID, PrincipalPrefix+e.Name))
		}
		if _, dup := byID[e.ID]; dup {
			ps = append(ps, fmt.Sprintf("%s/name: %q twice", at, e.Name))
		}
		if !slices.Contains(PrincipalTypes, e.Type) {
			ps = append(ps, fmt.Sprintf("%s/type: %q; one of %s", at, e.Type, strings.Join(PrincipalTypes, ", ")))
		}
		if Rank(e.Role) == 0 {
			ps = append(ps, fmt.Sprintf("%s/role: %q; one of %s", at, e.Role, strings.Join(Roles, ", ")))
		}
		if _, err := time.Parse(time.RFC3339, e.Created); err != nil {
			ps = append(ps, fmt.Sprintf("%s/created: %q is not an RFC 3339 time", at, e.Created))
		}
		p := Principal{ID: e.ID, Name: e.Name, Type: e.Type, Role: e.Role, Created: e.Created}
		if e.CreatedBy == nil {
			roots++
			if i != 0 || e.Role != RoleAdmin {
				ps = append(ps, fmt.Sprintf("%s/createdBy: null only for the first principal, an admin (New-PortalToken -Bootstrap)", at))
			}
		} else {
			p.CreatedBy = *e.CreatedBy
			by, ok := byID[p.CreatedBy]
			switch {
			case !ok:
				ps = append(ps, fmt.Sprintf("%s/createdBy: %q is no principal listed before this one", at, p.CreatedBy))
			case !by.MayGrant(e.Role):
				ps = append(ps, fmt.Sprintf("%s/role: %s, above its creator %s's %s (attenuation)", at, e.Role, by.Name, by.Role))
			}
		}
		byID[e.ID] = p
		out = append(out, p)
	}
	if len(ps) > 0 {
		return nil, nil, &ConfigError{File: ppath, Problems: ps}
	}

	tpath := filepath.Join(dir, TokensFile)
	var tdoc struct {
		Tokens []token `yaml:"tokens"`
	}
	if _, err := readYAML(tpath, &tdoc); err != nil {
		return nil, nil, err
	}
	names := map[string]bool{}
	ids := map[string]bool{}
	for i, t := range tdoc.Tokens {
		at := fmt.Sprintf("/tokens/%d", i)
		switch {
		case t.Principal != "" && t.Name != "":
			ps = append(ps, at+": principal or name, not both (name is a token from before 0.5.0)")
		case t.Principal != "":
			if _, ok := byID[t.Principal]; !ok {
				ps = append(ps, fmt.Sprintf("%s/principal: %q is in no entry of %s", at, t.Principal, PrincipalsFile))
			}
		case strings.TrimSpace(t.Name) == "":
			ps = append(ps, at+"/principal: required")
		default:
			if names[t.Name] {
				ps = append(ps, fmt.Sprintf("%s/name: %q twice", at, t.Name))
			}
			names[t.Name] = true
		}
		if !sha256Hex.MatchString(t.SHA256) {
			ps = append(ps, at+"/sha256: 64 lower-case hex digits, the SHA-256 of the token")
		} else if ids[t.SHA256[:TokenIDLength]] {
			ps = append(ps, at+"/sha256: the same first "+fmt.Sprint(TokenIDLength)+" digits as another token's; mint it again")
		} else {
			ids[t.SHA256[:TokenIDLength]] = true
		}
		if t.Created != "" {
			if _, err := time.Parse(time.RFC3339, t.Created); err != nil {
				ps = append(ps, fmt.Sprintf("%s/created: %q is not an RFC 3339 time", at, t.Created))
			}
		}
	}
	if len(ps) > 0 {
		return nil, nil, &ConfigError{File: tpath, Problems: ps}
	}
	return out, tdoc.Tokens, nil
}

// Principals is every principal, in file order.
func (t *Tenant) Principals() []Principal {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.principals)
}

// PrincipalByID returns the principal with that id.
func (t *Tenant) PrincipalByID(id string) (Principal, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, p := range t.principals {
		if p.ID == id {
			return p, true
		}
	}
	return Principal{}, false
}

// TokenList is every token, in file order, by id and owner.
func (t *Tenant) TokenList() []TokenSummary {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]TokenSummary, 0, len(t.tokens))
	for _, k := range t.tokens {
		out = append(out, t.summarise(k))
	}
	return out
}

func (t *Tenant) summarise(k token) TokenSummary {
	s := TokenSummary{ID: k.SHA256[:TokenIDLength], Principal: k.Principal, Name: k.Name, Created: k.Created, Legacy: k.Principal == ""}
	for _, p := range t.principals {
		if p.ID == k.Principal {
			s.Name = p.Name
		}
	}
	return s
}

// LegacyTokens names the tokens that belong to no principal: each acts as
// admin until it is rotated, and the binary warns about them at boot.
func (t *Tenant) LegacyTokens() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	var out []string
	for _, k := range t.tokens {
		if k.Principal == "" {
			out = append(out, k.Name)
		}
	}
	return out
}

// PrincipalsEnvelope serves the principals as Graph.Node nodes of Kind
// Principal (ontology.yaml): id ui:principal:<name>, properties type, role,
// created and createdBy, which the first admin's node leaves out.
func (t *Tenant) PrincipalsEnvelope(stamp string) *graph.Envelope {
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Ontology = "ontology.yaml"
	for _, p := range t.Principals() {
		props := map[string]any{"type": p.Type, "role": p.Role, "created": p.Created}
		if p.CreatedBy != "" {
			props["createdBy"] = p.CreatedBy
		}
		env.Nodes = append(env.Nodes, graph.Node{ID: p.ID, Kind: "Principal", Name: p.Name, Properties: props})
	}
	env.Recount()
	return env
}

// AddPrincipal creates a principal of type user as by. Attenuation comes
// first: by may grant only a role equal to or below its own (ErrForbidden).
// Then creating principals is identity, so by must be an admin, and one
// with a principal: a token from before 0.5.0 has no id to record as
// createdBy. A taken name is ErrExists; a bad name, role or type is
// ErrInvalid.
func (t *Tenant) AddPrincipal(by Principal, name, typ, role string) (Principal, error) {
	if typ == "" {
		typ = "user"
	}
	if Rank(role) == 0 {
		return Principal{}, fmt.Errorf("%w: role %q; one of %s", ErrInvalid, role, strings.Join(Roles, ", "))
	}
	if !by.MayGrant(role) {
		return Principal{}, fmt.Errorf("%w: attenuation: %s %s may create principals of role %s only, not %s", ErrForbidden,
			orNone(by.Role), by.Name, strings.Join(grantable(by.Role), " or "), role)
	}
	if !by.Administers() {
		return Principal{}, fmt.Errorf("%w: creating a principal is identity, which needs role admin; %s is %s", ErrForbidden, by.Name, orNone(by.Role))
	}
	if by.ID == "" {
		return Principal{}, fmt.Errorf("%w: %s's token belongs to no principal (from before 0.5.0), so it cannot be recorded as createdBy; bootstrap a principal with New-PortalToken -Bootstrap", ErrForbidden, by.Name)
	}
	if typ != "user" {
		return Principal{}, fmt.Errorf("%w: type %q; only user principals are created here (agents are minted via Marker, later)", ErrInvalid, typ)
	}
	if !PrincipalName.MatchString(name) {
		return Principal{}, fmt.Errorf("%w: name %q must match %s", ErrInvalid, name, PrincipalName)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ps, ts, err := loadIdentity(t.dir)
	if err != nil {
		return Principal{}, err
	}
	if !slices.ContainsFunc(ps, func(p Principal) bool { return p.ID == by.ID }) {
		return Principal{}, fmt.Errorf("%w: %s is no longer in %s", ErrForbidden, by.ID, PrincipalsFile)
	}
	if slices.ContainsFunc(ps, func(p Principal) bool { return p.Name == name }) {
		return Principal{}, fmt.Errorf("%w: a principal named %s", ErrExists, name)
	}
	p := Principal{ID: PrincipalPrefix + name, Name: name, Type: typ, Role: role, Created: t.now().UTC().Format(CreatedLayout), CreatedBy: by.ID}
	ps = append(ps, p)
	if err := writePrincipals(filepath.Join(t.dir, PrincipalsFile), t.Name, ps); err != nil {
		return Principal{}, err
	}
	t.principals, t.tokens = ps, ts
	return p, nil
}

// RevokeToken removes the token whose SHA-256 starts with id (TokenSummary.ID)
// from tokens.yaml, as by, who must be an admin. It stops working at once.
func (t *Tenant) RevokeToken(by Principal, id string) (TokenSummary, error) {
	if !by.Administers() {
		return TokenSummary{}, fmt.Errorf("%w: revoking a token is identity, which needs role admin; %s is %s", ErrForbidden, by.Name, orNone(by.Role))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	ps, ts, err := loadIdentity(t.dir)
	if err != nil {
		return TokenSummary{}, err
	}
	i := slices.IndexFunc(ts, func(k token) bool { return len(id) == TokenIDLength && strings.HasPrefix(k.SHA256, id) })
	if i < 0 {
		return TokenSummary{}, fmt.Errorf("%w: %s", ErrNoSuchToken, id)
	}
	gone := ts[i]
	ts = slices.Delete(ts, i, i+1)
	if err := writeTokens(filepath.Join(t.dir, TokensFile), t.Name, ts); err != nil {
		return TokenSummary{}, err
	}
	t.principals, t.tokens = ps, ts
	return t.summarise(gone), nil
}

func orNone(role string) string {
	if role == "" {
		return "of no role"
	}
	return role
}

func grantable(role string) []string {
	var out []string
	for _, r := range Roles {
		if Rank(r) <= Rank(role) {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return []string{"(none)"}
	}
	return out
}

// The files are written in one fixed block shape, the one New-PortalToken
// writes and reads: the file's leading comment kept, then one entry per
// item, every string single-quoted.

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func writePrincipals(path, tenantName string, ps []Principal) error {
	var b strings.Builder
	b.WriteString(header(path, "# Principals of tenant "+tenantName+": who may hold a token, and with what role (docs/design.md 38).\n"))
	b.WriteString("principals:\n")
	for _, p := range ps {
		by := "null"
		if p.CreatedBy != "" {
			by = quote(p.CreatedBy)
		}
		fmt.Fprintf(&b, "  - id: %s\n    name: %s\n    type: %s\n    role: %s\n    created: %s\n    createdBy: %s\n",
			quote(p.ID), quote(p.Name), p.Type, p.Role, quote(p.Created), by)
	}
	return writeAtomic(path, b.String())
}

func writeTokens(path, tenantName string, ts []token) error {
	var b strings.Builder
	b.WriteString(header(path, "# Bearer tokens of tenant "+tenantName+", as the SHA-256 of each token (New-PortalToken).\n"))
	if len(ts) == 0 {
		b.WriteString("tokens: []\n")
	} else {
		b.WriteString("tokens:\n")
	}
	for _, k := range ts {
		if k.Principal == "" {
			fmt.Fprintf(&b, "  - name: %s\n    sha256: %s\n", quote(k.Name), k.SHA256)
			continue
		}
		fmt.Fprintf(&b, "  - principal: %s\n    sha256: %s\n", quote(k.Principal), k.SHA256)
		if k.Created != "" {
			fmt.Fprintf(&b, "    created: %s\n", quote(k.Created))
		}
	}
	return writeAtomic(path, b.String())
}

// header is the leading comment lines of the file at path, or def.
func header(path, def string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return def
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "#") {
			break
		}
		b.WriteString(line)
	}
	if b.Len() == 0 {
		return def
	}
	return b.String()
}

// writeAtomic writes beside path and renames over it, so a reader never
// sees half a file.
func writeAtomic(path, text string) error {
	var r [6]byte
	_, _ = rand.Read(r[:])
	tmp := path + ".tmp-" + hex.EncodeToString(r[:])
	if err := os.WriteFile(tmp, []byte(text), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
