package tenant

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/feed"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Feeds (docs/design.md 40). The merged config's feeds: list names where
// cards are published; the tenant's ledger.jsonl, beside its identity files
// and gitignored like them, records every push, one JSON line each,
// appended and never rewritten. It is the third and last file the binary
// writes outside -artifacts.
const (
	LedgerFile = "ledger.jsonl"
	// FeedPrefix + id is a feed's node id; CardPrefix + feed/name@version a
	// pushed card's.
	FeedPrefix = "ui:feed:"
	CardPrefix = "ui:card:"
	// zeroHash is the prev of the ledger's first line.
	zeroHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

// Push results, as the ledger records them.
const (
	ResultPushed    = "pushed"
	ResultDuplicate = "duplicate"
	ResultInvalid   = "invalid"
	ResultFailed    = "failed"
)

// Visibilities a feed may declare; private when absent. Recorded on the Feed
// node and not enforced in 0.6.0: Portal's reads are open, and the feed
// services themselves are bound to 127.0.0.1.
var Visibilities = []string{"private", "public"}

// Errors of a push the server maps onto 404 and 409.
var (
	ErrNoFeed    = errors.New("no such feed")
	ErrDuplicate = errors.New("duplicate")
)

// LedgerEntry is one line of ledger.jsonl: who pushed, to which feed, which
// card at which version with which content hash, and what came of it.
// Principal is null for a legacy token; Card, Version, Type and Hash are null
// for a manifest that did not decode that far; Digest is the feed's own
// name for what it stored, set only when Result is pushed. Seq and Prev
// chain the line (docs/design.md 45): Seq is its position in the file from
// 1, Prev the SHA-256 of the line before, 64 zeros for the first. A line
// written before 0.10.0 has neither.
type LedgerEntry struct {
	At        string  `json:"at"`
	Principal *string `json:"principal"`
	Name      string  `json:"name"`
	Role      string  `json:"role"`
	Feed      string  `json:"feed"`
	Kind      string  `json:"kind"`
	Card      *string `json:"card"`
	Version   *string `json:"version"`
	Type      *string `json:"type"`
	Hash      *string `json:"hash"`
	Digest    *string `json:"digest"`
	Files     int     `json:"files"`
	Result    string  `json:"result"`
	Message   string  `json:"message"`
	Seq       int     `json:"seq,omitempty"`
	Prev      string  `json:"prev,omitempty"`
}

// LedgerLine is one line of ledger.jsonl with the SHA-256 of its bytes as
// written (no newline), the hash the next line's Prev names.
type LedgerLine struct {
	LedgerEntry
	Hash string
}

var envVar = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// loadFeeds reads the merged config's feeds. The schema has checked each
// item's keys; this checks what an overlay naming only what it changes
// cannot: every feed has a kind and an http(s) url.
func loadFeeds(c map[string]any) ([]feed.Feed, []string) {
	var out []feed.Feed
	var ps []string
	list, _ := c["feeds"].([]any)
	seen := map[string]bool{}
	for i, raw := range list {
		m, _ := raw.(map[string]any)
		at := fmt.Sprintf("/feeds/%d", i)
		str := func(k string) string { s, _ := m[k].(string); return s }
		f := feed.Feed{ID: str("id"), Label: str("label"), Kind: str("kind"), URL: str("url"), Visibility: str("visibility"), TokenEnv: str("token_env")}
		if seen[f.ID] {
			ps = append(ps, fmt.Sprintf("%s/id: %q twice", at, f.ID))
		}
		seen[f.ID] = true
		if f.Label == "" {
			f.Label = f.ID
		}
		if f.Visibility == "" {
			f.Visibility = "private"
		}
		if f.Kind != feed.KindOCI && f.Kind != feed.KindNuGet {
			ps = append(ps, at+"/kind: required, oci or nuget")
		}
		if u, err := url.Parse(f.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			ps = append(ps, fmt.Sprintf("%s/url: %q is not an http(s) URL", at, f.URL))
		}
		if f.TokenEnv != "" && !envVar.MatchString(f.TokenEnv) {
			ps = append(ps, fmt.Sprintf("%s/token_env: %q names no environment variable", at, f.TokenEnv))
		}
		out = append(out, f)
	}
	return out, ps
}

// loadLedger reads ledger.jsonl and the bytes of each line; no file is an
// empty ledger. A line that is not a ledger entry stops the binary, naming
// the line. A broken chain does not: GET .../ledger reports it.
func loadLedger(path string) ([]LedgerEntry, [][]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var out []LedgerEntry
	var raw [][]byte
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e LedgerEntry
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.DisallowUnknownFields()
		err := dec.Decode(&e)
		if err == nil && e.Result == ResultPushed && (e.Card == nil || e.Version == nil || e.Type == nil || e.Hash == nil) {
			err = errors.New("a pushed entry names its card, version, type and hash")
		}
		if err != nil || e.At == "" || e.Feed == "" || e.Result == "" {
			msg := "not a ledger entry"
			if err != nil {
				msg += ": " + err.Error()
			}
			return nil, nil, &ConfigError{File: path, Problems: []string{fmt.Sprintf("line %d: %s", n, msg)}}
		}
		out = append(out, e)
		raw = append(raw, bytes.Clone(line))
	}
	return out, raw, sc.Err()
}

// Feed returns the named feed.
func (t *Tenant) Feed(id string) (feed.Feed, error) {
	for _, f := range t.Feeds {
		if f.ID == id {
			return f, nil
		}
	}
	return feed.Feed{}, fmt.Errorf("%w: %s; tenant %s has %s", ErrNoFeed, id, t.Name, strings.Join(t.feedIDs(), ", "))
}

func (t *Tenant) feedIDs() []string {
	var ids []string
	for _, f := range t.Feeds {
		ids = append(ids, f.ID)
	}
	return ids
}

// Ledger is a copy of every entry, oldest first.
func (t *Tenant) Ledger() []LedgerEntry {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.ledger)
}

// LedgerLines is every entry with the hash of its line, oldest first.
func (t *Tenant) LedgerLines() []LedgerLine {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]LedgerLine, len(t.ledger))
	for i, e := range t.ledger {
		out[i] = LedgerLine{LedgerEntry: e, Hash: lineHash(t.ledgerRaw[i])}
	}
	return out
}

func lineHash(line []byte) string {
	sum := sha256.Sum256(line)
	return hex.EncodeToString(sum[:])
}

// FeedsEnvelope is the tenant's feeds as a graph: one Feed node per feed in
// config order, and one Card node per card pushed to it (from the ledger's
// pushed entries), joined Feed to Card by has-card.
func (t *Tenant) FeedsEnvelope(stamp string) *graph.Envelope {
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Ontology = "ontology.yaml"
	for i, f := range t.Feeds {
		props := map[string]any{"kind": f.Kind, "url": f.URL, "visibility": f.Visibility, "order": i}
		if f.TokenEnv != "" {
			props["tokenEnv"] = f.TokenEnv
		}
		env.Nodes = append(env.Nodes, graph.Node{ID: FeedPrefix + f.ID, Kind: "Feed", Name: f.Label, Properties: props})
	}
	for _, e := range t.Ledger() {
		if e.Result != ResultPushed {
			continue
		}
		if _, err := t.Feed(e.Feed); err != nil {
			continue // a feed the config no longer names keeps its ledger lines
		}
		id := CardID(e.Feed, *e.Card, *e.Version)
		props := map[string]any{"feed": e.Feed, "version": *e.Version, "type": *e.Type, "hash": *e.Hash, "pushed": e.At, "pushedBy": e.Name}
		if e.Digest != nil {
			props["digest"] = *e.Digest
		}
		env.Nodes = append(env.Nodes, graph.Node{ID: id, Kind: "Card", Name: *e.Card, Properties: props})
		env.Edges = append(env.Edges, graph.NewEdge(FeedPrefix+e.Feed, id, "has-card"))
	}
	env.Recount()
	return env
}

// CardID is a pushed card's node id: ui:card:<feed>/<name>@<version>.
func CardID(feedID, name, version string) string {
	return CardPrefix + feedID + "/" + name + "@" + version
}

// Pusher is what Push sends a card with: feed.Client, or a fake in tests.
type Pusher interface {
	Push(ctx context.Context, f feed.Feed, c *feed.Card, by, created string) (string, error)
}

// Push publishes one card to the named feed as who and appends one ledger
// entry, whatever the outcome once the feed is known: pushed, duplicate (the
// ledger, or the feed itself, already has that name and version: 409),
// invalid (the manifest or a path: 422) or failed (no key, or the feed
// refused or failed). Pushes are one at a time per tenant, so two pushes of
// one version cannot both pass the duplicate check. The returned entry is
// what the ledger holds; the error says why it is not pushed.
func (t *Tenant) Push(ctx context.Context, p Pusher, who Principal, feedID string, manifest []byte, files []feed.File) (LedgerEntry, error) {
	f, err := t.Feed(feedID)
	if err != nil {
		return LedgerEntry{}, err
	}
	t.pushMu.Lock()
	defer t.pushMu.Unlock()
	e := LedgerEntry{At: t.now().UTC().Format(CreatedLayout), Principal: orNil(who.ID), Name: who.Name, Role: who.Role, Feed: f.ID, Kind: f.Kind}
	card, err := feed.NewCard(manifest, files)
	var bad *feed.InvalidError
	if errors.As(err, &bad) {
		e.Card, e.Version = orNil(bad.Name), orNil(bad.Version)
		e.Files = len(files) + 1
		e.Result, e.Message = ResultInvalid, err.Error()
		return t.record(e, err)
	}
	m := card.Manifest
	e.Card, e.Version, e.Type, e.Hash, e.Files = &m.Name, &m.Version, &m.Type, &card.Hash, len(card.Files)
	for _, prev := range t.Ledger() {
		if prev.Result == ResultPushed && prev.Feed == f.ID && *prev.Card == m.Name && *prev.Version == m.Version {
			e.Result = ResultDuplicate
			e.Message = fmt.Sprintf("%s %s is already in %s: pushed %s by %s, %s", m.Name, m.Version, f.ID, prev.At, prev.Name, *prev.Hash)
			return t.record(e, fmt.Errorf("%w: %s", ErrDuplicate, e.Message))
		}
	}
	d, err := p.Push(ctx, f, card, who.Name, e.At)
	switch {
	case errors.Is(err, feed.ErrExists):
		e.Result, e.Message = ResultDuplicate, err.Error()
		return t.record(e, fmt.Errorf("%w: %s", ErrDuplicate, err.Error()))
	case errors.Is(err, feed.ErrUnpackable):
		e.Result, e.Message = ResultInvalid, err.Error()
		return t.record(e, err)
	case err != nil:
		e.Result, e.Message = ResultFailed, err.Error()
		return t.record(e, err)
	}
	e.Digest = &d
	e.Result = ResultPushed
	e.Message = fmt.Sprintf("%s %s to %s", m.Name, m.Version, f.ID)
	return t.record(e, nil)
}

// record chains e to the ledger's last line (seq, prev), appends it to
// ledger.jsonl (one line, one write, synced) and to the in-memory ledger,
// and returns e with cause. A ledger that cannot be written is the error,
// whatever cause was. Pushes are one at a time (pushMu), so no line comes
// between the one read here and the one written.
func (t *Tenant) record(e LedgerEntry, cause error) (LedgerEntry, error) {
	t.mu.RLock()
	e.Seq, e.Prev = len(t.ledger)+1, zeroHash
	if n := len(t.ledgerRaw); n > 0 {
		e.Prev = lineHash(t.ledgerRaw[n-1])
	}
	t.mu.RUnlock()
	line, err := json.Marshal(e)
	if err != nil {
		return e, err
	}
	fh, err := os.OpenFile(filepath.Join(t.dir, LedgerFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return e, fmt.Errorf("ledger: %w", err)
	}
	_, werr := fh.Write(append(line, '\n'))
	serr := fh.Sync()
	cerr := fh.Close()
	if err := errors.Join(werr, serr, cerr); err != nil {
		return e, fmt.Errorf("ledger: %w", err)
	}
	t.mu.Lock()
	t.ledger = append(t.ledger, e)
	t.ledgerRaw = append(t.ledgerRaw, line)
	t.mu.Unlock()
	return e, cause
}

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
