package graph

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// IDPrefix is one registered node id prefix: who issues ids with it and their
// shape. schemas/ids.md is the prose form of this table (design.md 14).
type IDPrefix struct {
	Prefix string `json:"prefix"`
	Owner  string `json:"owner"`
	Shape  string `json:"shape"`
}

var idPrefixes = []IDPrefix{
	{Prefix: "fs:", Owner: "Graph.Emitter.FileSystem", Shape: "fs: + path relative to the root, forward slashes; the root is fs:.; no trailing slash; case preserved"},
	{Prefix: "ui:", Owner: "Portal", Shape: "ui: + a non-empty name; Portal's screen graph (Portal ontology.yaml idRule per Kind)"},
	{Prefix: "run:", Owner: "Graph.RunReport", Shape: "run:<repo folder name>:<startedAt UTC stamp ending Z>"},
	{Prefix: "finding:", Owner: "Graph.RunReport", Shape: "finding:<n>, n a positive integer"},
	{Prefix: "decision:", Owner: "Graph.RunReport", Shape: "decision:<n>, n a positive integer"},
	{Prefix: "notrun:", Owner: "Graph.RunReport", Shape: "notrun:<n>, n a positive integer"},
	{Prefix: "touched:", Owner: "Graph.RunReport", Shape: "touched:<path relative to the repo>, forward slashes"},
	{Prefix: "alert:", Owner: "Claude.Agent.Alerts", Shape: "alert:<rule>:<seq>, or alert:<rule>:line-<n> for a ledger line without seq; rule matches ^[a-z][a-z0-9-]*$, seq and n positive integers"},
	{Prefix: "ledger:", Owner: "Claude.Agent.Alerts", Shape: "ledger:<seq>, or ledger:line-<n> for a ledger line without seq; seq and n positive integers"},
}

// IDPrefixes returns the registered id prefixes, in registry order.
func IDPrefixes() []IDPrefix {
	out := make([]IDPrefix, len(idPrefixes))
	copy(out, idPrefixes)
	return out
}

// PrefixOf returns the registered prefix an id starts with, if any.
func PrefixOf(id string) (IDPrefix, bool) {
	for _, p := range idPrefixes {
		if strings.HasPrefix(id, p.Prefix) {
			return p, true
		}
	}
	return IDPrefix{}, false
}

var (
	positiveInt = regexp.MustCompile(`^[1-9][0-9]*$`)
	runRest     = regexp.MustCompile(`^[^:/\\]+:[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?Z$`)
	ledgerRest  = regexp.MustCompile(`^(line-)?[1-9][0-9]*$`)
	alertRest   = regexp.MustCompile(`^[a-z][a-z0-9-]*:(line-)?[1-9][0-9]*$`)
)

// ValidID checks that id has the shape registered for prefix ("fs" or "fs:").
// A nil error means the id conforms.
func ValidID(prefix, id string) error {
	if !strings.HasSuffix(prefix, ":") {
		prefix += ":"
	}
	var reg *IDPrefix
	for i := range idPrefixes {
		if idPrefixes[i].Prefix == prefix {
			reg = &idPrefixes[i]
		}
	}
	if reg == nil {
		return fmt.Errorf("%q is not a registered prefix", prefix)
	}
	if !strings.HasPrefix(id, prefix) {
		return fmt.Errorf("does not start with %s", prefix)
	}
	rest := id[len(prefix):]
	if rest == "" {
		return errors.New("nothing after the prefix")
	}
	switch prefix {
	case "fs:":
		if rest == "." {
			return nil
		}
		return relativePath(rest, true)
	case "touched:":
		return relativePath(rest, false)
	case "run:":
		if !runRest.MatchString(rest) {
			return errors.New("must be run:<repo folder name>:<UTC stamp ending Z>")
		}
	case "alert:":
		if !alertRest.MatchString(rest) {
			return errors.New("must be alert:<rule>:<seq> or alert:<rule>:line-<n>")
		}
	case "ledger:":
		if !ledgerRest.MatchString(rest) {
			return errors.New("must be ledger:<seq> or ledger:line-<n>")
		}
	case "finding:", "decision:", "notrun:":
		if !positiveInt.MatchString(rest) {
			return errors.New("must end in a positive integer")
		}
	}
	return nil
}

// relativePath checks a root-relative path with forward slashes. strict also
// rejects . and .. segments, which an fs: id never has because the emitter
// writes the path relative to its root.
func relativePath(p string, strict bool) error {
	switch {
	case strings.Contains(p, `\`):
		return errors.New("uses a backslash; separators are forward slashes")
	case strings.HasPrefix(p, "/"):
		return errors.New("starts with a slash; paths are relative to the root")
	case strings.HasSuffix(p, "/"):
		return errors.New("ends with a slash")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == "" {
			return errors.New("has an empty path segment")
		}
		if strict && (seg == "." || seg == "..") {
			return fmt.Errorf("has a %q segment", seg)
		}
	}
	return nil
}

// escapeEdgePart percent-encodes the two characters that would break the
// from|to|kind split: % first, so an encoded | is never re-encoded.
func escapeEdgePart(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "%", "%25"), EdgeIDSeparator, "%7C")
}

// unescapeEdgePart reverses escapeEdgePart. Any % not starting %25 or %7C
// (either case) is malformed.
func unescapeEdgePart(s string) (string, error) {
	if !strings.Contains(s, "%") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		if i+2 >= len(s) {
			return "", fmt.Errorf("%q has a %% at %d not followed by two hex digits", s, i)
		}
		switch strings.ToUpper(s[i+1 : i+3]) {
		case "25":
			b.WriteByte('%')
		case "7C":
			b.WriteByte('|')
		default:
			return "", fmt.Errorf("%q has %s; only %%25 and %%7C are allowed", s, s[i:i+3])
		}
		i += 2
	}
	return b.String(), nil
}

// ParseEdgeID splits an edge id into its decoded from, to and kind. It fails
// when the id does not have exactly three |-separated parts or a part holds a
// malformed escape.
func ParseEdgeID(id string) (from, to, kind string, err error) {
	parts := strings.Split(id, EdgeIDSeparator)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("has %d |-separated parts, want 3 (escape | in ids as %%7C)", len(parts))
	}
	if from, err = unescapeEdgePart(parts[0]); err != nil {
		return "", "", "", err
	}
	if to, err = unescapeEdgePart(parts[1]); err != nil {
		return "", "", "", err
	}
	return from, to, parts[2], nil
}
