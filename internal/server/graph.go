package server

import (
	"net/http"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
)

// The tenant graph (docs/design.md 53): every node and edge of the mounted
// layers, or, with source, of the layers whose source block names that
// emitter. The filter is the URL's, never the page's state, so a filtered
// graph is a link: /<tenant>/graph?source=Emitter.Terraform. The envelope
// names no ontology: its nodes keep their layers' Kinds.

func (s *Server) GetTenantGraph(w http.ResponseWriter, r *http.Request, name api.Tenant, params api.GetTenantGraphParams) {
	if !s.tenantOK(w, name) {
		return
	}
	env := graph.NewEnvelope(portal.Module, portal.Version, time.Now().UTC().Format(store.StampLayout))
	nodes, edges := s.p.TenantGraph(params.Source)
	env.Nodes = append(env.Nodes, nodes...)
	env.Edges = append(env.Edges, edges...)
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// GetGraphPage serves index.html at /<tenant>/graph; the shell draws it
// under the Page whose view is graph (Graph/Nodes) and reads source itself.
func (s *Server) GetGraphPage(w http.ResponseWriter, r *http.Request, name api.Tenant, _ api.GetGraphPageParams) {
	if s.tenantOK(w, name) {
		s.index(w)
	}
}
