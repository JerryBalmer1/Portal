package tenant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"

	portal "github.com/JerryBalmer1/Portal"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Connection is one entry of connections.yaml. TokenEnv names the
// environment variable holding the secret; the secret is read when a call
// needs it and never stored or served.
type Connection struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	URL      string `yaml:"url" json:"url"`
	TokenEnv string `yaml:"token_env" json:"token_env"`
}

// ConnectionTypes are the types a connection may have.
var ConnectionTypes = []string{"github"}

// GitHubAPI is a github connection's url when it names none.
const GitHubAPI = "https://api.github.com"

var (
	connName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	envName  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Errors of a connection call, mapped by the server onto 404, 422 and 502.
var (
	ErrNoConnection = errors.New("no such connection")
	ErrNoToken      = errors.New("token not set")
)

// UpstreamError is a failed call to the service a connection names.
type UpstreamError struct {
	Status  int
	Message string
}

func (e *UpstreamError) Error() string {
	if e.Status == 0 {
		return e.Message
	}
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}

func loadConnections(path string) ([]Connection, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Connections []Connection `yaml:"connections"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return nil, &ConfigError{File: path, Problems: []string{err.Error()}}
	}
	var ps []string
	names := map[string]bool{}
	for i := range doc.Connections {
		c := &doc.Connections[i]
		at := fmt.Sprintf("/connections/%d", i)
		if !connName.MatchString(c.Name) {
			ps = append(ps, fmt.Sprintf("%s/name: %q must match %s", at, c.Name, connName))
		}
		if names[c.Name] {
			ps = append(ps, fmt.Sprintf("%s/name: %q twice", at, c.Name))
		}
		names[c.Name] = true
		switch c.Type {
		case "github":
			if c.URL == "" {
				c.URL = GitHubAPI
			}
		default:
			ps = append(ps, fmt.Sprintf("%s/type: %q; one of %s", at, c.Type, strings.Join(ConnectionTypes, ", ")))
		}
		if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			ps = append(ps, fmt.Sprintf("%s/url: %q is not an http(s) URL", at, c.URL))
		}
		if !envName.MatchString(c.TokenEnv) {
			ps = append(ps, fmt.Sprintf("%s/token_env: %q names no environment variable", at, c.TokenEnv))
		}
	}
	if len(ps) > 0 {
		return nil, &ConfigError{File: path, Problems: ps}
	}
	return doc.Connections, nil
}

// Connection returns the named connection.
func (t *Tenant) Connection(name string) (Connection, error) {
	for _, c := range t.Connections {
		if c.Name == name {
			return c, nil
		}
	}
	return Connection{}, fmt.Errorf("%w: %s", ErrNoConnection, name)
}

// GitHub calls one github connection. Client and Getenv are injectable for
// tests; nil means http.DefaultClient and os.Getenv.
type GitHub struct {
	Client *http.Client
	Getenv func(string) string
}

// maxRepoPages caps how far GitHub's pagination is followed: 1000 repos.
const maxRepoPages = 10

// Repos lists the repositories the connection's token can see (GET
// <url>/user/repos, every page up to 1000) as a Graph.Node envelope, one
// node of Kind git.repository per repository, id
// ui:conn:<connection>/<owner>/<repo>.
func (g GitHub) Repos(ctx context.Context, c Connection, stamp string) (*graph.Envelope, error) {
	getenv := g.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	client := g.Client
	if client == nil {
		client = http.DefaultClient
	}
	secret := getenv(c.TokenEnv)
	if secret == "" {
		return nil, fmt.Errorf("%w: connection %s reads its token from %s, which is not set", ErrNoToken, c.Name, c.TokenEnv)
	}
	env := graph.NewEnvelope(portal.Module, portal.Version, stamp)
	for page := 1; page <= maxRepoPages; page++ {
		u := strings.TrimRight(c.URL, "/") + fmt.Sprintf("/user/repos?per_page=100&page=%d&sort=full_name", page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, &UpstreamError{Message: err.Error()}
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("Authorization", "Bearer "+secret)
		res, err := client.Do(req)
		if err != nil {
			return nil, &UpstreamError{Message: err.Error()}
		}
		body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
		res.Body.Close()
		if err != nil {
			return nil, &UpstreamError{Message: err.Error()}
		}
		if res.StatusCode != http.StatusOK {
			var e struct {
				Message string `json:"message"`
			}
			_ = json.Unmarshal(body, &e)
			if e.Message == "" {
				e.Message = http.StatusText(res.StatusCode)
			}
			return nil, &UpstreamError{Status: res.StatusCode, Message: e.Message}
		}
		var repos []struct {
			FullName      string `json:"full_name"`
			Description   string `json:"description"`
			Private       bool   `json:"private"`
			Archived      bool   `json:"archived"`
			DefaultBranch string `json:"default_branch"`
			HTMLURL       string `json:"html_url"`
			UpdatedAt     string `json:"updated_at"`
		}
		if err := json.Unmarshal(body, &repos); err != nil {
			return nil, &UpstreamError{Status: res.StatusCode, Message: "not a list of repositories: " + err.Error()}
		}
		for _, r := range repos {
			props := map[string]any{
				"connection": c.Name, "fullName": r.FullName, "private": r.Private, "archived": r.Archived,
				"defaultBranch": r.DefaultBranch, "url": r.HTMLURL, "updated": r.UpdatedAt,
			}
			if r.Description != "" {
				props["description"] = r.Description
			}
			env.Nodes = append(env.Nodes, graph.Node{ID: "ui:conn:" + c.Name + "/" + r.FullName, Kind: "git.repository", Name: r.FullName, Properties: props})
		}
		if len(repos) < 100 {
			break
		}
	}
	env.Recount()
	if ps := graph.Validate(env); graph.HasErrors(ps) {
		return nil, &UpstreamError{Message: "the repositories do not make a valid envelope: " + strings.Join(problemStrings(ps), "; ")}
	}
	return env, nil
}
