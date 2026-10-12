package graph

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Where an id prefix's format came from (design.md 23).
const (
	IDFormatOntology = "ontology"
	IDFormatRegistry = "registry"
)

// ErrUnregisteredPrefix is wrapped by ResolveIDFormat for a prefix the
// registry does not hold.
var ErrUnregisteredPrefix = errors.New("not a registered prefix")

// IDFormat is the format ids with one registered prefix must have. GraphNode
// reserves the prefix and its owner only; the format comes from the owner's
// ontology.yaml (its Kinds' idPattern) when that file is reachable beside the
// repo, else from the registry's Shape, the fallback (design.md 23).
type IDFormat struct {
	Prefix   string   `json:"prefix"`
	Owner    string   `json:"owner"`
	Source   string   `json:"source"`             // ontology | registry
	Ontology string   `json:"ontology,omitempty"` // the owner's ontology.yaml, when Source is ontology
	Patterns []string `json:"patterns,omitempty"` // the owner's idPatterns for this prefix, when Source is ontology
	Shape    string   `json:"shape"`              // the registry's shape, the fallback

	compiled []*regexp.Regexp
}

// OwnerOntologyPaths are where an owner's ontology.yaml is looked for under a
// siblings folder, in order: <siblings>/<Owner>/ontology.yaml, then
// <siblings>/<Owner>/src/<Owner>/ontology.yaml.
func OwnerOntologyPaths(siblings, owner string) []string {
	return []string{
		filepath.Join(siblings, owner, "ontology.yaml"),
		filepath.Join(siblings, owner, "src", owner, "ontology.yaml"),
	}
}

// ResolveIDFormat finds the format for a registered prefix ("fs" or "fs:").
// siblings is the folder holding the owners' repos side by side (the parent of
// the repo calling it); "" skips the lookup and uses the registry. An owner's
// ontology is used when one of OwnerOntologyPaths exists, reads as YAML, has
// module equal to the owner, and declares at least one Kind idPattern that
// starts with the prefix (after an optional ^). Otherwise the registry's Shape
// applies. The error is for an unregistered prefix, or an owner ontology that
// is used but holds an idPattern that does not compile.
func ResolveIDFormat(siblings, prefix string) (IDFormat, error) {
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
		return IDFormat{}, fmt.Errorf("%q is %w", prefix, ErrUnregisteredPrefix)
	}
	f := IDFormat{Prefix: reg.Prefix, Owner: reg.Owner, Source: IDFormatRegistry, Shape: reg.Shape}
	if siblings == "" {
		return f, nil
	}
	for _, path := range OwnerOntologyPaths(siblings, reg.Owner) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		doc, err := DecodeYAML(data)
		if err != nil {
			continue
		}
		if m, _ := doc.(map[string]any); m == nil || m["module"] != reg.Owner {
			continue
		}
		patterns := idPatternsFor(doc, prefix)
		if len(patterns) == 0 {
			continue
		}
		f.Source, f.Ontology, f.Patterns = IDFormatOntology, path, patterns
		for _, p := range patterns {
			re, err := regexp.Compile(`^(?:` + p + `)$`)
			if err != nil {
				return IDFormat{}, fmt.Errorf("%s: idPattern %q does not compile: %v", path, p, err)
			}
			f.compiled = append(f.compiled, re)
		}
		return f, nil
	}
	return f, nil
}

// idPatternsFor returns the idPatterns of an ontology's Kinds that start with
// prefix, literally, after an optional ^, in Kind name order.
func idPatternsFor(doc any, prefix string) []string {
	kinds := mapAt(doc, "kinds")
	var out []string
	for _, k := range sortedKeys(kinds) {
		p, _ := mapAt(kinds, k)["idPattern"].(string)
		if p != "" && strings.HasPrefix(strings.TrimPrefix(p, "^"), prefix) {
			out = append(out, p)
		}
	}
	return out
}

// Check reports why id does not have this format; nil means it does. With
// Source ontology the id must match one of Patterns whole; with registry it
// must have the registry's Shape (ValidID). LedgerRoot and a tenant chain's
// ids (LedgerTenantPrefix) pass either way.
func (f IDFormat) Check(id string) error {
	if f.Source != IDFormatOntology {
		return ValidID(f.Prefix, id)
	}
	if !strings.HasPrefix(id, f.Prefix) {
		return fmt.Errorf("does not start with %s", f.Prefix)
	}
	if id == f.Prefix {
		if id == LedgerRoot {
			return nil
		}
		return errors.New("nothing after the prefix")
	}
	if f.Prefix == LedgerRoot && isLedgerTenant(id[len(f.Prefix):]) {
		return nil
	}
	for _, re := range f.compiled {
		if re.MatchString(id) {
			return nil
		}
	}
	msg := "matches no idPattern for " + f.Prefix + " in " + f.Ontology + ": " + strings.Join(f.Patterns, " | ")
	if f.Prefix == LedgerRoot {
		msg += "; the bare ledger: and ledger:tenant/<tenant>/<seq> (ledger:tenant/<tenant> its root) pass whichever format applies"
	}
	return errors.New(msg)
}

// ValidIDIn is ValidID with the owner's ontology looked for under siblings
// first (ResolveIDFormat). siblings "" is ValidID.
func ValidIDIn(siblings, prefix, id string) error {
	f, err := ResolveIDFormat(siblings, prefix)
	if err != nil {
		return err
	}
	return f.Check(id)
}

// idPatternProblems reports a Kind idPattern that does not compile, code
// schema, so validate-ontology catches it before an id check uses it.
func idPatternProblems(doc any) []Problem {
	kinds := mapAt(doc, "kinds")
	var out []Problem
	for _, k := range sortedKeys(kinds) {
		p, ok := mapAt(kinds, k)["idPattern"].(string)
		if !ok || p == "" {
			continue
		}
		if _, err := regexp.Compile(`^(?:` + p + `)$`); err != nil {
			out = append(out, Problem{Code: "schema", Path: "/kinds/" + k + "/idPattern", Severity: SeverityError,
				Message: fmt.Sprintf("idPattern %q is not a regular expression: %v", p, err)})
		}
	}
	return out
}
