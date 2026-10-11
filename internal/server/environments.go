package server

import (
	"net/http"
	"strings"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// The environments graph (docs/design.md 46): the environment.* nodes of
// the mounted layers as their layers emit them, every edge of a mounted
// layer touching one, and the node one hop away. Portal serves its own
// Kinds for the ends it owns, a Principal as listPrincipals has it and a
// Run as listAgents has it (its layer and ledger count, which the page's
// Runs and Ledger links need), and has-run between the two when the
// principal holds the run; any other end as its layer emits it. The
// envelope names no ontology: its nodes keep their layers' Kinds.

func (s *Server) GetEnvironmentGraph(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	g := s.p.EnvironmentGraph()
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Nodes = append(env.Nodes, g.Nodes...)
	served, holder := s.environmentEnds(stamp, g)
	for _, n := range g.Nodes {
		if _, ok := served[n.ID]; !ok {
			served[n.ID] = n
		}
	}
	for _, id := range g.Ends {
		if n, ok := served[id]; ok {
			env.Nodes = append(env.Nodes, n)
		}
	}
	for _, e := range g.Edges {
		// An end no one serves would dangle: the edge is left out.
		_, from := served[e.From]
		_, to := served[e.To]
		if !from || !to {
			continue
		}
		env.Edges = append(env.Edges, e)
	}
	// A served Run's principal, when it is served too, holds it: has-run,
	// as listAgents joins them. It adds no node, only what the page's
	// links for that principal need.
	for _, id := range g.Ends {
		if p, ok := holder[id]; ok {
			if _, ok := served[p]; ok {
				env.Edges = append(env.Edges, graph.NewEdge(p, id, "has-run"))
			}
		}
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// environmentEnds resolves the graph's other ends: Portal's Run for an
// agent run, Portal's Principal for a principal (dev-bootstrap made up
// when principals.yaml lacks it, as the agents page does), else the node
// as a mounted layer holds it. An end none of these has is absent. holder
// is each Run's principal, as listAgents groups them.
func (s *Server) environmentEnds(stamp string, g store.EnvironmentGraph) (out map[string]graph.Node, holder map[string]string) {
	out = map[string]graph.Node{}
	holder = map[string]string{}
	if len(g.Ends) == 0 {
		return out, holder
	}
	runs := map[string]graph.Node{}
	for by, list := range s.agentSet(stamp).runs {
		for _, r := range list {
			runs[r.Run.ID] = r.Run
			holder[r.Run.ID] = by
		}
	}
	principals := map[string]graph.Node{}
	for _, n := range s.t.PrincipalsEnvelope(stamp).Nodes {
		principals[n.ID] = n
	}
	devID := tenant.PrincipalPrefix + tenant.DevPrincipalName
	if _, ok := principals[devID]; !ok {
		principals[devID] = graph.Node{ID: devID, Kind: "Principal", Name: tenant.DevPrincipalName,
			Properties: map[string]any{"type": "user", "role": tenant.RoleAdmin}}
	}
	for _, id := range g.Ends {
		if n, ok := runs[id]; ok {
			out[id] = n
		} else if n, ok := principals[id]; ok && strings.HasPrefix(id, tenant.PrincipalPrefix) {
			out[id] = n
		} else if n, ok := g.Held[id]; ok {
			out[id] = n
		}
	}
	return out, holder
}
