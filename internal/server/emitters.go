package server

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// An emitter is a card of type emitter in one of the tenant's feeds
// (docs/design.md 44), served as the Card node listFeeds serves under the
// card's own id. Every mounted layer was produced by the emitter its
// ontology's module names; one no feed holds a card of is still served, as
// an Emitter node ui:emitter:<name> flagged unknown.

// emitter is one emitter: its node and the layers it produced, newest first.
type emitter struct {
	node   graph.Node
	layers []*store.Layer
}

// emitters is the tenant's emitters by name: every card of type emitter (the
// newest push when several cards name one emitter) and every emitter a
// mounted layer names.
func (s *Server) emitters(stamp string) []emitter {
	byName := map[string]*emitter{}
	for _, n := range s.t.FeedsEnvelope(stamp).Nodes {
		if n.Kind != "Card" || n.Properties["type"] != tenant.EmitterType {
			continue
		}
		name := tenant.EmitterName(n.Name)
		if prev, ok := byName[name]; ok && prev.node.Properties["pushed"].(string) > n.Properties["pushed"].(string) {
			continue
		}
		props := map[string]any{"emitter": name}
		for k, v := range n.Properties {
			props[k] = v
		}
		n.Properties = props
		byName[name] = &emitter{node: n}
	}
	for _, l := range s.p.Layers() {
		name := l.Emitter()
		e, ok := byName[name]
		if !ok {
			e = &emitter{node: graph.Node{ID: tenant.EmitterPrefix + name, Kind: "Emitter", Name: name, Properties: map[string]any{"unknown": true}}}
			byName[name] = e
		}
		e.layers = append(e.layers, l)
	}
	out := make([]emitter, 0, len(byName))
	for _, e := range byName {
		sort.SliceStable(e.layers, func(i, j int) bool {
			if a, b := e.layers[i].Env.Layer, e.layers[j].Env.Layer; a != b {
				return a > b
			}
			return e.layers[i].Name < e.layers[j].Name
		})
		e.node.Properties["layers"] = len(e.layers)
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name() < out[j].name() })
	return out
}

func (e emitter) name() string {
	if n, ok := e.node.Properties["emitter"].(string); ok {
		return n
	}
	return e.node.Name
}

func findEmitter(es []emitter, name string) (emitter, bool) {
	for _, e := range es {
		if e.name() == name {
			return e, true
		}
	}
	return emitter{}, false
}

// layerNode is a mounted layer as a Layer node.
func layerNode(l *store.Layer) graph.Node {
	c := recount(l.Env)
	props := map[string]any{"emitter": l.Emitter(), "module": l.Env.Module, "stamp": l.Env.Layer, "nodes": c.Nodes, "edges": c.Edges}
	for k, v := range map[string]string{"version": l.Env.Version, "mountedBy": l.MountedBy, "path": l.Path} {
		if v != "" {
			props[k] = v
		}
	}
	return graph.Node{ID: store.LayerID(l.Name), Kind: "Layer", Name: l.Name, Properties: props}
}

// add puts the emitter, its layers and both edges between them into env.
func (e emitter) add(env *graph.Envelope) {
	env.Nodes = append(env.Nodes, e.node)
	for _, l := range e.layers {
		ln := layerNode(l)
		env.Nodes = append(env.Nodes, ln)
		env.Edges = append(env.Edges, graph.NewEdge(ln.ID, e.node.ID, "produced-by"), graph.NewEdge(e.node.ID, ln.ID, "has-layer"))
	}
}

func (s *Server) ListEmitters(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	env := servedEnvelope(stamp)
	for _, e := range s.emitters(stamp) {
		e.add(env)
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) GetEmitter(w http.ResponseWriter, r *http.Request, name api.Tenant, emitterName string) {
	if !s.tenantOK(w, name) {
		return
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	e, ok := findEmitter(s.emitters(stamp), emitterName)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no emitter %s in tenant %s: an emitter is a card of type emitter in one of its feeds, or the module a mounted layer's ontology names", emitterName, name), nil)
		return
	}
	env := servedEnvelope(stamp)
	env.Root = e.node.ID
	e.add(env)
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// GetEmittersPage serves index.html at /<tenant>/emitters; the shell draws it
// under the Page whose view is emitters (Emitters/Catalogue).
func (s *Server) GetEmittersPage(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		s.index(w)
	}
}

// GetEmitterPage serves index.html at one emitter's page,
// /<tenant>/emitters/<name>, and at each page of the emitters service, which
// keeps its path: the literal route wins over getShellPage's, and boot and
// push refuse an emitter card named like such a page.
func (s *Server) GetEmitterPage(w http.ResponseWriter, r *http.Request, name api.Tenant, emitterName string) {
	if !s.tenantOK(w, name) {
		return
	}
	if i, ok := s.t.Menu.NodeIndex()[tenant.RailID(tenant.EmittersService+"/"+emitterName)]; ok && s.t.Menu.Nodes[i].Kind == "Page" {
		s.index(w)
		return
	}
	if _, ok := findEmitter(s.emitters(""), emitterName); !ok {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no emitter %s in tenant %s, and no page %s/%s in its menu", emitterName, name, tenant.EmittersService, emitterName), nil)
		return
	}
	s.index(w)
}
