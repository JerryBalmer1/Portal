package server

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// cardsDir holds the nine emitter cards Portal.Emitters v0.2.0 pushes
// (emitters/<folder>/manifest.yaml, byte for byte), one folder per card
// name, and the Tool.Git card.
const cardsDir = "../../tests/fixtures/cards"

// emitterCards are the nine cards' names and versions.
var emitterCards = [][2]string{
	{"Emitter.Agent", "0.1.4"}, {"Emitter.AzureDevOps", "0.3.1"}, {"Emitter.Environment", "0.3.0"},
	{"Emitter.FileSystem", "0.2.0"}, {"Emitter.Git", "0.1.1"}, {"Emitter.Ledger", "0.3.0"},
	{"Emitter.Network", "0.4.0"}, {"Emitter.Reference", "0.1.1"}, {"Emitter.Terraform", "0.17.1"},
}

func cardID(name, version string) string { return tenant.CardID("cards-oci", name, version) }

func emitterCard(t *testing.T, name string) []part {
	t.Helper()
	m, err := os.ReadFile(filepath.Join(cardsDir, name, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return []part{{"manifest", "manifest.yaml", string(m)}}
}

// stamped copies a fixture layer folder (its one *.graph.json and its
// ontology.yaml) to a temporary folder with Graph.Node's source block
// inserted and nothing else changed: emitter, version and target as given,
// stamp the layer's own (docs/design.md 49). No layer on disk is stamped
// yet: Portal.Agent's stamping (ce2a76f) and Emitter.Agent 0.1.4 have not run
// since, so each test stamps a copy as that code would.
func stamped(t *testing.T, dir, emitter, version, target string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.graph.json"))
	if len(files) != 1 {
		t.Fatalf("%s: %d layers, want 1", dir, len(files))
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var head struct {
		Layer  string          `json:"layer"`
		Source json.RawMessage `json:"source"`
	}
	if err := json.Unmarshal(data, &head); err != nil || head.Source != nil {
		t.Fatalf("%s: %v, or already stamped", files[0], err)
	}
	src, _ := json.Marshal(graph.EnvelopeSource{Emitter: emitter, Version: version, Target: target, Stamp: head.Layer})
	i := bytes.IndexByte(data, '{')
	out := append(append(append(bytes.Clone(data[:i+1]), `"source":`...), src...), ',')
	out = append(out, data[i+1:]...)
	to := t.TempDir()
	if err := os.WriteFile(filepath.Join(to, filepath.Base(files[0])), out, 0o644); err != nil {
		t.Fatal(err)
	}
	ont, err := os.ReadFile(filepath.Join(dir, "ontology.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(to, "ontology.yaml"), ont, 0o644); err != nil {
		t.Fatal(err)
	}
	return to
}

// emittersPortal is feedsPortal with the nine cards pushed to cards-oci (a
// ledger line each, as any push), and five layers:
//   - agent-runs, stamped Emitter.Agent 0.1.4 (the card's version): ok;
//   - ledger, stamped Emitter.Ledger 0.3.0 (the card's): ok;
//   - agent-environment, stamped Emitter.Environment 0.3.0 (the card's) and
//     agent-alerts, stamped Emitter.Environment 0.4.0 (Claude.Agent.Alerts'
//     ModuleVersion, as New-AgentAlertLayer writes it): version-mismatch;
//   - ledger-old, the broken ledger copy stamped Emitter.Unlisted, which no
//     card names: no-card;
//   - filesystem at start, as it is (no source block): unstamped, under its
//     module Graph.Emitter.FileSystem, never the card Emitter.FileSystem
//     whose module that is.
//
// The other six cards have no layer: no-layers.
func emittersPortal(t *testing.T, h *harness) (string, *store.Portal) {
	t.Helper()
	s, p, _ := feedsPortal(t)
	for _, c := range emitterCards {
		h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", emitterCard(t, c[0]), 201)
	}
	for _, m := range [][2]string{
		{"agent-runs", stamped(t, runsDir, "Emitter.Agent", "0.1.4", "ledger.jsonl")},
		{"ledger", stamped(t, ledgerDir, "Emitter.Ledger", "0.3.0", "default")},
		{"agent-environment", stamped(t, envDir, "Emitter.Environment", "0.3.0", "default")},
		{alertsLayer, stamped(t, alertsDir, "Emitter.Environment", "0.4.0", "default")},
		{"ledger-old", stamped(t, "../../tests/fixtures/layers/ledger-broken", "Emitter.Unlisted", "1.0.0", "default")},
	} {
		j, _ := json.Marshal(m[1])
		h.do(t, s.URL, "POST", "/api/tenants/default/layers", `{"name":"`+m[0]+`","path":`+string(j)+`}`, 201)
	}
	if _, err := p.MountAtStart(store.Mount{Name: "filesystem", Path: fixtureDir}); err != nil {
		t.Fatal(err)
	}
	return s.URL, p
}

// rollCall is the envelope's emitters as "<name> <status> <layers>", in order.
func rollCall(env *graph.Envelope) []string {
	var out []string
	for _, n := range env.Nodes {
		if n.Kind == "Card" || n.Kind == "Emitter" {
			out = append(out, n.Name+" "+n.Properties["status"].(string)+" "+n.ID)
		}
	}
	return out
}

// Graph.Node 0.4.0 cards push, and the ledger line and the Card node carry
// the card's module, repo, url and emits.
func TestEmitterCardsOfGraphNode040Push(t *testing.T) {
	h := newHarness(t)
	s, _, _ := feedsPortal(t)
	for _, c := range emitterCards {
		var e tenant.LedgerEntry
		if err := json.Unmarshal(h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", emitterCard(t, c[0]), 201), &e); err != nil {
			t.Fatal(err)
		}
		if *e.Card != c[0] || *e.Version != c[1] || e.Module == "" || e.Repo == "" || !strings.HasPrefix(e.URL, "https://github.com/JerryBalmer1/") || len(e.Emits) == 0 {
			t.Errorf("%s: ledger line %+v", c[0], e)
		}
	}
	env := served(t, h.do(t, s.URL, "GET", "/api/tenants/default/feeds", "", 200))
	c := props(env, cardID("Emitter.Environment", "0.3.0"))
	emits, _ := json.Marshal(c["emits"])
	if c["module"] != "Claude.Agent.Orchestrator" || c["repo"] != "Portal.Agent" || c["url"] != "https://github.com/JerryBalmer1/Portal.Agent" ||
		string(emits) != `[{"facet":"environment","kinds":["environment.instance","environment.image","environment.network"]},{"facet":"alerts","kinds":["alert"]}]` {
		t.Errorf("Emitter.Environment's card %v", c)
	}
	// An emitter card without module, repo and emits is Graph.Node 0.4.0's 422.
	body := h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push",
		[]part{{"manifest", "manifest.yaml", "name: Emitter.Old\nversion: 1.0.0\ntype: emitter\ndescription: x\n"}}, 422)
	if !strings.Contains(string(body), `"code":"schema"`) {
		t.Errorf("422 body %s", body)
	}
}

// One row per name a card or a layer's source block gives, with its
// status; an unstamped layer under its module, never its card.
func TestEmittersMatchBySourceBlock(t *testing.T) {
	h := newHarness(t)
	base, _ := emittersPortal(t, h)

	env := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters", "", 200))
	want := []string{
		"Emitter.Agent ok " + cardID("Emitter.Agent", "0.1.4"),
		"Emitter.AzureDevOps no-layers " + cardID("Emitter.AzureDevOps", "0.3.1"),
		"Emitter.Environment version-mismatch " + cardID("Emitter.Environment", "0.3.0"),
		"Emitter.FileSystem no-layers " + cardID("Emitter.FileSystem", "0.2.0"),
		"Emitter.Git no-layers " + cardID("Emitter.Git", "0.1.1"),
		"Emitter.Ledger ok " + cardID("Emitter.Ledger", "0.3.0"),
		"Emitter.Network no-layers " + cardID("Emitter.Network", "0.4.0"),
		"Emitter.Reference no-layers " + cardID("Emitter.Reference", "0.1.1"),
		"Emitter.Terraform no-layers " + cardID("Emitter.Terraform", "0.17.1"),
		"Emitter.Unlisted no-card ui:emitter:Emitter.Unlisted",
		"Graph.Emitter.FileSystem unstamped ui:module:Graph.Emitter.FileSystem",
	}
	if got := rollCall(env); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("roll call\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	vs := func(id string) string { b, _ := json.Marshal(props(env, id)["layerVersions"]); return string(b) }
	for id, want := range map[string]string{
		cardID("Emitter.Agent", "0.1.4"):       `["0.1.4"]`,
		cardID("Emitter.Environment", "0.3.0"): `["0.3.0","0.4.0"]`,
		cardID("Emitter.Terraform", "0.17.1"):  `null`,
		"ui:emitter:Emitter.Unlisted":          `["1.0.0"]`,
		"ui:module:Graph.Emitter.FileSystem":   `null`,
	} {
		if got := vs(id); got != want {
			t.Errorf("%s layerVersions %s, want %s", id, got, want)
		}
	}
	envCard := props(env, cardID("Emitter.Environment", "0.3.0"))
	if envCard["emitter"] != "Emitter.Environment" || envCard["version"] != "0.3.0" || num(envCard["layers"]) != 2 || envCard["module"] != "Claude.Agent.Orchestrator" {
		t.Errorf("Emitter.Environment %v", envCard)
	}
	for _, e := range []string{
		"ui:layer:agent-runs|" + cardID("Emitter.Agent", "0.1.4") + "|produced-by",
		cardID("Emitter.Agent", "0.1.4") + "|ui:layer:agent-runs|has-layer",
		"ui:layer:ledger|" + cardID("Emitter.Ledger", "0.3.0") + "|produced-by",
		"ui:layer:agent-environment|" + cardID("Emitter.Environment", "0.3.0") + "|produced-by",
		"ui:layer:" + alertsLayer + "|" + cardID("Emitter.Environment", "0.3.0") + "|produced-by",
		"ui:layer:ledger-old|ui:emitter:Emitter.Unlisted|produced-by",
		"ui:layer:filesystem|ui:module:Graph.Emitter.FileSystem|produced-by",
	} {
		if !hasEdge(env, e) {
			t.Errorf("no edge %s", e)
		}
	}
	if hasEdge(env, "ui:layer:filesystem|"+cardID("Emitter.FileSystem", "0.2.0")+"|produced-by") {
		t.Error("the unstamped FileSystem layer is matched to the card whose module it names")
	}

	one := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters/Emitter.Environment", "", 200))
	if one.Root != cardID("Emitter.Environment", "0.3.0") {
		t.Errorf("root %q", one.Root)
	}
	var layers []string
	for _, e := range one.Edges {
		if e.Kind == "has-layer" {
			layers = append(layers, e.To)
		}
	}
	if strings.Join(layers, ",") != "ui:layer:"+alertsLayer+",ui:layer:agent-environment" {
		t.Errorf("Emitter.Environment's layers %v, want the alerts layer (newer) first", layers)
	}
	l := props(one, "ui:layer:"+alertsLayer)
	src, _ := json.Marshal(l["source"])
	if l["emitter"] != "Emitter.Environment" || l["module"] != "Claude.Agent.Orchestrator" ||
		string(src) != `{"emitter":"Emitter.Environment","stamp":"2026-10-11T19:00:31Z","target":"default","version":"0.4.0"}` {
		t.Errorf("alerts layer %v, source %s", l, src)
	}
	fs := props(served(t, h.do(t, base, "GET", "/api/tenants/default/emitters/Graph.Emitter.FileSystem", "", 200)), "ui:layer:filesystem")
	if fs["emitter"] != "Graph.Emitter.FileSystem" || fs["source"] != nil {
		t.Errorf("unstamped filesystem layer %v", fs)
	}
	h.do(t, base, "GET", "/default/emitters/Emitter.Environment", "", 200)
	h.do(t, base, "GET", "/default/emitters/Graph.Emitter.FileSystem", "", 200)
}

// A card no layer names is still an emitter; a name nothing gives is 404,
// and so is a module no unstamped layer carries, a card's module included:
// nothing maps one name onto another. An unstamped module named like a card
// is a row of its own, after the card, which its name finds.
func TestEmitterNamesAreNeverMapped(t *testing.T) {
	h := newHarness(t)
	base, p := emittersPortal(t, h)
	h.do(t, base, "GET", "/api/tenants/default/emitters/Emitter.Terraform", "", 200)
	h.do(t, base, "GET", "/default/emitters/Emitter.Terraform", "", 200)
	for _, name := range []string{"Graph.Emitter.Agent", "Graph.Emitter.Terraform", "Claude.Agent.Ledger", "Emitter.Nope"} {
		h.do(t, base, "GET", "/api/tenants/default/emitters/"+name, "", 404)
		h.do(t, base, "GET", "/default/emitters/"+name, "", 404)
	}
	h.do(t, base, "GET", "/default/emitters/sources", "", 200)

	if _, err := p.MountAtStart(store.Mount{Name: "odd", Envelope: json.RawMessage(`{"schema":"graph/1","module":"Emitter.Ledger","layer":"2026-10-09T00:00:00Z","nodes":[{"id":"a","kind":"K"}],"edges":[]}`)}); err != nil {
		t.Fatal(err)
	}
	env := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters", "", 200))
	got := strings.Join(rollCall(env), "\n")
	if !strings.Contains(got, "Emitter.Ledger ok "+cardID("Emitter.Ledger", "0.3.0")+"\nEmitter.Ledger unstamped ui:module:Emitter.Ledger\n") {
		t.Errorf("roll call\n%s", got)
	}
	if one := served(t, h.do(t, base, "GET", "/api/tenants/default/emitters/Emitter.Ledger", "", 200)); one.Root != cardID("Emitter.Ledger", "0.3.0") {
		t.Errorf("Emitter.Ledger is %s, want the card", one.Root)
	}

	// The unstamped module stops being one when its last layer is replaced.
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
		[]part{{"manifest", "manifest.yaml", "name: sources\nversion: 1.0.0\ntype: emitter\ndescription: x\nmodule: M\nrepo: R\nemits: [{facet: f, kinds: [K]}]\n"}}, 422)
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
