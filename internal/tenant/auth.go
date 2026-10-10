package tenant

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Principal is who a request is: a token's name, or anonymous.
type Principal struct {
	Name      string
	Anonymous bool
}

// Anonymous is the principal of a request with no Authorization header. It
// reads and never writes.
var Anonymous = Principal{Name: "anonymous", Anonymous: true}

// ErrBadToken: an Authorization header that is not a bearer token this
// tenant holds.
var ErrBadToken = errors.New("bad token")

type token struct {
	Name   string `yaml:"name"`
	SHA256 string `yaml:"sha256"`
}

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// HashToken is how tokens.yaml stores a token: SHA-256, lower-case hex.
func HashToken(t string) string {
	sum := sha256.Sum256([]byte(t))
	return hex.EncodeToString(sum[:])
}

// loadTokens reads tokens.yaml; no file means no tokens, so nothing writes.
func loadTokens(path string) ([]token, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Tokens []token `yaml:"tokens"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, &ConfigError{File: path, Problems: []string{err.Error()}}
	}
	var ps []string
	names := map[string]bool{}
	for i, t := range doc.Tokens {
		if strings.TrimSpace(t.Name) == "" {
			ps = append(ps, fmt.Sprintf("/tokens/%d/name: required", i))
		}
		if names[t.Name] {
			ps = append(ps, fmt.Sprintf("/tokens/%d/name: %q twice", i, t.Name))
		}
		names[t.Name] = true
		if !sha256Hex.MatchString(t.SHA256) {
			ps = append(ps, fmt.Sprintf("/tokens/%d/sha256: 64 lower-case hex digits, the SHA-256 of the token", i))
		}
	}
	if len(ps) > 0 {
		return nil, &ConfigError{File: path, Problems: ps}
	}
	return doc.Tokens, nil
}

// Authenticate reads an Authorization header value. Empty is Anonymous;
// "Bearer <token>" whose hash this tenant holds is that token's principal;
// anything else is ErrBadToken.
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
	for _, k := range t.tokens {
		if subtle.ConstantTimeCompare(sum, []byte(k.SHA256)) == 1 {
			return Principal{Name: k.Name}, nil
		}
	}
	return Principal{}, ErrBadToken
}

// Tokens is how many tokens the tenant holds.
func (t *Tenant) Tokens() int { return len(t.tokens) }
