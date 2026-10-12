package server

import (
	"fmt"
	"strings"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

// The tenant-actions layer (docs/design.md 55): the tenant's ledger.jsonl,
// Portal's action chain, mounted like any other layer, so the tenant graph
// and its query reach an action's request, result, subject and principal.
// Portal builds it from the file at boot and again whenever the file has
// grown, its own appends and the Runner's alike (store.Derived); it is
// never written and never pushed by Portal.Agent.
//
// Nodes are the ledger's lines, as GET .../ledger?source=portal serves them
// (LedgerEntry, ledger:tenant/<tenant>/<seq>), and each principal a request
// names (Principal, ui:principal:<name>). Edges:
//   - prev, a line to the line before it (newer to older), for every line
//     that carries seq and prev;
//   - request, a result to the request it answers, when that is a line;
//   - subject, a request to the container it asks about (env:container/...,
//     a node of the environments layer when that is mounted);
//   - principal, a request to the principal that asked.
//
// Its source block names Emitter.Ledger, at the version the mounted agent
// ledger layer's source block carries, so the roll call lists both layers
// under one card; with no agent ledger layer stamped Emitter.Ledger, at the
// version of the newest Emitter.Ledger card pushed to the tenant's feeds;
// with neither, it has no source block and is listed under its module,
// unstamped. It is mounted while the ledger holds at least one action line.

// ActionsLayer is the tenant-actions layer's name.
const ActionsLayer = "tenant-actions"

// LedgerEmitter is the card the layer's source block names.
const LedgerEmitter = "Emitter.Ledger"

// actionsLayer is the store.Derived build of the tenant-actions layer.
func (s *Server) actionsLayer(others []*store.Layer) (string, func() (*graph.Envelope, error)) {
	lines := s.t.LedgerLines()
	actions := 0
	for _, l := range lines {
		if l.Action != nil || l.Answer != nil {
			actions++
		}
	}
	version := s.ledgerEmitterVersion(others)
	last := ""
	if n := len(lines); n > 0 {
		last = lines[n-1].Hash
	}
	key := fmt.Sprintf("%d|%s|%d|%s|%d", len(lines), last, actions, version, len(s.t.Principals()))
	return key, func() (*graph.Envelope, error) {
		if actions == 0 {
			return nil, nil
		}
		return s.actionsEnvelope(version), nil
	}
}

// ledgerEmitterVersion is the version the layer's source block carries: the
// agent ledger layer's, when its source block names Emitter.Ledger, else the
// newest pushed Emitter.Ledger card's; "" for neither.
func (s *Server) ledgerEmitterVersion(others []*store.Layer) string {
	if l := store.AgentLedgerLayer(others); l != nil && l.Env.Source != nil && l.Env.Source.Emitter == LedgerEmitter && l.Env.Source.Version != "" {
		return l.Env.Source.Version
	}
	version, pushed := "", ""
	for _, n := range s.t.FeedsEnvelope(time.Now().UTC().Format(store.StampLayout)).Nodes {
		if n.Kind != "Card" || n.Name != LedgerEmitter || n.Properties["type"] != tenant.EmitterType {
			continue
		}
		at, _ := n.Properties["pushed"].(string)
		v, _ := n.Properties["version"].(string)
		if v != "" && at >= pushed {
			version, pushed = v, at
		}
	}
	return version
}

// actionsEnvelope builds the layer from the chain as served.
func (s *Server) actionsEnvelope(version string) *graph.Envelope {
	c := s.portalChain()
	joinActions(c)
	stamp := time.Now().UTC().Format(store.StampLayout)
	if c.Stamp != "" {
		stamp = c.Stamp
	}
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	if version != "" {
		env.Source = &graph.EnvelopeSource{Emitter: LedgerEmitter, Version: version, Target: s.t.Name, Stamp: stamp}
	}
	principals := map[string]graph.Node{}
	for _, n := range s.t.PrincipalsEnvelope(stamp).Nodes {
		principals[n.ID] = n
	}
	ids := map[string]bool{}
	for _, e := range c.Entries {
		ids[e.ID] = true
	}
	added := map[string]bool{}
	var people []graph.Node
	for i, e := range c.Entries {
		env.Nodes = append(env.Nodes, entryNode(c, e, nil))
		if i > 0 && !e.Unchained {
			env.Edges = append(env.Edges, graph.NewEdge(e.ID, c.Entries[i-1].ID, "prev"))
		}
		switch e.Props["event"] {
		case tenant.ActionResultKind:
			if req, _ := e.Props["request"].(string); ids[req] {
				env.Edges = append(env.Edges, graph.NewEdge(e.ID, req, "request"))
			}
		case tenant.ActionRequestKind:
			if subject, _ := e.Props["subject"].(string); subject != "" {
				env.Edges = append(env.Edges, graph.NewEdge(e.ID, subject, "subject"))
			}
			who, _ := e.Props["principal"].(string)
			if who == "" {
				continue
			}
			if !added[who] {
				added[who] = true
				n, ok := principals[who]
				if !ok {
					n = graph.Node{ID: who, Kind: "Principal", Name: strings.TrimPrefix(who, tenant.PrincipalPrefix)}
				}
				people = append(people, n)
			}
			env.Edges = append(env.Edges, graph.NewEdge(e.ID, who, "principal"))
		}
	}
	env.Nodes = append(env.Nodes, people...)
	if n := len(c.Entries); n > 0 {
		env.Root = c.Entries[n-1].ID
	}
	env.Recount()
	return env
}
