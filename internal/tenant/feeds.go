package tenant

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

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
	// lockTimeout bounds the wait for the file lock an append takes, as
	// the Runner's does; lockPoll is how often it tries again.
	lockTimeout = 30 * time.Second
	lockPoll    = 50 * time.Millisecond
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
// written before 0.10.0 has neither. Module, Repo, URL and Emits are the
// card's (Graph.Node 0.4.0 requires all but URL on an emitter card), written
// from 0.13.0 when the card has them, so the emitter's page is built from
// the card as pushed (docs/design.md 49); a line before 0.13.0 has none.
// Action is set on a line of kind action.request (docs/design.md 50), which
// is written and read as an ActionRequest: of the rest, only At (when it was
// requested), Name (the principal's name), Seq and Prev are set. Answer is
// set on a line of kind action.result, the Runner's, read as an
// ActionResult and never written here: At is when it finished, Name its
// source's emitter.
type LedgerEntry struct {
	At        string               `json:"at"`
	Principal *string              `json:"principal"`
	Name      string               `json:"name"`
	Role      string               `json:"role"`
	Feed      string               `json:"feed"`
	Kind      string               `json:"kind"`
	Card      *string              `json:"card"`
	Version   *string              `json:"version"`
	Type      *string              `json:"type"`
	Hash      *string              `json:"hash"`
	Digest    *string              `json:"digest"`
	Files     int                  `json:"files"`
	Result    string               `json:"result"`
	Message   string               `json:"message"`
	Seq       int                  `json:"seq,omitempty"`
	Prev      string               `json:"prev,omitempty"`
	Module    string               `json:"module,omitempty"`
	Repo      string               `json:"repo,omitempty"`
	URL       string               `json:"url,omitempty"`
	Emits     []graph.ManifestEmit `json:"emits,omitempty"`
	Action    *ActionRequest       `json:"-"`
	Answer    *ActionResult        `json:"-"`
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

// loadLedger reads ledger.jsonl and the bytes of each line, and returns
// how many bytes and lines it read; no file is an empty ledger. A line that
// is not a ledger entry stops the binary, naming the line. A broken chain
// does not: GET .../ledger reports it.
func loadLedger(path string) ([]LedgerEntry, [][]byte, int64, int, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, 0, 0, nil
	}
	if err != nil {
		return nil, nil, 0, 0, err
	}
	out, raw, used, lines, err := parseLedger(path, data, 0)
	if err != nil {
		return nil, nil, 0, 0, err
	}
	return out, raw, int64(used), lines, nil
}

// parseLedger reads the lines of data, numbering them on from after (the
// lines read before it), up to the first that is not a ledger entry: it
// returns the entries before that line, the bytes and lines they took, and
// that line's error. Blank lines are skipped; a last line with no LF is a
// line.
func parseLedger(path string, data []byte, after int) ([]LedgerEntry, [][]byte, int, int, error) {
	var out []LedgerEntry
	var raw [][]byte
	used, n := 0, after
	for used < len(data) {
		next := len(data)
		if i := bytes.IndexByte(data[used:], '\n'); i >= 0 {
			next = used + i + 1
		}
		if line := bytes.TrimSpace(data[used:next]); len(line) > 0 {
			e, err := parseLedgerLine(line)
			if err != nil {
				msg := "not a ledger entry: " + err.Error()
				return out, raw, used, n - after, &ConfigError{File: path, Problems: []string{fmt.Sprintf("line %d: %s", n+1, msg)}}
			}
			out = append(out, e)
			raw = append(raw, bytes.Clone(line))
		}
		used, n = next, n+1
	}
	return out, raw, used, n - after, nil
}

// parseLedgerLine reads one line by its kind: a request, a result, or a
// push.
func parseLedgerLine(line []byte) (LedgerEntry, error) {
	switch lineKind(line) {
	case ActionRequestKind:
		return decodeAction(line)
	case ActionResultKind:
		return decodeResult(line)
	}
	var e LedgerEntry
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&e); err != nil {
		return e, err
	}
	if e.Result == ResultPushed && (e.Card == nil || e.Version == nil || e.Type == nil || e.Hash == nil) {
		return e, errors.New("a pushed entry names its card, version, type and hash")
	}
	if e.At == "" || e.Feed == "" || e.Result == "" {
		return e, errors.New("a push names when, the feed and the result")
	}
	return e, nil
}

// readTail brings the in-memory ledger up to ledger.jsonl as f holds it:
// the lines appended since it was last read, the Runner's among them
// (docs/design.md 50). The caller holds ledgerMu. Unlocked (a read), a
// line counts once its LF is written, so one being written is left for the
// next read; locked (record), nothing is being written, and a last line
// without its LF is a line. A file shorter than what was read is a reset,
// read again from the start. A line that is not a ledger entry is the
// error, and nothing from it on is taken.
func (t *Tenant) readTail(f *os.File, locked bool) error {
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	from, after := t.ledgerEnd, t.ledgerLines
	reset := fi.Size() < from
	if reset {
		from, after = 0, 0
	} else if fi.Size() == from {
		return nil
	}
	data := make([]byte, fi.Size()-from)
	if _, err := f.ReadAt(data, from); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if !locked {
		data = data[:bytes.LastIndexByte(data, '\n')+1]
	}
	out, raw, used, lines, perr := parseLedger(f.Name(), data, after)
	t.mu.Lock()
	if reset {
		t.ledger, t.ledgerRaw = nil, nil
	}
	t.ledger = append(t.ledger, out...)
	t.ledgerRaw = append(t.ledgerRaw, raw...)
	t.ledgerEnd, t.ledgerLines = from+int64(used), after+lines
	t.mu.Unlock()
	return perr
}

// refresh is readTail for a read. It takes no lock on the file; when the
// file cannot be opened (the Runner holds it, on Windows) the ledger is
// served as last read. No file is a reset to an empty ledger.
func (t *Tenant) refresh() {
	t.ledgerMu.Lock()
	defer t.ledgerMu.Unlock()
	f, err := os.Open(filepath.Join(t.dir, LedgerFile))
	if errors.Is(err, os.ErrNotExist) {
		t.mu.Lock()
		t.ledger, t.ledgerRaw, t.ledgerEnd, t.ledgerLines = nil, nil, 0, 0
		t.mu.Unlock()
		return
	}
	if err != nil {
		return
	}
	defer f.Close()
	_ = t.readTail(f, false)
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

// Ledger is a copy of every entry, oldest first, the file's tail read
// first.
func (t *Tenant) Ledger() []LedgerEntry {
	t.refresh()
	t.mu.RLock()
	defer t.mu.RUnlock()
	return slices.Clone(t.ledger)
}

// LedgerLines is every entry with the hash of its line, oldest first, the
// file's tail read first.
func (t *Tenant) LedgerLines() []LedgerLine {
	t.refresh()
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
		for k, v := range map[string]string{"module": e.Module, "repo": e.Repo, "url": e.URL} {
			if v != "" {
				props[k] = v
			}
		}
		if len(e.Emits) > 0 {
			props["emits"] = emitsOf(e.Emits)
		}
		env.Nodes = append(env.Nodes, graph.Node{ID: id, Kind: "Card", Name: *e.Card, Properties: props})
		env.Edges = append(env.Edges, graph.NewEdge(FeedPrefix+e.Feed, id, "has-card"))
	}
	env.Recount()
	return env
}

// emitsOf is a card's emits as a node property: one {facet, kinds} per facet.
func emitsOf(emits []graph.ManifestEmit) []any {
	out := make([]any, len(emits))
	for i, e := range emits {
		kinds := make([]any, len(e.Kinds))
		for j, k := range e.Kinds {
			kinds[j] = k
		}
		out[i] = map[string]any{"facet": e.Facet, "kinds": kinds}
	}
	return out
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
	e.Module, e.Repo, e.URL, e.Emits = m.Module, m.Repo, m.URL, m.Emits
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

// record appends e to ledger.jsonl as the Runner appends (docs/design.md
// 50): under the file's lock (openLocked) it reads the tail first, so a
// line another writer appended since is counted, chains e to the last line
// (seq, prev), appends it (one line, one write, synced; after an LF when
// the file does not end in one) and keeps it in memory, and returns e with
// cause. A ledger that cannot be locked, read or written is the error,
// whatever cause was. ledgerMu keeps this process's appends and reads one
// at a time; a push also holds pushMu across its duplicate check. An
// action request (e.Action) is written as its ActionRequest, its id
// ledger:<seq>.
func (t *Tenant) record(e LedgerEntry, cause error) (LedgerEntry, error) {
	t.ledgerMu.Lock()
	defer t.ledgerMu.Unlock()
	fh, err := openLocked(filepath.Join(t.dir, LedgerFile), lockTimeout)
	if err != nil {
		return e, fmt.Errorf("ledger: %w", err)
	}
	defer fh.Close()
	if err := t.readTail(fh, true); err != nil {
		return e, fmt.Errorf("ledger: %w", err)
	}
	t.mu.RLock()
	e.Seq, e.Prev = len(t.ledger)+1, zeroHash
	if n := len(t.ledgerRaw); n > 0 {
		e.Prev = lineHash(t.ledgerRaw[n-1])
	}
	end := t.ledgerEnd
	t.mu.RUnlock()
	var v any = e
	if e.Action != nil {
		a := *e.Action
		a.ID, a.Seq, a.Prev = LedgerID(e.Seq), e.Seq, e.Prev
		e.Action = &a
		v = a
	}
	line, err := json.Marshal(v)
	if err != nil {
		return e, err
	}
	out := append(line, '\n')
	if end > 0 {
		last := make([]byte, 1)
		if _, err := fh.ReadAt(last, end-1); err != nil {
			return e, fmt.Errorf("ledger: %w", err)
		}
		if last[0] != '\n' {
			out = append([]byte{'\n'}, out...)
		}
	}
	_, serr := fh.Seek(end, io.SeekStart)
	_, werr := fh.Write(out)
	if err := errors.Join(serr, werr, fh.Sync()); err != nil {
		return e, fmt.Errorf("ledger: %w", err)
	}
	t.mu.Lock()
	t.ledger = append(t.ledger, e)
	t.ledgerRaw = append(t.ledgerRaw, line)
	t.ledgerEnd += int64(len(out))
	t.ledgerLines += bytes.Count(out, []byte{'\n'})
	t.mu.Unlock()
	return e, cause
}

func orNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
