package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		"ui:category:Menu": "Category", SeedMenuEntry: "Container", SeedRunsCanvas: "Widget", SeedRunsEntry: "Container"}
	for _, sec := range DesignerSections {
		want[SectionStateID(SeedDesigner, sec.Slug)] = "State"
	}
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

// The seed's menu is nodes and edges only: Containers that are-a Menu, each
// opening a Canvas, contained by nothing, so the screen tree never draws them.
// Graph opens the filesystem Canvas, Runs the agent-runs one (docs/design.md 35).
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
		case e.Kind == "contains" && (e.To == SeedMenuEntry || e.To == SeedRunsEntry):
			t.Errorf("menu entry %s is contained by %s", e.To, e.From)
		}
	}
	if !slices.Equal(entries, []string{SeedMenuEntry, SeedRunsEntry}) {
		t.Fatalf("menu entries %v, want [%s %s]", entries, SeedMenuEntry, SeedRunsEntry)
	}
	idx := env.NodeIndex()
	for _, w := range []struct {
		id, name, opens, source string
		order                   int64
	}{{SeedMenuEntry, "Graph", "ui:canvas", "filesystem", 0}, {SeedRunsEntry, "Runs", SeedRunsCanvas, RunsLayer, 1}} {
		n := env.Nodes[idx[w.id]]
		if n.Kind != "Container" || n.Name != w.name {
			t.Errorf("menu entry %s is %s %q, want Container %q", w.id, n.Kind, n.Name, w.name)
		}
		if fmt.Sprint(n.Properties["order"]) != fmt.Sprint(w.order) {
			t.Errorf("menu entry %s order %v, want %d", w.id, n.Properties["order"], w.order)
		}
		if opens[w.id] != w.opens {
			t.Errorf("menu entry %s opens %q, want %s", w.id, opens[w.id], w.opens)
		}
		if c := env.Nodes[idx[w.opens]]; c.Kind != "Widget" || c.Properties["type"] != "Canvas" || c.Properties["source"] != w.source {
			t.Errorf("%s is %s %v, want a Canvas Widget on %s", w.opens, c.Kind, c.Properties, w.source)
		}
	}
	// opens leaves only a Container and ends at a Container or a Widget.
	if _, err := p.AddEdge([]byte(`{"kind":"opens","from":"ui:canvas","to":"ui:root"}`)); err == nil {
		t.Error("opens from a Widget accepted")
	}
	if _, err := p.AddEdge([]byte(`{"kind":"opens","from":"ui:menu:graph","to":"ui:theme:default"}`)); err == nil {
		t.Error("opens to a Theme accepted")
	}
}

// sectionStates returns collapsed per section of the seed Designer, and
// fails on a section whose State is missing or not joined by has-state.
func sectionStates(t *testing.T, env *graph.Envelope) map[string]bool {
	t.Helper()
	joined := map[string]bool{}
	for _, e := range env.Edges {
		if e.Kind == "has-state" && e.From == SeedDesigner {
			joined[e.To] = true
		}
	}
	idx := env.NodeIndex()
	out := map[string]bool{}
	for _, sec := range DesignerSections {
		sid := SectionStateID(SeedDesigner, sec.Slug)
		i, ok := idx[sid]
		if !ok || env.Nodes[i].Kind != "State" || !joined[sid] {
			t.Fatalf("no State %s joined from %s", sid, SeedDesigner)
		}
		c, ok := env.Nodes[i].Properties["collapsed"].(bool)
		if !ok {
			t.Fatalf("%s collapsed is %v", sid, env.Nodes[i].Properties["collapsed"])
		}
		out[sec.Slug] = c
	}
	return out
}

// The seed gives the Designer one State per section, ui:state:designer:<slug>:
// Approve and Categories open, the other five collapsed.
func TestSeedGivesEachDesignerSectionAState(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := p.Screen()
	got := sectionStates(t, env)
	want := map[string]bool{"approve": false, "kinds": true, "categories": false, "add-panel": true,
		"add-container": true, "add-category": true, "add-menu-entry": true}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("seeded collapsed %v, want %v", got, want)
	}
	if SectionStateID(SeedDesigner, "add-panel") != "ui:state:designer:add-panel" {
		t.Errorf("section State id %s", SectionStateID(SeedDesigner, "add-panel"))
	}
}

// A section toggled through PATCH survives approve and restart.
func TestSectionStateRoundTripsThroughSnap(t *testing.T) {
	dir := t.TempDir()
	p, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	for id, body := range map[string]string{
		"ui:state:designer:kinds":   `{"properties":{"collapsed":false}}`,
		"ui:state:designer:approve": `{"properties":{"collapsed":true}}`,
	} {
		if _, _, err := p.PatchNode(id, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	snap, err := p.TakeSnap("sections")
	if err != nil {
		t.Fatal(err)
	}
	if got := sectionStates(t, snap.Snap.Env); got["kinds"] || !got["approve"] {
		t.Errorf("snap holds %v", got)
	}
	q, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := q.Screen()
	if got := sectionStates(t, env); got["kinds"] || !got["approve"] || got["categories"] || !got["add-panel"] {
		t.Errorf("restart holds %v", got)
	}
	// Reboot adds nothing: the snap had every section.
	if len(env.Nodes) != len(snap.Snap.Env.Nodes) || len(env.Edges) != len(snap.Snap.Env.Edges) {
		t.Errorf("restart has %d nodes %d edges, snap %d %d", len(env.Nodes), len(env.Edges), len(snap.Snap.Env.Nodes), len(snap.Snap.Env.Edges))
	}
}

// A snap from before section States (0.3.0) boots with the defaults; one
// without the seed Designer gets none.
func TestSnapWithoutSectionStatesBootsWithTheDefaults(t *testing.T) {
	dir := t.TempDir()
	p, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	for _, sec := range DesignerSections {
		if _, _, err := p.DeleteNode(SectionStateID(SeedDesigner, sec.Slug)); err != nil {
			t.Fatal(err)
		}
	}
	old, err := p.TakeSnap("0.3.0 shape")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustRead(t, old.Snap.Path)), "ui:state:designer:") {
		t.Fatal("the old-shape snap still holds section States")
	}
	q, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	env, _ := q.Screen()
	if got := sectionStates(t, env); !got["kinds"] || got["approve"] || got["categories"] {
		t.Errorf("defaults %v", got)
	}
	if ps := graph.Validate(env); graph.HasErrors(ps) {
		t.Errorf("booted screen: %v", ps)
	}

	if _, _, err := q.DeleteNode(SeedDesigner); err != nil {
		t.Fatal(err)
	}
	for _, sec := range DesignerSections {
		if _, _, err := q.DeleteNode(SectionStateID(SeedDesigner, sec.Slug)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := q.TakeSnap("no designer"); err != nil {
		t.Fatal(err)
	}
	r, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	env, _ = r.Screen()
	if strings.Contains(fmt.Sprint(env.Nodes), "ui:state:designer:") {
		t.Error("section States added with no Designer on screen")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fixtureLayers is the FileSystem fixture's directory, as the server tests
// mount it.
const fixtureLayers = "../../tests/fixtures/layers/filesystem"

func mountNode(t *testing.T, p *Portal, name string) (graph.Node, bool) {
	t.Helper()
	env, err := p.Screen()
	if err != nil {
		t.Fatal(err)
	}
	i, ok := env.NodeIndex()[MountID(name)]
	if !ok {
		return graph.Node{}, false
	}
	return env.Nodes[i], true
}

// A mount by path is a Mount node on screen; an approved snap brings the
// layer back at boot (docs/design.md 33).
func TestPathMountIsRecordedAndRemountedAfterApprove(t *testing.T) {
	dir := t.TempDir()
	p, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.MountLayer(Mount{Name: "runs", Path: fixtureLayers}); err != nil {
		t.Fatal(err)
	}
	n, ok := mountNode(t, p, "runs")
	abs, _ := filepath.Abs(fixtureLayers)
	if !ok || n.Kind != MountKind || n.Properties["layer"] != "runs" || n.Properties["path"] != abs {
		t.Fatalf("Mount node %+v (on screen %v), want path %s", n, ok, abs)
	}
	// The same path again changes nothing on screen, stamp included.
	before, _ := p.Screen()
	if _, err := p.MountLayer(Mount{Name: "runs", Path: fixtureLayers}); err != nil {
		t.Fatal(err)
	}
	if after, _ := p.Screen(); after.Layer != before.Layer || len(after.Nodes) != len(before.Nodes) {
		t.Errorf("remounting the same path moved the screen: %s %d -> %s %d", before.Layer, len(before.Nodes), after.Layer, len(after.Nodes))
	}
	if _, err := p.TakeSnap("runs"); err != nil {
		t.Fatal(err)
	}
	q, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Layers()) != 0 {
		t.Fatalf("New mounted %d layers; Remount is the caller's", len(q.Layers()))
	}
	mounted, failed := q.Remount()
	if len(failed) != 0 || len(mounted) != 1 || mounted[0].Name != "runs" || len(mounted[0].Env.Nodes) == 0 {
		t.Fatalf("Remount: mounted %v, failed %v", mounted, failed)
	}
	// Remount and MountAtStart never write the screen.
	if _, ok := mountNode(t, q, "filesystem"); ok {
		t.Error("before any flag mount, the screen has ui:mount:filesystem")
	}
	if _, err := q.MountAtStart(Mount{Name: "filesystem", Path: fixtureLayers}); err != nil {
		t.Fatal(err)
	}
	if _, ok := mountNode(t, q, "filesystem"); ok {
		t.Error("a flag mount was recorded on screen")
	}
}

// An inline mount cannot be read again, so it removes the name's Mount node.
func TestInlineMountRemovesTheMountNode(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.MountLayer(Mount{Name: "runs", Path: fixtureLayers}); err != nil {
		t.Fatal(err)
	}
	env := `{"schema":"graph/1","module":"M","layer":"2026-10-09T00:00:00Z","nodes":[{"id":"a","kind":"K"}],"edges":[]}`
	if _, err := p.MountLayer(Mount{Name: "runs", Envelope: []byte(env)}); err != nil {
		t.Fatal(err)
	}
	if n, ok := mountNode(t, p, "runs"); ok {
		t.Errorf("inline mount left %+v", n)
	}
}

// A recorded layer that is gone is reported and skipped; the boot goes on.
func TestRemountOfAMissingPathFailsThatLayerOnly(t *testing.T) {
	dir := t.TempDir()
	p, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "layers")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(fixtureLayers, "2026-10-09T00-51-29Z.graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gone, "2026-10-09T00-51-29Z.graph.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for name, path := range map[string]string{"gone": gone, "runs": fixtureLayers} {
		if _, err := p.MountLayer(Mount{Name: name, Path: path}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := p.TakeSnap(""); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	q, err := New(Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	mounted, failed := q.Remount()
	if len(mounted) != 1 || mounted[0].Name != "runs" || len(failed) != 1 || !strings.Contains(failed[0].Error(), "ui:mount:gone") {
		t.Errorf("Remount: mounted %v, failed %v", mounted, failed)
	}
	if _, ok := mountNode(t, q, "gone"); !ok {
		t.Error("a Mount node whose layer is gone was dropped from the screen")
	}
}

// A Mount node written by hand is checked: its layer is a layer name and its
// id follows from it.
func TestMountNodeChecks(t *testing.T) {
	p, err := New(Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for body, want := range map[string]string{
		`{"id":"ui:mount:x","kind":"Mount","name":"x","properties":{"layer":"x"}}`:                              "property-type",
		`{"id":"ui:mount:x","kind":"Mount","name":"x","properties":{"layer":"Bad Name","path":"p"}}`:            "property-type",
		`{"id":"ui:mount:x","kind":"Mount","name":"x","properties":{"layer":"y","path":"p"}}`:                   "property-type",
		`{"id":"ui:mount:x","kind":"Mount","name":"x","properties":{"layer":"x","path":"p","color":"#ffffff"}}`: "display",
	} {
		_, err := p.AddNode([]byte(body))
		var inv *InvalidError
		if !errors.As(err, &inv) || !slices.ContainsFunc(inv.Problems, func(q graph.Problem) bool { return q.Code == want }) {
			t.Errorf("%s: %v, want %s", body, err, want)
		}
	}
	if _, err := p.AddNode([]byte(`{"id":"ui:mount:x","kind":"Mount","name":"x","properties":{"layer":"x","path":"p"}}`)); err != nil {
		t.Errorf("a well-formed Mount node was refused: %v", err)
	}
}
