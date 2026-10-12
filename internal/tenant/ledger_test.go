package tenant

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	portal "github.com/JerryBalmer1/Portal"
)

// TestRecordWaitsForTheRunnersLock: while another writer holds the file's
// lock (openLocked, the Runner's FileShare.None), an ask waits; the line
// that writer appends under the lock is read before seq and prev are
// computed, so the ask is chained to it (docs/design.md 50).
func TestRecordWaitsForTheRunnersLock(t *testing.T) {
	dir := writeTenant(t, map[string]string{"portal.yaml": "{}\n"})
	tt, err := Load(Options{Dir: dir, Base: portal.BaseYAML, Schema: portal.ConfigSchema, Stamp: "2026-10-11T00:00:00.000Z"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tt.RequestAction(admin, "logs", "env:container/claude-agent-dev/agent"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tt.dir, LedgerFile)
	held, err := openLocked(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan ActionRequest, 1)
	go func() {
		a, err := tt.RequestAction(admin, "stop", "env:container/claude-agent-dev/agent")
		if err != nil {
			t.Error(err)
		}
		done <- a
	}()
	select {
	case a := <-done:
		t.Fatalf("asked while the lock was held: %+v", a)
	case <-time.After(300 * time.Millisecond):
	}
	data, err := readAll(held)
	if err != nil {
		t.Fatal(err)
	}
	result := `{"kind":"action.result","id":"ledger:2","request":"ledger:1","status":"done","started":"2026-10-11T22:00:00.000Z","finished":"2026-10-11T22:00:01.000Z","exit":0,"output":"","source":[{"emitter":"Emitter.Ledger","version":"0.4.0","target":"default","stamp":"2026-10-11T22:00:01.000Z"}],"seq":2,"prev":"` + lineHash(trimLF(data)) + `"}`
	if _, err := held.WriteAt([]byte(result+"\n"), int64(len(data))); err != nil {
		t.Fatal(err)
	}
	held.Close()
	select {
	case a := <-done:
		if a.ID != "ledger:tenant/default/3" || a.Seq != 3 || a.Prev != lineHash([]byte(result)) {
			t.Errorf("ask after the Runner's line %+v, want ledger:tenant/default/3 chained to it", a)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the ask never ran once the lock was released")
	}
	lines := tt.Ledger()
	if len(lines) != 3 || lines[1].Answer == nil || lines[1].Answer.Request != "ledger:1" {
		t.Errorf("ledger %+v, want request, result, request", lines)
	}
}

// readAll reads f from its start, through the held handle: on Windows the
// file opens to no one else while it is held, the test included.
func readAll(f *os.File) ([]byte, error) {
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	b := make([]byte, fi.Size())
	_, err = f.ReadAt(b, 0)
	return b, err
}

func trimLF(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
