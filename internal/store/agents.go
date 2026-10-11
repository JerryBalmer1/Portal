package store

import (
	"sort"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// AgentModule is the module of the layers whose Run nodes are agent runs.
// It owns the id prefix agent:; the portal re-serves its Run and
// PolicyDecision ids and never mints one (docs/design.md 43).
const AgentModule = "Graph.Emitter.Agent"

// AgentRun is one Run node of a mounted Graph.Emitter.Agent layer, as the
// portal's Kind Run (ontology.yaml) under the emitter's own id, with the
// layer's PolicyDecisions under it as Kind Decision.
type AgentRun struct {
	Run       graph.Node
	Decisions []graph.Node
	// MountedBy is the layer's: the principal whose token mounted it, or "".
	MountedBy string
}

// AgentRuns reads every Run node of every mounted layer whose module is
// Graph.Emitter.Agent, newest first by startedAt (then by id). A Run id in
// two layers is kept once, from the layer with the later stamp, the later
// mount on a tie.
func (p *Portal) AgentRuns() []AgentRun {
	byID := map[string]AgentRun{}
	stamp := map[string]string{}
	for _, l := range p.Layers() {
		if l.Env.Module != AgentModule {
			continue
		}
		for _, r := range layerRuns(l) {
			if s, ok := stamp[r.Run.ID]; ok && s > l.Env.Layer {
				continue
			}
			byID[r.Run.ID] = r
			stamp[r.Run.ID] = l.Env.Layer
		}
	}
	out := make([]AgentRun, 0, len(byID))
	for _, r := range byID {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := out[i].Run.Properties["startedAt"].(string)
		b, _ := out[j].Run.Properties["startedAt"].(string)
		if a != b {
			return a > b
		}
		return out[i].Run.ID > out[j].Run.ID
	})
	return out
}

// layerRuns walks one layer along the emitter's edges: a Run contains its
// Steps, a Step contains its Action, an Action decided its PolicyDecisions.
func layerRuns(l *Layer) []AgentRun {
	env := l.Env
	idx := env.NodeIndex()
	out := map[string][]string{}
	for _, e := range env.Edges {
		if e.Kind == "contains" || e.Kind == "decided" {
			out[e.From+"|"+e.Kind] = append(out[e.From+"|"+e.Kind], e.To)
		}
	}
	of := func(from, edge, kind string) []graph.Node {
		var ns []graph.Node
		for _, id := range out[from+"|"+edge] {
			if i, ok := idx[id]; ok && env.Nodes[i].Kind == kind {
				ns = append(ns, env.Nodes[i])
			}
		}
		return ns
	}
	var runs []AgentRun
	for _, n := range env.Nodes {
		if n.Kind != "Run" {
			continue
		}
		props := map[string]any{"layer": l.Name, "steps": 0, "allow": 0, "deny": 0, "witness": 0}
		for _, k := range []string{"startedAt", "stopped", "finishedAt", "exit"} {
			if v, ok := n.Properties[k]; ok {
				props[k] = v
			}
		}
		r := AgentRun{MountedBy: l.MountedBy}
		for _, step := range of(n.ID, "contains", "Step") {
			props["steps"] = props["steps"].(int) + 1
			ordinal := step.Properties["ordinal"]
			for _, action := range of(step.ID, "contains", "Action") {
				for _, d := range of(action.ID, "decided", "PolicyDecision") {
					effect, _ := d.Properties["effect"].(string)
					if c, ok := props[effect].(int); ok {
						props[effect] = c + 1
					}
					dp := map[string]any{"layer": l.Name, "effect": effect, "step": ordinal}
					for _, k := range []string{"phase", "rule", "reason"} {
						if v, ok := d.Properties[k]; ok {
							dp[k] = v
						}
					}
					r.Decisions = append(r.Decisions, graph.Node{ID: d.ID, Kind: "Decision", Name: d.Name, Properties: dp})
				}
			}
		}
		r.Run = graph.Node{ID: n.ID, Kind: "Run", Name: n.Name, Properties: props}
		runs = append(runs, r)
	}
	return runs
}
