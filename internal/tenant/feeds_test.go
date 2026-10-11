package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/feed"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// fakePusher stores what it is sent and answers like a feed: ErrExists for
// a name and version it holds, err when set.
type fakePusher struct {
	held map[string]bool
	err  error
}

func (f *fakePusher) Push(_ context.Context, fd feed.Feed, c *feed.Card, by, created string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	key := fd.ID + "/" + c.Manifest.Name + "@" + c.Manifest.Version
	if f.held[key] {
		return "", feed.ErrExists
	}
	f.held[key] = true
	return "sha256:" + strings.Repeat("d", 64), nil
}

const card = "name: Tool.Git\nversion: 0.1.0\ntype: tool\ndescription: Git CLI.\n"

var admin = Principal{ID: "ui:principal:jerry", Name: "jerry", Type: "user", Role: RoleAdmin}

func TestFeedsMergeOverlayOverBase(t *testing.T) {
	base, err := LoadConfig(portal.BaseYAML, []byte("{}"), portal.ConfigSchema)
	if err != nil {
		t.Fatal(err)
	}
	fs, ps := loadFeeds(base)
	if len(ps) > 0 || len(fs) != 2 || fs[0].ID != "cards-oci" || fs[0].Kind != "oci" || fs[1].ID != "cards-nuget" || fs[1].TokenEnv != "PORTAL_NUGET_KEY" || fs[1].Visibility != "private" {
		t.Fatalf("base feeds %+v %v", fs, ps)
	}
	// One feed overridden by id, key by key; one added; the other kept as the base has it.
	overlay := "feeds:\n  - { id: cards-oci, url: \"http://127.0.0.1:9201/team\", visibility: public }\n  - { id: team-nuget, kind: nuget, url: \"https://nuget.example/v3/index.json\" }\n"
	c, err := LoadConfig(portal.BaseYAML, []byte(overlay), portal.ConfigSchema)
	if err != nil {
		t.Fatal(err)
	}
	fs, _ = loadFeeds(c)
	if len(fs) != 3 {
		t.Fatalf("feeds %+v", fs)
	}
	if fs[0].URL != "http://127.0.0.1:9201/team" || fs[0].Kind != "oci" || fs[0].Visibility != "public" || fs[0].Label != "Cards (OCI)" {
		t.Errorf("overridden %+v", fs[0])
	}
	if fs[1].URL != "http://127.0.0.1:8202/v3/index.json" {
		t.Errorf("kept %+v", fs[1])
	}
	if fs[2].ID != "team-nuget" || fs[2].Visibility != "private" || fs[2].Label != "team-nuget" {
		t.Errorf("added %+v", fs[2])
	}
}

func TestBadFeedsFailBoot(t *testing.T) {
	for overlay, want := range map[string]string{
		"feeds:\n  - { id: x, kind: npm, url: \"http://x\" }\n":                    "/feeds/0/kind",
		"feeds:\n  - { id: x, url: \"http://x\" }\n":                               "/feeds/2/kind",
		"feeds:\n  - { id: x, kind: oci, url: \"ftp://x\" }\n":                     "/feeds/0/url",
		"feeds:\n  - { id: x, kind: oci, url: \"http://x\", colour: red }\n":       "/feeds/0",
		"feeds:\n  - { id: x, kind: oci, url: \"http://x\", visibility: team }\n":  "/feeds/0/visibility",
		"feeds:\n  - { id: x, kind: oci, url: \"http://x\", token_env: \"1A\" }\n": "/feeds/0/token_env",
		"feeds:\n  - { id: Bad, kind: oci, url: \"http://x\" }\n":                  "/feeds/0/id",
	} {
		_, err := load(t, map[string]string{"portal.yaml": overlay})
		var ce *ConfigError
		if !errors.As(err, &ce) || !strings.Contains(strings.Join(ce.Problems, " "), want) {
			t.Errorf("%q: want a problem at %s, got %v", overlay, want, err)
		}
	}
}

func TestPushWritesOneLedgerLinePerPush(t *testing.T) {
	dir := writeTenant(t, map[string]string{"portal.yaml": "{}\n"})
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	tt, err := Load(Options{Dir: dir, Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-11T00:00:00.000Z", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	p := &fakePusher{held: map[string]bool{}}
	files := []feed.File{{Path: "install.yaml", Data: []byte("tool: Tool.Git\n")}}

	e, err := tt.Push(context.Background(), p, admin, "cards-oci", []byte(card), files)
	if err != nil {
		t.Fatal(err)
	}
	if e.Result != ResultPushed || *e.Card != "Tool.Git" || *e.Version != "0.1.0" || e.Files != 2 || e.Digest == nil || *e.Principal != admin.ID || e.At != "2026-10-11T09:00:00.000Z" {
		t.Errorf("entry %+v", e)
	}
	// Again: the ledger already holds it, so the feed is not asked.
	_, err = tt.Push(context.Background(), p, admin, "cards-oci", []byte(card), files)
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("second push: %v", err)
	}
	// The same card in the other feed is a different push.
	if _, err := tt.Push(context.Background(), p, admin, "cards-nuget", []byte(card), files); err != nil {
		t.Errorf("other feed: %v", err)
	}
	// A feed that already holds a version the ledger does not know is a duplicate too.
	p.held["cards-oci/Tool.Git@0.2.0"] = true
	if _, err := tt.Push(context.Background(), p, admin, "cards-oci", []byte(strings.Replace(card, "0.1.0", "0.2.0", 1)), files); !errors.Is(err, ErrDuplicate) {
		t.Errorf("held by the feed: %v", err)
	}
	// An invalid manifest, then a feed that fails.
	if _, err := tt.Push(context.Background(), p, admin, "cards-oci", []byte("name: Tool.Git\nversion: x\n"), nil); !errors.Is(err, feed.ErrInvalid) {
		t.Errorf("invalid: %v", err)
	}
	p.err = &feed.UpstreamError{Status: 500, Message: "boom"}
	if _, err := tt.Push(context.Background(), p, Principal{Name: "old", Role: RoleAdmin, Legacy: true}, "cards-oci", []byte(strings.Replace(card, "0.1.0", "0.3.0", 1)), nil); err == nil {
		t.Error("a failing feed passed")
	}
	if _, err := tt.Push(context.Background(), p, admin, "nope", []byte(card), nil); !errors.Is(err, ErrNoFeed) {
		t.Errorf("unknown feed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, Default, LedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	var results []string
	for _, l := range lines {
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatal(err)
		}
		// Every key on every line, null when it does not apply.
		for _, k := range []string{"at", "principal", "name", "role", "feed", "kind", "card", "version", "type", "hash", "digest", "files", "result", "message"} {
			if _, ok := m[k]; !ok {
				t.Errorf("line lacks %s: %s", k, l)
			}
		}
		results = append(results, m["result"].(string))
	}
	if got := strings.Join(results, ","); got != "pushed,duplicate,pushed,duplicate,invalid,failed" {
		t.Errorf("results %s (the unknown feed writes nothing)", got)
	}
	var legacy LedgerEntry
	_ = json.Unmarshal([]byte(lines[5]), &legacy)
	if legacy.Principal != nil || legacy.Name != "old" {
		t.Errorf("legacy token line %+v", legacy)
	}
	var invalid LedgerEntry
	_ = json.Unmarshal([]byte(lines[4]), &invalid)
	if invalid.Hash != nil || invalid.Digest != nil || *invalid.Card != "Tool.Git" {
		t.Errorf("invalid line %+v", invalid)
	}

	// A reload reads the ledger back: the cards are on the feeds envelope, which is a valid graph.
	again, err := Load(Options{Dir: dir, Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-11T00:00:00.000Z"})
	if err != nil {
		t.Fatal(err)
	}
	env := again.FeedsEnvelope("2026-10-11T10:00:00.000Z")
	if ps := graph.Validate(env); graph.HasErrors(ps) {
		t.Fatalf("feeds envelope: %v", ps)
	}
	if env.Counts.ByKind["Feed"] != 2 || env.Counts.ByKind["Card"] != 2 || len(env.Edges) != 2 {
		t.Errorf("counts %+v", env.Counts)
	}
	if _, ok := env.NodeIndex()[CardID("cards-oci", "Tool.Git", "0.1.0")]; !ok {
		t.Error("no ui:card:cards-oci/Tool.Git@0.1.0")
	}
	if _, err := again.Push(context.Background(), p, admin, "cards-oci", []byte(card), files); !errors.Is(err, ErrDuplicate) {
		t.Errorf("after reload: %v", err)
	}
}

func TestABadLedgerLineStopsBoot(t *testing.T) {
	_, err := load(t, map[string]string{"portal.yaml": "{}", LedgerFile: "{\"at\":\"x\",\"feed\":\"f\",\"result\":\"failed\"}\nnot json\n"})
	var ce *ConfigError
	if !errors.As(err, &ce) || !strings.Contains(ce.Problems[0], "line 2") {
		t.Errorf("want line 2 named, got %v", err)
	}
	_, err = load(t, map[string]string{"portal.yaml": "{}", LedgerFile: "{\"at\":\"x\",\"feed\":\"f\",\"result\":\"pushed\",\"extra\":1}\n"})
	if !errors.As(err, &ce) {
		t.Errorf("an unknown key: %v", err)
	}
	_, err = load(t, map[string]string{"portal.yaml": "{}", LedgerFile: "{\"at\":\"x\",\"feed\":\"f\",\"result\":\"pushed\"}\n"})
	if !errors.As(err, &ce) || !strings.Contains(ce.Problems[0], "line 1") {
		t.Errorf("a pushed line without its card: %v", err)
	}
	if _, err := load(t, map[string]string{"portal.yaml": "{}", LedgerFile: ""}); err != nil {
		t.Errorf("an empty ledger: %v", err)
	}
}
