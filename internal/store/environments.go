package store

import (
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// EnvironmentKindPrefix starts every Kind the environments graph draws:
// environment.instance, environment.image, environment.network and any
// other a layer emits (docs/design.md 46).
const EnvironmentKindPrefix = "environment."

// EnvironmentGraph is what the mounted layers say about environments: every
// node of an environment.* Kind, every edge with one of them at an end, and,
// for each such edge, the node at its other end when a mounted layer holds
// it. Ends lists every other end, held or not, in the order the edges name
// them, so the caller can serve the ones Portal owns (a Principal) itself.
type EnvironmentGraph struct {
	Nodes []graph.Node
	Edges []graph.Edge
	// Held is the other ends a mounted layer holds, by id, as that layer
	// emits them; the first layer in mount order wins.
	Held map[string]graph.Node
	Ends []string
}

// EnvironmentGraph reads the mounted layers, read-only. A node id seen in
// two layers keeps its first, as NodesOfKind does, and a node with no stamp
// gets stamp.layer from its layer. An edge id seen twice is kept once.
func (p *Portal) EnvironmentGraph() EnvironmentGraph {
	layers := p.Layers()
	g := EnvironmentGraph{Held: map[string]graph.Node{}}
	env := map[string]bool{}
	for _, l := range layers {
		for _, n := range l.Env.Nodes {
			if !strings.HasPrefix(n.Kind, EnvironmentKindPrefix) || env[n.ID] {
				continue
			}
			env[n.ID] = true
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: l.Env.Layer}
			}
			g.Nodes = append(g.Nodes, n)
		}
	}
	seenEdge := map[string]bool{}
	seenEnd := map[string]bool{}
	for _, l := range layers {
		for _, e := range l.Env.Edges {
			if !env[e.From] && !env[e.To] {
				continue
			}
			id := e.ID
			if id == "" {
				id = graph.EdgeID(e.From, e.To, e.Kind)
			}
			if seenEdge[id] {
				continue
			}
			seenEdge[id] = true
			g.Edges = append(g.Edges, e)
			for _, end := range []string{e.From, e.To} {
				if !env[end] && !seenEnd[end] {
					seenEnd[end] = true
					g.Ends = append(g.Ends, end)
				}
			}
		}
	}
	want := map[string]bool{}
	for _, id := range g.Ends {
		want[id] = true
	}
	for _, l := range layers {
		for _, n := range l.Env.Nodes {
			if _, ok := g.Held[n.ID]; ok || !want[n.ID] {
				continue
			}
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: l.Env.Layer}
			}
			g.Held[n.ID] = n
		}
	}
	return g
}
