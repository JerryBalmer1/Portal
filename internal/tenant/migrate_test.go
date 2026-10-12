package tenant

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The 0.16.1 id migration (docs/design.md 55) over a three-line ledger: a
// request, a push and the Runner's result, chained, the action lines in the
// legacy ledger:<seq> form.
func threeLines(t *testing.T) []string {
	t.Helper()
	req := `{"kind":"action.request","id":"ledger:1","action":"logs","subject":"env:container/claude-agent-dev/agent","principal":"ui:principal:root","requested":"2026-10-09T02:50:56.000Z","status":"requested","seq":1,"prev":"` + zeroHash + `"}`
	push := `{"at":"2026-10-09T02:50:57.000Z","principal":"ui:principal:root","name":"root","role":"admin","feed":"cards-oci","kind":"oci","card":"Tool.Git","version":"0.1.0","type":"tool","hash":"sha256:00","digest":"sha256:01","files":1,"result":"pushed","message":"","seq":2,"prev":"` + lineHash([]byte(req)) + `"}`
	res := `{"kind":"action.result","id":"ledger:3","request":"ledger:1","status":"done","started":"2026-10-09T02:50:58.600Z","finished":"2026-10-09T02:50:58.900Z","exit":0,"output":"ok\n","source":{"emitter":"Emitter.Ledger","version":"0.4.0","target":"default","stamp":"2026-10-09T02:50:58.900Z"},"seq":3,"prev":"` + lineHash([]byte(push)) + `"}`
	return []string{req, push, res}
}

func writeLedger(t *testing.T, lines []string) (string, []byte) {
	t.Helper()
	dir := t.TempDir()
	data := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(filepath.Join(dir, LedgerFile), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, data
}

func TestMigrationOfThreeLinesKeepsTheChain(t *testing.T) {
	dir, before := writeLedger(t, threeLines(t))
	m, err := migrateLedger(dir, "default", "2026-10-12T00-00-00Z")
	if err != nil {
		t.Fatal(err)
	}
	if m.Lines != 3 || filepath.Base(m.Backup) != "ledger.jsonl.2026-10-12T00-00-00Z.bak" {
		t.Errorf("migration %+v, want 3 lines (two ids, and the push's prev) and the backup beside it", m)
	}
	if b, err := os.ReadFile(m.Backup); err != nil || string(b) != string(before) {
		t.Errorf("backup %v: not the file as it was", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, LedgerFile))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(after), "\n"), "\n")
	old := threeLines(t)
	if len(got) != 3 {
		t.Fatalf("%d lines", len(got))
	}
	// Ids in the tenant's form, the old one kept right after id; request
	// too; every other member as written, in its order.
	if want := strings.Replace(old[0], `"id":"ledger:1",`, `"id":"ledger:tenant/default/1","legacyId":"ledger:1",`, 1); got[0] != want {
		t.Errorf("line 1\n got %s\nwant %s", got[0], want)
	}
	if want := strings.Replace(old[1], lineHash([]byte(old[0])), lineHash([]byte(got[0])), 1); got[1] != want {
		t.Errorf("line 2: only its prev changes\n got %s\nwant %s", got[1], want)
	}
	want := strings.Replace(old[2], `"id":"ledger:3","request":"ledger:1",`, `"id":"ledger:tenant/default/3","legacyId":"ledger:3","request":"ledger:tenant/default/1",`, 1)
	if want = strings.Replace(want, lineHash([]byte(old[1])), lineHash([]byte(got[1])), 1); got[2] != want {
		t.Errorf("line 3\n got %s\nwant %s", got[2], want)
	}
	// The chain holds: each prev is the hash of the line before as it is now,
	// and every line reads as before.
	entries, raw, _, _, err := parseLedger("", after, 0)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < len(entries); i++ {
		if entries[i].Prev != lineHash(raw[i-1]) {
			t.Errorf("line %d's prev does not name line %d", i+1, i)
		}
	}
	if a := entries[0].Action; a == nil || a.ID != "ledger:tenant/default/1" || a.LegacyID != "ledger:1" {
		t.Errorf("line 1 reads as %+v", a)
	}
	if r := entries[2].Answer; r == nil || r.ID != "ledger:tenant/default/3" || r.Request != "ledger:tenant/default/1" || r.LegacyID != "ledger:3" {
		t.Errorf("line 3 reads as %+v", r)
	}
	// Once: nothing is left to migrate, and no second copy is made.
	m, err = migrateLedger(dir, "default", "2026-10-12T00-00-01Z")
	if err != nil || m.Lines != 0 || m.Backup != "" {
		t.Errorf("second run %+v %v, want nothing", m, err)
	}
	if baks, _ := filepath.Glob(filepath.Join(dir, "*.bak")); len(baks) != 1 {
		t.Errorf("backups %v, want one", baks)
	}
}

func TestMigrationLeavesABrokenLinkBroken(t *testing.T) {
	lines := threeLines(t)
	// The push's prev names nothing: the chain breaks at line 2.
	lines[1] = strings.Replace(lines[1], lineHash([]byte(lines[0])), strings.Repeat("ab", 32), 1)
	dir, _ := writeLedger(t, lines)
	if _, err := migrateLedger(dir, "default", "2026-10-12T00-00-00Z"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, LedgerFile))
	got := strings.Split(strings.TrimSuffix(string(after), "\n"), "\n")
	if got[1] != lines[1] {
		t.Errorf("the broken line was rewritten:\n%s", got[1])
	}
	// Line 3's prev named the push as it was before it was broken, so it
	// names nothing now either: it stays as written; its ids move.
	keep := lines[2][strings.Index(lines[2], `"prev":`):]
	if !strings.HasSuffix(got[2], keep) || !strings.Contains(got[2], `"id":"ledger:tenant/default/3"`) {
		t.Errorf("line 3 %s", got[2])
	}
}

func TestMigrationLeavesAnUnreadableLedgerToTheBoot(t *testing.T) {
	lines := append(threeLines(t), `{"kind":"action.result","id":"ledger:4"}`)
	dir, before := writeLedger(t, lines)
	m, err := migrateLedger(dir, "default", "2026-10-12T00-00-00Z")
	if err != nil || m.Lines != 0 {
		t.Errorf("migration %+v %v, want nothing done", m, err)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, LedgerFile)); string(after) != string(before) {
		t.Error("an unreadable ledger was rewritten")
	}
}
