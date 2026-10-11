package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// Actions (docs/design.md 50). The fixture ledger.jsonl holds three
// requests as root: 1 logs on the agent container, 2 restart on mitm, 3
// stop on the ledger keeper. The ledger layer fixture answers 1 (entry 56,
// done) and 2 (entry 57, failed, exit 1), and leaves 3 the gap.
const (
	actionsLedger = "../../tests/fixtures/ledger/actions.ledger.jsonl"
	agentBox      = "env:container/claude-agent-dev/agent"
	actionsPath   = "/api/tenants/default/actions"
)

// actionsPortal is a portal whose tenant's ledger.jsonl is the actions
// fixture, with the given layers mounted in order.
func actionsPortal(t *testing.T, h *harness, mounts ...[2]string) string {
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
	return s.URL
}

func ask(action, subject string) string {
	b, _ := json.Marshal(map[string]string{"action": action, "subject": subject})
	return string(b)
}

func TestPostActionValidatesAndTakesThePrincipalFromTheToken(t *testing.T) {
	h := newHarness(t)
	s, _, dir := feedsPortal(t)
	good := ask("logs", agentBox)
	h.doAs(t, "", s.URL, "POST", actionsPath, good, 401)
	h.doAs(t, readerToken, s.URL, "POST", actionsPath, good, 403)
	// A token from before 0.5.0 belongs to no principal: no one to ask as.
	h.doAs(t, legacyToken, s.URL, "POST", actionsPath, good, 403)
	for _, bad := range []string{
		ask("nuke", agentBox), ask("Logs", agentBox), ask("", agentBox),
		ask("stop", "env:compose/claude-agent-dev"), ask("stop", "env:image/claude-agent:dev"),
		ask("stop", "claude-agent-dev/agent"), ask("stop", "env:container/claude-agent-dev"), ask("stop", ""),
	} {
		h.do(t, s.URL, "POST", actionsPath, bad, 422)
	}
	// The body never names who asks: a principal in it is refused, not used.
	h.doAs(t, contribToken, s.URL, "POST", actionsPath, `{"action":"logs","subject":"`+agentBox+`","principal":"ui:principal:tester"}`, 400)
	h.do(t, s.URL, "POST", actionsPath, `not json`, 400)
	if _, err := os.Stat(filepath.Join(dir, tenant.LedgerFile)); !os.IsNotExist(err) {
		t.Fatalf("a refused ask wrote the ledger: %v", err)
	}

	var a, b tenant.ActionRequest
	if err := json.Unmarshal(h.doAs(t, contribToken, s.URL, "POST", actionsPath, good, 201), &a); err != nil {
		t.Fatal(err)
	}
	if a.Kind != "action.request" || a.ID != "ledger:1" || a.Action != "logs" || a.Subject != agentBox || a.Principal != "ui:principal:contrib" ||
		a.Status != "requested" || a.Seq != 1 || a.Prev != store.ZeroHash || a.Requested == "" {
		t.Errorf("first ask %+v", a)
	}
	if err := json.Unmarshal(h.do(t, s.URL, "POST", actionsPath, ask("restart", "env:container/claude-agent-dev/mitm"), 201), &b); err != nil {
		t.Fatal(err)
	}
	if b.ID != "ledger:2" || b.Principal != "ui:principal:"+tester || b.Seq != 2 {
		t.Errorf("second ask %+v", b)
	}
	// The file holds the lines as returned, member for member in the
	// contract's order, chained.
	data, err := os.ReadFile(filepath.Join(dir, tenant.LedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], `{"kind":"action.request","id":"ledger:1","action":"logs","subject":"`+agentBox+`","principal":"ui:principal:contrib","requested":"`) ||
		!strings.HasSuffix(lines[0], `","status":"requested","seq":1,"prev":"`+store.ZeroHash+`"}`) {
		t.Fatalf("ledger.jsonl %q", lines)
	}
	if want := tenantLineHash(lines[0]); b.Prev != want {
		t.Errorf("line 2's prev %s, want the hash of line 1 %s", b.Prev, want)
	}
	c := props(served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if c["valid"] != true || num(c["count"]) != 2 {
		t.Errorf("portal chain %v", c)
	}
	// No ledger layer: both asked, neither answered.
	env := served(t, h.do(t, s.URL, "GET", actionsPath, "", 200))
	r := props(env, ActionsID)
	if num(r["count"]) != 2 || num(r["requested"]) != 2 || num(r["done"]) != 0 || num(r["failed"]) != 0 {
		t.Errorf("actions root %v", r)
	}
	if q := props(env, "ui:ledger:portal/1"); q["principal"] != "ui:principal:contrib" || q["by"] != "contrib" || q["outcome"] != "requested" || q["lineId"] != "ledger:1" || q["resultId"] != nil {
		t.Errorf("request 1 %v", q)
	}
}

// tenantLineHash is a line's SHA-256, the next line's prev.
func tenantLineHash(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:])
}

// TestActionsJoinTheirResults: requests from Portal's chain, results from
// the ledger layer, newest first, each with its result; the gap stays
// requested.
func TestActionsJoinTheirResults(t *testing.T) {
	h := newHarness(t)
	base := actionsPortal(t, h, [2]string{fixtureRunLayer, runsDir}, [2]string{"ledger", ledgerDir})
	env := served(t, h.do(t, base, "GET", actionsPath, "", 200))
	if env.Root != ActionsID {
		t.Fatalf("root %s", env.Root)
	}
	r := props(env, ActionsID)
	if num(r["count"]) != 3 || num(r["requested"]) != 1 || num(r["done"]) != 1 || num(r["failed"]) != 1 {
		t.Errorf("root %v, want 3: 1 requested, 1 done, 1 failed", r)
	}
	var order []string
	for _, e := range env.Edges {
		if e.Kind == "has-action" {
			order = append(order, e.To)
		}
	}
	if strings.Join(order, ",") != "ui:ledger:portal/3,ui:ledger:portal/2,ui:ledger:portal/1" {
		t.Errorf("requests %v, want newest first", order)
	}
	want := map[string][3]any{ // outcome, resultId, resultSeq
		"ui:ledger:portal/1": {"done", "ledger:56", 56},
		"ui:ledger:portal/2": {"failed", "ledger:57", 57},
		"ui:ledger:portal/3": {"requested", nil, -1},
	}
	for id, w := range want {
		q := props(env, id)
		if q["event"] != "action.request" || q["outcome"] != w[0] || q["resultId"] != w[1] || num(q["resultSeq"]) != w[2] || q["status"] != "requested" || q["principal"] != "ui:principal:root" {
			t.Errorf("%s %v, want %v", id, q, w)
		}
	}
	res := props(env, "ledger:57")
	if res["event"] != "action.result" || res["request"] != "ledger:2" || num(res["requestSeq"]) != 2 || res["status"] != "failed" || num(res["exit"]) != 1 ||
		!strings.HasPrefix(res["output"].(string), "Error response from daemon") || res["source"] != "agent" || res["started"] == nil || res["finished"] == nil {
		t.Errorf("result 57 %v", res)
	}
	i := env.NodeIndex()["ledger:56"]
	if src := env.Nodes[i].Source; len(src) != 1 || src[0].Tool != "Claude.Agent.Runner" || src[0].Line != 56 {
		t.Errorf("result 56's source block %+v", src)
	}
	for _, e := range []string{"ui:ledger:portal/1|ledger:56|answered-by", "ledger:56|ui:ledger:portal/1|answers", "ui:ledger:portal/2|ledger:57|answered-by", "ledger:57|ui:ledger:portal/2|answers"} {
		if !hasEdge(env, e) {
			t.Errorf("no edge %s", e)
		}
	}
	if props(env, "ledger:55") != nil || len(env.Nodes) != 6 {
		t.Errorf("%d nodes, want the root, three requests and two results", len(env.Nodes))
	}

	// One request, with its result; the gap alone; no request at line 9.
	one := served(t, h.do(t, base, "GET", actionsPath+"/2", "", 200))
	if one.Root != "ui:ledger:portal/2" || props(one, "ledger:57") == nil || !hasEdge(one, "ui:ledger:portal/2|ledger:57|answered-by") || len(one.Nodes) != 2 {
		t.Errorf("action 2: root %s, %d nodes", one.Root, len(one.Nodes))
	}
	gap := served(t, h.do(t, base, "GET", actionsPath+"/3", "", 200))
	if len(gap.Nodes) != 1 || props(gap, "ui:ledger:portal/3")["outcome"] != "requested" {
		t.Errorf("action 3: %d nodes", len(gap.Nodes))
	}
	h.do(t, base, "GET", actionsPath+"/9", "", 404)

	// The entry pages link both ways.
	e := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/portal/1", "", 200))
	if props(e, "ledger:56") == nil || !hasEdge(e, "ui:ledger:portal/1|ledger:56|answered-by") || props(e, "ui:ledger:portal/1")["resultId"] != "ledger:56" {
		t.Error("request 1's entry page lacks its result")
	}
	e = served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/agent/57", "", 200))
	if props(e, "ui:ledger:portal/2") == nil || !hasEdge(e, "ledger:57|ui:ledger:portal/2|answers") || num(props(e, "ledger:57")["requestSeq"]) != 2 {
		t.Error("result 57's entry page lacks its request")
	}
	// The chain's own pages carry the join too.
	page := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/portal", "", 200))
	if props(page, "ui:ledger:portal/2")["outcome"] != "failed" {
		t.Errorf("portal chain's request 2 %v", props(page, "ui:ledger:portal/2"))
	}
	c := props(served(t, h.do(t, base, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if c["valid"] != true || num(c["count"]) != 3 || num(c["unchained"]) != 0 {
		t.Errorf("the fixture's chain %v", c)
	}
}

// TestActionResultsAreReadFromTheLedgerLayerOnly: an alert layer's copy of
// a result answers nothing, mounted alone or after the ledger layer.
func TestActionResultsAreReadFromTheLedgerLayerOnly(t *testing.T) {
	h := newHarness(t)
	fp := strings.Repeat("e", 64)
	copyOf := alertLayer(t, "copies", "2026-10-09T02:52:00Z", []map[string]any{
		alertNodeOf(alertID(fp), "info", "action-seen", "An action result.", "2026-10-09T02:52:00Z", "open", fp),
		{"id": "ledger:58", "kind": "LedgerEntry", "name": "58 action.result", "properties": map[string]any{
			"seq": 58, "stamp": "2026-10-09T02:51:30Z", "event": "action.result", "kind": "action.result", "request": "ledger:3", "status": "done", "exit": 0, "output": "stopped"}},
	}, [][3]string{{alertID(fp), "ledger:58", "about"}})

	base := actionsPortal(t, h)
	h.do(t, base, "POST", "/api/tenants/default/layers", copyOf, 201)
	if r := props(served(t, h.do(t, base, "GET", actionsPath, "", 200)), ActionsID); num(r["requested"]) != 3 {
		t.Errorf("with only an alert layer's copy: %v, want all three requested", r)
	}
	h.do(t, base, "GET", "/api/tenants/default/ledger/agent", "", 404)

	base = actionsPortal(t, h, [2]string{"ledger", ledgerDir})
	h.do(t, base, "POST", "/api/tenants/default/layers", copyOf, 201)
	env := served(t, h.do(t, base, "GET", actionsPath, "", 200))
	if q := props(env, "ui:ledger:portal/3"); q["outcome"] != "requested" || q["resultId"] != nil {
		t.Errorf("request 3 %v: the copy answered it", q)
	}
	if props(env, "ledger:58") != nil {
		t.Error("the copy is served")
	}
}

// TestActionLinesSurviveARestartAndABadOneStopsIt: the boot reads request
// lines among pushes; a request line that breaks the contract stops it,
// naming the line.
func TestActionLinesSurviveARestartAndABadOneStopsIt(t *testing.T) {
	h := newHarness(t)
	s, p, dir := feedsPortal(t)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", cardParts(t, "0.1.0"), 201)
	h.do(t, s.URL, "POST", actionsPath, ask("stop", agentBox), 201)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", cardParts(t, "0.2.0"), 201)
	s = reloadTenant(t, p, dir)
	env := served(t, h.do(t, s.URL, "GET", actionsPath, "", 200))
	if q := props(env, "ui:ledger:portal/2"); q["action"] != "stop" || q["lineId"] != "ledger:2" || q["by"] != tester {
		t.Errorf("request 2 after a restart %v", q)
	}
	c := props(served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if c["valid"] != true || num(c["count"]) != 3 {
		t.Errorf("pushes and an ask in one chain %v", c)
	}
	// The feeds graph is the pushes' alone.
	if f := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/feeds", "", 200)); len(f.Nodes) != 4 {
		t.Errorf("feeds envelope %d nodes", len(f.Nodes))
	}

	path := filepath.Join(dir, tenant.LedgerFile)
	data, _ := os.ReadFile(path)
	bad := strings.Replace(string(data), `"action":"stop"`, `"action":"nuke"`, 1)
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope})
	if err == nil || !strings.Contains(err.Error(), "line 2: not a ledger entry") || !strings.Contains(err.Error(), "nuke") {
		t.Errorf("a bad request line: %v", err)
	}
}

// TestActionSubjectsAreTheGraphsContainers: the page offers Logs, Stop and
// Restart on an environment.instance whose properties.kind is container
// (nodemenu.js isContainer), and the API takes exactly those as subjects.
// Over the real environment layer, the two rules pick the same three nodes;
// the compose node, images and networks are refused (422).
func TestActionSubjectsAreTheGraphsContainers(t *testing.T) {
	h := newHarness(t)
	base := envPortal(t, h, [2]string{"agent-environment", envDir})
	g, _ := environmentGraph(t, h, base)
	var menu, api []string
	for _, n := range g.Nodes {
		if n.Kind == "environment.instance" && n.Properties["kind"] == "container" && store.IsEnvironment(n) {
			menu = append(menu, n.ID)
		}
		if tenant.CheckAction("logs", n.ID) == nil {
			api = append(api, n.ID)
		}
	}
	want := "env:container/claude-agent-dev/agent,env:container/claude-agent-dev/ledger,env:container/claude-agent-dev/mitm"
	if strings.Join(menu, ",") != want || strings.Join(api, ",") != want {
		t.Errorf("menu offers actions on %v, the API takes %v; want %s", menu, api, want)
	}
	for _, n := range g.Nodes {
		if !strings.HasPrefix(n.ID, tenant.ActionSubjectPrefix) && strings.HasPrefix(n.ID, "env:") {
			h.do(t, base, "POST", actionsPath, ask("restart", n.ID), 422)
		}
	}
}
