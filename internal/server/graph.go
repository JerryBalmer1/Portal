package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
)

// The tenant graph (docs/design.md 53): every node and edge of the mounted
// layers, or, with source, of the layers whose source block names that
// emitter. The filter is the URL's, never the page's state, so a filtered
// graph is a link: /<tenant>/graph?source=Emitter.Terraform. The envelope
// names no ontology: its nodes keep their layers' Kinds.

func (s *Server) GetTenantGraph(w http.ResponseWriter, r *http.Request, name api.Tenant, params api.GetTenantGraphParams) {
	if !s.tenantOK(w, name) {
		return
	}
	env := graph.NewEnvelope(portal.Module, portal.Version, time.Now().UTC().Format(store.StampLayout))
	nodes, edges := s.p.TenantGraph(params.Source)
	env.Nodes = append(env.Nodes, nodes...)
	env.Edges = append(env.Edges, edges...)
	env.Recount()
	writeJSON(w, http.StatusOK, env)
}

// GetGraphPage serves index.html at /<tenant>/graph; the shell draws it
// under the Page whose view is graph (Graph/Nodes) and reads source itself.
func (s *Server) GetGraphPage(w http.ResponseWriter, r *http.Request, name api.Tenant, _ api.GetGraphPageParams) {
	if s.tenantOK(w, name) {
		s.index(w)
	}
}

// The tenant graph query (docs/design.md 54): one breadth-first walk over
// every mounted layer, served as an envelope with the paths it took and the
// query as resolved. Empty is 200; a from id no node has is 400, named.

// queryResult is the envelope with what the query adds beside it.
type queryResult struct {
	*graph.Envelope
	Paths       [][]string `json:"paths"`
	Query       queryEcho  `json:"query"`
	Truncated   bool       `json:"truncated"`
	TruncatedAt *int       `json:"truncatedAt,omitempty"`
}

type queryEcho struct {
	From      []string `json:"from"`
	Kind      string   `json:"kind,omitempty"`
	Follow    []string `json:"follow"`
	Direction string   `json:"direction"`
	Depth     int      `json:"depth"`
	Until     string   `json:"until,omitempty"`
	Where     []string `json:"where"`
	Source    string   `json:"source,omitempty"`
	Limit     int      `json:"limit"`
}

func (s *Server) QueryTenantGraph(w http.ResponseWriter, r *http.Request, name api.Tenant, params api.QueryTenantGraphParams) {
	if !s.tenantOK(w, name) {
		return
	}
	walk, err := parseWalk(r.URL.Query(), params)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return
	}
	res, err := s.p.GraphQuery(walk)
	var unknown *store.UnknownNodeError
	if errors.As(err, &unknown) {
		writeError(w, http.StatusBadRequest, "unknown_node", "from: "+unknown.Error(), nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	env := graph.NewEnvelope(portal.Module, portal.Version, time.Now().UTC().Format(store.StampLayout))
	env.Nodes = append(env.Nodes, res.Nodes...)
	env.Edges = append(env.Edges, res.Edges...)
	env.Recount()
	out := queryResult{Envelope: env, Paths: res.Paths, Truncated: res.Truncated, Query: queryEcho{
		From: walk.From, Kind: walk.Kind, Follow: walk.Follow, Direction: walk.Direction, Depth: walk.Depth,
		Until: walk.Until, Where: []string{}, Source: walk.Source, Limit: walk.Limit,
	}}
	if out.Paths == nil {
		out.Paths = [][]string{}
	}
	for _, f := range walk.Where {
		out.Query.Where = append(out.Query.Where, f.String())
	}
	if res.Truncated {
		out.TruncatedAt = &res.TruncatedAt
	}
	writeJSON(w, http.StatusOK, out)
}

// parseWalk resolves the query's parameters, defaults filled in. Depth and
// limit are read from q too, since an absent one and 0 bind alike.
func parseWalk(q url.Values, params api.QueryTenantGraphParams) (store.Walk, error) {
	w := store.Walk{
		From: splitList(params.From), Kind: params.Kind, Follow: splitList(params.Follow), Direction: string(params.Direction),
		Depth: store.WalkDepth, Until: params.Until, Source: params.Source, Limit: store.WalkLimit,
	}
	switch {
	case len(w.From) == 0 && w.Kind == "":
		return w, errors.New("give from (start node ids) or kind")
	case len(w.From) > 0 && w.Kind != "":
		return w, errors.New("give from or kind, not both")
	}
	switch w.Direction {
	case "":
		w.Direction = store.DirOut
	case store.DirOut, store.DirIn, store.DirBoth:
	default:
		return w, fmt.Errorf("direction %q is not out, in or both", w.Direction)
	}
	if q.Has("depth") {
		if params.Depth < 0 || params.Depth > store.WalkMaxDepth {
			return w, fmt.Errorf("depth %d is not 0 to %d", params.Depth, store.WalkMaxDepth)
		}
		w.Depth = params.Depth
	}
	if q.Has("limit") {
		if params.Limit < 1 || params.Limit > store.WalkMaxLimit {
			return w, fmt.Errorf("limit %d is not 1 to %d", params.Limit, store.WalkMaxLimit)
		}
		w.Limit = params.Limit
	}
	for _, raw := range params.Where {
		f, err := store.ParseWhere(raw)
		if err != nil {
			return w, err
		}
		w.Where = append(w.Where, f)
	}
	return w, nil
}

// splitList reads a comma-separated list, empty items dropped; never nil.
func splitList(s string) []string {
	out := []string{}
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
