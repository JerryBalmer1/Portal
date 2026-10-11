package tenant

// Ledger chains (docs/design.md 45): GET /api/tenants/<tenant>/ledger serves
// two, by source. agent is the agent's ledger, read from a mounted layer;
// portal is this tenant's ledger, ledger.jsonl, whose lines carry seq and
// prev since 0.10.0 and are appended by Portal and by Portal.Agent's Runner
// alike (docs/design.md 50): one chain, its entries ledger:<seq> (LedgerID).
const (
	// LedgerService is the menu service whose pages keep their paths under
	// /<tenant>/ledger/<page>, beside the chains' entries at
	// /<tenant>/ledger/<source>.
	LedgerService = "ledger"
	SourceAgent   = "agent"
	SourcePortal  = "portal"
	// LedgerPrefix + source is a chain's node id.
	LedgerPrefix = "ui:ledger:"
)

// LedgerSources are the chains' sources, in the order they are listed.
var LedgerSources = []string{SourceAgent, SourcePortal}
