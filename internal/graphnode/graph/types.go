// Package graph holds the base shape every graph module in the family shares:
// Node, Edge and Envelope, plus Load and Validate. The JSON Schemas under
// ../schemas are the contract; these types follow them, never the reverse.
package graph

// SchemaVersion is the value of Envelope.Schema this package understands.
const SchemaVersion = "graph/1"

// EdgeIDSeparator joins from, to and kind into an Edge.ID.
const EdgeIDSeparator = "|"

// Source is one provenance item: where a node or edge came from.
type Source struct {
	Tool    string `json:"tool,omitempty"`
	Command string `json:"command,omitempty"`
	Path    string `json:"path,omitempty"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
}

// Finding is something noticed about a node, an edge, or the graph.
type Finding struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message"`
	Target   string `json:"target,omitempty"`
}

// Stamp records which layer emitted an element and when a merge saw it.
type Stamp struct {
	Layer     string `json:"layer,omitempty"`
	FirstSeen string `json:"firstSeen,omitempty"`
	LastSeen  string `json:"lastSeen,omitempty"`
}

// Node is a thing in a graph.
type Node struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Name       string         `json:"name,omitempty"`
	Properties map[string]any `json:"properties,omitempty"`
	Source     []Source       `json:"source,omitempty"`
	Findings   []Finding      `json:"findings,omitempty"`
	Stamp      *Stamp         `json:"stamp,omitempty"`
}

// Edge is a relation between two nodes. ID must equal EdgeID(From, To, Kind).
type Edge struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	From       string         `json:"from"`
	To         string         `json:"to"`
	Properties map[string]any `json:"properties,omitempty"`
	Source     []Source       `json:"source,omitempty"`
	Findings   []Finding      `json:"findings,omitempty"`
	Stamp      *Stamp         `json:"stamp,omitempty"`
}

// Counts summarises an envelope.
type Counts struct {
	Nodes  int            `json:"nodes"`
	Edges  int            `json:"edges"`
	ByKind map[string]int `json:"byKind,omitempty"`
}

// Envelope is the document an emitter writes: one layer of one module.
type Envelope struct {
	Schema   string    `json:"schema"`
	Ontology string    `json:"ontology,omitempty"`
	Module   string    `json:"module"`
	Version  string    `json:"version,omitempty"`
	Layer    string    `json:"layer"`
	Root     string    `json:"root,omitempty"`
	Nodes    []Node    `json:"nodes"`
	Edges    []Edge    `json:"edges"`
	Findings []Finding `json:"findings"`
	Counts   *Counts   `json:"counts,omitempty"`
}

// EdgeID builds the id an edge must carry: from|to|kind with % and | in from
// and to percent-encoded as %25 and %7C (design.md 15). Ids without either
// character are unchanged.
func EdgeID(from, to, kind string) string {
	return escapeEdgePart(from) + EdgeIDSeparator + escapeEdgePart(to) + EdgeIDSeparator + kind
}

// NewEdge returns an edge with its ID set by the rule.
func NewEdge(from, to, kind string) Edge {
	return Edge{ID: EdgeID(from, to, kind), Kind: kind, From: from, To: to}
}

// NewEnvelope returns an empty envelope for a module and layer with the
// schema version set and the lists non-nil so they serialise as [] not null.
func NewEnvelope(module, version, layer string) *Envelope {
	return &Envelope{
		Schema:   SchemaVersion,
		Module:   module,
		Version:  version,
		Layer:    layer,
		Nodes:    []Node{},
		Edges:    []Edge{},
		Findings: []Finding{},
	}
}

// Recount fills Counts from the current nodes and edges.
func (e *Envelope) Recount() {
	c := &Counts{Nodes: len(e.Nodes), Edges: len(e.Edges), ByKind: map[string]int{}}
	for _, n := range e.Nodes {
		c.ByKind[n.Kind]++
	}
	e.Counts = c
}

// NodeIndex returns a map from node id to index in Nodes.
func (e *Envelope) NodeIndex() map[string]int {
	m := make(map[string]int, len(e.Nodes))
	for i, n := range e.Nodes {
		m[n.ID] = i
	}
	return m
}
