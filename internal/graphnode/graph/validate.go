package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"sync"

	"github.com/JerryBalmer1/Portal/internal/graphnode/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// Problem is one validation finding. Severity is "error" or "warning"; only
// errors make an envelope or ontology invalid (design.md 12).
type Problem struct {
	Code     string `json:"code"` // schema | duplicate-node | duplicate-edge | dangling-edge | edge-id | edgeid | counts | kind | stamp | display | action | duplicate-name | emitter-name
	Path     string `json:"path"` // JSON pointer-ish location, or the element id
	Message  string `json:"message"`
	Severity string `json:"severity"` // error | warning
}

// Severities a Problem can carry.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

func (p Problem) String() string { return p.Severity + " " + p.Code + " " + p.Path + ": " + p.Message }

// HasErrors reports whether any problem is an error; warnings alone do not count.
func HasErrors(ps []Problem) bool {
	for _, p := range ps {
		if p.Severity != SeverityWarning {
			return true
		}
	}
	return false
}

// finish gives every problem without a severity "error", then drops repeats.
func finish(ps []Problem) []Problem {
	for i := range ps {
		if ps[i].Severity == "" {
			ps[i].Severity = SeverityError
		}
	}
	return dedupe(ps)
}

var (
	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

// schemaFor returns the compiled schema by file name, e.g. "envelope.schema.json"
// or "kinds/action-run.schema.json".
func schemaFor(name string) (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		c := jsonschema.NewCompiler()
		compiled = map[string]*jsonschema.Schema{}
		var names []string
		err := fs.WalkDir(schemas.FS, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(p, ".schema.json") {
				return nil
			}
			b, err := schemas.FS.ReadFile(p)
			if err != nil {
				return err
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
			if err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			url := schemas.Base + p
			if err := c.AddResource(url, doc); err != nil {
				return fmt.Errorf("%s: %w", p, err)
			}
			names = append(names, p)
			return nil
		})
		if err != nil {
			compileErr = err
			return
		}
		sort.Strings(names)
		for _, p := range names {
			s, err := c.Compile(schemas.Base + p)
			if err != nil {
				compileErr = fmt.Errorf("compile %s: %w", p, err)
				return
			}
			compiled[p] = s
		}
	})
	if compileErr != nil {
		return nil, compileErr
	}
	s, ok := compiled[name]
	if !ok {
		return nil, fmt.Errorf("no schema %q", name)
	}
	return s, nil
}

// SchemaNames lists the embedded schemas.
func SchemaNames() []string {
	if _, err := schemaFor("envelope.schema.json"); err != nil {
		return nil
	}
	names := make([]string, 0, len(compiled))
	for k := range compiled {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// SchemaBytes returns the raw embedded schema by name.
func SchemaBytes(name string) ([]byte, error) {
	return schemas.FS.ReadFile(name)
}

// ValidateBytes checks raw envelope JSON against the schema, then runs the
// structural rules. A nil result means conformant.
func ValidateBytes(data []byte) []Problem {
	var problems []Problem
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return []Problem{{Code: "schema", Path: "/", Message: "not JSON: " + err.Error()}}
	}
	problems = append(problems, validateAgainst("envelope.schema.json", "", doc)...)
	env, err := Decode(bytes.NewReader(data))
	if err != nil {
		return append(problems, Problem{Code: "schema", Path: "/", Message: "does not decode as an envelope: " + err.Error()})
	}
	problems = append(problems, structural(env)...)
	problems = append(problems, baseKinds(doc)...)
	return finish(problems)
}

// dedupe drops repeats: a base Kind node is checked by the envelope schema and
// again by its own schema, so a stamp problem on it would otherwise show twice.
func dedupe(ps []Problem) []Problem {
	seen := map[Problem]bool{}
	var out []Problem
	for _, p := range ps {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// Validate checks a typed envelope. It round-trips through JSON so the schema
// sees exactly what would be written to disk.
func Validate(env *Envelope) []Problem {
	b, err := Marshal(env)
	if err != nil {
		return []Problem{{Code: "schema", Path: "/", Message: err.Error()}}
	}
	return ValidateBytes(b)
}

// ValidateOntology checks a decoded ontology document (from YAML or JSON):
// the schema, with problems under a display block coded display and under an
// action coded action, then the 0.1.0 forms still accepted with a warning.
func ValidateOntology(doc any) []Problem {
	ps := validateAgainst("ontology.schema.json", "", doc)
	for i := range ps {
		switch {
		case strings.Contains(ps[i].Path, "/display"):
			ps[i].Code = "display"
		case strings.Contains(ps[i].Path, "/actions"):
			ps[i].Code = "action"
		}
	}
	ps = append(ps, idPatternProblems(doc)...)
	return finish(append(ps, ontologyWarnings(doc)...))
}

func validateAgainst(name, prefix string, doc any) []Problem {
	s, err := schemaFor(name)
	if err != nil {
		return []Problem{{Code: "schema", Path: "/", Message: err.Error()}}
	}
	err = s.Validate(doc)
	if err == nil {
		return nil
	}
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []Problem{{Code: "schema", Path: prefix + "/", Message: err.Error()}}
	}
	var out []Problem
	flatten(ve, prefix, &out)
	return out
}

func flatten(ve *jsonschema.ValidationError, prefix string, out *[]Problem) {
	if len(ve.Causes) == 0 {
		loc := prefix + "/" + strings.Join(ve.InstanceLocation, "/")
		msg := ve.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		code := "schema"
		if _, ok := ve.ErrorKind.(*kind.Pattern); ok && strings.HasSuffix(ve.SchemaURL, "/$defs/utcStamp") {
			code = "stamp"
		}
		*out = append(*out, Problem{Code: code, Path: loc, Message: msg})
		return
	}
	for _, c := range ve.Causes {
		flatten(c, prefix, out)
	}
}

func structural(env *Envelope) []Problem {
	var out []Problem
	ids := map[string]bool{}
	for i, n := range env.Nodes {
		if ids[n.ID] {
			out = append(out, Problem{Code: "duplicate-node", Path: fmt.Sprintf("/nodes/%d", i), Message: "node id " + n.ID + " appears more than once"})
		}
		ids[n.ID] = true
	}
	eids := map[string]bool{}
	for i, e := range env.Edges {
		p := fmt.Sprintf("/edges/%d", i)
		if eids[e.ID] {
			out = append(out, Problem{Code: "duplicate-edge", Path: p, Message: "edge id " + e.ID + " appears more than once"})
		}
		eids[e.ID] = true
		if from, to, kind, err := ParseEdgeID(e.ID); err != nil {
			out = append(out, Problem{Code: "edgeid", Path: p, Message: fmt.Sprintf("edge id %q is malformed: %v", e.ID, err)})
		} else if from != e.From || to != e.To || kind != e.Kind {
			out = append(out, Problem{Code: "edge-id", Path: p, Message: fmt.Sprintf("edge id %q must be %q", e.ID, EdgeID(e.From, e.To, e.Kind))})
		}
		if !ids[e.From] {
			out = append(out, dangling(env.Module, p, "from", e.From, e))
		}
		if !ids[e.To] {
			out = append(out, dangling(env.Module, p, "to", e.To, e))
		}
	}
	if env.Counts != nil {
		if env.Counts.Nodes != len(env.Nodes) {
			out = append(out, Problem{Code: "counts", Path: "/counts/nodes", Message: fmt.Sprintf("says %d, envelope has %d", env.Counts.Nodes, len(env.Nodes))})
		}
		if env.Counts.Edges != len(env.Edges) {
			out = append(out, Problem{Code: "counts", Path: "/counts/edges", Message: fmt.Sprintf("says %d, envelope has %d", env.Counts.Edges, len(env.Edges))})
		}
	}
	return out
}

// dangling reports an edge endpoint missing from the envelope. An endpoint
// whose registered prefix belongs to another module is a cross-layer edge that
// a merge resolves, so it is a warning; so is the target of an about edge from
// an alert: id onto any registered prefix, the module's own included, because
// an alert layer points into every other layer (design.md 26). Anything else is
// an error (design.md 14).
func dangling(module, path, end, id string, e Edge) Problem {
	if p, ok := PrefixOf(id); ok && p.Owner != module {
		return Problem{Code: "dangling-edge", Path: path, Severity: SeverityWarning,
			Message: end + " " + id + " is not a node in this envelope; its prefix " + p.Prefix + " belongs to " + p.Owner + ", resolved at merge"}
	} else if ok && end == "to" && e.Kind == "about" && strings.HasPrefix(e.From, "alert:") {
		return Problem{Code: "dangling-edge", Path: path, Severity: SeverityWarning,
			Message: end + " " + id + " is not a node in this envelope; an about edge from an alert: id may point at any registered prefix, " + p.Prefix + " here, resolved at merge"}
	}
	return Problem{Code: "dangling-edge", Path: path, Message: end + " " + id + " is not a node in this envelope"}
}

// baseKindSchemas maps a base Kind to the schema that constrains it.
var baseKindSchemas = map[string]string{
	"ActionRun": "kinds/action-run.schema.json",
	"Category":  "kinds/category.schema.json",
}

// baseKinds validates every node whose Kind is one GraphNode itself declares.
func baseKinds(doc any) []Problem {
	m, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	nodes, ok := m["nodes"].([]any)
	if !ok {
		return nil
	}
	var out []Problem
	for i, raw := range nodes {
		n, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := n["kind"].(string)
		name, ok := baseKindSchemas[kind]
		if !ok {
			continue
		}
		for _, p := range validateAgainst(name, fmt.Sprintf("/nodes/%d", i), raw) {
			if p.Code != "stamp" {
				p.Code = "kind"
			}
			out = append(out, p)
		}
	}
	return out
}

// MarshalProblems renders problems as JSON for tooling.
func MarshalProblems(ps []Problem) ([]byte, error) {
	if ps == nil {
		ps = []Problem{}
	}
	return json.MarshalIndent(ps, "", "  ")
}
