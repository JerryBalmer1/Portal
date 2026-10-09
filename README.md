> Terms used here (screen graph, boot, Container, Widget, Layout, Category, State, Theme, Ref, menu entry, layer, snap, approve) are defined in [ONTOLOGY.md](ONTOLOGY.md).

# Portal

The portal of the graph family: a blank page plus one API. Every panel on the page is a node in a Graph.Node envelope; the API reads and changes that graph, mounts data layers read-only, approves the screen as a snap and diffs two snaps. Widgets are modules, one renderer per base Category; the graph widget (Canvas) is one of them. No UI model of its own (docs/design.md 1); the first data it draws is Graph.Emitter.FileSystem's layer. A start boots from the newest approved snap, else from the dev seed (docs/design.md 20). The portal serves on one fixed port, 8200 (docs/design.md 24); the top bar's menu is graph too: Containers that `is-a` Menu, each `opens` what it shows (docs/design.md 25).

Rows, bottom to top:

```
Graph.Node           the shape: schemas, validator (lifted here at v0.2.1, internal/graphnode)
emitters             write layers: Graph.Emitter.FileSystem, ...
Portal               mounts layers read-only; its own screen is a graph/1 envelope
Graph                orchestrates; calls the PowerShell wrappers here
```

Nothing edits below its own row.

## What is here

| Path | What |
|---|---|
| `api/openapi.yaml` | The one API, OpenAPI 3.0.3. Source of truth for routes, handlers and `web/client.js`. |
| `api/codegen/` | oapi-codegen v2.8.0 config and the `go:generate` directive. |
| `internal/gen/api/` | Generated from the spec: models, chi server interface, embedded spec. Read-only. |
| `internal/server/` | The handlers; Go spec tests (every operation against the spec, route and client parity). |
| `internal/store/` | Screen graph and its boot, validation, layers, snaps and approval, diff. |
| `internal/graphnode/` | Graph.Node's schemas and `graph` package, lifted at tag v0.2.1 with `ui:` registered to Portal (docs/design.md 4). |
| `cmd/portal/` | The server: `portal [-port 8200] [-artifacts artifacts] [-mount name=path]... [-filesystem <dir or file>] [-no-seed]`, on 127.0.0.1; `-port 0` is refused; `PORTAL_SNAP=<stamp>` boots from that snap; `PORTAL_MODE=production` hides the Designer. |
| `cmd/graphnode/` | Graph.Node's validator CLI, lifted: `graphnode validate <graph.json> [--ontology <ontology.yaml>]`, `validate-ontology <ontology.yaml>`, `prefixes`, `check-id <prefix> <id>`, `schema [name]`, `version`. |
| `web/` | The page: `index.html`, `portal.js`, `client.js`, renderers `canvas.js`, `table.js`, `detail.js`, `designer.js` (with Approve; collapsible sections, each a State node), `base.css`, vendored Cytoscape. |
| `ontology.yaml` | Module `Portal` 0.3.4: Kinds Container, Widget, Layout, Category, State, Theme, Ref; edges contains, laid-out-by, is-a, opens, displays, has-state, themed-by; findings absent, approved. |
| `src/Portal/` | PowerShell module: `Start-Portal`, `Stop-Portal`, `Get-PortalStatus`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`. |
| `tests/` | Pester (`Invoke-Tests.ps1`, `Portal.Tests.ps1`) and fixtures: `layers/filesystem/` (a FileSystem layer and its ontology), `invalid.graph.json` (Graph.Node's at v0.2.1). |
| `docs/` | `design.md` (decisions, numbered), `manual-check-list.md`, `screenshots/portal.png`. |
| `.claude/skills/` | `manual-check-list`, `ontology-doc`, `readme` (this repo's docs); `graph-node` (0.2.1, copied read-only from Graph.Node v0.2.1); `run-report` 0.2.2 and `git-guard` 0.1.0 (copied read-only from Graph's committed HEAD, 0.2.8). |
| `.claude/settings.json` | Wires `git-guard` as a PreToolUse hook on the Bash and PowerShell tools: an agent's `git commit`, `git push`, `git tag`, `--amend`, `gh pr merge` and `gh release create` are refused. |
| `.env.example` | `PORTAL_PORT=8200`; copy to `.env` (gitignored) to move the portal. |

## Requirements

| Tool | Notes |
|---|---|
| Windows x64 | The only build target. |
| Go 1.27.0+ | `go.mod`; versions pinned there, `go.sum` committed. |
| PowerShell 7.4+ | |
| Pester 5.5+ | `Install-Module Pester -Scope CurrentUser` |
| InvokeBuild | `Install-Module InvokeBuild -Scope CurrentUser` |
| Edge or Chrome | `Invoke-Build Screenshot`; Chrome for Smoke's top-bar and Canvas-fit checks and Pester's Designer check (each skips without it). |

## Build

```powershell
Invoke-Build              # Modules, Build, Test, Smoke, Pester
Invoke-Build Build        # portal.exe and graphnode.exe into build/win-x64/
Invoke-Build Test         # go vet, go test
Invoke-Build Smoke        # on artifacts/smoke, port 18200: seed, menu entry, approve, Theme opacity, approve, restart, entry in the top bar, Canvas fits its panel; artifacts/smoke-report.json
Invoke-Build Up           # portal on http://127.0.0.1:8200 over artifacts/ (-Port, else PORTAL_PORT from .env, else the environment); replaces a running one, -NoReplace throws instead
Invoke-Build Status       # port, URL and boot stamp
Invoke-Build Down         # stop it
Invoke-Build Generate     # internal/gen from api/openapi.yaml
Invoke-Build Screenshot   # docs/screenshots/portal.png
```

Pester runs only as `pwsh -NoProfile -File .\tests\Invoke-Tests.ps1`.

## Use from PowerShell

```powershell
Import-Module .\src\Portal\Portal.psd1
$p = Start-Portal -Port 18200 -Artifacts $env:TEMP\portal-demo -Filesystem .\tests\fixtures\layers\filesystem
$p.Boot                                                             # seed
(Get-PortalStatus -Artifacts $env:TEMP\portal-demo).Url             # http://127.0.0.1:18200
$a = New-PortalSnap -Note 'first layout' -Url $p.Url
Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/nodes/ui:theme:default" -ContentType application/json -Body '{"properties":{"opacity":0.2}}' | Out-Null
$b = New-PortalSnap -Note 'lighter glass' -Url $p.Url
$b.diff.changed.nodes                                               # ui:theme:default
(Compare-PortalSnap $a $b -Url $p.Url).changed.nodes.id             # ui:theme:default
Stop-Portal -Artifacts $env:TEMP\portal-demo
$p = Start-Portal -Port 18200 -Artifacts $env:TEMP\portal-demo -Filesystem .\tests\fixtures\layers\filesystem
$p.Boot -eq $b.stamp                                                # True
(Get-PortalScreen -Url $p.Url).nodes.Count                          # 18
Stop-Portal -Artifacts $env:TEMP\portal-demo
```

`Start-Portal -Snap <stamp>` boots from an older snap instead of the newest. Without `-Port` it serves on 8200, or `PORTAL_PORT` from `.env` or the environment; a busy port throws naming its owner.

A menu entry is three writes through the same POSTs the Designer uses:

```powershell
$entry = @{ id = 'ui:menu:designer'; kind = 'Container'; name = 'Designer'; properties = @{ order = 1 } } | ConvertTo-Json -Compress
Invoke-RestMethod -Method Post -Uri "$url/api/nodes" -ContentType application/json -Body $entry
Invoke-RestMethod -Method Post -Uri "$url/api/edges" -ContentType application/json -Body '{"kind":"is-a","from":"ui:menu:designer","to":"ui:category:Menu"}'
Invoke-RestMethod -Method Post -Uri "$url/api/edges" -ContentType application/json -Body '{"kind":"opens","from":"ui:menu:designer","to":"ui:designer"}'
```

## Use the API

```
GET    /api/screen                          the screen graph (envelope); header X-Portal-Boot: <stamp>, seed or empty
POST   /api/nodes      POST /api/edges      add; Graph.Node shape; validated before accept
PATCH  /api/nodes/{id}                      properties and display State only
DELETE /api/nodes/{id}                      the node and its edges
GET    /api/layers     POST /api/layers     mounted layers; mount from a path or an envelope
GET    /api/layers/{name}/nodes?kind=&from= query a layer
POST   /api/snap       {"note": "..."}      approve: artifacts/snaps/<stamp>.graph.json, previous, diff summary
GET    /api/diff?a=<stamp>&b=<stamp>        added, removed, changed, and the merge
GET    /api/ontology                        portal ontology and every layer's
```

A refused write answers 422 with `problems`, each `{code, path, message, severity}`; the codes are listed in [ONTOLOGY.md](ONTOLOGY.md).

## Status

0.3.4: `Invoke-Build Up` stops a running portal, waits up to 10 s for its port and starts a new one, printing `replaced <old stamp> with <new stamp>`; `-NoReplace` keeps the old throw. 0.3.3: `graph-node` recopied at 0.2.1 and the Graph.Node lift moved to tag v0.2.1 (`ui:` still registered to Portal). 0.3.2: the `git-guard` hook wired in `.claude/settings.json`, `run-report` recopied at 0.2.2 (writes only to this repo's `artifacts/`). 0.3.1: Designer sections collapsible, each a State node (`ui:state:designer:<slug>`), Title Case headings and buttons, the Canvas fits its panel after load. Decisions and what was deferred: docs/design.md (entry 32 is 0.3.4's, 31 0.3.3's, 30 0.3.2's, 28 and 29 0.3.1's, 24 to 27 0.3.0's, 18 to 23 0.2.0's; 9, 14, 15, 16 and 22 name the next questions).
