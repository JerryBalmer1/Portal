---
name: ontology-doc
description: Keep ONTOLOGY.md, the terms door to Portal, in step with ontology.yaml and the API. Use whenever a Kind, property, edge Kind or finding Kind in ontology.yaml changes, a problem code is added or removed in internal/store, a term in ONTOLOGY.md's table changes, or something "Not here yet" lists ships or is dropped.
---

# ONTOLOGY.md

Adapted from Graph.Node's skill of the same name. `ONTOLOGY.md` is the door for readers who came for the terms: the screen graph, the portal's Kinds and edges, and the problem codes a refused write carries. `README.md` (skill `readme`) is the door for building and using it and never repeats this.

## When to update it

In the same task as any change to:
- `ontology.yaml`: a Kind, a property or whether it is required, an edge Kind or its from/to, a finding Kind
- a problem code emitted by `internal/store/check.go` or `screen.go`, or one Graph.Node adds
- the display rule (docs/design.md 5, 22), the Ref rule (9), production mode (3), the stamp or snap format (12), the boot order (20) or approval (21)
- a design.md entry that defers something, or something "Not here yet" lists shipping

## Rules

- Line 1 is one `>` line linking `README.md`. Pester ("ONTOLOGY.md first line links README.md") checks it.
- Sections in order: Terminology; Kinds; Problem codes; Not here yet.
- The Kinds table has one row per Kind in `ontology.yaml`, no more and no fewer; required properties in bold exactly as `required: true` says; edges exactly as `edgeKinds` from/to give them.
- The problem codes table lists every portal code the store emits; Graph.Node's codes are named in one line, not redefined.
- Every "Not here yet" item names its design.md entry as `(docs/design.md N)`.
- Field and code names backticked and spelled as in the code.

## Procedure

1. Read `ontology.yaml`, `internal/store/check.go`, `internal/store/screen.go`, docs/design.md and the current ONTOLOGY.md.
2. Compare Kinds and edges row by row; grep the store for `Code:` and `add("` to list the codes.
3. Edit. Run `graphnode validate-ontology ontology.yaml` (build/win-x64/graphnode.exe) and Pester in a fresh process; both clean.
4. Stage ONTOLOGY.md. In the report, list the rows changed and any term you could not resolve.
