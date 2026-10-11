package server

import (
	"fmt"
	"net/http"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// An agent is a principal (docs/design.md 43): each principal of type
// agent, and each principal whose token mounted a layer holding a Run.
// There is no Agent Kind. The runs are the Run nodes of the mounted
// Graph.Emitter.Agent layers under their own agent: ids, which the portal
// never mints.

// agentSet is the tenant's agents, in principals.yaml order with
// dev-bootstrap last when the file lacks it, and each one's runs, newest
// first.
type agentSet struct {
	order []graph.Node
	runs  map[string][]store.AgentRun
}

func (s *Server) agentSet(stamp string) agentSet {
	devID := tenant.PrincipalPrefix + tenant.DevPrincipalName
	known := map[string]bool{}
	pe := s.t.PrincipalsEnvelope(stamp)
	for _, n := range pe.Nodes {
		known[n.ID] = true
	}
	set := agentSet{runs: map[string][]store.AgentRun{}}
	for _, r := range s.p.AgentRuns() {
		// No principal (a legacy token, a mount at start), or one
		// principals.yaml no longer holds: dev-bootstrap.
		by := r.MountedBy
		if !known[by] {
			by = devID
		}
		set.runs[by] = append(set.runs[by], r)
	}
	for _, n := range pe.Nodes {
		if n.Properties["type"] == "agent" || len(set.runs[n.ID]) > 0 {
			set.order = append(set.order, n)
		}
	}
	if !known[devID] && len(set.runs[devID]) > 0 {
		// The admin Invoke-Build Dev mints, before it has been minted.
		set.order = append(set.order, graph.Node{ID: devID, Kind: "Principal", Name: tenant.DevPrincipalName,
			Properties: map[string]any{"type": "user", "role": tenant.RoleAdmin}})
	}
	return set
}

func (a agentSet) find(name string) (graph.Node, bool) {
	for _, n := range a.order {
		if n.Name == name {
			return n, true
		}
	}
	return graph.Node{}, false
}

func agentsEnvelope(stamp string) *graph.Envelope {
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Ontology = "ontology.yaml"
	return env
}

func (s *Server) ListAgents(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	set := s.agentSet(stamp)
	env := agentsEnvelope(stamp)
	for _, p := range set.order {
		env.Nodes = append(env.Nodes, p)
		for _, run := range set.runs[p.ID] {
			env.Nodes = append(env.Nodes, run.Run)
			env.Edges = append(env.Edges, graph.NewEdge(p.ID, run.Run.ID, "has-run"))
		}
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) ListAgentRuns(w http.ResponseWriter, r *http.Request, name api.Tenant, agent string) {
	if !s.tenantOK(w, name) {
		return
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	set := s.agentSet(stamp)
	p, ok := set.find(agent)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no agent %s in tenant %s: an agent is a principal of type agent, or one whose token mounted a layer of %s runs", agent, name, store.AgentModule), nil)
		return
	}
	env := agentsEnvelope(stamp)
	env.Root = p.ID
	env.Nodes = append(env.Nodes, p)
	for _, run := range set.runs[p.ID] {
		env.Nodes = append(env.Nodes, run.Run)
		env.Edges = append(env.Edges, graph.NewEdge(p.ID, run.Run.ID, "has-run"))
		for _, d := range run.Decisions {
			env.Nodes = append(env.Nodes, d)
			env.Edges = append(env.Edges, graph.NewEdge(run.Run.ID, d.ID, "has-decision"))
		}
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// GetAgentsPage serves index.html at /<tenant>/agents; the shell draws it
// under the Page whose view is agents (Identity/Agents).
func (s *Server) GetAgentsPage(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		s.index(w)
	}
}

// GetAgentPage serves index.html at one agent's page, /<tenant>/agents/<name>.
func (s *Server) GetAgentPage(w http.ResponseWriter, r *http.Request, name api.Tenant, agent string) {
	if !s.tenantOK(w, name) {
		return
	}
	if _, ok := s.agentSet("").find(agent); !ok {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no agent %s in tenant %s", agent, name), nil)
		return
	}
	s.index(w)
}
