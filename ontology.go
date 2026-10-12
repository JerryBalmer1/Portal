// Package portal embeds ontology.yaml, the portal's own Kinds and edge Kinds
// (module Portal, id prefix ui:). The server checks every screen write
// against it; GET /api/ontology serves it.
package portal

import _ "embed"

// OntologyYAML is ontology.yaml as written on disk.
//
//go:embed ontology.yaml
var OntologyYAML []byte

// Module and Version are the envelope header of every screen graph and snap.
const (
	Module  = "Portal"
	Version = "0.16.0"
)
