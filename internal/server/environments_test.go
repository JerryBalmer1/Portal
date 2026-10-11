package server

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// The environment fixtures (docs/design.md 46). agent-environment is
// Portal.Agent's agent-environment layer from Up on 2026-10-11, byte for
// byte: a compose node, its three containers (member-of), the principal it
// ran as (runs-as) and the run it hosted (hosts), which no fixture holds.
// agent-environment-tagged is the same with env, group and tags on three of
// its four nodes and hosts pointing at the agent-runs fixture's run.
const (
	envDir       = "../../tests/fixtures/layers/agent-environment"
	envTaggedDir = "../../tests/fixtures/layers/agent-environment-tagged"
	envCompose   = "claude-agent-dev"
	envRealRun   = "agent:run/2026-10-11T10:08:49.8679152Z"
	devPrincipal = "ui:principal:dev-bootstrap"
)

var envContainers = []string{"claude-agent-dev/agent", "claude-agent-dev/ledger", "claude-agent-dev/mitm"}

// envPortal is a portal with the FileSystem fixture and the named layers
// mounted by path, in that order.
func envPortal(t *testing.T, h *harness, mounts ...[2]string) string {
	t.Helper()
	s, _, _ := newPortal(t, false)
	for _, m := range mounts {
		abs, _ := filepath.Abs(m[1])
		j, _ := json.Marshal(abs)
		h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"`+m[0]+`","path":`+string(j)+`}`, 201)
	}
	return s.URL
}

func environmentGraph(t *testing.T, h *harness, base string) (*graph.Envelope, []byte) {
	t.Helper()
	data := h.do(t, base, "GET", "/api/tenants/default/environments/graph", "", 200)
	var env graph.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	return &env, data
}

func edgeKinds(env *graph.Envelope) []string {
	var out []string
	for _, e := range env.Edges {
		out = append(out, e.Kind+" "+e.From+" -> "+e.To)
	}
	sort.Strings(out)
	return out
}

// Nothing mounted that is an environment: an empty graph, not a 404.
func TestEnvironmentGraphIsEmptyWithoutAnEnvironmentLayer(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir})
	env, _ := environmentGraph(t, h, base)
	if len(env.Nodes) != 0 || len(env.Edges) != 0 {
		t.Errorf("graph %d nodes, %d edges; want none", len(env.Nodes), len(env.Edges))
	}
}

// The real layer alone: its four environment.instance nodes as the layer
// has them, the principal one runs-as edge away, and no hosts edge, since
// no mounted layer holds the run it names.
func TestEnvironmentGraphOfTheRealLayer(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{"agent-environment", envDir})
	env, data := environmentGraph(t, h, base)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Fatalf("not a valid envelope: %v", ps)
	}
	var ids []string
	for _, n := range env.Nodes {
		ids = append(ids, n.Kind+" "+n.ID)
	}
	want := []string{"environment.instance " + envCompose}
	for _, c := range envContainers {
		want = append(want, "environment.instance "+c)
	}
	want = append(want, "Principal "+devPrincipal)
	if !slices.Equal(ids, want) {
		t.Errorf("nodes\n%v\nwant\n%v", ids, want)
	}
	idx := env.NodeIndex()
	compose := env.Nodes[idx[envCompose]]
	if compose.Properties["kind"] != "compose" || compose.Properties["state"] != "running" || compose.Stamp == nil || compose.Stamp.Layer != "2026-10-11T10:08:56Z" {
		t.Errorf("compose node %+v: its layer's properties and stamp.layer", compose)
	}
	for _, c := range envContainers {
		if k := env.Nodes[idx[c]].Properties["kind"]; k != "container" {
			t.Errorf("%s kind %v, want container", c, k)
		}
	}
	wantEdges := []string{
		"member-of claude-agent-dev/agent -> " + envCompose,
		"member-of claude-agent-dev/ledger -> " + envCompose,
		"member-of claude-agent-dev/mitm -> " + envCompose,
		"runs-as " + envCompose + " -> " + devPrincipal,
	}
	if got := edgeKinds(env); !slices.Equal(got, wantEdges) {
		t.Errorf("edges\n%v\nwant\n%v", got, wantEdges)
	}
	if strings.Contains(string(data), envRealRun) {
		t.Errorf("the hosts edge to %s is served, but no mounted layer holds that run", envRealRun)
	}
	// dev-bootstrap is made up, as the agents page does, when principals.yaml lacks it.
	if p := env.Nodes[idx[devPrincipal]]; p.Name != "dev-bootstrap" || p.Properties["role"] != "admin" {
		t.Errorf("principal %+v", p)
	}
}

// With the runs and ledger fixtures mounted, the hosts edge lands on the
// run, served as listAgents has it (its layer and ledger count), and goes
// no further: none of the run's Steps, Decisions or ledger entries come
// along. env, group and tags are read from the nodes as they are.
func TestEnvironmentGraphOneHopNeighbours(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir}, [2]string{"agent-environment", envTaggedDir})
	env, data := environmentGraph(t, h, base)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Fatalf("not a valid envelope: %v", ps)
	}
	if len(env.Nodes) != 6 {
		var ids []string
		for _, n := range env.Nodes {
			ids = append(ids, n.ID)
		}
		t.Fatalf("nodes %v, want the four instances, the principal and the run", ids)
	}
	idx := env.NodeIndex()
	run, ok := idx[fixtureRun]
	if !ok {
		t.Fatalf("the run %s is not served", fixtureRun)
	}
	r := env.Nodes[run]
	if r.Kind != "Run" || r.Properties["layer"] != fixtureRunLayer || r.Properties["ledger"] != float64(fixtureLedgerRun) {
		t.Errorf("run %+v: kind Run, layer %s, ledger %d", r, fixtureRunLayer, fixtureLedgerRun)
	}
	for _, n := range env.Nodes {
		if strings.HasPrefix(n.ID, fixtureRun+"/") || strings.HasPrefix(n.ID, "ledger:") {
			t.Errorf("%s (%s) is two hops away", n.ID, n.Kind)
		}
	}
	kinds := map[string]int{}
	for _, e := range env.Edges {
		kinds[e.Kind]++
	}
	if kinds["member-of"] != 3 || kinds["runs-as"] != 1 || kinds["hosts"] != 1 || len(kinds) != 3 {
		t.Errorf("edge kinds %v, want member-of 3, runs-as 1, hosts 1", kinds)
	}
	if got := edgeKinds(env); !slices.Contains(got, "hosts "+envCompose+" -> "+fixtureRun) {
		t.Errorf("edges %v lack hosts to the run", got)
	}
	c := env.Nodes[idx[envCompose]].Properties
	if c["env"] != "dev" || c["group"] != "stack" || len(c["tags"].([]any)) != 2 {
		t.Errorf("compose fields %v: env dev, group stack, two tags", c)
	}
	if l := env.Nodes[idx["claude-agent-dev/ledger"]].Properties; l["env"] != nil || l["tags"] != nil {
		t.Errorf("ledger keeper %v: carries no env or tags, and Portal infers none", l)
	}
}

// The real layer and its tagged copy hold the same ids: the first mounted
// wins, as listNodes does, and each edge is served once.
func TestEnvironmentGraphFirstLayerWins(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{"agent-environment", envDir}, [2]string{"agent-environment-tagged", envTaggedDir})
	env, _ := environmentGraph(t, h, base)
	idx := env.NodeIndex()
	if v := env.Nodes[idx[envCompose]].Properties["env"]; v != nil {
		t.Errorf("compose env %v: the first mount, the real layer, carries none", v)
	}
	seen := map[string]bool{}
	for _, e := range env.Edges {
		if seen[e.ID] {
			t.Errorf("edge %s twice", e.ID)
		}
		seen[e.ID] = true
	}
}

// A layer mounted with a token that names no principal (from before 0.5.0)
// hangs its runs off dev-bootstrap, the principal the environment ran as:
// the graph joins the two by has-run, which the page's Details and Runs
// links for a principal need, and adds no node for it.
func TestEnvironmentGraphJoinsThePrincipalToTheRunsItHolds(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	for _, m := range [][3]string{{legacyToken, fixtureRunLayer, runsDir}, {testToken, "agent-environment", envTaggedDir}} {
		abs, _ := filepath.Abs(m[2])
		j, _ := json.Marshal(abs)
		h.doAs(t, m[0], s.URL, "POST", "/api/tenants/default/layers", `{"name":"`+m[1]+`","path":`+string(j)+`}`, 201)
	}
	env, data := environmentGraph(t, h, s.URL)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Fatalf("not a valid envelope: %v", ps)
	}
	if got := edgeKinds(env); !slices.Contains(got, "has-run "+devPrincipal+" -> "+fixtureRun) {
		t.Errorf("edges %v lack has-run from dev-bootstrap to the run", got)
	}
	if len(env.Nodes) != 6 {
		t.Errorf("%d nodes, want 6: has-run adds none", len(env.Nodes))
	}
}
