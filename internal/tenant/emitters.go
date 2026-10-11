package tenant

import (
	"fmt"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Emitters (docs/design.md 44). An emitter is a card of type emitter in one
// of the tenant's feeds, served under the card's own id; there is no
// emitter id but ui:emitter:<name>, for an emitter a mounted layer names
// that no feed holds a card of.
const (
	// EmitterType is the card type that makes a card an emitter.
	EmitterType = "emitter"
	// EmittersService is the menu service whose pages keep their paths
	// under /<tenant>/emitters/<page>, beside the emitter pages
	// /<tenant>/emitters/<name>.
	EmittersService = "emitters"
	// EmitterPrefix + name is the id of an emitter no feed holds a card of.
	EmitterPrefix = "ui:emitter:"
)

// EmitterName is the emitter a card of type emitter is, named as its layers
// name it: by the module of their ontology. Portal.Emitters names the card
// of the module Graph.Emitter.<X> Emitter.<X>, so that card is the emitter
// Graph.Emitter.<X>; a card named any other way is the emitter of its own
// name.
func EmitterName(card string) string {
	if strings.HasPrefix(card, "Emitter.") {
		return "Graph." + card
	}
	return card
}

// emitterPage is the page of the emitters service a card named card would
// shadow at /<tenant>/emitters/<name>, by its name or its emitter name, or
// "" when it shadows none.
func emitterPage(menu *graph.Envelope, card string) string {
	svc := RailID(EmittersService)
	idx := menu.NodeIndex()
	for _, e := range menu.Edges {
		if e.Kind != "has-page" || e.From != svc {
			continue
		}
		i, ok := idx[e.To]
		if !ok {
			continue
		}
		path, _ := menu.Nodes[i].Properties["path"].(string)
		page := strings.TrimPrefix(path, EmittersService+"/")
		if page == card || page == EmitterName(card) {
			return page
		}
	}
	return ""
}

// checkEmitterCards is the boot check of the ledger against the menu: no
// pushed card of type emitter is named like a page of the emitters service,
// whose route its emitter page would take. A push cannot make one, since
// Graph.Node's manifest schema wants a dotted card name and a menu id has no
// dot; a ledger written by hand can.
func checkEmitterCards(menu *graph.Envelope, ledger []LedgerEntry) []string {
	var out []string
	for i, e := range ledger {
		if e.Result != ResultPushed || *e.Type != EmitterType {
			continue
		}
		if page := emitterPage(menu, *e.Card); page != "" {
			out = append(out, fmt.Sprintf("entry %d: emitter card %s %s (pushed to %s) is named like the page %s/%s, whose route /<tenant>/%s/%s it would take", i+1, *e.Card, *e.Version, e.Feed, EmittersService, page, EmittersService, page))
		}
	}
	return out
}
