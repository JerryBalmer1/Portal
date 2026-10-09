package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Snap is one screen graph written to artifacts/snaps.
type Snap struct {
	Stamp string // file-safe: 2026-10-09T01-02-03.456Z
	Layer string // the envelope layer: 2026-10-09T01:02:03.456Z
	Path  string
	Env   *graph.Envelope
}

var snapStamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}[-:][0-9]{2}[-:][0-9]{2}([.][0-9]+)?Z$`)

// FileStamp turns a layer stamp into the snap's file name stem.
func FileStamp(layer string) string { return strings.ReplaceAll(layer, ":", "-") }

func (p *Portal) snapDir() string { return filepath.Join(p.artifacts, "snaps") }

// newestSnap returns the stamp of the newest snap under artifacts/snaps by
// name (stamps sort as time), or "" when there is none. Files whose name is
// not a snap stamp are ignored.
func (p *Portal) newestSnap() (string, error) {
	matches, err := filepath.Glob(filepath.Join(p.snapDir(), "*.graph.json"))
	if err != nil {
		return "", err
	}
	newest := ""
	for _, m := range matches {
		stamp := strings.TrimSuffix(filepath.Base(m), ".graph.json")
		if snapStamp.MatchString(stamp) && stamp > newest {
			newest = stamp
		}
	}
	return newest, nil
}

// ApprovedFinding is the envelope finding Kind that carries a snap's note.
const ApprovedFinding = "approved"

// Approval is what POST /api/snap returns beyond the snap: the snap before
// it, if any, and what this one added, removed and changed against it.
type Approval struct {
	Snap     *Snap
	Note     string
	Previous string // "" for the first snap
	Diff     *Diff  // nil for the first snap
}

// TakeSnap approves the screen: it writes the full screen graph (production
// mode does not hide anything here: a snap is the model, not the view) to
// artifacts/snaps/<stamp>.graph.json with ontology.yaml beside it, the note
// as the envelope's one approved finding, and diffs it against the snap
// before it (docs/design.md 21).
func (p *Portal) TakeSnap(note string) (*Approval, error) {
	if len(note) > maxNote {
		return nil, fmt.Errorf("%w: note is %d bytes; at most %d", ErrBadRequest, len(note), maxNote)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	env, err := roundTrip(p.screen)
	if err != nil {
		return nil, err
	}
	env.Layer = p.stamp()
	env.Ontology = "ontology.yaml"
	msg := strings.TrimSpace(note)
	if msg == "" {
		msg = "approved without a note"
	}
	env.Findings = []graph.Finding{{Kind: ApprovedFinding, Severity: "info", Message: msg}}
	env.Recount()
	if ps := graph.Validate(env); graph.HasErrors(ps) {
		return nil, &InvalidError{Message: "the screen does not validate", Problems: ps}
	}
	prev, err := p.newestSnap()
	if err != nil {
		return nil, err
	}
	dir := p.snapDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "ontology.yaml"), portal.OntologyYAML, 0o644); err != nil {
		return nil, err
	}
	s := &Snap{Stamp: FileStamp(env.Layer), Layer: env.Layer, Env: env}
	s.Path = filepath.Join(dir, s.Stamp+".graph.json")
	if err := graph.Save(s.Path, env); err != nil {
		return nil, err
	}
	a := &Approval{Snap: s, Note: strings.TrimSpace(note)}
	if prev == "" {
		return a, nil
	}
	ps, err := p.LoadSnap(prev)
	if err != nil {
		return nil, fmt.Errorf("previous snap %s: %w", prev, err)
	}
	if a.Diff, err = DiffSnaps(ps, s); err != nil {
		return nil, err
	}
	a.Previous = prev
	return a, nil
}

// maxNote caps an approval note.
const maxNote = 2000

// LoadSnap reads one snap by stamp, in either form.
func (p *Portal) LoadSnap(stamp string) (*Snap, error) {
	if !snapStamp.MatchString(stamp) {
		return nil, fmt.Errorf("%w: %q is not a snap stamp (2026-10-09T01-02-03.456Z)", ErrBadRequest, stamp)
	}
	s := &Snap{Stamp: FileStamp(stamp)}
	s.Path = filepath.Join(p.snapDir(), s.Stamp+".graph.json")
	env, err := graph.Load(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: no snap %s", ErrNotFound, s.Stamp)
	}
	if err != nil {
		return nil, err
	}
	s.Env, s.Layer = env, env.Layer
	return s, nil
}

// Change is one element present in both snaps whose content differs.
type Change struct {
	ID     string         `json:"id"`
	Before map[string]any `json:"before"`
	After  map[string]any `json:"after"`
}

// Diff is snap b against snap a, and the design-11 merge of a then b.
type Diff struct {
	A, B         string
	AddedNodes   []graph.Node
	AddedEdges   []graph.Edge
	RemovedNodes []graph.Node
	RemovedEdges []graph.Edge
	ChangedNodes []Change
	ChangedEdges []Change
	Merged       *graph.Envelope
}

// DiffSnaps merges a then b the Graph.Node design 11 way, in memory only:
// upsert on id, firstSeen the first layer an id appears in, lastSeen the
// last, and an element absent from b kept and marked with an absent finding
// rather than deleted. Content compares id, kind, name, properties, source
// and findings; stamps are bookkeeping and never count as a change.
func DiffSnaps(a, b *Snap) (*Diff, error) {
	d := &Diff{A: a.Stamp, B: b.Stamp, AddedNodes: []graph.Node{}, AddedEdges: []graph.Edge{},
		RemovedNodes: []graph.Node{}, RemovedEdges: []graph.Edge{}, ChangedNodes: []Change{}, ChangedEdges: []Change{}}
	merged := graph.NewEnvelope(portal.Module, portal.Version, b.Env.Layer)
	merged.Root = b.Env.Root
	merged.Ontology = "ontology.yaml"

	aNodes := map[string]graph.Node{}
	for _, n := range a.Env.Nodes {
		aNodes[n.ID] = n
	}
	bNodes := map[string]bool{}
	for _, n := range b.Env.Nodes {
		bNodes[n.ID] = true
		first := b.Env.Layer
		if old, ok := aNodes[n.ID]; ok {
			first = a.Env.Layer
			if c, changed, err := compare(n.ID, old, n); err != nil {
				return nil, err
			} else if changed {
				d.ChangedNodes = append(d.ChangedNodes, c)
			}
		} else {
			d.AddedNodes = append(d.AddedNodes, n)
		}
		n.Stamp = &graph.Stamp{Layer: b.Env.Layer, FirstSeen: first, LastSeen: b.Env.Layer}
		merged.Nodes = append(merged.Nodes, n)
	}
	for _, n := range a.Env.Nodes {
		if bNodes[n.ID] {
			continue
		}
		d.RemovedNodes = append(d.RemovedNodes, n)
		n.Stamp = &graph.Stamp{Layer: a.Env.Layer, FirstSeen: a.Env.Layer, LastSeen: a.Env.Layer}
		n.Findings = append(n.Findings, absent(n.ID, b.Stamp))
		merged.Nodes = append(merged.Nodes, n)
	}

	aEdges := map[string]graph.Edge{}
	for _, e := range a.Env.Edges {
		aEdges[e.ID] = e
	}
	bEdges := map[string]bool{}
	for _, e := range b.Env.Edges {
		bEdges[e.ID] = true
		first := b.Env.Layer
		if old, ok := aEdges[e.ID]; ok {
			first = a.Env.Layer
			if c, changed, err := compare(e.ID, old, e); err != nil {
				return nil, err
			} else if changed {
				d.ChangedEdges = append(d.ChangedEdges, c)
			}
		} else {
			d.AddedEdges = append(d.AddedEdges, e)
		}
		e.Stamp = &graph.Stamp{Layer: b.Env.Layer, FirstSeen: first, LastSeen: b.Env.Layer}
		merged.Edges = append(merged.Edges, e)
	}
	for _, e := range a.Env.Edges {
		if bEdges[e.ID] {
			continue
		}
		d.RemovedEdges = append(d.RemovedEdges, e)
		e.Stamp = &graph.Stamp{Layer: a.Env.Layer, FirstSeen: a.Env.Layer, LastSeen: a.Env.Layer}
		e.Findings = append(e.Findings, absent(e.ID, b.Stamp))
		merged.Edges = append(merged.Edges, e)
	}
	merged.Recount()
	d.Merged = merged
	return d, nil
}

func absent(id, b string) graph.Finding {
	return graph.Finding{Kind: "absent", Severity: "info", Message: "absent from snap " + b, Target: id}
}

// compare reports whether two elements differ in content, stamps aside.
func compare(id string, before, after any) (Change, bool, error) {
	bm, err := contentMap(before)
	if err != nil {
		return Change{}, false, err
	}
	am, err := contentMap(after)
	if err != nil {
		return Change{}, false, err
	}
	bb, _ := json.Marshal(bm)
	ab, _ := json.Marshal(am)
	if bytes.Equal(bb, ab) {
		return Change{}, false, nil
	}
	return Change{ID: id, Before: bm, After: am}, true, nil
}

func contentMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := decodeInto(b, &m); err != nil {
		return nil, err
	}
	delete(m, "stamp")
	return m, nil
}
