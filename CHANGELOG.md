# Changelog

## 0.3.2 — 2026-10-09

- `git-guard` skill (0.1.0) copied read-only from Graph's committed HEAD (0.2.8) and wired in a new `.claude/settings.json` as a PreToolUse hook on the Bash and PowerShell tools: an agent's commit, push, tag, `--amend`, `gh pr merge` and `gh release create` are refused (docs/design.md 30).
- `run-report` skill recopied at Graph's committed 0.2.2: the report is written only to this repo's `artifacts/run-report.json`, never to `C:\__Code\Graph\artifacts`. Portal's `.build.ps1` has no Report task and no doc said otherwise, so nothing else needed correcting.
- Pester: both copied skills match Graph's committed copy (`version:` line and full text of every file, no extra files); `.claude/settings.json` holds the git-guard entry once; the hook exits 0, 0, 2, 2, 2, 2 for status, add, commit, push, tag and `gh pr merge`. Go tests 30, Pester 54.

## 0.3.1 — 2026-10-09

- Designer sections collapsible: Approve, Kinds (ontology.yaml), Categories, Add Panel, Add Container, Add Category and Add Menu Entry each have a head row (heading left, the panels' `collapse`/`expand` button right) and a body that collapsing hides. Open or closed is a State node per section, `ui:state:designer:<slug>`, joined from `ui:designer` by `has-state`, so it is in the graph and in snaps. The seed has all seven, Approve and Categories open, the rest collapsed (18 nodes, 15 edges). A snap without them boots with them added at those defaults. A toggle writes the State and updates the section in place without redrawing the page. The page reads a node's own State by id, since the Designer now holds eight (docs/design.md 28).
- Title Case in the Designer: every heading and button, each button saying its section (`Approve`, `Add Panel`, `Add Container`, `Add Category`, `Add Menu Entry`); field labels stay lower case.
- Canvas fit: `cy.resize()` and `cy.fit()` on the next frame after layout and from a debounced `ResizeObserver` on the panel body (which also fires when a collapsed panel expands), only once the container has a size. Smoke loads the page in headless Chrome and fails unless the drawn graph is wider than half its panel: 662 of 903 px on the fixture (docs/design.md 29).
- `ontology.yaml` 0.3.1: State's description and idRule name the section States (prose only). Go tests 30, Pester 46. Screenshot refreshed.

## 0.3.0 — 2026-10-09

- Fixed port 8200: `Start-Portal` and `Invoke-Build Up` take `-Port`, else `PORTAL_PORT` from `.env` (`.env.example` at the root; `.env` gitignored), else the environment, else 8200. Port 0 is refused by the wrappers and by the binary (`-port` now defaults to 8200); a busy port fails naming the port and its owning process, and another is never picked. Tests, Smoke and Screenshot use 18200. New `Invoke-Build Status` and `Get-PortalStatus`: port, URL and boot stamp (docs/design.md 24).
- Menu as nodes: a seeded `Menu` Category; a menu entry is a Container that `is-a` Menu and `opens` the Container or Widget it shows (new edge Kind `opens`). The seed's one entry, "Graph" (`ui:menu:graph`), opens the filesystem Canvas `ui:canvas`. The top bar draws the entries from `GET /api/screen`, knowing only the name Menu; choosing one expands and scrolls to its target. The Designer's "Add menu entry" form is one node and two edges through the existing POSTs, shows in the top bar without a reload, and Approve snaps it (docs/design.md 25).
- Designer lists refresh after every add from `GET /api/screen`: Categories lists every Category as a tree by refines depth, the category and refines dropdowns follow it, and the panel list offers only Categories that reach a renderer. Pinned by a Go test: a Category added refining Canvas is on the screen with its `is-a` edge (docs/design.md 26).
- `run-report` skill recopied from Graph's committed 0.2.3 (skill `version: 0.2.1`); Pester now compares the copy's version and files with Graph's committed skill, not Graph's ModuleVersion (docs/design.md 27).
- Smoke: seed, add a menu entry, approve, Theme opacity, approve, restart, and the entry is in the restarted top bar (headless Chrome `--dump-dom`). design.md 24–27.

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
