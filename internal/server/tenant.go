package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

type principalKey struct{}

// auth reads the bearer token before any route: no token reads only, a
// token the tenant holds may write, any other token is refused (401), on a
// read too, so a mistyped token never passes as anonymous.
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, err := s.t.Authenticate(r.Header.Get("Authorization"))
		if err != nil {
			unauthorized(w, "the bearer token is not one tenant "+s.t.Name+" holds (tenants/"+s.t.Name+"/tokens.yaml)")
			return
		}
		if who.Anonymous && !safeMethod(r.Method) {
			unauthorized(w, r.Method+" writes, and a write needs a bearer token; without one the portal reads only")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, who)))
	})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func principal(r *http.Request) tenant.Principal {
	if who, ok := r.Context().Value(principalKey{}).(tenant.Principal); ok {
		return who
	}
	return tenant.Anonymous
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="portal"`)
	writeError(w, http.StatusUnauthorized, "unauthorized", msg, nil)
}

// tenantOK answers 404 unless name is the tenant this process serves.
func (s *Server) tenantOK(w http.ResponseWriter, name string) bool {
	if name == s.t.Name {
		return true
	}
	writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no tenant %s; this portal serves %s", name, s.t.Name), nil)
	return false
}

// GetIndex sends the viewer to the first page of the first service.
func (s *Server) GetIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/"+s.t.Name+"/"+s.firstPage(), http.StatusFound)
}

// firstPage is the path of the first Page of the first Service not docked
// at the bottom, both by order.
func (s *Server) firstPage() string {
	env := s.t.Menu
	idx := env.NodeIndex()
	order := func(id string) int {
		v, _ := env.Nodes[idx[id]].Properties["order"].(int)
		return v
	}
	var services []string
	for _, n := range env.Nodes {
		if n.Kind == "Service" && n.Properties["dock"] != "bottom" {
			services = append(services, n.ID)
		}
	}
	sort.SliceStable(services, func(i, j int) bool { return order(services[i]) < order(services[j]) })
	for _, svc := range services {
		var pages []string
		for _, e := range env.Edges {
			if e.Kind == "has-page" && e.From == svc {
				pages = append(pages, e.To)
			}
		}
		sort.SliceStable(pages, func(i, j int) bool { return order(pages[i]) < order(pages[j]) })
		if len(pages) > 0 {
			path, _ := env.Nodes[idx[pages[0]]].Properties["path"].(string)
			return path
		}
	}
	return ""
}

func (s *Server) GetShellPage(w http.ResponseWriter, r *http.Request, name api.Tenant, service, page string) {
	s.shell(w, name, service, page)
}

func (s *Server) GetShellItem(w http.ResponseWriter, r *http.Request, name api.Tenant, service, page, id string) {
	s.shell(w, name, service, page)
}

// shell serves index.html at the route of a page the menu has; the page
// reads the route from its URL, so a deep link survives a refresh.
func (s *Server) shell(w http.ResponseWriter, name, service, page string) {
	if !s.tenantOK(w, name) {
		return
	}
	i, ok := s.t.Menu.NodeIndex()[tenant.RailID(service+"/"+page)]
	if !ok || s.t.Menu.Nodes[i].Kind != "Page" {
		writeError(w, http.StatusNotFound, "not_found", fmt.Sprintf("no page %s/%s in tenant %s's menu", service, page, name), nil)
		return
	}
	data, err := fs.ReadFile(s.web, "index.html")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
		return
	}
	writeAsset(w, "text/html", data)
}

func (s *Server) GetConfig(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		writeJSON(w, http.StatusOK, s.t.Config)
	}
}

func (s *Server) GetMenu(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		writeJSON(w, http.StatusOK, s.t.Menu)
	}
}

func (s *Server) GetWhoami(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	who := principal(r)
	writeJSON(w, http.StatusOK, struct {
		Name      string  `json:"name"`
		Anonymous bool    `json:"anonymous"`
		Writes    bool    `json:"writes"`
		Tenant    string  `json:"tenant"`
		Marker    *string `json:"marker"`
	}{who.Name, who.Anonymous, !who.Anonymous, s.t.Name, nil})
}

func (s *Server) ListConnections(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	getenv := s.gh.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	out := []api.ConnectionSummary{}
	for _, c := range s.t.Connections {
		out = append(out, api.ConnectionSummary{Name: c.Name, Type: api.ConnectionSummaryType(c.Type), Url: c.URL, TokenEnv: c.TokenEnv, TokenSet: getenv(c.TokenEnv) != ""})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) GetConnectionRepos(w http.ResponseWriter, r *http.Request, name api.Tenant, conn string) {
	if !s.tenantOK(w, name) {
		return
	}
	// A read, but it spends the tenant's secret for the connection, so it
	// needs a principal like a write does (docs/design.md 36).
	if principal(r).Anonymous {
		unauthorized(w, "listing a connection's repositories uses the tenant's "+conn+" credential and needs a bearer token")
		return
	}
	c, err := s.t.Connection(conn)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
		return
	}
	env, err := s.gh.Repos(r.Context(), c, time.Now().UTC().Format(store.StampLayout))
	var up *tenant.UpstreamError
	switch {
	case errors.Is(err, tenant.ErrNoToken):
		writeError(w, http.StatusUnprocessableEntity, "no_token", err.Error(), nil)
	case errors.As(err, &up):
		writeError(w, http.StatusBadGateway, "upstream", fmt.Sprintf("%s (%s): %s", c.Name, c.URL, up.Error()), nil)
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	default:
		writeJSON(w, http.StatusOK, env)
	}
}

func (s *Server) ListNodes(w http.ResponseWriter, r *http.Request, name api.Tenant, params api.ListNodesParams) {
	if !s.tenantOK(w, name) {
		return
	}
	if params.Kind == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "kind names the Kind to list", nil)
		return
	}
	env, err := s.p.NodesOfKind(params.Kind)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, env)
}

// MountLayerUnscoped is mountLayer at its pre-0.4.0 path, for this
// process's tenant.
func (s *Server) MountLayerUnscoped(w http.ResponseWriter, r *http.Request) {
	s.MountLayer(w, r, s.t.Name)
}
