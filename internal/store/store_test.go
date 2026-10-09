package store

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

func TestOntologyValidates(t *testing.T) {
	data, err := os.ReadFile("../../ontology.yaml")
	if err != nil {
		t.Fatal(err)
	}
	o, ps, err := ParseOntology(data)
	if err != nil || graph.HasErrors(ps) {
		t.Fatalf("ontology.yaml: %v %v", err, ps)
	}
	for _, k := range []string{"Container", "Widget", "Layout", "Category", "State", "Theme", "Ref"} {
		if _, ok := o.Kinds[k]; !ok {
			t.Errorf("Kind %s missing", k)
		}
	}
	for _, e := range []string{"contains", "laid-out-by", "is-a", "displays", "has-state", "themed-by", "opens"} {
		if _, ok := o.EdgeKinds[e]; !ok {
			t.Errorf("edge Kind %s missing", e)
		}
	}
}

func TestNoSeedLeavesTheScreenEmpty(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir(), NoSeed: true})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := p.Screen()
	if len(env.Nodes) != 0 {
		t.Errorf("%d nodes", len(env.Nodes))
	}
}

func TestStampsStrictlyIncrease(t *testing.T) {
	fixed := time.Date(2026, 10, 9, 1, 2, 3, 0, time.UTC)
	p, err := New(Options{Artifacts: t.TempDir(), Now: func() time.Time { return fixed }})
	if err != nil {
		t.Fatal(err)
	}
	a0, _ := p.TakeSnap("")
	b0, _ := p.TakeSnap("")
	a, b := a0.Snap, b0.Snap
	if !(a.Layer < b.Layer) || a.Path == b.Path {
		t.Errorf("snaps in the same instant: %s, %s", a.Layer, b.Layer)
	}
	if FileStamp(a.Layer) != a.Stamp {
		t.Errorf("stamp %s from layer %s", a.Stamp, a.Layer)
	}
}

func TestProductionHidesRefinedDesignerToo(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir(), Production: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{
		`{"id":"ui:category:Inspector","kind":"Category","name":"Inspector","properties":{"label":"Inspector"}}`,
		`{"id":"ui:inspector","kind":"Widget"}`,
	} {
		if _, err := p.AddNode([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	for _, body := range []string{
		`{"kind":"is-a","from":"ui:category:Inspector","to":"ui:category:Designer"}`,
		`{"kind":"is-a","from":"ui:inspector","to":"ui:category:Inspector"}`,
	} {
		if _, err := p.AddEdge([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	env, _ := p.Screen()
	idx := env.NodeIndex()
	for _, id := range []string{"ui:designer", "ui:inspector", "ui:category:Inspector"} {
		if _, ok := idx[id]; ok {
			t.Errorf("%s shown in production", id)
		}
	}
	if _, ok := idx["ui:category:Designer"]; !ok {
		t.Error("the Designer Category itself was hidden")
	}
}

func TestSeedValidatesAndBootsFromSeed(t *testing.T) {
	dir := t.TempDir()
	p, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	if p.Boot() != BootSeed {
		t.Errorf("boot %q, want %q", p.Boot(), BootSeed)
	}
	env, _ := p.Screen()
	if ps := graph.Validate(env); graph.HasErrors(ps) {
		t.Fatalf("seed: %v", ps)
	}
	want := map[string]string{Root: "Container", "ui:layout:root": "Layout", SeedTheme: "Theme", "ui:canvas": "Widget", "ui:designer": "Container",
		"ui:category:Canvas": "Category", "ui:category:Table": "Category", "ui:category:Detail": "Category", "ui:category:Designer": "Category",
		"ui:category:Menu": "Category", SeedMenuEntry: "Container"}
	idx := env.NodeIndex()
	for id, kind := range want {
		if i, ok := idx[id]; !ok || env.Nodes[i].Kind != kind {
			t.Errorf("seed lacks %s (%s)", id, kind)
		}
	}
	if len(env.Nodes) != len(want) {
		t.Errorf("seed has %d nodes, want %d", len(env.Nodes), len(want))
	}
	found := false
	for _, e := range env.Edges {
		found = found || e.ID == graph.EdgeID(Root, SeedTheme, "themed-by")
	}
	if !found {
		t.Error("seed root is not themed-by the seed Theme")
	}
	// Nothing is written until a snap is approved.
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("booting wrote %d entries under artifacts", len(entries))
	}
}

func TestBootsFromTheNewestSnapAndPortalSnapForcesOne(t *testing.T) {
	dir := t.TempDir()
	p, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	first, err := p.TakeSnap("seed")
	if err != nil {
		t.Fatal(err)
	}
	if first.Previous != "" || first.Diff != nil {
		t.Errorf("first snap has a previous: %q", first.Previous)
	}
	if _, _, err := p.PatchNode(SeedTheme, []byte(`{"properties":{"opacity":0.4}}`)); err != nil {
		t.Fatal(err)
	}
	second, err := p.TakeSnap("  more glass  ")
	if err != nil {
		t.Fatal(err)
	}
	if second.Previous != first.Snap.Stamp || second.Note != "more glass" {
		t.Errorf("second: previous %q note %q", second.Previous, second.Note)
	}
	d := second.Diff
	if len(d.ChangedNodes) != 1 || d.ChangedNodes[0].ID != SeedTheme ||
		len(d.AddedNodes)+len(d.AddedEdges)+len(d.RemovedNodes)+len(d.RemovedEdges)+len(d.ChangedEdges) != 0 {
		t.Errorf("diff is not exactly the Theme: %+v", d)
	}
	if f := second.Snap.Env.Findings; len(f) != 1 || f[0].Kind != ApprovedFinding || f[0].Message != "more glass" {
		t.Errorf("snap findings %+v", f)
	}
	// An unapproved change does not outlive the process.
	if _, _, err := p.PatchNode(SeedTheme, []byte(`{"properties":{"opacity":0.9}}`)); err != nil {
		t.Fatal(err)
	}

	q, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	if q.Boot() != second.Snap.Stamp {
		t.Errorf("boot %q, want the newest snap %q", q.Boot(), second.Snap.Stamp)
	}
	env, _ := q.Screen()
	if got := env.Nodes[env.NodeIndex()[SeedTheme]].Properties["opacity"]; fmt.Sprint(got) != "0.4" {
		t.Errorf("booted Theme opacity %v, want 0.4", got)
	}
	if len(env.Findings) != 0 || env.Module != "Portal" || !(env.Layer > second.Snap.Layer) {
		t.Errorf("booted screen: findings %v module %s layer %s", env.Findings, env.Module, env.Layer)
	}

	r, err := New(Options{Artifacts: dir, Snap: first.Snap.Layer})
	if err != nil {
		t.Fatal(err)
	}
	if r.Boot() != first.Snap.Stamp {
		t.Errorf("PORTAL_SNAP boot %q, want %q", r.Boot(), first.Snap.Stamp)
	}
	if _, err := New(Options{Artifacts: dir, Snap: "2000-01-01T00-00-00.000Z"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("a missing PORTAL_SNAP: %v", err)
	}
	if _, err := New(Options{Artifacts: dir, Snap: "../x"}); !errors.Is(err, ErrBadRequest) {
		t.Errorf("a malformed PORTAL_SNAP: %v", err)
	}
}

func TestNoteIsCapped(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.TakeSnap(strings.Repeat("x", maxNote+1)); !errors.Is(err, ErrBadRequest) {
		t.Errorf("long note: %v", err)
	}
}

func TestThemeChecks(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	codes := func(err error) []string {
		var inv *InvalidError
		if !errors.As(err, &inv) {
			return nil
		}
		var out []string
		for _, p := range inv.Problems {
			out = append(out, p.Code)
		}
		return out
	}
	for body, want := range map[string]string{
		`{"id":"ui:theme:x","kind":"Theme","properties":{"color":"orange"}}`: "property-type",
		`{"id":"ui:theme:x","kind":"Theme","properties":{"opacity":1.5}}`:    "property-type",
		`{"id":"ui:theme:x","kind":"Theme","properties":{"font":" "}}`:       "property-type",
		`{"id":"ui:x","kind":"Widget","properties":{"opacity":0.5}}`:         "display",
		`{"id":"ui:x","kind":"Container","properties":{"font":"serif"}}`:     "display",
		`{"id":"ui:theme:x","kind":"Theme","properties":{"collapsed":true}}`: "unknown-property",
	} {
		_, err := p.AddNode([]byte(body))
		if got := codes(err); !slices.Contains(got, want) {
			t.Errorf("%s: codes %v, want %s", body, got, want)
		}
	}
	if _, err := p.AddNode([]byte(`{"id":"ui:theme:x","kind":"Theme","properties":{"color":"#102030","opacity":0,"font":"serif"}}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := p.AddEdge([]byte(`{"kind":"themed-by","from":"ui:canvas","to":"ui:theme:x"}`)); !slices.Contains(codes(err), "edge-endpoints") {
		t.Errorf("themed-by from a Widget: %v", err)
	}
	if _, err := p.AddEdge([]byte(`{"kind":"themed-by","from":"ui:designer","to":"ui:theme:x"}`)); err != nil {
		t.Errorf("themed-by from a Container: %v", err)
	}
}

// Graph.Node 0.2.0 lets a dangling edge to another owner's prefix through as
// a warning; the screen is never merged, so the portal still refuses it.
func TestDanglingEdgeToAnotherOwnerIsStillRefused(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.AddEdge([]byte(`{"kind":"displays","from":"ui:canvas","to":"fs:src"}`))
	var inv *InvalidError
	if !errors.As(err, &inv) {
		t.Fatalf("displays to fs:src accepted: %v", err)
	}
	for _, pr := range inv.Problems {
		if pr.Code == "dangling-edge" && pr.Severity == graph.SeverityError {
			return
		}
	}
	t.Errorf("no dangling-edge error: %v", inv.Problems)
}

// The seed's menu is nodes and edges only: one Container that is-a Menu and
// opens the Canvas, contained by nothing, so the screen tree never draws it.
func TestSeedMenuEntryIsAContainerThatIsAMenuAndOpensTheCanvas(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := p.Screen()
	menu := categoryByName(env, MenuCategory)
	if menu == "" {
		t.Fatal("seed has no Menu Category")
	}
	var entries []string
	opens := map[string]string{}
	for _, e := range env.Edges {
		switch {
		case e.Kind == "is-a" && e.To == menu:
			entries = append(entries, e.From)
		case e.Kind == "opens":
			opens[e.From] = e.To
		case e.Kind == "contains" && e.To == SeedMenuEntry:
			t.Errorf("menu entry is contained by %s", e.From)
		}
	}
	if !slices.Equal(entries, []string{SeedMenuEntry}) {
		t.Fatalf("menu entries %v, want [%s]", entries, SeedMenuEntry)
	}
	n := env.Nodes[env.NodeIndex()[SeedMenuEntry]]
	if n.Kind != "Container" || n.Name != "Graph" {
		t.Errorf("menu entry is %s %q, want Container \"Graph\"", n.Kind, n.Name)
	}
	if opens[SeedMenuEntry] != "ui:canvas" {
		t.Errorf("menu entry opens %q, want ui:canvas", opens[SeedMenuEntry])
	}
	// opens leaves only a Container and ends at a Container or a Widget.
	if _, err := p.AddEdge([]byte(`{"kind":"opens","from":"ui:canvas","to":"ui:root"}`)); err == nil {
		t.Error("opens from a Widget accepted")
	}
	if _, err := p.AddEdge([]byte(`{"kind":"opens","from":"ui:menu:graph","to":"ui:theme:default"}`)); err == nil {
		t.Error("opens to a Theme accepted")
	}
}
