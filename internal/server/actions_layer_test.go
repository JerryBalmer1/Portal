package server

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// The tenant-actions layer (docs/design.md 55): the actions fixture's five
// lines, migrated at boot to ledger:tenant/default/<seq>, mounted as a layer
// with prev, request, subject and principal edges.

func tl(seq string) string { return "ledger:tenant/default/" + seq }

// actionsLayerPortal is actionsPortal that also returns the tenant's
// directory, for the Runner's appends.
func actionsLayerPortal(t *testing.T, h *harness, mounts ...[2]string) (string, string, *store.Portal) {
	t.Helper()
	_, p, dir := feedsPortal(t)
	data, err := os.ReadFile(actionsLedger)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, tenant.LedgerFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s := reloadTenant(t, p, dir)
	for _, m := range mounts {
		abs, _ := filepath.Abs(m[1])
		j, _ := json.Marshal(abs)
		h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"`+m[0]+`","path":`+string(j)+`}`, 201)
	}
	return s.URL, dir, p
}

// mountedActions is the tenant-actions layer as mounted: GET
// .../layers/<name>/nodes serves its nodes in a new envelope, without the
// source block and the edges that end in another layer.
func mountedActions(t *testing.T, p *store.Portal) *graph.Envelope {
	t.Helper()
	for _, l := range p.Layers() {
		if l.Name == ActionsLayer {
			return l.Env
		}
	}
	t.Fatalf("no layer %s", ActionsLayer)
	return nil
}

func hasNode(env *graph.Envelope, id string) bool { _, ok := env.NodeIndex()[id]; return ok }

// actionsLayerEnv is the tenant-actions layer as GET .../layers/<name>/nodes
// serves it.
func actionsLayerEnv(t *testing.T, h *harness, base string) *graph.Envelope {
	t.Helper()
	return layerServed(t, h.do(t, base, "GET", "/api/tenants/default/layers/"+ActionsLayer+"/nodes", "", 200))
}

// layerServed decodes what a layer or the query serves and checks it as a
// mount checks a layer: Graph.Node, no error. A layer is not the screen;
// its nodes keep their own modules' Kinds and ids.
func layerServed(t *testing.T, body []byte) *graph.Envelope {
	t.Helper()
	var env graph.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatal(err)
	}
	if ps := graph.ValidateBytes(body); graph.HasErrors(ps) {
		t.Fatalf("graphnode validate: %+v", ps)
	}
	return &env
}

func layerEdgeKinds(env *graph.Envelope) []string {
	seen := map[string]bool{}
	for _, e := range env.Edges {
		seen[e.Kind] = true
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestLedgerIDsAreGraphNodeIDs(t *testing.T) {
	// The lifted Graph.Node (v0.4.2) takes the tenant chain's ids and its
	// root, and still refuses the forms Portal 0.16.1 first tried.
	for id, ok := range map[string]bool{
		tenant.LedgerID("default", 23): true, tenant.LedgerRoot("default"): true, "ledger:23": true,
		"ledger:default/23": false, "ledger:tenant/Default/23": false, "ledger:tenant/default/0": false,
	} {
		if err := graph.ValidID("ledger", id); (err == nil) != ok {
			t.Errorf("ValidID(ledger, %q) = %v, want ok=%v", id, err, ok)
		}
	}
	if got := tenant.LedgerID("default", 23); got != "ledger:tenant/default/23" {
		t.Errorf("LedgerID = %s", got)
	}
	for id, want := range map[string][4]any{
		"ledger:tenant/default/23": {"default", 23, false, true},
		"ledger:23":                {"", 23, true, true},
		"ledger:tenant/default":    {"", 0, false, false},
		"ledger:line-3":            {"", 0, false, false},
		"ledger:default/23":        {"", 0, false, false},
		"ui:ledger:portal/2":       {"", 0, false, false},
	} {
		name, seq, legacy, ok := tenant.LedgerSeq(id)
		if got := [4]any{name, seq, legacy, ok}; got != want {
			t.Errorf("LedgerSeq(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestTenantActionsLayerMountsWithFourEdgeKinds(t *testing.T) {
	h := newHarness(t)
	agentLedger := stamped(t, ledgerDir, LedgerEmitter, "0.3.0", "default")
	base, dir, p := actionsLayerPortal(t, h, [2]string{"agent-environment", envDir}, [2]string{"agent-ledger", agentLedger})
	actionsLayerEnv(t, h, base)
	env := mountedActions(t, p)

	var ids []string
	for _, n := range env.Nodes {
		ids = append(ids, n.ID)
	}
	want := []string{tl("1"), tl("2"), tl("3"), tl("4"), tl("5"), "ui:principal:root"}
	if !slices.Equal(ids, want) {
		t.Errorf("nodes %v, want the five lines and the one principal %v", ids, want)
	}
	if k := layerEdgeKinds(env); !slices.Equal(k, []string{"prev", "principal", "request", "subject"}) {
		t.Errorf("edge kinds %v", k)
	}
	for _, e := range []string{
		tl("2") + "|" + tl("1") + "|prev", tl("5") + "|" + tl("4") + "|prev",
		tl("4") + "|" + tl("1") + "|request", tl("5") + "|" + tl("2") + "|request",
		tl("1") + "|" + agentBox + "|subject", tl("2") + "|env:container/claude-agent-dev/mitm|subject",
		tl("3") + "|ui:principal:root|principal",
	} {
		if !hasEdge(env, e) {
			t.Errorf("no edge %s", e)
		}
	}
	if n := props(env, tl("2")); n["legacyId"] != "ledger:2" || n["event"] != "action.request" || n["outcome"] != "failed" {
		t.Errorf("line 2 %v", n)
	}
	if env.Root != tl("5") || env.Source == nil || *env.Source != (graph.EnvelopeSource{Emitter: LedgerEmitter, Version: "0.3.0", Target: "default", Stamp: env.Layer}) {
		t.Errorf("root %s, source %+v: want the newest line and Emitter.Ledger at the agent ledger layer's version", env.Root, env.Source)
	}
	// Strict: Graph.Node validates the layer (warnings only: the subjects
	// it does not hold), and every ledger: id has the registered shape.
	data, _ := json.Marshal(env)
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Errorf("graphnode validate: %v", ps)
	}
	for _, n := range env.Nodes {
		if strings.HasPrefix(n.ID, "ledger:") {
			if err := graph.ValidID("ledger", n.ID); err != nil {
				t.Errorf("%s: %v", n.ID, err)
			}
		}
	}
	// The agent's chain is still read from the agent ledger layer, never from
	// tenant-actions, which holds ledger: ids too.
	agent := props(served(t, h.do(t, base, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:agent")
	if agent["layer"] != "agent-ledger" || num(agent["count"]) != 12 {
		t.Errorf("agent chain %v", agent)
	}

	// Portal's own append and the Runner's are in the layer on the next read,
	// without a restart; the Runner's legacy request id is joined in the
	// tenant's form.
	h.do(t, base, "POST", actionsPath, ask("logs", agentBox), 201)
	runnerAppends(t, dir, runnerResult(t, dir, "ledger:3", "done", true))
	actionsLayerEnv(t, h, base)
	env = mountedActions(t, p)
	for _, e := range []string{tl("6") + "|" + agentBox + "|subject", tl("6") + "|" + tl("5") + "|prev", tl("7") + "|" + tl("3") + "|request", tl("7") + "|" + tl("6") + "|prev"} {
		if !hasEdge(env, e) {
			t.Errorf("after the appends, no edge %s", e)
		}
	}
	h.do(t, base, "POST", "/api/tenants/default/layers", `{"name":"`+ActionsLayer+`","envelope":`+string(data)+`}`, 400)
}

func TestQueryFromAResultReachesItsRequestSubjectAndPrincipal(t *testing.T) {
	h := newHarness(t)
	base, _, _ := actionsLayerPortal(t, h, [2]string{"agent-environment", envDir})
	q := func(direction string) *graph.Envelope {
		v := url.Values{"from": {tl("5")}, "direction": {direction}, "depth": {"5"}}
		return layerServed(t, h.do(t, base, "GET", "/api/tenants/default/query?"+v.Encode(), "", 200))
	}
	out := q("out")
	for _, id := range []string{tl("5"), tl("2"), "env:container/claude-agent-dev/mitm", "ui:principal:root", tl("4"), tl("1")} {
		if !hasNode(out, id) {
			t.Errorf("out from the result: no %s", id)
		}
	}
	for _, e := range []string{tl("5") + "|" + tl("2") + "|request", tl("2") + "|env:container/claude-agent-dev/mitm|subject", tl("2") + "|ui:principal:root|principal"} {
		if !hasEdge(out, e) {
			t.Errorf("out from the result: no edge %s", e)
		}
	}
	// in walks the edges to a node: the newest line has none, so in from it
	// is the line alone (docs/design.md 55: the row link walks out).
	if in := q("in"); len(in.Nodes) != 1 || in.Nodes[0].ID != tl("5") {
		t.Errorf("in from the newest result: %d nodes", len(in.Nodes))
	}
}

func TestTenantActionsLayerSourceAndTheRollCall(t *testing.T) {
	h := newHarness(t)
	emitter := func(base string) map[string]any {
		env := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters", "", 200))
		for _, n := range env.Nodes {
			if n.Name == LedgerEmitter {
				return n.Properties
			}
		}
		return nil
	}
	// The card pushed and the agent ledger layer at the card's version: both
	// layers under the one card, ok.
	base, _, _ := actionsLayerPortal(t, h, [2]string{"agent-ledger", stamped(t, ledgerDir, LedgerEmitter, "0.3.0", "default")})
	h.doForm(t, testToken, base, "/api/tenants/default/feeds/cards-oci/push", emitterCard(t, LedgerEmitter), 201)
	if e := emitter(base); e == nil || e["status"] != "ok" || num(e["layers"]) != 2 {
		t.Errorf("Emitter.Ledger with the agent ledger layer: %v, want ok with 2 layers", e)
	}
	// No agent ledger layer: the card's version.
	base, _, p := actionsLayerPortal(t, h)
	h.doForm(t, testToken, base, "/api/tenants/default/feeds/cards-oci/push", emitterCard(t, LedgerEmitter), 201)
	if src := mountedActions(t, p).Source; src == nil || src.Version != "0.3.0" {
		t.Errorf("source %+v, want the card's 0.3.0", src)
	}
	if e := emitter(base); e == nil || e["status"] != "ok" || num(e["layers"]) != 1 {
		t.Errorf("Emitter.Ledger with the card only: %v", e)
	}
	// Neither: no source block, so the roll call lists it unstamped under
	// its module, never under a card.
	base, _, p = actionsLayerPortal(t, h)
	if src := mountedActions(t, p).Source; src != nil {
		t.Errorf("source %+v, want none", src)
	}
	if e := emitter(base); e != nil {
		t.Errorf("Emitter.Ledger with neither: %v", e)
	}
}

func TestNoActionLineNoTenantActionsLayer(t *testing.T) {
	h := newHarness(t)
	s, _, _ := feedsPortal(t)
	h.do(t, s.URL, "GET", "/api/tenants/default/layers/"+ActionsLayer+"/nodes", "", 404)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", emitterCard(t, LedgerEmitter), 201)
	h.do(t, s.URL, "GET", "/api/tenants/default/layers/"+ActionsLayer+"/nodes", "", 404)
	h.do(t, s.URL, "POST", actionsPath, ask("logs", agentBox), 201)
	env := actionsLayerEnv(t, h, s.URL)
	if len(env.Nodes) != 3 || !hasEdge(env, tl("2")+"|"+tl("1")+"|prev") {
		t.Errorf("after the first ask: %d nodes, want the push, the ask and its principal", len(env.Nodes))
	}
}
