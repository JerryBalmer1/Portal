package server

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// The alert fixture (docs/design.md 47, 48) is Portal.Agent's alert layer
// from Up on 2026-10-11 (agent/Orchestrator/artifacts/alert-layers), byte
// for byte: one alert under alert:<fingerprint>, info, egress-attempt, open,
// about ledger:303 and raised in a run, neither of which a fixture holds.
const (
	alertsDir    = "../../tests/fixtures/layers/agent-alerts"
	alertsLayer  = "agent-alerts"
	realAlert    = "e62d4da8120f3a864c3576cd6b3bbb838923641c71b01f9153d41e4973face6c"
	realAlertRun = "agent:run/2026-10-11T19:00:22.5540487Z"
)

// The varied alerts, a layer the tests mount inline (variedAlerts), since
// the real layer holds one alert: five alerts under alert:<fingerprint>, the
// fingerprint SHA-256 of the rule and the subject with a newline between,
// all raised in the agent-runs fixture's run. It carries copies of ledger:46,
// ledger:52 and env:compose/claude-agent-dev, which Portal never serves.
const (
	variedLayer    = "agent-alerts-varied"
	alertDecision  = "b9885880ee651752e2b1dfa400d8238803c67f927ff74e8fd774ca0f19fa5027" // critical, open, about step 3's pre deny
	alertLedger52  = "5af632474080efde717583489cc8334ed38b593f90ef18bba42f684edcf92118" // critical, resolved, about ledger:52
	alertRunDenied = "f433234ac4a8ea943e4769832fdb4d8ecd194cfac4920e5c5e71006a259f66ee" // warning, acknowledged, about the run
	alertLedger46  = "ad50168288b7cacc483d2e3d7e56611506c8e51f11d4530d646f87f0586b46f2" // warning, open, about ledger:46
	alertEnv       = "a133d7bec5d0ce29d8fef760b9eea4894a0dc073f170420468d55cd29c33779d" // info, open, about env:compose/claude-agent-dev
)

// alertOrder is the varied alerts as listAlerts sorts them: critical
// first, newest first, then by fingerprint (the two criticals share a
// stamp).
var alertOrder = []string{alertLedger52, alertDecision, alertRunDenied, alertLedger46, alertEnv}

// alertID is an alert's id in its layer.
func alertID(fp string) string { return "alert:" + fp }

// alertLayer is the body of a mount by envelope: layer name, module
// Claude.Agent.Orchestrator, the given nodes and edges.
func alertLayer(t *testing.T, name, stamp string, nodes []map[string]any, edges [][3]string) string {
	t.Helper()
	var es []map[string]any
	for _, e := range edges {
		es = append(es, map[string]any{"id": graph.EdgeID(e[0], e[1], e[2]), "kind": e[2], "from": e[0], "to": e[1]})
	}
	if es == nil {
		es = []map[string]any{}
	}
	b, err := json.Marshal(map[string]any{"name": name, "envelope": map[string]any{"schema": "graph/1", "module": "Claude.Agent.Orchestrator",
		"version": "0.3.0", "layer": stamp, "nodes": nodes, "edges": es}})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// alertNodeOf is a node of Kind alert under id.
func alertNodeOf(id, severity, rule, summary, stamp, state, fp string) map[string]any {
	return map[string]any{"id": id, "kind": "alert", "name": rule + " " + fp[:8], "properties": map[string]any{
		"severity": severity, "rule": rule, "summary": summary, "stamp": stamp, "state": state, "fingerprint": fp}}
}

// variedAlerts is the varied alerts' layer (see alertDecision).
func variedAlerts(t *testing.T) string {
	t.Helper()
	nodes := []map[string]any{
		alertNodeOf(alertID(alertDecision), "critical", "policy-deny", "Step 3's pre hook denied the action the agent asked for.", "2026-10-09T02:50:49Z", "open", alertDecision),
		alertNodeOf(alertID(alertLedger52), "critical", "policy-deny", "Ledger entry 52 records a denied pre decision.", "2026-10-09T02:50:49Z", "resolved", alertLedger52),
		alertNodeOf(alertID(alertLedger46), "warning", "egress-check", "The egress check ran before the run's first step.", "2026-10-09T02:50:44Z", "open", alertLedger46),
		alertNodeOf(alertID(alertRunDenied), "warning", "run-denied", "The run stopped with a denied step.", "2026-10-09T02:50:52Z", "acknowledged", alertRunDenied),
		alertNodeOf(alertID(alertEnv), "info", "image-by-tag", "The compose stack runs the agent image by tag, not by digest.", "2026-10-09T02:50:43Z", "open", alertEnv),
		{"id": "ledger:46", "kind": "LedgerEntry", "name": "46 agent.egress-check", "properties": map[string]any{"seq": 46, "stamp": "2026-10-09T02:50:43Z", "event": "agent.egress-check", "run": fixtureRun}},
		{"id": "ledger:52", "kind": "LedgerEntry", "name": "52 policy.pre", "properties": map[string]any{"seq": 52, "stamp": "2026-10-09T02:50:49Z", "event": "policy.pre", "run": fixtureRun}},
		{"id": envCompose, "kind": "environment.instance", "name": "claude-agent-dev (compose on MSI)", "properties": map[string]any{"kind": "compose", "state": "running", "composeProject": "claude-agent-dev", "env": "dev"}},
	}
	about := map[string]string{alertDecision: fixtureRun + "/step-3/pre", alertLedger52: "ledger:52", alertLedger46: "ledger:46", alertRunDenied: fixtureRun, alertEnv: envCompose}
	var edges [][3]string
	for _, fp := range []string{alertDecision, alertLedger52, alertLedger46, alertRunDenied, alertEnv} {
		edges = append(edges, [3]string{alertID(fp), about[fp], "about"}, [3]string{alertID(fp), fixtureRun, "raised-in"})
	}
	return alertLayer(t, variedLayer, "2026-10-09T02:51:05Z", nodes, edges)
}

// alertsPortal is a portal with the runs, ledger and tagged environment
// fixtures mounted, in that order, and the varied alerts after them.
func alertsPortal(t *testing.T, h *harness) string {
	t.Helper()
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir}, [2]string{"agent-environment", envTaggedDir})
	h.do(t, base, "POST", "/api/tenants/default/layers", variedAlerts(t), 201)
	return base
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

// The real layer alone with the runs and ledger fixtures: its one alert as
// the layer has it. Neither ledger:303 nor its run is in a mounted layer, so
// about and run name them and no edge leads there.
func TestAlertsOfTheRealLayer(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir}, [2]string{alertsLayer, alertsDir})
	env := listAlerts(t, h, base, "")
	if got := page(env); !slices.Equal(got, []string{realAlert}) {
		t.Fatalf("alerts %v, want the real layer's one", got)
	}
	root := props(env, alertsID)
	if num(root["count"]) != 1 || num(root["open"].(map[string]any)["info"]) != 1 {
		t.Errorf("root %v", root)
	}
	a := props(env, alertID(realAlert))
	if a["severity"] != "info" || a["rule"] != "egress-attempt" || a["state"] != "open" || a["open"] != true || a["layer"] != alertsLayer ||
		a["about"] != "ledger:303" || a["run"] != realAlertRun || a["stamp"] != "2026-10-11T19:00:22.5540487Z" {
		t.Errorf("alert %v", a)
	}
	if got := edgeKinds(env); !slices.Equal(got, []string{"has-alert " + alertsID + " -> " + alertID(realAlert)}) {
		t.Errorf("edges %v: about and raised-in lead nowhere Portal serves", got)
	}
	one := alertsOf(t, h.do(t, base, "GET", "/api/tenants/default/alerts/"+realAlert, "", 200))
	if one.Root != alertID(realAlert) || len(one.Nodes) != 1 {
		t.Errorf("getAlert root %q, %d nodes", one.Root, len(one.Nodes))
	}
}

// The varied alerts: five in order, the root's counts and chip values,
// and each alert's subject and run as Portal serves them, from the layers
// that hold them and never from the alert layer's copies.
func TestAlertsOfTheVariedLayer(t *testing.T) {
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
	a := props(env, alertID(alertDecision))
	if a["severity"] != "critical" || a["rule"] != "policy-deny" || a["state"] != "open" || a["open"] != true || a["layer"] != variedLayer ||
		a["about"] != fixtureRun+"/step-3/pre" || a["run"] != fixtureRun || !strings.HasPrefix(a["summary"].(string), "Step 3's pre hook") {
		t.Errorf("alert %v", a)
	}
	var want []string
	for _, fp := range alertOrder {
		want = append(want, "has-alert "+alertsID+" -> "+alertID(fp), "raised-in "+alertID(fp)+" -> "+fixtureRun)
	}
	want = append(want,
		"about "+alertID(alertDecision)+" -> "+fixtureRun+"/step-3/pre",
		"about "+alertID(alertLedger52)+" -> ledger:52",
		"about "+alertID(alertRunDenied)+" -> "+fixtureRun,
		"about "+alertID(alertLedger46)+" -> ledger:46",
		"about "+alertID(alertEnv)+" -> "+envCompose)
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
	if c := props(env, envCompose); kind(envCompose) != "environment.instance" || c["group"] != "stack" || c["host"] != "MSI" {
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
	if env.Root != alertID(alertLedger46) {
		t.Errorf("root %q", env.Root)
	}
	want := []string{"about " + alertID(alertLedger46) + " -> ledger:46", "raised-in " + alertID(alertLedger46) + " -> " + fixtureRun}
	if got := edgeKinds(env); !slices.Equal(got, want) {
		t.Errorf("edges %v", got)
	}
	if e := props(env, "ledger:46"); e["event"] != "agent.egress-check" || e["runLayer"] != fixtureRunLayer {
		t.Errorf("entry %v", e)
	}
	body := h.do(t, base, "GET", "/api/tenants/default/alerts/nope", "", 404)
	if !strings.Contains(string(body), "no alert alert:nope in tenant default; its mounted layers hold 5") {
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
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir})
	h.do(t, base, "POST", "/api/tenants/default/layers", variedAlerts(t), 201)
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
	if a := props(env, alertID(alertEnv)); a["about"] != envCompose || len(env.Nodes) != 2 {
		t.Errorf("alert %v, %d nodes: want about named and only the alert and its run served", a, len(env.Nodes))
	}
}

// A re-emit with a later stamp wins: the egress alert resolved in a second
// layer is no longer open, and is served once, from that layer.
func TestAlertReEmitWins(t *testing.T) {
	h := newHarness(t)
	base := alertsPortal(t, h)
	later := alertLayer(t, "agent-alerts-later", "2026-10-09T03:00:00Z",
		[]map[string]any{alertNodeOf(alertID(alertLedger46), "warning", "egress-check", "resolved", "2026-10-09T02:50:44Z", "resolved", alertLedger46)}, nil)
	h.do(t, base, "POST", "/api/tenants/default/layers", later, 201)
	env := listAlerts(t, h, base, "")
	if got := page(env); !slices.Equal(got, alertOrder) {
		t.Errorf("order %v", got)
	}
	a := props(env, alertID(alertLedger46))
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

// The old form is ignored, not half-read (docs/design.md 48): a layer whose
// alert nodes sit under alert/<fingerprint> mounts, a plain id being no
// prefix's, and Portal shows no alert from it: none listed, none open for
// the badge, no alert page, no alerts row on its subject and no count on its
// run. A fingerprint of 64 hex under alert/ is no alert either.
func TestOldAlertIDsAreIgnored(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{fixtureRunLayer, runsDir})
	old := alertLayer(t, "agent-alerts-old", "2026-10-09T02:51:05Z", []map[string]any{
		alertNodeOf("alert/3f9c2a7e5b1d4c08", "critical", "policy-deny", "Step 3's pre hook denied the action the agent asked for.", "2026-10-09T02:50:49Z", "open", "3f9c2a7e5b1d4c08"),
		alertNodeOf("alert/"+alertDecision, "critical", "policy-deny", "The same, its fingerprint full length.", "2026-10-09T02:50:49Z", "open", alertDecision),
	}, [][3]string{
		{"alert/3f9c2a7e5b1d4c08", fixtureRun + "/step-3/pre", "about"}, {"alert/3f9c2a7e5b1d4c08", fixtureRun, "raised-in"},
		{"alert/" + alertDecision, fixtureRun + "/step-3/pre", "about"}, {"alert/" + alertDecision, fixtureRun, "raised-in"},
	})
	h.do(t, base, "POST", "/api/tenants/default/layers", old, 201)
	env := listAlerts(t, h, base, "")
	root := props(env, alertsID)
	if len(env.Nodes) != 1 || num(root["count"]) != 0 || num(root["alerts"]) != 0 {
		t.Errorf("alerts %+v; want none from alert/ ids", env.Nodes)
	}
	if open := root["open"].(map[string]any); num(open["critical"]) != 0 {
		t.Errorf("open %v; the badge would count an old alert", open)
	}
	for _, fp := range []string{"3f9c2a7e5b1d4c08", alertDecision} {
		h.do(t, base, "GET", "/api/tenants/default/alerts/"+fp, "", 404)
	}
	if got := page(listAlerts(t, h, base, "?about="+url.QueryEscape(fixtureRun+"/step-3/pre"))); len(got) != 0 {
		t.Errorf("the decision's alerts row would show %v", got)
	}
	agents := served(t, h.do(t, base, "GET", "/api/tenants/default/agents", "", 200))
	if _, ok := props(agents, fixtureRun)["alerts"]; ok {
		t.Errorf("run %s counts alerts from alert/ ids", fixtureRun)
	}
}
