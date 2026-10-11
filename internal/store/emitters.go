package store

// LayerID is the id of the Layer node GET .../emitters serves for a mounted
// layer (docs/design.md 44): ui:layer:<name>.
func LayerID(name string) string { return "ui:layer:" + name }

// Emitter is the name of the emitter that produced the layer: the module its
// ontology (the envelope's ontology field, read at mount) names, or the
// envelope's own module when it has no ontology the portal could read (an
// inline mount of a relative ontology, a file gone).
func (l *Layer) Emitter() string {
	if m, ok := l.Ontology["module"].(string); ok && m != "" {
		return m
	}
	return l.Env.Module
}
