# Portal design decisions

Numbered. Do not reopen without a proposal in a report. Entries 1 to 3 are the 0.1.0 brief's; 4 to 17 are the choices made scaffolding 0.1.0 without asking, each with what it rules out; 18 on are 0.2.0's. An entry a later one changes says so in place.

## 1. Everything on screen is a graph

Everything on screen is a graph in the Graph.Node envelope (`graph/1`, module `Portal`, ids prefixed `ui:`). The portal has no UI model of its own: it has nodes, edges and one API. Containers, Widgets, Layouts, Categories, States, Themes, Refs and (since 0.3.5, entry 33) Mounts are nodes; `contains`, `laid-out-by`, `is-a`, `displays`, `has-state` and `themed-by` are edges (`ontology.yaml`). The page fetches `GET /api/screen` and draws it; it keeps no state the graph does not hold, beyond the transient Canvas selection (entry 11). New panel types are Category nodes with `is-a` edges, never new code: a Category without a renderer of its own uses the one of the Category it `is-a` (entry 10).

## 2. No write to any data layer, ever

A mounted layer is read from disk or from a request body, validated, and held in memory read-only. No route writes it, patches it or appends to it, in memory or on disk; `GET /api/layers/{name}/nodes` returns a filtered copy. The only files the binary writes are under `-artifacts`: `snaps/<stamp>.graph.json` and `snaps/ontology.yaml` (since 0.2.0 nothing under `state/`, entry 20). Since 0.5.0 (entry 38) it also writes the tenant's two identity files, `tenants/<tenant>/principals.yaml` and `tokens.yaml`, on an admin's Add principal and Revoke; neither is a data layer. Since 0.6.0 (entry 40) it appends one line per push to the tenant's `tenants/<tenant>/ledger.jsonl` (since 0.14.0, entry 50, also one per action request: Portal asks, it runs nothing; since 0.14.1, entry 51, the file is one ledger the Runner appends its results to as well, under the same lock, and Portal only reads those), and sends a pushed card to a feed (an OCI registry or a NuGet feed) over HTTP; neither a ledger nor a feed is a data layer. Pester "never writes to a mounted layer" hashes the fixture before and after. This is Graph.Node design 11's "a layer is never edited" seen from a consumer.

## 3. PORTAL_MODE=production hides Designer and nothing else

With `PORTAL_MODE=production`, `GET /api/screen` leaves out every node that `is-a` the Category named `Designer`, directly or through Categories that `is-a` it, with the edges that touch those nodes. The Designer Category node itself stays. Nothing else changes: writes are still accepted, snaps still hold the full model (entry 13), State nodes of hidden nodes stay (they are not Designer), the Designer's section States among them since 0.3.1 (entry 28). Any other value of `PORTAL_MODE` than `production`, `development` or unset stops the binary at startup, so a typo never silently shows the Designer. The page needs no mode switch: it draws what it is given.

## 4. Graph.Node is lifted, not imported

`internal/graphnode/schemas` (byte for byte) and `internal/graphnode/graph` (its non-test files) plus `cmd/graphnode` (import path rewritten) are copies of Graph.Node at tag `v0.3.3` (a536b0b, since 0.12.1, entry 48; `v0.3.0`, fa8fbf3, from 0.6.0, entry 40; `v0.2.3`, 1807fd7, from 0.3.6, entry 34; `v0.2.1`, f804420, from 0.3.3, entry 31; `v0.2.0`, ee4df2c, before), with one registered edit up to v0.2.1 and none since v0.2.3, whose `ui:` row is already Portal's (entry 34): the `ui:` row of the id prefix registry (`graph/ids.go` and its prose twin `schemas/ids.md`) names `Portal` as owner, with Portal's shape, instead of reserving it (for GraphRenderer at v0.2.0, for this repo's 0.1.0 name at v0.2.1), because Portal issues every `ui:` id. Pester "Lifted Graph.Node" compares each file with `git show <tag>:<path>` after applying exactly that edit, so any other drift fails; the `graph-node` skill and `tests/fixtures/invalid.graph.json` are compared with the same tag. 0.2.0 pinned the lift to a tag rather than HEAD (the brief), so another session's uncommitted or later work in Graph.Node never moves it; a new tag is a new recopy task. The 0.1.0 text follows. The brief asks for the validator to be lifted; importing `github.com/JerryBalmer1/GraphNode` would need a tag that does not exist or a `replace` onto the sibling checkout, which the brief rules out for tests. This contradicts the copied graph-node skill's "do not copy GraphNode's files into your module"; the brief wins and the cost is drift, which Pester "Lifted Graph.Node" catches by comparing every lifted file against `git show HEAD:<path>` in `../Graph.Node`. It compares against HEAD, not the working tree, because another session was mid-way through Graph.Node 0.2.0 while this was built (uncommitted schema, CLI and fixture changes); a lift follows what Graph.Node has committed. When 0.2.0 is committed the test fails and the copy is redone in one task. Graph.Node's `validate.go` is not gofmt-clean upstream; the copy keeps its bytes.

## 5. Display lives in State and Theme nodes only

Graph.Node rejects a `display` key on a node or edge (code `schema`). The portal also refuses display properties inside `properties` on every Kind but the display Kinds, State and (since 0.2.0, entry 22) Theme: `display`, `color`, `shape`, `icon`, `collapsed`, `selected`, `scroll`, `opacity`, `font` (code `display`), and refuses them on edges. `label` is not on the list because Graph.Node's Category requires a `label` property. Per-node display state (`collapsed`, `selected`, `scroll`) is a State node `ui:state:<id without ui:>` joined by `has-state`, written by `PATCH /api/nodes/{id}` with `state`. Kind display (colour, shape, label template) for data layers comes from each layer's own ontology, as Graph.Node design 8 says.

## 6. OpenAPI first, lifted from GraphRenderer

`api/openapi.yaml` (OpenAPI 3.0.3, YAML as the brief names it, where GraphRenderer has JSON) is the API's source of truth. `go generate ./api/...` runs oapi-codegen v2.8.0 (the `tool` line in `go.mod`, GraphRenderer's pin) into one package, `internal/gen/api`: models, the chi server interface and the embedded spec. GraphRenderer splits servers by tag because it has two binaries; the portal has one. `internal/gen` is generated and never edited by hand. `internal/server` implements the interface; routes come only from the generated code. The page and its assets are operations too (`getIndex`, `getStatic`, tag `page`), as in GraphRenderer. Go tests lifted from GraphRenderer's harness validate every request and response against the spec (status codes included), check route parity both ways, and check that `web/client.js` ROUTES name every API operation by operationId with the spec's method and path. Pinned: Go 1.27.0, chi v5.3.2, oapi-codegen v2.8.0, kin-openapi v0.149.0 (tests), jsonschema/v6 v6.0.3, go.yaml.in/yaml/v3 v3.0.4, Cytoscape 3.34.3 (vendored from GraphRenderer).

Lifted from GraphRenderer: the spec-first layout, codegen config shape and `go:generate` directive; `Serve` (prints the bound URL), `writeJSON`, `writeError`, `writeAsset`, `paramError`, `notFound`, `methodNotAllowed`; the `web` package's flat static map and `ContentType`; the spec test harness (`newHarness`, `do`, `specOps`, route and client parity); `client.js`'s `call` and Error contract; the glass stylesheet and its eleven tokens (fixed in `:root`, no theme switching); the Canvas renderer's shapes, tinted-glass style and `splitAlpha`; vendored Cytoscape; the PowerShell binary launcher (free port, quoting, readiness poll, log redirect); `Find-HeadlessBrowser`. Written fresh: the store (screen, checks, layers, snaps, diff), every handler body, the ontology, `portal.js` (recursive Containers, Layout modes, is-a renderer resolution, State collapse), the Table, Detail and Designer renderers, the Canvas's data path, label templates and grey fallback, the PowerShell module's six functions and `.build.ps1`.

GraphRenderer's canvas reads `display.label` as a property name; Graph.Node defines it as a template (`{name}`, `{properties.x}`). The portal's Canvas implements the template, so FileSystem directories read `docs/`.

## 7. The dev seed

With no snap to boot from (entry 20) the binary seeds: root Container `ui:root`, grid Layout `ui:layout:root` (12 columns), Theme `ui:theme:default` joined by `themed-by` (0.2.0, entry 22), the four Categories `ui:category:Canvas`, `Table`, `Detail`, `Designer`, one Canvas Widget `ui:canvas` with source `filesystem` at bounds `{0,0,8,9}`, and the Designer Container `ui:designer` at `{8,0,4,9}`. Since 0.3.0 also the Category `ui:category:Menu` and one menu entry, the Container `ui:menu:graph` named "Graph" that opens `ui:canvas` (entry 25): 11 nodes, 8 edges. Since 0.3.1 also a State per Designer section, `ui:state:designer:<slug>`, each joined from `ui:designer` by `has-state` (entry 28): 18 nodes, 15 edges. The Designer Container is beyond the brief's seed list: without it a fresh portal has no way to add panels from the page. `-no-seed` leaves the screen empty. The seed is validated like any write; a seed that fails stops the binary. A snap that fails validation also stops it, with the problems and a hint to move it aside, rather than being reseeded over.

## 8. Every write validates the whole next screen

A write builds the next screen envelope and runs Graph.Node's `ValidateBytes` over all of it (for POSTs, with the raw body in place of the decoded element, so keys the Go types would drop, `display` above all, are still seen), then the portal's checks over the elements the write touched. Only a clean result replaces the current screen (in memory since 0.2.0; entry 20). Since Graph.Node 0.2.0 a problem carries a `severity`; only errors refuse a write, and a refusal lists errors only (entry 19). Checking the portal rules only on touched elements means a later change elsewhere (a Category deleted, a layer remounted) never blocks unrelated writes; it surfaces when the stale element is next written. Problem codes: Graph.Node's (`schema`, `duplicate-node`, `duplicate-edge`, `dangling-edge`, `edge-id`, `edgeid` (a malformed edge id, since 0.2.0), `counts`, `kind`, `stamp`) plus the portal's `id-prefix`, `unknown-kind`, `unknown-property`, `property-type` (wrong type, a missing required property, Layout `mode` not grid/flow/canvas, malformed `bounds` or `scroll`, a Theme `color` not #rrggbb, `opacity` outside 0 to 1, a blank `font`), `display`, `unknown-edge-kind`, `edge-endpoints`, `unknown-category`, `unknown-ref`, `unknown-field` (a top-level node key outside id, kind, name, properties, source, findings, stamp). Statuses: 400 for a body of the wrong shape, 404 for no such node, 409 for an id already taken, 422 with `problems` for everything else. Every write returns what it stored: the node, the edge, `{node, state}` for PATCH, `{node, edges}` for DELETE.

## 9. Ref: displays without a dangling edge

The brief's `displays` edge goes from a Widget to a node in a mounted data layer. That node is not in the screen envelope, so the edge would be Graph.Node's `dangling-edge`, an error. The portal adds a Kind `Ref` (`ui:ref:<layer>/<node id>`, properties `layer`, `node`), a stand-in on screen for one data node; `displays` goes Widget to Ref. Writing a Ref checks that the layer is mounted and holds the node (`unknown-ref`). The Detail renderer shows the node its Ref names. An extra Kind is a change to the brief's ontology and is the first thing to revisit if Graph.Node grows cross-envelope edges.

## 10. Categories, and Categories that refine Categories

Category is Graph.Node's base Kind, whose schema requires `properties.label`. The brief's Category has name and description: the name is the node's `name`, `label` repeats it, `description` is a property. The portal declares `is-a` from Category to Category as well as from Widget and Container, so a new panel type is a Category node plus `is-a` to an existing one, and the page draws it with the nearest base renderer up the chain (Canvas, Table, Detail, Designer are the only names bound to code). Graph.Node says only the taxonomy module writes Category nodes; the portal writes its own, in its own layer, which no taxonomy exists yet to own. Graph.Node's Category schema does not fire when `properties` is absent entirely, so a Category with no properties passes Graph.Node; the portal's own required-property check (`property-type`) still refuses it. Graph.Node 0.2.0 still has this gap (checked against the tag: no `properties` passes, `properties: {}` is `kind`), so the check stays Portal's own and a Go test pins it (`TestPortalOntologyChecks`). Closing the gap belongs to Graph.Node, as a 0.2.1 item: its Category schema should require `properties`. When that ships, the Go test expects `kind` as well; the portal's check stays, since it covers every portal Kind.

## 11. Widget and Container properties

`type` names a Category and must name one that is on screen (`unknown-category`); it is informational, the `is-a` edge decides the renderer. `source` is a mounted layer name with an optional query, `filesystem?kind=File&from=fs:src`, the same `kind` and `from` that `GET /api/layers/{name}/nodes` takes. `bounds` is `{x, y, w, h}`: grid cells under a grid Layout, pixels under a canvas Layout, ignored under flow; `w` and `h` above 0. Containers carry `bounds` too, beyond the brief's list, so a nested Container can be placed in its parent's grid. `parent` is informational; the `contains` edge decides. `order` sorts siblings; `visible: false` hides a node and what it contains. A Container with no Layout flows. Canvas selection (a node tapped on a Canvas or a row clicked on a Table) is passed to Detail renderers on the page only, not written: it is a viewer's pointer, not part of the screen.

## 12. Stamps

Every layer stamp the portal writes is UTC ending `Z` with exactly three fraction digits (`2026-10-09T01:02:03.456Z`), which Graph.Node's `utcStamp` allows; fixed width keeps string order equal to time order, and milliseconds keep two snaps a smoke test takes in the same second apart. Stamps are strictly increasing within a process. A snap's file name replaces `:` with `-` (`2026-10-09T01-02-03.456Z.graph.json`), the FileSystem emitter's convention; `GET /api/diff` accepts either form and refuses anything else (400), so no path can be smuggled in. Screen nodes carry no `stamp`.

## 13. Snaps and the diff

`POST /api/snap` writes the full screen (never production-filtered: a snap is the model, not a view) with `ontology: ontology.yaml` and the portal ontology copied beside it, after validating it. Since 0.2.0 it is the Approve action and the snap carries its note as an envelope finding (entry 21). `GET /api/diff` is Graph.Node design 11's merge done in memory, never written: upsert a then b on id; `firstSeen` the first snap an id is in, `lastSeen` the last; an element in a but not b is kept in `merged` with an `absent` finding (declared in `ontology.yaml`) rather than deleted. `added`, `removed` and `changed` are reported for nodes and edges; content compares everything but `stamp`. The merged envelope validates. Diff of a snap with itself is empty.

## 14. Layers

`POST /api/layers` takes `{name, path}` (a `.graph.json`, or a directory whose newest `*.graph.json` by name is taken) or `{name, envelope}`. The envelope must pass Graph.Node validation or the mount is refused (422) with its problems. Mounting a taken name replaces that layer. The envelope's `ontology` is resolved relative to the file (Graph.Node design 8); a URL is not fetched and an inline envelope cannot resolve a relative one; an ontology that cannot be read or fails `validate-ontology` leaves the layer mounted without one, `hasOntology: false` and `ontologyProblems` set, and its Canvas draws grey. Mounts are in memory only: a restart mounts what its flags say (since 0.3.5 also what the screen's Mount nodes record: a mount by path is a Mount node, which an approval saves, entry 33). `-filesystem` defaults to `C:\__Code\Graph.Emitter.FileSystem\artifacts\layers` and is skipped when absent; the newest layer there is copied to `tests/fixtures/layers/filesystem/` with its ontology, so tests need no sibling. Query: `from` keeps that node and everything reachable along edges in their direction (404 when the node is not there), then `kind` filters; edges survive when both ends do.

## 15. PATCH and DELETE

`PATCH /api/nodes/{id}` takes `properties` and `state` and nothing else (400 otherwise). A property set to `null` is removed. `state` upserts the State node and its `has-state` edge; a State node has no State (400). `DELETE /api/nodes/{id}` removes the node and every edge touching it, nothing more: children of a deleted Container stay as unreachable nodes, not drawn. There is no edge deletion route, because the brief's API has none; an edge goes when one of its nodes does.

## 16. Layout of the repo and the build

The Go module is at the repo root, as in Graph.Node, so `web/` and `ontology.yaml` can be embedded and `cmd/graphnode validate-ontology` reads as the brief writes it. Binaries go to `build/win-x64/` (`portal.exe`, `graphnode.exe`), which the brief's `.gitignore` names, rather than GraphRenderer's `src/<module>/bin`; 0.1.0 also built linux-x64, which nothing ran (entry 23). The PowerShell module `src/Portal` is thin wrappers over the API for the orchestrator: it records a started portal in `<artifacts>/portal.json` so `Stop-Portal` works from another process, and stamps stay strings (`ConvertFrom-Json -DateKind String` where PowerShell has it). Its names are the Portal verb-noun set (entry 18), so none clashes with the Graph module. Invoke-Build tasks: Modules (checks Go, Pester 5.5+, InvokeBuild; installs nothing), Generate, Build, Test (vet and test), Up, Status (0.3.0, entry 24), Down, Smoke, Pester, Screenshot; the default chain is Modules, Build, Test, Smoke, Pester. Smoke runs on a fresh `artifacts/smoke` over the fixture layer, so it is reproducible without siblings; Screenshot uses the sibling's newest layer when present. There is no DriftCheck for `internal/gen` yet: regenerate with Generate and let the diff show in review.

## 17. Where the repo is

The 0.1.0 brief named the repo by its old two-part name; the checkout is `C:\__Code\Portal`, and its workspace file named only `.` and `../GraphRenderer`. Work went into `C:\__Code\Portal` and `Portal.code-workspace` gained `../Graph.Node`. Since 0.2.0 the name is `Portal` everywhere (entry 18), and the run report's Run id takes the folder name, `Portal`.

## 18. One name: Portal

0.2.0's brief renames every mention of the old name to `Portal`: the Go module path `github.com/JerryBalmer1/Portal`, the envelope and ontology `module`, the spec title, the page title, the docs, the skills this repo owns and the run report. The PowerShell module moved to `src/Portal` (`Portal.psd1`, `Portal.psm1`) and its functions to one verb-noun set: `Start-Portal`, `Stop-Portal`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`, private helpers `Get-PortalBinaryPath`, `Get-PortalRecord`, `Invoke-PortalApi`, `Resolve-PortalUrl`. Graph exported `Start-GraphPortal` and `Stop-GraphPortal` for GraphRenderer, so the old names shadowed one pair (Graph 0.2.2, committed while this task ran, renames its pair `Start-GraphRenderer` and `Stop-GraphRenderer` and keeps the old names as deprecated aliases until its 0.3.0, so the new set here matters either way); Pester "shares no command name with the Graph module" checks the new set against Graph's psd1. Files were moved with `git mv`. A snap written by 0.1.0 still boots: the booted screen takes this binary's `module` and `version` (entry 20). The Pester test "names the repo Portal" scans every tracked and new file but the fixtures, so the old name cannot come back; this file therefore never spells it. `Portal.code-workspace` held no mention and is unchanged. The copied `run-report` skill names Graph's own functions, not this repo's; it is read-only and stays as Graph committed it (recopied at Graph's committed 0.2.2, and at Graph 0.2.3 in 0.3.0, entry 27).

## 19. Graph.Node 0.2.0 severities in the portal

Graph.Node 0.2.0 gives every problem a `severity`, `error` or `warning`, and only errors make an envelope or ontology invalid. Every place the portal gated on "any problem" now gates on `graph.HasErrors`: booting from a snap, a snap's validation, mounting a layer, the portal ontology, a layer ontology. A write's `check` keeps errors only, so a refusal never lists a warning and a warning never refuses a write; the portal's own problems carry `severity: error`, and the API's `Problem` gained an optional `severity`. One warning is promoted back to an error: 0.2.0 reports a `dangling-edge` whose missing end carries another owner's registered prefix (an `fs:` id, say) as a warning that a merge resolves. The screen is never merged with a data layer, so such an edge would dangle forever; the portal keeps it an error and Ref stays the way to point at data (entry 9). The FileSystem fixture's ontology now draws two kinds of warning (0.1.0 shape names, the 0.1.0 action inputs form); it still mounts with its ontology and colour, since warnings are not errors. The fixture is the FileSystem emitter's and is not edited here. The Canvas also learned 0.2.0's display block, since the sibling FileSystem layer already uses it: `circle` and `square` draw as Cytoscape's `ellipse` and `round-rectangle` (`diamond` and `hexagon` are Cytoscape names already, and the 0.1.0 names still work), `size` n draws 18 + 8n pixels wide (26, the old default, at 1), and an edge Kind's `style` (solid, dashed, dotted) is its line style. Without it, every node of that layer drew as a default circle.

## 20. Boot order: newest snap, PORTAL_SNAP, else seed

`store.New` boots the screen from, in order: the snap `PORTAL_SNAP=<stamp>` names (either stamp form; a stamp that is malformed, missing or invalid stops the binary rather than silently seeding); else the newest `artifacts/snaps/*.graph.json` whose name is a snap stamp, newest by name, which is newest by time (entry 12); else the dev seed, or an empty screen with `-no-seed`. `GET /api/screen` reports what it booted from in the response header `X-Portal-Boot`: the snap's file-safe stamp, `seed` or `empty`. A header and not an envelope key, because the boot is a fact about the process, not part of the screen graph, and an envelope key would end up in snaps. `Start-Portal` reads the header when it polls for readiness and records it as `Boot`; `-Snap` sets `PORTAL_SNAP` for the child, and an absent `-Snap` unsets an inherited one. The booted screen takes the binary's `module`, `version` and `ontology`, drops the snap's envelope findings (the approval note belongs to that snap), and gets a new layer stamp later than the snap's, so stamps keep increasing across restarts even if the clock moved back. With snaps as the boot source, the 0.1.0 live state file (`state/screen.graph.json`, and `state/ontology.yaml` beside it) had no reader left, so it is gone: the screen lives in memory and only an approved snap outlives the process. An unapproved change is lost on restart, on purpose; Approve is the save. Booting writes nothing.

## 21. Approve

The Designer panel gains an Approve form, a note and a button, first in the panel. It calls `POST /api/snap` with `{note}`; the body is optional (no body is an approval without a note) and refuses unknown keys and a note over 2000 bytes (400). The note is trimmed and stored in the snap as the envelope's one finding, Kind `approved`, severity `info` (declared in `ontology.yaml`'s `findingKinds`); an empty note is stored as "approved without a note" because Graph.Node requires a finding message. An envelope finding rather than a node, so a note never shows in a diff; rather than a side file, so a snap stays one self-describing envelope. The response adds `note`, `previous` (the newest snap on disk before this one, which is not necessarily the one the process booted from) and `diff`, the ids added, removed and changed against `previous`; both are absent for the first snap. `GET /api/diff` still gives the full diff. The Designer shows the stamp, the note and the summary and does not redraw, since approving changes nothing on screen. The top bar's 0.1.0 `snap` button did the same call without a note or a diff and showed only the stamp; with Approve in the Designer it was a second, poorer way to do one thing, and it would have let production mode, where the Designer is hidden, approve. It is removed (entry 23).

## 22. Theme

A Kind `Theme` (`ui:theme:<name>`) with `color` (accent, `#rrggbb`, the pattern Graph.Node's display colour uses), `opacity` (the glass surface's opacity, 0 to 1) and `font` (a CSS font-family), none required, joined from a Container by `themed-by` (Container to Theme only). Theme is display, which entry 5 kept to State; the brief asks for it as a node so a theme change shows in a diff like any other node, so Theme becomes the second display Kind and entry 5 says so, and `opacity` and `font` joined the refused display keys everywhere else. The seed's `ui:theme:default` on `ui:root` holds base.css's own values (`#f09648`, 0.11, the Segoe UI Variable stack), so a seeded page looks as before. The page sets a Container's Theme as the stylesheet's tokens on its box (`--gr-accent`, `--gr-surface` as white at that opacity, `font-family`), so it reaches everything inside without a Theme of its own; the top bar sits outside the root and keeps base.css. No Theme editor was added to the Designer: the brief did not ask for one, and `PATCH /api/nodes/ui:theme:default` changes one. Whether the Designer should edit Themes is an open question.

## 23. What 0.2.0 removed

The brief asks to remove what it did not ask for and nothing uses. Removed: the linux-x64 binaries in `build/` and the linux-x64 target in Invoke-Build Build, with the linux branch of the binary lookup (nothing runs the portal on Linux; the brief names Windows paths); the live state file and its writes (entry 20); the `X-Portal-Mode` response header (undocumented in the spec and read by nothing; `Start-Portal` records the mode itself); the `-host` flag (nothing passed it; the portal listens on 127.0.0.1 only, which was its default); the `-dev` flag and `web.FS(dir)` (serving the page from disk; no task, test or doc used it); the `*Server` that `server.New` returned beside the handler (every caller discarded it); `web.StaticNames` (no caller); `vendor/LICENSE` from the embed (never served: `/static/LICENSE` is 404; the file stays in the repo beside Cytoscape); the `.gr-bar label` rule (no label in the bar); the top bar's `snap` button (entry 21). Kept, with the reason: every API operation (each has a caller: the page, a PowerShell function, or Smoke); the Table and Detail Categories in the seed, which the 0.1.0 brief's seed names and the Designer's Add-panel form offers, so they are not unreferenced panels; `-mount` and `-no-seed`, which `Start-Portal -Mount` and `-NoSeed` expose; Ref, which Detail draws.

## 24. One fixed port: 8200

0.3.0's brief fixes the port. `Start-Portal` and Invoke-Build `Up` take the port from, in order: `-Port` (`Invoke-Build Up -Port <n>`), `PORTAL_PORT` in the repo's `.env`, the `PORTAL_PORT` environment variable, else 8200. `.env.example` at the root says `PORTAL_PORT=8200`; `.env` is gitignored (it already was). The `.env` reader and the busy-port check are lifted from Graph 0.2.2 (`Get-GraphRendererPort`, `Assert-GraphPortFree`), as the private `Get-PortalPort` and `Assert-PortalPortFree`, so the two portals of the family read their ports the same way (Graph's renderer is on 8100). Port 0, which 0.2.0 used to pick a free port, is refused by `Start-Portal`, by a `.env` or environment value, and by the binary itself (`-port 0`, now defaulting to 8200 instead of 8080). A busy port throws before the binary starts, naming the port and the owning process (`Get-NetTCPConnection`; when Windows cannot say, the message says so); the binary, started by hand on a busy port, stops with the port in its message. Neither picks another port. The tests, Smoke and Screenshot use 18200, so a portal on 8200 keeps running through a build; since every test now shares one port, Pester's Production-mode block reads the development screen and stops that portal before it starts the production one, rather than running both at once. Go tests keep `httptest`'s own listener: they never start the binary. Invoke-Build `Status` is new, because the brief names it and the repo had none: it prints the port, the URL and the boot stamp (live from `X-Portal-Boot`) of the portal recorded in `artifacts/portal.json`, or the port it would use and `not running`. It calls a new exported `Get-PortalStatus`, which is the seventh function and has its own Pester test, by the rule that every task's work lives in the module. `Up` now also prints the boot stamp.

## 25. Menu as nodes

The top bar's menu is graph, like every panel (entry 1). A Category `Menu` is seeded (`ui:category:Menu`, no `is-a`: it is not a renderer and refines nothing). A menu entry is a Container with these edges:

```
ui:menu:<slug>  --is-a-->   ui:category:Menu      exactly the Category named Menu
ui:menu:<slug>  --opens-->  <Container or Widget>  what the entry shows
```

The seed has one, `ui:menu:graph`, named "Graph", `properties: {order: 0}`, opening `ui:canvas`. `opens` is a new edge Kind in `ontology.yaml`: from Container, to Container or Widget. The brief says the entry opens "the filesystem Canvas container", but the seed's Canvas is the Widget `ui:canvas`, not a Container; rather than wrap it in a new Container (which would change every 0.2.0 snap's shape), `opens` may end at a Widget too. An `opens` to anything else (a Layout, a Theme) is `edge-endpoints`; from anything but a Container, the same. A menu entry is contained by nothing, so the screen tree never draws it; `portal.js`'s fallback root (no `screen.root`) skips menu entries for the same reason. The entry's name is its label; `order` sorts entries, then id. The page knows one name, `Menu`: the top bar is every Container with an `is-a` edge straight to the Category named Menu, and each entry's button shows what its first `opens` edge points at; an entry with none is drawn disabled. Choosing an entry expands the target if its State says collapsed (a `PATCH` with `state`, as the collapse toggle writes it), scrolls it into view and outlines it for a moment; which entry was chosen is the viewer's pointer, like a Canvas selection (entry 11), and is not written. The Go side knows `Menu` only as a seed name; no Go check treats menu entries differently from other Containers. Production mode keeps the menu: it is not the Designer. The Designer's new form "Add menu entry" (name, opens) is one `POST /api/nodes` (`{id: ui:menu:<slug>, kind: Container, name, properties: {order: <last + 1>}}`) and two `POST /api/edges`, with the form's usual rollback, and the page redraws from `GET /api/screen` so the entry appears in the top bar without a reload; Approve snaps it like any node. The form's targets are every Container and Widget but the root and menu entries; the Add-panel and Add-container parent lists leave menu entries out as well, since nothing inside one would be drawn. A 0.2.0 snap has no Menu Category, so it boots with an empty top bar, and the form says so if used; `POST /api/nodes` of `ui:category:Menu` (with `label`) brings the menu back. The seed's entry is not added to old snaps, since a snap is the model (entry 20).

## 26. The Designer's lists come from the API after every add

The brief asks that the Designer's lists refresh after every add. They already did one way: every successful form calls the page's `refresh`, which reads `GET /api/screen` and `GET /api/ontology` again and mounts every panel anew, the Designer with it. Every list and dropdown in the Designer is built from that freshly read screen, never from what the form just sent, and that stays the one way, so the top bar, new panels and the Designer cannot disagree. The Categories list now shows every Category node as a tree by refines depth: each Category whose `is-a` target is not a Category on screen (or that is in an `is-a` cycle) at the top, sorted by name, then what refines it, indented and prefixed `└`, depth first. The `category` and `refines` dropdowns use the same order with a `·` per level. The Add-panel `category` list keeps only Categories that are, or refine, a base renderer (so Menu, which has none, is not offered as a panel type). A Go test pins the API side: a Category added by `POST`, refining Canvas, is on `GET /api/screen` with its `is-a` edge (`TestRefinedCategoryIsOnTheScreenWithItsRefinesEdge`). The page side was checked in Chrome against a portal on 18200: after Add Category "Tree" refining Canvas the list read `Canvas`, `└ Tree (refines Canvas)`, `Designer`, `Detail`, `Menu`, `Table` and both dropdowns offered `· Tree`; after Add menu entry "Designer" the top bar held `ui:menu:graph` then `ui:menu:designer`, with no reload either time.

## 27. run-report skill recopied at Graph 0.2.3

The brief makes the recopy conditional: recopy `.claude/skills/run-report` once Graph 0.2.3 lands, whose skill is `version: 0.2.1`; leave it alone while Graph is at 0.2.2. When this task started, Graph's HEAD was 0.2.2 and 0.2.3 sat staged but uncommitted in its index, so the copy was left alone; Graph committed 0.2.3 (`eb0b9c4`) while this task ran, and Pester's version check went red, so the copy was redone from Graph's HEAD (`git show HEAD:<path>` of every file under the skill, never the working tree). Graph 0.2.3 decouples the skill's `version:` from Graph's `ModuleVersion` (its design.md 9), so the Pester check that compared the copy's version with Graph's committed `ModuleVersion` could never pass again. It now compares the copy's `version:` with the one in Graph's committed `SKILL.md`, and every file of the copy with Graph's committed file, byte for byte after line endings, the way the `graph-node` check already does against Graph.Node's tag.

## 28. Designer section state as nodes

0.3.1's brief makes each Designer section (Approve, Kinds (ontology.yaml), Categories, Add Panel, Add Container, Add Category, Add Menu Entry) a collapsible block: a head with the heading left and the panels' own `collapse`/`expand` button right, and a body that collapsing hides; the head stays. Open or closed is a State node, as a panel's is (entry 5), so it is in the graph, in a snap and in a diff:

```
ui:designer  --has-state-->  ui:state:designer:<slug>   {collapsed: true|false}
```

The slugs are `approve`, `kinds`, `categories`, `add-panel`, `add-container`, `add-category`, `add-menu-entry`. The id extends the State idRule (`ui:state:<id without ui:>`) with `:<slug>`; `ontology.yaml`'s State description and idRule say so (prose only: no Kind, property or edge Kind changed). Choices made without asking:

- **Owner.** The section States hang off the Designer Container by `has-state`, the edge every State already uses, rather than a new edge Kind or the Designer Category. A node now may have more than one State, so the page's `state(id)` takes the State whose id is the node's own (`ui:state:<id without ui:>`), and falls back to the first `has-state` target only when that is not a section's; before, it took the first, which would have read Approve's open State as the Designer panel's own. `PATCH` with `state` already upserts exactly `StateID(id)`, so Go needed no change there.
- **Seed.** The seed adds all seven, Approve and Categories `collapsed: false`, the other five `collapsed: true`, written out rather than left absent, so the defaults are visible in the graph. `store.DesignerSections` (Go) and `SECTIONS` in `web/designer.js` both list slug, heading and default; Go needs the slugs and defaults to seed, the page the headings to draw. A Go test reads `SECTIONS` off the served `designer.js` and fails unless the two lists match.
- **Old snaps.** A snap without them (every 0.3.0 snap) boots with the seven added at their defaults, checked like any write, when the snap has `ui:designer` as a Container; a section State the snap has is never overwritten. This is the one place a booted screen differs from its snap: the first approval after such a boot lists the seven States and their edges as added. A snap without `ui:designer` gets none. Entry 25 left the seed's menu entry out of old snaps because a snap is the model; section state is display, defaults for a block the old page did not have, so the brief's "get the defaults" is taken as adding them.
- **Other Designers.** A second Container that `is-a` Designer reads `ui:state:<its id>:<slug>`, absent at first, and draws the defaults; its first toggle `POST`s the State and its `has-state` edge. Only the seed Designer is backfilled at boot.
- **Writes and redraw.** A toggle `PATCH`es the section State's `collapsed` (or `POST`s it when absent), then shows the written value in place. It does not call the page's `refresh` as the panel toggle does, because a refresh mounts every panel again and re-runs the Canvas's force layout, so the graph would jump each time a form was opened. The next refresh reads the same value back from `GET /api/screen`, so the page and the graph agree.
- **Production.** Entry 3 stands: hidden nodes' States stay, the seven section States with them, without their `has-state` edges (which touch the hidden Designer).
- **Deletion.** `DELETE /api/nodes/ui:designer` removes its edges, the seven `has-state` among them, and leaves the States, as for any node's State (entry 15).

Casing: every heading and button in the Designer is Title Case, each button saying its section's heading (`Approve`, `Add Panel`, `Add Container`, `Add Category`, `Add Menu Entry`); field labels stay lower case and the panel chrome (`collapse`, `expand`) is unchanged, as the brief asks. The section toggles reuse that chrome's words. Coverage: Go (`TestSeedGivesEachDesignerSectionAState`, `TestSectionStateRoundTripsThroughSnap`, `TestSnapWithoutSectionStatesBootsWithTheDefaults`, `TestServedDesignerHasTheSevenTitleCaseSections`) and Pester (the seeded States; the page drawn by headless Chrome `--dump-dom` with the seven headings, the five buttons and the collapsed classes; a `PATCH` through an approval; a 0.3.0-shaped snap booting with the defaults).

## 29. Canvas fit timing

The brief reports that on first load the Canvas laid out and fit before its container had a size, so the graph was drawn a few pixels wide until something resized the window. Cytoscape reads its container's size when it is made and on `resize()`; nothing called `resize()`. `canvas.js` now has one `fit`: when the Cytoscape root has a non-zero width and height it calls `cy.resize()` then `cy.fit()` (24 px padding), and does nothing otherwise. It runs on the next animation frame after the layout, and from a `ResizeObserver` on the panel body, debounced to 100 ms. The observer also covers "refit after a panel expands": a collapsed panel's body is `display: none`, and its first non-zero size fires the observer. Today an expand redraws the page from the graph and mounts a fresh Canvas, which the frame callback fits, and the observer catches anything that resizes it in place. `destroy` disconnects the observer and clears the timer.

After each fit the Canvas host records `data-fit="<graph>/<panel>"`: the drawn graph's width (Cytoscape's rendered bounding box of all elements) and its panel's, in CSS pixels, as it already records `data-coloured`. Smoke's new `Test-CanvasFit` loads the page in headless Chrome at 1400×900 (`--dump-dom`) and fails unless the graph is wider than half the panel; it skips, and says so in `smoke-report.json`, without Chrome. Measured on the fixture layer: 662 of 903 px (0.733). A build with the two fit calls taken out, for comparison only and not kept, measured 650 of 903 in the same headless run: headless Chrome gives the container its size before the layer's nodes arrive, so it did not reproduce the zero-size first load. The check therefore guards against regressions (a graph left tiny, or no fit at all, which fails as "recorded no fit"); it does not reproduce the case the brief saw.

## 30. git-guard hook and run-report 0.2.2

0.3.2's brief copies `.claude/skills/git-guard` and `.claude/skills/run-report` from Graph at HEAD (`bdfd2a5`, Graph 0.2.8), each file by `git show HEAD:<path>`, never Graph's working tree, replacing the old copy whole. `run-report` goes from skill `version: 0.2.1` to `0.2.2`: the report is written once, to `<repo>\artifacts\run-report.json` of the repo the task ran in, and never to `C:\__Code\Graph\artifacts`. `git-guard` is new at `version: 0.1.0`. Both are read-only here and recopied when Graph's committed `version:` changes. The Pester check that compared `run-report` with Graph's committed skill now runs for both: the `version:` line, the full text of every committed file after line endings, and no file in the copy that Graph's lacks.

- **settings.json.** The repo had no `.claude/settings.json`, so merging `settings.fragment.json` "keeping existing hooks" left nothing to keep: the file is the fragment. Pester checks that `PreToolUse` holds exactly one entry with the fragment's matcher (`Bash|PowerShell`) and command, so a later merge that adds hooks beside it still passes and one that drops or doubles it fails. The file is tracked; only a `settings.local.json` would be personal.
- **Hook test.** Pester pipes the hook JSON for six commands (status, add, commit, push, tag, and gh pr merge) to `pwsh -NoProfile -File .claude/skills/git-guard/hook.ps1` and expects 0, 0, 2, 2, 2, 2, with the `git-guard:` line on stderr for each 2. The brief's last command differs from the skill's own list (which ends with a chained push); the brief's list is what is pinned.
- **The hook is live for agents in this repo.** Once the file existed, it refused one of this task's own shell calls: a heredoc writing this entry, whose text quoted the denied commands. Nothing ran. Text that names the commands goes through the editor tools, which the hook does not see (the skill's Limits: it matches command text, not intent).
- **Report task.** The brief asks to correct `.build.ps1` or any doc that says the Report task also writes to `C:\__Code\Graph\artifacts`. Portal's `.build.ps1` has no Report task, and no README, ONTOLOGY.md, design.md or check-list line named Graph's `artifacts`. The only such text was in the old `run-report` copy, which the recopy replaces, so nothing else changed. A run report for a Portal task lands in `C:\__Code\Portal\artifacts\run-report.json` (`artifacts/` is gitignored here).
- **Not changed.** CLAUDE.md still says Pester checks the copies against "Graph's committed psd1"; since 0.3.0 (entry 27) it checks Graph's committed skill. The agent rules are Jerry's to edit.

## 31. graph-node 0.2.1 and the lift at Graph.Node v0.2.1

0.3.3's brief is a workspace-wide skill recopy from Graph at HEAD (`git-guard` 0.1.0, `run-report` 0.2.2, `graph-node` 0.2.1). Portal's `git-guard` and `run-report` already matched Graph's committed files byte for byte; only `graph-node/SKILL.md` differed (0.2.0 here). Here that skill shares one pin with the lift (entry 4), so recopying it alone would have split the pin. Asked, Jerry chose to move the pin: `$GraphNodeTag` is `v0.2.1` and the skill, `internal/graphnode/schemas`, `internal/graphnode/graph` (non-test files) and `cmd/graphnode/main.go` are recopied from Graph.Node `v0.2.1` (f804420). Graph's committed `graph-node` is byte for byte Graph.Node v0.2.1's, so both sources agree.

- **What v0.2.1 brings.** `cmd/graphnode` reports `0.2.1`; the registry gains `alert:` and `ledger:` (owned by Claude.Agent.Alerts) with shape checks in `graph.ValidID`; upstream's `ui:` row now reserves the prefix for this repo's 0.1.0 name instead of GraphRenderer. Nothing in Portal issues `alert:` or `ledger:` ids. `tests/fixtures/invalid.graph.json` is unchanged between the tags.
- **The registered edit stays.** It still turns upstream's `ui:` row into `Owner: "Portal"` with Portal's shape; only the row it replaces changed. Pester builds that row's old text by joining two strings, because "names the repo Portal" fails on any tracked file that spells the 0.1.0 name.
- **The graph-node copy is exempt from "names the repo Portal".** The 0.2.1 skill's prefix table spells the 0.1.0 name, and the copy is read-only, so the check skips `.claude/skills/graph-node/` as it skips fixtures and vendored files.
- **graph-node is checked twice.** Against Graph.Node's pinned tag (entry 4) and, beside `run-report` and `git-guard`, against Graph's committed skill (version line, full text, no extra files), as the brief asks. A Graph HEAD that moves past the pinned tag fails the second check, and that starts a new recopy task. Since 0.3.6 the second check is only that Graph's copy is not newer (entry 34).
- **settings.json and Report task.** Only `git-guard` has a `settings.fragment.json`, and its entry is already in `.claude/settings.json` once (entry 30), so the merge changed nothing. No doc or build task says the Report task writes to Graph's `artifacts/` (entry 30), and `artifacts/` is gitignored.
- **Version 0.3.3** in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated for the embedded spec.
- **Not changed.** CLAUDE.md still names the pinned tag as v0.2.0. The agent rules are Jerry's to edit.

## 32. Up replaces a running portal

0.3.4's brief: when a portal already holds the port, `Invoke-Build Up` calls `Stop-Portal`, waits for the port to free (bounded, 10 s), starts, and logs `replaced <old stamp> with <new stamp>`; `-NoReplace` keeps the throw. The logic lives in two private functions, `Invoke-PortalUp` and `Wait-PortalPortFree`, so Pester can drive it on the test port over its own artifacts; Up calls `Invoke-PortalUp` in the module's scope.

- **"A portal holds the port" means the portal recorded in `artifacts/portal.json` is alive** (`Get-PortalStatus` Running). `Stop-Portal` stops only what it recorded, so a port held by anything else, a portal started over other artifacts included, is never stopped: `Start-Portal` throws naming the owner, as before. Stopping an unrecorded process by port would be a guess.
- **The wait is on the recorded portal's port**, polled every 200 ms with `Assert-PortalPortFree`. After 10 s it throws `port <n> is still held 10 s after Stop-Portal: <owner>` and does not start; Pester runs the same path with a 1 s bound against a listener the test holds.
- **The stamps** are the old portal's `X-Portal-Boot` as `Get-PortalStatus` reads it (else the recorded one) and the new record's `Boot`. They can be equal (both the newest snap); the line is printed anyway, since a replacement happened.
- **`-NoReplace`** goes straight to `Start-Portal`, so the message is 0.3.3's `A portal is already running at … Stop it with Stop-Portal first.`
- **Report.** Portal's `.build.ps1` still has no Report task; the run report is written with Graph's `New-GraphRunReport -Repo C:\__Code\Portal`, as the run-report skill says for an agent's ad-hoc script.
- **Version 0.3.4** in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated for the embedded spec.

## 33. A mount by path is a Mount node; the top bar shows the open view's layer

Claude.Agent's `Up` posts `{name: agent-runs, path}` to `POST /api/layers`. Portal mounted the layer and held it, but nothing drew it: every menu entry opened `ui:canvas`, whose source is `filesystem`. The top bar showed the screen graph's own counts and stamp, never a data layer's, and a restart lost the mount (entry 14). Asked, Jerry chose the graph-native fix: the mount is recorded in the screen graph and the top bar names the open view's layer.

```
ui:mount:<layer>   Kind Mount   {layer, path}   no edges; draws nothing
```

- **Recorded on the screen, saved only by Approve.** A `POST /api/layers` by path upserts `ui:mount:<name>`, checked like any write. A new Kind `Mount` (`layer`, `path`, both required; the id must be `ui:mount:<layer>` and `layer` a layer name, else `property-type`), not a property on the Widget, because a layer is mounted once and drawn by any number of Widgets. Mounts do not get a side file: entry 20 keeps `POST /api/snap` the only way anything reaches disk, and entry 1 keeps all page state in the graph. So the restart rule stays as before. A mount survives a restart once an approval holds its Mount node; an unapproved mount is lost, like any unapproved change.
- **The path as given, made absolute.** A directory stays a directory, so at boot the newest `*.graph.json` in it is mounted, not the file that was newest when it was recorded. A caller that posts a directory (Claude.Agent's `artifacts\layers`) gets its latest run after every restart; a caller that posts one file gets that file.
- **Remounting the same path writes nothing.** The screen and its stamp stay as they are, so a caller that posts on every start does not churn the screen. A different path updates the node. A mount of that name from an inline envelope removes the node, since an inline envelope cannot be read again and the node would mount a stale file.
- **Boot.** `cmd/portal` boots the screen, then `Remount` mounts each Mount node's layer in screen order, then the flags (`-filesystem`, `-mount`) mount theirs, so a flag naming the same layer wins. Flag mounts go through `MountAtStart` and are never recorded: the flags mount again on every start. A recorded layer that cannot be mounted (its path gone, its envelope invalid) is printed `portal: not mounted: ui:mount:<name>: …` and skipped. Its node stays, and the boot goes on, unlike a bad snap, which stops it (entry 20): a data layer belongs to another repo and may come back. `store.New` mounts nothing itself, so booting still writes and reads nothing beyond the snap.
- **Top bar.** `#stats` shows `<layer> · <n> nodes · <m> edges · <stamp>` for the open view's layer, from `GET /api/layers` (the layer's whole envelope, not a Widget's query). The open view is the target of the menu entry the viewer last chose, else the first entry's target by order, else the root. Like a Canvas selection, it is a viewer's pointer and is not written. A target's layer is its `source`, or for a Container the first `source` in what it holds, depth first in drawing order. A source that is not mounted reads `<layer> · not mounted`; no source reads `no layer`. The screen graph's counts and stamp move to a badge beside it, `screen <n> · <m> · <stamp>`. `#stats` carries `data-layer` for tests.
- **The live screen, not the seed.** Until 0.3.6, which seeds both (entry 35). The `agent-runs` Canvas Widget and the Runs entry's `opens` edge are screen-graph changes, not seeded: the seed knows only the FileSystem layer. On the 8200 portal they were made through the API and approved (snap `2026-10-09T11-20-42.411Z`, under the gitignored `artifacts/`): `ui:canvas:agent-runs` (Canvas, `source: agent-runs`, bounds `{0, 9, 8, 9}`, contained by `ui:root`), and `ui:menu:runs` deleted and posted again opening it, since there is no edge deletion route (entry 15). After a restart the page's Runs entry read `agent-runs · 169 nodes · 154 edges · 2026-10-09T10:29:45Z`. Claude.Agent posting its layer directory rather than one file is that repo's change.
- **Coverage.** Go: `TestPathMountIsRecordedAndRemountedAfterApprove`, `TestInlineMountRemovesTheMountNode`, `TestRemountOfAMissingPathFailsThatLayerOnly`, `TestMountNodeChecks`, `TestPathMountSurvivesApproveAndRestart`. Pester: the Mount node after `Mount-PortalLayer` by path and none by envelope or flag, and the top bar from headless Chrome `--dump-dom` (layer, counts and stamp; the screen badge).
- **Version 0.3.5** in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated for the embedded spec.

## 34. The lift and graph-node at Graph.Node v0.2.3

0.3.6's brief, Jerry's answer to the 0.3.5 status report: recopy Graph.Node at v0.2.3, bump `$GraphNodeTag`, re-check the `ui:` edit, recopy `graph-node` if its version moved. `internal/graphnode/schemas`, `internal/graphnode/graph` (non-test files), `cmd/graphnode/main.go` and `.claude/skills/graph-node` (three files) are `git show v0.2.3:<path>` (1807fd7), the Go files with the import path rewritten. The file sets are the same as at v0.2.1.

- **What v0.2.3 brings.** `graphnode version` prints 0.2.3. The registry gains `agent:`, `runner:`, `git:` and `ref:` with shape checks in `graph.ValidID`. The ontology schema gains the `triangle` and `star` shapes and a Kind display's `when` override rules. The envelope may carry a `source: {emitter, version, target, stamp}` block. A dangling edge onto the envelope's own module's prefix stays an error under `graph.Validate`, which is what Portal calls. Portal issues none of the new prefixes and declares no `when` rules, and its Go code compiled and passed unchanged. `tests/fixtures/invalid.graph.json` is unchanged between the tags.
- **No registered edit.** At v0.2.3 upstream's `ui:` row is exactly the row Portal's edit wrote: `Owner: "Portal"`, Portal's shape, in `graph/ids.go` and `schemas/ids.md`. The lift is upstream's but for the import path. `$Registered` in Pester stays as the place a future edit goes, and is empty. Its old text was the only place this repo spelled the 0.1.0 name in pieces, so that is gone too. "registers ui: as Portal's own" still checks `graphnode prefixes`.
- **graph-node follows the tag, not Graph's copy.** The brief recopies the skill when its version moves, and it moved: 0.2.1 to 0.2.3. Graph's committed copy (HEAD bdfd2a5, Graph 0.2.8) is still 0.2.1, so the 0.3.3 check that the copy equals Graph's (entry 31) and the check that it equals the pinned tag's cannot both pass. Graph.Node is where the skill is written, and Graph's is a copy of it, so the pinned tag wins. Pester drops graph-node from the "Graph's committed skill" check and adds "graph-node is never older than Graph's committed copy": a Graph copy newer than ours fails, which means a newer Graph.Node tag to name and recopy. `run-report` and `git-guard` are still compared with Graph's committed files in full. This makes the CLAUDE.md line "Pester checks each against Graph's committed skill" untrue for graph-node. CLAUDE.md is Jerry's to edit (entry 31), so the report proposes the wording.
- **Version 0.3.6** in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated for the embedded spec.

## 35. The seed has the Runs entry and its agent-runs Canvas

0.3.5 made the Runs view on the 8200 portal's screen and approved it, and a fresh checkout booted without it (entry 33). Asked, Jerry chose the seed. The seed gains the two nodes and four edges of that approved snap, with the same ids and values:

```
ui:canvas:agent-runs  Widget     {type: Canvas, source: agent-runs, bounds {0, 9, 8, 9}}  contained by ui:root, is-a Canvas
ui:menu:runs          Container  "Runs" {order: 1}                                        is-a Menu, opens ui:canvas:agent-runs
```

- **Order 1, not the snap's 3.** The snap's 3 came from entries added and deleted on the live screen. In the seed, Runs is the second entry after Graph (0). Smoke's own entry moves from order 1 to 2 so no two entries share an order, and its top-bar check expects Graph, Runs, Smoke. The README's menu-entry example moves to order 2 for the same reason.
- **No Mount node in the seed.** A Mount records a path (entry 33), and the only path is on this machine (`C:\__Code\Claude.Agent\artifacts\layers`). Claude.Agent's `Up` posts the mount, and an approval then records it. Until then the Runs Canvas shows the layer API's 404 in its panel and the top bar reads `agent-runs · not mounted` when Runs is chosen. Nothing new was added to the renderer for that state.
- **Booting from a snap is unchanged.** The seed is used only when there is no snap (entry 20). The 8200 portal's newest snap already holds both nodes, and an older snap boots without them, as before. Unlike the Designer's section States (entry 28), the seed's Widgets and menu entries are not added to an old snap at boot: they are screen content, which the approved snap decides.
- **Screenshot mounts agent-runs.** `Invoke-Build Screenshot` passes `-Mount agent-runs=C:\__Code\Claude.Agent\artifacts\layers` when that directory exists, as it already uses the FileSystem sibling's layers, so the picture shows the Runs Canvas drawn rather than its error. Smoke and Pester do not: they run without siblings.
- **Coverage.** Go `TestSeedValidatesAndBootsFromSeed` (20 nodes) and `TestSeedMenuEntryIsAContainerThatIsAMenuAndOpensTheCanvas` (both entries, their order, and the Canvas each opens with its source). Pester "seeds the menu as nodes" (both entries, their Canvases contained by `ui:root`, no Mount node). Smoke's seed id list and top bar.

## 36. The shell, tenants, tokens and service connections

0.4.0's brief: a DevOps-style shell (banner, left rail of services, breadcrumb, main pane) in a dark theme, built as a tenant consuming the product; a base and tenant config split; the tenant in every API path; a bearer-token stub; service connections with GitHub first. Three points clashed with entries 1 and 25 and with the API's shape, and Jerry chose:

- **The menu is config served as an envelope.** `portal.base.yaml`'s `menu:` tree, merged with the tenant's overlay, is served by `GET /api/tenants/{tenant}/menu` as a Graph.Node envelope, so entry 1 holds for the rail too. It is not the screen graph. It is built at boot, checked with Graph.Node and the portal ontology (a menu that fails stops the binary), and never written or snapped.
- **Every route moves under `/api/tenants/{tenant}/`**, and `POST /api/layers` is kept, deprecated, as the mount for this process's tenant, so Claude.Agent's Up still finds its path.
- **A token is required for every write**: the page, the PowerShell module, Smoke and Claude.Agent alike. Claude.Agent's mount now answers 401 until it sends one; that is that repo's change.

Choices made without asking:

- **Two Kinds and an edge for the menu.** `Service` (`ui:rail:<service>`: `path`, `order`, `dock`) and `Page` (`ui:rail:<service>:<page>`, one more `:<id>` per level: `path`, `order`, `nodeKind`, `view`, `screen`, `sections`, `filter`), joined parent to child by `has-page`. These are new Kinds rather than menu-entry Containers (entry 25), because a rail entry carries a route and a Kind to list, which Container does not declare, and because entry 25's `ui:menu:graph` would collide in meaning with the Graph service. Icons are display, so a Service's or Page's icon is its State's `icon` (`ui:state:rail:...`, joined by `has-state`); State gains `icon`, and entry 5 stands.
- **Item fields beyond the brief's `{id, label, icon, path, children}`.** `kind` (the Kind a table page lists; Graph.Node's kind pattern), `view` (`table`, `screen` or `connections`), `screen` (the screen node a screen page draws), `sections` (a Designer page's section slugs), `filter` (values offered as chips) and `dock` (`bottom` pins Settings). `path` must be the parent's path plus the item's id, so a route and its menu node always agree.
- **Page Kinds.** Each leaf names a Kind of its own, dotted by service (`portal.summary`, `emitter.run`, `workflow.run.step`, `environment.instance`, ...). Nothing emits them yet, so those pages show their empty state. Designer/Themes lists `Theme`, so it shows the seed's Theme. Graph/Explore draws `ui:canvas`. Designer/Sections draws `ui:designer`, and Designer/Export draws it with only `approve`.
- **Workflows.** "A workflow has Plan, Runs (a run has Steps), History" is the All page's children: Plan, Runs (holding Steps) and History are Pages under `ui:rail:workflows:all`. The rail shows a service's pages only. A Page's own Pages are the tabs of its item page, `/<tenant>/workflows/all/<id>`, flattened (`Runs › Steps`), with the chosen tab in `?tab=<path>`. Each tab is a table of its Kind, not yet filtered to the item: nothing links a run to its workflow yet.
- **Instances' kinds** are `filter: [container, compose, kubernetes, host]`: chips over each node's `properties.kind`, shown even when the page is empty.
- **Config.** `portal.base.yaml` and `schema/portal.schema.json` are embedded in the binary, so a tenant cannot edit the base on disk. The schema requires nothing, so an overlay names only what it changes. After the merge the binary checks that the product is named, that every entry has a label and a path, that ids are unique among siblings, that each page knows what it draws, and that each service has a page. A tenant without `portal.yaml` stops the binary; `tenants/default/portal.yaml` is committed as `{}`.
- **Merge.** Mappings merge key by key. A list whose items all carry a string `id` merges item by item on `id`, with new ids appended in overlay order. Anything else in the overlay replaces the base's, `null` included. So a tenant can relabel one page, add a service or narrow a filter without restating the menu.
- **One tenant per process.** `PORTAL_TENANT` (default `default`) names it, and `-tenants` (default `tenants`) holds `<tenant>/portal.yaml`, `tokens.yaml` and `connections.yaml`. Any other tenant in a path is 404. Tenant files are read once, at boot; a new token or connection needs a restart. Settings/Tenants lists `portal.tenant` nodes, which nothing emits.
- **Shell routes.** `/<tenant>/<service>/<page>` and `/<tenant>/<service>/<page>/<id>` are spec operations (`getShellPage`, `getShellItem`) serving `index.html` for a page the menu has, and 404 otherwise. `/` is a 302 to the first page of the first service not docked at the bottom.
- **The page's own pointers.** Which service is expanded (by default the route's), whether the rail is collapsed, the search text and the chosen tab are the viewer's pointers, like a Canvas selection (entry 11). They are not written to the graph or to storage; the tab is in the URL. Search filters the rows of the table on screen.
- **`GET /api/tenants/{tenant}/nodes?kind=`** lists one Kind across the screen (as `getScreen` shows it, so production hides the Designer here too) and every mounted layer, first id wins. It returns nodes only, since edges between envelopes would dangle. A node without a stamp gets `stamp.layer` from its envelope, which the table shows as `updated`. `kind` is required (400).
- **Tokens.** `tokens.yaml` holds `{name, sha256}` entries, the SHA-256 of the token in lower-case hex, compared in constant time. Only `Authorization: Bearer <token>` counts. No header means `anonymous`, which may GET and nothing else (401 with `WWW-Authenticate`). A token the tenant does not hold is 401 on any request, so a mistyped token never passes as anonymous. `New-PortalToken` is the eighth exported function: it mints 32 random bytes, appends the hash and returns the token once. It writes under `tenants/`, but it is the module and not the binary, so entry 2 stands. The module sends `PORTAL_TOKEN`. The page takes a token in the avatar's panel and keeps it in `sessionStorage` for the tab: it is the viewer's credential, not page state. A token the portal refuses is dropped.
- **Marker.** Nothing issues markers, so whoami's `marker` is always null and the banner reads `marker: none`.
- **Connections.** `connections.yaml` holds `{name, type, url, token_env}`; `github` is the only type, and its `url` defaults to `https://api.github.com`. The secret is read from `token_env` when a call needs it and never stored or served. The list carries `tokenSet` instead. `GET .../connections/{name}/repos` calls `GET <url>/user/repos?per_page=100&sort=full_name`, following pages to 1000 repos, and answers an envelope of `git.repository` nodes. They are validated, with ids `ui:conn:<connection>/<owner>/<repo>`: Portal issues them, and `git:`'s registered shapes have no repository. An unset variable is 422 `no_token`; a GitHub error is 502 carrying GitHub's message. It is a GET, but it spends the tenant's GitHub token, so it needs a principal: an anonymous call is 401 like a write. The middleware lets every GET through, and the handler refuses the anonymous principal itself. (It was first built as an open read; Jerry asked for the token in the same release.)
- **What the old page lost.** The top bar's screen menu (entry 25) and its layer stats (entry 33) are gone: the rail is the tenant's menu. The layer stats moved to the toolbar of a page that draws a screen node (`#stats` with `data-layer`, the `screen …` badge beside it), and the menu entries stay in the seed, in snaps and in the Designer's Add Menu Entry, though nothing draws them. Whether to retire entry 25, or show screen menu entries in the rail, is open. The root grid is no longer drawn whole; a page draws one screen node.
- **Dark theme.** base.css keeps its `--gr-*` tokens with dark values: near-black base, a 32 px grid of faint lines, solid cards, no gradient backdrop, no blur. A Container's Theme still sets them on what it holds (entry 22); the shell chrome reads base.css as the top bar did. The 8088 portal-temp named as the model was not reachable, so the look follows the brief's words, not that page.
- **Canvas fit.** A page-wide Canvas is fitted by its height first, so the graph can be less than half the panel's width while it fills the height. `canvas.js` now also records `data-fit-height`, and Smoke's `Test-CanvasFit` passes when either dimension is above half (0.853 here). Entry 29's regression check is otherwise unchanged.
- **Screenshots.** `Invoke-Build Screenshot` writes `summary.png`, `service-connections.png` and `explore.png` over a temporary copy of `tenants/default` holding the example connection; `portal.png` is removed.
- **Coverage.** Go: `internal/tenant` (base only, base plus overlay, bad overlay, token and connection files, merge) and `internal/server`'s `TestAuth…`, `TestConnectionsHideSecrets`, `TestGitHubReposEndpoint` (a fake GitHub, two pages), `TestMenuShape`, `TestNodesOfKindSpansScreenAndLayers` and `TestShellRoutes`. Every operation is checked against the spec with the test token. Pester covers whoami, 401 without a token and with a bad one, the repos endpoint against a fake GitHub on localhost (no token 401, token 200), the menu, config and routes, the rail drawn from the menu, `New-PortalToken`, a bad overlay stopping the binary, and the gitignore. Smoke adds the anonymous-write 401 and the rail.
- **Version 0.4.0** in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated.

## 37. Read-only page for anonymous; the wheel zoom speed

0.4.1's brief: when the principal is anonymous, every control that would write is disabled with the "sign in to write" tooltip, a write the portal still refuses (401) is a toast, and an error body is never drawn inside a panel; and a theme key `graph.wheelZoomSpeed` (default 1.0, range 0.1 to 5) in `portal.base.yaml`, validated by the schema, which a tenant overlay may override and the Canvas reads to scale its wheel zoom step.

Choices made without asking:

- **Who is read-only.** The page reads whoami's `writes`: false (`anonymous`, or whoami failing) is read-only. The toolbar's `read-only: sign in to write` badge follows the same test.
- **Which controls.** Every button that would write: a panel's collapse/expand (PATCH), the Designer's section toggles (PATCH or POST), Approve (POST snap) and the Add Panel, Add Container, Add Category and Add Menu Entry submits. Each is `disabled`, `title="sign in to write"` and `data-readonly="true"` (`web/toast.js`'s `guard`). Form fields stay editable; only the button that sends is off. Settings/Service connections' Test is a GET and is left as it was: it still answers `unauthorized` in its own row (entry 36).
- **Toasts for every refused write, not only 401.** `web/toast.js` shows `code: message`, and a refusal's problems one per line, bottom right, for 5 s. The collapse toggle used to print the whole error JSON into the panel body, and the Designer's status line printed refusals; both now toast, and the Designer's status keeps only Approve's result. A panel's read errors (a layer that will not load, a Category with no renderer) still draw in the panel: they are what the panel is, not a write that failed. A refused collapse no longer redraws the page.
- **The theme block is config, not node display.** Entry 5 keeps display on State and Theme nodes; `theme` in the config is the shell's own settings (how the wheel feels), not how a node looks, so it is not a node and is not in a diff or a snap. It is `theme.graph.wheelZoomSpeed` in `portal.base.yaml` and `schema/portal.schema.json` (`minimum` 0.1, `maximum` 5; `additionalProperties: false` at `theme` and `theme.graph`), and in the spec's `Config` schema, so `GET .../config` documents it. A value outside the range, in the base or an overlay, stops the binary like any schema failure. The overlay merges as any mapping does (entry 36).
- **What the speed scales.** The product's wheel step is Cytoscape's `wheelSensitivity` 0.2, as before; the Canvas uses `0.2 × wheelZoomSpeed`, and records it as `data-wheel-sensitivity` on its host for the tests. The page clamps to 0.1 to 5 again and takes 1 when the key is absent.
- **The old example.** `theme: dark` was the Go and Pester example of an unknown top-level key; `theme` is now known, so the example is `colours: dark`, and `theme: dark` is a type error at `/theme`.
- **Coverage.** Go `TestWheelZoomSpeed` (base 1.0; 2.5, 0.1 and 5 from an overlay) and four new `TestBadOverlayFailsBoot` cases (`theme: dark`, 10, 0.05, an unknown key under `graph`). Pester: an anonymous PATCH collapse of `ui:canvas` is 401, the screen holds no collapsed State for it, and headless Chrome (anonymous) still draws the Canvas with its nodes, its collapse disabled with the tooltip, no `gr-error`, and `data-wheel-sensitivity="0.2"`; every Designer button is disabled with the tooltip; an overlay with `wheelZoomSpeed: 10` stops the binary naming the file and `/theme/graph/wheelZoomSpeed`. No automated test clicks a disabled button or sees the toast (`--dump-dom` cannot click); both were driven by script in Chrome for manual item 2.6.
- **Version 0.4.1** in `Portal.psd1`, `ontology.yaml` (no Kind changed), `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated.

## 38. Identity: principals, roles, tokens that belong to a principal

0.5.0's brief:
- A node Kind `Principal`, served under the tenant: `name`, `type` (user or agent), `role` (reader, contributor or admin), `created`, and `createdBy`, null only for the first admin.
- Tokens belong to a principal. `New-PortalToken -Name <principal>` issues one for an existing principal; `-Bootstrap` makes the first admin, once.
- Old tokens without a principal work as admin until rotated, and a warning at boot names them.
- Roles are enforced: reader is GET only, contributor everything but identity and settings, admin all. Refusals are 403, shown as toasts. Attenuation is enforced server-side.
- Pages: Identity/Users (list, Add for admin), Identity/Agents (empty state), Settings/Tokens (by principal, hash prefix and created; Revoke for admin).
- Service connections record `createdBy`.

One point clashed with entry 2, and Jerry chose:

- **The binary writes the tenant's two identity files.** Add and Revoke must persist, or a revoked token comes back after a restart, but entry 2 limited the binary's writes to `-artifacts`. Asked, Jerry chose `tenants/<tenant>/principals.yaml` and `tokens.yaml` (both gitignored) as the only files the binary writes outside `-artifacts`. The options passed over were identity under `-artifacts` (split across two directories, and lost with `artifacts/`) and a read-only binary whose buttons print commands. Entry 2 and CLAUDE.md now say so. Neither file is a data layer, so "no write to any data layer" holds as written.

Choices made without asking:

- **Ids and names.** A principal is `ui:principal:<name>`, the name `^[a-z0-9][a-z0-9._-]{0,63}$`: a tenant's or connection's pattern, capped at 64. So a name is an id segment and a valid `New-PortalToken -Name`. That parameter used to take spaces and capitals; it names a principal now, so it takes this pattern.
- **Principal is a node, served as an envelope.** `GET /api/tenants/{tenant}/principals` is an envelope of `Principal` nodes (module `Portal`) in file order, checked by the portal ontology like the menu (entry 36), and never on the screen graph. The portal refuses null properties (entry 8), so the first admin's node leaves `createdBy` out, while the file says `createdBy: null`. `createdBy` is a property, not an edge: one reference does not earn a new edge Kind.
- **Files.** `principals.yaml` holds `{id, name, type, role, created, createdBy}` per principal. A `tokens.yaml` entry is `{principal, sha256, created}`, or the old `{name, sha256}`, a legacy token; an entry with both is refused. At boot both files are checked together, and any failure stops the binary:
  - the id is `ui:principal:<name>`, names are unique, type and role are known, and `created` is RFC 3339;
  - `createdBy` is null only on the first entry and only for an admin, so bootstrap happens once in the file as in the module;
  - every other `createdBy` names an earlier principal of a role at least as high, so attenuation holds in the file too;
  - every token's `principal` is in `principals.yaml`, and no two tokens share their first 12 hex digits.
- **The binary's writes.** Add and Revoke take the tenant's lock and re-read both files. Each then changes one file, writes it whole (a temp file and a rename, keeping the file's leading comment) and takes what it read as the new state. So a token `New-PortalToken` mints after boot goes live at the next Add or Revoke, not only at a restart. The tests rely on this and the docs say it. Both writers use one fixed block shape with every string single-quoted, so the module reads what the binary writes with a small private reader of that shape (`Read-PortalYamlList`) and needs no YAML module. An emptied token list is written `tokens: []`, which the module turns back into a block list.
- **Roles.** `reader` reads. `contributor` writes the screen, layers and snaps. `admin` also writes identity: `POST .../principals` and `DELETE .../tokens/{id}`. Nothing writes settings yet (connections and config are files), so "settings" guards no route today. The middleware refuses a non-GET from anonymous (401, as in 0.4.1) and from a reader (403). The identity handlers check attenuation, then admin. A 403 body is `{code: forbidden, message}`, and the spec documents 403 on every write.
- **Attenuation first.** A contributor asking for an admin is refused for attenuation, not for being a contributor. Today only admins create principals, and an admin may grant any role, so with the admin check first attenuation would never show through the API. Checking it first lets it surface, and the Go test sees it. `tenant.AddPrincipal` enforces both checks itself, so a future caller cannot skip them.
- **Legacy tokens.** An entry with `name` and no `principal` authenticates as an admin named after the token: whoami says `legacy: true` and has no `id`. The binary prints `portal: warning: tokens.yaml entries without a principal act as admin until rotated: <names>` to stderr. A legacy token cannot create principals, since it has no id to record as `createdBy`; that is a 403 pointing at `-Bootstrap`. It may revoke tokens, its own entry included.
- **One principal, many tokens.** `New-PortalToken -Name` no longer refuses a second token for the same name: rotating means minting a new token, then revoking the old one. The API names a token by the first 12 hex digits of its hash (`TokenSummary.id`), never by the token or the whole hash.
- **-Bootstrap.** It refuses when `principals.yaml` holds any principal, whatever the name, and then writes nothing. Without `-Bootstrap`, on a tenant with no principals, the error says to bootstrap. Legacy tokens are not principals, so an old tenant can bootstrap and keep its old tokens working until it revokes them.
- **Reads stay open.** Anonymous still reads everything (0.4.1), including the principal and token lists: names, roles, hash prefixes and dates, never a secret. `whoami` gains `id`, `type`, `role`, `legacy` and `administers`, and `writes` now means contributor or admin. The repos call (entry 36) still needs only a principal, so a reader may make it: it is a GET.
- **The page.**
  - The menu's Users, Agents and Tokens pages were table views of Kinds nothing emits (`identity.user`, `identity.agent`, `portal.token`). They now have views of their own: `users` and `agents`, both drawn by `web/principals.js`, and `tokens`, drawn by `web/tokens.js`, as Service connections has `connections`. These are page renderers like `nodes.js`, not panel types, so entry 1's "new panel types are Category nodes" is untouched.
  - Add sends `{name, role, type: user}`. The role list disables roles above the viewer's own. The button is disabled unless whoami says `administers`, with the tooltip `admin only: <name> is <role>`, or `sign in to write` for anonymous. Write controls on screen pages take their tooltip from the same `refusal`, so a reader sees `<name> is reader, which reads only`.
  - Revoke asks once more by turning into `Revoke <id>?`; there is no browser dialog. If the viewer revokes their own token, the next read is 401, which drops the token as in 0.4.0.
  - A refused Add or Revoke is a toast (entry 37). Marker holders is unchanged.
- **Agents.** `type: agent` is in the schema, the file check and the ontology, but `POST .../principals` refuses it (422, "agents are minted via Marker, later"). The Agents page is its empty state, saying so.
- **Service connections.** A connection's `created_by` (snake case, like `token_env`) is optional, so files from before 0.5.0 still load. When present it must name a principal. It is served as `createdBy` (null when absent) and drawn in a "created by" column. Nothing creates connections through the API yet, so whoever edits the file sets it. The committed examples now form a consistent set: `principals.example.yaml` (admin `example`), a token of it, and a connection it made. Go loads all three together, and Screenshot uses them.
- **Pester drives the page.** `--dump-dom` cannot click, so Pester drives the Users Add flow over Chrome's DevTools protocol. `Invoke-PageScript` in Portal.Tests.ps1 uses port 18203, puts the token in the tab's sessionStorage, then runs a script that fills the form and submits it. Like the DOM tests, it skips without Chrome.
- **Coverage.**
  - Go, `internal/server`: `TestRoleMatrix` (anonymous, reader, contributor and admin against read, write, identity and revoke), `TestAttenuationRefusal`, `TestLegacyTokenIsAdminUntilRotated`, and `TestAddAndRevokePersist` (the files, a reload, and the envelope against Graph.Node and the portal ontology). The spec test exercises the four new operations.
  - Go, `internal/tenant`: `TestBootstrapOnceOnly`, `TestAttenuation`, `TestTokensBelongToPrincipals` and `TestIdentityExamplesLoad`.
  - Pester: `-Bootstrap` once and `-Name` only for an existing principal; a portal reading what the module wrote; the boot warning; the anonymous Users, Agents and Tokens pages; the Users Add flow in Chrome; and a token minted for the added user going live at the next revoke and being refused a write (403).
  - Smoke bootstraps its admin and checks the role. Screenshot adds `identity-users.png`.
- **Version 0.5.0** in `Portal.psd1`, `ontology.yaml` (Kind `Principal`), `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated.

## 39. Dev mode: an auto-minted admin, a browser sign-in shortcut, a visible warning

0.5.1's brief: nobody pastes tokens during development, and nobody demos or ships a dev-mode portal without being told.
- A tenant's `portal.yaml` gains `mode: dev | demo | prod`, prod when absent, served on `GET /api/tenants/{tenant}` and shown in the banner.
- `Invoke-Build Dev` refuses unless the mode is dev, mints an admin `dev-bootstrap` into the gitignored identity files (replacing its previous token), sets `PORTAL_TOKEN`, runs Up and prints the token once, DEV ONLY. Up never mints.
- The avatar's box signs in and out with a token kept in sessionStorage. In mode dev it also offers "Use dev token", read from `GET .../dev-token`, which exists only in mode dev and only on 127.0.0.1.
- Mode dev shows a persistent amber bar; mode demo a thin blue `DEMO` bar and no shortcuts.
- CLAUDE.md stops an agent from screenshot or demo work while any tenant is in mode dev. The README's "Before you demo" lists the steps out of it, and Pester fails if `tenants/default/portal.yaml` is committed with mode dev.
- `tenants/default` is set to dev for now.

Choices made without asking:

- **Where the served token comes from.** `tokens.yaml` holds hashes only, so the binary cannot recover a token. It serves the token it was handed: `cmd/portal` reads `PORTAL_TOKEN` at boot, which `Invoke-Build Dev` sets for its process and `Start-Portal` passes on. It keeps it only when the tenant is in mode dev and the token authenticates as `dev-bootstrap`. Each request checks again, so a revoked dev token is never handed out. No new variable: `PORTAL_TOKEN` is already the module's credential, and a non-dev token in it is never served.
- **What 404 means.** `GET .../dev-token` answers 200 only when all of these hold at request time; anything else is 404 with one message, so a caller cannot tell which condition failed:
  - the mode is dev;
  - the binary holds a live dev-bootstrap token;
  - the caller's address is loopback;
  - the `Host` it named is loopback (127.0.0.1, ::1 or localhost).
  The listener is already 127.0.0.1 only (`host` in `cmd/portal`). The address check is for a test or a future listener; the Host check stops a page on another origin that rebinds its name to 127.0.0.1. The route needs no token: it is how an anonymous tab signs in. The answer carries `Cache-Control: no-store`.
- **dev-bootstrap outside mode dev.** Its tokens are refused, 401 naming the mode, on reads too. The boot warns while `tokens.yaml` still holds any (`... is in mode demo and tokens.yaml still holds 1 token(s) of dev-bootstrap ...`). So a tenant switched to demo cannot be driven by the dev admin even if the revoke was forgotten. The page drops a refused token at boot and reads as anonymous, so a tab left signed in from dev mode does not stop at an error card.
- **dev-bootstrap in the identity files.** Entry 38 allows `createdBy: null` only on the first principal. dev-bootstrap is minted into tenants that already have their first admin (`tenants/default` has one), and a real admin is bootstrapped into tenants that only had dev-bootstrap. So:
  - `dev-bootstrap`, a user admin with `createdBy: null`, may sit anywhere in `principals.yaml`.
  - The once-only rule counts every other principal: null `createdBy` only on the first one that is not dev-bootstrap. `New-PortalToken -Bootstrap` ignores dev-bootstrap the same way, and appends the real admin beside it rather than rewriting the file.
  - The name is reserved: `POST .../principals` refuses it (422), and so does `New-PortalToken -Name dev-bootstrap`. A dev-bootstrap made any other way would carry a creator and stop the next boot.
  - Principals made by dev-bootstrap keep `createdBy: 'ui:principal:dev-bootstrap'`, and the principal itself stays after its token is revoked. Removing principals is not in the API.
- **Mode is read twice.** The binary reads the merged config with its YAML decoder; the schema refuses an unknown value at boot, and the merge writes `mode: prod` when the overlay names none, so `GET .../config` says it too. The module has no YAML decoder (entry 38): a private `Get-PortalTenantMode` reads a top-level block line `mode: <value>`. A mode written in flow style (`{ mode: dev }`) reads as prod there, so `Dev` refuses rather than mints. Write the mode as a block line.
- **`Dev` reuses `Up`.** It is `task Dev DevGate, DevMint, Up, {...}`. DevGate refuses before Build, so a refusal builds and writes nothing. DevMint sets `PORTAL_TOKEN` in the Invoke-Build process, which Up's portal inherits. The minting is a private `New-PortalDevToken`, which refuses outside mode dev itself, not an exported function: it exists only for the build task. It writes `tokens.yaml` whole, in the binary's block shape, keeping the file's leading comment and every other token, legacy ones included.
- **Sign-in was mostly there.** The avatar box (paste, Sign in, Sign out, sessionStorage, the bearer header) shipped in 0.4.0 (entry 36). 0.5.1 adds:
  - the role beside the name in the avatar (`jerry · admin`);
  - "Use dev token" in mode dev, offered to anonymous and to any principal but dev-bootstrap;
  - the mode badge and the bars.
  The bars are an element of `index.html` in a grid row of their own under the banner, with no close control. They are page chrome, not panels: entry 1's "new panel types are Category nodes" is untouched.
- **Before you demo, in a working order.** The brief listed: switch to demo, revoke dev-bootstrap, mint a real admin, check the bar, check the avatar. With dev-bootstrap refused outside dev, the revoke needs a real admin, and in demo dev-bootstrap cannot sign in to do it. So the README orders it: mint the real admin, sign in as it and revoke dev-bootstrap, switch to demo and restart, check the bar, check the avatar. All five steps are kept.
- **The commit guard reads git.** The Pester test reads `tenants/default/portal.yaml` as committed (`git show HEAD:`) and as staged (`git show :`), and fails if either says `mode: dev`. Reading git rather than the working tree means a local, unstaged dev mode passes, and the test fails at the moment it matters: when dev is staged for a commit. With this task's change staged, it fails, as asked.
- **Screenshots refuse in dev.** `Invoke-Build Screenshot` copies `tenants/default/portal.yaml` into its temporary tenant, so it would capture the amber bar into `docs/screenshots/`, which are shown to others. It refuses while that file is mode dev, which is the CLAUDE.md rule enforced in the build. Smoke and Pester copy the same file and so run in mode dev now. They pass there: nothing they check depends on the mode, and the dev tests set their own.
- **PORTAL_MODE is separate.** `PORTAL_MODE=production` (entry 3) hides the Designer and is set by `Start-Portal -Production`. The tenant mode says what the tenant is run for. Demo does not hide the Designer; tying the two is not decided.
- **Coverage.**
  - Go, `internal/tenant`: modes and their default, dev-bootstrap's place in the file, its tokens only in mode dev, and the reserved name.
  - Go, `internal/server`: the token served in mode dev and not after a revoke; 404 for a LAN or public caller, a rebound or LAN Host, and for no, the admin's, a reader's or an unknown token passed; 404 and a 401 naming the mode in demo, prod and absent; a reader refused Add while the dev admin may.
  - Pester:
    - the commit guard;
    - the mode reader;
    - the mint (once, replacing only its own token, refusing outside dev);
    - `-Bootstrap` beside dev-bootstrap;
    - `Invoke-Build Dev` refusing and writing nothing, and Up not minting;
    - the boot lines;
    - in Chrome over DevTools: the amber bar, Use dev token to `dev-bootstrap · admin` and an Add, a reader signed in through the box unable to Add, the DEMO bar with no shortcut and a dropped dev token, and no bar in prod.

## 40. Feeds: the stock registry, NuGet feed and Postgres; push through the API; a ledger line per push

0.6.0's brief:
- Three stock services in Docker Compose: an OCI registry, a NuGet v3 feed and Postgres 16. Volumes sit under a tenant-scoped path, and no host port is published beyond the portal's own. Postgres is only provisioned, with a database and a health check.
- `feeds:` in `portal.base.yaml`, overridable per tenant.
- Every feed is a node, and Artifacts → Feeds lists the feeds from the graph.
- `POST /api/tenants/<t>/feeds/<feed>/push` takes a manifest card. The card is checked with Graph.Node 0.3.0's validator, the push is contributor or admin only, it goes to the underlying feed, writes one ledger entry, and a duplicate name and version is 409.
- A read-only Feeds page.
- `Invoke-Build Feeds`, which refuses in mode prod.

Three points clashed with this repo's rules or with the host, and Jerry chose:

- **The binary pushes to feeds and appends a ledger.** Entry 2 limited the binary's writes to `-artifacts` and the two identity files. A push sends a card to a feed over HTTP, and its ledger line needs a home. Asked, Jerry chose `tenants/<tenant>/ledger.jsonl` beside the identity files, gitignored, so it outlives clearing `artifacts/`. The options passed over were `artifacts/ledger/` (lost with `artifacts/`) and a Postgres table now (Postgres's use brought forward, and a driver added). Entry 2 and CLAUDE.md now name the ledger and the push. A feed is not a data layer, so "no write to any data layer" holds as written.
- **The two feeds are published on 127.0.0.1.** The portal binary runs on the host, so with nothing published it could not reach the registry or the NuGet feed. Asked, Jerry chose loopback-only ports: the registry on 127.0.0.1:8201 and the NuGet feed on 127.0.0.1:8202. Postgres is not published. Running the portal itself in Compose was the option passed over.
- **Docker was not running** when the work started, so `Invoke-Build Feeds` ran end to end only once Jerry had started Docker Desktop (the report has the result).

Choices made without asking:

- **The lift moves to Graph.Node v0.3.0** (`fa8fbf3`). The brief names Graph.Node 0.3.0 and says to reuse its validator, and CLAUDE.md recopies the lift when a tag is named (entry 4). `internal/graphnode/schemas`, `internal/graphnode/graph` (its non-test files), `cmd/graphnode/main.go` and `.claude/skills/graph-node` are `git show v0.3.0:<path>`, the Go files with the import path rewritten. There is still no registered edit. v0.3.0 adds:
  - `schemas/manifest.schema.json` and `index.schema.json`;
  - `graph.DecodeYAML`, `ValidateManifest`, `LoadManifest`, `BuildIndex`;
  - the CLI's `validate-manifest`, `validate-index` and `index`;
  - the skill at 0.3.0 with `reference/manifest.example.yaml`.

  The push calls `graph.DecodeYAML` and `graph.ValidateManifest` on the uploaded bytes. Nothing of the manifest rules is restated here.
- **compose.yaml is new.** Portal had no Compose file, so there were no existing services for the new ones to sit beside.
  - The project is `portal-<tenant>`.
  - Data is in `<tenants>/<tenant>/data/{registry,nuget,postgres}`, through `PORTAL_FEEDS_DATA`. `tenants/*/data/` is gitignored.
- **Images**, each pinned to an exact tag:
  - `registry:2.8.3`: the last 2.x of Distribution's reference registry, as the brief names `registry:2`. It is about 10 MB and stores OCI image manifests.
  - `bagetter/bagetter:1.6.5`: BaGetter, the maintained fork of BaGet (BaGet's last release was 2021). It is one container with SQLite and files on disk, about 90 MB, and serves the NuGet v3 protocol with the API-key push.
    - Passed over: NuGet.Server (v2, .NET Framework, Windows only); Sleet (a static feed with no push endpoint); ProGet, Nexus and Forgejo (several hundred MB, built for many formats).
  - `postgres:16.15-alpine`: the newest 16.x when this was written (2026-09-21), Alpine for size. It runs database `portal`, user `portal`, `pg_isready` as the health check, and `PGDATA` in a subdirectory of the volume. The binary does not connect to it yet.
- **Secrets in .env.** `PORTAL_NUGET_KEY` (BaGetter's `ApiKey`) and `PORTAL_POSTGRES_PASSWORD` come from the environment or the repo's `.env`. Compose reads `.env` for interpolation and refuses without them.
  - The first `Invoke-Build Feeds` writes both to `.env` as 32 random bytes in hex (private `Initialize-PortalFeedEnv`).
  - The portal reads `PORTAL_NUGET_KEY` at push time, through the feed's `token_env`, the same way connections read theirs (entry 36). It is never stored or served.
- **`feeds:` in the config.** Each feed is `{id, label, kind: oci|nuget, url, visibility: private|public, token_env}`.
  - The brief's "name" is `id`, because the merge keys lists on `id` (entry 36). A tenant overrides one feed key by key, or adds one.
  - After the merge each feed needs a kind and an http(s) url, and ids are unique. A failure stops the binary.
  - `url` is the registry plus the repository namespace for oci (`http://127.0.0.1:8201/cards`), and the service index for nuget.
  - `visibility` defaults to private. It is recorded on the node and not enforced: Portal's reads are all open (entry 38), and the services are loopback only.
- **Feeds are a graph.**
  - A Kind `Feed`, id `ui:feed:<id>`: `kind`, `url` and `visibility` required, `order` and `tokenEnv` optional.
  - A Kind `Card`, id `ui:card:<feed>/<name>@<version>`: `feed`, `version`, `type`, `hash`, `pushed` and `pushedBy` required, `digest` optional.
  - An edge `has-card`, Feed to Card.

  "Register the prefix the same way existing Kinds are" means `ontology.yaml`. Portal issues only `ui:`, and the id registry is in the read-only lift. The edge was first named `holds`, but an existing test uses `holds` as its example of an unknown edge Kind, and `has-card` matches `has-page`. `GET .../feeds` serves the envelope, built from the config and the ledger's pushed lines. It is checked at boot with Graph.Node and the portal ontology, like the menu, and never written or snapped.
- **Artifacts → Feeds was not on the rail.** Artifacts had Exports and Bundles only, so Feeds was added as its first page, with a new `view: feeds` drawn by `web/feeds.js`. That is a page renderer, like `users` and `tokens` (entry 38), not a panel type, so entry 1 is untouched. Each feed is a card (kind, visibility, URL) with a row per card: name, version, type, hash prefix (the whole hash and the digest on hover), pushed and by. There are no write controls; the dev bar and the avatar are unchanged.
- **The push.**
  - The body is `multipart/form-data`: one `manifest` part and one `file` part per other file, whose filename is its path in the card's folder with forward slashes. Go's `Part.FileName` keeps only the base name, so the path is read from `Content-Disposition` itself.
  - The checks, in order:
    - the middleware's 401 and 403;
    - the feed (404);
    - the body (400);
    - the card: Graph.Node's problems (code `schema`, paths `manifest.yaml:/...`), then each path (code `file`: clean, relative, no `..`, no case-folded duplicate, nothing the .nupkg format reserves at the root) (422);
    - a duplicate (409);
    - the feed (502, or 422 `no_token` when the key's variable is unset).
  - Pushes are serialized per tenant, and each one is bounded at 2 minutes.
  - `Publish-PortalCard -Feed -Path [-Url]` is the ninth exported function: it sends a folder (dot files skipped) and returns the ledger entry. It exists because every task's work lives in the module (entry 24).
- **Duplicates.** The ledger is checked first: a pushed line with that feed, name and version is 409, and the feed is not asked. Then the feed itself: OCI `HEAD /v2/<repo>/manifests/<tag>`, NuGet's flat container version list, and the feed's own 409 on the push. A feed holding a version the ledger does not know is 409 too. The same card in the other feed is not a duplicate.
- **OCI packaging.** A card is an OCI artifact in the shape ORAS writes, so `oras pull 127.0.0.1:8201/cards/tool.git:0.1.0` restores the folder:
  - an image manifest with `artifactType: application/vnd.graphnode.card.v1`;
  - the empty config (`application/vnd.oci.empty.v1+json`, `{}`);
  - one layer per file (`application/vnd.graphnode.card.file.v1`), titled by its path (`org.opencontainers.image.title`), `manifest.yaml` first;
  - annotations for the card's name, version, description, type, content hash, author (the pusher) and created time.

  The repository is the feed URL's path plus the card's name in lower case (`cards/tool.git`). The tag is the version, with SemVer's `+` written `_`. A name that is not a valid repository component is 422. The push is the distribution API's monolithic upload (HEAD, POST, PUT per missing blob, then PUT the manifest), written on `net/http` with no new dependency. The digest is the manifest's.
- **NuGet packaging.** A card is a `.nupkg`, the only thing a NuGet v3 feed accepts. It holds:
  - `<name>.nuspec`: id the card's name, version its version, authors the pusher, description its description, tags `graphnode-card <type> <tags>`;
  - `[Content_Types].xml`;
  - every file of the card at its own path, `manifest.yaml` at the root.

  So extracting the package (nuget, `Save-PSResource`) gives back the card's folder. There is no `lib/`, so nothing mistakes a card for a library. The zip is deterministic (fixed times), so one card always packs to the same bytes. The push is NuGet's `PUT <PackagePublish/2.0.0>` multipart with `X-NuGet-ApiKey`. The digest is the .nupkg's SHA-256.
- **Content hash.** `sha256:` over one line per file, sorted by path: the path, a tab, the file's SHA-256, a newline. It names the card's content whatever the feed or packaging, so a card pushed to both feeds carries one hash. The feed's own name for what it stored is a separate `digest`.
- **The ledger.** `tenants/<tenant>/ledger.jsonl` holds one JSON line per push that names a feed the tenant has, whatever its result. Refusals before that (401, 403, a feed the tenant lacks, a malformed body) write none.
  - Each line has `{at, principal, name, role, feed, kind, card, version, type, hash, digest, files, result, message}`. `result` is `pushed`, `duplicate`, `invalid` or `failed`. `principal` is null for a legacy token. A field that does not apply is null, never absent.
  - Each line is appended in one write and synced.
  - At boot the file is read and checked: a line that is not an entry, or a pushed line without its card, stops the binary and names the line.
  - The lines are not nodes and carry no ids: `ledger:` belongs to Claude.Agent.Alerts. The Card nodes on `GET .../feeds` are derived from the pushed lines. Ledger → Entries still lists Kind `ledger.entry`, which nothing emits. (Since 0.10.0 the lines are chained and served as LedgerEntry nodes `ui:ledger:portal/<seq>`, and Ledger → Entries lists the chains: entry 45.)
- **`Invoke-Build Feeds`.**
  - FeedsGate refuses in mode prod, before Build, as DevGate refuses outside dev. It also needs `docker` on PATH.
  - Feeds then sets the Compose environment and runs `docker compose -p portal-<tenant> up -d --wait`, which waits for the health checks of the registry (`wget /v2/`) and Postgres (`pg_isready`).
  - It waits for the NuGet service index, then runs Up, so the portal holds `PORTAL_NUGET_KEY`.
  - It pushes `tests/fixtures/cards/Tool.Git` (Portal.Tools' `tools/Git` at `c330712`, copied) to both feeds with `Publish-PortalCard`, and checks `GET .../feeds`, the registry's tag list and the NuGet version list.
  - A 409 on a rerun is reported, not failed.
  - The token is `PORTAL_TOKEN`. When that is unset, a tenant in mode dev gets a fresh dev-bootstrap token, as DevMint mints it, and a tenant in mode demo must set it.
  - Up still never mints. Since 0.6.0's follow-up, Up (`Invoke-PortalUp`) also takes `PORTAL_NUGET_KEY` from `.env` when the environment lacks it, through the same private reader Feeds uses (`Get-PortalEnvValue`). It only reads; it never writes `.env`. So a portal started by Up or by Feeds can push to cards-nuget.
- **Coverage.**
  - Go, `internal/feed`: the card checked with Graph.Node, the paths, the content hash, the OCI ref and manifest, the .nupkg, and pushes to a fake registry and a fake NuGet feed (`internal/feed/feedtest`), each twice for `ErrExists`.
  - Go, `internal/tenant`: the feeds merge, bad feeds stopping the boot, one ledger line per push with every key, a reload, and a bad ledger stopping the boot.
  - Go, `internal/server`: both operations against the spec, the multipart request included; contributor, admin and legacy pushes, with 401 and 403; 409; the feeds envelope against Graph.Node and the portal ontology; the ledger file's shape.
  - Pester: the feeds envelope through `graphnode validate`; `Publish-PortalCard`'s 401, 403, 404, 422, 409 and 502, with their ledger lines and `-WhatIf`; the page in Chrome; Compose's pins, ports and volumes; the gitignore; Feeds refusing in mode prod and writing nothing; the `.env` secrets; Up taking `PORTAL_NUGET_KEY` from `.env` (a NuGet push goes from 422 `no_token` to the feed, and `.env` is unchanged).
- **Version 0.6.0** in `Portal.psd1`, `ontology.yaml` (Kinds `Feed` and `Card`, edge `has-card`), `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated.

## 41. Tidy after 0.6.0: Use dev token absent in prod, CLAUDE.md's stale claims, a revoke line in Before you demo

No version change; nothing in the binary or the page changed.

- **Use dev token was already there.** 0.5.1 shipped it (entry 39): `devSignIn` in `web/portal.js` draws it only when the tenant's mode is dev, `client.getDevToken` reads `GET .../dev-token`, and `setToken` puts the token in `sessionStorage` (`portal.token`), the bearer every write already sends. Pester already covered dev (the click, the stored 64-character token, the avatar `dev-bootstrap · admin`, then an Add whose row says `createdBy` dev-bootstrap, which proves the next POST carried it) and demo (not offered). Prod was checked only by a DOM dump that never opened the avatar's box, so the prod test now opens it and asserts no `button[data-dev-token]` and no `.pt-dev-signin`. It stays one test, so the Pester count does not move.
- **CLAUDE.md.** Its skills line said graph-node was checked against Graph's committed skill, which stopped at 0.3.6 (entry 34); it now says what Pester checks. Its lift line said the copy carries one registered edit; none has been registered since v0.2.3 (entries 4, 34). Its layout lacked `internal/tenant/`, `internal/feed/`, the Tool.Git fixture and the Feeds and Status tasks. Every other version and path in CLAUDE.md and README was checked against the tree and held.
- **Not changed: CLAUDE.md's read-only siblings.** The rule names `../Graph.Node`, `../GraphRenderer`, `../Graph` and `../Graph.Emitter.FileSystem`, but 0.6.0 copied its card fixture from `../Portal.Tools` and README's example reads it. Widening a rule is the owner's call, so the line stays as written and the report names it.
- **The revoke line** goes after the numbered steps of Before you demo, not into them, so their order and numbering stay as they were.

## 42. Layers get an address and a page (0.7.0)

- **Location.** `POST /api/tenants/<tenant>/layers` answers 201 with `Location: /<tenant>/layers/<name>`. That is the layer's page, not an API resource: the spec has no `GET .../layers/{name}` operation, and the prompt asked that Location point at the page. The header is declared `required` on the 201, so the spec test checks it on every mount.
- **The page route is its own operation, `getLayerPage`, at `/{tenant}/layers/{name}`.** The literal `layers` segment wins over `getShellPage`'s `{service}` in chi. So no menu service may have the id `layers`, and the merged-config check refuses one (`/menu/<i>/id: layers is the route of the layer pages`). It serves `index.html` only for a mounted layer and is 404 otherwise, as the shell routes are 404 for a page the menu lacks.
- **"The layers list" did not exist, so it is a new menu Page: Graph/Layers (`view: layers`, `web/layers.js`).** It lists every mounted layer (name, module, node and edge counts, stamp, path), each name linking to the layer's page. Like Artifacts/Feeds it is a view, not a new UI model: a Page node in the menu envelope names it. Its rows come from `GET .../layers` (LayerSummary), as Settings/Tokens' rows come from a summary list, not from an envelope.
- **The layer page is drawn under Graph/Layers.** The shell maps `/<tenant>/layers/<name>` onto the Page whose view is `layers`, so the rail marks Graph/Layers and the crumbs read tenant › Graph › Layers › name. The title is the layer's name. The toolbar has a badge `<n> nodes · <m> edges · <stamp>` (module and path in its tooltip) and the count of rows shown. The nodes come from `GET .../layers/{name}/nodes` and are drawn by `nodes.table`, the table the Graph pages (Kinds, Taxonomy) draw: id, kind, label, updated. A node with no stamp shows the layer's stamp. The banner's search filters the rows. A row's id is not a link: a layer's node has no item page of its own yet.
- **`POST /api/layers` is gone: 410, code `gone`, message `POST /api/layers is gone; mount with POST /api/tenants/<tenant>/layers`.** It mounts nothing. The bearer-token middleware still runs first, so a write without a token is 401 before the 410. Answering 410 to everyone would have meant exempting one path from the middleware by hand. The spec keeps the operation (`deprecated`, now 410 or 401) so route parity still holds. Removing a route breaks callers, so the version is 0.7.0. Claude.Agent's Up must POST to the tenant route; that repo is not touched here.
- **Pester's port race.** Block "Up replaces a running portal" now waits for 18200 in its `AfterEach`, as the other blocks' `AfterAll`s do (the one-line fix the 0.6.0 report named). The `Boot order` block's `AfterEach` has the same shape and was the other race in that report; the prompt named one test, so it is left as is.
- **CLAUDE.md.** The read-only siblings now include the Portal.* seeds `../Portal.Tools`, `../Portal.Skills`, `../Portal.Workflows`, `../Portal.Emitters` and `../Portal.Ontology` (entry 41 left this to the owner; the owner has now asked). `../Portal.Agent` exists beside them but was not named, so it is not on the list.

## 43. Agents and their runs (0.8.0)

The brief asked for Kinds Agent, Run and Decision with ids `agent:<name>` and `agent:run/<stamp>`, "matching what Graph.Emitter.Agent already emits". Two points clashed with Graph.Node, and Jerry chose:

- **`agent:` belongs to Graph.Emitter.Agent, and Portal never mints an id under it.** Graph.Node's registry (`schemas/ids.md`, lifted) gives `agent:` to Graph.Emitter.Agent, and its skill says never to issue ids under another module's prefix. `agent:<name>` is already the emitter's AgentFile shape (`agent:CLAUDE.md`), so an agent named like a file would collide with one. Portal's own ids stay `ui:`.
- **There is no Agent Kind. An agent is a principal**, the Kind 0.5.0 has (`ui:principal:<name>`, entry 38). Runs are grouped by the principal whose token mounted the layer. A layer mounted with no principal is dev-bootstrap's (entry 39): a token from before 0.5.0, a `-mount` or `-filesystem` flag at start, or a principal principals.yaml no longer holds. Every request that writes carries a token (entry 38), so no anonymous mount exists. Run and Decision keep the emitter's `agent:run/...` ids. The first question offered `ui:agent:<name>`, and Jerry answered this instead.

Choices made without asking:

- **Which layers.** "Whose ontology is Graph.Emitter.Agent" is read as the envelope's `module`, which equals its ontology's `module`. A layer of that module with no Run (the AgentFile layer) gives no runs.
- **Run.** A Kind, re-serving a Run node of such a layer under its own id. It carries `layer`, the emitter's `startedAt`, `stopped`, `finishedAt` and `exit`, and four counts. `steps` is the Steps it `contains`; `allow`, `deny` and `witness` are its PolicyDecisions by `effect`, found along the emitter's edges (Run `contains` Step, Step `contains` Action, Action `decided` PolicyDecision), pre and post both. The brief's "stamp" is `startedAt`.
- **Decision.** A Kind, re-serving a PolicyDecision under its own id, with `layer`, `effect`, `phase`, `rule`, `step` (the Step's ordinal) and `reason`. The brief's "Decision" is the emitter's PolicyDecision; "Run" keeps its name.
- **Edges.** `has-run` (Principal to Run) and `has-decision` (Run to Decision), named like `has-page` and `has-card` (entry 40).
- **Borrowed ids.** The portal's node check requires `ui:` (code `id-prefix`). Run and Decision are excepted only in envelopes the API serves (`CheckEnvelope`). There the id must be a valid `agent:` id (`graph.ValidID`). On the screen they are refused like any other non-`ui:` id, so no run reaches a snap. Graph.Node itself checks no owner, so the served envelopes validate as they are.
- **A Run id in two layers is served once,** from the layer with the later stamp, the later mount on a tie. Two copies of one run (a directory remounted, a layer mounted twice) cannot make two nodes with one id, and the newer emit is the more complete one.
- **dev-bootstrap when principals.yaml lacks it.** It is served as `type: user`, `role: admin` (what Invoke-Build Dev makes, entry 39), with no `created`. So `created` on Principal is no longer required. It is absent nowhere else: listPrincipals always reads the file.
- **Which principals are agents.** Every principal of type `agent` (none exist yet; they come with Marker, entry 38) and every principal holding a run, in principals.yaml order, then a dev-bootstrap made up as above. A principal who mounted no runs is not listed.
- **Who mounted a layer.** `store.Mount` gains `By`, which the JSON body cannot set (`json:"-"`). The handler fills it from the request's principal. The Layer keeps it as `MountedBy`, served on LayerSummary as `mountedBy`. A path mount records it on its Mount node (`mountedBy`, a new optional property), and Remount reads it back. Without that, an approved path mount's runs would move to dev-bootstrap at the next boot. A Mount node from before 0.8.0 has none, so its runs are dev-bootstrap's.
- **API.** `listAgents` is an envelope of the agents and their runs; `listAgentRuns` is one agent, its runs newest first and their decisions, rooted at the principal. Both are envelopes like `listFeeds` (entry 40), not summaries, because what they serve are nodes. A name that is no agent is 404.
- **Pages, and which menu entry: Identity → Agents.** Its `view: agents` was principals.js's list of type-agent principals, always empty (entry 38). It is now `web/agents.js`, the list of agents: name, type, role, runs, denied (red when not zero), last run. The rail links the entry to `/<tenant>/agents`, the brief's route. The shell maps that route, and `/<tenant>/agents/<name>`, onto the Page, as `/<tenant>/layers/<name>` maps onto Graph/Layers (entry 42), so the rail and the crumbs place them. `/<tenant>/identity/agents` still draws the same list.
  - The page routes are their own operations, `getAgentsPage` and `getAgentPage`. The literal `agents` wins over `{service}`, so no menu service may have the id `agents`, and the merged-config check refuses one.
  - An agent's page shows a badge `<n> runs · <d> denied` and one row per run, newest first: started, finished (`not stopped` without one), steps, allow, deny (red when not zero), witness, exit, and the layer, linking to its page. Under each run is a line of links, one per decision, `step <n> <phase> <effect>` (a deny in red).
- **"Each decision to its node."** A layer's node has no item page (entry 42), so a decision links to the layer page with its id after `#`. The layer page marks that row (`pt-target`, `aria-current`) and scrolls to it. The fragment never reaches the server.
- **Not added: a PowerShell wrapper.** The brief asked for routes, pages and tests, and the module's `Mount-PortalLayer` already returns `mountedBy`. A `Get-PortalAgent` is left for when a task needs one.
- **Coverage.**
  - Go: the four operations against the spec; grouping by the mounting principal; a flag mount and a legacy mount under dev-bootstrap, made up when absent; counts against the fixture (3 steps, 4 allow, 1 deny); Decision ids equal to the layer's PolicyDecision ids; newest first; a shared Run id served once; `mountedBy` on the Mount node across approval and restart; both envelopes through Graph.Node and the portal ontology; the screen refusing a Run with an `agent:` id; the `agents` service id refused.
  - Pester: both pages in Chrome; the counts, the red deny, the layer link, every decision link, and the marked row on the layer page.
- **Fixture.** `tests/fixtures/layers/agent-runs/` is Portal.Emitters' `emitters/Agent/artifacts/layers/2026-10-09T02-50-56Z.runs.graph.json` and the `ontology.yaml` beside it (Graph.Emitter.Agent 0.1.2), at cd862d9. It is the run of Graph.Emitter.Agent's policy-smoke ledger. Graph's `artifacts/layers/Graph.Emitter.Agent` holds another emit of the same ledger, same stamp and version, different bytes. CLAUDE.md does not list `../Graph.Emitter.Agent` among the read-only siblings, so it was not read.
- **Version 0.8.0:** new operations and Kinds, and Identity/Agents changed what it draws.

## 44. Emitters and the layers they produced (0.9.0)

The brief: an emitter is the card Portal.Emitters pushes to a feed, under the card's own id, with no new id prefix. A mounted layer gets an edge `produced-by` to the emitter its ontology names, and the reverse `has-layer`. Two routes, two pages on the existing Emitters menu entry, and a fixture.

Asked, and Jerry chose:

- **The `emitters` service keeps its id; boot refuses none.** The brief asked for a boot refusal of a service named emitters, as for agents and layers. portal.base.yaml already has that service (Catalogue, Sources, Runs, Schemas), so the product would not boot. Jerry's answer: keep the id and drop the refusal. Catalogue becomes the list (`view: emitters`) at `/<tenant>/emitters`, as Identity/Agents is at `/<tenant>/agents`. One emitter's page is `/<tenant>/emitters/<name>`. Sources, Runs and Schemas keep their paths, because the boot check below reserves their names.
  - `getEmitterPage` serves the shell for a page of the emitters service as well. Its literal route wins over `getShellPage`'s in chi. The page's router does the same: a page the menu has under `emitters/` is that page, any other name is an emitter. `/<tenant>/emitters/catalogue` still draws the list.
  - **The boot check.** A pushed ledger line whose card has type `emitter` and is named like a page of that service, by its name or its emitter name, stops the binary. The check names the ledger entry and the page. In practice only a hand-written ledger can hold one: Graph.Node's manifest schema requires a dotted card name (`^[A-Za-z][A-Za-z0-9-]*([.][A-Za-z][A-Za-z0-9-]*)+$`), and a menu id has no dot (`^[a-z0-9][a-z0-9-]*$`). So a push cannot make one. A push-time refusal was written first; it was dead code and was removed.

Choices made without asking:

- **Which name an emitter has.** It is the module its layers name: the `module` of the layer's ontology (the file the envelope's `ontology` field names, read at mount), or the envelope's own `module` when the portal could not read one (an inline mount of a relative ontology). Portal.Emitters names the card of the module `Graph.Emitter.<X>` as `Emitter.<X>` (all seven at cd862d9), so the card `Emitter.<X>` is the emitter `Graph.Emitter.<X>`. A card named any other way is the emitter of its own name. The brief's names, Graph.Emitter.Git.Repo and so on, are read as those modules.
- **No new Kind for a known emitter: it is the Card**, served under the card's own id `ui:card:<feed>/<name>@<version>` with every property listFeeds gives it. It also carries two new optional Card properties: `emitter` (its name) and `layers` (how many mounted layers it produced). When several cards name one emitter (versions, feeds), the newest push wins, by `pushed`.
- **One new Kind, `Emitter`, for an unknown emitter only.** A layer can name an emitter that no feed holds a card of, and the brief has it shown by name, flagged unknown. A Card is a push to a feed, with a required `feed`, `version`, `hash` and `pushed`, and such an emitter has none of them. So it is Kind Emitter, `ui:emitter:<name>`, with `unknown` (always true) and `layers`. The id is under Portal's own `ui:`, so no new prefix.
- **One new Kind, `Layer`**, for the edges' other end: `ui:layer:<name>` with `emitter`, `module`, `version`, `stamp`, `nodes`, `edges`, `mountedBy` and `path`. The Mount Kind was not reused. It is a screen record of a path mount, with a required `path` and a remount at boot, and an inline layer has neither.
- **Edges.** `produced-by` (Layer to Card or Emitter) and `has-layer` (Card or Emitter to Layer). Both are served, as the brief named both.
- **API.**
  - `listEmitters` is an envelope of every emitter by name, each followed by its layers newest first, with both edges.
  - `getEmitter` is one emitter, rooted at its node.
  - Both are envelopes, like listAgents (entry 43). A name that is no emitter is 404, including a card's own name (`Emitter.Agent`): the emitter is `Graph.Emitter.Agent`.
  - An emitter stops being one when its last layer goes and no card names it.
- **"Card link".** The node id is the card's id; no URL is served. The page links a card to its row on the feeds page (`/<tenant>/artifacts/feeds#<card id>`). feeds.js marks that row and scrolls to it, as the layer page does for a decision (entry 43).
- **"Principal".** A layer's `mountedBy` (entry 43). The emitter page shows the principal's name. A layer mounted with no principal shows nothing; listAgents's dev-bootstrap fallback is not repeated here.
- **Pages.**
  - The list has one row per emitter: name (linking its page), version, feed, card (linking its row on the feeds page) and layers. The toolbar count reads `<n> emitters · <u> unknown`.
  - An unknown emitter's row is amber (the dev bar's hue) with an `unknown` badge and empty version, feed and card.
  - An emitter's page shows a badge `<n> layers`, the card's version and feed with its link (or the unknown badge), and its layers newest first: name (linking the layer page), nodes, edges, stamp and principal.
  - The layer page gains a line above its nodes, `produced by <emitter> <version>`, linking the emitter's page, with the unknown badge in place of the version.
- **A layer whose module is named like a page** (a single-word module such as `runs`, possible only for a layer Portal.Emitters did not emit) has its emitter page shadowed by that page. It is listed, and its link draws the page. Left as is; noted in ONTOLOGY.md's Not here yet.
- **Fixture.** `tests/fixtures/cards/Emitter.Agent/manifest.yaml` is Portal.Emitters' `emitters/Agent/manifest.yaml` at cd862d9, byte for byte (Emitter.Agent 0.1.3, type emitter). It is the card of the agent-runs fixture's emitter (entry 43). The layer itself was emitted by Graph.Emitter.Agent 0.1.2, so the card's version and the layer's differ, and both are shown.
  - The "seed feed card" is the card as a feed holds it, which for Portal is a pushed ledger line. Go pushes the fixture through the fake registry (a real push, Graph.Node's manifest check included). Pester writes the ledger line before boot, as its Feeds block does.
- **Coverage.**
  - Go: the four operations against the spec; produced-by resolving to the pushed card's id with emitter, feed, version and layer count; the unknown FileSystem emitter; the one-emitter envelope rooted at the card, with the layer's counts, stamp and principal; both envelopes through Graph.Node and the portal ontology; 404 for an unmounted name, for the card's own name, and for an emitter whose last layer was replaced; the Sources page keeping its path, and the shell routes; the boot check.
  - Pester: both pages in Chrome; the counts, the amber unknown row, the card link landing on its marked row, Sources keeping its path, the layer rows, and the produced-by line on both layer pages.
- **Not added: a PowerShell wrapper** (`Get-PortalEmitter`). The brief asked for routes, pages and tests.
- **Version 0.9.0:** new operations and Kinds, and Emitters/Catalogue changed what it draws.

## 45. Ledger chains: verified, paged, joined to runs (0.10.0)

The brief: Portal.Agent adds a layer named `ledger`, one LedgerEntry node per ledger entry under `ledger:<seq>` (the registered shape, the same ids as the Alerts layer's), with `seq`, `stamp`, `prev`, `hash` and the run id, edges `prev` and `records`, and the chain's verify on the layer's root node. No new id prefix. Portal's own push ledger is a second chain, served the same way under source `portal`. Three routes, three pages on the existing Ledger menu entry, a ledger count per run on the agent page, a fixture with a good and a broken chain.

Asked, and Jerry chose:

- **Portal's push ledger becomes a chain from now on.** Its lines (entry 40) had no `seq`, `prev` or hash, so there was no chain to verify. Jerry's answer: every new line carries `seq` (its line number from 1) and `prev` (the SHA-256 of the line before as written, without its newline; 64 zeros for the first). This is the agent chain's own rule (Claude.Agent.Ledger's stream contract, as Portal.Emitters' `Read-AgentLedger.ps1` reads it). Lines from before 0.10.0 are an unchained head: counted (`unchained`), never a break. A line without `seq` after the chain began is a break (`unchained`). The options passed over were a chain derived on read (always "valid", so verify would mean nothing) and leaving `portal` out.
- **Portal's entries are `ui:ledger:portal/<seq>`.** `ledger:` is registered to Claude.Agent.Alerts, so Portal mints no id under it, and `ledger:<seq>` would collide with the agent's entries. The source in the id leaves room for another Portal chain. The chain nodes are `ui:ledger:agent` and `ui:ledger:portal`.

Choices made without asking:

- **The real layer was not read.** CLAUDE.md does not list `../Portal.Agent` among the siblings Portal may read. The fixture is built to the brief's shape: twelve entries, seq 44 to 55, of synthetic stream-contract lines whose `prev` and `hash` are real SHA-256s, so the chain is consistent.
  - Nine entries (46 to 54) belong to the agent-runs fixture's run. `records` points at that Run, or at its Step for a `policy.pre` or `policy.post` line, by the runs layer's ids.
  - The fixture's module is `Portal.Agent` 0.1.0, with an ontology beside it declaring LedgerEntry and the two edges.
  - When the real layer lands, the fixture should be replaced by a copy, as entries 43 and 44 did.
- **The layer's root node is the newest entry.** A root needs an id, and `ledger:` allows only `ledger:<seq>` and `ledger:line-<n>`; any other node would need a prefix Portal.Agent does not own. So the fixture's envelope `root` is `ledger:55`, which also carries `valid`, `count`, `first`, `last` and, when broken, `break`. Portal reads those from whatever node `root` names, so a separate root node works too.
- **Which layer is the agent's ledger.** Any mounted layer holding a LedgerEntry node under a `ledger:` id, whatever its name or module; the latest stamp wins when several do (each emit is the whole ledger again). An entry with no integer `seq` (`ledger:line-<n>`) is not a link of the chain and is left out.
- **Portal verifies, and the layer may overrule.**
  - An entry follows the one before when its seq is one more and its `prev` is that entry's `hash`. The first entry, when its seq is 1, has `prev` of 64 zeros.
  - `breakAt` is the seq of the first entry that does not follow. `reason` is `prev-mismatch`, `seq-gap`, `unchained` or `layer`.
  - The layer's own verdict is served as `claimed`. A layer that says its chain is broken is broken, at the earlier of the two breaks, since it checked lines and keys Portal never sees.
  - A layer that says valid does not overrule Portal. The broken fixture's root still says valid, as a layer verified before an edit would.
- **What verify can and cannot see.**
  - For the agent chain Portal has the hashes, not the lines. A line edited and re-hashed breaks the link to the next entry. In the broken fixture, entry 50's `event` was changed and re-hashed, so 51's `prev` no longer matches and the chain breaks at 51. An edit to an entry's other fields that leaves `hash` alone is invisible to Portal.
  - For its own ledger Portal hashes the bytes, so any edit to a line breaks the link after it. An edit to the last line is invisible until the next push chains to it.
  - Neither chain has a MAC check: that needs a key Portal does not hold.
- **A broken chain does not stop the boot.** A line that is not a ledger entry still does (entry 40). A break is a finding to show, red on the page, and the next push chains to the line as it now is.
- **Seq is the line number.** A Portal line's `seq` is its position in the file, so its id is stable. A line whose written `seq` differs (an edit by hand) is served with `written` and breaks the chain there (`seq-gap`).
- **Kinds and edges.**
  - New Kind `Ledger`, a chain: `source`, `count`, `valid`, `first`, `last`, `breakAt`, `reason`, `stamp`, `layer`, `claimed` and `unchained`; on one chain's page also `entries`, `run` and `next`.
  - New Kind `LedgerEntry`: `source`, `seq`, `stamp`, `hash` and `prev`; the agent's `event`, `run` and `runLayer`; Portal's `by`, `feed`, `card`, `version`, `result`, `message`, `unchained` and `written`.
  - New edges `has-entry` (Ledger to LedgerEntry), `prev`, `records` (LedgerEntry to Run or Step) and `recorded-in` (the reverse). `recorded-in` is declared for the layer; Portal serves `records` only.
  - Run gains optional `ledger`, its count of entries.
  - The portal check takes `ledger:` ids for LedgerEntry in what the API serves, as it takes `agent:` for Run (entry 43). A Portal entry stays `ui:`.
- **`records` ends in the runs layer.** Served envelopes do not copy the Run or Step. The edge is cross-layer, a Graph.Node warning resolved at merge, as it is in the ledger layer itself. Each entry instead carries `runLayer`, the mounted layer holding its Run, so the page can link `/<tenant>/layers/<runLayer>#<id>` with the target's row marked (entry 43). The link goes to the Step when the entry records one.
- **`prev` edges only between served entries.** A page holds 100 entries by default. An edge to an entry off the page would dangle, which is an error for Portal's own `ui:` ids. `getLedgerEntry` serves the entry before too, so its `prev` edge always lands.
- **API.**
  - `listLedgers`: the chains, agent first while a ledger layer is mounted, then portal, always.
  - `getLedger`: one chain's entries newest first. `limit` is 1 to 1000, and absent or 0 means 100. `before` pages by seq (the root's `next` gives it), and `run` keeps one run's entries.
  - `getLedgerEntry`: one entry with its chain and the entry before.
  - 404 for an unknown source (agent too, while no ledger layer is mounted) or seq, and 400 for a bad `limit` or a seq that is no integer.
- **Pages, and the reservation.**
  - Ledger/Entries becomes the list (`view: ledger`) at `/<tenant>/ledger`, as Emitters/Catalogue did (entry 44). Verify and Keys keep their paths and Kinds.
  - `/<tenant>/ledger/<source>` lists a chain's entries, and `/<tenant>/ledger/<source>/<seq>` shows one.
  - The literal routes also serve the shell for a page of the ledger service, and `/<tenant>/ledger/<page>/<id>` for its items, as `getShellItem` would.
  - The sources are fixed names, so the reservation runs the other way from entry 44's: boot refuses a ledger page with the id `agent` or `portal`.
  - **The list** has one row per chain: source (linking its entries), verify, entries, first, last, stamp, and the layer or `ledger.jsonl · <n> before the chain`. Verify is `valid` in green, or `broken at <seq>` in red with the reason as its title. The toolbar reads `<n> chains · <b> broken`.
  - **A chain's entries** have a badge `<n> entries · <verify>`. Each row has seq (linking the entry), stamp, event and run for the agent or result, card and by for Portal, and the hash's first 12 digits. The entry the chain breaks at is marked red, and an `older entries` link pages.
  - **One entry** shows its fields, its `prev` linking the entry before, and the break when it is there.
  - The agent page's runs gain a `ledger` column while a ledger layer is mounted, linking `/<tenant>/ledger/agent?run=<id>`.
- **Proposal for Graph.Node, not made here.** Graph.Node registers `ledger:` to Claude.Agent.Alerts. If Portal.Agent's layer issues `ledger:<seq>` ids as the brief says, the registry needs to say that prefix is shared by the two (or move it), or `graphnode check-id` will name the wrong owner.
- **Coverage.**
  - Go:
    - the six operations against the spec, the shell routes included;
    - the good fixture: valid, 12 entries, 44 to 55;
    - the broken copy: broken at 51, `prev-mismatch`, `claimed` true; and a layer's own break overruling;
    - every `records` edge of both fixtures resolving to an id of the runs fixture;
    - paging, the run filter, the `prev` and `records` edges, and one entry;
    - Portal's chain: an unchained head, a hand edit breaking it at the next line, and the next push chaining on;
    - the run's `ledger` count; the boot refusal of a ledger page named `portal`; the ledger line's shape with `seq` and `prev`.
  - Pester, in Chrome: the list in green and red; the entries newest first with their run links; the run filter; the layer page marking the Step's row; one entry; and the agent page's count. Also the ledger line's `seq` and `prev` on a push.
- **Version 0.10.0:** new operations and Kinds, and a field in the push ledger's lines.

## 46. The environments graph and the node menu (0.11.0)

The brief: Environments/Instances fills from Portal.Agent's `agent-environment` layer (Kind `environment.instance`, `properties.kind` compose or container, ids `claude-agent-dev` and `claude-agent-dev/<service>`, edges `member-of`, `runs-as` and `hosts`). Add a page Environments → Graph at `/<tenant>/environments/graph` over every `environment.*` Kind and what is one edge away, grouped by the `env`, `group` and `tags` the nodes carry, and the first right-click menu (Details, Show graph, Runs, Ledger) on the graph's nodes and the Instances rows. Every node and item is a link; the selected node is `#<id>`.

Asked, and Jerry chose:

- **Details links to the node's item page.** The brief names an "existing quick-details drawer", and the page has none. The details view that exists is the item page, `/<tenant>/<service>/<page>/<id>` (its fields and tabs). So Details is a link to the item page of the Page that lists the node's Kind, and no drawer was built.
- **`graph` is reserved in the Environments service.** The brief's "boot refuses a page named like a kind, as before" matched no rule in the code. Jerry's answer: the merged-config check refuses an Environments page with the id `graph` whose view is not `environments-graph` (`/menu/<i>/children/<j>/view: environments/graph is the environments graph, where every Show graph lands; ...`). A tenant overlay cannot turn the address every Show graph links to into a table of some Kind. The other options were refusing a page id that is a Kind's last segment, and adding no rule.

Choices made without asking:

- **The real layer was read, at the brief's direction.** CLAUDE.md does not list `../Portal.Agent` (entries 42, 45), but the brief asks for a fixture built from that real layer. The layer is in `agent/Orchestrator/artifacts/environment-layers/`, not in `ledger-layers/` as the brief says; that folder holds the ledger layer. CLAUDE.md's list is not widened, since that is the owner's call (entry 41).
- **Fixtures.**
  - `tests/fixtures/layers/agent-environment/` is `2026-10-11T10-08-56Z.environment.graph.json` and the `ontology.yaml` beside it, byte for byte, from Portal.Agent's working tree at 66eba88 (Up on 2026-10-11; Claude.Agent.Orchestrator 0.1.0).
    - It carries no `env`, `group` or `tags`.
    - Its `hosts` edge names `agent:run/2026-10-11T10:08:49.8679152Z`, which no fixture holds.
    - Its `runs-as` edge names `ui:principal:dev-bootstrap`.
  - `tests/fixtures/layers/agent-environment-tagged/` is derived from it, and only these things change:
    - `env: dev` on the compose node, the agent and mitm.
    - `group: stack` on the compose node and the agent; `group: egress` on mitm.
    - `tags`: `[agent, compose]`, `[agent]` and `[egress]`.
    - The ledger keeper carries none of the three, so it lands in `unset`.
    - `hosts` points at the agent-runs fixture's run.
    - Its ontology declares the three properties, marked fixture only.
- **API: `getEnvironmentGraph`, `GET /api/tenants/<tenant>/environments/graph`.** It is an envelope (module Portal) with no `ontology` of its own, because its nodes keep their layers' Kinds.
  - **Nodes.** Every node of a Kind starting `environment.` in the mounted layers, with its layer's id and properties. The first id wins in mount order and a node with no stamp gets `stamp.layer`, as `listNodes` does (entry 42).
  - **Edges.** Every edge of a mounted layer with such a node at one end, its kind kept.
  - **Neighbours, one hop only.**
    - A Run is served as `listAgents` serves it, so it carries its `layer` and `ledger` count, which the menu needs.
    - A Principal is served as `listPrincipals` serves it. dev-bootstrap is made up when `principals.yaml` lacks it, as the agents page does (entry 43).
    - Any other node is served as its layer emits it.
  - **Dropped edges.** An edge whose other end nothing serves is left out, so nothing dangles. This is the real layer's `hosts` edge until its runs layer is mounted.
  - **`has-run`.** A served Principal holding a served Run (by `listAgents`' grouping) gets a `has-run` edge to it. It adds no node. Without it, a principal's Details and Runs would link an agent page that is 404 for a principal holding no run.
  - Empty, not 404, when nothing is mounted.
  - The envelope does not go through the portal check (`CheckEnvelope`), which would refuse unprefixed ids and Kinds outside `ontology.yaml`. Like `listNodes`, it is layer data served read-only.
  - `ontology.yaml` is unchanged but for its version: no Kind or edge Kind is added.
- **"Ledger root" one edge away.** Neither real layer has an edge between them. The ledger layer's root is `ledger.jsonl` (a `LedgerChain`, module Claude.Agent.Ledger) with no edges, and its entries `records` the run. So the ledger is reached through the run: Ledger links `/<tenant>/ledger/agent?run=<id>` when the hosted Run counts entries. A ledger node would be drawn if an edge reached it.
- **Page.** Environments → Graph is `{ id: graph, path: environments/graph, view: environments-graph }`, first under Environments, so the rail lists it above Instances. It is `web/envgraph.js`, drawn as SVG and not with the vendored Cytoscape, because a canvas node cannot be an `<a>`.
  - **Layout.** A small force layout of its own:
    - Repulsion, doubled between two envs.
    - A link distance by edge kind: `member-of` 90, `runs-as` and `hosts` 170, any other 130.
    - A pull toward each env's mean, and a weak pull to the centre.
  - **Settle.** The layout settles in about 300 ticks, four a frame, and sets `data-settled` on the `svg`. Until it first settles, the view is fitted to the drawing, never closer than 1.5 times life size. After that the view stays put, so a drag does not move it under the pointer. Initial positions are fixed per env, so a reload lays out the same way.
  - **Pins.** Dragging a node pins it, shown with a dashed outline. Double-clicking releases it.
  - **Selection.**
    - Selecting a node pins it and its neighbours where the layout settles them.
    - Clicking a node's link selects it in place (`history.pushState`).
    - Back, Forward or an edited fragment reach the page through the shell's new `onHash` hook, so its pins and layout stay.
    - An item route, `/<tenant>/environments/graph/<id>`, is rewritten to `#<id>`.
    - An id the graph lacks says so in red in the toolbar.
  - **Look.**
    - Colour is an instance's `properties.kind`: compose, container, kubernetes or host.
    - Shape is the Kind: instance a circle, image a square, network a diamond, any other a hexagon.
    - A node one edge away is smaller and at 55% opacity.
    - `member-of` is solid, `runs-as` dashed, `hosts` dotted and any other dash-dot, all with arrowheads.
    - A legend under the graph says all of this.
- **Grouping.** It is read from node fields only.
  - **env.** `env` is a string. Missing or empty is `unset`. dev, qa and prod are always offered as chips, and any other value gets its own hull and chip, since Portal reads env and does not judge it.
  - **group and tags.** `group` draws a hull inside its env's hull, labelled at its foot. `tags` is a list, or one string read as one tag.
  - **Neighbours.** A neighbour is in no hull.
  - **Chips.** The env and tag chips are Instances' chips (`nodes.chips`, which now takes its group's aria-label). A chip hides what it does not match; a neighbour stays while a shown node is next to it.
  - **Search.** The banner's search dims what it does not match.
  - **Count.** The toolbar counts `<n> nodes · <m> one edge away · <e> edges · <u> without env`.
- **The node menu, `web/nodemenu.js`.** It opens on right-click, and on Shift+F10 or the menu key, which reach the page as a `contextmenu` at 0,0, so the menu opens beside the focused node or row. Arrow keys, Home and End move through it, Enter follows, and Escape returns focus.
  - **Items.** Every item is an `<a>`. One with nowhere to go has no `href`, has `aria-disabled`, and says why in its title.
    - **Details:** the node's item page. For a Run, its row on the runs layer's page (entry 43's `#<id>`). For a Principal, its agent page, when a `has-run` edge says it holds a run.
    - **Show graph:** `/<tenant>/environments/graph#<id>`.
    - **Runs:** the newest Run a `hosts` edge reaches, as its row on the layer page. For a Principal, its agent page, as above.
    - **Ledger:** `/<tenant>/ledger/agent?run=<id>`, when that Run counts ledger entries.
    - A container has no `hosts` edge, so its Runs and Ledger are greyed.
  - **Where it opens.** On the graph, and on every table page of an `environment.*` Kind: Instances now, and Images and Networks once they fill. The rows read the environments graph on the first right-click.
- **Not added.**
  - A PowerShell wrapper (`Get-PortalEnvironmentGraph`): the brief asks for routes, pages and tests.
  - A way to drag or release a pin by keyboard: nodes and items are reachable, and selection pins by keyboard.
- **For later.**
  - Entry 45's ledger fixture should be replaced by a copy of the real ledger layer. That layer's module is Claude.Agent.Ledger, its root `ledger.jsonl` is a `LedgerChain`, and it has 236 entries. Portal finds it by its `ledger:<seq>` entries, so it mounts.
  - Kubernetes and host colours are checked only in the stylesheet: no layer emits them yet.
- **Coverage.**
  - Go:
    - the operation against the spec (200, 404 for another tenant), the page and its item route;
    - empty with no environment layer;
    - the real layer: four instances, dev-bootstrap made up, member-of and runs-as, its hosts edge left out;
    - one hop: the Run with layer and ledger 9, no Step, Decision or ledger entry, the three edge kinds;
    - the first layer winning on an id and every edge once;
    - `has-run` from dev-bootstrap to runs a legacy token mounted;
    - the boot refusal of a graph page with another view;
    - the menu order.
  - Pester (Chrome):
    - the rail order, the real layer's nodes as links, edges by kind, the unset chips and hull, the shapes;
    - the tagged copy's hulls, tag chips, the node after `#` selected with its five neighbours pinned, the item route rewritten, a chip hiding the keeper;
    - a drag pinning and a double-click releasing;
    - the menu on a compose node, a container, the principal and the run, its keys, Show graph selecting in place, and the Instances row's menu landing on the graph with the node selected.
- **Version 0.11.0:** a new operation and page.

## 47. Alerts: the page, the badge and the reverse links (0.12.0)

The brief: Portal.Agent is adding an agent-alerts layer, nodes of Kind `alert` under the plain id `alert/<fingerprint>` with `severity` (critical, warning, info), `rule`, `summary`, `stamp`, `state` and `fingerprint`, an `about` edge to the subject (a ledger entry, a run, a decision, an environment instance) and a `raised-in` edge to a run root. Portal reads it and never writes an alert. Build on a hand-made fixture of that shape until the real layer lands. Add Compliance/Alerts with an item page, a banner badge, the API (paged 100, filters severity, rule and state), an alerts row on each subject's page and an alert count on the agent page's runs, a right-click menu on alert rows, and a boot rule.

Choices made without asking:

- **The fixture, `tests/fixtures/layers/agent-alerts/`.** It is hand-made, module `Claude.Agent.Alerts` 0.2.0 (the module the in-progress alerts code in Portal.Agent carries, a version after its 0.1.0), layer `2026-10-09T02:51:05Z`. It holds five alerts, all raised in the agent-runs fixture's run:
  - `3f9c2a7e5b1d4c08`: critical, `policy-deny`, open, about step 3's pre deny (a PolicyDecision).
  - `07e3d9a5c1f86b42`: critical, `policy-deny`, resolved, about `ledger:52`.
  - `5d27e8f1a9c3b604`: warning, `run-denied`, acknowledged, about the run.
  - `a41e09b7c2d85f36`: warning, `egress-check`, open, about `ledger:46`.
  - `c8b6f4d2e0a17935`: info, `image-by-tag`, open, about the compose node `claude-agent-dev`.
- **The fixture carries copies of three subjects.** Graph.Node refuses an edge that dangles to a plain id, or to an id under the envelope's own module's prefix (`dangling-edge`, error); only an end under another module's prefix may dangle (warning, "resolved at merge"). `ledger:` is registered to Claude.Agent.Alerts and `claude-agent-dev` is plain, so the layer must hold `ledger:46`, `ledger:52` and `claude-agent-dev` for its `about` edges to validate. It does so as Portal.Agent's `Export-LedgerAlert` already does for the ledger entries its findings name. The copies keep only `seq`, `stamp`, `event` and `run`, or `kind`, `state` and `composeProject`, and its ontology declares them as copies.
- **An alert layer's copies are references, never data.** A layer holding an `alert` node under `alert/` is an alert layer (`store.IsAlertLayer`). Its other nodes:
  - never make it the agent's ledger. Without this rule, its later stamp would take the agent chain from the ledger layer (entry 45's "latest stamp wins") and the chain would be two entries;
  - are no environments (the environments graph skips the layer);
  - are not listed on a table page (`listNodes` gives an alert layer's alerts only);
  - are never served as a subject (`HeldNodes`).
  So a subject is served only from the layer that owns it. With no environment layer mounted, `claude-agent-dev` is no one's: its alert has no `about` edge, and `about` still names it.
- **API, spec first.**
  - `listAlerts`, `GET /api/tenants/<tenant>/alerts`: one page (`limit`, 1 to 1000, 100 when absent) under the list node `ui:alerts`, Kind `Alerts`.
    - The root carries `count`, `alerts` (matching), `open` (the open alerts by severity, critical, warning and info always), and `rules` and `states` (every value, sorted, for the chips). It also carries the filters given, and `next`.
    - Filters: `severity`, `rule` and `state` as the brief asks. Two were added: `run` (raised in that Run), which the agent page's count links, and `about` (about that id), which the subject pages read.
    - `after` is a fingerprint: the page starts after that alert in the order. It is found among all the alerts, so a page still follows an alert whose state has since left the filter. A fingerprint no layer holds is 400.
  - `getAlert`, `GET .../alerts/<fingerprint>`: the alert, its subject and its run; 404 for a fingerprint no mounted layer holds.
  - Each alert is served as Portal's Kind `Alert` under its layer's id, with `fingerprint`, `severity`, `rule`, `summary`, `stamp`, `state` (absent when the layer gives none), `open`, `layer`, `about` and `run`. It is joined by `has-alert` from the root, and by `about` and `raised-in` to the nodes Portal serves. An edge to an end Portal does not serve is left out, as the environments graph does (entry 46).
  - **Order:** critical, warning, info, then any other severity; then the newest stamp; then the fingerprint.
  - **Open:** state `open` or no state. The badge and `state=open` count an alert with no state as open, since an alert nobody has acted on should not drop out of sight.
  - **Re-emits:** a fingerprint in two layers is kept from the later stamp (the later mount on a tie), as a Run is (entry 43): a re-emit is how a state change arrives.
  - **No ontology of its own.** The envelope names none, like the environments graph's, because a subject keeps its layer's Kind (`environment.instance`).
- **One resolver for the environments graph and the alerts** (`internal/server/resolve.go`). The brief asks for "the same resolver the environments graph uses". Entry 46's `environmentEnds` became `resolver`, which serves:
  - a Run as `listAgents` does;
  - a Decision (a PolicyDecision of a runs layer) as `listAgentRuns` does;
  - an agent ledger entry (`ledger:<seq>`) as `getLedgerEntry` does;
  - a Principal as `listPrincipals` does, dev-bootstrap made up;
  - any other node as the first layer holding it, alert layers aside.

  The Decision and ledger entry cases are new. No environment edge reaches either today, so the environments graph serves what it served, and its tests are unchanged. On the page, the node menu's Details became `nodemenu.detailsHref`. It now also knows a Decision (its runs layer's row), a LedgerEntry (its entry page) and an Alert (its page). The alerts page links subjects and runs with it.
- **Run gains `alerts`**, the alerts raised in it, while any alert is mounted, as `ledger` is while a ledger layer is (entry 45). It counts by `raised-in`, the run's own alerts, not by `about`. The agent page's column links `/<tenant>/compliance/alerts?run=<id>`.
- **Page.** Compliance/Alerts is `{ id: alerts, path: compliance/alerts, view: alerts }`, first under Compliance; `web/alerts.js`.
  - **Rows:** severity in its colour (critical red, warning amber, info muted, with a matching bar on the row), rule, summary (a link to the alert's page; the brief's table names no link column), subject, run and stamp. An alert that is no longer open is dimmed.
  - **Chips:** severity (the three, always) and rule and state (the root's values). A chip is a move to the list with that filter in the URL, from the first page, so a filtered list is a link and the filter applies across pages, not only to the page on screen. The banner's search filters the rows on the page. "more alerts" pages with `after`.
  - **Item page:** `/<tenant>/compliance/alerts/<fingerprint>`, through `getShellItem` like any page's item. It shows id, severity, rule, summary, state, stamp, fingerprint, layer (a link), `about` and `raised-in` as links, or as plain text with why when Portal serves no node there.
- **Badge.** It sits beside the mode, in the banner, and is read on every page through `listAlerts?limit=1`.
  - **Shape:** the count of open alerts at their top severity, `1 critical` in red or `<n> warning` in amber, with every open count in its title. Nothing when only info alerts, or none, are open.
  - **Link:** the alerts page filtered to that severity and `state=open`, the alerts it counts.
  - **Mode:** it shows beside DEMO and prod's nothing as well as beside DEV.
- **Reverse links: an "alerts" row** with one link per alert about the node, in its colour, from `listAlerts?about=<id>`, shown only when there is one.
  - **Ledger entry:** a field on its page.
  - **Run and Decision:** their item is their row on the runs layer's page (entries 43 and 46). The row the fragment marks gets an alerts row beneath it.
  - **Environment instance:** a field on its item page. Every table page's item gets the row, since it costs one read and shows only when an alert is about the node.
- **The menu.** It is the node menu, opened with an item list of its own (`nodemenu.open` takes one): Details (the alert's page), Subject, Run (its row on the runs layer's page) and Ledger (the run's entries on the agent chain, `?run=`). It follows the same greyed-with-why rule: `no about edge`, `no mounted layer Portal reads holds <id>`, `no mounted runs layer holds <id>`, and `no raised-in edge to a mounted run` or `the agent ledger records nothing of this run`.
- **Boot.** A Compliance page with the id `alerts` and any view but `alerts` stops the boot: `/menu/<i>/children/<j>/view: compliance/alerts is the alerts page, where the banner's count and every alert's page are; its view is alerts, not "<view>"`. This is the rule entry 46 made for `environments/graph`. `alerts` joins the schema's view enum.
- **Ontology.** Kinds `Alerts` and `Alert`, edges `has-alert`, `about` and `raised-in`, and Run's optional `alerts`. No id prefix is added: alerts keep their layer's plain ids, and the list node is `ui:alerts`.
- **CLAUDE.md** now lists `../Portal.Agent` as readable, with the alert layer's future home, as the brief asks and as entries 45 and 46 proposed.
- **Fixed in passing.** Manual check item 2.14's fixture paths had `\t`, `\f` and `\a` turned into control characters.
- **Not added.**
  - A PowerShell wrapper for the two routes. The brief asks for routes, pages and tests.
  - A way to acknowledge or resolve an alert, since Portal never writes one.
  - Filtering by more than one value of a filter.
- **For later.**
  - Replace the fixture with a copy of the real layer once it lands under Portal.Agent's `artifacts/alert-layers/`.
  - Portal.Agent's alerts code today emits another shape: Kind `Finding`, ids `alert:<rule>:<seq>` (the shape Graph.Node registers for `alert:`), severities info, warn and error, and a `raised-by` edge to a LedgerEntry. Portal reads only the brief's shape, so a layer of that shape mounts and shows no alerts. Such a layer also holds copies of `ledger:<seq>` entries but no `alert` node, so it is no alert layer: mounted with a later stamp than the ledger layer, it would take the agent chain, as it could before 0.12.0.
- **Coverage.**
  - Go:
    - empty with no alert layer, and no `alerts` on a run;
    - the fixture's order, root counts, chip values, edges, and each subject from its own layer;
    - one alert, and 404;
    - every filter, pages of two and `after` past a filtered-out alert; `limit` and `after` 400s;
    - the alert layer neither the ledger, an environment nor on a table page;
    - a re-emit winning;
    - the run's `alerts`;
    - the spec operations, the page and its item route;
    - the menu order and the boot refusal.
  - Pester (Chrome):
    - the rail, the rows in order and colour, subject and run links;
    - the chips through the URL, the item page, and 404's message;
    - the badge and `badgeOf`;
    - the alerts rows on entry 52, the decision's and the run's rows, `claude-agent-dev`, and the agent page's count;
    - the menu by mouse and keyboard, and greyed with why on an orphan alert;
    - the boot refusal.
- **Version 0.12.0:** new operations, Kinds and a page.

## 48. New prefixes only, no mapping: alerts and environments read env: and alert: (0.12.1)

The brief: Portal.Agent now emits environment ids as `env:compose/<project>`, `env:container/<project>/<service>`, `env:image/<ref>` and `env:network/<name>`, and alert ids as `alert:<fingerprint>` (64 hex). Graph.Node 0.3.3 registers both. Make the alert reader, the copies-only guard, the subject resolver, the badge, the reverse alerts rows and the right-click menu match the new forms only, with no fallback and no mapping. Bring the lift to 0.3.3. Replace the environment and alert fixtures with the real layers from Portal.Agent's latest Up, and prove the old alert form is ignored.

**New prefixes only, no mapping.** Portal reads an alert only under `alert:<fingerprint>` and an environment node only under one of the four `env:` forms. `alert/<fingerprint>`, Claude.Agent.Alerts' `alert:<rule>:<seq>` and a bare environment id (`claude-agent-dev`, `claude-agent-dev/mitm`) are not read. Nothing rewrites them to the new form, and nothing falls back to them.

Choices made without asking:

- **Where the match lives.** Two functions decide, and every listed reader goes through them:
  - `store.AlertFingerprint(id)`: `alert:` then exactly 64 lower-case hex. `store.Alerts` (the list, the item, the badge's open counts, the `about` filter behind the reverse rows, the run's `alerts` count) and `store.IsAlertLayer` (the copies-only guard: ledger, environments, table pages, `HeldNodes`) use it.
  - `store.IsEnvironment(n)`: an `environment.*` Kind and `graph.ValidID("env:", id)`, so the lifted registry's own pattern decides. `store.EnvironmentGraph` uses it.
  - On the page, `nodemenu.isEnvironment` is the same test with the same pattern. `envgraph.js` uses it for which nodes are its own. `nodes.js` uses it to decide whether an Instances, Images or Networks row opens the node menu.
- **The fingerprint is checked, not only the prefix.** Since `alert:<rule>:<seq>` shares the prefix, `alert:` alone would half-read Claude.Agent.Alerts' ids. The 64-hex pattern is what Graph.Node 0.3.3 registers for the fingerprint form.
- **The subject resolver needed no change.** `resolver.resolve` matches `agent:run/` Runs, Decisions, `ui:principal:` and `ledger:` by what Portal serves. It matches nothing on alert or environment ids. Any other id is served from the first non-alert layer holding it, whatever its form (see Still reading a bare id).
- **The badge and the reverse rows needed no page change.** Both read `listAlerts`, the badge for `open` and the rows for `?about=<id>`, so they follow `store.Alerts`. The alert menu and the item page read `properties.fingerprint` and the route `/<tenant>/compliance/alerts/<fingerprint>`, which keeps the 64 hex.
- **The lift moves to Graph.Node v0.3.3** (`a536b0b`). Without it, `alert:<64 hex>` fails `ValidID` and refuses the alert layer. `internal/graphnode/schemas`, `internal/graphnode/graph` (non-test files, `idformat.go` new), `cmd/graphnode/main.go`, `.claude/skills/graph-node` (0.3.3) and `tests/fixtures/invalid.graph.json` are `git show v0.3.3:<path>`, the Go files with the import path rewritten. Pester's `$GraphNodeTag` is `v0.3.3`. There is still no registered edit. v0.3.3 brings:
  - `env:` owned by Claude.Agent.Orchestrator;
  - `alert:<fingerprint>` accepted;
  - `SharedWith` on `alert:` (Orchestrator) and `ledger:` (Portal, Orchestrator, Ledger);
  - an `about` edge from an `alert:` id may dangle to any registered prefix, as a warning.

  CLAUDE.md's two mentions of the pinned tag now say v0.3.3, as its rule says to do when a tag is named. Its fixtures line no longer calls the alert layer hand-made.
- **The fixtures are the real layers from the latest Up** (`agent/Orchestrator/artifacts/up.json`, started 2026-10-11T19:00:11Z). Each graph.json is byte for byte, without the folder's `ontology.yaml`, as before:
  - `agent-environment/2026-10-11T19-00-30Z.environment.graph.json` holds nine nodes (a compose node, three containers, three images, two networks) and eleven edges (`member-of`, `runs-image`, `attached-to`, `runs-as`, `hosts`).
  - `agent-alerts/2026-10-11T19-00-31Z.alerts.graph.json` holds one alert, info, `egress-attempt`, open, about `ledger:303`, raised in `agent:run/2026-10-11T19:00:22.5540487Z`. Neither end is in a fixture, and the layer carries no copies.

  The three old files are deleted.
- **The tagged copy is rebuilt from the new real layer** in the same way as before: `group` and `tags` on the compose node, agent and mitm, and the hosts edge (and its id) onto the agent-runs fixture's run. The real layer now gives every node `env: dev`, so the copy drops `env` from the ledger keeper. That keeps one node `unset`, which the hulls, the dev chip and the page's gap need.
- **The varied alerts live in the tests, not in a fixture.** One real alert cannot show the severity order, the chips, paging, the badge's colours, a resolved or acknowledged state, or the subjects' alerts rows. Go (`variedAlerts`) and Pester (`Mount-AlEnvelope` in the Alerts block) mount five alerts inline in the new form, module Claude.Agent.Orchestrator 0.3.0. They mean what the old hand-made fixture meant, and each fingerprint is SHA-256 of the rule and the subject with a newline between, as the registry defines it. They keep copies of `ledger:46`, `ledger:52` and `env:compose/claude-agent-dev`, so the copies-only guard is still tested. The real fixture has tests of its own: Go `TestAlertsOfTheRealLayer`, the spec test's alert calls, and a Pester It that mounts it by path.
- **The old form is proven ignored.** Go `TestOldAlertIDsAreIgnored` mounts a layer with two alerts under `alert/`, one with 16 hex and one with 64. It gets 201, then no alert in the list, no open count, 404 on both fingerprints, nothing under `?about=` and no `alerts` on the run. `TestEnvironmentGraphIgnoresBareIDs` mounts two bare-id `environment.instance` nodes and gets an empty graph. Manual check 1.22 shows the same over the API.
- **Spec text only.** `api/openapi.yaml` describes the new forms. No operation, parameter or schema changes, but the regenerated `internal/gen` embeds the spec, so it changes. Line 40 avoids a bare `env: ` because YAML reads it as a mapping.
- **Version 0.12.1:** a reader change with no new operation or Kind. `ontology.yaml`'s Alert idRule and `fingerprint` say `alert:`.
- **Still reading a bare id** (reported, not changed, since the brief names the readers to change and these are generic):
  - A table page (`listNodes`, Environments/Instances, Images and Networks) lists every node of its Kind in a mounted layer, bare-id `environment.instance` nodes included. Their rows no longer open the node menu.
  - The subject resolver and the environments graph's far ends serve any node a non-alert layer holds, so an alert `about` a bare id is still linked when some layer holds that id.
  - A layer of `alert/` ids is no alert layer any more. Any LedgerEntry or `environment.*` copies it carries are read as ordinary nodes: a LedgerEntry copy can become the agent chain if its stamp is the latest, and an `env:` copy an environment. The real layers carry no copies.

## 49. Emitters match by source block, never by name (0.13.0)

The brief: Graph.Node v0.4.0 gives emitter cards `module`, `repo`, `emits` (facet and kinds) and an optional `url`; Portal.Emitters v0.2.0 pushes nine such cards; Portal.Agent's layers and Emitter.Agent's export fill Graph.Node's design-20 source block `{emitter, version, target, stamp}`. Recopy the lift and `graph-node` from v0.4.0. Remove the `Emitter.<X>` → `Graph.Emitter.<X>` mapping. An emitter is its card name, and a layer belongs to it when `source.emitter` is that name. A layer with no source block is listed under its top-level `module` as `unstamped`, never matched to a card. The Catalogue is a roll call with statuses `ok`, `version-mismatch`, `no-layers`, `no-card` and `unstamped`, and chips by status and facet. One Sources page per emitter at `/{t}/emitters/{name}`, built from the card. The nine cards are fixtures, with tests for each status.

**Nothing maps one name onto another.** `tenant.EmitterName` is gone. A card is the emitter of its own name. `store.Layer.Emitter()` returns the source block's `emitter` (stamped) or the envelope's `module` (unstamped), never the ontology's module. Only a stamped layer can reach a card, and only by the card's exact name. A card's `module` is shown on its page but never used to match: the unstamped FileSystem fixture (`Graph.Emitter.FileSystem`) does not join the card `Emitter.FileSystem`, whose module that is. Go and Pester both check this.

Choices made without asking:

- **The lift moves to Graph.Node v0.4.0** (`b0fd7a1`). `internal/graphnode/schemas`, `internal/graphnode/graph` (non-test files), `cmd/graphnode/main.go`, `.claude/skills/graph-node` (0.4.0) and `tests/fixtures/invalid.graph.json` are `git show v0.4.0:<path>`, the Go files with the import path rewritten. The diff from v0.3.3 is upstream's own: the manifest schema (`module`, `repo`, `url`, `emits`; an emitter card requires the first three), `graph.Manifest`/`ManifestEmit`, the `emitter-name` warning and the CLI version. There is still no registered edit. Pester's `$GraphNodeTag` is `v0.4.0`. CLAUDE.md's two pin mentions say v0.4.0, as its rule asks when a tag is named. The skill's own text still says `go get ...@v0.3.3` in one line. That is upstream's at the tag and the copy is read-only, so it is left as is.
- **A push now refuses a 0.3-shaped emitter card** (no `module`, `repo`, `emits`): 422 `schema`, from the lifted validator. `emitter-name` is a warning and a push drops warnings (as before), so `Emitter.Git.Repository` would still push.
- **The card's source fields are kept on the ledger line.** The ledger held name, version, type and hash, not the manifest, so a Sources page "built from the card" had nothing to read. Reading the card back from the registry on every page would make a page read depend on a feed being up. A push now writes `module`, `repo`, `url` and `emits` on its line when the card has them (`omitempty`). `FeedsEnvelope` carries them on the Card node, so `listFeeds` and `listEmitters` serve them. A line from before 0.13.0 has none, and the page says "The push recorded no emits". The spec's `LedgerEntry` gains the four optional properties. The ledger is still one of the three files the binary writes, and still append-only. The chain hashes the line's bytes, so longer lines chain the same way.
- **Statuses.** A row is a card when a feed holds a card of that name (newest push wins, as before). It is `no-layers` when no layer names it. It is `version-mismatch` when any naming layer's `source.version` differs from the card's `version`, and `ok` otherwise. A name only source blocks give is `no-card`, and a module of unstamped layers is `unstamped`. Every stamped row carries `layerVersions` (distinct, sorted), so a mismatch shows both sides: the card's `version` and the layers'.
- **Ids.** A card stays under its own `ui:card:` id. A `no-card` name is `ui:emitter:<name>`, as the old unknown emitter was. An unstamped module is `ui:module:<module>`, a new id shape under Portal's `ui:`. That way an unstamped module named like a card or a `source.emitter` is its own row and never a duplicate node. Both are Kind `Emitter`. Its required `unknown` is replaced by a required `status` (`no-card` or `unstamped`), plus `layerVersions` on a no-card row. Card gains `module`, `repo`, `url`, `emits`, `status` and `layerVersions`. Layer gains `source`, the block as it is in the layer. `Layer.emitter` is the source block's emitter, or the module.
- **Order and lookup.** Rows are sorted by name, with an unstamped module after an emitter of the same name. `getEmitter` and the page find a card or `source.emitter` name first. An unstamped module that shares a name has a row but no page of its own (ONTOLOGY.md, Not here yet). Go checks this with an inline layer of module `Emitter.Ledger`.
- **"Sources" is the emitter's own page.** `/{t}/emitters/{name}` keeps its route (entry 44) and now draws the source. It shows a definition list: module, version, layer versions (red on a mismatch), repo linking `url`, `url` as "Open in source" (both open a new tab), the card linking its row on Artifacts/Feeds, and status. Under it are the emits table (facet, kinds) and the layers table: name linking its page, source version (red where it is not the card's), target, source stamp, nodes, edges and principal. An unstamped layer shows its envelope stamp. The rail keeps Emitters/Catalogue current there, as in 0.9.0. The menu page Emitters/Sources (`kind: emitter.source`) is unchanged: the brief puts the page per emitter at `/{t}/emitters/{name}`.
- **"Every id a link"**: the emitter name, the card and each layer link to their pages. `mountedBy` stays the principal's name, since there is no page per principal outside Identity/Agents.
- **Chips.** Status chips (all five, always) and facet chips (the facets the cards emit) filter on the page. They keep the filter in the URL (`?status=`, `?facet=`) with `history.replaceState`, so a filtered list is a link that the list draws on load. They combine with the banner search. The toolbar reads `<n> emitters · <count> <status> ...` for each status present.
- **Colours** in base.css: `ok` green and `version-mismatch` red (`--gr-ok`, `--gr-bad`), `no-card` amber (the old unknown's hue), `no-layers` and `unstamped` muted. The layer page's produced-by line reads `produced by <emitter> <source.version> [(card <v>)] <status>`.
- **No layer on disk is stamped yet.** Portal.Agent's stamping commit (ce2a76f) and Emitter.Agent 0.1.4 have not run since, and every runs, ledger, environment and alert layer under `../Portal.Agent/agent/*/artifacts` and `../Portal.Emitters/emitters/Agent/artifacts` has `source: null`. The fixtures stay byte for byte the real layers. The tests (Go `stamped`, Pester `New-StampedLayer`) copy one to a temp folder and insert the source block after the opening brace, changing nothing else, with the values each emitter's code writes:
  - agent-runs: `Emitter.Agent` 0.1.4, target the ledger file.
  - ledger: `Emitter.Ledger`, the Claude.Agent.Ledger psd1's 0.3.0.
  - agent-environment: `Emitter.Environment`, EnvironmentLayer.ontology.yaml's 0.3.0.
  - agent-alerts: `Emitter.Environment`, Claude.Agent.Alerts' 0.4.0.

  Portal.Agent's layers use target `default` and the layer's stamp. `no-card` has no real case, so the broken ledger copy is stamped `Emitter.Unlisted` 1.0.0.
- **The fixtures.** `tests/fixtures/cards/<card name>/manifest.yaml` holds the nine cards, `git show v0.2.0:emitters/<folder>/manifest.yaml` from Portal.Emitters (clean at v0.2.0): Agent 0.1.4, AzureDevOps 0.3.1, Environment 0.3.0, FileSystem 0.2.0, Git 0.1.1 (folder Git.Repository), Ledger 0.3.0, Network 0.4.0, Reference 0.1.1, Terraform 0.17.1. `Emitter.Agent` replaces the 0.1.3 copy. Go pushes all nine through the fake registry, a real push with the lifted check. Pester has no feed to push to, so it writes nine chained ledger lines from the cards. It reads their `emits` with a narrow reader (every card writes one flow map per line) and fails on any other line.
- **Pester's Emitters block has the tag `Emitters`**, so `Invoke-Tests.ps1 -Tag Emitters` runs it alone. No other block is tagged.
- **Version 0.13.0:** the meaning of an operation's result changed, Kinds and properties changed, and a ledger field was added.

## 50. Actions on containers: Portal asks, the Runner executes, the ledger records, the graph shows the gap (0.14.0)

The brief: on Environments → Graph, right-click a container to get Logs, Stop and Restart. Choosing one appends an `action.request` line to the tenant's ledger as the signed-in principal, shows the id of the request it made, and the page shows the result once the Runner has written one. Portal never executes anything; it asks. Portal.Agent is building the Runner against this contract, unchanged:

- A request line, which Portal writes: `kind: action.request`, `id: ledger:<seq>`, `action` (logs, stop or restart), `subject` (`env:container/<project>/<service>`), `principal`, `requested`, `status: requested`.
- A result line, which the Runner writes and Portal only reads: `kind: action.result`, `id: ledger:<seq>`, `request` (the request's `ledger:<seq>`), `status` (done or failed), `started`, `finished`, `exit`, `output`, and a source block.

Since 0.14.1 both lines live in one file, `tenants/<t>/ledger.jsonl`: one ledger, one chain, one seq space (entry 51 has the choices).

The work asked for:
- `POST`, `GET .../actions` and `GET .../actions/{seq}`.
- The menu items, on containers only.
- An Actions page at `/{t}/actions`.
- The ledger entry page showing both kinds of line, linked both ways.
- Action lines read from the ledger layer only (0.14.0; since 0.14.1, from the tenant's ledger only).
- A fixture with one done, one failed and one unanswered request.
- Tests, and this entry.

**The four roles as they stand in code.**
- **Portal asks, as a principal.** `POST /api/tenants/<t>/actions` (`server.PostAction`) takes `{action, subject}` and nothing else. The principal is the token's, from the middleware's context (`principal(r)`). `tenant.RequestAction` appends the line through `record`, the push's write path, and returns it. Nothing in Portal starts, stops or reads a container.
- **The Runner executes.** Portal.Agent's Claude.Agent.Runner reads the requests from `tenants/<t>/ledger.jsonl` and appends its `action.result` lines to the same file, under the same lock, with the same `seq` and `prev`. That is its code, not Portal's.
- **The ledger records.** Request and result are lines of `tenants/<t>/ledger.jsonl`, chained like every line since 0.10.0: one ledger. The agent's own ledger, and so its ledger layer, carries no action lines.
- **The graph shows the gap.** Each request is served as its LedgerEntry with an `outcome`: `requested` until a result names it, then the result's status. Request and result are joined by `answered-by` and `answers`. The Actions page lists what is still `requested`.

Asked, and Jerry chose:

- **Split chains (0.14.0, undone in 0.14.1).** In 0.14.0 Portal served the request as `ui:ledger:portal/<seq>` (the contract id as `lineId`) and read results from the agent ledger layer only. Portal.Agent's final shape put both lines in the tenant's file instead, so 0.14.1 took the option 0.14.0 passed over:
  - **One ledger.** The Runner appends its results to the tenant's `ledger.jsonl`, and Portal reads the file's tail before every read and every append.
  - Every line of that file is served as `ledger:<seq>`, its position, which is the id an action line carries. `ui:ledger:portal/<seq>` and `lineId` are gone.
  - Results are read from `ledger.jsonl` only, never from a mounted layer or an alert layer's copy.

  The option still passed over: requests through the layer only, where a new request stays invisible until the next ledger emit.

Choices made without asking:

- **The line on disk.**
  - Members are in the contract's order, then `seq` and `prev`, which every line has carried since 0.10.0 (entry 45). The id's seq is the line number.
  - `requested` uses the identity files' layout (`2026-10-11T09:00:00.000Z`).
  - `tenant.LedgerEntry` gains `Action` (`json:"-"`). `record` writes an action as its `ActionRequest`, and the boot reads a line whose `kind` is `action.request` as one.
  - A request line that breaks the contract stops the boot, naming the line, as a bad push line does (entry 40). That covers an unknown action, a subject that is no container, an id that is not `ledger:<n>`, a missing principal, time or status, or an extra member.
  - Push lines are unchanged. Every reader of push lines already skips a line whose `result` is empty.
- **One writer at a time.** `record` takes `ledgerMu` around reading the last line and appending the new one. A push still holds `pushMu` across its duplicate check and the feed call, but an ask does not wait up to two minutes behind a push. Since 0.14.1 it also takes the file's lock, the Runner's (entry 51).
- **Who may ask.**
  - Contributor or admin, as any write (the middleware: 401 without a token, 403 for a reader).
  - A token from before 0.5.0 is 403: it belongs to no principal, and the line names one.
  - A body that names `principal`, or any member but the two, is 400. The principal is never taken from the body, and a body that tries to set it is refused rather than ignored.
- **What is checked.**
  - `action` is exactly one of the three, case-sensitive.
  - `subject` passes the lifted registry's `env:` check and starts `env:container/`. Anything else is 422.
  - Portal does not check that a mounted layer holds the container: the Runner answers for what it can reach, and a failed result says so.
- **Reading a result** (0.14.1: `tenant.decodeResult`, served by `server.portalChain`).
  - A line of `ledger.jsonl` whose `kind` is `action.result` is a result. Portal serves `event: action.result` and the contract's fields: `request`, `status`, `started`, `finished`, `exit` and `output`, with the line's source as `writer` and a node source block naming its emitter, `ledger.jsonl` and the line.
  - A mounted layer's `action.result` node, the agent ledger layer's or an alert layer's copy, answers nothing. Copies only for alert layers, as before (entry 47).
- **The join** (`server.joinActions`, run wherever the chains are built, so every ledger route carries it):
  - A result answers the request before it in the chain whose id its `request` names. With several, the newest wins, so a retry's result overrides an earlier one.
  - The request gets `outcome`, `resultId` and `resultSeq`, and the result gets `requestSeq`.
  - **Id reuse after a reset is no problem on one chain.** A `ledger.jsonl` started over reuses `ledger:1`, but a reset drops requests and results alike: no old result is left to answer the new request. (In 0.14.0, with results in the agent ledger, one could.)
- **Served shapes.**
  - **Kind `Actions`**, `ui:actions`, with `count`, `requested`, `done` and `failed`, joined by `has-action` to each request, newest first.
  - **LedgerEntry gains** `action`, `subject`, `principal`, `status`, `outcome`, `resultId`, `resultSeq`, `request`, `requestSeq`, `started`, `finished`, `exit`, `output` and (0.14.1) `writer`. `event` now also names `action.request` and `action.result` in the portal chain. (0.14.0's `lineId` is gone in 0.14.1.)
  - **New edges** `answered-by` (request to result) and `answers` (back).
  - **Ledger entry route.** `getLedgerEntry` serves the other half beside the entry, with both edges.
  - **One request.** `getAction` is one request, with its result when there is one; 404 when that line is no request.
  - The actions list has no paging (a later need).
- **Menu.**
  - `nodemenu.isContainer` decides: an environment node of Kind `environment.instance` with `properties.kind` container. The brief's rule, the API's subject rule and that test pick the same three nodes of the real layer, which Go checks.
  - The three items are buttons below a rule, since they go nowhere. Entry 46's "every item is an `<a>`" still holds for the items that link.
  - They come from `nodemenu.items`, so an Instances row's menu (the same menu, entry 46) has them too.
  - While the viewer may not write they are greyed with `toast.refusal`'s reason (`sign in to write`, or the reader's role).
  - Stop and Restart ask first in an in-page dialog, not the browser's `confirm()`, with Cancel focused and Escape or a click outside as Cancel. Logs does not ask.
- **After the ask**, a notice (a toast, since 0.14.0 able to hold a node) reads `asked: <action> <subject> · ledger:<seq>` for 20 seconds. The id links `/<t>/ledger/portal/<seq>`.
- **The Actions page.** It is Environments/Actions, `{ id: actions, path: environments/actions, view: actions }`, last under Environments, served at `/<t>/actions` like `/<t>/agents` (entry 43). It is `web/actions.js`.
  - **Rows:** action (linking the entry), subject (linking the graph with it selected), principal, requested, status and result (linking the result's entry, in the same chain).
  - **Principal links** go to the Users row marked by `#<id>` (`principals.js` now marks it), an agent's page for type agent, or plain text with why for a principal `principals.yaml` lacks.
  - **Status colours:** requested in the warning amber (the gap), done green and failed red with a red bar on the row.
  - A click anywhere on a row but a link opens the request's entry. The toolbar reads `<n> actions · <r> requested · <d> done · <f> failed`.
  - No boot rule was added: `/<t>/actions` is two segments, so no shell route competes with it.
- **The entry page** (`ledger.js`).
  - **A request** shows its action, subject (linked), status, outcome, `answered by` (the result's link, or `nothing yet: ...`) and principal (linked).
  - **A result** shows `answers` (the request's link, or why not), status, started, finished, exit, the output in a `pre` block, `written by` from its `writer` (emitter, version, tenant, stamp) and `line` from its source block.
  - **The portal chain's list** shows `<action> · <outcome>`, the subject and who asked for a request, and `<status> · answers <request>` and the emitter for a result.
- **Fixtures** (as of 0.14.1).
  - The ledger layer fixture is entry 45's again: 12 entries, 44 to 55, no action lines. 0.14.0's entries 56 and 57 are gone with the split chains.
  - `tests/fixtures/ledger/actions.ledger.jsonl` is a tenant ledger, one chain: three requests by `ui:principal:root` (1 logs on the agent, 2 restart on mitm, 3 stop on the ledger keeper, which nothing answers), then the Runner's results, 4 answering 1 (done, exit 0) and 5 answering 2 (failed, exit 1), in the Runner's member order with `source` a one-item list.
- **CLAUDE.md and entry 2** say the ledger takes an action request too, and since 0.14.1 that it is one ledger the Runner appends to as well. The binary still writes the same three files and nothing more.
- **Not added.**
  - A PowerShell wrapper (the brief asks for routes, a page and tests).
  - Paging the actions list.
  - Cancelling a request.
  - A pending mark on the graph's node itself.
- **Coverage.**
  - **Go:**
    - the three operations and the page against the spec;
    - the ask's 401, 403 for a reader and a legacy token, 422 for eight bad asks, and 400 for a body naming a principal;
    - the principal from the token, and the line on disk member for member, chained;
    - the join: done, failed and the gap, edges, one request, 404, both entry pages;
    - a layer's result answering nothing (0.14.1: a ledger layer's, and an alert layer's copy);
    - a restart reading request lines among pushes, and a bad request line stopping the boot;
    - the menu's containers being the API's subjects over the real environment layer, with every other `env:` node refused.
  - **Pester (tag `Actions`):**
    - the ask over HTTP and the line on disk;
    - the join;
    - the page's rows, colours, links and row click;
    - the entry pages both ways, with the output in a `pre`;
    - the menu on a container (and not on compose or an image), Logs at once, Restart cancelled, Stop confirmed, and the notice's link;
    - greyed for anonymous on an Instances row.

    The Environments graph block follows the container's three new items.
- **Version 0.14.0:** new operations, a Kind, edges and a page, and a line kind in the ledger.

## 51. One ledger, one seq space (0.14.1)

Portal.Agent's final shape, now committed (Runner 0.2.0, its design 9 and 10): `action.request` and `action.result` both live in `tenants/<t>/ledger.jsonl` as one chain, appended by Portal and by the Runner with the same lock, `seq` and `prev`. The agent ledger layer carries no action lines. 0.14.1 undoes 0.14.0's split chains (entry 50).

What was asked, as built:
- **A request is served under its own id**, `ledger:<seq>`. `ui:ledger:portal/<seq>` and `lineId` are dropped.
- **The loader accepts `action.result` lines** (`tenant.decodeResult`): `kind`, `id`, `request`, `status`, `started`, `finished`, `exit`, `output`, `source`, `seq`, `prev`, and nothing else. Results are read from `ledger.jsonl`, never from the mounted agent layer or any copy. The copies-only rule for alert layers is unchanged.
- **`record` re-reads the tail under the file lock** before computing `seq` and `prev`. Go's `TestLinesAppendedAfterBootAreReadAndChainedTo` appends a line between boot and record; `TestRecordWaitsForTheRunnersLock` holds the lock while an ask waits.
- **Reads see lines appended after boot.** `Tenant.Ledger` and `Tenant.LedgerLines` read the tail first, so the ledger page, the actions page and the join see the Runner's lines without a restart.
- **The entryKind note** (entry 50's "Not read yet from a real layer") is removed, since results no longer come from that layer.
- **Id reuse after a reset** is no problem on one chain, since a reset drops both sides (entry 50's join).

Choices made without asking:
- **`source` is a one-item list, or the one object.** The brief names a one-item list. The committed Runner (`Add-ActionResult`, and its tests) writes one object `{emitter, version, target, stamp}`. Portal reads either as the list of one, with all four members set and no others, so the Runner's real output boots. A list of any other length is refused. Portal.Agent may want to settle the shape; Portal takes both until then.
- **Every line of `ledger.jsonl` is `ledger:<position>`**, pushes included, as the Runner's `Read-TenantLedger` numbers them. An action line whose own id names another seq is served at its position, with `written` saying the seq its id names, and so is a `seq-gap` break (entry 45). The served id therefore always names its line.
- **The agent chain's ids are `ledger:<seq>` too**, in another seq space: `ledger:5` names one line in each chain, told apart by `source`. No route serves both chains in one envelope. An alert's `about` a `ledger:<seq>` still resolves to the agent chain (entry 47), and Alerts 0.5.0's `action-failed` draws no `about` edge, for this reason (its design 11). Telling the two apart by id is still open, for Graph.Node and Portal.
- **The lock is the Runner's.** The Runner opens the file with `FileShare.None`. Portal's `openLocked` opens it with no sharing on Windows (`CreateFile`, share mode 0), retrying a sharing violation every 50 ms for up to 30 s, as the Runner does; on Unix it takes `flock(LOCK_EX)`, which is what .NET does there for `FileShare.None`. While either holds the file, the other waits.
- **Reads take no file lock.** A read opens the file shared and takes the lines after the last one it read, up to the last LF, so a line being written is left for the next read. On Windows, while the Runner holds the file, the open fails and the ledger is served as last read.
- **`record` ends a torn last line.** Under the lock nothing is being written, so a last line without its LF is a line. Portal writes an LF before its own line when the file does not end in one, as the Runner does.
- **A reset is a file shorter than what was read, or no file.** It is read again from the start. A file rewritten to the same length or longer is not noticed until restart.
- **A bad line appended after boot** stops `record` with the line's error (500), and the reads stop before it. Only the boot stops the binary.
- **A result answers only a request before it** in the chain. The Runner's guard (its design 10) makes that the only kind it writes.
- **The entry page of a result** shows `written by` (emitter, version, tenant, stamp) and `line` (`ledger.jsonl:<n>`). The chain's list shows `<status> · answers <request>` and the emitter.
- **The ledger layer fixture** is entry 45's again (12 entries, 44 to 55), as committed in 0.10.0. The tenant fixture gains the two results.
- **Version 0.14.1:** no operation, Kind or edge changes. `LedgerEntry` loses `lineId` and gains `writer`, and the portal chain's ids change.

## 52. Card files with non-ASCII names (0.14.2)

Portal.Emitters' `Push-AllCards` pushed Emitter.Terraform and was refused with 422: `files/156 "=?utf-8?B?dGVzdHMvZml4dHVyZXMvaGNsL2VuYy3ml6XmnKwvbWFpbi50Zg==?=" must match ^[A-Za-z0-9_][A-Za-z0-9._ -]*$`. The path is `tests/fixtures/hcl/enc-日本/main.tf`. .NET's multipart, which PowerShell uses, sends a non-ASCII filename as RFC 2047 encoded words, and Go's `mime.ParseMediaType` hands them on undecoded. Even decoded, the segment `enc-日本` failed the ASCII rule.

What was asked, as built:
- **The push endpoint decodes encoded-word filenames** (`server.filenameOf`) before the card is checked. RFC 2231's `filename*` was already decoded by `mime`, and a raw UTF-8 filename is taken as sent.
- **A segment may hold Unicode letters and digits** (`feed.segment`, now `^[\p{L}\p{Nd}_][\p{L}\p{M}\p{Nd}._ -]*$`). It still starts with a letter, digit or underscore, and the separators are still dot, space and hyphen. Control characters, `.`, `..` and empty segments are still refused.
- **Tests.** Go's `TestNonASCIIFilenamesArePushedAsTheirPath` pushes that exact encoded name and the raw UTF-8 path, and checks the card's hash against the decoded path. `TestNewCardTakesUnicodeSegments` lists what is taken and what is refused. Pester's Feeds block pushes a card holding that file with `Publish-PortalCard`, and checks that .NET puts that encoded word on the wire. Manual check 1.24 runs the same push.

Choices made without asking:
- **Only a filename that is wholly encoded words is decoded**: one that starts with `=?` and ends with `?=`, which is what .NET sends. A filename that only contains `=?...?=` somewhere is taken as sent. The segment rule refuses `=` and `?` anyway.
- **A malformed encoded word stays as sent**, as `mime.WordDecoder` leaves it, so the segment rule refuses it with 422 and names the file. A charset Go cannot decode (anything but UTF-8, ISO-8859-1 and US-ASCII) is a 400, since the body cannot be read as a card.
- **"Digits" means decimal digits (`\p{Nd}`)**, any script's. Superscripts, circled numbers and Roman numerals (`\p{No}`, `\p{Nl}`) are not taken.
- **Combining marks (`\p{M}`) are taken after the first character.** A decomposed name (macOS writes `é` as `e` and U+0301) or an Indic script needs them, and a mark cannot stand first.
- **No Unicode normalisation.** A path is pushed as its bytes, so NFC and NFD spellings of one name are two paths. The duplicate check is still `strings.ToLower`.
- **Other Unicode spaces and format characters are refused**, such as U+3000 and U+200B, and so are punctuation and symbols other than `_ . -` and space. A path that is not UTF-8 is refused by name ("is not UTF-8").
- **No feed change.** Go's `archive/zip` sets the UTF-8 flag on a non-ASCII entry name, so a `.nupkg` holds the path. An OCI layer carries the path in its `org.opencontainers.image.title` annotation, which is a JSON string.
- **Version 0.14.2:** no operation, Kind or edge changes. pushCard's summary states the decoding and the segment rule.

## 53. The tenant graph, filtered by source: Show graph from the Catalogue (0.15.0)

The brief: on Emitters/Catalogue, a right-click on a card row opens Show graph, the tenant Graph page filtered to the nodes whose `source.emitter` is the card's name. Link-first: the filter is a URL, `/{t}/graph?source=<name>`, never UI state. `GET /api/tenants/{t}/graph` takes the same `source`, exact and case-sensitive, and keeps an edge only when both its ends are kept. The active filter is a chip whose X links to `/{t}/graph`. No match is the normal empty state, with the chip. The Catalogue gets a `nodes` column from the same filter, and the card's page a Show graph link near the version.

The brief assumed three things the code did not have. There was no `GET /api/tenants/{t}/graph`; the nearest, `.../environments/graph`, serves `environment.*` nodes only. There was no page at `/{t}/graph`: the Graph service's pages are `/{t}/graph/<page>`, and its Explore is a screen of Canvas widgets bound to one layer each. And a node has no `source.emitter`: in Graph.Node a node's `source` is a list of `{tool, command, path, file, line}`, and the `{emitter, version, target, stamp}` block (Graph.Node design 20) is the envelope's, one per layer. Jerry's answer: build a new tenant graph. It is one envelope of every mounted layer's nodes and edges, and `source` keeps the layers whose envelope `source.emitter` matches. It is drawn on a new Graph page at `/{t}/graph`, and Explore keeps `/{t}/graph/explore`.

What was asked, as built:
- **`GET /api/tenants/{tenant}/graph`** (`getTenantGraph`, tag `graph`, `store.TenantGraph`) returns module Portal with no ontology, so its nodes keep their layers' Kinds. Every node of every mounted layer is in it; the first id wins in mount order, and a node with no stamp gets `stamp.layer`, as `listNodes` serves them. An edge of a mounted layer is in it when both its ends are, an edge id seen twice once.
- **`?source=<name>`** reads only the layers whose source block has `emitter` equal to the name. The match is exact and case-sensitive (`emitter.environment` matches nothing), and a layer with no source block never matches. Their nodes are kept, and an edge only when both its ends are: the ledger fixture's nine `records` edges into Emitter.Agent's run are in the whole graph and not in `?source=Emitter.Ledger`. No match is 200 with no nodes, never 404. An empty `source=` is no filter.
- **`/{tenant}/graph`** (`getGraphPage`) serves the shell. Its `source` query parameter is declared on the operation, like the API's (`components/parameters/GraphSource`). The page is Graph/Nodes (`{ id: nodes, path: graph/nodes, view: graph }`, second under Graph). `web/graph.js` draws it with the vendored Cytoscape, as the Canvas draws a layer. The toolbar holds `N nodes · M edges` (`#graph-stats`) and, with a filter, the chip `source: <name>` whose X is a link to `/{t}/graph` that routes in the shell. No nodes is the empty state ("No nodes"; with a filter, "No mounted layer's source block names <name>."), never an error card.
- **Catalogue.** A new last column, `nodes`, is the roll call's new `nodes` property: how many nodes `getTenantGraph` serves with `source` set to the emitter's name (`emitter.nodeCount`, which calls `store.TenantGraph`). 0 shows as 0. A right-click (or Shift+F10) on a row opens the node menu (`nodemenu.open`, the container menu's code, with its own items as alerts.js does): Show graph → `/{t}/graph?source=<name>`, Open card → `/{t}/emitters/<name>`.
- **The emitter's page** has Show graph as a plain link (`a.pt-show-graph`) after the version, in its `dd`. Its title gives the node count.
- **Tests.** Go: `TestTenantGraphFiltersBySourceEmitter` covers the whole graph, a match (Emitter.Ledger, Emitter.Environment, Emitter.Agent), edge pruning, no match (a card with no layer, the wrong case, an unstamped module) and the roll call's `nodes`. The spec walk adds six calls. Pester's Emitters block covers the `nodes` column in the row regexes, the counts against the API, the menu's hrefs and greyed items, a click through Show graph to the chip and its X, the empty state for Emitter.Terraform, and the Show graph links on the emitter pages.

Why the filter keys on the emitter's name, not on an id prefix:
- **One emitter can own several prefixes.** Emitter.Environment's two layers hold `env:compose/`, `env:container/`, `env:image/`, `env:network/` and `alert:` ids. Emitter.Agent's hold `agent:run/` and its steps, actions and decisions under it. A prefix filter would need a list per emitter, and that list is not written anywhere Portal reads.
- **One prefix can be written by several emitters, or by none.** `ledger:<seq>` is Emitter.Ledger's agent chain and also Portal's own ledger lines (entry 51). A layer may also point at another's ids across a cross-layer edge. A prefix says what shape an id has, not which emitter wrote it.
- **The prefix registry names a module, not a card.** Graph.Node registers `env:` to Claude.Agent.Orchestrator. Getting from that to the card Emitter.Environment is the name mapping entry 49 removed.
- **The source block is the emitter's own statement.** It is what the roll call already matches a layer to its card by (entry 49), so the Catalogue's `layers`, its `nodes` and Show graph agree, and an unstamped layer is unmatched in all three.

Choices made without asking:
- **The source block is per layer, so the filter is per layer.** A node is in `?source=X` when a layer stamped X holds it, even if an earlier layer holds the same id. In the whole graph, that id is served from the earlier layer.
- **An alert layer's copies of its subjects are left out, and its alerts kept**, as `listNodes` does. An alert's `about` and `raised-in` edges survive only when their ends come from a non-alert layer of the same filter.
- **The whole graph has unstamped layers in it.** Only the filter needs a source block.
- **An unstamped module's row** shows `nodes` 0 and greys Show graph ("unstamped: its layers carry no source block, so no source filter matches them"). Its name, as a source, would match a stamped emitter of the same name, if there were one, and not its own layers. Its emitter page has no Show graph.
- **"Open card" is the emitter's page** (`/{t}/emitters/<name>`, built from the card, entry 49), the only page per card; the card's row on Artifacts/Feeds stays its card link. It is greyed for a no-card name and an unstamped module ("no feed of the tenant holds a card of this name", "unstamped: no card is matched to a module"). Show graph still links for a no-card name, and its page has Show graph beside its name, since its layers are stamped.
- **The page is Graph/Nodes**, second under Graph after Explore. The `/` redirect, Explore's address and the other pages keep their paths. No boot refusal: `/{t}/graph` is two segments, so no shell route competes. A menu without a `view: graph` page gives no Show graph (the item is greyed, "the menu has no tenant graph", and the page has no link).
- **Colour and shape** come from the mounted layers' ontologies merged into one, the first Kind wins in mount order, through the Canvas's own `styleFor` (now exported, with `splitAlpha`); a Kind with no display is grey. The page sets no display (entries 5, 22).
- **Node clicks do nothing yet.** The brief asked for a filtered view, not for selecting or opening a node. Positions are the viewer's (entry 11).
- **Version 0.15.0:** two new operations (`getTenantGraph`, `getGraphPage`), property `nodes` on Card and Emitter, view `graph` in the config schema, page Graph/Nodes. No Kind or edge Kind is added.

## 54. The tenant graph query: one walk every graph page can call (0.16.0)

The brief: one API that answers multi-hop questions across every mounted layer of a tenant, in memory, breadth-first, returned as a normal envelope. `GET /api/tenants/{t}/query` takes `from` (ids, comma-separated) or `kind`, `follow`, `direction`, `depth`, `until`, `where` (repeatable), `source` and `limit`, and returns nodes, edges, `paths` and `query` (the parameters as resolved), with `truncated` and `truncatedAt` when the cap is hit. Empty is 200, an unknown `from` 400 with the id named. The tenant graph's `?source=` becomes one case of it. No store change: the in-memory maps stay the store, and the query is the contract a SQLite or graph-store adapter sits behind later. Graph/Explore gets a query bar whose fields are the URL, its result drawn with the tenant graph's renderer and its paths listed under it; a right-click on any node of any graph page offers Query from here.

What was asked, as built:
- **`queryTenantGraph`** (tag `graph`, `internal/server/graph.go`, `store.RunWalk` in `internal/store/query.go`). Starts are the `from` ids, or every node of `kind`. The walk goes one hop at a time for every start together, each start with its own visited set, so each node is expanded once per start and an edge followed at most once per direction. `follow` names the edge kinds walked (absent or empty, all). `direction` is `out` (the default), `in` or `both`. `depth` is 0 to 10, 3 when absent. A node of Kind `until` is kept and not expanded. A node enters the result only when every `where` matches it, and only then is it expanded. `limit` is 1 to 5000, 500 when absent: the walk stops when the next node would pass it, with `truncated: true` and `truncatedAt` the hop it was on (0 when the starts alone pass it). The result's edges are every edge between two of its nodes.
- **`source`** keeps the layers whose source block names that emitter, entry 53's rule, before the walk: a `from` id outside them is unknown (400).
- **The tenant graph is the query.** `store.TenantGraph(source)` is `GraphQuery(Walk{Source: source, Depth: 0})`: no `from` and no `kind` starts at every node, which only the store may ask (the API refuses it, 400). Its nodes and edges are the ones 0.15.0 served, and `TestTenantGraphFiltersBySourceEmitter` runs unchanged. One difference no test or page sees: edges come out in the order of their `from` node's first appearance, not layer by layer.
- **Graph/Explore's query bar.** The Page gets `query: true` (`portal.base.yaml`; `query` on a menu item in `schema/portal.schema.json`, and on Page in `ontology.yaml`). The shell draws the bar (`web/query.js`) on top: `from`, `follow`, `direction`, `depth`, `until`. Submitting it routes to `/<t>/graph/explore?<the fields>`, in the API's order, empty fields left out; `kind`, `where`, `source` and `limit` ride along hidden as they came. While the URL holds `from` or `kind`, the result is drawn in place of the screen by `graph.js`'s `draw` (now exported). The toolbar shows `N nodes · M edges · P paths`, and the truncation as a chip. Without a query, Explore's Canvases are drawn as before, under the bar. The paths are listed under the graph, one id chain each, every id a button that selects and centres its node.
- **Query from here.** `nodemenu.items` gains a fifth item, Query from here, `/<t>/graph/explore?from=<id>`, greyed with why when no Page has `query: true`. The environments graph and the environment pages' rows have it. A Cytoscape graph (Graph/Nodes, Explore's Canvases and Explore's result) had no menu; a right-click on a node now opens one holding that item alone (`nodemenu.wireCanvas`).

Why the walk lives in Portal:
- **The question spans layers, and only Portal holds them all.** A container's network, an agent's run and the ledger line that records it come from three emitters. No emitter can answer across another's layer. Graph.Node defines and validates the envelope and holds no mounted graph. Portal already merges the mounted layers by the rule every page uses (first id wins in mount order, an alert layer's copies left out; entry 53), so the walk sees the graph the pages show.
- **One walk, not one per page.** The environments graph (entry 46), the tenant graph (53) and the node menu each walked the layers their own way. The query is the one every graph page can call; the tenant graph is already its first caller.

Why in memory:
- **The layers are already there.** Every mounted layer is held in memory read-only (entry 2); the dev tenant's eleven layers hold about 1,600 nodes. Building the adjacency per request is linear in nodes and edges, and cheaper than encoding the answer. A store would add a second copy and a write path to keep in step, against entry 2.
- **The caps keep it bounded:** depth at most 10, at most 5000 nodes, a visited set per start.

What an adapter implements: `store.Adjacency`, two calls. The walk uses nothing else.
- **`Neighbours(id, direction, follow) (node, hops, ok)`**: the node `id` names, and its hops (the edge, and the node at its other end) out of it, into it, or both, along the edge kinds in `follow`. `ok` is false when no node has the id. The walk calls it once per `from` id to resolve it, once per node it expands, and once per result node (out, every kind) for the edges between result nodes.
- **`NodesOfKind(kind)`**: every node of a Kind in a stable order; every node when `kind` is empty.
- The source filter, the first-id rule, `stamp.layer` and the alert copies are the adapter's view, as they are `memGraph`'s. `where`, `until`, depth, the cap and the paths stay in the walk.

Choices made without asking:
- **`from` and `kind` together is 400**, not an intersection: the brief has `kind` instead of `from`.
- **A start is always expanded**, even when it is of Kind `until`; otherwise `kind=X&until=X` would return the starts alone. A start must pass `where` like any node; one that fails is left out and not walked from.
- **`where`** keys are `id`, `kind` or `name`, else a dotted path into `properties` (`facet.key`; `properties.facet.key` is read the same). The match is exact against a string, or a number or boolean as JSON writes it (`n=1`, `internal=true`). A list or a map never matches. Several are ANDed.
- **A path ends at every node the walk stopped at**: `until` matched, depth ran out, or nothing new lay beyond it (a dead end, or every neighbour already reached from that start). The brief named the first two. Without the third, a walk that runs out of graph before its depth would list no path at all. A start that reaches nothing is a path of one.
- **Truncation** counts the result, across starts: a node that a second start reaches again does not count twice. The walk stops at the first node that would pass `limit`, and what was reached is served, with its edges and paths.
- **Errors.** 400 `unknown_node` for a `from` id no node has, with the message `from: no node <id> in the mounted layers`. 400 `bad_request` for no start, both kinds of start, or a bad `direction`, `depth`, `limit` or `where`. `depth` and `limit` are read from the URL to tell absent from 0, which the generated binding cannot.
- **The response** is the envelope (module Portal, no ontology) with `paths`, `query` and `truncated` beside its members, which Graph.Node's envelope schema allows (`additionalProperties: true`); `truncatedAt` only when truncated. The spec's `GraphQuery` is `allOf` the Envelope and those members.
- **Explore's URL is the query**, as `?source=` is Graph/Nodes': the bar holds no state of its own, and a query is a link. The direction select's empty choice is "out (default)", so an absent `direction` stays absent and an explicit `direction=out` stays explicit: the bar round-trips.
- **A path's ids select and centre** their node in the drawn graph rather than navigate: the result is already on screen. Query from here, on a node's right-click, is the way on. At most 500 paths are listed; the heading says how many there are.
- **The JS client** sends a list as a repeated parameter (`where`).
- **The tenant ledger is not in the walk.** `tenants/<t>/ledger.jsonl` (its action requests and results, entry 51) is no mounted layer, and its ids, `ledger:<seq>`, are the same strings as Emitter.Ledger's agent chain. Merging it into the maps would let one id stand for two lines (first id wins). The brief's third example asks for the ledger chain backwards from an action result; `docs/api/query.md` walks the mounted agent chain instead and says why. Not scheduled: serving the tenant ledger as a layer needs ids of its own first.
- **Version 0.16.0:** one new operation (`queryTenantGraph`), property `query` on Page and on a menu item, Explore's query bar and the node menu's fifth item. No Kind or edge Kind is added.

## 55. The tenant's action chain under ledger:tenant/<tenant>, mounted as tenant-actions (0.16.1)

The brief: `GET /api/tenants/default/query?from=<action result id>&depth=5` returns the chain back to the request, the subject container and the principal, with no collision against the agent ledger; Portal's action chain becomes a mounted layer like everything else. Entry 54 left the tenant ledger out of the walk because its action lines carried `ledger:<seq>`, the same strings as Emitter.Ledger's agent chain. The first attempt (2026-10-12, stop report) found that no namespaced `ledger:` form passed Graph.Node 0.4.0's id check. Graph.Node 0.4.2 (entry 29 there) added `ledger:tenant/<tenant>/<seq>`, with `ledger:tenant/<tenant>` as that chain's root. Portal repins to v0.4.2 and takes that form.

Why under `ledger:` and not a new prefix:
- **One validator rule, one card.** The lines are ledger lines, with the same writer contract, seq and prev hash, appended by Portal and the Runner under one lock (entries 50, 51). A new prefix (`action:`, say) would need its own owner, shape, registry row and emitter card for what is the same thing under a second number space. Namespacing keeps one prefix: Graph.Node checks it with one rule, and the Emitter.Ledger card describes both chains, so the roll call lists both layers under it.
- **The collision is exactly what the namespace removes.** Line 23 of the tenant ledger is `ledger:tenant/default/23`, and the agent chain's 23 stays `ledger:23`. Both can be mounted, and the query's first-id-wins rule (entry 53) no longer has two lines for one id.

What was asked, as built:
- **The repin.** `internal/graphnode`, `cmd/graphnode` and `.claude/skills/graph-node` are recopied from Graph.Node v0.4.2 (46055d5), differing only in the import path; the Pester tag, CLAUDE.md and README name v0.4.2. Portal's own `graphnode check-id ledger ledger:tenant/default/23` passes; `ledger:default/23` is refused with the four accepted shapes.
- **Ids** (`internal/tenant/actions.go`). `LedgerID(tenant, seq)` is `ledger:tenant/<tenant>/<seq>`, and `LedgerRoot(tenant)` is `ledger:tenant/<tenant>`. `LedgerSeq` reads that form, or the legacy `ledger:<seq>`, as `(tenant, seq, legacy, ok)`. A request Portal writes carries the new form. A line is read in either form. `NormalizeID` turns a legacy id into this tenant's form, which is how a result's `request` is served and joined.
- **The migration** (`internal/tenant/migrate.go`, at `tenant.Load` before the ledger is read, under the file's lock). Every action line still carrying `ledger:<seq>` gets the new id, keeping the number, and `legacyId` (the old id) right after `id`. A result's `request` is rewritten the same way. A rewritten line has new bytes, so the next line's `prev` (the SHA-256 of the line before, entry 45) is rewritten to the new hash, and so on down the file. Every other member, and the order of members, stays as written. The file is copied first to `ledger.jsonl.<UTC stamp>.bak` beside it (`.<n>.bak` when that name is taken; a copy is never overwritten). A ledger with no legacy line is not touched and gets no copy, so the migration happens once. The boot prints `ledger: N lines rewritten ...` with the copy's path.
- **The tenant-actions layer** (`internal/server/actions_layer.go`, mounted through `store.Derived`, `internal/store/derived.go`). Nodes are the ledger's lines, as `GET .../ledger/portal` serves them (LedgerEntry, joined: `outcome`, `resultId`, `requestSeq`), then each principal a request names (Principal, its node from `principals.yaml`, or a bare one when it is no longer there). Edges: `prev`, a line to the one before it (newer to older), for every chained line; `request`, a result to the request it answers; `subject`, a request to its `env:container/...`; `principal`, a request to the principal. The root is the newest line. The module is Portal; the three new edge kinds are declared in `ontology.yaml`.
- **Its source block** names Emitter.Ledger at the version the mounted agent ledger layer's source block carries (`store.AgentLedgerLayer`, the layer the agent chain is read from), so both layers are under one card and agree with it. With no agent ledger layer stamped Emitter.Ledger, it uses the version of the newest Emitter.Ledger card pushed to the tenant's feeds. With neither, it has no source block and is listed under its module, unstamped, never matched to a card. The target is the tenant, and the stamp the layer's (the newest line's time).
- **On boot and on every append.** `store.Derived` registers a builder that returns a key and a build. Every read of the mounted layers (`Layers`, `Query`, `LayerOntologies`, `NodesOfKind`) asks for the key first: the ledger's line count and last line hash, the action count, the source version and the principal count. It rebuilds and remounts only when the key has changed. Portal's own append and the Runner's are therefore in the layer on the next read, with no restart and no poller. The layer is rebuilt from the ledger at every start, which is how it survives one; it is never written to disk.
- **Actions page.** A seventh column, `id`, shows the request's id. A click on a row opens Graph/Explore at `?from=<id>&direction=out&depth=5`; `queryHref(id, more)` takes the bar's other fields. The action still links the request's entry page.
- **The contract** is `docs/api/actions.md`: the shape of both lines, the `ids` section (format, legacy form, migration), and what Portal.Agent's Runner must write. `postAction`, `listActions` and `getAction` keep every other member as before; only the ids changed.

Choices made without asking:
- **prev stays a hash.** The brief has "kind, request, prev all use the new form". `kind` is no id, and `prev` is the SHA-256 of the line before (entry 45): `GET .../ledger` verifies the chain with it, and Portal.Agent's Runner writes it the same way. Turning it into an id would break every chain check. The migration recomputes `prev` so the chain stays valid; the `prev` edge in the layer carries the id link.
- **A link that was broken stays broken.** A `prev` is rewritten only when it named the old hash of the line before it. A chain that broke at line k before the migration breaks at line k after it, rather than being silently repaired.
- **The migration runs at every boot and does work only on legacy lines.** Until Portal.Agent writes the new form, a result the Runner appends in the old form is read as this tenant's (`NormalizeID`) and served in the new form, and the next boot migrates it, with a new copy. Once the Runner writes the new form there is nothing left to migrate. A ledger with a line that is no ledger entry is not migrated: the boot stops on that line as it always has.
- **Mounted while the ledger holds an action line.** A ledger of pushes alone has no action chain to query; the layer appears with the first request. Its nodes are every line, pushes included, so `prev` is the real chain.
- **The row link walks out, not in** (asked, 2026-10-12). With the edges as specified (prev newer to older, request result to request, subject and principal request to node), `direction=in` from a result reaches only the newer lines. `out` reaches its request, the request's subject and principal, and the lines before it. Jerry chose to keep the edges and use `out` for the link and the report's query. `TestQueryFromAResultReachesItsRequestSubjectAndPrincipal` checks both directions.
- **The agent chain skips `ledger:tenant/...`.** `graph.ValidID` now accepts the tenant form, so `store.isAgentEntry` excludes it explicitly. Without that, the agent chain could be read from tenant-actions when its stamp is newer.
- **The name `tenant-actions` is Portal's.** `POST .../layers` and `-mount` refuse it with 400, and the derived build goes through `mountLayer`, which does not refuse it.
- **GET `.../layers/tenant-actions/nodes`** serves it as any layer's nodes are served: a new envelope, without the source block and without the `subject` edges that end in the environments layer. The tenant graph and the query have them.
- **The CLAUDE.md write rule.** The ledger was append-only for the binary. The one-time rewrite and its copy beside the file are now named there as the only exception, both under `tenants/<tenant>/`.
- **Version 0.16.1:** the ids, the migration, one derived layer, three edge kinds (`request`, `subject`, `principal`), property `legacyId` on LedgerEntry, the Actions page's `id` column and row link. No new operation.

## 56. Refresh: one order, derived from the repos, declared in a file (0.16.2)

The brief: one command, `Invoke-Build Refresh`, takes everything down and brings everything back up, in an order derived from the repos themselves and kept in a file in this repo so it is never guessed again. Phase 1 read every `Portal` and `Portal.*` repo (build script, README, CLAUDE.md, docs/) and wrote `docs/refresh/dependency-order.md`: what each repo does, what it depends on, the sequence, and eight conflicts, each resolved there. Phase 2 built the task on it.

Choices made without asking:
- **The order lives in two files.** `dependency-order.md` is the reasoning, and `order.psd1` is the same sequence as data. Refresh reads only the psd1. Pester fails when the doc's sequence table, its card-only rows or the repos on disk disagree with it. A data file rather than parsing Markdown, so the doc can be prose and the build stays simple.
- **The dependents' build files are `<Repo>.build.ps1`, not `.build.ps1`.** Only Portal.Agent and Portal.Emitters have one. The order names each file. Ontology, Skills, Tools and Workflows have none, are listed as `CardOnly`, and get one line saying so. Refresh warns about a `Portal.*` folder the psd1 does not name, and Pester fails on one.
- **Portal.Emitters `Mount` is called unchanged, ordered before the live push** (conflict 1). It restarts Portal twice. Running its `Emit` and doing the mount, snap and verify here would copy Portal.Emitters' code into Portal. Putting Mount before Portal.Agent's `Up` means nothing it restarts holds an unapproved layer yet. Rule R3 (no restart after a live push) and R4 (an approval after the last push) make the order checkable.
- **Six rules, checked before anything runs** (`Test-PortalRefreshOrder`): R1 phases down, up, check; R2 `portal-down` last down and `portal-dev` first up, a step that needs a portal only while one is up; R3; R4; R5 known repos, actions and phases, unique ids; R6 one catalogue, last. A broken order stops Refresh with every problem named.
- **Each invoke step runs in a child `pwsh`, its output in a log, through files.** `Start-Portal` starts the portal with `-NoNewWindow`, so the portal inherits the step's handles. A pipe it inherited would never close, and `Start-Process -Wait` waits for descendants. So `Invoke-PortalRefreshStep` redirects to files and waits on the child alone. One line per step is printed, and the log's last 20 lines on a failure.
- **The approval takes the dev token from `GET .../dev-token`** (loopback, mode dev). Every restart re-mints the dev token, and the child processes' tokens do not reach Refresh. The token is never printed. The snap's note is `Invoke-Build Refresh <stamp>`.
- **Agent `Down` is skipped without a Docker engine** (conflict 3). The lane's `Down` throws then, and there is nothing to stop.
- **`Feeds` is not a step** (conflict 5). It replaces the portal, and the Catalogue reads pushes from the ledger, not from the running registry.
- **The Catalogue check.** One row per Card or Emitter node of `GET .../emitters` (Layer nodes are not rows). It fails, naming each card, when a name that read `ok` in `artifacts/refresh/last.json` reads `no-layers` or `version-mismatch`. A name new since the last Refresh, or one not `ok` before, is no regression. `last.json` is written only when the check passes, so a regression keeps failing until it is fixed rather than being forgotten after one run. The failing roll call is `failed.json`. Both are under the gitignored `artifacts/`.
- **`-DryRun`** prints each step's plan or skip and runs nothing. It is a new parameter of `.build.ps1`, used by Refresh only.
- **No new exported function.** Refresh is a build task. Its logic is six private functions (`Get-PortalRefreshOrder`, `Test-PortalRefreshOrder`, `Invoke-PortalRefresh`, `Invoke-PortalRefreshStep`, `ConvertTo-PortalCatalogueRow`, `Compare-PortalCatalogue`), and `scripts/Refresh-Portal.ps1` runs the task from any directory.
- **Not run end to end in this task.** Refresh runs builds in the sibling repos, which this repo's CLAUDE.md never lets an agent build or run. The order, the rules and the loop are tested in Pester with the handlers replaced over a fake workspace. The child-process runner is tested on a scratch build file. `-DryRun` was run against the real workspace. The first real Refresh is the user's.
- **Pester's Refresh block has the tag `Refresh`.**
- **Version 0.16.2:** a build task, a script and a doc; no API, Kind or behaviour of the binary changed.

## 57. A portal that lost its record is still this repo's portal (0.16.3)

The 2026-10-12 Refresh stopped at step 3: `Down` printed "No portal is recorded in ...\artifacts" and passed, and `Dev` then found port 8200 held by `portal.exe~`, a portal this repo's binary had started over `artifacts/` the evening before whose `portal.json` was gone. Two weaknesses made that possible. `Stop-Portal` removed the record whether or not the stop worked (`Stop-Process` and `Wait-Process` both silenced). And nothing looked for a portal without a record, because entry 32 says stopping an unrecorded process by port would be a guess. What removed that record is not known; the fix closes every path to the state rather than one.

Choices made without asking:
- **Entry 32 is narrowed, not dropped.** A process is this repo's portal for an artifacts directory when its image is `<repo>\build\win-x64\portal.exe`, or the same with `~` (Go's build renames a binary in use on Windows; the running process then reports `portal.exe~`), compared as full paths, **and** its `-artifacts` argument is the same directory (full path, no trailing separator, case-insensitive). That is a fact read from the process, not a guess from the port. Anything else holding the port is still never stopped (`Get-PortalProcess`, private).
- **The command line is split as Windows splits it for a Go program** (CommandLineToArgvW's backslash and quote rules), so `-artifacts` reads as the portal read it. `-name value`, `--name value` and `-name=value` are all taken, as Go's flag package takes them (`ConvertFrom-PortalCommandLine`, private).
- **The choke point is the module, not the tasks.** `Stop-Portal` stops the recorded process and every unrecorded portal over the same artifacts; `Get-PortalStatus` reports an unrecorded one as running with `Recorded` false and its port from its command line. `Invoke-PortalUp` needs no change: it already asks Status and calls Stop-Portal. So Down, Up, Dev, Feeds, Refresh's steps 2 and 3, Portal.Emitters' `Restart-Portal` (it calls Down then Dev) and every Pester cleanup get it at once.
- **A recorded id is trusted only when it passes the same test.** A record whose process has exited, or whose id Windows has given to another program, no longer counts as running and is never stopped; Stop-Portal removes it and says so with `-Verbose`. Before, Stop-Portal force-stopped whatever held the recorded id.
- **The record is removed only when nothing survives.** After `Stop-Process` and a 10 s wait, a portal still running keeps `portal.json` and Stop-Portal throws naming its id and path. So Down fails instead of passing over a running portal.
- **`Start-Portal` refuses a second portal over the same artifacts, recorded or not**, with entry 32's message plus `, unrecorded` when it is. When the port is held by this repo's portal over other artifacts, the busy-port message goes on: `It is this repo's portal serving <dir>: Stop-Portal -Artifacts '<dir>' stops it.` It still stops nothing itself.
- **Refresh checks the port after Portal's own down step** (`Repo` Portal, `Phase` down): `Assert-PortalPortFree` on the port Up uses, else the step fails `FAILED after Down: <owner> (<the step's own line>)`. A Down that passes with 8200 held now fails at step 2, where the problem is. The check is a handler, `portfree`, so Pester replaces it.
- **`Build` first removes `build/win-x64/*.exe~` that no process runs.** One in use is left, silently: Up's Build runs while the old portal still runs, so a warning there would fire on every replace.
- **`Status` says `running (process <id>, unrecorded)`** for such a portal.
- **Not covered:** a portal a crashed Pester run left on 18200 over a TestDrive directory already deleted. Its `-artifacts` matches no directory anyone passes, so it is found only by `Start-Portal`'s message naming it.
- **Version 0.16.3:** module behaviour only; no API, Kind or binary change. The spec's version moves, so `internal/gen` is regenerated.

## 58. Graph interaction defaults live in tenant config; per-page overrides live in the browser, never the server (0.16.4)

The 2026-10-13 brief: mouse-wheel zoom on every graph canvas runs at a speed set in tenant config, `ui.graph.wheelSpeed` (default 1.0, range 0.1 to 5.0), and each graph page has a small control at the top right of its toolbar, labelled Wheel, that changes the speed for that page. The override is kept in the browser's localStorage under `portal:<tenant>:<page>:wheelSpeed`, and Reset clears it. Nothing is hard-coded in JS, and at 1.0 the wheel behaves exactly as it does today.

The rule this entry records: **graph interaction defaults live in tenant config; per-page overrides live in the browser, never the server.** A default is the tenant's, validated at boot, served by `GET .../config`, and changed by editing `tenants/<tenant>/portal.yaml`. One viewer's preference for one page is that viewer's pointer, like a Canvas selection or the collapsed rail (entry 11). It is never written, never in a diff or a snap, and never sent to the server.

Choices made without asking:
- **The key is `theme.graph.wheelZoomSpeed`, not a new `ui.graph.wheelSpeed`.** 0.4.1 already shipped this exact setting (entry 37): default 1.0 in `portal.base.yaml`, range 0.1 to 5 in `schema/portal.schema.json` and in the spec's `Config`, a tenant may override it, `GET .../config` serves it, and the Canvas scales its wheel step by it. A second key for the same setting would make a tenant set it twice, or set one and wonder why the other wins. Renaming the key would break every overlay that sets it. So the brief's "add the key and expose it" was already done. What 0.16.4 adds is the per-page control, and the key reaching every graph page, not only the Canvas. The schema, spec and base descriptions now say "every graph page". No spec operation changes.
- **"Every graph canvas" means every Cytoscape graph:** Graph/Nodes (`graph.js`), Graph/Explore's Canvases (`canvas.js`) and Explore's query result (`query.js`, drawn by `graph.js`'s `draw`). Environments/Graph is SVG with no wheel zoom (entry 46). It gets no control, since there is nothing for one to change; wheel zoom there would be a feature of its own.
- **One module, `web/wheel-speed.js`.** The product's step, Cytoscape's `wheelSensitivity` 0.2 at speed 1, moved there from `canvas.js` and `graph.js`, which each held a copy. The page's speed is the override when present and valid, else the config's value clamped to 0.1..5, else 1. A Cytoscape graph is made with `sensitivity()` and registers with `track(cy, host)`. `track` records `data-wheel-sensitivity` on the host, now on Graph/Nodes and query results too, and puts the control in the toolbar. Each renderer's `destroy` untracks. At speed 1 the step is exactly 0.2, as before.
- **The step and the range are still constants in JS.** 0.2 is the product's step that speed 1 means, not a setting. 0.1 and 5 repeat the schema's range so the slider and the clamp agree with the boot check. The speed itself comes only from config or the viewer.
- **Changed in place, not redrawn.** Cytoscape 3.34.3 reads its renderer's `wheelSensitivity` on every wheel event, so the control sets `cy.renderer().wheelSensitivity` on every graph the page tracks. The layout, zoom and pan stay where the viewer left them.
- **`<page>` is the menu page's path** (`graph/nodes`, `graph/explore`), so the key reads `portal:default:graph/explore:wheelSpeed`. Explore with and without a query is one page with one speed. A page that is not a menu page (a layer's, an agent's, an item's) shows no control and keeps the config's speed. The shell calls `use({tenant, id, config, toolbar})` in `drawPage`, so a page reads its override fresh each time it opens.
- **Storage is best effort.** Every localStorage read and write is in try/catch. A value that is absent, not a number, or outside 0.1..5 is ignored, and the config's speed applies. When storage throws (a private window, blocked site data), a change still holds for that page view, then is gone. Reset removes the key. Moving the slider back to the default keeps the key until Reset, so the control's `data-from` says `browser`.
- **Placement.** The control is appended to the page's toolbar with `order: 1; margin-left: auto`. It sits at the right, after everything else whatever the DOM order, and it never moves the stats, the source chip or the truncated chip, which keep their places on the left. Explore's query bar is in the content, not the toolbar, so it is untouched. In a narrow window the toolbar wraps as it already did. The control is a range input (0.1 to 5, step 0.1), an output (`1.0×`) and a reset button (`↺`), disabled while the page has no override and titled with the tenant default. It is not a write, so it is live for anonymous viewers too.
- **The tenant files.** `tenants/default/portal.yaml` now names `theme.graph.wheelZoomSpeed: 1.0`, with a one-line comment, and is left unstaged, as the brief says (it is mode dev). There is no demo tenant config and no tenant template in this repo. A fresh tenant is any directory with a `portal.yaml`, even `{}`, merged over `portal.base.yaml`, which already carries the key at 1.0 and now has its one-line comment too. So nothing else was added or staged for them.
- **Coverage.** Go `TestWheelZoomSpeed` adds the literal `5.0` (accepted) and `0.05` (refused at `/theme/graph/wheelZoomSpeed`). `TestBadOverlayFailsBoot` already refused 0.05 and 10. Pester adds four tests:
  - `tenants/default/portal.yaml` names the key in range, and `GET .../config` serves the same value.
  - Every `web/*.js` that calls `cytoscape({` is `canvas.js` or `graph.js`, imports `sensitivity` and `track` from `wheel-speed.js` and uses both. `query.js` draws through `graph.js`. `/static/wheel-speed.js` is served as JavaScript.
  - Headless Chrome finds the control last in the toolbar of Graph/Nodes, Explore and an Explore query, at `data-wheel-speed="1"` from config, Reset disabled.
  - Driven over DevTools on Explore: the slider at 2.5 writes the key, and moves the Canvas's step and its live renderer to 0.5. An invalid `9` stored for Nodes falls back to 0.2. Back on Explore the override is read again. Reset removes the key and returns to 0.2.

  `Invoke-Build Smoke` adds `checks.wheelSpeed`. It loads every menu page that draws a Cytoscape graph (view `graph`, or a screen whose node is-a Canvas) and fails unless the control is there at the config's speed and the graph took the matching step.
- **Version 0.16.4** in `Portal.psd1`, `ontology.yaml` (no Kind changed), `ontology.go` and `api/openapi.yaml` (the `Config` description only); `internal/gen` is regenerated.
