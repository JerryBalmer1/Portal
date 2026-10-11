# Changelog

## 0.4.1 — 2026-10-10

- Read-only page for an anonymous viewer. While whoami says the principal may not write, every control that would write is disabled with the tooltip `sign in to write`: a panel's collapse/expand, the Designer's section toggles, Approve, Add Panel, Add Container, Add Category and Add Menu Entry. A write the portal still refuses (401, or any refusal) is a toast at the bottom right (`web/toast.js`), never an error body inside a panel; the collapse toggle used to print the error JSON into the panel, and the Designer's status line printed refusals. A refused collapse no longer redraws the page (docs/design.md 37).
- Config `theme.graph.wheelZoomSpeed` (default 1.0 in `portal.base.yaml`, 0.1 to 5 in `schema/portal.schema.json` and the spec's `Config`). A tenant overlay may override it; a value outside the range stops the binary. The Canvas sets Cytoscape's `wheelSensitivity` to 0.2 × the speed (docs/design.md 37).
- `theme` is now a known top-level config key, so the bad-overlay example in tests and manual item 1.11 is `colours: dark`. New manual items 1.12 and 2.6.
- Go tests 47, Pester 69.

## 0.4.0 — 2026-10-10

- The page is a DevOps-style shell. The banner holds the product, a breadcrumb that mirrors the URL, search, the marker (`none`), help and the avatar. The left rail holds the services: a click opens a service's pages in place and closes the others, a hover on a closed service shows a flyout, the chevron collapses the rail to icons, and Settings is pinned at the foot. The main pane has a title, a toolbar and the content. Routes are `/<tenant>/<service>/<page>[/<id>]` and deep links survive a refresh. Dark theme: cards on a near-black grid (docs/design.md 36).
- Config split. `portal.base.yaml` (embedded) holds product defaults and the menu; `tenants/<tenant>/portal.yaml` is the tenant's overlay, deep-merged over it. Both are validated against `schema/portal.schema.json` at boot, and a failure stops the binary. `PORTAL_TENANT` (default `default`) and `-tenants` say which tenant a process serves.
- API: every path is `/api/tenants/{tenant}/...`. New: `config`, `menu` (an envelope of the new Kinds `Service` and `Page`, edge `has-page`, icons in State `icon`), `auth/whoami`, `connections`, `connections/{name}/repos` (GitHub, as `git.repository` nodes; needs a token though it is a GET) and `nodes?kind=` (one Kind across the screen and the mounted layers). `/` redirects to the first page. The unscoped `POST /api/layers` is kept, deprecated, for Claude.Agent.
- Bearer tokens. `tenants/<tenant>/tokens.yaml` (gitignored) holds SHA-256 hashes. No token reads only, a write without one is 401, and a token the tenant does not hold is 401 on any request. New `New-PortalToken`. The module sends `PORTAL_TOKEN`, and the page takes a token in the avatar panel.
- Service connections in `tenants/<tenant>/connections.yaml` (gitignored; example committed). Settings/Service connections lists them without secrets and has a Test button per connection.
- Every menu leaf is a page. Table pages list their Kind or show an empty state. Graph/Explore draws the Canvas with its layer's counts in the toolbar. Designer/Sections and Designer/Export draw the Designer. The top bar's screen menu and stats are gone. Screenshots are now `summary.png`, `service-connections.png` and `explore.png`.
- Go tests 46, Pester 68.

## 0.3.6 — 2026-10-09

- Graph.Node lift at tag v0.2.3 (1807fd7): `internal/graphnode/schemas`, `internal/graphnode/graph` and `cmd/graphnode` (reports 0.2.3) recopied. The lift brings the `agent:`, `runner:`, `git:` and `ref:` id prefixes, the `triangle` and `star` shapes, display `when` rules and the envelope `source` block. Upstream now registers `ui:` to Portal with Portal's shape, so the lift carries no edit beyond the import path (docs/design.md 34).
- `graph-node` skill recopied at 0.2.3 from Graph.Node v0.2.3. Graph's committed copy is still 0.2.1, so Pester now checks graph-node against the pinned tag, and checks only that Graph's copy is not newer. `run-report` and `git-guard` are still compared with Graph's committed files in full (docs/design.md 34).
- Dev seed: the Runs menu entry `ui:menu:runs` (order 1) opens `ui:canvas:agent-runs`, a Canvas on the `agent-runs` layer below the filesystem Canvas, so a fresh checkout has the Runs view. The seed holds no Mount node: the layer is drawn once Claude.Agent mounts it. The seed is now 20 nodes and 19 edges (docs/design.md 35).
- Smoke expects Graph, Runs and Smoke in the top bar, with its own entry at order 2. `Invoke-Build Screenshot` mounts Claude.Agent's newest agent-runs layer when that sibling has one. Screenshot refreshed.
- Go tests 35, Pester 61.

## 0.3.5 — 2026-10-09

- A mount by path is a Mount node. `POST /api/layers` with `{name, path}` records `ui:mount:<name>` (`layer`, and `path` as given, made absolute) on the screen, so an approved snap mounts the layer again at boot. A directory path mounts its newest `*.graph.json` each time. Mounting the same path again leaves the screen and its stamp unchanged. An inline mount removes the node. Flag mounts (`-filesystem`, `-mount`) are never recorded and win over a recorded layer of the same name. A recorded layer that is gone at boot is printed and skipped (docs/design.md 33).
- Top bar: the stats show the open view's data layer, `<layer> · <n> nodes · <m> edges · <stamp>`, from `GET /api/layers`. The open view is the target of the last menu entry chosen, else the first entry's. The screen graph's counts and stamp move to a `screen …` badge beside it.
- `ontology.yaml` 0.3.5: Kind `Mount`. Go tests 35, Pester 61.

## 0.3.4 — 2026-10-09

- `Invoke-Build Up` replaces a running portal: when `artifacts/portal.json` records a live portal it calls `Stop-Portal`, waits for that port to free (10 s at most, else it throws naming the owner and starts nothing), then starts, printing `replaced <old stamp> with <new stamp>`. `-NoReplace` keeps 0.3.3's throw. The logic is the private `Invoke-PortalUp` and `Wait-PortalPortFree` (docs/design.md 32).
- CLAUDE.md: the lift pin reads Graph.Node v0.2.1, and the read-only skill line names all three copies and what Pester checks them against.
- Pester: replace, nothing to replace, `-NoReplace` and the timeout path, plus a check that Up wires them. Go tests 30, Pester 60.

## 0.3.3 — 2026-10-09

0.3.3: git-guard hook; run-report 0.2.2; graph-node 0.2.1

- `graph-node` skill recopied at 0.2.1. Graph's committed copy is byte for byte the copy at Graph.Node tag v0.2.1. `git-guard` 0.1.0 and `run-report` 0.2.2 already matched Graph's committed files. The `.claude/settings.json` git-guard entry was already in place (docs/design.md 31).
- Graph.Node lift moved from tag v0.2.0 to v0.2.1. The lift covers `internal/graphnode/schemas`, `internal/graphnode/graph` and `cmd/graphnode` (now reports 0.2.1). It brings the `alert:` and `ledger:` id prefixes and their shape checks. The one registered edit still names Portal as the `ui:` owner.
- Pester: `$GraphNodeTag` is now v0.2.1 and the registered edit matches upstream's new `ui:` row. graph-node is also checked against Graph's committed skill (version, full text, no extra files). "names the repo Portal" now skips the read-only graph-node copy. Go tests 30, Pester 55.

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
