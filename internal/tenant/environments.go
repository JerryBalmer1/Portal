package tenant

const (
	// EnvironmentsService is the menu service whose page graph draws the
	// environments graph (docs/design.md 46).
	EnvironmentsService = "environments"
	// EnvironmentsGraphPage is that page's id: /<tenant>/environments/graph,
	// the selected node after #, the address every Show graph links to.
	EnvironmentsGraphPage = "graph"
	// EnvironmentsGraphView is the view that draws it, and the only view
	// the page may have.
	EnvironmentsGraphView = "environments-graph"
)
