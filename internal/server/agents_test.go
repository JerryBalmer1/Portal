package server

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
)

// fixtureRun is the one Run of the agent-runs fixture (Graph.Emitter.Agent's
// policy-smoke ledger): three steps, four allow and one deny.
const fixtureRun = "agent:run/2026-10-09T02:50:43.7976027Z"

// runsEnvelope is a Graph.Emitter.Agent layer of two runs, the newer one
// first in the file: 2026-10-10 with one step, a pre deny and a post
// witness; 2026-10-08 with no steps.
const runsEnvelope = `{"schema":"graph/1","module":"Graph.Emitter.Agent","version":"0.1.3","layer":"2026-10-10T09:01:00Z","nodes":[
 {"id":"agent:run/2026-10-10T09:00:00Z","kind":"Run","name":"run new","properties":{"startedAt":"2026-10-10T09:00:00Z","stopped":true,"finishedAt":"2026-10-10T09:01:00Z","exit":1}},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1","kind":"Step","properties":{"ordinal":1,"text":"File.delete /work/x"}},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1/action","kind":"Action","properties":{"tool":"Bash"}},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1/pre","kind":"PolicyDecision","name":"deny rules[0] (pre)","properties":{"verdict":"deny","effect":"deny","phase":"pre","rule":"rules[0]","reason":"File.delete denied"}},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1/post","kind":"PolicyDecision","name":"witness default (post)","properties":{"verdict":"allow","effect":"witness","phase":"post","rule":"default"}},
 {"id":"agent:run/2026-10-08T00:00:00Z","kind":"Run","name":"run old","properties":{"startedAt":"2026-10-08T00:00:00Z","stopped":false}}],
"edges":[
 {"id":"agent:run/2026-10-10T09:00:00Z|agent:run/2026-10-10T09:00:00Z/step-1|contains","kind":"contains","from":"agent:run/2026-10-10T09:00:00Z","to":"agent:run/2026-10-10T09:00:00Z/step-1"},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1|agent:run/2026-10-10T09:00:00Z/step-1/action|contains","kind":"contains","from":"agent:run/2026-10-10T09:00:00Z/step-1","to":"agent:run/2026-10-10T09:00:00Z/step-1/action"},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1/action|agent:run/2026-10-10T09:00:00Z/step-1/pre|decided","kind":"decided","from":"agent:run/2026-10-10T09:00:00Z/step-1/action","to":"agent:run/2026-10-10T09:00:00Z/step-1/pre"},
 {"id":"agent:run/2026-10-10T09:00:00Z/step-1/action|agent:run/2026-10-10T09:00:00Z/step-1/post|decided","kind":"decided","from":"agent:run/2026-10-10T09:00:00Z/step-1/action","to":"agent:run/2026-10-10T09:00:00Z/step-1/post"}]}`

// served decodes an agents envelope and checks it as the menu and the
// feeds are checked: Graph.Node, then the portal ontology.
func served(t *testing.T, body []byte) *graph.Envelope {
	t.Helper()
	var env graph.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	if ps := graph.ValidateBytes(body); graph.HasErrors(ps) {
		t.Fatalf("graphnode validate: %+v", ps)
	}
	p, err := store.New(store.Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if ps := p.CheckEnvelope(&env); len(ps) > 0 {
		t.Fatalf("portal ontology: %+v", ps)
	}
	return &env
}

func props(env *graph.Envelope, id string) map[string]any {
	i, ok := env.NodeIndex()[id]
	if !ok {
		return nil
	}
	return env.Nodes[i].Properties
}

func num(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	}
	return -1
}

func TestAgentsGroupRunsByTheMountingPrincipal(t *testing.T) {
	h := newHarness(t)
	s, p, _ := newPortal(t, false)
	// At start: no principal, so dev-bootstrap, which the test tenant's
	// principals.yaml lacks.
	if _, err := p.MountAtStart(store.Mount{Name: "agent-runs", Path: runsDir}); err != nil {
		t.Fatal(err)
	}
	h.doAs(t, contribToken, s.URL, "POST", "/api/tenants/default/layers", `{"name":"contrib-runs","envelope":`+runsEnvelope+`}`, 201)

	env := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/agents", "", 200))
	var principals []string
	for _, n := range env.Nodes {
		if n.Kind == "Principal" {
			principals = append(principals, n.Name)
		}
	}
	if strings.Join(principals, ",") != "contrib,dev-bootstrap" {
		t.Errorf("agents %v, want contrib then dev-bootstrap (tester and reed mounted nothing)", principals)
	}
	if dev := props(env, "ui:principal:dev-bootstrap"); dev["role"] != "admin" || dev["type"] != "user" || dev["created"] != nil {
		t.Errorf("dev-bootstrap, absent from principals.yaml, is served as %v", dev)
	}
	run := props(env, fixtureRun)
	if run["layer"] != "agent-runs" || num(run["steps"]) != 3 || num(run["allow"]) != 4 || num(run["deny"]) != 1 || num(run["witness"]) != 0 {
		t.Errorf("fixture run %v, want layer agent-runs, 3 steps, 4 allow, 1 deny, 0 witness", run)
	}
	if run["startedAt"] != "2026-10-09T02:50:43Z" || run["stopped"] != true {
		t.Errorf("fixture run stamps %v", run)
	}
	for _, e := range []string{
		"ui:principal:dev-bootstrap|" + fixtureRun + "|has-run",
		"ui:principal:contrib|agent:run/2026-10-10T09:00:00Z|has-run",
		"ui:principal:contrib|agent:run/2026-10-08T00:00:00Z|has-run",
	} {
		if !hasEdge(env, e) {
			t.Errorf("no edge %s", e)
		}
	}

	// The same ids as the layer: a Run here is the Run node there.
	layer := map[string]bool{}
	var q graph.Envelope
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/layers/agent-runs/nodes?kind=PolicyDecision", "", 200), &q)
	for _, n := range q.Nodes {
		layer[n.ID] = true
	}

	dev := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/agents/dev-bootstrap/runs", "", 200))
	if dev.Root != "ui:principal:dev-bootstrap" {
		t.Errorf("root %q", dev.Root)
	}
	var decisions int
	for _, n := range dev.Nodes {
		if n.Kind == "Decision" {
			decisions++
			if !layer[n.ID] {
				t.Errorf("Decision %s is not a PolicyDecision id of the layer", n.ID)
			}
			if !hasEdge(dev, fixtureRun+"|"+n.ID+"|has-decision") {
				t.Errorf("no has-decision to %s", n.ID)
			}
		}
	}
	if decisions != 5 {
		t.Errorf("%d decisions, want the layer's 5", decisions)
	}
	if d := props(dev, fixtureRun+"/step-3/pre"); d["effect"] != "deny" || num(d["step"]) != 3 || d["layer"] != "agent-runs" {
		t.Errorf("step-3/pre %v, want the deny of step 3", d)
	}

	mine := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/agents/contrib/runs", "", 200))
	var order []string
	for _, n := range mine.Nodes {
		if n.Kind == "Run" {
			order = append(order, n.ID)
		}
	}
	if strings.Join(order, ",") != "agent:run/2026-10-10T09:00:00Z,agent:run/2026-10-08T00:00:00Z" {
		t.Errorf("runs %v, want newest first", order)
	}
	if r := props(mine, "agent:run/2026-10-10T09:00:00Z"); num(r["steps"]) != 1 || num(r["deny"]) != 1 || num(r["witness"]) != 1 || num(r["exit"]) != 1 {
		t.Errorf("new run %v", r)
	}
	if r := props(mine, "agent:run/2026-10-08T00:00:00Z"); num(r["steps"]) != 0 || r["finishedAt"] != nil {
		t.Errorf("old run %v", r)
	}
	h.do(t, s.URL, "GET", "/api/tenants/default/agents/tester/runs", "", 404)
	h.doAs(t, "", s.URL, "GET", "/default/agents/contrib", "", 200)
	h.doAs(t, "", s.URL, "GET", "/default/agents/tester", "", 404)
}

// A legacy token names no principal, so its layer's runs go to dev-bootstrap;
// a Run id in two layers is served once, from the later mount on a tie.
func TestAgentsLegacyMountAndASharedRunID(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	runs, _ := filepath.Abs(runsDir)
	rj, _ := json.Marshal(runs)
	var l struct{ MountedBy string }
	_ = json.Unmarshal(h.doAs(t, legacyToken, s.URL, "POST", "/api/tenants/default/layers", `{"name":"legacy-runs","path":`+string(rj)+`}`, 201), &l)
	if l.MountedBy != "" {
		t.Errorf("a legacy mount says mountedBy %q", l.MountedBy)
	}
	env := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/agents", "", 200))
	if !hasEdge(env, "ui:principal:dev-bootstrap|"+fixtureRun+"|has-run") {
		t.Error("the legacy mount's run is not dev-bootstrap's")
	}
	_ = json.Unmarshal(h.doAs(t, contribToken, s.URL, "POST", "/api/tenants/default/layers", `{"name":"contrib-copy","path":`+string(rj)+`}`, 201), &l)
	if l.MountedBy != "ui:principal:contrib" {
		t.Errorf("contrib's mount says mountedBy %q", l.MountedBy)
	}
	env = served(t, h.do(t, s.URL, "GET", "/api/tenants/default/agents", "", 200))
	var runsSeen int
	for _, n := range env.Nodes {
		if n.ID == fixtureRun {
			runsSeen++
			if n.Properties["layer"] != "contrib-copy" {
				t.Errorf("the shared run comes from %v, want the later mount", n.Properties["layer"])
			}
		}
	}
	if runsSeen != 1 {
		t.Errorf("run %s served %d times", fixtureRun, runsSeen)
	}
	if _, ok := env.NodeIndex()["ui:principal:dev-bootstrap"]; ok {
		t.Error("dev-bootstrap is listed though it holds no run now")
	}
}

// A path mount records its principal on the Mount node, so the runs keep
// their agent after approve and a restart.
func TestAgentOfAPathMountSurvivesApproveAndRestart(t *testing.T) {
	h := newHarness(t)
	s, _, dir := newPortal(t, false)
	runs, _ := filepath.Abs(runsDir)
	rj, _ := json.Marshal(runs)
	h.doAs(t, contribToken, s.URL, "POST", "/api/tenants/default/layers", `{"name":"agent-runs","path":`+string(rj)+`}`, 201)
	var screen graph.Envelope
	_ = json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/screen", "", 200), &screen)
	if m := props(&screen, store.MountID("agent-runs")); m["mountedBy"] != "ui:principal:contrib" {
		t.Fatalf("Mount node %v, want mountedBy ui:principal:contrib", m)
	}
	snapStamp(t, h, s.URL)
	p, err := store.New(store.Options{Artifacts: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, failed := p.Remount(); len(failed) != 0 {
		t.Fatal(failed)
	}
	r := serve(t, p)
	env := served(t, h.do(t, r.URL, "GET", "/api/tenants/default/agents", "", 200))
	if !hasEdge(env, "ui:principal:contrib|"+fixtureRun+"|has-run") {
		t.Error("after restart the run is not contrib's")
	}
}

// Run and Decision keep agent: ids only in what the API serves; the screen
// takes ui: ids alone.
func TestScreenRefusesABorrowedID(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	var e struct {
		Problems []struct{ Code string }
	}
	_ = json.Unmarshal(h.do(t, s.URL, "POST", "/api/tenants/default/nodes",
		`{"id":"agent:run/x","kind":"Run","properties":{"layer":"l","startedAt":"s","stopped":true,"steps":0,"allow":0,"deny":0,"witness":0}}`, 422), &e)
	var codes []string
	for _, p := range e.Problems {
		codes = append(codes, p.Code)
	}
	if !strings.Contains(strings.Join(codes, ","), "id-prefix") {
		t.Errorf("problems %v, want id-prefix", codes)
	}
}

func hasEdge(env *graph.Envelope, id string) bool {
	for _, e := range env.Edges {
		if e.ID == id {
			return true
		}
	}
	return false
}
