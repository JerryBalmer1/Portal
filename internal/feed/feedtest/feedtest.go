// Package feedtest holds fakes of the two feed services for tests: an OCI
// registry speaking the distribution API's push and HEAD calls, and a NuGet
// v3 feed with a service index, a flat container and the push PUT. Only
// tests import it.
package feedtest

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Registry is a fake OCI registry.
type Registry struct {
	*httptest.Server
	mu        sync.Mutex
	Blobs     map[string][]byte
	Manifests map[string][]byte // "<repo>:<tag>"
	uploads   int
}

var (
	blobPath     = regexp.MustCompile(`^/v2/(.+)/blobs/(sha256:[0-9a-f]{64})$`)
	uploadStart  = regexp.MustCompile(`^/v2/(.+)/blobs/uploads/$`)
	uploadPath   = regexp.MustCompile(`^/v2/(.+)/blobs/uploads/(\d+)$`)
	manifestPath = regexp.MustCompile(`^/v2/(.+)/manifests/([^/]+)$`)
)

// NewRegistry starts a fake registry, closed when t ends.
func NewRegistry(t *testing.T) *Registry {
	t.Helper()
	g := &Registry{Blobs: map[string][]byte{}, Manifests: map[string][]byte{}}
	g.Server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.Close)
	return g
}

func (g *Registry) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := r.URL.Path
	switch {
	case r.Method == http.MethodHead && blobPath.MatchString(p):
		if _, ok := g.Blobs[blobPath.FindStringSubmatch(p)[2]]; ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodPost && uploadStart.MatchString(p):
		g.uploads++
		w.Header().Set("Location", fmt.Sprintf("/v2/%s/blobs/uploads/%d", uploadStart.FindStringSubmatch(p)[1], g.uploads))
		w.WriteHeader(http.StatusAccepted)
	case r.Method == http.MethodPut && uploadPath.MatchString(p):
		body, _ := io.ReadAll(r.Body)
		d := r.URL.Query().Get("digest")
		if digest(body) != d {
			http.Error(w, `{"errors":[{"code":"DIGEST_INVALID"}]}`, http.StatusBadRequest)
			return
		}
		g.Blobs[d] = body
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodHead && manifestPath.MatchString(p):
		m := manifestPath.FindStringSubmatch(p)
		if _, ok := g.Manifests[m[1]+":"+m[2]]; ok {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case r.Method == http.MethodPut && manifestPath.MatchString(p):
		m := manifestPath.FindStringSubmatch(p)
		body, _ := io.ReadAll(r.Body)
		var man struct {
			Config struct{ Digest string }   `json:"config"`
			Layers []struct{ Digest string } `json:"layers"`
		}
		if err := json.Unmarshal(body, &man); err != nil {
			http.Error(w, `{"errors":[{"code":"MANIFEST_INVALID"}]}`, http.StatusBadRequest)
			return
		}
		for _, d := range append([]string{man.Config.Digest}, func() []string {
			var ds []string
			for _, l := range man.Layers {
				ds = append(ds, l.Digest)
			}
			return ds
		}()...) {
			if _, ok := g.Blobs[d]; !ok {
				http.Error(w, `{"errors":[{"code":"MANIFEST_BLOB_UNKNOWN"}]}`, http.StatusBadRequest)
				return
			}
		}
		g.Manifests[m[1]+":"+m[2]] = body
		w.Header().Set("Docker-Content-Digest", digest(body))
		w.WriteHeader(http.StatusCreated)
	default:
		http.NotFound(w, r)
	}
}

// Manifest is the stored manifest of repo:tag, nil when absent.
func (g *Registry) Manifest(ref string) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Manifests[ref]
}

// NuGet is a fake NuGet v3 feed. Key is the API key a push must carry; ""
// takes any.
type NuGet struct {
	*httptest.Server
	Key      string
	mu       sync.Mutex
	Packages map[string][]byte // "<id lower>/<version lower>"
}

// NewNuGet starts a fake feed, closed when t ends. Its service index is
// URL + "/v3/index.json".
func NewNuGet(t *testing.T, key string) *NuGet {
	t.Helper()
	g := &NuGet{Key: key, Packages: map[string][]byte{}}
	g.Server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.Close)
	return g
}

// Index is the feed's service index URL.
func (g *NuGet) Index() string { return g.URL + "/v3/index.json" }

func (g *NuGet) serve(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	p := r.URL.Path
	switch {
	case r.Method == http.MethodGet && p == "/v3/index.json":
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "3.0.0", "resources": []map[string]string{
			{"@id": g.URL + "/api/v2/package", "@type": "PackagePublish/2.0.0"},
			{"@id": g.URL + "/v3/package", "@type": "PackageBaseAddress/3.0.0"},
		}})
	case r.Method == http.MethodGet && strings.HasPrefix(p, "/v3/package/") && strings.HasSuffix(p, "/index.json"):
		id := strings.TrimSuffix(strings.TrimPrefix(p, "/v3/package/"), "/index.json")
		var vs []string
		for k := range g.Packages {
			if pid, v, _ := strings.Cut(k, "/"); pid == id {
				vs = append(vs, v)
			}
		}
		if len(vs) == 0 {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"versions": vs})
	case r.Method == http.MethodPut && p == "/api/v2/package":
		if g.Key != "" && r.Header.Get("X-NuGet-ApiKey") != g.Key {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f, _, err := r.FormFile("package")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		pkg, _ := io.ReadAll(f)
		id, version, err := Nuspec(pkg)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		key := strings.ToLower(id) + "/" + strings.ToLower(version)
		if _, ok := g.Packages[key]; ok {
			w.WriteHeader(http.StatusConflict)
			return
		}
		g.Packages[key] = pkg
		w.WriteHeader(http.StatusCreated)
	default:
		http.NotFound(w, r)
	}
}

// Package is a stored .nupkg, nil when absent.
func (g *NuGet) Package(id, version string) []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.Packages[strings.ToLower(id)+"/"+strings.ToLower(version)]
}

// Nuspec reads a .nupkg's id and version from the .nuspec at its root.
func Nuspec(pkg []byte) (id, version string, err error) {
	z, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	if err != nil {
		return "", "", err
	}
	for _, f := range z.File {
		if strings.HasSuffix(f.Name, ".nuspec") && !strings.Contains(f.Name, "/") {
			rc, _ := f.Open()
			data, _ := io.ReadAll(rc)
			rc.Close()
			var spec struct {
				Metadata struct {
					ID      string `xml:"id"`
					Version string `xml:"version"`
				} `xml:"metadata"`
			}
			if err := xml.Unmarshal(data, &spec); err != nil {
				return "", "", err
			}
			return spec.Metadata.ID, spec.Metadata.Version, nil
		}
	}
	return "", "", fmt.Errorf("no .nuspec at the package root")
}
