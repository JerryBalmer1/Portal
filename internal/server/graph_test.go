package server

import (
	"encoding/json"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// In emittersPortal: the run of the agent-runs fixture (Emitter.Agent's
// layer), which nine records edges of the ledger fixture (Emitter.Ledger's)
// point into; the run the agent-environment fixture's compose hosts, which no
// layer there holds; and the alert of the agent-alerts fixture, which is
// Emitter.Environment's with agent-environment.
const (
	graphRun     = "agent:run/2026-10-09T02:50:43.7976027Z"
	graphRecords = "ledger:46|" + graphRun + "|records"
	graphHosted  = "agent:run/2026-10-11T19:00:22.5540487Z"
	graphAlert   = "alert:e62d4da8120f3a864c3576cd6b3bbb838923641c71b01f9153d41e4973face6c"
)

// closed fails when an edge of env has an end that is not one of its
// nodes, or a node is served twice.
func closed(t *testing.T, what string, env *graph.Envelope) {
	t.Helper()
	ids := map[string]bool{}
	for _, n := range env.Nodes {
		if ids[n.ID] {
			t.Errorf("%s: node %s twice", what, n.ID)
		}
		ids[n.ID] = true
		if n.Stamp == nil || n.Stamp.Layer == "" {
			t.Errorf("%s: node %s has no stamp.layer", what, n.ID)
		}
	}
	for _, e := range env.Edges {
		if !ids[e.From] || !ids[e.To] {
			t.Errorf("%s: edge %s dangles", what, e.ID)
		}
	}
	if env.Counts == nil || env.Counts.Nodes != len(env.Nodes) || env.Counts.Edges != len(env.Edges) {
		t.Errorf("%s: counts %+v", what, env.Counts)
	}
}

// source keeps the nodes of the layers whose source block names the
// emitter, exact and case-sensitive, and an edge only when both its ends
// are kept; the roll call's nodes is that count (docs/design.md 53).
func TestTenantGraphFiltersBySourceEmitter(t *testing.T) {
	h := newHarness(t)
	base, _ := emittersPortal(t, h)
	get := func(q string) *graph.Envelope {
		// Graph.Node only: the envelope keeps its layers' Kinds, which the
		// portal ontology does not hold.
		body := h.do(t, base, "GET", "/api/tenants/default/graph"+q, "", 200)
		var env graph.Envelope
		if err := json.Unmarshal(body, &env); err != nil {
			t.Fatal(err)
		}
		if ps := graph.ValidateBytes(body); graph.HasErrors(ps) {
			t.Fatalf("graph%s: graphnode validate: %+v", q, ps)
		}
		closed(t, "graph"+q, &env)
		return &env
	}

	all := get("")
	if !hasEdge(all, graphRecords) {
		t.Error("the whole graph lacks the ledger's records edges into Emitter.Agent's run")
	}
	if hasEdge(all, "env:compose/claude-agent-dev|ui:principal:dev-bootstrap|runs-as") || hasEdge(all, "env:compose/claude-agent-dev|"+graphHosted+"|hosts") {
		t.Error("an edge to a node no mounted layer holds is served")
	}
	if !has(nodeIDs(all), "fs:src") {
		t.Error("the whole graph lacks the unstamped filesystem layer's nodes")
	}

	// Match, with edge pruning: Emitter.Ledger's twelve entries keep the
	// eleven edges between them; the nine records edges into Emitter.Agent's
	// run, kept in the whole graph, are pruned.
	led := get("?source=Emitter.Ledger")
	if len(led.Nodes) != 12 || len(led.Edges) != 11 {
		t.Errorf("Emitter.Ledger: %d nodes, %d edges, want 12 and 11", len(led.Nodes), len(led.Edges))
	}
	if hasEdge(led, graphRecords) || has(nodeIDs(led), graphRun) {
		t.Error("an edge to another emitter's node survives the filter")
	}
	// Emitter.Environment's two layers: its nine environment nodes and the
	// alert, whose about and raised-in edges lead out of them.
	env := get("?source=Emitter.Environment")
	if len(env.Nodes) != 10 || len(env.Edges) != 9 {
		t.Errorf("Emitter.Environment: %d nodes, %d edges, want 10 and 9", len(env.Nodes), len(env.Edges))
	}
	ids := nodeIDs(env)
	if !has(ids, graphAlert) || !has(ids, "env:compose/claude-agent-dev") || has(ids, "ledger:46") || has(ids, "fs:src") {
		t.Errorf("Emitter.Environment's nodes %v", ids)
	}
	runs := get("?source=Emitter.Agent")
	if len(runs.Nodes) != 12 || len(runs.Edges) != 11 || !has(nodeIDs(runs), graphRun) {
		t.Errorf("Emitter.Agent: %d nodes, %d edges, want 12 and 11", len(runs.Nodes), len(runs.Edges))
	}

	// No match: a card with no layer, a name in the wrong case, an unstamped
	// module (no source block ever matches) and an empty source, which is
	// none at all.
	for _, q := range []string{"Emitter.Terraform", "emitter.environment", "Graph.Emitter.FileSystem", "Emitter"} {
		if e := get("?source=" + q); len(e.Nodes) != 0 || len(e.Edges) != 0 {
			t.Errorf("source %s: %d nodes, %d edges, want none", q, len(e.Nodes), len(e.Edges))
		}
	}
	if e := get("?source="); len(e.Nodes) != len(all.Nodes) || len(e.Edges) != len(all.Edges) {
		t.Errorf("an empty source: %d nodes, want %d", len(e.Nodes), len(all.Nodes))
	}

	roll := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters", "", 200))
	for id, want := range map[string]int{
		cardID("Emitter.Environment", "0.3.0"): 10,
		cardID("Emitter.Agent", "0.1.4"):       12,
		cardID("Emitter.Terraform", "0.17.1"):  0,
		"ui:emitter:Emitter.Unlisted":          len(get("?source=Emitter.Unlisted").Nodes),
		"ui:module:Graph.Emitter.FileSystem":   0,
	} {
		if got := num(props(roll, id)["nodes"]); got != want {
			t.Errorf("%s nodes %d, want %d", id, got, want)
		}
	}
	if one := props(served(t, h.do(t, base, "GET", "/api/tenants/default/emitters/Emitter.Environment", "", 200)), cardID("Emitter.Environment", "0.3.0")); num(one["nodes"]) != 10 {
		t.Errorf("getEmitter's nodes %v", one["nodes"])
	}

	h.do(t, base, "GET", "/default/graph", "", 200)
	h.do(t, base, "GET", "/default/graph?source=Emitter.Environment", "", 200)
	h.do(t, base, "GET", "/other/graph", "", 404)
	h.do(t, base, "GET", "/api/tenants/other/graph?source=Emitter.Environment", "", 404)
}

func nodeIDs(env *graph.Envelope) []string {
	out := make([]string, 0, len(env.Nodes))
	for _, n := range env.Nodes {
		out = append(out, n.ID)
	}
	return out
}
