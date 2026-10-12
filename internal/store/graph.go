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
// once, and only when both its ends are among the nodes kept. Since 0.16.0
// it is the query (docs/design.md 54) with every node a start, no kind, the
// source set, depth 0 and no cap.
func (p *Portal) TenantGraph(source string) (nodes []graph.Node, edges []graph.Edge) {
	r, _ := p.GraphQuery(Walk{Source: source, Depth: 0})
	return r.Nodes, r.Edges
}
