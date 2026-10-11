// Package tenant is what one tenant of the portal runs with: the product's
// base config (portal.base.yaml) with the tenant's overlay
// (tenants/<tenant>/portal.yaml) deep-merged over it, the menu built from
// that config as a Graph.Node envelope, its feeds and their ledger
// (ledger.jsonl), its identity (principals.yaml, and
// tokens.yaml: the bearer tokens and whose they are) and the service
// connections (connections.yaml). Everything is read at boot and checked: a
// file that fails stops the binary. The only files written are the two
// identity files, by an admin's Add principal and Revoke (identity.go), and
// the ledger, one line appended per push (feeds.go).
package tenant

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"github.com/JerryBalmer1/Portal/internal/feed"
	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Default is the tenant PORTAL_TENANT names when unset.
const Default = "default"

// Tenant modes (docs/design.md 39): the overlay's mode, prod when absent.
// dev allows the dev-bootstrap principal's tokens and the dev-token
// endpoint; demo and prod refuse both.
const (
	ModeDev  = "dev"
	ModeDemo = "demo"
	ModeProd = "prod"
)

// NamePattern is a tenant name: a path segment in every API route.
var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Tenant is one loaded tenant. Safe for concurrent use. Name, Config, Menu
// and Connections never change after Load; principals and tokens change
// only through AddPrincipal and RevokeToken, under mu.
type Tenant struct {
	Name        string
	Config      map[string]any
	Menu        *graph.Envelope
	Connections []Connection
	Feeds       []feed.Feed
	dir         string
	now         func() time.Time
	mu          sync.RWMutex
	principals  []Principal
	tokens      []token
	ledger      []LedgerEntry
	ledgerRaw   [][]byte
	ledgerEnd   int64
	ledgerLines int
	pushMu      sync.Mutex
	ledgerMu    sync.Mutex
}

// Options configures Load.
type Options struct {
	// Name is the tenant; empty means Default.
	Name string
	// Dir holds one directory per tenant: <Dir>/<Name>/portal.yaml, and
	// optionally principals.yaml, tokens.yaml and connections.yaml beside it.
	Dir string
	// Base and Schema are portal.base.yaml and schema/portal.schema.json.
	Base, Schema []byte
	// Stamp is the menu envelope's layer stamp.
	Stamp string
	// Check runs the portal ontology over the menu envelope (store's
	// CheckEnvelope); nil skips it.
	Check func(*graph.Envelope) []graph.Problem
	// Now stamps a new principal's created; nil means time.Now.
	Now func() time.Time
}

// ConfigError lists every problem found in one file, or in the merge.
type ConfigError struct {
	File     string
	Problems []string
}

func (e *ConfigError) Error() string {
	return e.File + ": " + strings.Join(e.Problems, "; ")
}

// Load reads, validates and merges one tenant. Every failure names its file.
func Load(o Options) (*Tenant, error) {
	name := o.Name
	if name == "" {
		name = Default
	}
	if !NamePattern.MatchString(name) {
		return nil, fmt.Errorf("tenant %q: a tenant name matches %s", name, NamePattern)
	}
	schema, err := compileSchema(o.Schema)
	if err != nil {
		return nil, fmt.Errorf("schema/portal.schema.json: %w", err)
	}
	base, err := readConfig("portal.base.yaml", o.Base, schema)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(o.Dir, name)
	overlayPath := filepath.Join(dir, "portal.yaml")
	data, err := os.ReadFile(overlayPath)
	if err != nil {
		return nil, fmt.Errorf("tenant %s: %w (every tenant has a portal.yaml, even an empty one)", name, err)
	}
	overlay, err := readConfig(overlayPath, data, schema)
	if err != nil {
		return nil, err
	}
	merged := Merge(base, overlay).(map[string]any)
	if ps := checkMerged(merged); len(ps) > 0 {
		return nil, &ConfigError{File: "portal.base.yaml + " + overlayPath, Problems: ps}
	}
	defaultMode(merged)
	t := &Tenant{Name: name, Config: merged, dir: dir, now: o.Now}
	var fps []string
	if t.Feeds, fps = loadFeeds(merged); len(fps) > 0 {
		return nil, &ConfigError{File: "portal.base.yaml + " + overlayPath, Problems: fps}
	}
	if t.now == nil {
		t.now = time.Now
	}
	if t.Menu, err = BuildMenu(merged, o.Stamp); err != nil {
		return nil, err
	}
	ps := graph.Validate(t.Menu)
	if o.Check != nil {
		ps = append(ps, o.Check(t.Menu)...)
	}
	if graph.HasErrors(ps) {
		return nil, &ConfigError{File: "menu (from portal.base.yaml + " + overlayPath + ")", Problems: problemStrings(ps)}
	}
	if t.principals, t.tokens, err = loadIdentity(dir); err != nil {
		return nil, err
	}
	if t.Connections, err = loadConnections(filepath.Join(dir, "connections.yaml"), t.principals); err != nil {
		return nil, err
	}
	if t.ledger, t.ledgerRaw, t.ledgerEnd, t.ledgerLines, err = loadLedger(filepath.Join(dir, LedgerFile)); err != nil {
		return nil, err
	}
	if ps := checkEmitterCards(t.Menu, t.ledger); len(ps) > 0 {
		return nil, &ConfigError{File: filepath.Join(dir, LedgerFile) + " against the menu (portal.base.yaml + " + overlayPath + ")", Problems: ps}
	}
	feeds := t.FeedsEnvelope(o.Stamp)
	ps = graph.Validate(feeds)
	if o.Check != nil {
		ps = append(ps, o.Check(feeds)...)
	}
	if graph.HasErrors(ps) {
		return nil, &ConfigError{File: "feeds (from portal.base.yaml + " + overlayPath + " and " + LedgerFile + ")", Problems: problemStrings(ps)}
	}
	return t, nil
}

// LoadConfig merges an overlay over the base without a tenant directory:
// the config half of Load, for tests and tools.
func LoadConfig(base, overlay, schemaDoc []byte) (map[string]any, error) {
	schema, err := compileSchema(schemaDoc)
	if err != nil {
		return nil, err
	}
	b, err := readConfig("portal.base.yaml", base, schema)
	if err != nil {
		return nil, err
	}
	ov, err := readConfig("overlay", overlay, schema)
	if err != nil {
		return nil, err
	}
	merged := Merge(b, ov).(map[string]any)
	if ps := checkMerged(merged); len(ps) > 0 {
		return nil, &ConfigError{File: "merged", Problems: ps}
	}
	if _, ps := loadFeeds(merged); len(ps) > 0 {
		return nil, &ConfigError{File: "merged", Problems: ps}
	}
	defaultMode(merged)
	return merged, nil
}

// defaultMode writes mode prod into a merged config that names none, so
// GET .../config and Mode always say which mode the tenant runs in.
func defaultMode(c map[string]any) {
	if _, ok := c["mode"].(string); !ok {
		c["mode"] = ModeProd
	}
}

// Mode is the tenant's mode: dev, demo or prod (docs/design.md 39).
func (t *Tenant) Mode() string {
	if m, ok := t.Config["mode"].(string); ok {
		return m
	}
	return ModeProd
}

func problemStrings(ps []graph.Problem) []string {
	var out []string
	for _, p := range ps {
		if p.Severity != graph.SeverityWarning {
			out = append(out, p.String())
		}
	}
	return out
}

const schemaURL = "https://portal.jerrybalmer.dev/schemas/portal.schema.json"

func compileSchema(doc []byte) (*jsonschema.Schema, error) {
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource(schemaURL, v); err != nil {
		return nil, err
	}
	return c.Compile(schemaURL)
}

// readConfig decodes YAML to JSON-shaped data and validates it. An empty
// document is an empty mapping.
func readConfig(file string, data []byte, schema *jsonschema.Schema) (map[string]any, error) {
	v, err := decodeYAML(data)
	if err != nil {
		return nil, &ConfigError{File: file, Problems: []string{err.Error()}}
	}
	if v == nil {
		v = map[string]any{}
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, &ConfigError{File: file, Problems: []string{"the document is not a mapping"}}
	}
	if err := schema.Validate(v); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return nil, &ConfigError{File: file, Problems: flatten(ve)}
		}
		return nil, &ConfigError{File: file, Problems: []string{err.Error()}}
	}
	return m, nil
}

// flatten turns a schema error tree into one line per leaf: /path: message.
func flatten(ve *jsonschema.ValidationError) []string {
	if len(ve.Causes) == 0 {
		// As graph's flatten: the leaf message is the text after the last ": ".
		msg := ve.Error()
		if i := strings.LastIndex(msg, ": "); i >= 0 {
			msg = msg[i+2:]
		}
		return []string{"/" + strings.Join(ve.InstanceLocation, "/") + ": " + msg}
	}
	var out []string
	for _, c := range ve.Causes {
		out = append(out, flatten(c)...)
	}
	sort.Strings(out)
	return out
}

// decodeYAML returns YAML as JSON-shaped data: map[string]any, []any,
// float64, string, bool, nil.
func decodeYAML(data []byte) (any, error) {
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	js, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var v any
	return v, json.Unmarshal(js, &v)
}

// Merge deep-merges overlay over base and returns a new value: mappings
// merge key by key; a list whose items all carry a string id merges item by
// item on id, overlay items with a new id appended in their order; anything
// else in the overlay replaces the base's.
func Merge(base, overlay any) any {
	switch o := overlay.(type) {
	case map[string]any:
		b, ok := base.(map[string]any)
		if !ok {
			return clone(o)
		}
		out := clone(b).(map[string]any)
		for k, v := range o {
			out[k] = Merge(b[k], v)
		}
		return out
	case []any:
		b, ok := base.([]any)
		if !ok || !keyed(b) || !keyed(o) {
			return clone(o)
		}
		out := clone(b).([]any)
		at := map[string]int{}
		for i, item := range out {
			at[item.(map[string]any)["id"].(string)] = i
		}
		for _, item := range o {
			id := item.(map[string]any)["id"].(string)
			if i, ok := at[id]; ok {
				out[i] = Merge(out[i], item)
			} else {
				at[id] = len(out)
				out = append(out, clone(item))
			}
		}
		return out
	}
	return overlay
}

func keyed(list []any) bool {
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return false
		}
		if _, ok := m["id"].(string); !ok {
			return false
		}
	}
	return true
}

func clone(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, x := range t {
			out[k] = clone(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = clone(x)
		}
		return out
	}
	return v
}

// checkMerged is what the schema cannot say for an overlay that names only
// what it changes: the merged config names the product and has a menu whose
// every entry has a label and a path that is its parent's path plus its id,
// ids unique among siblings, no service at layers or agents (the layer and
// agent pages' routes), no page of the ledger service with the id of a
// ledger source (whose entries' route it would take), the environments
// service's page graph drawing the environments graph and nothing else
// (every Show graph links there), the compliance service's page alerts
// drawing the alerts and nothing else (the banner's count and every alert's
// item page are there), and a page that knows what it draws.
func checkMerged(c map[string]any) []string {
	var out []string
	product, _ := c["product"].(map[string]any)
	if s, _ := product["name"].(string); s == "" {
		out = append(out, "/product/name: required")
	}
	menu, _ := c["menu"].([]any)
	if len(menu) == 0 {
		out = append(out, "/menu: at least one service")
	}
	var walk func(items []any, at, parent string, depth int)
	walk = func(items []any, at, parent string, depth int) {
		seen := map[string]bool{}
		for i, raw := range items {
			item := raw.(map[string]any)
			here := fmt.Sprintf("%s/%d", at, i)
			id, _ := item["id"].(string)
			if seen[id] {
				out = append(out, here+"/id: "+id+" twice among its siblings")
			}
			seen[id] = true
			if s, _ := item["label"].(string); s == "" {
				out = append(out, here+"/label: required")
			}
			want := id
			if parent != "" {
				want = parent + "/" + id
			}
			if s, _ := item["path"].(string); s != want {
				out = append(out, fmt.Sprintf("%s/path: %q, want %q (its parent's path and its id)", here, s, want))
			}
			kids, _ := item["children"].([]any)
			if depth == 1 && parent == LedgerService && slices.Contains(LedgerSources, id) {
				out = append(out, fmt.Sprintf("%s/id: %s is a ledger source, whose entries' route /<tenant>/%s/%s this page would take", here, id, LedgerService, id))
			}
			if depth == 1 && parent == EnvironmentsService && id == EnvironmentsGraphPage {
				if view, _ := item["view"].(string); view != EnvironmentsGraphView {
					out = append(out, fmt.Sprintf("%s/view: %s/%s is the environments graph, where every Show graph lands; its view is %s, not %q", here, EnvironmentsService, id, EnvironmentsGraphView, view))
				}
			}
			if depth == 1 && parent == ComplianceService && id == AlertsPage {
				if view, _ := item["view"].(string); view != AlertsView {
					out = append(out, fmt.Sprintf("%s/view: %s/%s is the alerts page, where the banner's count and every alert's page are; its view is %s, not %q", here, ComplianceService, id, AlertsView, view))
				}
			}
			if depth > 0 {
				view, _ := item["view"].(string)
				kind, _ := item["kind"].(string)
				screen, _ := item["screen"].(string)
				switch {
				case view == "screen" && screen == "":
					out = append(out, here+"/screen: view screen names the screen node it draws")
				case (view == "" || view == "table") && kind == "":
					out = append(out, here+"/kind: a table page names the Kind it lists")
				}
			} else {
				if len(kids) == 0 {
					out = append(out, here+"/children: a service has at least one page")
				}
				if id == "layers" {
					out = append(out, here+"/id: layers is the route of the layer pages, /<tenant>/layers/<name>")
				}
				if id == "agents" {
					out = append(out, here+"/id: agents is the route of the agent pages, /<tenant>/agents/<name>")
				}
			}
			walk(kids, here+"/children", want, depth+1)
		}
	}
	walk(menu, "/menu", "", 0)
	return out
}
