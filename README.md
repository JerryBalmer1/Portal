> Terms used here (screen graph, boot, tenant, base config, overlay, menu, Service, Page, principal, role, attenuation, bootstrap, token, service connection, Container, Widget, Layout, Category, State, Theme, Ref, menu entry, layer, snap, approve) are defined in [ONTOLOGY.md](ONTOLOGY.md).

# Portal

The portal of the graph family: a DevOps-style shell plus one API. A top banner (product, breadcrumb, search, marker, help, avatar), a left rail of services whose pages open in place, and a main pane. The rail is the tenant's menu, served as a Graph.Node envelope built from config (`portal.base.yaml` with the tenant's overlay merged over it); each page lists nodes of its Kind, or draws a screen node (the Canvas on Graph/Explore, the Designer on Designer/Sections), or lists the service connections, the tenant's principals or its tokens (docs/design.md 36, 38). Everything a page draws is a node in a Graph.Node envelope. The API reads and changes the screen graph, mounts data layers read-only, approves the screen as a snap and diffs two snaps (docs/design.md 1). Every API path names the tenant, `/api/tenants/<tenant>/...`. A request without a bearer token reads only; writing needs a token the tenant's `tokens.yaml` holds, and the role of the principal it belongs to decides which writes: reader none, contributor all but identity and settings, admin all (docs/design.md 38). A start boots from the newest approved snap, else from the dev seed (docs/design.md 20). The portal serves on one fixed port, 8200 (docs/design.md 24).

Rows, bottom to top:

```
Graph.Node           the shape: schemas, validator (lifted here at v0.2.3, internal/graphnode)
emitters             write layers: Graph.Emitter.FileSystem, ...
Portal               mounts layers read-only; its own screen and menu are graph/1 envelopes
Graph                orchestrates; calls the PowerShell wrappers here
```

Nothing edits below its own row.

![Overview/Summary](docs/screenshots/summary.png)

## What is here

| Path | What |
|---|---|
| `api/openapi.yaml` | The one API, OpenAPI 3.0.3. Source of truth for routes, handlers and `web/client.js`. |
| `api/codegen/` | oapi-codegen v2.8.0 config and the `go:generate` directive. |
| `internal/gen/api/` | Generated from the spec: models, chi server interface, embedded spec. Read-only. |
| `internal/server/` | The handlers and the bearer-token and role middleware; Go spec tests (every operation against the spec, route and client parity), the tenant tests (auth, menu, connections, GitHub mocked, shell routes) and the identity tests (role matrix, legacy tokens, attenuation, add and revoke on disk). |
| `internal/store/` | Screen graph and its boot, validation, layers, snaps and approval, diff, nodes of a Kind across screen and layers. |
| `internal/tenant/` | One tenant: base config plus overlay, merged and validated; the menu envelope; identity (principals, roles, tokens, attenuation; the only files the binary writes outside `-artifacts`); service connections and the GitHub call. |
| `internal/graphnode/` | Graph.Node's schemas and `graph` package, lifted at tag v0.2.3, unchanged but for the import path; upstream registers `ui:` to Portal (docs/design.md 4, 34). |
| `cmd/portal/` | The server: `portal [-port 8200] [-artifacts artifacts] [-tenants tenants] [-mount name=path]... [-filesystem <dir or file>] [-no-seed]`, on 127.0.0.1; `-port 0` is refused; `PORTAL_TENANT` names the tenant (default `default`); `PORTAL_SNAP=<stamp>` boots from that snap; `PORTAL_MODE=production` hides the Designer. A tenant file that fails its check stops it; a token with no principal is named in a warning on stderr. |
| `cmd/graphnode/` | Graph.Node's validator CLI, lifted: `graphnode validate <graph.json> [--ontology <ontology.yaml>]`, `validate-ontology <ontology.yaml>`, `prefixes`, `check-id <prefix> <id>`, `schema [name]`, `version`. |
| `portal.base.yaml` | Product defaults: product name and help link, `theme` (the Canvas's `graph.wheelZoomSpeed`, 1.0), and the menu tree (services, pages, item tabs). Embedded in the binary; never edited by a tenant. |
| `schema/portal.schema.json` | The one schema for the base and every overlay. |
| `tenants/default/` | The default tenant: `portal.yaml` (an empty overlay), `principals.example.yaml`, `tokens.example.yaml` and `connections.example.yaml`. `principals.yaml`, `tokens.yaml` and `connections.yaml` are the tenant's own and gitignored. |
| `web/` | The page: `index.html`, `portal.js` (the shell), `client.js`, `screen.js` (draws screen nodes), page renderers `nodes.js`, `connections.js`, `principals.js` (Identity/Users with an admin's Add, Identity/Agents) and `tokens.js` (Settings/Tokens with an admin's Revoke), `icons.js`, Category renderers `canvas.js`, `table.js`, `detail.js`, `designer.js` (with Approve; collapsible sections, each a State node), `toast.js` (refused writes; write controls off, with why, for anonymous or a role that may not), `base.css` (dark), vendored Cytoscape. |
| `ontology.yaml` | Module `Portal` 0.5.0: Kinds Container, Widget, Layout, Category, State, Theme, Ref, Mount, Service, Page, Principal; edges contains, laid-out-by, is-a, opens, displays, has-page, has-state, themed-by; findings absent, approved. |
| `src/Portal/` | PowerShell module: `Start-Portal`, `Stop-Portal`, `Get-PortalStatus`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`, `New-PortalToken`. |
| `tests/` | Pester (`Invoke-Tests.ps1`, `Portal.Tests.ps1`) and fixtures: `layers/filesystem/` (a FileSystem layer and its ontology), `invalid.graph.json` (Graph.Node's at v0.2.3). |
| `docs/` | `design.md` (decisions, numbered), `manual-check-list.md`, `screenshots/` (`summary.png`, `service-connections.png`, `explore.png`, `identity-users.png`), `prompts/` (task reports). |
| `.claude/skills/` | `manual-check-list`, `ontology-doc`, `readme` (this repo's docs); `graph-node` (0.2.3, copied read-only from Graph.Node v0.2.3); `run-report` 0.2.2 and `git-guard` 0.1.0 (copied read-only from Graph's committed HEAD, 0.2.8). |
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
| Edge or Chrome | `Invoke-Build Screenshot`; Chrome for Smoke's rail and Canvas-fit checks and Pester's page checks (each skips without it). |

## Build

```powershell
Invoke-Build              # Modules, Build, Test, Smoke, Pester
Invoke-Build Build        # portal.exe and graphnode.exe into build/win-x64/
Invoke-Build Test         # go vet, go test
Invoke-Build Smoke        # on artifacts/smoke, port 18200, its own tenant and bootstrap admin token: anonymous write refused, seed, menu entry, approve, Theme opacity, approve, restart, rail drawn from the menu, Canvas fits its panel; artifacts/smoke-report.json
Invoke-Build Up           # portal on http://127.0.0.1:8200 over artifacts/ and tenants/ (-Port, else PORTAL_PORT from .env, else the environment); replaces a running one, -NoReplace throws instead
Invoke-Build Status       # port, URL and boot stamp
Invoke-Build Down         # stop it
Invoke-Build Generate     # internal/gen from api/openapi.yaml
Invoke-Build Screenshot   # docs/screenshots/summary.png, service-connections.png, explore.png, identity-users.png; mounts Claude.Agent's newest agent-runs layer when present
```

Pester runs only as `pwsh -NoProfile -File .\tests\Invoke-Tests.ps1`.

## Use from PowerShell

The tenant here is a copy of `tenants/default` in a temp directory, so the repo's own `principals.yaml` and `tokens.yaml` are never touched. `New-PortalToken -Bootstrap` makes the tenant's first principal, an admin, and issues its token: it adds the token's hash to `tokens.yaml` and returns the token once. The module sends `PORTAL_TOKEN` as the bearer token.

```powershell
Import-Module .\src\Portal\Portal.psd1
$env:PORTAL_TENANTS = "$env:TEMP\portal-demo-tenants"
New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default" | Out-Null
Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default"
$env:PORTAL_TOKEN = (New-PortalToken -Name demo -Bootstrap).Token
$p = Start-Portal -Port 18200 -Artifacts $env:TEMP\portal-demo -Filesystem .\tests\fixtures\layers\filesystem
$p.Boot                                                             # seed
$p.Tenant                                                           # default
(Get-PortalStatus -Artifacts $env:TEMP\portal-demo).Url             # http://127.0.0.1:18200
$a = New-PortalSnap -Note 'first layout' -Url $p.Url
$auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/tenants/default/nodes/ui:theme:default" -Headers $auth -ContentType application/json -Body '{"properties":{"opacity":0.2}}' | Out-Null
$b = New-PortalSnap -Note 'lighter glass' -Url $p.Url
$b.diff.changed.nodes                                               # ui:theme:default
(Compare-PortalSnap $a $b -Url $p.Url).changed.nodes.id             # ui:theme:default
(Invoke-RestMethod "$($p.Url)/api/tenants/default/auth/whoami" -Headers $auth).role   # admin
Stop-Portal -Artifacts $env:TEMP\portal-demo
$p = Start-Portal -Port 18200 -Artifacts $env:TEMP\portal-demo -Filesystem .\tests\fixtures\layers\filesystem
$p.Boot -eq $b.stamp                                                # True
(Get-PortalScreen -Url $p.Url).nodes.Count                          # 20
Stop-Portal -Artifacts $env:TEMP\portal-demo
```

### First user

A tenant starts with no principals, and without a token everything reads only. Make the first admin once, then let it add the rest:

1. `New-PortalToken -Name <you> -Bootstrap` writes `tenants/<tenant>/principals.yaml` with you as admin (`createdBy` null) and your token's hash in `tokens.yaml`, and prints the token. A second `-Bootstrap` is an error.
2. Start (or restart) the portal, and give the token in the page's avatar menu, or set `PORTAL_TOKEN` for the module.
3. On Identity/Users, Add a user with a name and a role (reader, contributor or admin; never above your own). The portal appends it to `principals.yaml`, `createdBy` you.
4. `New-PortalToken -Name <user>` issues that user's token. The portal reads it at its next start, or at once after the next Add or Revoke, which re-read the files.
5. Settings/Tokens lists every token by principal, hash prefix and created; an admin's Revoke removes one at once.

A `tokens.yaml` from before 0.5.0 keeps working: an entry with `name` and no `principal` acts as admin until revoked, and the portal names it in a warning at boot. It cannot add users; bootstrap a principal first.

`Start-Portal -Snap <stamp>` boots from an older snap instead of the newest. `-Tenant` (else `PORTAL_TENANT`, else `default`) and `-Tenants` (else `PORTAL_TENANTS`, else the repo's `tenants/`) say which tenant it serves. Without `-Port` it serves on 8200, or `PORTAL_PORT` from `.env` or the environment; a busy port throws naming its owner. On the page, the avatar menu takes a token for the browser tab.

## Use the API

Every path below starts `/api/tenants/{tenant}`. The process serves one tenant; any other is 404. Writes, and `/connections/{name}/repos`, which spends the tenant's GitHub token, need `Authorization: Bearer <token>` (401 without it); a token the tenant does not hold is 401 on any request. A write the principal's role does not allow is 403: a reader writes nothing, a contributor no identity (`POST /principals`, `DELETE /tokens/{id}`), and no one creates a principal above its own role.

```
GET    /config                              the merged config: portal.base.yaml with the tenant's portal.yaml over it
GET    /menu                                the rail as an envelope: Service and Page nodes, has-page, icon States
GET    /auth/whoami                         {id, name, type, role, anonymous, legacy, writes, administers, tenant, marker}
GET    /principals     POST /principals     Principal nodes; add a user {name, role}: admin only, role at most yours (403)
GET    /tokens                              {id (hash prefix), principal, name, created, legacy} per token, never a token
DELETE /tokens/{id}                         revoke: removed from tokens.yaml, refused from then on; admin only
GET    /connections                         service connections, no secrets: name, type, url, token_env, tokenSet, createdBy
GET    /connections/{name}/repos            github: the token's repositories as git.repository nodes; needs a bearer token (401); 422 no GitHub token, 502 GitHub refused
GET    /nodes?kind=                         every node of a Kind on the screen and in the mounted layers
GET    /screen                              the screen graph (envelope); header X-Portal-Boot: <stamp>, seed or empty
POST   /nodes          POST /edges          add; Graph.Node shape; validated before accept
PATCH  /nodes/{id}                          properties and display State only
DELETE /nodes/{id}                          the node and its edges
GET    /layers         POST /layers         mounted layers; mount from a path (recorded as ui:mount:<name>) or an envelope
GET    /layers/{name}/nodes?kind=&from=     query a layer
POST   /snap           {"note": "..."}      approve: artifacts/snaps/<stamp>.graph.json, previous, diff summary
GET    /diff?a=<stamp>&b=<stamp>            added, removed, changed, and the merge
GET    /ontology                            portal ontology and every layer's
```

`POST /api/layers`, without the tenant, is kept (deprecated) as the mount for this process's tenant, so Claude.Agent's Up still finds it; it needs a token like any write. The page's routes are `/<tenant>/<service>/<page>[/<id>]`; `/` redirects to the first page. A refused write answers 422 with `problems`, each `{code, path, message, severity}`; the codes are listed in [ONTOLOGY.md](ONTOLOGY.md).

## Status

0.5.0: identity. Principals (Kind `Principal`: user or agent, role reader, contributor or admin, `createdBy`) in `principals.yaml`; tokens belong to a principal; roles enforced (403), with attenuation; `New-PortalToken -Bootstrap`; Identity/Users with Add and Settings/Tokens with Revoke; service connections record `createdBy` (docs/design.md 38). 0.4.1: read-only page for an anonymous viewer (write controls disabled, refused writes as toasts) and the config's `theme.graph.wheelZoomSpeed` (docs/design.md 37). 0.4.0: the DevOps-style shell (banner, rail from the tenant's menu, main pane), dark theme, base and tenant config split, tenant in every API path, bearer-token writes, service connections with a first GitHub type (docs/design.md 36). 0.3.6: the Graph.Node lift and `graph-node` skill at v0.2.3; the dev seed has the Runs menu entry and its `agent-runs` Canvas. 0.3.5: a layer mounted by path is a Mount node on screen (`ui:mount:<name>`), so an approved snap mounts it again at boot. 0.3.4: `Invoke-Build Up` replaces a running portal. 0.3.3: `graph-node` and the lift at v0.2.1. 0.3.2: the `git-guard` hook, `run-report` 0.2.2. 0.3.1: Designer sections collapsible, each a State node; the Canvas fits its panel after load. Decisions and what was deferred: docs/design.md (38 is 0.5.0's, 37 0.4.1's, 36 0.4.0's, 34 and 35 0.3.6's, 33 0.3.5's, 32 0.3.4's, 31 0.3.3's, 30 0.3.2's, 28 and 29 0.3.1's, 24 to 27 0.3.0's, 18 to 23 0.2.0's; 9, 15, 16, 22, 36 and 38 name the next questions).
