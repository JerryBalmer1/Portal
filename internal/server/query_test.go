package server

import (
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// The chain layer, stamped Emitter.Chain: a → b → c → d along next, c a
// Gate, b on lane side, and an aside edge a → x. The other layer, stamped
// Emitter.Other, holds e and the edge d → e.
const (
	chainLayer = `{"schema":"graph/1","module":"Chain","layer":"2026-10-12T00:00:00Z",
"source":{"emitter":"Emitter.Chain","version":"1.0.0","target":"chain","stamp":"2026-10-12T00:00:00Z"},
"nodes":[{"id":"a","kind":"Step","properties":{"lane":"main","n":1}},{"id":"b","kind":"Step","properties":{"lane":"side","n":2}},
{"id":"c","kind":"Gate","properties":{"lane":"main","n":3}},{"id":"d","kind":"Step","properties":{"lane":"main","n":4,"facet":{"key":"v"}}},
{"id":"x","kind":"Step","properties":{"lane":"main"}}],
"edges":[{"id":"a|b|next","kind":"next","from":"a","to":"b"},{"id":"b|c|next","kind":"next","from":"b","to":"c"},
{"id":"c|d|next","kind":"next","from":"c","to":"d"},{"id":"a|x|aside","kind":"aside","from":"a","to":"x"}]}`
	otherLayer = `{"schema":"graph/1","module":"Other","layer":"2026-10-12T00:00:01Z",
"source":{"emitter":"Emitter.Other","version":"1.0.0","target":"other","stamp":"2026-10-12T00:00:01Z"},
"nodes":[{"id":"d","kind":"Step"},{"id":"e","kind":"Step"}],
"edges":[{"id":"d|e|next","kind":"next","from":"d","to":"e"}]}`
)

// mountChain mounts the chain layer, then the other, in that order: the
// first id wins, so d is the chain's.
func mountChain(t *testing.T, h *harness, base string) {
	t.Helper()
	for _, m := range [][2]string{{"chain", chainLayer}, {"other", otherLayer}} {
		h.do(t, base, "POST", "/api/tenants/default/layers", `{"name":"`+m[0]+`","envelope":`+m[1]+`}`, 201)
	}
}

// queried is a queryTenantGraph answer.
type queried struct {
	graph.Envelope
	Paths       [][]string     `json:"paths"`
	Query       map[string]any `json:"query"`
	Truncated   bool           `json:"truncated"`
	TruncatedAt *int           `json:"truncatedAt"`
}

func (q *queried) ids() string { return strings.Join(nodeIDs(&q.Envelope), ",") }

func (q *queried) paths() string {
	var out []string
	for _, p := range q.Paths {
		out = append(out, strings.Join(p, ">"))
	}
	return strings.Join(out, " ")
}

// The walk: depth, until, direction, where, limit, source, follow, and the
// 400s and the empty 200 (docs/design.md 54).
func TestQueryWalksTheTenantGraph(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	mountChain(t, h, s.URL)
	get := func(q string) *queried {
		t.Helper()
		body := h.do(t, s.URL, "GET", "/api/tenants/default/query?"+q, "", 200)
		if ps := graph.ValidateBytes(body); graph.HasErrors(ps) {
			t.Fatalf("%s: graphnode validate: %+v", q, ps)
		}
		var r queried
		if err := json.Unmarshal(body, &r); err != nil {
			t.Fatal(err)
		}
		closed(t, q, &r.Envelope)
		return &r
	}

	for _, c := range []struct{ q, nodes, paths string }{
		// Depth 1, 2, 3 along next.
		{"from=a&follow=next&depth=1", "a,b", "a>b"},
		{"from=a&follow=next&depth=2", "a,b,c", "a>b>c"},
		{"from=a&follow=next&depth=3", "a,b,c,d", "a>b>c>d"},
		// The default depth is 3, the default follow every kind.
		{"from=a", "a,b,x,c,d", "a>x a>b>c>d"},
		{"from=a&depth=0", "a", "a"},
		// until: c is kept, d is not reached.
		{"from=a&follow=next&until=Gate", "a,b,c", "a>b>c"},
		// direction in walks the edges backwards; both walks either way.
		{"from=d&follow=next&direction=in", "d,c,b,a", "d>c>b>a"},
		{"from=c&follow=next&direction=both&depth=1", "c,d,b", "c>d c>b"},
		// where: b is filtered out, so it is not expanded either.
		{"from=a&follow=next&where=lane=main", "a", "a"},
		{"from=a&where=lane=main", "a,x", "a>x"},
		{"from=a&where=properties.lane=main&where=kind=Step", "a,x", "a>x"},
		{"from=a&where=facet.key=v&depth=10", "", ""},
		{"from=a&where=n=1&depth=0", "a", "a"},
		// Two starts, each with its own visited set.
		{"from=a,c&follow=next&depth=1", "a,c,b,d", "a>b c>d"},
		// kind starts at every node of the Kind.
		{"kind=Gate&depth=0", "c", "c"},
		// Across layers: d → e is the other layer's edge.
		{"from=c&follow=next", "c,d,e", "c>d>e"},
	} {
		r := get(c.q)
		if r.ids() != c.nodes || r.paths() != c.paths {
			t.Errorf("%s: nodes %s paths %s, want %s and %s", c.q, r.ids(), r.paths(), c.nodes, c.paths)
		}
		if r.Truncated {
			t.Errorf("%s: truncated", c.q)
		}
	}
	// Edges are those between two nodes of the result.
	if r := get("from=a&follow=next&depth=2"); len(r.Edges) != 2 || !hasEdge(&r.Envelope, "a|b|next") || !hasEdge(&r.Envelope, "b|c|next") {
		t.Errorf("depth 2 edges %+v", r.Edges)
	}
	if r := get("from=a,c&follow=next&depth=1"); len(r.Edges) != 3 {
		t.Errorf("two starts: %d edges, want a|b, b|c and c|d", len(r.Edges))
	}

	// The query as resolved.
	r := get("from=a&follow=next&where=lane=main")
	q, _ := json.Marshal(r.Query)
	if string(q) != `{"depth":3,"direction":"out","follow":["next"],"from":["a"],"limit":500,"where":["lane=main"]}` {
		t.Errorf("query echo %s", q)
	}

	// limit: a and b fit, c is hop 2's.
	r = get("from=a&follow=next&limit=2")
	if r.ids() != "a,b" || !r.Truncated || r.TruncatedAt == nil || *r.TruncatedAt != 2 {
		t.Errorf("limit 2: nodes %s truncated %v at %v", r.ids(), r.Truncated, r.TruncatedAt)
	}
	if r = get("kind=Step&depth=0&limit=1"); r.ids() != "a" || !r.Truncated || *r.TruncatedAt != 0 {
		t.Errorf("limit 1 on four starts: nodes %s truncated %v", r.ids(), r.Truncated)
	}

	// source keeps one emitter's layers: e is the other's.
	if r = get("from=a&follow=next&depth=10&source=Emitter.Chain"); r.ids() != "a,b,c,d" {
		t.Errorf("source Emitter.Chain: %s", r.ids())
	}
	if r = get("from=a&follow=next&depth=10"); r.ids() != "a,b,c,d,e" {
		t.Errorf("no source: %s", r.ids())
	}
	if r = get("kind=Step&depth=0&source=Emitter.Other"); r.ids() != "d,e" {
		t.Errorf("source Emitter.Other: %s", r.ids())
	}

	// Empty is 200.
	for _, q := range []string{"kind=Nope", "kind=Step&source=Emitter.Nope", "from=a&where=lane=nowhere"} {
		if r = get(q); len(r.Nodes) != 0 || len(r.Edges) != 0 || len(r.Paths) != 0 {
			t.Errorf("%s: %s, want empty", q, r.ids())
		}
	}

	// 400s, an unknown from named.
	for q, says := range map[string]string{
		"from=a,nope":                 "no node nope",
		"from=a&source=Emitter.Other": "no node a",
		"":                            "give from",
		"from=a&kind=Step":            "not both",
		"from=a&direction=up":         "direction",
		"from=a&depth=11":             "depth 11",
		"from=a&depth=-1":             "depth -1",
		"from=a&limit=0":              "limit 0",
		"from=a&limit=5001":           "limit 5001",
		"from=a&where=lane":           "key=value",
		"from=a&where=" + url.QueryEscape("a..b=1"): "empty key",
	} {
		body := h.do(t, s.URL, "GET", "/api/tenants/default/query?"+q, "", 400)
		if !strings.Contains(string(body), says) {
			t.Errorf("%s: %s, want %q", q, body, says)
		}
	}
	h.do(t, s.URL, "GET", "/api/tenants/other/query?from=a", "", 404)
}

// The tenant graph is the query with every node a start, depth 0 and no
// cap: per source, the layers' nodes, the first id winning, and the edges
// between them.
func TestTenantGraphIsTheQueryAtDepthZero(t *testing.T) {
	h := newHarness(t)
	s, _, _ := newPortal(t, false)
	mountChain(t, h, s.URL)
	get := func(source string) *graph.Envelope {
		var env graph.Envelope
		if err := json.Unmarshal(h.do(t, s.URL, "GET", "/api/tenants/default/graph?source="+source, "", 200), &env); err != nil {
			t.Fatal(err)
		}
		closed(t, source, &env)
		return &env
	}
	for source, want := range map[string]string{"Emitter.Chain": "a,b,c,d,x 4", "Emitter.Other": "d,e 1", "Emitter.Nope": " 0"} {
		env := get(source)
		if got := fmt.Sprintf("%s %d", strings.Join(nodeIDs(env), ","), len(env.Edges)); got != want {
			t.Errorf("source %s: %s, want %s", source, got, want)
		}
	}
	all := get("")
	ids := nodeIDs(all)
	if !slices.Contains(ids, "fs:src") || !slices.Contains(ids, "e") || !hasEdge(all, "d|e|next") {
		t.Errorf("the whole graph: %v", ids)
	}
	// d is the chain's, first in mount order.
	for _, n := range all.Nodes {
		if n.ID == "d" && n.Stamp.Layer != "2026-10-12T00:00:00Z" {
			t.Errorf("d from layer %s, want the chain's", n.Stamp.Layer)
		}
	}
}
