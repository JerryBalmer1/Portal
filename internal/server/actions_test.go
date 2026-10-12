package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// Actions (docs/design.md 50). The fixture ledger.jsonl is one chain: three
// requests as root (1 logs on the agent container, 2 restart on mitm, 3
// stop on the ledger keeper), then the Runner's results, 4 answering 1
// (done) and 5 answering 2 (failed, exit 1); 3 is the gap.
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
	if a.Kind != "action.request" || a.ID != "ledger:tenant/default/1" || a.Action != "logs" || a.Subject != agentBox || a.Principal != "ui:principal:contrib" ||
		a.Status != "requested" || a.Seq != 1 || a.Prev != store.ZeroHash || a.Requested == "" {
		t.Errorf("first ask %+v", a)
	}
	if err := json.Unmarshal(h.do(t, s.URL, "POST", actionsPath, ask("restart", "env:container/claude-agent-dev/mitm"), 201), &b); err != nil {
		t.Fatal(err)
	}
	if b.ID != "ledger:tenant/default/2" || b.Principal != "ui:principal:"+tester || b.Seq != 2 {
		t.Errorf("second ask %+v", b)
	}
	// The file holds the lines as returned, member for member in the
	// contract's order, chained.
	data, err := os.ReadFile(filepath.Join(dir, tenant.LedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], `{"kind":"action.request","id":"ledger:tenant/default/1","action":"logs","subject":"`+agentBox+`","principal":"ui:principal:contrib","requested":"`) ||
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
	if q := props(env, "ledger:tenant/default/1"); q["principal"] != "ui:principal:contrib" || q["by"] != "contrib" || q["outcome"] != "requested" || q["lineId"] != nil || q["resultId"] != nil {
		t.Errorf("request 1 %v", q)
	}
}

// tenantLineHash is a line's SHA-256, the next line's prev.
func tenantLineHash(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:])
}

// TestActionsJoinTheirResults: requests and results from the one chain,
// newest request first, each with its result; the gap stays requested. A
// mounted ledger layer changes nothing.
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
	if strings.Join(order, ",") != "ledger:tenant/default/3,ledger:tenant/default/2,ledger:tenant/default/1" {
		t.Errorf("requests %v, want newest first", order)
	}
	want := map[string][3]any{ // outcome, resultId, resultSeq
		"ledger:tenant/default/1": {"done", "ledger:tenant/default/4", 4},
		"ledger:tenant/default/2": {"failed", "ledger:tenant/default/5", 5},
		"ledger:tenant/default/3": {"requested", nil, -1},
	}
	for id, w := range want {
		q := props(env, id)
		if q["event"] != "action.request" || q["outcome"] != w[0] || q["resultId"] != w[1] || num(q["resultSeq"]) != w[2] || q["status"] != "requested" ||
			q["principal"] != "ui:principal:root" || q["source"] != "portal" || q["lineId"] != nil {
			t.Errorf("%s %v, want %v", id, q, w)
		}
	}
	res := props(env, "ledger:tenant/default/5")
	if res["event"] != "action.result" || res["request"] != "ledger:tenant/default/2" || num(res["requestSeq"]) != 2 || res["status"] != "failed" || num(res["exit"]) != 1 ||
		!strings.HasPrefix(res["output"].(string), "Error response from daemon") || res["source"] != "portal" || res["started"] == nil || res["finished"] == nil {
		t.Errorf("result 5 %v", res)
	}
	if w, _ := res["writer"].([]any); len(w) != 1 || w[0].(map[string]any)["emitter"] != "Emitter.Ledger" || w[0].(map[string]any)["target"] != "default" {
		t.Errorf("result 5's writer %v, want the line's source, one item", res["writer"])
	}
	i := env.NodeIndex()["ledger:tenant/default/4"]
	if src := env.Nodes[i].Source; len(src) != 1 || src[0].Tool != "Emitter.Ledger" || src[0].Path != tenant.LedgerFile || src[0].Line != 4 {
		t.Errorf("result 4's source block %+v", src)
	}
	for _, e := range []string{"ledger:tenant/default/1|ledger:tenant/default/4|answered-by", "ledger:tenant/default/4|ledger:tenant/default/1|answers", "ledger:tenant/default/2|ledger:tenant/default/5|answered-by", "ledger:tenant/default/5|ledger:tenant/default/2|answers"} {
		if !hasEdge(env, e) {
			t.Errorf("no edge %s", e)
		}
	}
	if len(env.Nodes) != 6 {
		t.Errorf("%d nodes, want the root, three requests and two results", len(env.Nodes))
	}

	// One request, with its result; the gap alone; no request at line 4
	// (a result) or 9 (none).
	one := served(t, h.do(t, base, "GET", actionsPath+"/2", "", 200))
	if one.Root != "ledger:tenant/default/2" || props(one, "ledger:tenant/default/5") == nil || !hasEdge(one, "ledger:tenant/default/2|ledger:tenant/default/5|answered-by") || len(one.Nodes) != 2 {
		t.Errorf("action 2: root %s, %d nodes", one.Root, len(one.Nodes))
	}
	gap := served(t, h.do(t, base, "GET", actionsPath+"/3", "", 200))
	if len(gap.Nodes) != 1 || props(gap, "ledger:tenant/default/3")["outcome"] != "requested" {
		t.Errorf("action 3: %d nodes", len(gap.Nodes))
	}
	h.do(t, base, "GET", actionsPath+"/4", "", 404)
	h.do(t, base, "GET", actionsPath+"/9", "", 404)

	// The entry pages link both ways, in the one chain.
	e := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/portal/1", "", 200))
	if props(e, "ledger:tenant/default/4") == nil || !hasEdge(e, "ledger:tenant/default/1|ledger:tenant/default/4|answered-by") || props(e, "ledger:tenant/default/1")["resultId"] != "ledger:tenant/default/4" {
		t.Error("request 1's entry page lacks its result")
	}
	e = served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/portal/5", "", 200))
	if props(e, "ledger:tenant/default/2") == nil || !hasEdge(e, "ledger:tenant/default/5|ledger:tenant/default/2|answers") || num(props(e, "ledger:tenant/default/5")["requestSeq"]) != 2 || !hasEdge(e, "ledger:tenant/default/5|ledger:tenant/default/4|prev") {
		t.Error("result 5's entry page lacks its request or the entry before it")
	}
	// A result whose request is not the line before it brings it; the
	// line before is served once either way.
	e = served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/portal/4", "", 200))
	if len(e.Nodes) != 4 || !hasEdge(e, "ledger:tenant/default/4|ledger:tenant/default/1|answers") || !hasEdge(e, "ledger:tenant/default/4|ledger:tenant/default/3|prev") {
		t.Errorf("result 4's entry page: %d nodes, want the chain, 4, 3 and its request 1", len(e.Nodes))
	}
	// The chain's own pages carry the join too.
	page := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/portal", "", 200))
	if props(page, "ledger:tenant/default/2")["outcome"] != "failed" || num(props(page, "ledger:tenant/default/4")["requestSeq"]) != 1 {
		t.Errorf("portal chain's request 2 %v", props(page, "ledger:tenant/default/2"))
	}
	c := props(served(t, h.do(t, base, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if c["valid"] != true || num(c["count"]) != 5 || num(c["unchained"]) != 0 {
		t.Errorf("the fixture's chain %v", c)
	}
}

// TestActionResultsAreReadFromTheTenantLedgerOnly: an action.result node
// in a mounted ledger layer, or in an alert layer's copy, answers nothing;
// nor does a result line before the request it names.
func TestActionResultsAreReadFromTheTenantLedgerOnly(t *testing.T) {
	h := newHarness(t)
	fp := strings.Repeat("e", 64)
	result := map[string]any{"id": "ledger:58", "kind": "LedgerEntry", "name": "58 action.result", "properties": map[string]any{
		"seq": 58, "stamp": "2026-10-09T02:51:30Z", "event": "action.result", "kind": "action.result", "request": "ledger:tenant/default/3", "status": "done", "exit": 0, "output": "stopped"}}
	copyOf := alertLayer(t, "copies", "2026-10-09T02:52:00Z", []map[string]any{
		alertNodeOf(alertID(fp), "info", "action-seen", "An action result.", "2026-10-09T02:52:00Z", "open", fp), result,
	}, [][3]string{{alertID(fp), "ledger:58", "about"}})
	old, _ := json.Marshal(map[string]any{"name": "old-ledger", "envelope": map[string]any{"schema": "graph/1", "module": "Portal.Agent",
		"version": "0.1.0", "layer": "2026-10-09T02:52:00Z", "nodes": []map[string]any{result}, "edges": []any{}}})

	base := actionsPortal(t, h)
	h.do(t, base, "POST", "/api/tenants/default/layers", string(old), 201)
	h.do(t, base, "POST", "/api/tenants/default/layers", copyOf, 201)
	env := served(t, h.do(t, base, "GET", actionsPath, "", 200))
	if q := props(env, "ledger:tenant/default/3"); q["outcome"] != "requested" || q["resultId"] != nil || q["source"] != "portal" {
		t.Errorf("request 3 %v: a layer answered it", q)
	}
	if r := props(env, ActionsID); num(r["requested"]) != 1 || len(env.Nodes) != 6 {
		t.Errorf("actions %v, %d nodes: a layer's result is served", r, len(env.Nodes))
	}

	// A result line ahead of the request it names answers nothing.
	_, p, dir := feedsPortal(t)
	runnerAppends(t, dir, runnerResult(t, dir, "ledger:2", "done", true))
	s := reloadTenant(t, p, dir)
	h.do(t, s.URL, "POST", actionsPath, ask("stop", agentBox), 201)
	env = served(t, h.do(t, s.URL, "GET", actionsPath, "", 200))
	if q := props(env, "ledger:tenant/default/2"); q["outcome"] != "requested" || num(props(env, ActionsID)["count"]) != 1 {
		t.Errorf("request 2 %v: answered by the line before it", q)
	}
}

// runnerResult is an action.result line as Portal.Agent's Runner 0.2.0
// appends it to dir's ledger.jsonl next: member for member, id and seq the
// next position, prev the last line's hash; its source one object (object)
// or a one-item list.
func runnerResult(t *testing.T, dir, request, status string, object bool) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, tenant.LedgerFile))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	prev, n := store.ZeroHash, 0
	for _, l := range strings.Split(string(data), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			prev, n = tenantLineHash(l), n+1
		}
	}
	src := `{"emitter":"Emitter.Ledger","version":"0.4.0","target":"default","stamp":"2026-10-11T22:00:01.000Z"}`
	if !object {
		src = "[" + src + "]"
	}
	exit := 0
	if status == "failed" {
		exit = 1
	}
	return fmt.Sprintf(`{"kind":"action.result","id":"ledger:%d","request":"%s","status":"%s","started":"2026-10-11T22:00:00.000Z","finished":"2026-10-11T22:00:01.000Z","exit":%d,"output":"","source":%s,"seq":%d,"prev":"%s"}`,
		n+1, request, status, exit, src, n+1, prev)
}

// runnerAppends appends line and its LF to dir's ledger.jsonl, as the
// Runner does, behind the running portal's back.
func runnerAppends(t *testing.T, dir, line string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, tenant.LedgerFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

// TestLinesAppendedAfterBootAreReadAndChainedTo: the Runner appends a
// result between boot and the next ask. The actions page and the ledger
// page see it without a restart, and the ask is chained to it: seq after
// it, prev its hash, so the file stays one chain. A line without its LF is
// still being written to a read, and ended before the next append. A reset
// (the file removed) drops both sides, so the next ask is ledger:tenant/default/1 again
// and nothing old answers it.
func TestLinesAppendedAfterBootAreReadAndChainedTo(t *testing.T) {
	h := newHarness(t)
	_, p, dir := feedsPortal(t)
	data, err := os.ReadFile(actionsLedger)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	path := filepath.Join(dir, tenant.LedgerFile)
	if err := os.WriteFile(path, []byte(strings.Join(lines[:3], "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := reloadTenant(t, p, dir)
	if r := props(served(t, h.do(t, s.URL, "GET", actionsPath, "", 200)), ActionsID); num(r["requested"]) != 3 {
		t.Fatalf("at boot %v, want three requested", r)
	}

	// Between boot and the next read, the Runner answers 1, its source a
	// bare object as Runner 0.2.0 writes it.
	result := runnerResult(t, dir, "ledger:1", "done", true)
	runnerAppends(t, dir, result)
	env := served(t, h.do(t, s.URL, "GET", actionsPath, "", 200))
	if q := props(env, "ledger:tenant/default/1"); q["outcome"] != "done" || q["resultId"] != "ledger:tenant/default/4" {
		t.Errorf("request 1 %v, want done by ledger:tenant/default/4, appended after boot", q)
	}
	if w, _ := props(env, "ledger:tenant/default/4")["writer"].([]any); len(w) != 1 {
		t.Errorf("a bare source object is read as the list of one: %v", props(env, "ledger:tenant/default/4")["writer"])
	}
	if e := props(served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger/portal?limit=1", "", 200)), "ledger:tenant/default/4"); e == nil || e["event"] != "action.result" {
		t.Errorf("the ledger page's newest entry %v, want the Runner's line", e)
	}

	// The next ask: seq 5, chained to the Runner's line.
	var a tenant.ActionRequest
	if err := json.Unmarshal(h.do(t, s.URL, "POST", actionsPath, ask("logs", agentBox), 201), &a); err != nil {
		t.Fatal(err)
	}
	if a.ID != "ledger:tenant/default/5" || a.Seq != 5 || a.Prev != tenantLineHash(result) {
		t.Errorf("ask after the Runner's line %+v, want ledger:tenant/default/5 chained to it", a)
	}
	// A line without its LF: a read leaves it; the next append ends it.
	torn := runnerResult(t, dir, "ledger:5", "failed", false)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(torn); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if q := props(served(t, h.do(t, s.URL, "GET", actionsPath, "", 200)), "ledger:tenant/default/5"); q["outcome"] != "requested" {
		t.Errorf("request 5 %v: a line without its LF is still being written", q)
	}
	if err := json.Unmarshal(h.do(t, s.URL, "POST", actionsPath, ask("restart", agentBox), 201), &a); err != nil {
		t.Fatal(err)
	}
	if a.Seq != 7 || a.Prev != tenantLineHash(torn) {
		t.Errorf("ask after a torn line %+v, want seq 7 chained to it", a)
	}
	c := props(served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if c["valid"] != true || num(c["count"]) != 7 {
		t.Errorf("one chain, both writers %v", c)
	}
	if q := props(served(t, h.do(t, s.URL, "GET", actionsPath, "", 200)), "ledger:tenant/default/5"); q["outcome"] != "failed" {
		t.Errorf("request 5 %v, want failed by the torn line, now ended", q)
	}
	if _, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope}); err != nil {
		t.Errorf("the file both wrote does not boot: %v", err)
	}

	// A reset drops requests and results alike; ids start again at 1.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if r := props(served(t, h.do(t, s.URL, "GET", actionsPath, "", 200)), ActionsID); num(r["count"]) != 0 {
		t.Errorf("after a reset %v, want none", r)
	}
	if err := json.Unmarshal(h.do(t, s.URL, "POST", actionsPath, ask("logs", agentBox), 201), &a); err != nil {
		t.Fatal(err)
	}
	q := props(served(t, h.do(t, s.URL, "GET", actionsPath, "", 200)), "ledger:tenant/default/1")
	if a.ID != "ledger:tenant/default/1" || a.Prev != store.ZeroHash || q["outcome"] != "requested" {
		t.Errorf("first ask after a reset %+v %v, want ledger:tenant/default/1, unanswered", a, q)
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
	if q := props(env, "ledger:tenant/default/2"); q["action"] != "stop" || q["lineId"] != nil || q["by"] != tester {
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

	// A result line breaking the contract stops it too: a member missing,
	// one too many, or a value out of range.
	good := runnerResult(t, dir, "ledger:2", "done", false)
	for name, bad := range map[string]string{
		"status":      strings.Replace(good, `"status":"done"`, `"status":"ok"`, 1),
		"request":     strings.Replace(good, `"request":"ledger:2"`, `"request":"ui:ledger:portal/2"`, 1),
		"exit":        strings.Replace(good, `"exit":0,`, ``, 1),
		"output":      strings.Replace(good, `"output":"",`, ``, 1),
		"unknown":     strings.Replace(good, `"output":""`, `"output":"","host":"x"`, 1),
		"two sources": strings.Replace(good, `"source":[`, `"source":[{"emitter":"a","version":"1","target":"t","stamp":"s"},`, 1),
		"no target":   strings.Replace(good, `"target":"default",`, ``, 1),
	} {
		if bad == good {
			t.Fatalf("%s: nothing replaced", name)
		}
		if err := os.WriteFile(path, []byte(bad+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope})
		if err == nil || !strings.Contains(err.Error(), "line 1: not a ledger entry") {
			t.Errorf("a result line, %s: %v", name, err)
		}
	}
	if err := os.WriteFile(path, []byte(good+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope}); err != nil {
		t.Errorf("the good result line: %v", err)
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
