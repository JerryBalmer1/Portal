package tenant

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Actions (docs/design.md 50). Portal asks, as the signed-in principal, for
// an action on a container; it never runs one. The ask is one line of the
// tenant's ledger.jsonl, appended through record like a push's, in the
// shape Portal.Agent's Runner reads; the Runner's answer is one more line
// of the same file, appended under the same lock with the same seq and
// prev: one ledger, one chain, one seq space. Portal reads results from
// that file only.
const (
	// ActionRequestKind is the kind of a request line, ActionResultKind
	// the kind of the Runner's answer.
	ActionRequestKind = "action.request"
	ActionResultKind  = "action.result"
	// ActionSubjectPrefix starts every subject: a container of the
	// environments graph, env:container/<project>/<service>.
	ActionSubjectPrefix = "env:container/"
	// ActionRequested is a request line's status as Portal writes it.
	ActionRequested = "requested"
	// ActionsView draws the actions page: the menu's Page whose view it is
	// (Environments/Actions), served at /<tenant>/actions.
	ActionsView = "actions"
	// ledgerIDPrefix is the prefix of the id every action line carries:
	// registered to Claude.Agent.Alerts and shared with Portal since
	// Graph.Node 0.3.3. Since 0.16.1 an action line's id is its tenant's
	// own chain, ledger:tenant/<tenant>/<seq> (Graph.Node 0.4.2,
	// docs/design.md 55); ledger:<seq> is the legacy form, migrated at
	// boot and still read.
	ledgerIDPrefix = "ledger:"
	// LedgerTenantPrefix starts a tenant chain's id: graph.LedgerTenantPrefix.
	LedgerTenantPrefix = graph.LedgerTenantPrefix
)

// ActionNames are the actions a container may be asked for; ResultStatuses
// the statuses a result may carry.
var (
	ActionNames    = []string{"logs", "stop", "restart"}
	ResultStatuses = []string{"done", "failed"}
)

// ActionRequest is one request line of ledger.jsonl, member for member the
// contract Portal.Agent's Runner reads (kind, id, action, subject,
// principal, requested, status), then seq and prev, which chain it as every
// line since 0.10.0 is chained (docs/design.md 45). ID is ledger:tenant/<tenant>/<seq>, seq
// its line number.
type ActionRequest struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Action    string `json:"action"`
	Subject   string `json:"subject"`
	Principal string `json:"principal"`
	Requested string `json:"requested"`
	Status    string `json:"status"`
	Seq       int    `json:"seq"`
	Prev      string `json:"prev"`
	// LegacyID is the id the line carried before the 0.16.1 migration,
	// ledger:<seq>; absent on a line written in the namespaced form.
	LegacyID string `json:"legacyId,omitempty"`
}

// ActionResult is one result line of ledger.jsonl, member for member the
// contract Portal.Agent's Runner writes (kind, id, request, status,
// started, finished, exit, output, source, seq, prev). Request is the
// id of the request it answers, either form; Exit and Output are pointers so a
// line without them is told from a zero. Portal never writes one.
type ActionResult struct {
	Kind     string        `json:"kind"`
	ID       string        `json:"id"`
	Request  string        `json:"request"`
	Status   string        `json:"status"`
	Started  string        `json:"started"`
	Finished string        `json:"finished"`
	Exit     *int          `json:"exit"`
	Output   *string       `json:"output"`
	Source   ResultSources `json:"source"`
	Seq      int           `json:"seq"`
	Prev     string        `json:"prev"`
	// LegacyID is the id the line carried before the 0.16.1 migration.
	LegacyID string `json:"legacyId,omitempty"`
}

// ResultSource is who wrote a result line: the emitter (Emitter.Ledger),
// its version, the tenant it wrote for and when, Graph.Node's envelope
// source block in shape.
type ResultSource struct {
	Emitter string `json:"emitter"`
	Version string `json:"version"`
	Target  string `json:"target"`
	Stamp   string `json:"stamp"`
}

// ResultSources is a result line's source: a one-item list, or the one
// object itself, as Runner 0.2.0 writes it (docs/design.md 50). Either way
// it is read as the list of one, every member set, nothing else.
type ResultSources []ResultSource

func (rs *ResultSources) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] != '[' {
		b = append(append([]byte{'['}, b...), ']')
	}
	var out []ResultSource
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if len(out) != 1 {
		return fmt.Errorf("source: one item, not %d", len(out))
	}
	if s := out[0]; s.Emitter == "" || s.Version == "" || s.Target == "" || s.Stamp == "" {
		return errors.New("source: names its emitter, version, target and stamp")
	}
	*rs = out
	return nil
}

// LedgerID is the id of line seq of tenant's ledger,
// ledger:tenant/<tenant>/<seq> (docs/design.md 55): the id an action line
// of seq carries, and the entry id of any line there.
func LedgerID(tenant string, seq int) string {
	return LedgerTenantPrefix + tenant + "/" + strconv.Itoa(seq)
}

// LedgerID is the id of line seq of this tenant's ledger.
func (t *Tenant) LedgerID(seq int) string { return LedgerID(t.Name, seq) }

// LedgerRoot is the root of tenant's chain, ledger:tenant/<tenant>.
func LedgerRoot(tenant string) string { return LedgerTenantPrefix + tenant }

// LedgerSeq reads a tenant ledger id: ledger:tenant/<tenant>/<seq>, or the
// legacy ledger:<seq> (legacy true). It returns the tenant ("" for the
// legacy form) and seq; ok is false for any other id, a chain root
// included.
func LedgerSeq(id string) (tenant string, seq int, legacy, ok bool) {
	rest, ok := strings.CutPrefix(id, ledgerIDPrefix)
	if !ok || graph.ValidID(ledgerIDPrefix, id) != nil {
		return "", 0, false, false
	}
	if r, ns := strings.CutPrefix(rest, "tenant/"); ns {
		name, n, found := strings.Cut(r, "/")
		if !found {
			return "", 0, false, false // the chain root
		}
		rest = n
		tenant = name
	} else if strings.HasPrefix(rest, "line-") {
		return "", 0, false, false
	}
	seq, err := strconv.Atoi(rest)
	if err != nil || seq < 1 {
		return "", 0, false, false
	}
	return tenant, seq, tenant == "", true
}

// NormalizeID is id in this tenant's namespaced form: a legacy ledger:<seq>
// (a line of this tenant's ledger, as the Runner wrote it before it took
// the namespaced form) becomes ledger:tenant/<tenant>/<seq>; any other id
// is returned as it is.
func (t *Tenant) NormalizeID(id string) string {
	if _, seq, legacy, ok := LedgerSeq(id); ok && legacy {
		return t.LedgerID(seq)
	}
	return id
}

// CheckAction is the request's own check: an action of ActionNames on a
// subject that is a container's id (env:container/<project>/<service>, as
// Graph.Node registers it). It does not ask whether a mounted layer holds
// the container: the Runner answers for what it can reach.
func CheckAction(action, subject string) error {
	if !slices.Contains(ActionNames, action) {
		return fmt.Errorf("%w: action %q; one of %s", ErrInvalid, action, strings.Join(ActionNames, ", "))
	}
	if !strings.HasPrefix(subject, ActionSubjectPrefix) || graph.ValidID("env:", subject) != nil {
		return fmt.Errorf("%w: subject %q is no container; it is %s<project>/<service>", ErrInvalid, subject, ActionSubjectPrefix)
	}
	return nil
}

// RequestAction appends one request line as who and returns it as written.
// A token that belongs to no principal (from before 0.5.0) cannot ask: the
// line names the principal, and there is none (ErrForbidden).
func (t *Tenant) RequestAction(who Principal, action, subject string) (ActionRequest, error) {
	if who.ID == "" {
		return ActionRequest{}, fmt.Errorf("%w: an action is asked as a principal, and %s's token belongs to none (from before 0.5.0); bootstrap a principal with New-PortalToken -Bootstrap", ErrForbidden, who.Name)
	}
	if err := CheckAction(action, subject); err != nil {
		return ActionRequest{}, err
	}
	a := ActionRequest{Kind: ActionRequestKind, Action: action, Subject: subject, Principal: who.ID,
		Requested: t.now().UTC().Format(CreatedLayout), Status: ActionRequested}
	e, err := t.record(LedgerEntry{At: a.Requested, Name: who.Name, Role: who.Role, Action: &a}, nil)
	if err != nil {
		return ActionRequest{}, err
	}
	return *e.Action, nil
}

// decodeAction reads a ledger line whose kind is action.request: every
// member of the contract, nothing else, an action and subject CheckAction
// takes, a principal, a time and a status. Its id is checked by the chain,
// not here: a line whose id names another seq is served with that id.
func decodeAction(line []byte) (LedgerEntry, error) {
	var a ActionRequest
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return LedgerEntry{}, err
	}
	if err := CheckAction(a.Action, a.Subject); err != nil {
		return LedgerEntry{}, err
	}
	if _, _, _, ok := LedgerSeq(a.ID); !ok {
		return LedgerEntry{}, fmt.Errorf("id %q is not ledger:tenant/<tenant>/<seq> (or the legacy ledger:<seq>)", a.ID)
	}
	if a.Principal == "" || a.Requested == "" || a.Status == "" {
		return LedgerEntry{}, errors.New("an action request names its principal, when it was requested and its status")
	}
	return LedgerEntry{At: a.Requested, Name: strings.TrimPrefix(a.Principal, PrincipalPrefix), Seq: a.Seq, Prev: a.Prev, Action: &a}, nil
}

// decodeResult reads a ledger line whose kind is action.result: every
// member of the contract, nothing else, ids that are LedgerSeq's, a status
// of ResultStatuses, both times, an exit and an output (empty is an
// output), and a source of one item. Whether a request of that id is in
// the chain is the join's to say, not the boot's: a result naming none is
// served, answering nothing.
func decodeResult(line []byte) (LedgerEntry, error) {
	var r ActionResult
	dec := json.NewDecoder(bytes.NewReader(line))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return LedgerEntry{}, err
	}
	for _, id := range []string{r.ID, r.Request} {
		if _, _, _, ok := LedgerSeq(id); !ok {
			return LedgerEntry{}, fmt.Errorf("id %q is not ledger:tenant/<tenant>/<seq> (or the legacy ledger:<seq>)", id)
		}
	}
	if !slices.Contains(ResultStatuses, r.Status) {
		return LedgerEntry{}, fmt.Errorf("status %q; one of %s", r.Status, strings.Join(ResultStatuses, ", "))
	}
	if r.Started == "" || r.Finished == "" || r.Exit == nil || r.Output == nil || r.Source == nil {
		return LedgerEntry{}, errors.New("an action result names when it started and finished, its exit, its output and its source")
	}
	return LedgerEntry{At: r.Finished, Name: r.Source[0].Emitter, Seq: r.Seq, Prev: r.Prev, Answer: &r}, nil
}

// lineKind is the kind member of a ledger line, "" when it has none.
func lineKind(line []byte) string {
	var k struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(line, &k)
	return k.Kind
}
