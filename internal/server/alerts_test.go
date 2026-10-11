package server

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// The alert fixture (docs/design.md 47) is hand-made in the shape the brief
// gives Portal.Agent's agent-alerts layer, until the real one lands: five
// alerts, all raised in the agent-runs fixture's run, about a decision
// (step 3's pre deny), two entries of the ledger fixture (46 and 52), the
// run and the environment fixture's compose node. It carries copies of
// ledger:46, ledger:52 and claude-agent-dev, so its about edges do not
// dangle; Portal never serves those copies.
const (
	alertsDir      = "../../tests/fixtures/layers/agent-alerts"
	alertsLayer    = "agent-alerts"
	alertDecision  = "3f9c2a7e5b1d4c08" // critical, open, about step 3's pre deny
	alertLedger52  = "07e3d9a5c1f86b42" // critical, resolved, about ledger:52
	alertRunDenied = "5d27e8f1a9c3b604" // warning, acknowledged, about the run
	alertLedger46  = "a41e09b7c2d85f36" // warning, open, about ledger:46
	alertEnv       = "c8b6f4d2e0a17935" // info, open, about claude-agent-dev
)

// alertOrder is the fixture's alerts as listAlerts sorts them: critical
// first, newest first, then by fingerprint (the two criticals share a
// stamp).
var alertOrder = []string{alertLedger52, alertDecision, alertRunDenied, alertLedger46, alertEnv}

// alertsPortal is a portal with the runs, ledger, tagged environment and
// alert fixtures mounted, in that order.
func alertsPortal(t *testing.T, h *harness) string {
	t.Helper()
	return envPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir},
		[2]string{"agent-environment", envTaggedDir}, [2]string{alertsLayer, alertsDir})
}

// alertsOf decodes an alerts envelope, which must pass Graph.Node.
func alertsOf(t *testing.T, data []byte) *graph.Envelope {
	t.Helper()
	if ps := graph.ValidateBytes(data); graph.HasErrors(ps) {
		t.Fatalf("not a valid envelope: %v", ps)
	}
	var env graph.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	return &env
}

// page is the fingerprints of the Alerts on a listAlerts page, in order.
func page(env *graph.Envelope) []string {
	var out []string
	for _, n := range env.Nodes {
		if n.Kind == "Alert" {
			out = append(out, n.Properties["fingerprint"].(string))
		}
	}
	return out
}

func listAlerts(t *testing.T, h *harness, base, query string) *graph.Envelope {
	t.Helper()
	return alertsOf(t, h.do(t, base, "GET", "/api/tenants/default/alerts"+query, "", 200))
}

// Nothing mounted that holds alerts: an empty list, not a 404, and no
// alerts count on a run.
func TestAlertsAreEmptyWithoutAnAlertLayer(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir})
	env := listAlerts(t, h, base, "")
	root := props(env, alertsID)
	if len(env.Nodes) != 1 || num(root["count"]) != 0 || num(root["alerts"]) != 0 {
		t.Errorf("alerts %+v", env.Nodes)
	}
	if open := root["open"].(map[string]any); num(open["critical"]) != 0 || num(open["warning"]) != 0 || num(open["info"]) != 0 {
		t.Errorf("open %v", open)
	}
	agents := served(t, h.do(t, base, "GET", "/api/tenants/default/agents", "", 200))
	if _, ok := props(agents, fixtureRun)["alerts"]; ok {
		t.Errorf("run %s counts alerts with no alert layer mounted", fixtureRun)
	}
}

// The fixture: five alerts in order, the root's counts and chip values,
// and each alert's subject and run as Portal serves them, from the layers
// that hold them and never from the alert layer's copies.
func TestAlertsOfTheFixture(t *testing.T) {
	h := newHarness(t)
	base := alertsPortal(t, h)
	env := listAlerts(t, h, base, "")
	if env.Root != alertsID || env.Ontology != "" {
		t.Errorf("root %q ontology %q", env.Root, env.Ontology)
	}
	if got := page(env); !slices.Equal(got, alertOrder) {
		t.Errorf("order\n%v\nwant\n%v", got, alertOrder)
	}
	root := props(env, alertsID)
	if num(root["count"]) != 5 || num(root["alerts"]) != 5 || root["next"] != nil {
		t.Errorf("root %v", root)
	}
	open := root["open"].(map[string]any)
	if num(open["critical"]) != 1 || num(open["warning"]) != 1 || num(open["info"]) != 1 {
		t.Errorf("open %v; want one of each: the resolved critical and the acknowledged warning are not open", open)
	}
	if b, _ := json.Marshal([]any{root["rules"], root["states"]}); string(b) != `[["egress-check","image-by-tag","policy-deny","run-denied"],["acknowledged","open","resolved"]]` {
		t.Errorf("rules and states %s", b)
	}
	a := props(env, "alert/"+alertDecision)
	if a["severity"] != "critical" || a["rule"] != "policy-deny" || a["state"] != "open" || a["open"] != true || a["layer"] != alertsLayer ||
		a["about"] != fixtureRun+"/step-3/pre" || a["run"] != fixtureRun || !strings.HasPrefix(a["summary"].(string), "Step 3's pre hook") {
		t.Errorf("alert %v", a)
	}
	var want []string
	for _, fp := range alertOrder {
		want = append(want, "has-alert "+alertsID+" -> alert/"+fp, "raised-in alert/"+fp+" -> "+fixtureRun)
	}
	want = append(want,
		"about alert/"+alertDecision+" -> "+fixtureRun+"/step-3/pre",
		"about alert/"+alertLedger52+" -> ledger:52",
		"about alert/"+alertRunDenied+" -> "+fixtureRun,
		"about alert/"+alertLedger46+" -> ledger:46",
		"about alert/"+alertEnv+" -> "+envCompose)
	slices.Sort(want)
	if got := edgeKinds(env); !slices.Equal(got, want) {
		t.Errorf("edges\n%v\nwant\n%v", got, want)
	}
	idx := env.NodeIndex()
	kind := func(id string) string { return env.Nodes[idx[id]].Kind }
	// The decision as listAgentRuns serves it, the run as listAgents does.
	if d := props(env, fixtureRun+"/step-3/pre"); kind(fixtureRun+"/step-3/pre") != "Decision" || d["effect"] != "deny" || d["layer"] != fixtureRunLayer {
		t.Errorf("decision %s %v", kind(fixtureRun+"/step-3/pre"), d)
	}
	if r := props(env, fixtureRun); kind(fixtureRun) != "Run" || r["layer"] != fixtureRunLayer || num(r["ledger"]) != fixtureLedgerRun || num(r["alerts"]) != 5 {
		t.Errorf("run %s %v", kind(fixtureRun), r)
	}
	// The ledger entry as getLedgerEntry serves it: from the ledger layer,
	// with its hash, which the alert layer's copy lacks.
	if e := props(env, "ledger:52"); kind("ledger:52") != "LedgerEntry" || e["source"] != "agent" || num(e["seq"]) != 52 || len(e["hash"].(string)) != 64 {
		t.Errorf("entry %v", e)
	}
	// The compose node as the environment layer emits it, with its tags.
	if c := props(env, envCompose); kind(envCompose) != "environment.instance" || c["env"] != "dev" || c["host"] != "MSI" {
		t.Errorf("compose %v", c)
	}
	if n := len(env.Nodes); n != 1+5+5 {
		t.Errorf("%d nodes, want the root, five alerts and five ends", n)
	}
}

// One alert, its subject and run; an unknown fingerprint is 404.
func TestGetAlert(t *testing.T) {
	h := newHarness(t)
	base := alertsPortal(t, h)
	env := alertsOf(t, h.do(t, base, "GET", "/api/tenants/default/alerts/"+alertLedger46, "", 200))
	if env.Root != "alert/"+alertLedger46 {
		t.Errorf("root %q", env.Root)
	}
	want := []string{"about alert/" + alertLedger46 + " -> ledger:46", "raised-in alert/" + alertLedger46 + " -> " + fixtureRun}
	if got := edgeKinds(env); !slices.Equal(got, want) {
		t.Errorf("edges %v", got)
	}
	if e := props(env, "ledger:46"); e["event"] != "agent.egress-check" || e["runLayer"] != fixtureRunLayer {
		t.Errorf("entry %v", e)
	}
	body := h.do(t, base, "GET", "/api/tenants/default/alerts/nope", "", 404)
	if !strings.Contains(string(body), "no alert alert/nope in tenant default; its mounted layers hold 5") {
		t.Errorf("404 %s", body)
	}
}

// Every filter, alone, and the pages they make; after is a place in the
// order, found even when its alert has left the filter.
func TestAlertsFiltersAndPages(t *testing.T) {
	h := newHarness(t)
	base := alertsPortal(t, h)
	for query, want := range map[string][]string{
		"?severity=critical":                    {alertLedger52, alertDecision},
		"?severity=info":                        {alertEnv},
		"?rule=policy-deny":                     {alertLedger52, alertDecision},
		"?state=open":                           {alertDecision, alertLedger46, alertEnv},
		"?state=acknowledged":                   {alertRunDenied},
		"?run=" + url.QueryEscape(fixtureRun):   alertOrder,
		"?run=agent:run/none":                   nil,
		"?about=ledger:46":                      {alertLedger46},
		"?about=" + url.QueryEscape(envCompose): {alertEnv},
		"?severity=warning&state=open":          {alertLedger46},
	} {
		env := listAlerts(t, h, base, query)
		if got := page(env); !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v", query, got, want)
		}
		if r := props(env, alertsID); num(r["alerts"]) != len(want) || num(r["count"]) != 5 {
			t.Errorf("%s: root %v", query, r)
		}
	}
	if r := props(listAlerts(t, h, base, "?severity=critical&rule=policy-deny"), alertsID); r["severity"] != "critical" || r["rule"] != "policy-deny" || r["state"] != nil {
		t.Errorf("the filters given are on the root: %v", r)
	}
	var got []string
	query := "?limit=2"
	for i := 0; i < 4; i++ {
		env := listAlerts(t, h, base, query)
		got = append(got, page(env)...)
		next, _ := props(env, alertsID)["next"].(string)
		if next == "" {
			break
		}
		query = "?limit=2&after=" + next
	}
	if !slices.Equal(got, alertOrder) {
		t.Errorf("pages of two: %v", got)
	}
	// The resolved critical is no open alert, but after still places it.
	if got := page(listAlerts(t, h, base, "?state=open&after="+alertLedger52)); !slices.Equal(got, []string{alertDecision, alertLedger46, alertEnv}) {
		t.Errorf("after a filtered-out alert: %v", got)
	}
	h.do(t, base, "GET", "/api/tenants/default/alerts?limit=1001", "", 400)
	h.do(t, base, "GET", "/api/tenants/default/alerts?after=nope", "", 400)
}

// The alert layer, though its stamp is the latest and it holds LedgerEntry
// and environment.instance nodes, is neither the agent's ledger nor an
// environment: those are copies. With no environment layer mounted, the
// compose node is no one's, so its alert has no about edge, though about
// still names it.
func TestAlertLayerIsNotTheLedgerNorAnEnvironment(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir}, [2]string{alertsLayer, alertsDir})
	ledgers := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger", "", 200))
	if a := props(ledgers, "ui:ledger:agent"); a["layer"] != "ledger" || num(a["count"]) != 12 || a["valid"] != true {
		t.Errorf("agent chain %v; want the ledger layer's twelve entries", a)
	}
	if g, _ := environmentGraph(t, h, base); len(g.Nodes) != 0 {
		t.Errorf("environments graph holds %d nodes from the alert layer's copy", len(g.Nodes))
	}
	var nodes graph.Envelope
	if err := json.Unmarshal(h.do(t, base, "GET", "/api/tenants/default/nodes?kind=environment.instance", "", 200), &nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes.Nodes) != 0 {
		t.Errorf("Instances lists the alert layer's copy: %+v", nodes.Nodes)
	}
	if err := json.Unmarshal(h.do(t, base, "GET", "/api/tenants/default/nodes?kind=alert", "", 200), &nodes); err != nil || len(nodes.Nodes) != 5 {
		t.Errorf("a page of Kind alert lists %d, want the five alerts (%v)", len(nodes.Nodes), err)
	}
	env := alertsOf(t, h.do(t, base, "GET", "/api/tenants/default/alerts/"+alertEnv, "", 200))
	if a := props(env, "alert/"+alertEnv); a["about"] != envCompose || len(env.Nodes) != 2 {
		t.Errorf("alert %v, %d nodes: want about named and only the alert and its run served", a, len(env.Nodes))
	}
}

// A re-emit with a later stamp wins: the egress alert resolved in a second
// layer is no longer open, and is served once, from that layer.
func TestAlertReEmitWins(t *testing.T) {
	h := newHarness(t)
	base := alertsPortal(t, h)
	later := `{"name":"agent-alerts-later","envelope":{"schema":"graph/1","module":"Claude.Agent.Alerts","version":"0.2.0","layer":"2026-10-09T03:00:00Z","nodes":[` +
		`{"id":"alert/` + alertLedger46 + `","kind":"alert","properties":{"severity":"warning","rule":"egress-check","summary":"resolved","stamp":"2026-10-09T02:50:44Z","state":"resolved","fingerprint":"` + alertLedger46 + `"}}],"edges":[]}}`
	h.do(t, base, "POST", "/api/tenants/default/layers", later, 201)
	env := listAlerts(t, h, base, "")
	if got := page(env); !slices.Equal(got, alertOrder) {
		t.Errorf("order %v", got)
	}
	a := props(env, "alert/"+alertLedger46)
	if a["state"] != "resolved" || a["open"] != false || a["layer"] != "agent-alerts-later" || a["about"] != nil {
		t.Errorf("re-emitted alert %v", a)
	}
	if open := props(env, alertsID)["open"].(map[string]any); num(open["warning"]) != 0 {
		t.Errorf("open %v", open)
	}
}

// The agents page's run counts the alerts raised in it.
func TestAgentRunsCountTheirAlerts(t *testing.T) {
	h := newHarness(t)
	base := alertsPortal(t, h)
	env := served(t, h.do(t, base, "GET", "/api/tenants/default/agents/"+tester+"/runs", "", 200))
	if r := props(env, fixtureRun); num(r["alerts"]) != 5 || num(r["ledger"]) != fixtureLedgerRun {
		t.Errorf("run %v", r)
	}
}
