package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// legacyShapes are the 0.1.0 shape names still accepted with a warning until
// 0.3.0 (design.md 13).
var legacyShapes = map[string]string{
	"round-rectangle": "square",
	"rectangle":       "square",
	"ellipse":         "circle",
	"octagon":         "hexagon",
}

// legacyShape warns when a display block or display rule uses a 0.1.0 shape.
func legacyShape(block map[string]any, path string) []Problem {
	shape, _ := block["shape"].(string)
	use, legacy := legacyShapes[shape]
	if !legacy {
		return nil
	}
	return []Problem{{Code: "display", Path: path + "/shape", Severity: SeverityWarning,
		Message: fmt.Sprintf("shape %q is a 0.1.0 name accepted until 0.3.0; use circle, square, diamond, hexagon, triangle or star (nearest: %s)", shape, use)}}
}

// declaredProperties returns the property names a Kind declares, and false
// when it declares none, so nothing can be checked against them.
func declaredProperties(kd map[string]any) (map[string]bool, bool) {
	list, ok := kd["properties"].([]any)
	if !ok {
		return nil, false
	}
	names := map[string]bool{}
	for _, p := range list {
		switch t := p.(type) {
		case string:
			names[t] = true
		case map[string]any:
			if n, ok := t["name"].(string); ok {
				names[n] = true
			}
		}
	}
	return names, true
}

// ontologyWarnings finds the 0.1.0 forms the schema still accepts (a legacy
// display shape, an action whose inputs are a name -> type map) and display
// rules keyed on a property the Kind does not declare (design.md 18).
func ontologyWarnings(doc any) []Problem {
	kinds := mapAt(doc, "kinds")
	var out []Problem
	for _, k := range sortedKeys(kinds) {
		kd, _ := kinds[k].(map[string]any)
		display := mapAt(kd, "display")
		out = append(out, legacyShape(display, "/kinds/"+k+"/display")...)
		declared, hasDeclared := declaredProperties(kd)
		rules, _ := display["when"].([]any)
		for i, r := range rules {
			rm, _ := r.(map[string]any)
			p := fmt.Sprintf("/kinds/%s/display/when/%d", k, i)
			out = append(out, legacyShape(rm, p)...)
			if !hasDeclared {
				continue
			}
			for _, name := range sortedKeys(mapAt(rm, "properties")) {
				if !declared[name] {
					out = append(out, Problem{Code: "display", Path: p + "/properties/" + name, Severity: SeverityWarning,
						Message: fmt.Sprintf("display rule keys on %s, which Kind %s does not declare in properties", name, k)})
				}
			}
		}
		actions, _ := kd["actions"].([]any)
		for i, a := range actions {
			am, _ := a.(map[string]any)
			if _, legacy := am["inputs"].(map[string]any); legacy {
				out = append(out, Problem{Code: "action", Path: fmt.Sprintf("/kinds/%s/actions/%d/inputs", k, i), Severity: SeverityWarning,
					Message: "inputs as a name -> type map is the 0.1.0 form accepted until 0.3.0; use [{name, type, required}]"})
			}
		}
	}
	return out
}

// actionInput is one declared input, from either form.
type actionInput struct {
	Name     string
	Type     string
	Required bool
}

// declaredAction is an action as the ontology declares it.
type declaredAction struct {
	Inputs  []actionInput
	Returns string // "" when no returns is declared
}

// findAction looks up "<Kind>.<action>" in the ontology. The action name is
// after the last dot, because a Kind name may itself contain dots.
func findAction(ontology any, ref string) (declaredAction, bool) {
	dot := strings.LastIndex(ref, ".")
	if dot <= 0 || dot == len(ref)-1 {
		return declaredAction{}, false
	}
	kd := mapAt(mapAt(ontology, "kinds"), ref[:dot])
	actions, _ := kd["actions"].([]any)
	for _, a := range actions {
		switch t := a.(type) {
		case string:
			if t == ref[dot+1:] {
				return declaredAction{}, true
			}
		case map[string]any:
			if t["name"] != ref[dot+1:] {
				continue
			}
			var d declaredAction
			switch in := t["inputs"].(type) {
			case []any:
				for _, raw := range in {
					m, _ := raw.(map[string]any)
					name, _ := m["name"].(string)
					typ, _ := m["type"].(string)
					req, _ := m["required"].(bool)
					d.Inputs = append(d.Inputs, actionInput{Name: name, Type: typ, Required: req})
				}
			case map[string]any:
				for _, name := range sortedKeys(in) {
					typ, _ := in[name].(string)
					d.Inputs = append(d.Inputs, actionInput{Name: name, Type: typ})
				}
			}
			if r, ok := t["returns"].(map[string]any); ok {
				d.Returns, _ = r["type"].(string)
			}
			return d, true
		}
	}
	return declaredAction{}, false
}

// matchesType reports whether a decoded JSON value fits a declared type. A
// type that is not a JSON type names a Kind, and the value is that node's id.
func matchesType(v any, typ string) bool {
	switch typ {
	case "string":
		_, ok := v.(string)
		return ok
	case "integer":
		n, ok := v.(json.Number)
		if !ok {
			return false
		}
		_, err := n.Int64()
		return err == nil
	case "number":
		_, ok := v.(json.Number)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	default:
		s, ok := v.(string)
		return ok && s != ""
	}
}

// CheckActionRuns checks every ActionRun's inputs and result against the
// action its properties.action names in the ontology (design.md 16). An action
// the ontology does not declare is a warning, because the ontology may lag the
// emitter (design.md 12).
func CheckActionRuns(env *Envelope, ontology any) []Problem {
	var out []Problem
	for i, n := range env.Nodes {
		if n.Kind != "ActionRun" {
			continue
		}
		p := fmt.Sprintf("/nodes/%d/properties", i)
		ref, _ := n.Properties["action"].(string)
		if ref == "" {
			continue // the action-run schema reports the missing action
		}
		decl, ok := findAction(ontology, ref)
		if !ok {
			out = append(out, Problem{Code: "action", Path: p + "/action", Severity: SeverityWarning, Message: ref + " is not declared in the ontology"})
			continue
		}
		given, _ := n.Properties["inputs"].(map[string]any)
		declared := map[string]bool{}
		for _, in := range decl.Inputs {
			declared[in.Name] = true
			v, present := given[in.Name]
			switch {
			case !present && in.Required:
				out = append(out, Problem{Code: "action", Path: p + "/inputs", Message: fmt.Sprintf("%s requires input %s", ref, in.Name)})
			case present && !matchesType(v, in.Type):
				out = append(out, Problem{Code: "action", Path: p + "/inputs/" + in.Name, Message: fmt.Sprintf("%s input %s must be %s", ref, in.Name, in.Type)})
			}
		}
		for _, name := range sortedKeys(given) {
			if !declared[name] {
				out = append(out, Problem{Code: "action", Path: p + "/inputs/" + name, Message: fmt.Sprintf("%s declares no input %s", ref, name)})
			}
		}
		if result, present := n.Properties["result"]; present && decl.Returns != "" && !matchesType(result, decl.Returns) {
			out = append(out, Problem{Code: "action", Path: p + "/result", Message: fmt.Sprintf("%s returns %s", ref, decl.Returns)})
		}
	}
	return out
}

// ValidateWithOntology runs ValidateBytes, then validates the ontology itself
// (problems under path "ontology:"), then checks ActionRuns against it. It is
// the on-request cross-check of design.md 12.
func ValidateWithOntology(data []byte, ontology any) []Problem {
	problems := ValidateBytes(data)
	for _, p := range ValidateOntology(ontology) {
		p.Path = "ontology:" + p.Path
		problems = append(problems, p)
	}
	env, err := Decode(bytes.NewReader(data))
	if err == nil {
		problems = append(problems, CheckActionRuns(env, ontology)...)
	}
	return finish(problems)
}

func mapAt(v any, key string) map[string]any {
	m, _ := v.(map[string]any)
	out, _ := m[key].(map[string]any)
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
