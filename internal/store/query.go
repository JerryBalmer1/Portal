package store

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// The tenant graph query (docs/design.md 54): one breadth-first walk over
// every mounted layer at once, from start nodes, along edges of the kinds
// asked, to a depth. The walk reads the graph through Adjacency only, so a
// SQLite or graph-store adapter replaces memGraph and nothing else.

// Directions a walk follows an edge in.
const (
	DirOut  = "out"
	DirIn   = "in"
	DirBoth = "both"
)

// Walk bounds: the depth and the node cap a request may ask for, and the
// ones it gets when it asks for none.
const (
	WalkDepth    = 3
	WalkMaxDepth = 10
	WalkLimit    = 500
	WalkMaxLimit = 5000
)

// Hop is one step out of a node: the edge followed and the node at its
// other end.
type Hop struct {
	Edge graph.Edge
	Node graph.Node
}

// Adjacency is everything the walk reads, the contract an adapter keeps.
type Adjacency interface {
	// Neighbours is the node id names and its hops in direction (out: the
	// edges from it, in: the edges to it, both: out then in), only along
	// the edge kinds in follow (every kind when follow is empty); ok is
	// false when no node has that id.
	Neighbours(id, direction string, follow []string) (n graph.Node, hops []Hop, ok bool)
	// NodesOfKind is every node of kind in a stable order; every node when
	// kind is "".
	NodesOfKind(kind string) []graph.Node
}

// Where is one where filter: the value at Path (a node's id, kind or name,
// else a dotted path into its properties) must be Value, exactly.
type Where struct {
	Path  []string
	Value string
}

// ParseWhere reads key=value, the key dotted (facet.key).
func ParseWhere(s string) (Where, error) {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return Where{}, fmt.Errorf("where %q is not key=value", s)
	}
	path := strings.Split(k, ".")
	if slices.Contains(path, "") {
		return Where{}, fmt.Errorf("where %q has an empty key segment", s)
	}
	return Where{Path: path, Value: v}, nil
}

// String is the filter as written.
func (w Where) String() string { return strings.Join(w.Path, ".") + "=" + w.Value }

// Match reports whether n's value at the path is the filter's: a string as
// is, a number or a boolean as JSON writes it. A list, a map or no value
// never matches.
func (w Where) Match(n graph.Node) bool {
	if len(w.Path) == 1 {
		switch w.Path[0] {
		case "id":
			return n.ID == w.Value
		case "kind":
			return n.Kind == w.Value
		case "name":
			return n.Name == w.Value
		}
	}
	path := w.Path
	if path[0] == "properties" && len(path) > 1 {
		path = path[1:]
	}
	var v any = n.Properties
	for _, k := range path {
		m, ok := v.(map[string]any)
		if !ok {
			return false
		}
		if v, ok = m[k]; !ok {
			return false
		}
	}
	switch x := v.(type) {
	case string:
		return x == w.Value
	case float64, int, int64, bool, json.Number:
		b, _ := json.Marshal(x)
		return string(b) == w.Value
	}
	return false
}

// Walk is one query. With From empty and Kind "", every node starts (the
// tenant graph; the API refuses it).
type Walk struct {
	From      []string
	Kind      string
	Follow    []string
	Direction string
	Depth     int
	Until     string
	Where     []Where
	Source    string
	// Limit caps the nodes in the result; 0 is no cap.
	Limit int
}

// WalkResult is what a walk reached: its nodes in the order first reached,
// the edges between them, a path from a start to each node the walk
// stopped at, and, when the cap was hit, the hop it was hit on.
type WalkResult struct {
	Nodes       []graph.Node
	Edges       []graph.Edge
	Paths       [][]string
	Truncated   bool
	TruncatedAt int
}

// UnknownNodeError is a from id no node has.
type UnknownNodeError struct{ ID string }

func (e *UnknownNodeError) Error() string { return "no node " + e.ID + " in the mounted layers" }

// RunWalk walks g breadth-first, one hop at a time for every start
// together, each start with its own visited set; so each node is expanded
// once per start and an edge followed at most once per direction. A node
// enters the result only when every where filter matches it, and only
// then is it expanded. A node of Kind until is kept and not expanded; a
// start always is. When the result would pass Limit the walk stops there.
// Edges are every edge of g between two nodes of the result, whichever
// way the walk went.
func RunWalk(g Adjacency, w Walk) (*WalkResult, error) {
	type start struct {
		id     string
		parent map[string]string
		order  []string
		front  []string
	}
	var starts []*start
	var first []graph.Node
	if len(w.From) > 0 {
		for _, id := range w.From {
			n, _, ok := g.Neighbours(id, DirOut, nil)
			if !ok {
				return nil, &UnknownNodeError{ID: id}
			}
			first = append(first, n)
		}
	} else {
		first = g.NodesOfKind(w.Kind)
	}
	pass := func(n graph.Node) bool {
		for _, f := range w.Where {
			if !f.Match(n) {
				return false
			}
		}
		return true
	}
	res := &WalkResult{}
	kept := map[string]bool{}
	add := func(n graph.Node) bool {
		if kept[n.ID] {
			return true
		}
		if w.Limit > 0 && len(res.Nodes) >= w.Limit {
			return false
		}
		kept[n.ID] = true
		res.Nodes = append(res.Nodes, n)
		return true
	}
	kinds := map[string]string{}
	seen := map[string]bool{}
	for _, n := range first {
		if seen[n.ID] || !pass(n) {
			continue
		}
		seen[n.ID] = true
		if !add(n) {
			res.Truncated = true
			break
		}
		kinds[n.ID] = n.Kind
		starts = append(starts, &start{id: n.ID, parent: map[string]string{n.ID: ""}, order: []string{n.ID}, front: []string{n.ID}})
	}
	for hop := 1; hop <= w.Depth && !res.Truncated; hop++ {
		for _, s := range starts {
			var next []string
			for _, id := range s.front {
				if hop > 1 && w.Until != "" && kinds[id] == w.Until {
					continue
				}
				_, hops, _ := g.Neighbours(id, w.Direction, w.Follow)
				for _, h := range hops {
					to := h.Node.ID
					if _, ok := s.parent[to]; ok || !pass(h.Node) {
						continue
					}
					if !add(h.Node) {
						res.Truncated, res.TruncatedAt = true, hop
						break
					}
					kinds[to] = h.Node.Kind
					s.parent[to] = id
					s.order = append(s.order, to)
					next = append(next, to)
				}
				if res.Truncated {
					break
				}
			}
			s.front = next
			if res.Truncated {
				break
			}
		}
	}
	// A path ends where the walk stopped: at a node no other node of the
	// same start was reached from.
	for _, s := range starts {
		inner := map[string]bool{}
		for _, id := range s.order {
			inner[s.parent[id]] = true
		}
		for _, id := range s.order {
			if inner[id] {
				continue
			}
			var p []string
			for at := id; at != ""; at = s.parent[at] {
				p = append(p, at)
			}
			slices.Reverse(p)
			res.Paths = append(res.Paths, p)
		}
	}
	edged := map[string]bool{}
	for _, n := range res.Nodes {
		_, hops, _ := g.Neighbours(n.ID, DirOut, nil)
		for _, h := range hops {
			id := h.Edge.ID
			if id == "" {
				id = graph.EdgeID(h.Edge.From, h.Edge.To, h.Edge.Kind)
			}
			if kept[h.Node.ID] && !edged[id] {
				edged[id] = true
				res.Edges = append(res.Edges, h.Edge)
			}
		}
	}
	return res, nil
}

// memGraph is Adjacency over the mounted layers in memory, as the tenant
// graph reads them (docs/design.md 53): with source, only the layers whose
// source block names that emitter; the first id wins in mount order; a
// node with no stamp gets stamp.layer from its layer; an alert layer's
// copies of its subjects are left out, its alerts kept; an edge is kept
// once, and only when both its ends are nodes.
type memGraph struct {
	nodes   map[string]graph.Node
	order   []string
	out, in map[string][]graph.Edge
}

func (p *Portal) memGraph(source string) *memGraph {
	var layers []*Layer
	for _, l := range p.Layers() {
		if source == "" || (l.Env.Source != nil && l.Env.Source.Emitter == source) {
			layers = append(layers, l)
		}
	}
	g := &memGraph{nodes: map[string]graph.Node{}, out: map[string][]graph.Edge{}, in: map[string][]graph.Edge{}}
	for _, l := range layers {
		copies := IsAlertLayer(l.Env)
		for _, n := range l.Env.Nodes {
			if _, ok := g.nodes[n.ID]; ok || (copies && n.Kind != AlertKind) {
				continue
			}
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: l.Env.Layer}
			}
			g.nodes[n.ID] = n
			g.order = append(g.order, n.ID)
		}
	}
	seen := map[string]bool{}
	for _, l := range layers {
		for _, e := range l.Env.Edges {
			_, a := g.nodes[e.From]
			_, b := g.nodes[e.To]
			if !a || !b {
				continue
			}
			id := e.ID
			if id == "" {
				id = graph.EdgeID(e.From, e.To, e.Kind)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			g.out[e.From] = append(g.out[e.From], e)
			g.in[e.To] = append(g.in[e.To], e)
		}
	}
	return g
}

func (g *memGraph) Neighbours(id, direction string, follow []string) (graph.Node, []Hop, bool) {
	n, ok := g.nodes[id]
	if !ok {
		return graph.Node{}, nil, false
	}
	var hops []Hop
	take := func(edges []graph.Edge, out bool) {
		for _, e := range edges {
			if len(follow) > 0 && !slices.Contains(follow, e.Kind) {
				continue
			}
			other := e.To
			if !out {
				other = e.From
			}
			hops = append(hops, Hop{Edge: e, Node: g.nodes[other]})
		}
	}
	if direction != DirIn {
		take(g.out[id], true)
	}
	if direction == DirIn || direction == DirBoth {
		take(g.in[id], false)
	}
	return n, hops, true
}

func (g *memGraph) NodesOfKind(kind string) []graph.Node {
	var out []graph.Node
	for _, id := range g.order {
		if n := g.nodes[id]; kind == "" || n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

// GraphQuery runs w over the mounted layers, or those of w.Source.
func (p *Portal) GraphQuery(w Walk) (*WalkResult, error) {
	return RunWalk(p.memGraph(w.Source), w)
}
