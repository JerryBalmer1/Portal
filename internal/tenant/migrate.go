package tenant

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// The 0.16.1 id migration (docs/design.md 55). Until 0.16.0 an action line
// of the tenant's ledger carried ledger:<seq>, the same id as line seq of
// the agent's chain; mounted in one graph they collided. At boot, before
// the ledger is read, every action line still in that form is rewritten in
// place, under the file's lock: its id and its request (a result's) become
// ledger:tenant/<tenant>/<seq>, keeping the number they name, and the old
// id stays on the line as legacyId, after id. A rewritten line has new
// bytes, so the prev of the line after it, the SHA-256 of the line before
// (docs/design.md 45), is rewritten to the new hash, and so on down the
// file; a prev that did not name the line before it is left as it was, so
// a chain that was broken stays broken where it was. Every other member,
// and the members' order, stay as written. Before the file is rewritten
// its bytes are copied beside it, ledger.jsonl.<stamp>.bak. A ledger with
// no legacy line is not touched and gets no copy.

// LedgerMigration says what the boot's migration did: how many lines it
// rewrote and where it copied the file first; zero when it did nothing.
type LedgerMigration struct {
	Lines  int
	Backup string
}

// BackupLayout stamps a migration's copy: UTC, no colons, so it is a file
// name on Windows.
const BackupLayout = "2006-01-02T15-04-05Z"

// migrateLedger runs the migration on dir's ledger.jsonl for tenant.
func migrateLedger(dir, tenant, stamp string) (LedgerMigration, error) {
	path := dir + string(os.PathSeparator) + LedgerFile
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return LedgerMigration{}, nil
	}
	fh, err := openLocked(path, lockTimeout)
	if err != nil {
		return LedgerMigration{}, fmt.Errorf("ledger: %w", err)
	}
	defer fh.Close()
	data, err := io.ReadAll(fh)
	if err != nil {
		return LedgerMigration{}, fmt.Errorf("ledger: %w", err)
	}
	out, n, err := migrateLines(data, tenant)
	if err != nil {
		return LedgerMigration{}, &ConfigError{File: path, Problems: []string{"id migration: " + err.Error()}}
	}
	if n == 0 {
		return LedgerMigration{}, nil
	}
	backup, err := writeBackup(path+"."+stamp, data)
	if err != nil {
		return LedgerMigration{}, fmt.Errorf("ledger: backup before the id migration: %w", err)
	}
	if err := fh.Truncate(0); err != nil {
		return LedgerMigration{}, fmt.Errorf("ledger: %w", err)
	}
	if _, err := fh.WriteAt(out, 0); err != nil {
		return LedgerMigration{}, fmt.Errorf("ledger: %w (the file as it was is %s)", err, backup)
	}
	if err := fh.Sync(); err != nil {
		return LedgerMigration{}, fmt.Errorf("ledger: %w (the file as it was is %s)", err, backup)
	}
	return LedgerMigration{Lines: n, Backup: backup}, nil
}

// writeBackup writes data to base.bak, or base.<n>.bak for the first n from
// 2 that is free: a copy is never overwritten.
func writeBackup(base string, data []byte) (string, error) {
	for n := 1; ; n++ {
		path := base + ".bak"
		if n > 1 {
			path = fmt.Sprintf("%s.%d.bak", base, n)
		}
		err := writeNew(path, data)
		if !errors.Is(err, os.ErrExist) {
			return path, err
		}
	}
}

// writeNew writes a file that must not exist yet.
func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Sync(), f.Close())
}

// migrateLines is the migration over a ledger's bytes: the bytes it would
// write and how many lines differ. Lines keep their order and their LFs; a
// blank line stays blank. A ledger with a line that is no ledger entry is
// not migrated: the boot then stops on that line as it always has. Each
// rewritten line must still read as a ledger entry, or nothing is
// rewritten.
func migrateLines(data []byte, tenant string) ([]byte, int, error) {
	if _, _, _, _, err := parseLedger("", data, 0); err != nil {
		return data, 0, nil
	}
	segs := bytes.Split(data, []byte{'\n'})
	oldPrev, newPrev := zeroHash, zeroHash
	changed := 0
	for i, seg := range segs {
		line := bytes.TrimSpace(seg)
		if len(line) == 0 {
			continue
		}
		next, err := migrateLine(line, tenant, oldPrev, newPrev)
		if err != nil {
			return nil, 0, fmt.Errorf("line %d: %w", i+1, err)
		}
		oldPrev, newPrev = lineHash(line), lineHash(next)
		if !bytes.Equal(next, line) {
			if _, err := parseLedgerLine(next); err != nil {
				return nil, 0, fmt.Errorf("line %d as rewritten is no ledger entry: %w", i+1, err)
			}
			segs[i] = next
			changed++
		}
	}
	return bytes.Join(segs, []byte{'\n'}), changed, nil
}

// migrateLine rewrites one line: a legacy id (and request), and a prev
// that named the old hash of the line before, oldPrev, to its new one.
func migrateLine(line []byte, tenant, oldPrev, newPrev string) ([]byte, error) {
	var m struct {
		ID      *string `json:"id"`
		Request *string `json:"request"`
		Prev    *string `json:"prev"`
	}
	kind := lineKind(line)
	if err := json.Unmarshal(line, &m); err != nil {
		return nil, err
	}
	out := line
	var err error
	if kind == ActionRequestKind || kind == ActionResultKind {
		if m.ID != nil {
			if _, seq, legacy, ok := LedgerSeq(*m.ID); ok && legacy {
				extra := `,"legacyId":` + jsonQuote(*m.ID)
				if out, err = setMember(out, "id", *m.ID, LedgerID(tenant, seq), extra); err != nil {
					return nil, err
				}
			}
		}
		if kind == ActionResultKind && m.Request != nil {
			if _, seq, legacy, ok := LedgerSeq(*m.Request); ok && legacy {
				if out, err = setMember(out, "request", *m.Request, LedgerID(tenant, seq), ""); err != nil {
					return nil, err
				}
			}
		}
	}
	if m.Prev != nil && *m.Prev == oldPrev && oldPrev != newPrev {
		if out, err = setMember(out, "prev", *m.Prev, newPrev, ""); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// setMember replaces the one member key whose value is the string old with
// the string val, then extra; the line must hold exactly one such member.
// A member inside a string value cannot match: there its quotes are
// escaped.
func setMember(line []byte, key, old, val, extra string) ([]byte, error) {
	re := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `"\s*:\s*` + regexp.QuoteMeta(jsonQuote(old)))
	if n := len(re.FindAllIndex(line, -1)); n != 1 {
		return nil, fmt.Errorf("%d members %q of value %s; want one", n, key, jsonQuote(old))
	}
	return re.ReplaceAllLiteral(line, []byte(`"`+key+`":`+jsonQuote(val)+extra)), nil
}

// jsonQuote is s as a JSON string, as encoding/json writes it.
func jsonQuote(s string) string {
	b, _ := json.Marshal(s)
	return strings.TrimSpace(string(b))
}
