package server

import (
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// Ledger chains (docs/design.md 45): the agent's ledger, from the mounted
// layer of LedgerEntry nodes under the ledger:<seq> ids it gave them, and
// the tenant's own push ledger, ledger.jsonl. Both are served as a Ledger
// node ui:ledger:<source> carrying the chain's verify, and its entries.

// defaultLedgerPage and maxLedgerPage bound getLedger's limit.
const (
	defaultLedgerPage = 100
	maxLedgerPage     = 1000
)

// chains is the tenant's chains in listing order: agent, when a ledger
// layer is mounted, then portal, with each action request joined to its
// result (joinActions).
func (s *Server) chains() []*store.Chain {
	var out []*store.Chain
	portal := s.portalChain()
	agent, ok := s.p.AgentLedger()
	if ok {
		out = append(out, agent)
	}
	joinActions(portal, agent)
	return append(out, portal)
}

func (s *Server) chain(source string) (*store.Chain, bool) {
	for _, c := range s.chains() {
		if c.Source == source {
			return c, true
		}
	}
	return nil, false
}

// portalChain is ledger.jsonl as a chain: each line is the entry
// ui:ledger:portal/<n>, n its line number, with the hash of its bytes.
func (s *Server) portalChain() *store.Chain {
	c := &store.Chain{Source: tenant.SourcePortal}
	for i, l := range s.t.LedgerLines() {
		n := i + 1
		e := store.ChainEntry{ID: tenant.PortalEntryID(n), Seq: n, Stamp: l.At, Prev: l.Prev, Hash: l.Hash, Unchained: l.Seq == 0,
			Props: map[string]any{"by": l.Name, "feed": l.Feed, "result": l.Result, "message": l.Message}}
		if l.Seq != 0 && l.Seq != n {
			e.Written = l.Seq
		}
		if a := l.Action; a != nil {
			// An action request (docs/design.md 50): its line's own members,
			// lineId the id it carries.
			e.Props = map[string]any{"event": a.Kind, "lineId": a.ID, "action": a.Action, "subject": a.Subject,
				"principal": a.Principal, "by": l.Name, "status": a.Status}
			e.Name = a.Action + " " + a.Subject
			c.Entries = append(c.Entries, e)
			c.Stamp = l.At
			continue
		}
		e.Name = l.Result
		if l.Card != nil {
			e.Props["card"] = *l.Card
			e.Name += " " + *l.Card
		}
		if l.Version != nil {
			e.Props["version"] = *l.Version
			e.Name += " " + *l.Version
		}
		e.Name += " to " + l.Feed
		c.Entries = append(c.Entries, e)
		c.Stamp = l.At
	}
	c.Verify()
	return c
}

// chainNode is a chain as its Ledger node.
func chainNode(c *store.Chain) graph.Node {
	props := map[string]any{"source": c.Source, "count": len(c.Entries), "valid": c.Valid}
	if len(c.Entries) > 0 {
		props["first"], props["last"] = c.First(), c.Last()
	}
	if c.Stamp != "" {
		props["stamp"] = c.Stamp
	}
	if !c.Valid {
		props["breakAt"], props["reason"] = c.BreakAt, c.Reason
	}
	if c.Layer != "" {
		props["layer"] = c.Layer
	}
	if c.Claimed != nil {
		props["claimed"] = *c.Claimed
	}
	if c.Source == tenant.SourcePortal {
		props["unchained"] = c.Unchained
	}
	return graph.Node{ID: tenant.LedgerPrefix + c.Source, Kind: "Ledger", Name: c.Source, Properties: props}
}

// entryNode is one entry as a LedgerEntry node; runLayer names the mounted
// runs layer of each Run id.
func entryNode(c *store.Chain, e store.ChainEntry, runLayer map[string]string) graph.Node {
	props := map[string]any{"source": c.Source, "seq": e.Seq, "stamp": e.Stamp, "hash": e.Hash}
	if e.Prev != "" {
		props["prev"] = e.Prev
	}
	if e.Unchained {
		props["unchained"] = true
	}
	if e.Written != 0 {
		props["written"] = e.Written
	}
	if e.Run != "" {
		props["run"] = e.Run
		if l, ok := runLayer[e.Run]; ok {
			props["runLayer"] = l
		}
	}
	for k, v := range e.Props {
		props[k] = v
	}
	return graph.Node{ID: e.ID, Kind: store.LedgerKind, Name: e.Name, Properties: props, Source: e.Source}
}

// runLayers maps each agent Run id to the mounted layer holding it.
func (s *Server) runLayers() map[string]string {
	out := map[string]string{}
	for _, r := range s.p.AgentRuns() {
		out[r.Run.ID], _ = r.Run.Properties["layer"].(string)
	}
	return out
}

// addEntries puts entries into env under the chain node: has-entry from the
// chain, prev to the entry before when it is in env too, records to the Run
// or Step, which ends in the runs layer.
func addEntries(env *graph.Envelope, c *store.Chain, chainID string, entries []store.ChainEntry, runLayer map[string]string) {
	in := map[int]string{}
	for _, e := range entries {
		in[e.Seq] = e.ID
	}
	for _, e := range entries {
		env.Nodes = append(env.Nodes, entryNode(c, e, runLayer))
		env.Edges = append(env.Edges, graph.NewEdge(chainID, e.ID, "has-entry"))
		if before, ok := in[e.Seq-1]; ok {
			env.Edges = append(env.Edges, graph.NewEdge(e.ID, before, "prev"))
		}
		if e.Records != "" {
			env.Edges = append(env.Edges, graph.NewEdge(e.ID, e.Records, "records"))
		}
	}
}

func (s *Server) noSource(w http.ResponseWriter, name, source string) {
	var have []string
	for _, c := range s.chains() {
		have = append(have, c.Source)
	}
	writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no ledger %s in tenant %s; it has %s (agent only while a ledger layer is mounted)", source, name, strings.Join(have, ", ")), nil)
}

func (s *Server) ListLedgers(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	env := servedEnvelope(time.Now().UTC().Format(store.StampLayout))
	for _, c := range s.chains() {
		env.Nodes = append(env.Nodes, chainNode(c))
	}
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) GetLedger(w http.ResponseWriter, r *http.Request, name api.Tenant, source string, params api.GetLedgerParams) {
	if !s.tenantOK(w, name) {
		return
	}
	limit := params.Limit
	if limit == 0 {
		limit = defaultLedgerPage
	}
	if limit < 0 || limit > maxLedgerPage {
		writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("limit is 1 to %d, not %d", maxLedgerPage, params.Limit), nil)
		return
	}
	c, ok := s.chain(source)
	if !ok {
		s.noSource(w, name, source)
		return
	}
	var match []store.ChainEntry
	for _, e := range c.Entries {
		if params.Run == "" || e.Run == params.Run {
			match = append(match, e)
		}
	}
	slices.Reverse(match)
	var page []store.ChainEntry
	for _, e := range match {
		if params.Before != 0 && e.Seq >= params.Before {
			continue
		}
		if len(page) == limit {
			break
		}
		page = append(page, e)
	}
	root := chainNode(c)
	root.Properties["entries"] = len(match)
	if params.Run != "" {
		root.Properties["run"] = params.Run
	}
	if n := len(page); n > 0 && page[n-1].Seq != match[len(match)-1].Seq {
		root.Properties["next"] = page[n-1].Seq
	}
	env := servedEnvelope(time.Now().UTC().Format(store.StampLayout))
	env.Root = root.ID
	env.Nodes = append(env.Nodes, root)
	addEntries(env, c, root.ID, page, s.runLayers())
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) GetLedgerEntry(w http.ResponseWriter, r *http.Request, name api.Tenant, source string, seq int) {
	if !s.tenantOK(w, name) {
		return
	}
	c, ok := s.chain(source)
	if !ok {
		s.noSource(w, name, source)
		return
	}
	e, ok := c.Entry(seq)
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no entry %d in ledger %s of tenant %s; it holds %d to %d", seq, source, name, c.First(), c.Last()), nil)
		return
	}
	entries := []store.ChainEntry{e}
	if before, ok := c.Entry(seq - 1); ok {
		entries = append(entries, before)
	}
	root := chainNode(c)
	env := servedEnvelope(time.Now().UTC().Format(store.StampLayout))
	env.Root = e.ID
	env.Nodes = append(env.Nodes, root)
	runLayer := s.runLayers()
	addEntries(env, c, root.ID, entries, runLayer)
	s.addAnswer(env, e, runLayer)
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// addAnswer puts the other half of an action into env beside e: a
// request's result, joined answered-by, or a result's request, joined
// answers, when the other chain holds it (docs/design.md 50).
func (s *Server) addAnswer(env *graph.Envelope, e store.ChainEntry, runLayer map[string]string) {
	portal, agent, pairs := s.actionPairs()
	for _, p := range pairs {
		if p.res == nil || (p.req.ID != e.ID && p.res.ID != e.ID) {
			continue
		}
		if p.req.ID == e.ID {
			env.Nodes = append(env.Nodes, entryNode(agent, *p.res, runLayer))
		} else {
			env.Nodes = append(env.Nodes, entryNode(portal, p.req, runLayer))
		}
		env.Edges = append(env.Edges, graph.NewEdge(p.req.ID, p.res.ID, "answered-by"), graph.NewEdge(p.res.ID, p.req.ID, "answers"))
		return
	}
}

// ledgerCounts counts the agent chain's entries by run, or is nil when no
// ledger layer is mounted.
func (s *Server) ledgerCounts() map[string]int {
	c, ok := s.p.AgentLedger()
	if !ok {
		return nil
	}
	out := map[string]int{}
	for _, e := range c.Entries {
		if e.Run != "" {
			out[e.Run]++
		}
	}
	return out
}

// GetLedgerPage serves index.html at /<tenant>/ledger; the shell draws it
// under the Page whose view is ledger (Ledger/Entries).
func (s *Server) GetLedgerPage(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		s.index(w)
	}
}

// ledgerMenuPage is whether the menu has the page ledger/<page>, whose path
// the literal ledger routes keep.
func (s *Server) ledgerMenuPage(page string) bool {
	i, ok := s.t.Menu.NodeIndex()[tenant.RailID(tenant.LedgerService+"/"+page)]
	return ok && s.t.Menu.Nodes[i].Kind == "Page"
}

// GetLedgerSourcePage serves index.html at a chain's entries,
// /<tenant>/ledger/<source>, and at each page of the ledger service, which
// keeps its path: the literal route wins over getShellPage's, and boot
// refuses a ledger page with a source's id.
func (s *Server) GetLedgerSourcePage(w http.ResponseWriter, r *http.Request, name api.Tenant, source string) {
	if !s.tenantOK(w, name) {
		return
	}
	if _, ok := s.chain(source); !ok && !s.ledgerMenuPage(source) {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no ledger %s in tenant %s, and no page %s/%s in its menu", source, name, tenant.LedgerService, source), nil)
		return
	}
	s.index(w)
}

// GetLedgerEntryPage serves index.html at one entry,
// /<tenant>/ledger/<source>/<seq>, and at an item of a page of the ledger
// service, as getShellItem would.
func (s *Server) GetLedgerEntryPage(w http.ResponseWriter, r *http.Request, name api.Tenant, source, seq string) {
	if !s.tenantOK(w, name) {
		return
	}
	if s.ledgerMenuPage(source) {
		s.index(w)
		return
	}
	c, ok := s.chain(source)
	n, err := strconv.Atoi(seq)
	if ok && err == nil {
		_, ok = c.Entry(n)
	}
	if !ok || err != nil {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no entry %s in ledger %s of tenant %s", seq, source, name), nil)
		return
	}
	s.index(w)
}
