package tenant

const (
	// ComplianceService is the menu service whose page alerts lists the
	// alerts (docs/design.md 47).
	ComplianceService = "compliance"
	// AlertsPage is that page's id: /<tenant>/compliance/alerts, where the
	// banner's alert count links and each alert's item page sits.
	AlertsPage = "alerts"
	// AlertsView is the view that draws it, and the only view the page may
	// have.
	AlertsView = "alerts"
)
