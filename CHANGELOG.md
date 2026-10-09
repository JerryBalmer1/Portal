# Changelog

## 0.2.0 — 2026-10-09

- Renamed to `Portal` everywhere: Go module `github.com/JerryBalmer1/Portal`, envelope and ontology `module`, spec and page title, docs, skills. PowerShell module `src/Portal` with `Start-Portal`, `Stop-Portal`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`; none clashes with the Graph module (docs/design.md 18).
- Graph.Node lifted again at tag `v0.2.0` (ee4df2c): schemas, `graph` (now with `ids.go` and `ontology.go`), `cmd/graphnode` (adds `prefixes`, `check-id`, `validate --ontology`). The copy's id registry names `Portal` as the owner of `ui:`. Pester compares the lift, the `graph-node` skill (0.2.0) and `tests/fixtures/invalid.graph.json` with the tag (docs/design.md 4).
- Problems carry Graph.Node's `severity`; only errors refuse a write or a mount, and a dangling edge on screen stays an error whatever its prefix (docs/design.md 19). New Graph.Node code `edgeid` for a malformed edge id. The Category-without-properties gap is still Portal's own check, noted for Graph.Node 0.2.1 (docs/design.md 10).
- Boot order: the newest `artifacts/snaps/*.graph.json`, or the one `PORTAL_SNAP=<stamp>` names, else the dev seed. `GET /api/screen` answers `X-Portal-Boot` with the stamp, `seed` or `empty`; `Start-Portal` records it as `Boot` and takes `-Snap`. The screen lives in memory; `artifacts/state/` is no longer written (docs/design.md 20).
- Approve: the Designer's Approve form calls `POST /api/snap` with `{note}`; the snap carries the note as an `approved` envelope finding, and the response adds `note`, `previous` and a `diff` summary against the previous snap. `New-PortalSnap -Note` (docs/design.md 21).
- The Canvas draws Graph.Node 0.2.0 display: `circle`, `square`, `size`, edge `style` (docs/design.md 19).
- Theme Kind (`color`, `opacity`, `font`) joined from a Container by `themed-by`; the seed's `ui:theme:default` on `ui:root` holds base.css's values; the page applies a Theme as the stylesheet tokens of its Container. Theme is a display Kind with State (docs/design.md 5, 22).
- Removed (docs/design.md 23): linux-x64 build output and target; the live state file; the `X-Portal-Mode` header; the `-host` and `-dev` flags; the top bar's snap button; `web.StaticNames`; `vendor/LICENSE` from the embed; an unused CSS rule.
- Smoke: boot from seed, approve, change the Theme's opacity, approve, restart, and assert the restart booted from the second stamp and that the second approval's diff is exactly the Theme node.
- `run-report` skill recopied at Graph's committed 0.2.2. design.md 18–23.

## 0.1.0 — 2026-10-09

- Principle (docs/design.md 1): everything on screen is a graph in the Graph.Node envelope; no UI model of its own; new panel types are Category nodes with `is-a` edges.
- `ontology.yaml`, module `Portal` (its 0.2.0 name), prefix `ui:`: Kinds Container, Widget, Layout, Category, State, Ref; edges `contains`, `laid-out-by`, `is-a`, `displays`, `has-state`; finding Kind `absent`. Passes `graphnode validate-ontology`.
- `api/openapi.yaml` and the server `cmd/portal` generated from it (oapi-codegen v2.8.0, chi v5.3.2): `GET /api/screen`; `POST /api/nodes`, `POST /api/edges`; `PATCH`, `DELETE /api/nodes/{id}`; `GET`, `POST /api/layers`; `GET /api/layers/{name}/nodes`; `POST /api/snap`; `GET /api/diff`; `GET /api/ontology`; `GET /` and `GET /static/{file}` for the page.
- Every write validates the whole next screen with Graph.Node and the touched elements with the portal ontology; 422 with problem codes. Display properties are refused on everything but State nodes.
- Data layers mount read-only from a path or an envelope and are refused with Graph.Node's problem codes when they fail; never written.
- Snaps under `artifacts/snaps/` with millisecond UTC stamps; the diff is Graph.Node design 11's merge, read-only.
- `PORTAL_MODE=production` hides nodes that `is-a` Designer and nothing else.
- Page (`web/`): draws the screen recursively by Layout (grid, flow, canvas), each panel by the base Category its `is-a` chain reaches: Canvas (GraphRenderer's Cytoscape drawing, colour from the layer ontology, grey fallback, label templates), Table, Detail, Designer (lists Kinds, adds panels, containers and Categories through the API). Collapse writes State.
- Dev seed on an empty state: root Container, grid Layout, four Categories, a Canvas Widget on layer `filesystem`, the Designer Container. `filesystem` is the newest Graph.Emitter.FileSystem layer, mounted at startup when present; a copy is in `tests/fixtures/layers/filesystem/`.
- Graph.Node 0.1.0 schemas, `graph` package and `cmd/graphnode` lifted into `internal/graphnode` and `cmd/graphnode`; Pester compares them with Graph.Node's committed HEAD.
- PowerShell module (0.2.0 names: `Portal`, `Start-Portal`, `Stop-Portal`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`): start, stop, read the screen, snap, diff, mount a layer.
- Invoke-Build: Modules, Generate, Build, Test, Up, Down, Smoke (writes `artifacts/smoke-report.json`), Pester, Screenshot; default Modules, Build, Test, Smoke, Pester.
- Skills: `manual-check-list`, `ontology-doc`, `readme` (adapted from Graph.Node's), `graph-node` (Graph.Node 0.1.0) and `run-report` (Graph 0.2.0) copied read-only with version-match tests.
- design.md 1–17.
