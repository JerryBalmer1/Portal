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
// (docs/design.md 44), named by the card and served as the Card node
// listFeeds serves under the card's own id. A mounted layer belongs to the
// emitter its Graph.Node source block names, source.emitter equal to the
// card's name, and to no other: nothing maps one name onto another
// (docs/design.md 49). The roll call has one row per name a card or a
// layer's source block gives, with a status; a layer with no source block
// is listed under its envelope's module, unstamped, never matched to a card.

// Statuses of an emitter in the roll call.
const (
	// statusOK: a card, and every layer naming it carries its version.
	statusOK = "ok"
	// statusVersionMismatch: a card, and a layer naming it carries another
	// version in its source block.
	statusVersionMismatch = "version-mismatch"
	// statusNoLayers: a card no mounted layer names.
	statusNoLayers = "no-layers"
	// statusNoCard: a name layers' source blocks give that no feed holds a
	// card of.
	statusNoCard = "no-card"
	// statusUnstamped: the module of layers with no source block.
	statusUnstamped = "unstamped"
)

// emitter is one row of the roll call: its node, its name and the layers it
// produced, newest first.
type emitter struct {
	node      graph.Node
	name      string
	unstamped bool
	layers    []*store.Layer
}

// emitters is the tenant's roll call by name: every card of type emitter
// (the newest push when several cards share a name), every name a mounted
// layer's source block gives, and the module of every unstamped layer, an
// unstamped module after an emitter of the same name.
func (s *Server) emitters(stamp string) []emitter {
	named := map[string]*emitter{}
	for _, n := range s.t.FeedsEnvelope(stamp).Nodes {
		if n.Kind != "Card" || n.Properties["type"] != tenant.EmitterType {
			continue
		}
		if prev, ok := named[n.Name]; ok && prev.node.Properties["pushed"].(string) > n.Properties["pushed"].(string) {
			continue
		}
		props := map[string]any{"emitter": n.Name}
		for k, v := range n.Properties {
			props[k] = v
		}
		n.Properties = props
		named[n.Name] = &emitter{node: n, name: n.Name}
	}
	modules := map[string]*emitter{}
	for _, l := range s.p.Layers() {
		name, stamped := l.Emitter()
		in, id := named, tenant.EmitterPrefix+name
		if !stamped {
			in, id = modules, tenant.ModulePrefix+name
		}
		e, ok := in[name]
		if !ok {
			e = &emitter{node: graph.Node{ID: id, Kind: "Emitter", Name: name, Properties: map[string]any{}}, name: name, unstamped: !stamped}
			in[name] = e
		}
		e.layers = append(e.layers, l)
	}
	out := make([]emitter, 0, len(named)+len(modules))
	for _, in := range []map[string]*emitter{named, modules} {
		for _, e := range in {
			sort.SliceStable(e.layers, func(i, j int) bool {
				if a, b := e.layers[i].Env.Layer, e.layers[j].Env.Layer; a != b {
					return a > b
				}
				return e.layers[i].Name < e.layers[j].Name
			})
			e.node.Properties["layers"] = len(e.layers)
			e.node.Properties["status"] = e.status()
			if vs := e.layerVersions(); len(vs) > 0 {
				e.node.Properties["layerVersions"] = vs
			}
			out = append(out, *e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].name != out[j].name {
			return out[i].name < out[j].name
		}
		return !out[i].unstamped
	})
	return out
}

// status is the emitter's status in the roll call.
func (e emitter) status() string {
	switch {
	case e.unstamped:
		return statusUnstamped
	case e.node.Kind != "Card":
		return statusNoCard
	case len(e.layers) == 0:
		return statusNoLayers
	}
	for _, l := range e.layers {
		if l.Env.Source.Version != e.node.Properties["version"] {
			return statusVersionMismatch
		}
	}
	return statusOK
}

// layerVersions is the source.version of the emitter's layers, distinct and
// sorted; none for an unstamped module.
func (e emitter) layerVersions() []any {
	if e.unstamped {
		return nil
	}
	seen := map[string]bool{}
	var vs []string
	for _, l := range e.layers {
		if v := l.Env.Source.Version; !seen[v] {
			seen[v] = true
			vs = append(vs, v)
		}
	}
	sort.Strings(vs)
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}

// findEmitter is the emitter named name: a card's or a source block's name
// first, then an unstamped module.
func findEmitter(es []emitter, name string) (emitter, bool) {
	for _, e := range es {
		if e.name == name {
			return e, true
		}
	}
	return emitter{}, false
}

// layerNode is a mounted layer as a Layer node, with its source block when
// it has one.
func layerNode(l *store.Layer) graph.Node {
	c := recount(l.Env)
	name, _ := l.Emitter()
	props := map[string]any{"emitter": name, "module": l.Env.Module, "stamp": l.Env.Layer, "nodes": c.Nodes, "edges": c.Edges}
	for k, v := range map[string]string{"version": l.Env.Version, "mountedBy": l.MountedBy, "path": l.Path} {
		if v != "" {
			props[k] = v
		}
	}
	if src := l.Env.Source; src != nil && src.Emitter != "" {
		props["source"] = map[string]any{"emitter": src.Emitter, "version": src.Version, "target": src.Target, "stamp": src.Stamp}
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
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no emitter %s in tenant %s: an emitter is a card of type emitter in one of its feeds, a name a mounted layer's source block gives, or the module of a mounted layer with no source block", emitterName, name), nil)
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
