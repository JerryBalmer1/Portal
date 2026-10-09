// Package schemas embeds the GraphNode JSON Schemas so the Go validator is self-contained.
package schemas

import "embed"

// FS holds node, edge, envelope, ontology and kinds/*.schema.json.
//
//go:embed *.schema.json kinds/*.schema.json
var FS embed.FS

// Base is the $id prefix every schema in FS uses.
const Base = "https://graphnode.jerrybalmer.dev/schemas/"
