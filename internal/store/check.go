package store

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// IDPrefix starts every screen node id.
const IDPrefix = "ui:"

// displayKeys are display properties. Graph.Node keeps display out of nodes
// (a top-level display key is a schema problem); the portal also keeps these
// out of properties on every Kind but the display Kinds (docs/design.md 5).
var displayKeys = map[string]bool{
	"display": true, "color": true, "shape": true, "icon": true,
	"collapsed": true, "selected": true, "scroll": true,
	"opacity": true, "font": true,
}

// displayKinds may carry display properties: State per node, Theme per
// Container (docs/design.md 5 and 22).
var displayKinds = map[string]bool{"State": true, "Theme": true}

// hexColor is Graph.Node's display colour, #rrggbb.
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// nodeFields are the top-level keys a screen node may carry. display is left
// to Graph.Node, which reports it as schema.
var nodeFields = map[string]bool{
	"id": true, "kind": true, "name": true, "properties": true,
	"source": true, "findings": true, "stamp": true, "display": true,
}

var layoutModes = []string{"grid", "flow", "canvas"}

// borrowedIDs are the Kinds whose nodes keep the id another module gave
// them, by that module's prefix: a Run and a Decision are Graph.Emitter.Agent's
// Run and PolicyDecision re-served (docs/design.md 43). Only in an envelope
// the API serves; a screen node is always ui:.
var borrowedIDs = map[string]string{"Run": "agent:", "Decision": "agent:"}

// touched names the elements a write added or changed. The portal checks
// only those; Graph.Node checks the whole next screen.
type touched struct {
	nodes map[string]bool
	edges map[string]bool
}

func touch(nodes, edges []string) touched {
	t := touched{nodes: map[string]bool{}, edges: map[string]bool{}}
	for _, n := range nodes {
		t.nodes[n] = true
	}
	for _, e := range edges {
		t.edges[e] = true
	}
	return t
}

// check validates a candidate screen: Graph.Node on the whole envelope (raw
// when given, so keys the Go types would drop are still seen), then the
// portal ontology on what the write touched. Only errors come back.
func (p *Portal) check(next *graph.Envelope, raw []byte, t touched) []graph.Problem {
	next.Recount()
	var problems []graph.Problem
	if raw != nil {
		problems = errorsOnly(graph.ValidateBytes(raw))
	} else {
		problems = errorsOnly(graph.Validate(next))
	}
	idx := next.NodeIndex()
	for i, n := range next.Nodes {
		if t.nodes[n.ID] {
			problems = append(problems, p.checkNode(next, fmt.Sprintf("/nodes/%d", i), n, false)...)
		}
	}
	for i, e := range next.Edges {
		if t.edges[e.ID] {
			problems = append(problems, p.checkEdge(next, idx, fmt.Sprintf("/edges/%d", i), e)...)
		}
	}
	return problems
}

// errorsOnly keeps the problems that refuse a write. Graph.Node 0.2.0 reports
// a dangling edge whose missing end carries another owner's prefix as a
// warning, resolved at merge; the screen is never merged with a data layer,
// so the portal keeps every dangling edge an error (docs/design.md 9). Other
// warnings are dropped.
func errorsOnly(ps []graph.Problem) []graph.Problem {
	var out []graph.Problem
	for _, p := range ps {
		if p.Code == "dangling-edge" {
			p.Severity = graph.SeverityError
		}
		if p.Severity != graph.SeverityWarning {
			out = append(out, p)
		}
	}
	return out
}

// checkNode checks one node against the portal ontology; served is an
// envelope the API serves (CheckEnvelope), where a borrowed Kind keeps its
// owner's id.
func (p *Portal) checkNode(env *graph.Envelope, at string, n graph.Node, served bool) []graph.Problem {
	var out []graph.Problem
	add := func(code, path, msg string) {
		out = append(out, graph.Problem{Code: code, Path: at + path, Message: msg, Severity: graph.SeverityError})
	}
	if prefix, ok := borrowedIDs[n.Kind]; ok && served {
		if err := graph.ValidID(prefix, n.ID); err != nil || !strings.HasPrefix(n.ID, prefix) {
			add("id-prefix", "/id", fmt.Sprintf("%s id %q must be the %s id it has in its layer, %s...", n.Kind, n.ID, prefix, prefix))
		}
	} else if !strings.HasPrefix(n.ID, IDPrefix) {
		add("id-prefix", "/id", fmt.Sprintf("screen node id %q must start with %q", n.ID, IDPrefix))
	}
	def, ok := p.ont.Kinds[n.Kind]
	if !ok {
		add("unknown-kind", "/kind", fmt.Sprintf("Kind %q is not in the portal ontology (%s)", n.Kind, strings.Join(sortedKeys(p.ont.Kinds), ", ")))
		return out
	}
	for _, name := range sortedKeys(n.Properties) {
		v := n.Properties[name]
		path := "/properties/" + name
		if displayKeys[name] && !displayKinds[n.Kind] {
			add("display", path, fmt.Sprintf("%q is a display property; display lives in a State node (has-state) or a Theme (themed-by), never on a %s", name, n.Kind))
			continue
		}
		pd, ok := def.Properties[name]
		if !ok {
			add("unknown-property", path, fmt.Sprintf("%s declares no property %q (%s)", n.Kind, name, strings.Join(sortedKeys(def.Properties), ", ")))
			continue
		}
		if msg := typeProblem(pd, v); msg != "" {
			add("property-type", path, msg)
		}
	}
	for _, name := range sortedKeys(def.Properties) {
		if def.Properties[name].Required {
			if _, ok := n.Properties[name]; !ok {
				add("property-type", "/properties", fmt.Sprintf("%s requires property %q", n.Kind, name))
			}
		}
	}
	switch n.Kind {
	case "Widget":
		if t, ok := n.Properties["type"].(string); ok && categoryByName(env, t) == "" {
			add("unknown-category", "/properties/type", fmt.Sprintf("no Category named %q on screen", t))
		}
	case MountKind:
		if layer, ok := n.Properties["layer"].(string); ok {
			if !layerName.MatchString(layer) {
				add("property-type", "/properties/layer", fmt.Sprintf("layer %q must match %s", layer, layerName))
			} else if n.ID != MountID(layer) {
				add("property-type", "/id", fmt.Sprintf("a Mount's id is %s, from its layer", MountID(layer)))
			}
		}
	case "Ref":
		layer, _ := n.Properties["layer"].(string)
		node, _ := n.Properties["node"].(string)
		if layer != "" && node != "" && !p.layerHas(layer, node) {
			add("unknown-ref", "/properties", fmt.Sprintf("no node %q in a mounted layer %q", node, layer))
		}
	}
	return out
}

func (p *Portal) checkEdge(env *graph.Envelope, idx map[string]int, at string, e graph.Edge) []graph.Problem {
	var out []graph.Problem
	add := func(code, path, msg string) {
		out = append(out, graph.Problem{Code: code, Path: at + path, Message: msg, Severity: graph.SeverityError})
	}
	def, ok := p.ont.EdgeKinds[e.Kind]
	if !ok {
		add("unknown-edge-kind", "/kind", fmt.Sprintf("edge Kind %q is not in the portal ontology (%s)", e.Kind, strings.Join(sortedKeys(p.ont.EdgeKinds), ", ")))
		return out
	}
	for name := range e.Properties {
		if displayKeys[name] {
			add("display", "/properties/"+name, fmt.Sprintf("%q is a display property; edges carry none", name))
		}
	}
	kindOf := func(id string) (string, bool) {
		i, ok := idx[id]
		if !ok {
			return "", false
		}
		return env.Nodes[i].Kind, true
	}
	if k, ok := kindOf(e.From); ok && !slices.Contains(def.From, k) {
		add("edge-endpoints", "/from", fmt.Sprintf("%s may start at %s, not %s", e.Kind, strings.Join(def.From, ", "), k))
	}
	k, ok := kindOf(e.To)
	if e.Kind == "is-a" && (!ok || k != "Category") {
		add("unknown-category", "/to", fmt.Sprintf("is-a must point at a Category node on screen; %q is not one", e.To))
		return out
	}
	if ok && !slices.Contains(def.To, k) {
		add("edge-endpoints", "/to", fmt.Sprintf("%s may end at %s, not %s", e.Kind, strings.Join(def.To, ", "), k))
	}
	return out
}

// typeProblem checks one value against its declared type, plus the shapes the
// ontology states in prose: Layout mode, bounds, scroll, and a Theme's color,
// opacity and font.
func typeProblem(pd PropDef, v any) string {
	if v == nil {
		return pd.Name + " is null; leave it out instead"
	}
	ok := true
	switch pd.Type {
	case "string":
		_, ok = v.(string)
	case "boolean":
		_, ok = v.(bool)
	case "integer":
		ok = isInteger(v)
	case "number":
		_, ok = number(v)
	case "object":
		_, ok = v.(map[string]any)
	case "array":
		_, ok = v.([]any)
	}
	if !ok {
		return fmt.Sprintf("%s must be %s", pd.Name, article(pd.Type))
	}
	switch pd.Name {
	case "mode":
		if s, _ := v.(string); !slices.Contains(layoutModes, s) {
			return fmt.Sprintf("mode must be one of %s", strings.Join(layoutModes, ", "))
		}
	case "bounds":
		m := v.(map[string]any)
		for _, k := range []string{"x", "y", "w", "h"} {
			if _, ok := number(m[k]); !ok {
				return "bounds must be {x, y, w, h}, all numbers"
			}
		}
		if len(m) != 4 {
			return "bounds holds x, y, w and h only"
		}
		if w, _ := number(m["w"]); w <= 0 {
			return "bounds.w must be above 0"
		}
		if h, _ := number(m["h"]); h <= 0 {
			return "bounds.h must be above 0"
		}
	case "color":
		if !hexColor.MatchString(v.(string)) {
			return "color must be #rrggbb"
		}
	case "opacity":
		if f, _ := number(v); f < 0 || f > 1 {
			return "opacity must be from 0 to 1"
		}
	case "font":
		if strings.TrimSpace(v.(string)) == "" {
			return "font must name a CSS font-family"
		}
	case "scroll":
		m := v.(map[string]any)
		for k := range m {
			if _, ok := number(m[k]); !ok || (k != "x" && k != "y") {
				return "scroll must be {x, y}, numbers"
			}
		}
	}
	return ""
}

func number(v any) (float64, bool) {
	switch t := v.(type) {
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	}
	return 0, false
}

func isInteger(v any) bool {
	if n, ok := v.(json.Number); ok {
		_, err := n.Int64()
		return err == nil
	}
	f, ok := number(v)
	return ok && f == float64(int64(f))
}

func article(t string) string {
	if t == "" {
		return "a value"
	}
	if strings.ContainsRune("aeiou", rune(t[0])) {
		return "an " + t
	}
	return "a " + t
}

// categoryByName returns the id of the Category node with that name, or "".
func categoryByName(env *graph.Envelope, name string) string {
	for _, n := range env.Nodes {
		if n.Kind == "Category" && n.Name == name {
			return n.ID
		}
	}
	return ""
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
