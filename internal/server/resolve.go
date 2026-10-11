package server

import (
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// resolver turns the id a layer's edge names into the node Portal serves
// for it, one hop away: the environments graph's other ends (docs/design.md
// 46) and an alert's subject and Run (docs/design.md 47). A Run is served
// as listAgents has it (its layer, ledger and alerts counts), a Decision as
// listAgentRuns has it, an agent ledger entry as getLedgerEntry has it, a
// Principal as listPrincipals has it (dev-bootstrap made up when
// principals.yaml lacks it, as the agents page does), and any other node
// as the first mounted layer holding it emits it. An alert layer's copies
// are never served (store.IsAlertLayer).
type resolver struct {
	s          *Server
	runs       map[string]graph.Node
	decisions  map[string]graph.Node
	principals map[string]graph.Node
	// holder is each Run's principal, as listAgents groups them.
	holder  map[string]string
	entries map[string]graph.Node
}

func (s *Server) resolver(stamp string) *resolver {
	r := &resolver{s: s, runs: map[string]graph.Node{}, decisions: map[string]graph.Node{}, principals: map[string]graph.Node{}, holder: map[string]string{}}
	for by, list := range s.agentSet(stamp).runs {
		for _, run := range list {
			r.runs[run.Run.ID] = run.Run
			r.holder[run.Run.ID] = by
			for _, d := range run.Decisions {
				r.decisions[d.ID] = d
			}
		}
	}
	for _, n := range s.t.PrincipalsEnvelope(stamp).Nodes {
		r.principals[n.ID] = n
	}
	devID := tenant.PrincipalPrefix + tenant.DevPrincipalName
	if _, ok := r.principals[devID]; !ok {
		r.principals[devID] = graph.Node{ID: devID, Kind: "Principal", Name: tenant.DevPrincipalName,
			Properties: map[string]any{"type": "user", "role": tenant.RoleAdmin}}
	}
	return r
}

// entry is the agent ledger's entry of id, read on the first ask.
func (r *resolver) entry(id string) (graph.Node, bool) {
	if r.entries == nil {
		r.entries = map[string]graph.Node{}
		if c, ok := r.s.chain(tenant.SourceAgent); ok {
			runLayer := r.s.runLayers()
			for _, e := range c.Entries {
				r.entries[e.ID] = entryNode(c, e, runLayer)
			}
		}
	}
	n, ok := r.entries[id]
	return n, ok
}

// resolve serves each of ids that Portal can; an id it cannot is absent.
func (r *resolver) resolve(ids []string) map[string]graph.Node {
	out := map[string]graph.Node{}
	rest := map[string]bool{}
	for _, id := range ids {
		if n, ok := r.runs[id]; ok {
			out[id] = n
		} else if n, ok := r.decisions[id]; ok {
			out[id] = n
		} else if n, ok := r.principals[id]; ok && strings.HasPrefix(id, tenant.PrincipalPrefix) {
			out[id] = n
		} else if strings.HasPrefix(id, store.AgentLedgerPrefix) {
			if n, ok := r.entry(id); ok {
				out[id] = n
			}
		} else {
			rest[id] = true
		}
	}
	for id, n := range r.s.p.HeldNodes(rest) {
		out[id] = n
	}
	return out
}
