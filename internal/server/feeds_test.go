package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"

	"github.com/JerryBalmer1/Portal/internal/feed"
	"github.com/JerryBalmer1/Portal/internal/feed/feedtest"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// nugetKey is the API key of the fake NuGet feed, and PORTAL_NUGET_KEY in
// testEnv, the variable the base config's cards-nuget names.
const nugetKey = "pt-nuget-key"

// cardFixture is Tool.Git, copied from Portal.Tools (tools/Git at c330712).
const cardFixture = "../../tests/fixtures/cards/Tool.Git"

// hostRewrite sends the base config's two feeds (127.0.0.1:8201 and :8202,
// the stock services of compose.yaml) to the fakes instead.
type hostRewrite map[string]string

func (h hostRewrite) RoundTrip(r *http.Request) (*http.Response, error) {
	if to, ok := h[r.URL.Host]; ok {
		r = r.Clone(r.Context())
		r.URL.Host = to
		r.Host = to
	}
	return http.DefaultTransport.RoundTrip(r)
}

// fakeFeeds is a feed client whose cards-oci is a fake registry and
// cards-nuget a fake NuGet feed (key nugetKey), with testEnv.
func fakeFeeds(t *testing.T) feed.Client {
	t.Helper()
	reg := feedtest.NewRegistry(t)
	ng := feedtest.NewNuGet(t, nugetKey)
	host := func(u string) string { p, _ := url.Parse(u); return p.Host }
	// The fake's service index names its own URLs, which the transport passes through.
	return feed.Client{
		HTTP:   &http.Client{Transport: hostRewrite{"127.0.0.1:8201": host(reg.URL), "127.0.0.1:8202": host(ng.URL)}},
		Getenv: func(k string) string { return testEnv[k] },
	}
}

// part is one multipart part: name manifest or file, its filename, its bytes.
type part struct{ name, filename, data string }

type formCall struct {
	token, target string
	parts         []part
	want          int
}

func cardParts(t *testing.T, version string) []part {
	t.Helper()
	m, err := os.ReadFile(filepath.Join(cardFixture, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	inst, err := os.ReadFile(filepath.Join(cardFixture, "install.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	manifest := strings.Replace(string(m), "version: 0.1.0", "version: "+version, 1)
	return []part{{"manifest", "manifest.yaml", manifest}, {"file", "install.yaml", string(inst)}}
}

// pushCalls are the spec test's pushes: every status the operation documents.
func pushCalls(t *testing.T) []formCall {
	const push = "/api/tenants/default/feeds/cards-oci/push"
	card := cardParts(t, "0.1.0")
	return []formCall{
		{testToken, push, card, 201},
		{testToken, push, card, 409},
		{testToken, "/api/tenants/default/feeds/cards-nuget/push", card, 201},
		{contribToken, push, cardParts(t, "0.1.1"), 201},
		{readerToken, push, cardParts(t, "0.1.2"), 403},
		{"", push, cardParts(t, "0.1.2"), 401},
		{testToken, push, []part{{"manifest", "manifest.yaml", "name: Tool.Git\nversion: one\ntype: tool\ndescription: x\n"}}, 422},
		{testToken, push, []part{{"manifest", "manifest.yaml", "name: Tool.Git\nversion: 0.1.3\ntype: tool\ndescription: x\n"}, {"file", "../escape", "x"}}, 422},
		{testToken, push, []part{{"file", "install.yaml", "x"}}, 400},
		{testToken, "/api/tenants/default/feeds/nope/push", card, 404},
		{testToken, "/api/tenants/other/feeds/cards-oci/push", card, 404},
	}
}

// doForm is do for a multipart body; "" token sends no Authorization.
func (h *harness) doForm(t *testing.T, token, base, target string, parts []part, want int) []byte {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for _, p := range parts {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", `form-data; name="`+p.name+`"; filename="`+p.filename+`"`)
		hdr.Set("Content-Type", "application/octet-stream")
		w, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(w, p.data)
	}
	_ = mw.Close()
	ctype := mw.FormDataContentType()
	req, _ := http.NewRequest(http.MethodPost, base+target, bytes.NewReader(body.Bytes()))
	req.Header.Set("Content-Type", ctype)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		t.Errorf("POST %s: status %d, want %d: %s", target, res.StatusCode, want, data)
	}
	vreq, _ := http.NewRequest(http.MethodPost, target, bytes.NewReader(body.Bytes()))
	vreq.Header.Set("Content-Type", ctype)
	route, params, err := h.router.FindRoute(vreq)
	if err != nil {
		t.Errorf("POST %s: not in spec: %v", target, err)
		return data
	}
	in := &openapi3filter.RequestValidationInput{Request: vreq, PathParams: params, Route: route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}}
	if want < 400 {
		if err := openapi3filter.ValidateRequest(context.Background(), in); err != nil {
			t.Errorf("POST %s: request invalid per spec: %v", target, err)
		}
	}
	out := &openapi3filter.ResponseValidationInput{RequestValidationInput: in, Status: res.StatusCode, Header: res.Header,
		Options: &openapi3filter.Options{IncludeResponseStatus: true}}
	out.SetBodyBytes(data)
	if err := openapi3filter.ValidateResponse(context.Background(), out); err != nil {
		t.Errorf("POST %s: response invalid per spec: %v", target, err)
	}
	return data
}

// feedsPortal is a server over a fresh tenant directory with fake feeds.
func feedsPortal(t *testing.T) (*httptest.Server, *store.Portal, string) {
	t.Helper()
	p, err := store.New(store.Options{Artifacts: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	tt, dir := newTenantDir(t, p, "https://api.github.com")
	s := httptest.NewServer(New(p, tt, Options{Feeds: fakeFeeds(t)}))
	t.Cleanup(s.Close)
	return s, p, dir
}

func TestPushNeedsContributorOrAdmin(t *testing.T) {
	h := newHarness(t)
	s, _, _ := feedsPortal(t)
	const push = "/api/tenants/default/feeds/cards-oci/push"
	h.doForm(t, "", s.URL, push, cardParts(t, "1.0.0"), 401)
	h.doForm(t, readerToken, s.URL, push, cardParts(t, "1.0.0"), 403)
	h.doForm(t, "pt-not-a-token", s.URL, push, cardParts(t, "1.0.0"), 401)
	var e tenant.LedgerEntry
	if err := json.Unmarshal(h.doForm(t, contribToken, s.URL, push, cardParts(t, "1.0.0"), 201), &e); err != nil {
		t.Fatal(err)
	}
	if e.Role != tenant.RoleContributor || *e.Principal != "ui:principal:contrib" {
		t.Errorf("contributor push %+v", e)
	}
	if err := json.Unmarshal(h.doForm(t, testToken, s.URL, push, cardParts(t, "1.0.1"), 201), &e); err != nil {
		t.Fatal(err)
	}
	if e.Role != tenant.RoleAdmin {
		t.Errorf("admin push %+v", e)
	}
	// A token from before 0.5.0 is an admin with no id: it pushes, and its ledger line's principal is null.
	if err := json.Unmarshal(h.doForm(t, legacyToken, s.URL, push, cardParts(t, "1.0.2"), 201), &e); err != nil {
		t.Fatal(err)
	}
	if e.Principal != nil || e.Name != "old" {
		t.Errorf("legacy push %+v", e)
	}
}

func TestDuplicateNameAndVersionIs409(t *testing.T) {
	h := newHarness(t)
	s, _, _ := feedsPortal(t)
	const push = "/api/tenants/default/feeds/cards-nuget/push"
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.1.0"), 201)
	body := h.doForm(t, contribToken, s.URL, push, cardParts(t, "0.1.0"), 409)
	if !strings.Contains(string(body), "already in cards-nuget") {
		t.Errorf("409 body %s", body)
	}
	// The same name at another version, or in the other feed, is not a duplicate.
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.2.0"), 201)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", cardParts(t, "0.1.0"), 201)
}

func TestFeedsEnvelopeIsAValidGraphOfFeedsAndCards(t *testing.T) {
	h := newHarness(t)
	s, p, _ := feedsPortal(t)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/cards-oci/push", cardParts(t, "0.1.0"), 201)
	var env graph.Envelope
	if err := json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/feeds", "", 200), &env); err != nil {
		t.Fatal(err)
	}
	ps := append(graph.Validate(&env), p.CheckEnvelope(&env)...)
	if graph.HasErrors(ps) {
		t.Fatalf("feeds envelope: %v", ps)
	}
	if env.Module != "Portal" || env.Counts.ByKind["Feed"] != 2 || env.Counts.ByKind["Card"] != 1 {
		t.Errorf("module %s counts %+v", env.Module, env.Counts)
	}
	idx := env.NodeIndex()
	oci, ok := idx["ui:feed:cards-oci"]
	if !ok || env.Nodes[oci].Properties["kind"] != "oci" || env.Nodes[oci].Properties["visibility"] != "private" {
		t.Errorf("ui:feed:cards-oci %+v", env.Nodes)
	}
	c, ok := idx["ui:card:cards-oci/Tool.Git@0.1.0"]
	if !ok {
		t.Fatalf("no card node: %+v", env.Nodes)
	}
	props := env.Nodes[c].Properties
	if props["version"] != "0.1.0" || props["type"] != "tool" || !strings.HasPrefix(props["hash"].(string), "sha256:") || props["pushedBy"] != tester {
		t.Errorf("card %+v", props)
	}
	if len(env.Edges) != 1 || env.Edges[0].ID != "ui:feed:cards-oci|ui:card:cards-oci/Tool.Git@0.1.0|has-card" {
		t.Errorf("edges %+v", env.Edges)
	}
}

func TestLedgerEntryShape(t *testing.T) {
	h := newHarness(t)
	s, _, dir := feedsPortal(t)
	const push = "/api/tenants/default/feeds/cards-oci/push"
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.1.0"), 201)
	h.doForm(t, testToken, s.URL, push, cardParts(t, "0.1.0"), 409)
	h.doForm(t, testToken, s.URL, push, []part{{"manifest", "manifest.yaml", "name: Tool.Git\n"}}, 422)
	h.doForm(t, readerToken, s.URL, push, cardParts(t, "0.1.0"), 403)
	h.doForm(t, testToken, s.URL, "/api/tenants/default/feeds/nope/push", cardParts(t, "0.1.0"), 404)
	data, err := os.ReadFile(filepath.Join(dir, tenant.LedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// One line per push that reached a feed the tenant has; refusals before that (403, 404) write none.
	if len(lines) != 3 {
		t.Fatalf("%d ledger lines, want 3:\n%s", len(lines), data)
	}
	keys := []string{"at", "principal", "name", "role", "feed", "kind", "card", "version", "type", "hash", "digest", "files", "result", "message", "seq", "prev"}
	want := []map[string]any{
		{"principal": "ui:principal:tester", "name": tester, "role": "admin", "feed": "cards-oci", "kind": "oci", "card": "Tool.Git", "version": "0.1.0", "type": "tool", "files": 2.0, "result": "pushed"},
		{"card": "Tool.Git", "version": "0.1.0", "digest": nil, "result": "duplicate"},
		{"card": "Tool.Git", "version": nil, "hash": nil, "digest": nil, "files": 1.0, "result": "invalid"},
	}
	for i, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		if len(m) != len(keys) {
			t.Errorf("line %d has %d keys, want %d: %s", i+1, len(m), len(keys), l)
		}
		for _, k := range keys {
			if _, ok := m[k]; !ok {
				t.Errorf("line %d lacks %s", i+1, k)
			}
		}
		for k, v := range want[i] {
			if m[k] != v {
				t.Errorf("line %d: %s = %v, want %v", i+1, k, m[k], v)
			}
		}
		// Chained (docs/design.md 45): seq is the line number, prev the
		// SHA-256 of the line before, 64 zeros for the first.
		prev := strings.Repeat("0", 64)
		if i > 0 {
			sum := sha256.Sum256([]byte(lines[i-1]))
			prev = hex.EncodeToString(sum[:])
		}
		if m["seq"] != float64(i+1) || m["prev"] != prev {
			t.Errorf("line %d: seq %v prev %v, want %d %s", i+1, m["seq"], m["prev"], i+1, prev)
		}
		if i == 0 {
			if !strings.HasPrefix(m["hash"].(string), "sha256:") || !strings.HasPrefix(m["digest"].(string), "sha256:") {
				t.Errorf("pushed line hash and digest: %s", l)
			}
			if !strings.HasSuffix(m["at"].(string), "Z") {
				t.Errorf("at %v", m["at"])
			}
		}
	}
}
