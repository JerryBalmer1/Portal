// Package server implements the generated api.ServerInterface over a
// store.Portal. Routes come only from internal/gen (api/openapi.yaml);
// nothing here registers a path by hand. Shape lifted from GraphRenderer's
// internal/server (docs/design.md 6).
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/web"
)

// maxBody caps a request body; a mounted envelope is the largest.
const maxBody = 64 << 20

// Server serves api/openapi.yaml.
type Server struct {
	p   *store.Portal
	web fs.FS
}

var _ api.ServerInterface = (*Server)(nil)

// New returns the handler serving p and the embedded page.
func New(p *store.Portal) http.Handler {
	s := &Server{p: p, web: web.FS}
	r := chi.NewRouter()
	r.NotFound(notFound)
	r.MethodNotAllowed(methodNotAllowed)
	return api.HandlerWithOptions(s, api.ChiServerOptions{BaseRouter: r, ErrorHandlerFunc: paramError})
}

// Serve listens on host:port, prints the bound URL (the PowerShell module
// and the tests read it), and shuts down when ctx ends.
func Serve(ctx context.Context, host string, port int, h http.Handler) error {
	ln, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	fmt.Printf("portal listening on http://%s\n", ln.Addr())
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) GetIndex(w http.ResponseWriter, r *http.Request) {
	data, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	writeAsset(w, "text/html", data)
}

func (s *Server) GetStatic(w http.ResponseWriter, r *http.Request, file string) {
	data, ctype, err := web.Static(s.web, file)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no static file "+file, nil)
		return
	}
	writeAsset(w, ctype, data)
}

func (s *Server) GetScreen(w http.ResponseWriter, r *http.Request) {
	env, err := s.p.Screen()
	if err != nil {
		writeStoreError(w, err)
		return
	}
	w.Header().Set("X-Portal-Boot", s.p.Boot())
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) PostNode(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	n, err := s.p.AddNode(body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) PostEdge(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	e, err := s.p.AddEdge(body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) PatchNode(w http.ResponseWriter, r *http.Request, id api.NodeId) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	n, st, err := s.p.PatchNode(id, body)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	out := struct {
		Node  *graph.Node `json:"node"`
		State *graph.Node `json:"state,omitempty"`
	}{n, st}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) DeleteNode(w http.ResponseWriter, r *http.Request, id api.NodeId) {
	n, edges, err := s.p.DeleteNode(id)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Node  *graph.Node `json:"node"`
		Edges []string    `json:"edges"`
	}{n, edges})
}

func (s *Server) ListLayers(w http.ResponseWriter, r *http.Request) {
	out := []api.LayerSummary{}
	for _, l := range s.p.Layers() {
		out = append(out, summary(l))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) MountLayer(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var m store.Mount
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "mount body is {name, path} or {name, envelope}: "+err.Error(), nil)
		return
	}
	l, err := s.p.MountLayer(m)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, summary(l))
}

func (s *Server) GetLayerNodes(w http.ResponseWriter, r *http.Request, name string, params api.GetLayerNodesParams) {
	env, err := s.p.Query(name, params.Kind, params.From)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

func (s *Server) PostSnap(w http.ResponseWriter, r *http.Request) {
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var req api.SnapRequest
	if len(bytes.TrimSpace(body)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "snap body is {note} or nothing: "+err.Error(), nil)
			return
		}
	}
	a, err := s.p.TakeSnap(req.Note)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	// The generated SnapResult cannot leave diff out; this one can.
	out := struct {
		api.SnapResult
		Previous string           `json:"previous,omitempty"`
		Diff     *api.DiffSummary `json:"diff,omitempty"`
	}{SnapResult: api.SnapResult{
		Stamp: a.Snap.Stamp, Layer: a.Snap.Layer, Path: a.Snap.Path, Counts: counts(a.Snap.Env.Counts), Note: a.Note,
	}, Previous: a.Previous}
	if a.Diff != nil {
		out.Diff = summarise(a.Diff)
	}
	writeJSON(w, http.StatusCreated, out)
}

// summarise keeps the ids of a diff.
func summarise(d *store.Diff) *api.DiffSummary {
	ids := func(nodes []graph.Node, edges []graph.Edge) api.Ids {
		out := api.Ids{Nodes: []string{}, Edges: []string{}}
		for _, n := range nodes {
			out.Nodes = append(out.Nodes, n.ID)
		}
		for _, e := range edges {
			out.Edges = append(out.Edges, e.ID)
		}
		return out
	}
	changed := api.Ids{Nodes: []string{}, Edges: []string{}}
	for _, c := range d.ChangedNodes {
		changed.Nodes = append(changed.Nodes, c.ID)
	}
	for _, c := range d.ChangedEdges {
		changed.Edges = append(changed.Edges, c.ID)
	}
	return &api.DiffSummary{
		Added:   ids(d.AddedNodes, d.AddedEdges),
		Removed: ids(d.RemovedNodes, d.RemovedEdges),
		Changed: changed,
	}
}

func (s *Server) GetDiff(w http.ResponseWriter, r *http.Request, params api.GetDiffParams) {
	a, err := s.p.LoadSnap(params.A)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	b, err := s.p.LoadSnap(params.B)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	d, err := store.DiffSnaps(a, b)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	type delta struct {
		Nodes []graph.Node `json:"nodes"`
		Edges []graph.Edge `json:"edges"`
	}
	type changes struct {
		Nodes []store.Change `json:"nodes"`
		Edges []store.Change `json:"edges"`
	}
	writeJSON(w, http.StatusOK, struct {
		A       string          `json:"a"`
		B       string          `json:"b"`
		Added   delta           `json:"added"`
		Removed delta           `json:"removed"`
		Changed changes         `json:"changed"`
		Merged  *graph.Envelope `json:"merged"`
	}{d.A, d.B, delta{d.AddedNodes, d.AddedEdges}, delta{d.RemovedNodes, d.RemovedEdges},
		changes{d.ChangedNodes, d.ChangedEdges}, d.Merged})
}

func (s *Server) GetOntology(w http.ResponseWriter, r *http.Request) {
	layers := map[string]*map[string]interface{}{}
	for name, doc := range s.p.LayerOntologies() {
		if doc == nil {
			layers[name] = nil
			continue
		}
		layers[name] = &doc
	}
	writeJSON(w, http.StatusOK, api.OntologySet{Portal: s.p.Ontology().Raw, Layers: layers})
}

func summary(l *store.Layer) api.LayerSummary {
	return api.LayerSummary{
		Name: l.Name, Module: l.Env.Module, Version: l.Env.Version, Stamp: l.Env.Layer, Path: l.Path,
		Counts: counts(recount(l.Env)), HasOntology: l.Ontology != nil, OntologyProblems: problems(l.OntologyProblems),
	}
}

func recount(env *graph.Envelope) *graph.Counts {
	c := &graph.Counts{Nodes: len(env.Nodes), Edges: len(env.Edges), ByKind: map[string]int{}}
	for _, n := range env.Nodes {
		c.ByKind[n.Kind]++
	}
	return c
}

func counts(c *graph.Counts) api.Counts {
	if c == nil {
		return api.Counts{}
	}
	return api.Counts{Nodes: c.Nodes, Edges: c.Edges, ByKind: c.ByKind}
}

func problems(ps []graph.Problem) []api.Problem {
	if len(ps) == 0 {
		return nil
	}
	out := make([]api.Problem, len(ps))
	for i, p := range ps {
		out[i] = api.Problem{Code: p.Code, Path: p.Path, Message: p.Message, Severity: api.ProblemSeverity(p.Severity)}
	}
	return out
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
		return nil, false
	}
	return body, true
}

// writeStoreError maps store errors onto the spec's error responses.
func writeStoreError(w http.ResponseWriter, err error) {
	var inv *store.InvalidError
	switch {
	case errors.As(err, &inv):
		writeError(w, http.StatusUnprocessableEntity, "invalid", inv.Message, problems(inv.Problems))
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, store.ErrBadRequest):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

func writeAsset(w http.ResponseWriter, ctype string, data []byte) {
	w.Header().Set("Content-Type", ctype+"; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(data)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string, ps []api.Problem) {
	writeJSON(w, status, api.Error{Code: code, Message: msg, Problems: ps})
}

// paramError answers parameter-binding failures from the generated wrapper
// with the spec Error body instead of plain text.
func paramError(w http.ResponseWriter, r *http.Request, err error) {
	writeError(w, http.StatusBadRequest, "bad_request", err.Error(), nil)
}

func notFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "no route "+r.Method+" "+r.URL.Path, nil)
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", r.Method+" "+r.URL.Path, nil)
}
