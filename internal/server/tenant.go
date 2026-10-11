package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/JerryBalmer1/Portal/internal/gen/api"
	"github.com/JerryBalmer1/Portal/internal/store"
	"github.com/JerryBalmer1/Portal/internal/tenant"
)

type principalKey struct{}

// auth reads the bearer token before any route: no token reads only, a
// token the tenant holds is its principal, any other token is refused
// (401), on a read too, so a mistyped token never passes as anonymous. A
// write needs a principal (401) whose role writes (403: a reader reads
// only). Identity writes check admin and attenuation in their handlers
// (docs/design.md 38).
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		who, err := s.t.Authenticate(r.Header.Get("Authorization"))
		if errors.Is(err, tenant.ErrDevOnly) {
			unauthorized(w, "the bearer token belongs to "+tenant.DevPrincipalName+", whose tokens work only while tenant "+s.t.Name+"'s mode is dev; it is "+s.t.Mode())
			return
		}
		if err != nil {
			unauthorized(w, "the bearer token is not one tenant "+s.t.Name+" holds (tenants/"+s.t.Name+"/tokens.yaml)")
			return
		}
		if !safeMethod(r.Method) {
			if who.Anonymous {
				unauthorized(w, r.Method+" writes, and a write needs a bearer token; without one the portal reads only")
				return
			}
			if !who.Writes() {
				forbidden(w, fmt.Sprintf("%s writes, and %s's role %s reads only", r.Method, who.Name, who.Role))
				return
			}
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

func forbidden(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusForbidden, "forbidden", msg, nil)
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

// GetTenant names the tenant and its mode (docs/design.md 39).
func (s *Server) GetTenant(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		writeJSON(w, http.StatusOK, api.TenantInfo{Name: s.t.Name, Mode: api.TenantInfoMode(s.t.Mode())})
	}
}

// GetDevToken hands the dev-bootstrap token to the page's "Use dev token",
// and is 404 unless all of these hold at the time of the request: the
// tenant's mode is dev, the binary was given a token (PORTAL_TOKEN) that is
// one of dev-bootstrap's and still in tokens.yaml, and the caller and the
// Host it asked for are loopback. The listener is 127.0.0.1 already; the
// Host check keeps a page on another origin that rebinds its name to
// 127.0.0.1 from reading it (docs/design.md 39).
func (s *Server) GetDevToken(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	p, ok := s.t.DevToken(s.devToken)
	if !ok || !loopback(r) {
		writeError(w, http.StatusNotFound, "not_found", "no dev token: it exists only in mode dev, from 127.0.0.1, on a portal started by Invoke-Build Dev", nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, api.DevToken{Token: s.devToken, Principal: p.ID, Role: p.Role})
}

// loopback is whether the request came from a loopback address and named a
// loopback host (127.0.0.1, ::1 or localhost, any port).
func loopback(r *http.Request) bool {
	addr, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	if ip := net.ParseIP(addr); ip == nil || !ip.IsLoopback() {
		return false
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
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
		ID          *string `json:"id"`
		Name        string  `json:"name"`
		Type        *string `json:"type"`
		Role        *string `json:"role"`
		Anonymous   bool    `json:"anonymous"`
		Legacy      bool    `json:"legacy"`
		Writes      bool    `json:"writes"`
		Administers bool    `json:"administers"`
		Tenant      string  `json:"tenant"`
		Marker      *string `json:"marker"`
	}{orNull(who.ID), who.Name, orNull(who.Type), orNull(who.Role), who.Anonymous, who.Legacy, who.Writes(), who.Administers(), s.t.Name, nil})
}

// orNull is s, or JSON null when s is empty.
func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *Server) ListPrincipals(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if s.tenantOK(w, name) {
		writeJSON(w, http.StatusOK, s.t.PrincipalsEnvelope(time.Now().UTC().Format(store.StampLayout)))
	}
}

func (s *Server) PostPrincipal(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	body, ok := readBody(w, r)
	if !ok {
		return
	}
	var req api.PrincipalRequest
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "principal body is {name, role, type?}: "+err.Error(), nil)
		return
	}
	p, err := s.t.AddPrincipal(principal(r), req.Name, req.Type, req.Role)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	env := s.t.PrincipalsEnvelope(time.Now().UTC().Format(store.StampLayout))
	writeJSON(w, http.StatusCreated, env.Nodes[env.NodeIndex()[p.ID]])
}

// tokenJSON is a TokenSummary with its nullable fields as JSON null.
type tokenJSON struct {
	ID        string  `json:"id"`
	Principal *string `json:"principal"`
	Name      string  `json:"name"`
	Created   *string `json:"created"`
	Legacy    bool    `json:"legacy"`
}

func tokenOut(k tenant.TokenSummary) tokenJSON {
	return tokenJSON{k.ID, orNull(k.Principal), k.Name, orNull(k.Created), k.Legacy}
}

func (s *Server) ListTokens(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	out := []tokenJSON{}
	for _, k := range s.t.TokenList() {
		out = append(out, tokenOut(k))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) RevokeToken(w http.ResponseWriter, r *http.Request, name api.Tenant, id string) {
	if !s.tenantOK(w, name) {
		return
	}
	k, err := s.t.RevokeToken(principal(r), id)
	if err != nil {
		writeIdentityError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenOut(k))
}

// writeIdentityError maps tenant identity errors onto the spec's responses.
func writeIdentityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, tenant.ErrForbidden):
		forbidden(w, err.Error())
	case errors.Is(err, tenant.ErrExists):
		writeError(w, http.StatusConflict, "conflict", err.Error(), nil)
	case errors.Is(err, tenant.ErrInvalid):
		writeError(w, http.StatusUnprocessableEntity, "invalid", err.Error(), nil)
	case errors.Is(err, tenant.ErrNoSuchToken):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), nil)
	default:
		writeError(w, http.StatusInternalServerError, "internal", err.Error(), nil)
	}
}

func (s *Server) ListConnections(w http.ResponseWriter, r *http.Request, name api.Tenant) {
	if !s.tenantOK(w, name) {
		return
	}
	getenv := s.gh.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	// The generated ConnectionSummary cannot carry createdBy as null.
	type summary struct {
		api.ConnectionSummary
		CreatedBy *string `json:"createdBy"`
	}
	out := []summary{}
	for _, c := range s.t.Connections {
		out = append(out, summary{api.ConnectionSummary{Name: c.Name, Type: api.ConnectionSummaryType(c.Type), Url: c.URL, TokenEnv: c.TokenEnv,
			TokenSet: getenv(c.TokenEnv) != ""}, orNull(c.CreatedBy)})
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
