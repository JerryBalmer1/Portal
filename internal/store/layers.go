package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
}

var layerName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Mount is the body of POST /api/layers.
type Mount struct {
	Name     string          `json:"name"`
	Path     string          `json:"path"`
	Envelope json.RawMessage `json:"envelope"`
}

// MountLayer validates and mounts one layer, replacing a layer of the same
// name. An envelope that fails Graph.Node validation is refused with its
// problems; an ontology that cannot be read or fails is reported on the
// layer, which then draws grey.
func (p *Portal) MountLayer(m Mount) (*Layer, error) {
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
	l := &Layer{Name: m.Name, Path: path, Env: env}
	l.Ontology, l.OntologyProblems = loadLayerOntology(path, env.Ontology)
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, x := range p.layers {
		if x.Name == l.Name {
			p.layers[i] = l
			return l, nil
		}
	}
	p.layers = append(p.layers, l)
	return l, nil
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

// Layers returns the mounted layers in mount order.
func (p *Portal) Layers() []*Layer {
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
