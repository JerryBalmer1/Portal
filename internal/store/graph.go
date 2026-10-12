package store

import "github.com/JerryBalmer1/Portal/internal/graphnode/graph"

// TenantGraph is every node and edge of the mounted layers (docs/design.md
// 53), read-only. With source not empty, only the layers whose Graph.Node
// source block names that emitter are read: source.emitter equal to source,
// exact and case-sensitive, as the roll call matches a layer to its card
// (docs/design.md 49); a layer with no source block never matches. A node id
// seen in two layers keeps its first in mount order, as NodesOfKind does, a
// node with no stamp gets stamp.layer from its layer, and an alert layer's
// copies of its subjects are left out (its alerts are kept). An edge is kept
// once, and only when both its ends are among the nodes kept.
func (p *Portal) TenantGraph(source string) (nodes []graph.Node, edges []graph.Edge) {
	var layers []*Layer
	for _, l := range p.Layers() {
		if source == "" || (l.Env.Source != nil && l.Env.Source.Emitter == source) {
			layers = append(layers, l)
		}
	}
	kept := map[string]bool{}
	for _, l := range layers {
		copies := IsAlertLayer(l.Env)
		for _, n := range l.Env.Nodes {
			if kept[n.ID] || (copies && n.Kind != AlertKind) {
				continue
			}
			kept[n.ID] = true
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: l.Env.Layer}
			}
			nodes = append(nodes, n)
		}
	}
	seen := map[string]bool{}
	for _, l := range layers {
		for _, e := range l.Env.Edges {
			if !kept[e.From] || !kept[e.To] {
				continue
			}
			id := e.ID
			if id == "" {
				id = graph.EdgeID(e.From, e.To, e.Kind)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			edges = append(edges, e)
		}
	}
	return nodes, edges
}
