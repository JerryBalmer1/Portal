package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// Actions (docs/design.md 50): four roles. Portal asks, as the principal
// the token names, by appending an action.request line to the tenant's
// ledger.jsonl; Portal.Agent's Runner executes it and appends an
// action.result line to the same file, under the same lock, chained by the
// same seq and prev; the graph shows the gap, a request with no result
// yet. Portal never executes anything.
//
// One ledger, one seq space: the request and its result are entries of
// Portal's chain under the ids their lines carry,
// ledger:tenant/<tenant>/<seq> (docs/design.md 55). The
// result's request names the request's id; the join puts the other's seq
// on each, and answered-by and answers join them where both are served.
// Results are read from ledger.jsonl only, never from a mounted layer.

// ActionsID is the actions list's node id.
const ActionsID = "ui:actions"

// joinActions puts what the join knows on the chain's action entries,
// oldest first: a request gets outcome (requested, or its result's
// status), and resultId and resultSeq when a later line is a result naming
// it (the newest, when several do); a result gets requestSeq when it names
// a request before it.
func joinActions(portal *store.Chain) {
	requests := map[string]int{}
	for i, e := range portal.Entries {
		switch e.Props["event"] {
		case tenant.ActionRequestKind:
			requests[e.ID] = i
			e.Props["outcome"] = tenant.ActionRequested
		case tenant.ActionResultKind:
			id, _ := e.Props["request"].(string)
			j, ok := requests[id]
			if !ok {
				continue
			}
			q := portal.Entries[j]
			e.Props["requestSeq"] = q.Seq
			q.Props["resultId"], q.Props["resultSeq"] = e.ID, e.Seq
			if st, ok := e.Props["status"].(string); ok && st != "" {
				q.Props["outcome"] = st
			}
		}
	}
}

// actionPair is a request and its result, nil while none is recorded.
type actionPair struct {
	req store.ChainEntry
	res *store.ChainEntry
}

// actionPairs is every request in Portal's chain, newest first, each with
// its result from the same chain; the chain as joined.
func (s *Server) actionPairs() (portal *store.Chain, out []actionPair) {
	portal = s.portalChain()
	joinActions(portal)
	for _, e := range portal.Entries {
		if e.Props["event"] != tenant.ActionRequestKind {
			continue
		}
		p := actionPair{req: e}
		if seq, ok := e.Props["resultSeq"].(int); ok {
			if r, ok := portal.Entry(seq); ok {
				p.res = &r
			}
		}
		out = append(out, p)
	}
	slices.Reverse(out)
	return portal, out
}

// addPair puts a request, its result and the two edges between them into
// env.
func addPair(env *graph.Envelope, portal *store.Chain, p actionPair, runLayer map[string]string) {
	env.Nodes = append(env.Nodes, entryNode(portal, p.req, runLayer))
	if p.res == nil {
		return
	}
	env.Nodes = append(env.Nodes, entryNode(portal, *p.res, runLayer))
	env.Edges = append(env.Edges, graph.NewEdge(p.req.ID, p.res.ID, "answered-by"), graph.NewEdge(p.res.ID, p.req.ID, "answers"))
}

func (s *Server) ListActions(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	portal, pairs := s.actionPairs()
	counts := map[string]int{tenant.ActionRequested: 0, "done": 0, "failed": 0}
	for _, p := range pairs {
		if o, ok := p.req.Props["outcome"].(string); ok {
			if _, known := counts[o]; known {
				counts[o]++
			}
		}
	}
	root := graph.Node{ID: ActionsID, Kind: "Actions", Name: "actions", Properties: map[string]any{
		"count": len(pairs), "requested": counts[tenant.ActionRequested], "done": counts["done"], "failed": counts["failed"]}}
	env := servedEnvelope(time.Now().UTC().Format(store.StampLayout))
	env.Root = root.ID
	env.Nodes = append(env.Nodes, root)
	runLayer := s.runLayers()
	for _, p := range pairs {
		addPair(env, portal, p, runLayer)
		env.Edges = append(env.Edges, graph.NewEdge(root.ID, p.req.ID, "has-action"))
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) GetAction(w http.ResponseWriter, r *http.Request, name api.Tenant, seq int) {
	if !s.tenantOK(w, name) {
		return
	}
	portal, pairs := s.actionPairs()
	i := slices.IndexFunc(pairs, func(p actionPair) bool { return p.req.Seq == seq })
	if i < 0 {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("line %d of tenant %s's ledger is no action request; it holds %d", seq, name, len(pairs)), nil)
		return
	}
	env := servedEnvelope(time.Now().UTC().Format(store.StampLayout))
	env.Root = pairs[i].req.ID
	addPair(env, portal, pairs[i], s.runLayers())
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// PostAction asks for an action on a container as the token's principal:
// the body names the action and the subject, never who asks. The
// middleware has already refused anonymous (401) and a reader (403).
func (s *Server) PostAction(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var req api.ActionAsk
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "an action is {action, subject}; the principal comes from the token: "+err.Error(), nil)
		return
	}
	a, err := s.t.RequestAction(principal(r), req.Action, req.Subject)
	switch {
	case err == nil:
		writeJSON(w, http.StatusCreated, a)
	case errors.Is(err, tenant.ErrForbidden):
		forbidden(w, err.Error())
	case errors.Is(err, tenant.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid", err.Error(), nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

// GetActionsPage serves index.html at /<tenant>/actions; the shell draws it
// under the Page whose view is actions (Environments/Actions).
func (s *Server) GetActionsPage(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		s.index(w)
	}
}
