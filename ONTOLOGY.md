> This file defines the terms and the portal's Kinds. [README.md](README.md) says how to build and use it.

# Portal ontology

Module `Portal`, version 0.3.4, id prefix `ui:` (registered to Portal in the lifted Graph.Node id registry), declared in `ontology.yaml` and validated with `graphnode validate-ontology`. Node, Edge, Envelope, Kind, Layer and Category mean what Graph.Node says (its ONTOLOGY.md); this file adds only the portal's terms.

## Terminology

| Term | Meaning |
|---|---|
| Screen graph | The envelope `GET /api/screen` returns: module `Portal`, `root` `ui:root`, every panel a node. Held in memory; it reaches disk only as a snap. |
| Boot | What a start loads the screen from: the snap `PORTAL_SNAP` names, else the newest snap, else the dev seed (`empty` with `-no-seed`). Reported in the `X-Portal-Boot` header (docs/design.md 20). |
| Container | Kind. A box holding Containers and Widgets by `contains`; arranged by its Layout, drawn with its Theme. Properties `parent`, `order`, `visible`, `bounds`. The root is the Container no `contains` points at. |
| Menu entry | A Container that `is-a` the Category named Menu and `opens` the Container or Widget it shows. Drawn in the top bar, by `order` then id, never on the screen; contained by nothing. The seed's is `ui:menu:graph`, "Graph", opening `ui:canvas` (docs/design.md 25). |
| Widget | Kind. A panel; its Category (`is-a`) decides how it draws, `source` says what. Properties `type`, `source`, `bounds`, `order`, `visible`. |
| Layout | Kind. `mode` grid, flow or canvas; `columns` for grid (12 when absent). Joined by `laid-out-by`. |
| Category | Graph.Node base Kind used as a panel type. `name`, `properties.label` (required by Graph.Node), `description`. The base Categories Canvas, Table, Detail and Designer have renderers; any other draws with the base it `is-a`. The seeded Category Menu has no renderer: its Containers are the top bar's entries. |
| State | Kind. Per-node display state: `collapsed`, `selected`, `scroll`. Id `ui:state:<id without ui:>`, joined by `has-state`. A display Kind. |
| Designer section | One collapsible block of the Designer: Approve, Kinds (ontology.yaml), Categories, Add Panel, Add Container, Add Category, Add Menu Entry. Whether it is collapsed is a State node `ui:state:<Designer id without ui:>:<slug>` (`ui:state:designer:add-panel` for the seed's), joined from the Designer by `has-state`; seeded with Approve and Categories open, and added with those defaults to a booted snap that lacks them (docs/design.md 28). |
| Theme | Kind. How a Container and what it holds look: `color` (accent, `#rrggbb`), `opacity` (glass surface, 0 to 1), `font` (CSS font-family). Joined by `themed-by`; a Container without one takes its parent's. A display Kind, and a node like any other, so a theme change is a changed node in a diff (docs/design.md 22). |
| Display Kind | State or Theme: the only Kinds whose `properties` may hold display keys (docs/design.md 5). |
| Ref | Kind. A stand-in on screen for one node of a mounted layer (`layer`, `node`), so `displays` stays inside the envelope (docs/design.md 9). |
| Data layer | A Graph.Node envelope mounted read-only under a name (`POST /api/layers`); never written (docs/design.md 2). |
| Source | A Widget's `source`: a layer name and optional `?kind=&from=` query. |
| Bounds | `{x, y, w, h}`: grid cells under grid, pixels under canvas. |
| Snap | The screen graph written to `artifacts/snaps/<stamp>.graph.json`; `<stamp>` is UTC with milliseconds, `:` as `-`. The newest is what the next start boots from. |
| Approve | `POST /api/snap` with an optional `note`, the Designer's Approve button: writes a snap whose envelope carries the note as an `approved` finding, and answers with the ids added, removed and changed against the snap before it (docs/design.md 21). |
| Diff | `GET /api/diff`: snap b against snap a, `added`, `removed`, `changed`, and `merged`, the Graph.Node design 11 merge with `absent` findings. |
| Production mode | `PORTAL_MODE=production`: nodes that `is-a` Designer are left out of the screen (docs/design.md 3). |

## Kinds

| Kind | Properties (required in bold) | Edges |
|---|---|---|
| Container | `parent`, `bounds`, `order`, `visible` | `contains` → Container, Widget; `laid-out-by` → Layout; `themed-by` → Theme; `is-a` → Category; `opens` → Container, Widget; `has-state` → State |
| Widget | `type`, `source`, `bounds`, `order`, `visible` | `is-a` → Category; `displays` → Ref; `has-state` → State |
| Layout | **`mode`**, `columns` | `has-state` → State |
| Category | **`label`**, `description` | `is-a` → Category; `has-state` → State |
| State | `collapsed`, `selected`, `scroll` | (target of `has-state`) |
| Theme | `color`, `opacity`, `font` | (target of `themed-by`) |
| Ref | **`layer`**, **`node`** | `has-state` → State |

Finding Kinds: `absent` (info), set by the diff on an element in snap a and not in snap b; `approved` (info), set by Approve on the snap envelope, its message the note.

## Problem codes

Graph.Node's: `schema`, `duplicate-node`, `duplicate-edge`, `dangling-edge`, `edge-id`, `edgeid`, `counts`, `kind`, `stamp`, and for ontologies `display` and `action`. Each carries a `severity`, `error` or `warning`; only errors refuse, and a dangling edge on screen is always an error here (docs/design.md 19). The portal's, all errors:

| Code | Means |
|---|---|
| `id-prefix` | A screen node id does not start `ui:`. |
| `unknown-kind` | The Kind is not in `ontology.yaml`. |
| `unknown-property` | A property the Kind does not declare. |
| `property-type` | A property of the wrong type, a required one missing, `mode` not grid/flow/canvas, `bounds` or `scroll` malformed, a Theme `color` not `#rrggbb`, `opacity` outside 0 to 1, a blank `font`. Also a Category with no `properties` at all, which Graph.Node 0.2.0 lets through (docs/design.md 10). |
| `display` | A display property (`display`, `color`, `shape`, `icon`, `collapsed`, `selected`, `scroll`, `opacity`, `font`) on a node that is not a display Kind, or on an edge. |
| `unknown-edge-kind` | The edge Kind is not in `ontology.yaml`. |
| `edge-endpoints` | The edge's ends are not of the Kinds its edge Kind allows (an `opens` to a Layout, say). |
| `unknown-category` | An `is-a` to something that is not a Category on screen, or a Widget `type` naming no Category. |
| `unknown-ref` | A Ref names a layer that is not mounted or a node it does not hold. |
| `unknown-field` | A top-level node key other than id, kind, name, properties, source, findings, stamp. |
| `ontology` | In a layer's `ontologyProblems`: its ontology is a URL, relative to an inline envelope, or unreadable; the layer draws grey (docs/design.md 14). |

## Not here yet

- Edges across envelopes, which would retire Ref (docs/design.md 9).
- Mounts that survive a restart (docs/design.md 14).
- An edge deletion route (docs/design.md 15).
- A drift check for `internal/gen` (docs/design.md 16).
- Editing a Theme from the Designer (docs/design.md 22).
- Graph.Node refusing a Category with no properties, a Graph.Node 0.2.1 item (docs/design.md 10).
