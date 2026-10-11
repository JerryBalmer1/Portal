package tenant

import (
	"fmt"
	"strings"

	"github.com/JerryBalmer1/Portal/internal/graphnode/graph"
)

// Emitters (docs/design.md 44, 49). An emitter is a card of type emitter in
// one of the tenant's feeds, named by the card and served under the card's
// own id; a mounted layer belongs to it when its Graph.Node source block's
// emitter is the card's name. Nothing maps one name onto another. There is
// no emitter id but ui:emitter:<name>, for a name a layer's source block
// gives that no feed holds a card of, and ui:module:<module>, for the module
// of layers with no source block.
const (
	// EmitterType is the card type that makes a card an emitter.
	EmitterType = "emitter"
	// EmittersService is the menu service whose pages keep their paths
	// under /<tenant>/emitters/<page>, beside the emitter pages
	// /<tenant>/emitters/<name>.
	EmittersService = "emitters"
	// EmitterPrefix + name is the id of an emitter a layer's source block
	// names that no feed holds a card of.
	EmitterPrefix = "ui:emitter:"
	// ModulePrefix + module is the id of the module of unstamped layers,
	// those with no source block, which are never matched to a card.
	ModulePrefix = "ui:module:"
)

// emitterPage is the page of the emitters service a card named card would
// shadow at /<tenant>/emitters/<name>, or "" when it shadows none.
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
		if page == card {
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
