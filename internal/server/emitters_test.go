package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// emitterCardDir is Graph.Emitter.Agent's card, Emitter.Agent 0.1.3, copied
// from Portal.Emitters (emitters/Agent/manifest.yaml at cd862d9).
const emitterCardDir = "../../tests/fixtures/cards/Emitter.Agent"

// agentCard is the card's id once pushed to cards-oci.
const agentCard = "ui:card:cards-oci/Emitter.Agent@0.1.3"

func emitterCard(t *testing.T) []part {
	t.Helper()
	m, err := os.ReadFile(filepath.Join(emitterCardDir, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return []part{{"manifest", "manifest.yaml", string(m)}}
}

// emittersPortal is feedsPortal with the seed card pushed to cards-oci (a
// ledger line, as any push), the agent-runs fixture mounted by path by
// tester, and the FileSystem fixture, whose emitter has no card, at start.
func emittersPortal(t *testing.T, h *harness) (string, *store.Portal) {
	t.Helper()
	s, p, _ := feedsPortal(t)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", emitterCard(t), 201)
	runs, _ := filepath.Abs(runsDir)
	runsJSON, _ := json.Marshal(runs)
	h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"agent-runs","path":`+string(runsJSON)+`}`, 201)
	if _, err := p.MountAtStart(store.Mount{Name: "filesystem", Path: fixtureDir}); err != nil {
		t.Fatal(err)
	}
	return s.URL, p
}

func TestEmitterOfALayerIsItsFeedCard(t *testing.T) {
	h := newHarness(t)
	base, _ := emittersPortal(t, h)

	env := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters", "", 200))
	var names []string
	for _, n := range env.Nodes {
		if n.Kind == "Card" || n.Kind == "Emitter" {
			names = append(names, n.Kind+" "+n.ID)
		}
	}
	if strings.Join(names, ",") != "Card "+agentCard+",Emitter ui:emitter:Graph.Emitter.FileSystem" {
		t.Errorf("emitters %v, want the card of Graph.Emitter.Agent then the unknown Graph.Emitter.FileSystem", names)
	}
	card := props(env, agentCard)
	if card["emitter"] != "Graph.Emitter.Agent" || card["feed"] != "cards-oci" || card["version"] != "0.1.3" || card["type"] != "emitter" || num(card["layers"]) != 1 || card["unknown"] != nil {
		t.Errorf("Graph.Emitter.Agent's card %v", card)
	}
	if fs := props(env, "ui:emitter:Graph.Emitter.FileSystem"); fs["unknown"] != true || num(fs["layers"]) != 1 {
		t.Errorf("the FileSystem emitter, which no feed holds, %v", fs)
	}
	// The layer's ontology names Graph.Emitter.Agent, and its produced-by
	// resolves to the card under the card's own id; has-layer is its reverse.
	for _, e := range []string{
		"ui:layer:agent-runs|" + agentCard + "|produced-by",
		agentCard + "|ui:layer:agent-runs|has-layer",
		"ui:layer:filesystem|ui:emitter:Graph.Emitter.FileSystem|produced-by",
	} {
		if !hasEdge(env, e) {
			t.Errorf("no edge %s", e)
		}
	}

	one := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters/Graph.Emitter.Agent", "", 200))
	if one.Root != agentCard {
		t.Errorf("root %q, want the card %s", one.Root, agentCard)
	}
	l := props(one, "ui:layer:agent-runs")
	if l["emitter"] != "Graph.Emitter.Agent" || l["stamp"] != "2026-10-09T02:50:56Z" || num(l["nodes"]) != 12 || num(l["edges"]) != 11 || l["mountedBy"] != "ui:principal:tester" {
		t.Errorf("agent-runs layer %v", l)
	}
	if props(one, "ui:layer:filesystem") != nil {
		t.Error("Graph.Emitter.Agent's envelope holds the FileSystem layer")
	}
	h.do(t, base, "GET", "/default/emitters/Graph.Emitter.Agent", "", 200)
}

// A name no feed card and no mounted layer gives is 404, on the API and the
// page, and so is an emitter whose last layer has gone; the emitters
// service's pages keep their paths.
func TestUnmountedEmitterIs404(t *testing.T) {
	h := newHarness(t)
	base, p := emittersPortal(t, h)
	h.do(t, base, "GET", "/api/tenants/default/emitters/Graph.Emitter.Terraform", "", 404)
	h.do(t, base, "GET", "/default/emitters/Graph.Emitter.Terraform", "", 404)
	h.do(t, base, "GET", "/api/tenants/default/emitters/Emitter.Agent", "", 404)
	h.do(t, base, "GET", "/default/emitters/sources", "", 200)

	// The FileSystem emitter is known only by its layer; inline, under the
	// same name, the layer's module is M, so the emitter is gone.
	h.do(t, base, "GET", "/api/tenants/default/emitters/Graph.Emitter.FileSystem", "", 200)
	if _, err := p.MountAtStart(store.Mount{Name: "filesystem", Envelope: json.RawMessage(`{"schema":"graph/1","module":"M","layer":"2026-10-09T00:00:00Z","nodes":[{"id":"a","kind":"K"}],"edges":[]}`)}); err != nil {
		t.Fatal(err)
	}
	h.do(t, base, "GET", "/api/tenants/default/emitters/Graph.Emitter.FileSystem", "", 404)
	h.do(t, base, "GET", "/default/emitters/Graph.Emitter.FileSystem", "", 404)
}

// An emitter card named like a page of the emitters service would take its
// route. A push cannot make one: Graph.Node's manifest schema wants a dotted
// name, and a menu id has no dot. A ledger that holds one anyway stops the
// boot.
func TestEmitterCardNamedLikeAPageStopsTheBoot(t *testing.T) {
	h := newHarness(t)
	s, p, dir := feedsPortal(t)
	body := h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push",
		[]part{{"manifest", "manifest.yaml", "name: sources\nversion: 1.0.0\ntype: emitter\ndescription: x\n"}}, 422)
	if !strings.Contains(string(body), `"code":"schema"`) || !strings.Contains(string(body), "manifest.yaml:/name") {
		t.Errorf("422 body %s", body)
	}
	ledger := filepath.Join(dir, tenant.LedgerFile)
	data, err := os.ReadFile(ledger)
	if err != nil {
		t.Fatal(err)
	}
	line := `{"at":"2026-10-11T00:00:00.000Z","principal":null,"name":"old","role":"admin","feed":"cards-oci","kind":"oci","card":"runs","version":"1.0.0","type":"emitter","hash":"sha256:` + strings.Repeat("ab", 32) + `","digest":null,"files":1,"result":"pushed","message":"runs 1.0.0 to cards-oci"}` + "\n"
	if err := os.WriteFile(ledger, append(data, line...), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope})
	if err == nil || !strings.Contains(err.Error(), "emitter card runs 1.0.0") || !strings.Contains(err.Error(), "emitters/runs") {
		t.Errorf("boot with an emitter card named runs: %v", err)
	}
	// The same line as a tool is no emitter, and takes no route.
	if err := os.WriteFile(ledger, append(data, strings.Replace(line, `"type":"emitter"`, `"type":"tool"`, 1)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := tenant.Load(tenant.Options{Dir: filepath.Dir(dir), Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-10T00:00:00.000Z", Check: p.CheckEnvelope}); err != nil {
		t.Errorf("boot with a tool card named runs: %v", err)
	}
}
