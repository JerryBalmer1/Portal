package tenant

import "strconv"

// Ledger chains (docs/design.md 45): GET /api/tenants/<tenant>/ledger serves
// two, by source. agent is the agent's ledger, read from a mounted layer;
// portal is this tenant's own push ledger, ledger.jsonl, whose lines carry
// seq and prev since 0.10.0.
const (
	// LedgerService is the menu service whose pages keep their paths under
	// /<tenant>/ledger/<page>, beside the chains' entries at
	// /<tenant>/ledger/<source>.
	LedgerService = "ledger"
	SourceAgent   = "agent"
	SourcePortal  = "portal"
	// LedgerPrefix + source is a chain's node id; PortalEntryID gives the id
	// of an entry of the portal chain. The agent chain's entries keep the
	// ledger:<seq> ids their layer gave them.
	LedgerPrefix = "ui:ledger:"
)

// LedgerSources are the chains' sources, in the order they are listed.
var LedgerSources = []string{SourceAgent, SourcePortal}

// PortalEntryID is the id of line seq of the tenant's ledger.jsonl:
// ui:ledger:portal/<seq>.
func PortalEntryID(seq int) string {
	return LedgerPrefix + SourcePortal + "/" + strconv.Itoa(seq)
}
