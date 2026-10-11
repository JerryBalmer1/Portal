package server

import (
	"fmt"
	"net/http"
	"slices"
	"sort"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
)

// Alerts (docs/design.md 47): the alert nodes of the mounted layers,
// served as Portal's Alert under the layer's own id alert/<fingerprint>,
// one page under the list node ui:alerts, with each alert's subject (its
// about edge) and Run (raised-in) as resolver serves them. The envelope
// names no ontology, as the environments graph's does: a subject keeps
// its layer's Kind. Nothing here writes; Portal never writes an alert.

// alertsID is the list's node; defaultAlertPage and maxAlertPage bound
// listAlerts' limit.
const (
	alertsID         = "ui:alerts"
	defaultAlertPage = 100
	maxAlertPage     = 1000
)

// alertNode is an alert as Portal's Alert.
func alertNode(a store.Alert) graph.Node {
	props := map[string]any{"fingerprint": a.Fingerprint, "severity": a.Severity, "rule": a.Rule, "summary": a.Summary,
		"stamp": a.Stamp, "open": a.Open(), "layer": a.Layer}
	if a.State != "" {
		props["state"] = a.State
	}
	if a.About != "" {
		props["about"] = a.About
	}
	if a.RaisedIn != "" {
		props["run"] = a.RaisedIn
	}
	return graph.Node{ID: a.Node.ID, Kind: "Alert", Name: a.Node.Name, Properties: props, Stamp: a.Node.Stamp}
}

// addAlerts puts the alerts into env with their subjects and Runs, and
// their about and raised-in edges to the ones Portal serves; from, when
// set, joins each to it by has-alert.
func (s *Server) addAlerts(env *graph.Envelope, stamp, from string, list []store.Alert) {
	var ends []string
	for _, a := range list {
		env.Nodes = append(env.Nodes, alertNode(a))
		if from != "" {
			env.Edges = append(env.Edges, graph.NewEdge(from, a.Node.ID, "has-alert"))
		}
		for _, id := range []string{a.About, a.RaisedIn} {
			if id != "" && !slices.Contains(ends, id) {
				ends = append(ends, id)
			}
		}
	}
	served := s.resolver(stamp).resolve(ends)
	for _, id := range ends {
		if n, ok := served[id]; ok {
			env.Nodes = append(env.Nodes, n)
		}
	}
	for _, a := range list {
		if _, ok := served[a.About]; ok {
			env.Edges = append(env.Edges, graph.NewEdge(a.Node.ID, a.About, "about"))
		}
		if _, ok := served[a.RaisedIn]; ok {
			env.Edges = append(env.Edges, graph.NewEdge(a.Node.ID, a.RaisedIn, "raised-in"))
		}
	}
}

// alertsRoot is the list node over every alert: its count, the open ones
// by severity, and every rule and state the alerts carry.
func alertsRoot(all []store.Alert) graph.Node {
	open := map[string]any{}
	for _, s := range store.Severities {
		open[s] = 0
	}
	rules, states := []string{}, []string{}
	for _, a := range all {
		if a.Open() {
			n, _ := open[a.Severity].(int)
			open[a.Severity] = n + 1
		}
		if a.Rule != "" && !slices.Contains(rules, a.Rule) {
			rules = append(rules, a.Rule)
		}
		state := a.State
		if state == "" {
			state = store.AlertOpen
		}
		if !slices.Contains(states, state) {
			states = append(states, state)
		}
	}
	sort.Strings(rules)
	sort.Strings(states)
	return graph.Node{ID: alertsID, Kind: "Alerts", Name: "alerts",
		Properties: map[string]any{"count": len(all), "open": open, "rules": rules, "states": states}}
}

// alertFilter is listAlerts' filters.
type alertFilter api.ListAlertsParams

// matches is whether a passes them.
func (p alertFilter) matches(a store.Alert) bool {
	state := a.State
	if state == "" {
		state = store.AlertOpen
	}
	return (p.Severity == "" || a.Severity == p.Severity) && (p.Rule == "" || a.Rule == p.Rule) &&
		(p.State == "" || state == p.State) && (p.Run == "" || a.RaisedIn == p.Run) && (p.About == "" || a.About == p.About)
}

func (s *Server) ListAlerts(w http.ResponseWriter, r *http.Request, name api.Tenant, params api.ListAlertsParams) {
	if !s.tenantOK(w, name) {
		return
	}
	limit := params.Limit
	if limit == 0 {
		limit = defaultAlertPage
	}
	if limit < 0 || limit > maxAlertPage {
		writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("limit is 1 to %d, not %d", maxAlertPage, params.Limit), nil)
		return
	}
	all := s.p.Alerts()
	// after is a place in the order: the alert it names, which may since
	// have left the filter (its state changed), is still found in all.
	var after *store.Alert
	if params.After != "" {
		for i := range all {
			if all[i].Fingerprint == params.After {
				after = &all[i]
			}
		}
		if after == nil {
			writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("after names no alert of tenant %s: it is the next of the page before, a fingerprint", name), nil)
			return
		}
	}
	f := alertFilter(params)
	var match, page []store.Alert
	for _, a := range all {
		if f.matches(a) {
			match = append(match, a)
		}
	}
	for _, a := range match {
		if after != nil && !store.AlertBefore(*after, a) {
			continue
		}
		if len(page) == limit {
			break
		}
		page = append(page, a)
	}
	root := alertsRoot(all)
	root.Properties["alerts"] = len(match)
	for k, v := range map[string]string{"severity": params.Severity, "rule": params.Rule, "state": params.State, "run": params.Run, "about": params.About} {
		if v != "" {
			root.Properties[k] = v
		}
	}
	if n := len(page); n > 0 && page[n-1].Fingerprint != match[len(match)-1].Fingerprint {
		root.Properties["next"] = page[n-1].Fingerprint
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Root = alertsID
	env.Nodes = append(env.Nodes, root)
	s.addAlerts(env, stamp, alertsID, page)
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) GetAlert(w http.ResponseWriter, r *http.Request, name api.Tenant, fingerprint string) {
	if !s.tenantOK(w, name) {
		return
	}
	all := s.p.Alerts()
	i := slices.IndexFunc(all, func(a store.Alert) bool { return a.Fingerprint == fingerprint })
	if i < 0 {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no alert %s%s in tenant %s; its mounted layers hold %d", store.AlertIDPrefix, fingerprint, name, len(all)), nil)
		return
	}
	stamp := time.Now().UTC().Format(store.StampLayout)
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	env.Root = all[i].Node.ID
	s.addAlerts(env, stamp, "", all[i:i+1])
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// alertCounts counts the alerts raised in each Run, or is nil when no
// alert is mounted.
func (s *Server) alertCounts() map[string]int {
	all := s.p.Alerts()
	if len(all) == 0 {
		return nil
	}
	out := map[string]int{}
	for _, a := range all {
		if a.RaisedIn != "" {
			out[a.RaisedIn]++
		}
	}
	return out
}
