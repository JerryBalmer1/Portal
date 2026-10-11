package store

// LayerID is the id of the Layer node GET .../emitters serves for a mounted
// layer (docs/design.md 44): ui:layer:<name>.
func LayerID(name string) string { return "ui:layer:" + name }

// Emitter is the name of the emitter that produced the layer, and whether
// the layer says so itself (docs/design.md 49): the emitter of its Graph.Node
// source block (design 20), which is the emitter's card name, stamped; or,
// for a layer with no source block, the envelope's own module, unstamped.
// Nothing maps one name onto another, and an unstamped module is never a
// card's name.
func (l *Layer) Emitter() (name string, stamped bool) {
	if s := l.Env.Source; s != nil && s.Emitter != "" {
		return s.Emitter, true
	}
	return l.Env.Module, false
}
