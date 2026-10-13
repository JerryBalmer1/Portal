// Package web holds the portal page: index.html, the JS (client, portal and
// one renderer per base Category), the stylesheet, and vendored Cytoscape.
// Lifted in shape from GraphRenderer's pkg/web (docs/design.md 6).
package web

import (
	"embed"
	"io/fs"
	"path"
	"strings"
)

// FS holds the page. vendor/LICENSE stays beside cytoscape.min.js in the
// repo and is not served.
//
//go:embed index.html *.js *.css vendor/cytoscape.min.js
var FS embed.FS

// static maps each flat /static/{file} name to its path in the FS.
var static = map[string]string{
	"client.js":        "client.js",
	"portal.js":        "portal.js",
	"screen.js":        "screen.js",
	"icons.js":         "icons.js",
	"nodes.js":         "nodes.js",
	"connections.js":   "connections.js",
	"principals.js":    "principals.js",
	"tokens.js":        "tokens.js",
	"feeds.js":         "feeds.js",
	"layers.js":        "layers.js",
	"agents.js":        "agents.js",
	"emitters.js":      "emitters.js",
	"ledger.js":        "ledger.js",
	"envgraph.js":      "envgraph.js",
	"graph.js":         "graph.js",
	"query.js":         "query.js",
	"nodemenu.js":      "nodemenu.js",
	"alerts.js":        "alerts.js",
	"actions.js":       "actions.js",
	"canvas.js":        "canvas.js",
	"table.js":         "table.js",
	"detail.js":        "detail.js",
	"designer.js":      "designer.js",
	"toast.js":         "toast.js",
	"wheel-speed.js":   "wheel-speed.js",
	"base.css":         "base.css",
	"cytoscape.min.js": "vendor/cytoscape.min.js",
}

// Static returns one /static file and its content type.
func Static(fsys fs.FS, name string) ([]byte, string, error) {
	p, ok := static[name]
	if !ok {
		return nil, "", fs.ErrNotExist
	}
	data, err := fs.ReadFile(fsys, p)
	if err != nil {
		return nil, "", err
	}
	return data, contentType(p), nil
}

// contentType for the extensions the spec allows.
func contentType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".js":
		return "text/javascript"
	case ".css":
		return "text/css"
	case ".html":
		return "text/html"
	}
	return ""
}
