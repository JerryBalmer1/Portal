package store

import (
	"fmt"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// NodesOfKind returns every node of one Kind on the screen (as GET
// /api/screen shows it, so production mode hides the Designer here too) and
// in each mounted layer, in that order, read-only. An id seen twice keeps
// its first. A node without a stamp gets stamp.layer from the envelope it
// came from, so a reader can say when it was last updated. The result holds
// nodes only: edges between envelopes would dangle.
func (p *Portal) NodesOfKind(kind string) (*graph.Envelope, error) {
	screen, err := p.Screen()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	layers := append([]*Layer(nil), p.layers...)
	stamp := p.screen.Layer
	p.mu.Unlock()
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	seen := map[string]bool{}
	add := func(from *graph.Envelope) {
		for _, n := range from.Nodes {
			if n.Kind != kind || seen[n.ID] {
				continue
			}
			seen[n.ID] = true
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: from.Layer}
			}
			env.Nodes = append(env.Nodes, n)
		}
	}
	add(screen)
	for _, l := range layers {
		add(l.Env)
	}
	env.Recount()
	return env, nil
}

// CheckEnvelope runs the portal ontology over every node and edge of an
// envelope that is not the screen (the tenant's menu), as a write checks
// what it touched. Graph.Node's own validation is the caller's.
func (p *Portal) CheckEnvelope(env *graph.Envelope) []graph.Problem {
	idx := env.NodeIndex()
	var out []graph.Problem
	for i, n := range env.Nodes {
		out = append(out, p.checkNode(env, pathOf("nodes", i), n, true)...)
	}
	for i, e := range env.Edges {
		out = append(out, p.checkEdge(env, idx, pathOf("edges", i), e)...)
	}
	return out
}

func pathOf(list string, i int) string { return fmt.Sprintf("/%s/%d", list, i) }
