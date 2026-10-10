package tenant

import (
	"strings"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Menu ids (ontology.yaml Service and Page): ui:rail:<service>, then
// :<page> per level. Icons are display, so they live in the State
// ui:state:rail:<...> of the entry, joined by has-state (docs/design.md 5).
const railPrefix = "ui:rail:"

// RailID is the id of the menu node at path (overview/summary ->
// ui:rail:overview:summary).
func RailID(path string) string { return railPrefix + strings.ReplaceAll(path, "/", ":") }

// BuildMenu turns the merged config's menu tree into an envelope: one
// Service per top-level entry, one Page per entry below it, has-page from
// parent to child, and a State with icon for every entry that names one.
func BuildMenu(config map[string]any, stamp string) (*graph.Envelope, error) {
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Ontology = "ontology.yaml"
	menu, _ := config["menu"].([]any)
	var walk func(items []any, parent string, depth int)
	walk = func(items []any, parent string, depth int) {
		for i, raw := range items {
			item := raw.(map[string]any)
			path, _ := item["path"].(string)
			label, _ := item["label"].(string)
			id := RailID(path)
			n := graph.Node{ID: id, Kind: "Service", Name: label, Properties: map[string]any{"path": path, "order": i}}
			if depth > 0 {
				n.Kind = "Page"
				for from, to := range map[string]string{"kind": "nodeKind", "view": "view", "screen": "screen", "sections": "sections", "filter": "filter"} {
					if v, ok := item[from]; ok {
						n.Properties[to] = v
					}
				}
			} else if dock, ok := item["dock"].(string); ok {
				n.Properties["dock"] = dock
			}
			env.Nodes = append(env.Nodes, n)
			if parent != "" {
				env.Edges = append(env.Edges, graph.NewEdge(parent, id, "has-page"))
			}
			if icon, ok := item["icon"].(string); ok {
				sid := "ui:state:" + strings.TrimPrefix(id, "ui:")
				env.Nodes = append(env.Nodes, graph.Node{ID: sid, Kind: "State", Name: "icon of " + label, Properties: map[string]any{"icon": icon}})
				env.Edges = append(env.Edges, graph.NewEdge(id, sid, "has-state"))
			}
			kids, _ := item["children"].([]any)
			walk(kids, id, depth+1)
		}
	}
	walk(menu, "", 0)
	env.Recount()
	return env, nil
}
