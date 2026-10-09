// Package graphnode holds a copy of Graph.Node 0.2.0 (C:\__Code\Graph.Node,
// tag v0.2.0, ee4df2c): schemas/ byte for byte, and graph/ with only its
// import path rewritten, plus one registered edit: the ui: row of the id
// prefix registry (graph/ids.go and schemas/ids.md) names Portal as its
// owner. It is lifted, not owned (docs/design.md 4): never edit it here.
// When Graph.Node tags a new version, recopy it; Pester "Lifted Graph.Node"
// compares against the tag and fails on any other drift.
package graphnode
