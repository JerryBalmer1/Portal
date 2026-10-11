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
// ledger.jsonl; Portal.Agent's Runner executes it and records an
// action.result line in the agent's ledger; the ledger layer brings that
// line back, and Portal reads it from there only; the graph shows the gap,
// a request with no result yet. Portal never executes anything.
//
// The two lines live in two chains, so each keeps the node id its chain
// gives it: the request ui:ledger:portal/<seq> (with lineId, the
// ledger:<seq> its line carries), the result ledger:<seq> as its layer gave
// it. The result's request names the request's lineId; the join puts the
// other's seq on each, and answered-by and answers join them where both
// are served.

// ActionsID is the actions list's node id.
const ActionsID = "ui:actions"

// joinActions puts what the join knows on both chains' entries: a request
// gets outcome (requested, or its result's status), and resultId and
// resultSeq when the agent chain holds a result naming it (the newest, when
// several do); a result gets requestSeq when Portal's chain holds the
// request it names. agent is nil while no ledger layer is mounted.
func joinActions(portal, agent *store.Chain) {
	requests := map[string]int{}
	for i, e := range portal.Entries {
		if e.Props["event"] == tenant.ActionRequestKind {
			requests[e.Props["lineId"].(string)] = i
			portal.Entries[i].Props["outcome"] = tenant.ActionRequested
		}
	}
	if agent == nil {
		return
	}
	for _, r := range agent.Entries {
		if r.Props["event"] != store.ActionResultKind {
			continue
		}
		id, _ := r.Props["request"].(string)
		i, ok := requests[id]
		if !ok {
			continue
		}
		q := portal.Entries[i]
		r.Props["requestSeq"] = q.Seq
		// Entries are oldest first: a later result overrides an earlier one.
		q.Props["resultId"], q.Props["resultSeq"] = r.ID, r.Seq
		if st, ok := r.Props["status"].(string); ok && st != "" {
			q.Props["outcome"] = st
		}
	}
}

// actionPair is a request and its result, nil while none is recorded.
type actionPair struct {
	req store.ChainEntry
	res *store.ChainEntry
}

// actionPairs is every request in Portal's chain, newest first, each with
// its result from the agent chain; the chains as joined.
func (s *Server) actionPairs() (portal, agent *store.Chain, out []actionPair) {
	for _, c := range s.chains() {
		if c.Source == tenant.SourcePortal {
			portal = c
		} else {
			agent = c
		}
	}
	for _, e := range portal.Entries {
		if e.Props["event"] != tenant.ActionRequestKind {
			continue
		}
		p := actionPair{req: e}
		if seq, ok := e.Props["resultSeq"].(int); ok && agent != nil {
			if r, ok := agent.Entry(seq); ok {
				p.res = &r
			}
		}
		out = append(out, p)
	}
	slices.Reverse(out)
	return portal, agent, out
}

// addPair puts a request, its result and the two edges between them into
// env.
func addPair(env *graph.Envelope, portal, agent *store.Chain, p actionPair, runLayer map[string]string) {
	env.Nodes = append(env.Nodes, entryNode(portal, p.req, runLayer))
	if p.res == nil {
		return
	}
	env.Nodes = append(env.Nodes, entryNode(agent, *p.res, runLayer))
	env.Edges = append(env.Edges, graph.NewEdge(p.req.ID, p.res.ID, "answered-by"), graph.NewEdge(p.res.ID, p.req.ID, "answers"))
}

func (s *Server) ListActions(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	portal, agent, pairs := s.actionPairs()
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
		addPair(env, portal, agent, p, runLayer)
		env.Edges = append(env.Edges, graph.NewEdge(root.ID, p.req.ID, "has-action"))
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) GetAction(w http.ResponseWriter, r *http.Request, name api.Tenant, seq int) {
	if !s.tenantOK(w, name) {
		return
	}
	portal, agent, pairs := s.actionPairs()
	i := slices.IndexFunc(pairs, func(p actionPair) bool { return p.req.Seq == seq })
	if i < 0 {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("line %d of tenant %s's ledger is no action request; it holds %d", seq, name, len(pairs)), nil)
		return
	}
	env := servedEnvelope(time.Now().UTC().Format(store.StampLayout))
	env.Root = pairs[i].req.ID
	addPair(env, portal, agent, pairs[i], s.runLayers())
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
