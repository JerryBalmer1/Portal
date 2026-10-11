package store

import (
	"sort"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Alerts (docs/design.md 47): the alert nodes of the mounted layers, which
// Portal.Agent's agent-alerts layer emits and Portal only reads. An alert is
// a node of Kind alert under the plain id alert/<fingerprint>, with severity,
// rule, summary, stamp, state and fingerprint, an about edge to what it is
// about (a ledger entry, a Run, a PolicyDecision, an environment instance)
// and a raised-in edge to the Run it was raised in.
const (
	// AlertKind is the Kind of an alert in its layer.
	AlertKind = "alert"
	// AlertIDPrefix starts every alert's id; the fingerprint follows it.
	AlertIDPrefix = "alert/"
	// AlertOpen is the state of an alert no one has acted on. An alert
	// with no state is open too.
	AlertOpen = "open"
)

// Severities in the order the page sorts them, the most severe first.
var Severities = []string{"critical", "warning", "info"}

// SeverityRank is a severity's place in Severities; any other value sorts
// after them all.
func SeverityRank(s string) int {
	for i, v := range Severities {
		if v == s {
			return i
		}
	}
	return len(Severities)
}

// Alert is one alert as its layer emits it, and the ends of its edges.
type Alert struct {
	Node graph.Node
	// Layer is the mounted layer it was read from.
	Layer       string
	Fingerprint string
	Severity    string
	Rule        string
	Summary     string
	Stamp       string
	State       string
	// About and RaisedIn are the ids its about and raised-in edges name, or
	// "" when it has none; the first of each when it has several.
	About    string
	RaisedIn string
}

// Open is whether no one has acted on the alert: state open, or none.
func (a Alert) Open() bool { return a.State == "" || a.State == AlertOpen }

// IsAlertLayer is whether a layer holds alerts. Its other nodes are copies
// of what its alerts are about, there so that an about edge does not
// dangle (an environment instance's id is plain, and ledger:<seq> is
// Claude.Agent.Alerts' own prefix): Portal reads them as references only,
// never as the agent's ledger or as environments.
func IsAlertLayer(env *graph.Envelope) bool {
	for _, n := range env.Nodes {
		if n.Kind == AlertKind && strings.HasPrefix(n.ID, AlertIDPrefix) {
			return true
		}
	}
	return false
}

// Alerts reads every alert of the mounted layers, read-only, the most
// severe first, then the newest (by stamp), then by fingerprint. An alert
// is re-emitted when its state changes, so a fingerprint in two layers is
// kept once, from the layer with the later stamp, the later mount on a tie.
// A node of Kind alert whose id is not alert/<fingerprint> is skipped; one
// with no stamp gets stamp.layer from its layer.
func (p *Portal) Alerts() []Alert {
	byFP := map[string]Alert{}
	stamp := map[string]string{}
	for _, l := range p.Layers() {
		env := l.Env
		about := map[string]string{}
		raised := map[string]string{}
		for _, e := range env.Edges {
			switch e.Kind {
			case "about":
				if _, ok := about[e.From]; !ok {
					about[e.From] = e.To
				}
			case "raised-in":
				if _, ok := raised[e.From]; !ok {
					raised[e.From] = e.To
				}
			}
		}
		for _, n := range env.Nodes {
			fp, ok := strings.CutPrefix(n.ID, AlertIDPrefix)
			if n.Kind != AlertKind || !ok || fp == "" {
				continue
			}
			if s, ok := stamp[fp]; ok && s > env.Layer {
				continue
			}
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: env.Layer}
			}
			a := Alert{Node: n, Layer: l.Name, Fingerprint: fp, About: about[n.ID], RaisedIn: raised[n.ID]}
			a.Severity, _ = n.Properties["severity"].(string)
			a.Rule, _ = n.Properties["rule"].(string)
			a.Summary, _ = n.Properties["summary"].(string)
			a.Stamp, _ = n.Properties["stamp"].(string)
			a.State, _ = n.Properties["state"].(string)
			byFP[fp] = a
			stamp[fp] = env.Layer
		}
	}
	out := make([]Alert, 0, len(byFP))
	for _, a := range byFP {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return AlertBefore(out[i], out[j]) })
	return out
}

// AlertBefore is the order Alerts sorts in: severity (SeverityRank), then
// the newer stamp, then the fingerprint.
func AlertBefore(a, b Alert) bool {
	if ra, rb := SeverityRank(a.Severity), SeverityRank(b.Severity); ra != rb {
		return ra < rb
	}
	if a.Stamp != b.Stamp {
		return a.Stamp > b.Stamp
	}
	return a.Fingerprint < b.Fingerprint
}

// HeldNodes is, for each id of want, the node as the first mounted layer
// holding it emits it, with stamp.layer from that layer when it has no
// stamp. An alert layer's copies are not held (IsAlertLayer).
func (p *Portal) HeldNodes(want map[string]bool) map[string]graph.Node {
	out := map[string]graph.Node{}
	if len(want) == 0 {
		return out
	}
	for _, l := range p.Layers() {
		if IsAlertLayer(l.Env) {
			continue
		}
		for _, n := range l.Env.Nodes {
			if _, ok := out[n.ID]; ok || !want[n.ID] {
				continue
			}
			if n.Stamp == nil {
				n.Stamp = &graph.Stamp{Layer: l.Env.Layer}
			}
			out[n.ID] = n
		}
	}
	return out
}
