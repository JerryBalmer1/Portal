package server

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// The ledger fixtures (docs/design.md 45): twelve entries of the agent's
// ledger, seq 44 to 55, nine of them in the agent-runs fixture's run; the
// broken copy has entry 50 altered and re-hashed, so 51's prev no longer
// names it, while its root still says valid.
const (
	ledgerFile       = "2026-10-09T02-51-00Z.ledger.graph.json"
	brokenLedgerDir  = "../../tests/fixtures/layers/ledger-broken"
	fixtureRunLayer  = "agent-runs"
	fixtureLedgerRun = 9
)

// ledgerPortal is a portal with the runs fixture and the given ledger
// fixture mounted by path.
func ledgerPortal(t *testing.T, h *harness, dir string) string {
	t.Helper()
	s, _, _ := newPortal(t, false)
	for name, d := range map[string]string{fixtureRunLayer: runsDir, "ledger": dir} {
		abs, _ := filepath.Abs(d)
		j, _ := json.Marshal(abs)
		h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"`+name+`","path":`+string(j)+`}`, 201)
	}
	return s.URL
}

// reloadTenant loads the tenant in td again, as a restart does, and serves
// it on p with fresh fake feeds.
func reloadTenant(t *testing.T, p *store.Portal, td string) *httptest.Server {
	t.Helper()
	tt, err := tenant.Load(tenant.Options{Dir: filepath.Dir(td), Base: portal.BaseYAML, Schema: portal.ConfigSchema,
		Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope})
	if err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(New(p, tt, Options{Feeds: fakeFeeds(t)}))
	t.Cleanup(s.Close)
	return s
}

func chainsOf(env *graph.Envelope) []string {
	var out []string
	for _, n := range env.Nodes {
		if n.Kind == "Ledger" {
			out = append(out, n.ID)
		}
	}
	return out
}

func TestKnownGoodLedgerVerifies(t *testing.T) {
	h := newHarness(t)
	base := ledgerPortal(t, h, ledgerDir)
	env := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger", "", 200))
	if got := strings.Join(chainsOf(env), ","); got != "ui:ledger:agent,ui:ledger:portal" {
		t.Errorf("chains %s, want agent then portal", got)
	}
	a := props(env, "ui:ledger:agent")
	if a["valid"] != true || num(a["count"]) != 12 || num(a["first"]) != 44 || num(a["last"]) != 55 || a["breakAt"] != nil || a["reason"] != nil {
		t.Errorf("agent chain %v, want valid, 12 entries, 44 to 55, no break", a)
	}
	if a["stamp"] != "2026-10-09T02:51:00Z" || a["layer"] != "ledger" || a["claimed"] != true || a["source"] != "agent" {
		t.Errorf("agent chain %v", a)
	}
	if p := props(env, "ui:ledger:portal"); p["valid"] != true || num(p["count"]) != 0 || num(p["unchained"]) != 0 || p["first"] != nil {
		t.Errorf("an empty portal chain %v", p)
	}
}

func TestAlteredLedgerEntryBreaksTheChain(t *testing.T) {
	h := newHarness(t)
	base := ledgerPortal(t, h, brokenLedgerDir)
	env := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger", "", 200))
	a := props(env, "ui:ledger:agent")
	if a["valid"] != false || num(a["breakAt"]) != 51 || a["reason"] != store.BreakPrev || num(a["count"]) != 12 {
		t.Errorf("broken chain %v, want broken at 51, prev-mismatch", a)
	}
	// The layer's root still says valid: it was verified before the edit.
	if a["claimed"] != true {
		t.Errorf("claimed %v, want the layer root's true", a["claimed"])
	}
	// A layer that says it is broken is broken, at the earlier break.
	c := store.Chain{Entries: []store.ChainEntry{{Seq: 1, Prev: store.ZeroHash, Hash: "a"}, {Seq: 2, Prev: "a", Hash: "b"}}}
	no := false
	c.Claimed, c.ClaimedBreak = &no, 2
	c.Verify()
	if c.Valid || c.BreakAt != 2 || c.Reason != store.BreakLayer {
		t.Errorf("a chain its layer calls broken at 2: %+v", c)
	}
}

// Every records edge of the ledger fixture ends at a node of the runs
// fixture, by the same id; served, each entry of the run names that layer.
func TestEveryRecordsEdgeResolvesToTheRunsLayer(t *testing.T) {
	read := func(dir, name string) *graph.Envelope {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		var env graph.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatal(err)
		}
		return &env
	}
	runs := read(runsDir, "2026-10-09T02-50-56Z.runs.graph.json").NodeIndex()
	for _, dir := range []string{ledgerDir, brokenLedgerDir} {
		var records int
		for _, e := range read(dir, ledgerFile).Edges {
			if e.Kind != "records" {
				continue
			}
			records++
			if _, ok := runs[e.To]; !ok {
				t.Errorf("%s: records %s -> %s, which the runs layer lacks", dir, e.From, e.To)
			}
		}
		if records != fixtureLedgerRun {
			t.Errorf("%s: %d records edges, want %d", dir, records, fixtureLedgerRun)
		}
	}

	h := newHarness(t)
	base := ledgerPortal(t, h, ledgerDir)
	env := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/agent?run="+url.QueryEscape(fixtureRun), "", 200))
	var n int
	for _, node := range env.Nodes {
		if node.Kind != "LedgerEntry" {
			continue
		}
		n++
		if node.Properties["run"] != fixtureRun || node.Properties["runLayer"] != fixtureRunLayer {
			t.Errorf("%s: run %v in %v", node.ID, node.Properties["run"], node.Properties["runLayer"])
		}
	}
	if n != fixtureLedgerRun || num(props(env, "ui:ledger:agent")["entries"]) != fixtureLedgerRun {
		t.Errorf("%d entries of the run, want %d", n, fixtureLedgerRun)
	}
	// The agent's page counts them per run.
	runsEnv := served(t, h.do(t, base, "GET", "/api/tenants/default/agents/tester/runs", "", 200))
	if got := num(props(runsEnv, fixtureRun)["ledger"]); got != fixtureLedgerRun {
		t.Errorf("run ledger count %d, want %d", got, fixtureLedgerRun)
	}
}

func TestLedgerEntriesNewestFirstInPages(t *testing.T) {
	h := newHarness(t)
	base := ledgerPortal(t, h, ledgerDir)
	page := func(q string) (*graph.Envelope, []string) {
		env := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/agent"+q, "", 200))
		var ids []string
		for _, n := range env.Nodes {
			if n.Kind == "LedgerEntry" {
				ids = append(ids, n.ID)
			}
		}
		return env, ids
	}
	env, ids := page("")
	if env.Root != "ui:ledger:agent" || len(ids) != 12 || ids[0] != "ledger:55" || ids[11] != "ledger:44" || props(env, env.Root)["next"] != nil {
		t.Errorf("root %s, entries %v, want ledger:55 down to ledger:44 and no next", env.Root, ids)
	}
	env, ids = page("?limit=5")
	if strings.Join(ids, ",") != "ledger:55,ledger:54,ledger:53,ledger:52,ledger:51" || num(props(env, env.Root)["next"]) != 51 {
		t.Errorf("first page %v, next %v", ids, props(env, env.Root)["next"])
	}
	// prev joins neighbours on the page; the last one's predecessor is on the next.
	if !hasEdge(env, "ledger:52|ledger:51|prev") || hasEdge(env, "ledger:51|ledger:50|prev") {
		t.Error("prev edges on the page")
	}
	if !hasEdge(env, "ledger:52|"+fixtureRun+"/step-3|records") || !hasEdge(env, "ui:ledger:agent|ledger:55|has-entry") {
		t.Error("records and has-entry edges")
	}
	env, ids = page("?limit=5&before=51")
	if strings.Join(ids, ",") != "ledger:50,ledger:49,ledger:48,ledger:47,ledger:46" || num(props(env, env.Root)["next"]) != 46 {
		t.Errorf("second page %v", ids)
	}
	if _, ids = page("?limit=5&before=46"); strings.Join(ids, ",") != "ledger:45,ledger:44" {
		t.Errorf("last page %v", ids)
	}
	e := props(env, "ledger:50")
	if num(e["seq"]) != 50 || e["event"] != "policy.pre" || e["stamp"] != "2026-10-09T02:50:47Z" || len(e["hash"].(string)) != 64 || len(e["prev"].(string)) != 64 || e["source"] != "agent" {
		t.Errorf("entry 50 %v", e)
	}

	one := served(t, h.do(t, base, "GET", "/api/tenants/default/ledger/agent/50", "", 200))
	if one.Root != "ledger:50" || props(one, "ledger:49") == nil || props(one, "ui:ledger:agent") == nil {
		t.Errorf("entry 50: root %s, nodes %d", one.Root, len(one.Nodes))
	}
	for _, id := range []string{"ledger:50|ledger:49|prev", "ledger:50|" + fixtureRun + "/step-2|records", "ui:ledger:agent|ledger:50|has-entry"} {
		if !hasEdge(one, id) {
			t.Errorf("entry 50 lacks %s", id)
		}
	}
	if props(one, "ledger:50")["prev"] != props(one, "ledger:49")["hash"] {
		t.Error("entry 50's prev is not entry 49's hash")
	}
	h.do(t, base, "GET", "/api/tenants/default/ledger/agent/43", "", 404)
	h.do(t, base, "GET", "/api/tenants/default/ledger/portal/1", "", 404)
	h.do(t, base, "GET", "/api/tenants/default/ledger/agents", "", 404)
}

// Portal's own push ledger is a chain from 0.10.0: each push's line names
// the hash of the line before. A line from before is an unchained head; a
// line edited by hand breaks the chain at the line after it.
func TestPortalLedgerChainsItsPushes(t *testing.T) {
	h := newHarness(t)
	s, p, dir := feedsPortal(t)
	path := filepath.Join(dir, tenant.LedgerFile)
	legacy := `{"at":"2026-10-11T00:00:00.000Z","principal":null,"name":"old","role":"admin","feed":"cards-oci","kind":"oci","card":"Tool.Old","version":"1.0.0","type":"tool","hash":"sha256:` + strings.Repeat("ab", 32) + `","digest":null,"files":1,"result":"pushed","message":"Tool.Old 1.0.0 to cards-oci"}` + "\n"
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	s = reloadTenant(t, p, dir)
	const push = "/api/tenants/default/feeds/cards-oci/push"
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.1.0"), 201)
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.1.0"), 409)
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.2.0"), 201)

	env := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger", "", 200))
	if got := strings.Join(chainsOf(env), ","); got != "ui:ledger:portal" {
		t.Errorf("chains %s, want portal alone with no ledger layer", got)
	}
	c := props(env, "ui:ledger:portal")
	if c["valid"] != true || num(c["count"]) != 4 || num(c["unchained"]) != 1 || num(c["first"]) != 1 || num(c["last"]) != 4 {
		t.Errorf("portal chain %v, want valid, 4 lines, 1 unchained", c)
	}
	h.do(t, s.URL, "GET", "/api/tenants/default/ledger/agent", "", 404)
	h.do(t, s.URL, "GET", "/default/ledger/agent", "", 404)
	one := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger/portal/2", "", 200))
	e := props(one, "ledger:tenant/default/2")
	if e["result"] != "pushed" || e["card"] != "Tool.Git" || e["version"] != "0.1.0" || e["by"] != tester || e["feed"] != "cards-oci" || e["prev"] != props(one, "ledger:tenant/default/1")["hash"] {
		t.Errorf("entry 2 %v", e)
	}
	if props(one, "ledger:tenant/default/1")["unchained"] != true || !hasEdge(one, "ledger:tenant/default/2|ledger:tenant/default/1|prev") {
		t.Errorf("entry 1 is the unchained head, joined by prev")
	}
	if props(one, "ledger:tenant/default/1")["prev"] != nil {
		t.Error("an unchained line names no prev")
	}

	// An edit by hand to line 2 breaks the chain at 3, whose prev named the
	// line as it was; the binary still boots.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), `"files":2,"result":"pushed","message":"Tool.Git 0.1.0`, `"files":3,"result":"pushed","message":"Tool.Git 0.1.0`, 1)
	if edited == string(data) {
		t.Fatal("no line to edit")
	}
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	s = reloadTenant(t, p, dir)
	c = props(served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if c["valid"] != false || num(c["breakAt"]) != 3 || c["reason"] != store.BreakPrev {
		t.Errorf("edited ledger %v, want broken at 3, prev-mismatch", c)
	}
	// The next push chains to the edited line as it now is.
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.3.0"), 201)
	c = props(served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger", "", 200)), "ui:ledger:portal")
	if num(c["count"]) != 5 || num(c["breakAt"]) != 3 {
		t.Errorf("after one more push %v", c)
	}
	page := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/ledger/portal?limit=2", "", 200))
	if props(page, "ledger:tenant/default/5") == nil || props(page, "ledger:tenant/default/4") == nil || props(page, "ledger:tenant/default/3") != nil || num(props(page, "ui:ledger:portal")["next"]) != 4 {
		t.Errorf("portal page of 2: %d nodes", len(page.Nodes))
	}
}
