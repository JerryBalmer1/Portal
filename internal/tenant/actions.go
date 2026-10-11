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
// shape Portal.Agent's Runner reads; the Runner's answer is a line of the
// agent's own ledger, which Portal reads from the mounted ledger layer only.
const (
	// ActionRequestKind is the kind of a request line; the Runner's
	// answer is store.ActionResultKind.
	ActionRequestKind = "action.request"
	// ActionSubjectPrefix starts every subject: a container of the
	// environments graph, env:container/<project>/<service>.
	ActionSubjectPrefix = "env:container/"
	// ActionRequested is a request line's status as Portal writes it.
	ActionRequested = "requested"
	// ActionsView draws the actions page: the menu's Page whose view it is
	// (Environments/Actions), served at /<tenant>/actions.
	ActionsView = "actions"
	// ledgerIDPrefix is the prefix of the id a request line carries,
	// ledger:<seq>: registered to Claude.Agent.Alerts and shared with
	// Portal since Graph.Node 0.3.3.
	ledgerIDPrefix = "ledger:"
)

// ActionNames are the actions a container may be asked for.
var ActionNames = []string{"logs", "stop", "restart"}

// ActionRequest is one request line of ledger.jsonl, member for member the
// contract Portal.Agent's Runner reads (kind, id, action, subject,
// principal, requested, status), then seq and prev, which chain it as every
// line since 0.10.0 is chained (docs/design.md 45). ID is ledger:<seq>, seq
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
}

// LedgerID is the id a request line of seq carries: ledger:<seq>.
func LedgerID(seq int) string { return ledgerIDPrefix + strconv.Itoa(seq) }

// LedgerSeq reads ledger:<seq>; false for any other id.
func LedgerSeq(id string) (int, bool) {
	rest, ok := strings.CutPrefix(id, ledgerIDPrefix)
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(rest)
	return n, err == nil && n > 0 && strconv.Itoa(n) == rest
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
	if _, ok := LedgerSeq(a.ID); !ok {
		return LedgerEntry{}, fmt.Errorf("id %q is not ledger:<seq>", a.ID)
	}
	if a.Principal == "" || a.Requested == "" || a.Status == "" {
		return LedgerEntry{}, errors.New("an action request names its principal, when it was requested and its status")
	}
	return LedgerEntry{At: a.Requested, Name: strings.TrimPrefix(a.Principal, PrincipalPrefix), Seq: a.Seq, Prev: a.Prev, Action: &a}, nil
}

// lineKind is the kind member of a ledger line, "" when it has none.
func lineKind(line []byte) string {
	var k struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(line, &k)
	return k.Kind
}
