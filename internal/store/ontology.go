package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"go.yaml.in/yaml/v3"
)

// PropDef is one declared property of a Kind.
type PropDef struct {
	Name     string
	Type     string
	Required bool
}

// KindDef is what the portal checks about a Kind.
type KindDef struct {
	Properties map[string]PropDef
}

// EdgeKindDef is what the portal checks about an edge Kind.
type EdgeKindDef struct {
	From, To []string
}

// Ontology is a decoded ontology.yaml: the raw document for GET /api/ontology
// and the parts the portal's checks read.
type Ontology struct {
	Raw       map[string]any
	Kinds     map[string]KindDef
	EdgeKinds map[string]EdgeKindDef
}

// DecodeYAML reads an ontology document and returns it as JSON-shaped data
// (map[string]any all the way down), the shape graph.ValidateOntology wants.
func DecodeYAML(data []byte) (map[string]any, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// A JSON round trip turns every map into map[string]any and every
	// number into float64, as the schema validator expects.
	js, err := json.Marshal(normaliseYAML(raw))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(js, &doc); err != nil {
		return nil, fmt.Errorf("ontology is not a mapping: %w", err)
	}
	return doc, nil
}

func normaliseYAML(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = normaliseYAML(val)
		}
		return t
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, val := range t {
			m[fmt.Sprint(k)] = normaliseYAML(val)
		}
		return m
	case []any:
		for i, val := range t {
			t[i] = normaliseYAML(val)
		}
		return t
	default:
		return v
	}
}

// ParseOntology validates an ontology document with Graph.Node and reads the
// Kinds and edge Kinds out of it.
func ParseOntology(data []byte) (*Ontology, []graph.Problem, error) {
	doc, err := DecodeYAML(data)
	if err != nil {
		return nil, nil, err
	}
	if ps := graph.ValidateOntology(doc); graph.HasErrors(ps) {
		return nil, ps, nil
	}
	o := &Ontology{Raw: doc, Kinds: map[string]KindDef{}, EdgeKinds: map[string]EdgeKindDef{}}
	kinds, _ := doc["kinds"].(map[string]any)
	for name, raw := range kinds {
		def := KindDef{Properties: map[string]PropDef{}}
		k, _ := raw.(map[string]any)
		props, _ := k["properties"].([]any)
		for _, p := range props {
			switch t := p.(type) {
			case string:
				def.Properties[t] = PropDef{Name: t}
			case map[string]any:
				pd := PropDef{}
				pd.Name, _ = t["name"].(string)
				pd.Type, _ = t["type"].(string)
				pd.Required, _ = t["required"].(bool)
				def.Properties[pd.Name] = pd
			}
		}
		o.Kinds[name] = def
	}
	edges, _ := doc["edgeKinds"].(map[string]any)
	for name, raw := range edges {
		e, _ := raw.(map[string]any)
		o.EdgeKinds[name] = EdgeKindDef{From: stringList(e["from"]), To: stringList(e["to"])}
	}
	return o, nil, nil
}

func stringList(v any) []string {
	list, _ := v.([]any)
	out := make([]string, 0, len(list))
	for _, s := range list {
		if str, ok := s.(string); ok {
			out = append(out, str)
		}
	}
	return out
}

// loadLayerOntology resolves envelope.ontology relative to the envelope file
// (Graph.Node design 8) and reads it. A URL is not fetched.
func loadLayerOntology(envPath, ref string) (map[string]any, []graph.Problem) {
	if ref == "" {
		return nil, nil
	}
	if strings.Contains(ref, "://") {
		return nil, []graph.Problem{{Code: "ontology", Path: "/ontology", Message: "a URL ontology is not fetched: " + ref, Severity: graph.SeverityError}}
	}
	if envPath == "" {
		return nil, []graph.Problem{{Code: "ontology", Path: "/ontology", Message: "a relative ontology cannot be resolved for an envelope mounted from the body: " + ref, Severity: graph.SeverityError}}
	}
	p := ref
	if !filepath.IsAbs(p) {
		p = filepath.Join(filepath.Dir(envPath), filepath.FromSlash(ref))
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, []graph.Problem{{Code: "ontology", Path: "/ontology", Message: err.Error(), Severity: graph.SeverityError}}
	}
	doc, err := DecodeYAML(data)
	if err != nil {
		return nil, []graph.Problem{{Code: "ontology", Path: "/ontology", Message: p + ": " + err.Error(), Severity: graph.SeverityError}}
	}
	if ps := graph.ValidateOntology(doc); graph.HasErrors(ps) {
		return nil, ps
	}
	return doc, nil
}
