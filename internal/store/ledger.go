package store

import (
	"sort"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Ledger chains (docs/design.md 45). A chain is a ledger whose every entry
// names the hash of the entry before it: the agent's ledger, from a mounted
// layer of LedgerEntry nodes under the ids that layer gave them (ledger:<seq>,
// a prefix Portal never mints), and Portal's own push ledger,
// tenants/<tenant>/ledger.jsonl. Both are verified here the same way.
const (
	// LedgerKind is the Kind of an entry, in the agent's layer and as the
	// API serves both chains.
	LedgerKind = "LedgerEntry"
	// AgentLedgerPrefix is the id prefix of the agent ledger's entries,
	// registered to Claude.Agent.Alerts; Portal re-serves those ids.
	AgentLedgerPrefix = "ledger:"
	// ZeroHash is the prev of a chain's first entry, seq 1.
	ZeroHash = "0000000000000000000000000000000000000000000000000000000000000000"
)

// Why a chain breaks.
const (
	BreakPrev      = "prev-mismatch" // prev is not the hash of the entry before
	BreakSeq       = "seq-gap"       // seq does not follow the entry before
	BreakUnchained = "unchained"     // a line without seq and prev after the chain began
	BreakLayer     = "layer"         // the layer's own verify says so
)

// ChainEntry is one entry of a chain: its node id, seq, stamp, prev and
// hash, and the run it belongs to. Records is the id of the Run or Step its
// records edge names in the agent's layer. A line of Portal's ledger has
// its position as Seq; Written is the seq the line itself says when that
// differs (an edit by hand), and Unchained a line written before lines
// carried seq and prev. Props are the entry's own further properties as the
// API serves them.
type ChainEntry struct {
	ID        string
	Seq       int
	Stamp     string
	Prev      string
	Hash      string
	Run       string
	Records   string
	Written   int
	Unchained bool
	Name      string
	Props     map[string]any
}

// Chain is one ledger chain, oldest entry first, and its verify.
type Chain struct {
	Source string
	// Layer is the mounted layer an agent chain was read from; Stamp is that
	// layer's stamp, or the newest entry's for Portal's own.
	Layer   string
	Stamp   string
	Entries []ChainEntry
	// Claimed is the layer's own verify, from its root node, when it gives one;
	// ClaimedBreak the seq it says the chain breaks at.
	Claimed      *bool
	ClaimedBreak int
	// Valid, BreakAt and Reason are Verify's: BreakAt is the seq of the first
	// entry that does not follow the one before it.
	Valid     bool
	BreakAt   int
	Reason    string
	Unchained int
}

// Verify walks the chain oldest first. An entry follows the one before it
// when its seq is one more and its prev is that entry's hash, whether or not
// that entry is chained itself; the first entry, when its seq is 1, has prev
// ZeroHash. Unchained lines may only come before the first chained one. A
// layer that says its chain is broken is broken, at the earlier of the two
// breaks: it checked lines and keys the portal never sees.
func (c *Chain) Verify() {
	c.Valid, c.BreakAt, c.Reason, c.Unchained = true, 0, "", 0
	fail := func(seq int, why string) {
		if c.Valid {
			c.Valid, c.BreakAt, c.Reason = false, seq, why
		}
	}
	var before *ChainEntry
	chained := false
	for i := range c.Entries {
		e := &c.Entries[i]
		switch {
		case e.Unchained && !chained:
			c.Unchained++
		case e.Unchained:
			fail(e.Seq, BreakUnchained)
		case e.Written != 0 && e.Written != e.Seq:
			fail(e.Seq, BreakSeq)
		case before == nil:
			if e.Seq == 1 && e.Prev != ZeroHash {
				fail(e.Seq, BreakPrev)
			}
		case e.Seq != before.Seq+1:
			fail(e.Seq, BreakSeq)
		case e.Prev != before.Hash:
			fail(e.Seq, BreakPrev)
		}
		chained = chained || !e.Unchained
		before = e
	}
	if c.Claimed != nil && !*c.Claimed && (c.Valid || (c.ClaimedBreak > 0 && c.ClaimedBreak < c.BreakAt)) {
		c.Valid, c.BreakAt, c.Reason = false, c.ClaimedBreak, BreakLayer
	}
}

// Entry returns the entry of seq.
func (c *Chain) Entry(seq int) (ChainEntry, bool) {
	for _, e := range c.Entries {
		if e.Seq == seq {
			return e, true
		}
	}
	return ChainEntry{}, false
}

// First and Last are the lowest and highest seq, 0 for an empty chain.
func (c *Chain) First() int {
	if len(c.Entries) == 0 {
		return 0
	}
	return c.Entries[0].Seq
}

func (c *Chain) Last() int {
	if len(c.Entries) == 0 {
		return 0
	}
	return c.Entries[len(c.Entries)-1].Seq
}

// AgentLedger reads the agent's ledger chain from the mounted layer that
// holds LedgerEntry nodes under ledger: ids, the one with the latest stamp
// (the later mount on a tie): each emit is the whole ledger again. The
// chain's own verify is the layer root's valid and break. False when no
// such layer is mounted.
func (p *Portal) AgentLedger() (*Chain, bool) {
	var from *Layer
	for _, l := range p.Layers() {
		if !holdsLedger(l.Env) {
			continue
		}
		if from == nil || l.Env.Layer >= from.Env.Layer {
			from = l
		}
	}
	if from == nil {
		return nil, false
	}
	env := from.Env
	records := map[string]string{}
	for _, e := range env.Edges {
		if e.Kind == "records" {
			records[e.From] = e.To
		}
	}
	c := &Chain{Source: "agent", Layer: from.Name, Stamp: env.Layer}
	for _, n := range env.Nodes {
		if n.Kind != LedgerKind || !strings.HasPrefix(n.ID, AgentLedgerPrefix) {
			continue
		}
		seq, ok := intProp(n.Properties["seq"])
		if !ok || graph.ValidID(AgentLedgerPrefix, n.ID) != nil {
			continue // ledger:line-<n>, a line without seq, is no link of the chain
		}
		e := ChainEntry{ID: n.ID, Seq: seq, Name: n.Name, Records: records[n.ID], Props: map[string]any{}}
		e.Stamp, _ = n.Properties["stamp"].(string)
		e.Prev, _ = n.Properties["prev"].(string)
		e.Hash, _ = n.Properties["hash"].(string)
		e.Run, _ = n.Properties["run"].(string)
		if ev, ok := n.Properties["event"].(string); ok {
			e.Props["event"] = ev
		}
		c.Entries = append(c.Entries, e)
	}
	sort.SliceStable(c.Entries, func(i, j int) bool { return c.Entries[i].Seq < c.Entries[j].Seq })
	if i, ok := env.NodeIndex()[env.Root]; ok {
		root := env.Nodes[i].Properties
		if v, ok := root["valid"].(bool); ok {
			c.Claimed = &v
			c.ClaimedBreak, _ = intProp(root["break"])
		}
	}
	c.Verify()
	return c, true
}

// holdsLedger is whether a layer holds the agent's ledger: a LedgerEntry
// node under a ledger: id.
func holdsLedger(env *graph.Envelope) bool {
	for _, n := range env.Nodes {
		if n.Kind == LedgerKind && strings.HasPrefix(n.ID, AgentLedgerPrefix) {
			return true
		}
	}
	return false
}

func intProp(v any) (int, bool) {
	if !isInteger(v) {
		return 0, false
	}
	f, _ := number(v)
	return int(f), true
}
