// Package store holds the portal's state: the screen graph (a Graph.Node
// envelope), the mounted data layers (read-only), and the snaps on disk.
// Every write builds the next screen, validates it with Graph.Node and the
// portal ontology, and only then replaces the current one.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Root is the id of the dev seed's root Container; SeedTheme its Theme.
const (
	Root      = "ui:root"
	SeedTheme = "ui:theme:default"
)

// DesignerCategory is the name of the Category production mode hides.
const DesignerCategory = "Designer"

// MenuCategory is the name of the Category whose Containers are the top
// bar's entries; each opens the Container or Widget it shows
// (docs/design.md 25).
const MenuCategory = "Menu"

// SeedMenuEntry is the dev seed's one menu entry, "Graph", opening ui:canvas.
const SeedMenuEntry = "ui:menu:graph"

var (
	// ErrNotFound: no such node, layer or snap.
	ErrNotFound = errors.New("not found")
	// ErrConflict: an id is already taken.
	ErrConflict = errors.New("conflict")
	// ErrBadRequest: the body is not the right shape.
	ErrBadRequest = errors.New("bad request")
)

// InvalidError carries the problems that refused a write.
type InvalidError struct {
	Message  string
	Problems []graph.Problem
}

func (e *InvalidError) Error() string {
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.String()
	}
	return e.Message + ": " + strings.Join(parts, "; ")
}

// Options configures New.
type Options struct {
	// Artifacts is the directory holding snaps/.
	Artifacts string
	// Snap, when set, is the stamp of the snap to boot from instead of the
	// newest (PORTAL_SNAP).
	Snap string
	// Production hides every node that is-a Designer from Screen.
	Production bool
	// NoSeed leaves the screen empty when there is no snap to boot from.
	NoSeed bool
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

// Boot values other than a snap stamp.
const (
	BootSeed  = "seed"
	BootEmpty = "empty"
)

// Portal is the state behind the API. Safe for concurrent use.
type Portal struct {
	mu         sync.Mutex
	ont        *Ontology
	screen     *graph.Envelope
	layers     []*Layer
	artifacts  string
	production bool
	boot       string
	now        func() time.Time
	lastStamp  time.Time
}

// New boots the screen (docs/design.md 20): from the snap Options.Snap names,
// else from the newest snap under artifacts/snaps, else from the dev seed
// (or empty with NoSeed). The screen then lives in memory; only an approved
// snap outlives the process.
func New(o Options) (*Portal, error) {
	ont, problems, err := ParseOntology(portal.OntologyYAML)
	if err != nil {
		return nil, fmt.Errorf("ontology.yaml: %w", err)
	}
	if len(problems) > 0 {
		return nil, &InvalidError{Message: "ontology.yaml fails graphnode validate-ontology", Problems: problems}
	}
	p := &Portal{ont: ont, artifacts: o.Artifacts, production: o.Production, now: o.Now}
	if p.now == nil {
		p.now = time.Now
	}
	if p.artifacts == "" {
		p.artifacts = "artifacts"
	}
	stamp := o.Snap
	if stamp == "" {
		if stamp, err = p.newestSnap(); err != nil {
			return nil, err
		}
	}
	if stamp == "" {
		p.screen = p.empty()
		p.boot = BootEmpty
		if !o.NoSeed {
			if err := p.seed(); err != nil {
				return nil, fmt.Errorf("seed: %w", err)
			}
			p.boot = BootSeed
		}
		return p, nil
	}
	s, err := p.LoadSnap(stamp)
	if err != nil {
		return nil, fmt.Errorf("boot from snap %s: %w", stamp, err)
	}
	if ps := graph.Validate(s.Env); graph.HasErrors(ps) {
		return nil, &InvalidError{Message: s.Path + " is not a valid envelope; move it aside or set PORTAL_SNAP to another stamp", Problems: ps}
	}
	// The snap is the model; the screen takes this binary's header and drops
	// the snap's approval finding, which belongs to that snap alone.
	env := s.Env
	env.Module, env.Version, env.Ontology, env.Findings = portal.Module, portal.Version, "ontology.yaml", []graph.Finding{}
	if t, err := time.Parse(time.RFC3339Nano, env.Layer); err == nil {
		p.lastStamp = t.UTC()
	}
	env.Layer = p.stamp()
	env.Recount()
	p.screen = env
	p.boot = s.Stamp
	return p, nil
}

// Boot is what the screen booted from: a snap stamp, BootSeed or BootEmpty.
func (p *Portal) Boot() string { return p.boot }

// Ontology returns the portal's ontology.
func (p *Portal) Ontology() *Ontology { return p.ont }

func (p *Portal) empty() *graph.Envelope {
	env := graph.NewEnvelope(portal.Module, portal.Version, p.stamp())
	env.Ontology = "ontology.yaml"
	env.Root = Root
	return env
}

// stamp is a UTC stamp ending Z with milliseconds, strictly increasing, so
// two snaps in the same second still sort and never share a name.
func (p *Portal) stamp() string {
	t := p.now().UTC().Truncate(time.Millisecond)
	if !t.After(p.lastStamp) {
		t = p.lastStamp.Add(time.Millisecond)
	}
	p.lastStamp = t
	return t.Format(StampLayout)
}

// StampLayout is the layer stamp format: Graph.Node's utcStamp with a fixed
// three-digit fraction, so string order is time order.
const StampLayout = "2006-01-02T15:04:05.000Z"

// seed builds the dev screen: one root Container, its grid Layout and Theme,
// the four base Categories and Menu, one Canvas Widget on the filesystem
// layer, the Designer panel and one menu entry opening the Canvas
// (docs/design.md 7 and 25). The Theme's values are base.css's own,
// so a seeded page looks as it did before Themes.
func (p *Portal) seed() error {
	env := p.screen
	cat := func(name, desc string) graph.Node {
		return graph.Node{ID: "ui:category:" + name, Kind: "Category", Name: name,
			Properties: map[string]any{"label": name, "description": desc}}
	}
	env.Nodes = append(env.Nodes,
		graph.Node{ID: Root, Kind: "Container", Name: "root", Properties: map[string]any{"order": 0}},
		graph.Node{ID: "ui:layout:root", Kind: "Layout", Name: "root grid", Properties: map[string]any{"mode": "grid", "columns": 12}},
		graph.Node{ID: SeedTheme, Kind: "Theme", Name: "default", Properties: map[string]any{
			"color": "#f09648", "opacity": 0.11, "font": `"Segoe UI Variable Text", system-ui, sans-serif`}},
		cat("Canvas", "Draws a mounted layer as a graph (GraphRenderer's canvas widget). Colour and shape come from the layer ontology's Kind display block; grey when there is none."),
		cat("Table", "Lists the nodes of a mounted layer query: id, Kind, name."),
		cat("Detail", "Shows one data node: the node selected on a Canvas, else the Ref its displays edge points at."),
		cat(DesignerCategory, "Lists the ontology Kinds and adds Containers, Widgets, Categories and menu entries through the API. Hidden when PORTAL_MODE=production."),
		cat(MenuCategory, "The top bar's entries: every Container that is-a Menu, in order, each showing the Container or Widget its opens edge points at."),
		graph.Node{ID: "ui:canvas", Kind: "Widget", Name: "filesystem", Properties: map[string]any{
			"type": "Canvas", "source": "filesystem", "bounds": map[string]any{"x": 0, "y": 0, "w": 8, "h": 9}}},
		graph.Node{ID: "ui:designer", Kind: "Container", Name: "designer", Properties: map[string]any{
			"order": 1, "parent": Root, "bounds": map[string]any{"x": 8, "y": 0, "w": 4, "h": 9}}},
		graph.Node{ID: SeedMenuEntry, Kind: "Container", Name: "Graph", Properties: map[string]any{"order": 0}},
	)
	env.Edges = append(env.Edges,
		graph.NewEdge(Root, "ui:layout:root", "laid-out-by"),
		graph.NewEdge(Root, SeedTheme, "themed-by"),
		graph.NewEdge(Root, "ui:canvas", "contains"),
		graph.NewEdge("ui:canvas", "ui:category:Canvas", "is-a"),
		graph.NewEdge(Root, "ui:designer", "contains"),
		graph.NewEdge("ui:designer", "ui:category:"+DesignerCategory, "is-a"),
		graph.NewEdge(SeedMenuEntry, "ui:category:"+MenuCategory, "is-a"),
		graph.NewEdge(SeedMenuEntry, "ui:canvas", "opens"),
	)
	all := make([]string, 0, len(env.Nodes))
	for _, n := range env.Nodes {
		all = append(all, n.ID)
	}
	edges := make([]string, 0, len(env.Edges))
	for _, e := range env.Edges {
		edges = append(edges, e.ID)
	}
	// Seed values are Go ints; the round trip makes them json.Number like any
	// other node, so the checks see what a reload would.
	next, err := roundTrip(env)
	if err != nil {
		return err
	}
	if ps := p.check(next, nil, touch(all, edges)); len(ps) > 0 {
		return &InvalidError{Message: "the dev seed does not validate", Problems: ps}
	}
	return p.commit(next)
}

// Screen returns a copy of the screen graph. In production mode every node
// that is-a Designer (directly or through Categories that is-a it) is left
// out, with the edges touching it; nothing else changes.
func (p *Portal) Screen() (*graph.Envelope, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	env, err := roundTrip(p.screen)
	if err != nil {
		return nil, err
	}
	if p.production {
		hideDesigner(env)
	}
	env.Recount()
	return env, nil
}

// Production reports whether Designer nodes are hidden.
func (p *Portal) Production() bool { return p.production }

func hideDesigner(env *graph.Envelope) {
	designer := categoryByName(env, DesignerCategory)
	if designer == "" {
		return
	}
	isA := map[string][]string{}
	for _, e := range env.Edges {
		if e.Kind == "is-a" {
			isA[e.From] = append(isA[e.From], e.To)
		}
	}
	var reaches func(id string, seen map[string]bool) bool
	reaches = func(id string, seen map[string]bool) bool {
		if seen[id] {
			return false
		}
		seen[id] = true
		for _, to := range isA[id] {
			if to == designer || reaches(to, seen) {
				return true
			}
		}
		return false
	}
	hidden := map[string]bool{}
	for _, n := range env.Nodes {
		if n.ID != designer && reaches(n.ID, map[string]bool{}) {
			hidden[n.ID] = true
		}
	}
	nodes := env.Nodes[:0]
	for _, n := range env.Nodes {
		if !hidden[n.ID] {
			nodes = append(nodes, n)
		}
	}
	env.Nodes = nodes
	edges := env.Edges[:0]
	for _, e := range env.Edges {
		if !hidden[e.From] && !hidden[e.To] {
			edges = append(edges, e)
		}
	}
	env.Edges = edges
}

// AddNode adds one node from a raw JSON body and returns it as stored.
func (p *Portal) AddNode(body []byte) (*graph.Node, error) {
	obj, err := decodeObject(body)
	if err != nil {
		return nil, err
	}
	var n graph.Node
	if err := decodeInto(body, &n); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.screen.NodeIndex()[n.ID]; ok && n.ID != "" {
		return nil, fmt.Errorf("%w: node %s already exists", ErrConflict, n.ID)
	}
	next, err := roundTrip(p.screen)
	if err != nil {
		return nil, err
	}
	next.Nodes = append(next.Nodes, n)
	raw, err := withRaw(next, "nodes", obj)
	if err != nil {
		return nil, err
	}
	problems := p.check(next, raw, touch([]string{n.ID}, nil))
	for _, k := range sortedKeys(obj) {
		if !nodeFields[k] {
			problems = append(problems, graph.Problem{Code: "unknown-field", Path: fmt.Sprintf("/nodes/%d/%s", len(next.Nodes)-1, k),
				Message: fmt.Sprintf("a screen node carries id, kind, name, properties, source, findings and stamp only, not %q", k)})
		}
	}
	if len(problems) > 0 {
		return nil, &InvalidError{Message: "node refused", Problems: problems}
	}
	if err := p.commit(next); err != nil {
		return nil, err
	}
	out := p.screen.Nodes[len(p.screen.Nodes)-1]
	return &out, nil
}

// AddEdge adds one edge from a raw JSON body. A missing id is set to
// from|to|kind; a wrong one is Graph.Node's edge-id problem.
func (p *Portal) AddEdge(body []byte) (*graph.Edge, error) {
	obj, err := decodeObject(body)
	if err != nil {
		return nil, err
	}
	var e graph.Edge
	if err := decodeInto(body, &e); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if _, ok := obj["id"]; !ok {
		e.ID = graph.EdgeID(e.From, e.To, e.Kind)
		obj["id"] = e.ID
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, x := range p.screen.Edges {
		if x.ID == e.ID {
			return nil, fmt.Errorf("%w: edge %s already exists", ErrConflict, e.ID)
		}
	}
	next, err := roundTrip(p.screen)
	if err != nil {
		return nil, err
	}
	next.Edges = append(next.Edges, e)
	raw, err := withRaw(next, "edges", obj)
	if err != nil {
		return nil, err
	}
	if ps := p.check(next, raw, touch(nil, []string{e.ID})); len(ps) > 0 {
		return nil, &InvalidError{Message: "edge refused", Problems: ps}
	}
	if err := p.commit(next); err != nil {
		return nil, err
	}
	out := p.screen.Edges[len(p.screen.Edges)-1]
	return &out, nil
}

// Patch is the body of PATCH /api/nodes/{id}.
type Patch struct {
	Properties map[string]any `json:"properties"`
	State      map[string]any `json:"state"`
}

// StateID is the id of a node's State node.
func StateID(id string) string { return "ui:state:" + strings.TrimPrefix(id, IDPrefix) }

// PatchNode merges properties (null removes a key) and display state into
// the node and its State node. It returns the node and, when state was
// given, the State node.
func (p *Portal) PatchNode(id string, body []byte) (*graph.Node, *graph.Node, error) {
	var patch Patch
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(&patch); err != nil {
		return nil, nil, fmt.Errorf("%w: PATCH changes properties and state only: %v", ErrBadRequest, err)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	next, err := roundTrip(p.screen)
	if err != nil {
		return nil, nil, err
	}
	idx := next.NodeIndex()
	i, ok := idx[id]
	if !ok {
		return nil, nil, fmt.Errorf("%w: no node %s", ErrNotFound, id)
	}
	nodes, edges := []string{id}, []string(nil)
	merge(&next.Nodes[i].Properties, patch.Properties)
	if patch.State != nil {
		if next.Nodes[i].Kind == "State" {
			return nil, nil, fmt.Errorf("%w: a State node has no State; patch its properties", ErrBadRequest)
		}
		sid := StateID(id)
		j, ok := idx[sid]
		if !ok {
			next.Nodes = append(next.Nodes, graph.Node{ID: sid, Kind: "State", Name: "state of " + id})
			j = len(next.Nodes) - 1
			e := graph.NewEdge(id, sid, "has-state")
			next.Edges = append(next.Edges, e)
			edges = append(edges, e.ID)
		}
		merge(&next.Nodes[j].Properties, patch.State)
		nodes = append(nodes, sid)
	}
	if ps := p.check(next, nil, touch(nodes, edges)); len(ps) > 0 {
		return nil, nil, &InvalidError{Message: "patch refused", Problems: ps}
	}
	if err := p.commit(next); err != nil {
		return nil, nil, err
	}
	idx = p.screen.NodeIndex()
	node := p.screen.Nodes[idx[id]]
	if patch.State == nil {
		return &node, nil, nil
	}
	state := p.screen.Nodes[idx[StateID(id)]]
	return &node, &state, nil
}

func merge(dst *map[string]any, src map[string]any) {
	if len(src) == 0 {
		return
	}
	if *dst == nil {
		*dst = map[string]any{}
	}
	for k, v := range src {
		if v == nil {
			delete(*dst, k)
		} else {
			(*dst)[k] = v
		}
	}
	if len(*dst) == 0 {
		*dst = nil
	}
}

// DeleteNode removes a node and every edge touching it.
func (p *Portal) DeleteNode(id string) (*graph.Node, []string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	next, err := roundTrip(p.screen)
	if err != nil {
		return nil, nil, err
	}
	i, ok := next.NodeIndex()[id]
	if !ok {
		return nil, nil, fmt.Errorf("%w: no node %s", ErrNotFound, id)
	}
	gone := next.Nodes[i]
	next.Nodes = append(next.Nodes[:i], next.Nodes[i+1:]...)
	removed := []string{}
	edges := next.Edges[:0]
	for _, e := range next.Edges {
		if e.From == id || e.To == id {
			removed = append(removed, e.ID)
			continue
		}
		edges = append(edges, e)
	}
	next.Edges = edges
	if ps := p.check(next, nil, touched{}); len(ps) > 0 {
		return nil, nil, &InvalidError{Message: "delete refused", Problems: ps}
	}
	if err := p.commit(next); err != nil {
		return nil, nil, err
	}
	return &gone, removed, nil
}

// commit stamps the next screen and makes it current. Nothing is written:
// POST /api/snap is the only way a screen reaches disk (docs/design.md 20).
func (p *Portal) commit(next *graph.Envelope) error {
	next.Layer = p.stamp()
	next.Recount()
	p.screen = next
	return nil
}

// roundTrip deep-copies an envelope through JSON, so numbers are json.Number
// exactly as after a reload from disk.
func roundTrip(env *graph.Envelope) (*graph.Envelope, error) {
	b, err := graph.Marshal(env)
	if err != nil {
		return nil, err
	}
	return graph.Decode(bytes.NewReader(b))
}

func decodeObject(body []byte) (map[string]any, error) {
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil || obj == nil {
		return nil, fmt.Errorf("%w: body must be one JSON object", ErrBadRequest)
	}
	return obj, nil
}

func decodeInto(body []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	return dec.Decode(v)
}

// withRaw serialises next with its last element of list replaced by the raw
// body, so Graph.Node sees keys the Go types drop (display, above all).
func withRaw(next *graph.Envelope, list string, raw map[string]any) ([]byte, error) {
	next.Recount()
	b, err := graph.Marshal(next)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := decodeInto(b, &doc); err != nil {
		return nil, err
	}
	items := doc[list].([]any)
	items[len(items)-1] = raw
	return json.Marshal(doc)
}
