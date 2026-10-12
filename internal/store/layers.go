package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Layer is one mounted data layer. It is never written: not in memory after
// mounting, and never on disk (docs/design.md 2).
type Layer struct {
	Name             string
	Path             string
	Env              *graph.Envelope
	Ontology         map[string]any
	OntologyProblems []graph.Problem
	// MountedBy is the id of the principal whose token mounted the layer,
	// "" when none did: a token from before 0.5.0, or a mount at start
	// (docs/design.md 43).
	MountedBy string
}

var layerName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Mount is the body of POST /api/tenants/{tenant}/layers, and By the
// principal whose token sent it, which the body cannot set.
type Mount struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Envelope json.RawMessage `json:"envelope"`
	By       string          `json:"-"`
}

// MountKind is the screen Kind that records a layer mounted by path through
// POST /api/tenants/{tenant}/layers, so an approved snap mounts it again at
// boot (docs/design.md 33), under the principal that mounted it (43).
const MountKind = "Mount"

// MountID is the id of the Mount node recording layer name.
func MountID(name string) string { return "ui:mount:" + name }

// MountLayer validates and mounts one layer, replacing a layer of the same
// name. An envelope that fails Graph.Node validation is refused with its
// problems; an ontology that cannot be read or fails is reported on the
// layer, which then draws grey. A mount by path records itself on the screen
// as the Mount node ui:mount:<name> with the path as given (a directory
// stays a directory) and, as mountedBy, the principal that sent it; a mount
// by envelope removes that node, since an inline envelope cannot be read
// again.
func (p *Portal) MountLayer(m Mount) (*Layer, error) { return p.mount(m, true) }

// MountAtStart mounts one layer without touching the screen: cmd/portal's
// -mount and -filesystem flags, which mount again on every start, and the
// screen's own Mount nodes at boot (Remount).
func (p *Portal) MountAtStart(m Mount) (*Layer, error) { return p.mount(m, false) }

// Remount mounts the layer each Mount node on the screen records, in screen
// order and under the principal it records, without touching the screen. A layer that cannot be mounted (its
// file gone, its envelope invalid) is reported in failed and left unmounted;
// it never stops the boot, and its Mount node stays.
func (p *Portal) Remount() (mounted []*Layer, failed []error) {
	p.mu.Lock()
	var ms []Mount
	for _, n := range p.screen.Nodes {
		if n.Kind == MountKind {
			layer, _ := n.Properties["layer"].(string)
			path, _ := n.Properties["path"].(string)
			by, _ := n.Properties["mountedBy"].(string)
			ms = append(ms, Mount{Name: layer, Path: path, By: by})
		}
	}
	p.mu.Unlock()
	for _, m := range ms {
		l, err := p.MountAtStart(m)
		if err != nil {
			failed = append(failed, fmt.Errorf("%s: %w", MountID(m.Name), err))
			continue
		}
		mounted = append(mounted, l)
	}
	return mounted, failed
}

func (p *Portal) mount(m Mount, record bool) (*Layer, error) {
	if d := p.derivedName(); d != "" && m.Name == d {
		return nil, fmt.Errorf("%w: layer %s is Portal's own, built from the tenant's ledger (docs/design.md 55); mount under another name", ErrBadRequest, m.Name)
	}
	return p.mountLayer(m, record)
}

// mountLayer is mount without the derived layer's name reserved: the
// derived layer's own mount (syncDerived) goes through here.
func (p *Portal) mountLayer(m Mount, record bool) (*Layer, error) {
	if !layerName.MatchString(m.Name) {
		return nil, fmt.Errorf("%w: layer name %q must match %s", ErrBadRequest, m.Name, layerName)
	}
	hasEnv := len(m.Envelope) > 0 && string(m.Envelope) != "null"
	if (m.Path == "") == !hasEnv {
		return nil, fmt.Errorf("%w: give exactly one of path and envelope", ErrBadRequest)
	}
	var data []byte
	path := ""
	if hasEnv {
		data = m.Envelope
	} else {
		var err error
		if path, err = resolveLayerPath(m.Path); err != nil {
			return nil, err
		}
		if data, err = os.ReadFile(path); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
	}
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		return nil, &InvalidError{Message: "layer " + m.Name + " fails graphnode validate", Problems: ps}
	}
	env, err := graph.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	l := &Layer{Name: m.Name, Path: path, Env: env, MountedBy: m.By}
	l.Ontology, l.OntologyProblems = loadLayerOntology(path, env.Ontology)
	p.mu.Lock()
	defer p.mu.Unlock()
	if record {
		given := ""
		if !hasEnv {
			if given, err = filepath.Abs(m.Path); err != nil {
				return nil, err
			}
		}
		if err := p.recordMount(m.Name, given, m.By); err != nil {
			return nil, err
		}
	}
	for i, x := range p.layers {
		if x.Name == l.Name {
			p.layers[i] = l
			return l, nil
		}
	}
	p.layers = append(p.layers, l)
	return l, nil
}

// recordMount makes the screen's Mount node for layer name say path and the
// principal by (left out when ""), or removes it when path is empty (an
// inline mount). The write is checked like any other and leaves the screen
// as it is when nothing changes, so mounting the same path again does not
// restamp it. Caller holds mu.
func (p *Portal) recordMount(name, path, by string) error {
	id := MountID(name)
	i, ok := p.screen.NodeIndex()[id]
	if path == "" && !ok {
		return nil
	}
	props := map[string]any{"layer": name, "path": path}
	if by != "" {
		props["mountedBy"] = by
	}
	if path != "" && ok {
		n := p.screen.Nodes[i]
		if n.Kind == MountKind && n.Name == name && reflect.DeepEqual(n.Properties, props) {
			return nil
		}
	}
	next, err := roundTrip(p.screen)
	if err != nil {
		return err
	}
	var nodes, edges []string
	if path == "" {
		next.Nodes = append(next.Nodes[:i], next.Nodes[i+1:]...)
		kept := next.Edges[:0]
		for _, e := range next.Edges {
			if e.From != id && e.To != id {
				kept = append(kept, e)
			}
		}
		next.Edges = kept
	} else {
		n := graph.Node{ID: id, Kind: MountKind, Name: name, Properties: props}
		if ok {
			next.Nodes[i] = n
		} else {
			next.Nodes = append(next.Nodes, n)
		}
		nodes = []string{id}
	}
	if ps := p.check(next, nil, touch(nodes, edges)); len(ps) > 0 {
		return &InvalidError{Message: "the screen refuses the Mount node " + id, Problems: ps}
	}
	return p.commit(next)
}

// resolveLayerPath takes a .graph.json file, or a directory whose newest
// *.graph.json by name (layer stamps sort as strings) is mounted.
func resolveLayerPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if !st.IsDir() {
		return abs, nil
	}
	matches, err := filepath.Glob(filepath.Join(abs, "*.graph.json"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: no *.graph.json in %s", ErrNotFound, abs)
	}
	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// Layers returns the mounted layers in mount order, the derived layer
// brought up to date first.
func (p *Portal) Layers() []*Layer {
	p.syncDerived()
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*Layer(nil), p.layers...)
}

func (p *Portal) layer(name string) *Layer {
	for _, l := range p.layers {
		if l.Name == name {
			return l
		}
	}
	return nil
}

// layerHas reports whether a mounted layer holds a node. Caller holds mu.
func (p *Portal) layerHas(name, id string) bool {
	l := p.layer(name)
	if l == nil {
		return false
	}
	_, ok := l.Env.NodeIndex()[id]
	return ok
}

// Query returns the nodes of one layer: those reachable from `from` along
// edges in their direction (all when from is empty), then those of `kind`
// (all when kind is empty). Edges survive when both ends do.
func (p *Portal) Query(name, kind, from string) (*graph.Envelope, error) {
	p.syncDerived()
	p.mu.Lock()
	l := p.layer(name)
	p.mu.Unlock()
	if l == nil {
		return nil, fmt.Errorf("%w: no layer %s", ErrNotFound, name)
	}
	src := l.Env
	keep := map[string]bool{}
	if from == "" {
		for _, n := range src.Nodes {
			keep[n.ID] = true
		}
	} else {
		if _, ok := src.NodeIndex()[from]; !ok {
			return nil, fmt.Errorf("%w: no node %s in layer %s", ErrNotFound, from, name)
		}
		out := map[string][]string{}
		for _, e := range src.Edges {
			out[e.From] = append(out[e.From], e.To)
		}
		queue := []string{from}
		keep[from] = true
		for len(queue) > 0 {
			id := queue[0]
			queue = queue[1:]
			for _, to := range out[id] {
				if !keep[to] {
					keep[to] = true
					queue = append(queue, to)
				}
			}
		}
	}
	env := graph.NewEnvelope(src.Module, src.Version, src.Layer)
	env.Root = src.Root
	if from != "" {
		env.Root = from
	}
	for _, n := range src.Nodes {
		if keep[n.ID] && (kind == "" || n.Kind == kind) {
			env.Nodes = append(env.Nodes, n)
		}
	}
	idx := env.NodeIndex()
	for _, e := range src.Edges {
		_, a := idx[e.From]
		_, b := idx[e.To]
		if a && b {
			env.Edges = append(env.Edges, e)
		}
	}
	if _, ok := idx[env.Root]; !ok {
		env.Root = ""
	}
	env.Recount()
	return env, nil
}

// LayerOntologies maps each mounted layer to its ontology (nil when none).
func (p *Portal) LayerOntologies() map[string]map[string]any {
	p.syncDerived()
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]map[string]any, len(p.layers))
	for _, l := range p.layers {
		out[l.Name] = l.Ontology
	}
	return out
}

// ParseMountFlag reads name=path, as cmd/portal -mount takes it.
func ParseMountFlag(s string) (Mount, error) {
	name, path, ok := strings.Cut(s, "=")
	if !ok || name == "" || path == "" {
		return Mount{}, fmt.Errorf("-mount wants name=path, got %q", s)
	}
	return Mount{Name: name, Path: path}, nil
}
