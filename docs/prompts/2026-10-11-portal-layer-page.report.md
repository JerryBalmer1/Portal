# Portal 0.7.0, layers get an address and a page: report

Task: five items. (1) `POST /api/tenants/{tenant}/layers` answers 201 with `Location`, and `/api/layers` answers 410. (2) A page for one layer, linked from the layers list. (3) CLAUDE.md's read-only list gains the Portal.* seeds. (4) The Pester port-race fix. (5) Browser check 2.9, if it runs headless. Decisions: docs/design.md 42.

## Counts

| Suite | Before | After | New |
|---|---|---|---|
| Go top-level tests | 78 pass | **79 pass**, 0 fail | `TestMountLocationIsTheLayerPageAndTheOldRouteIsGone` |
| Pester | 98 pass | **100 pass**, 0 fail, 0 skipped | `a mount answers Location at the layer's page ...; POST /api/layers is 410 ...` and `Graph/Layers links each layer to its page ...` (headless Chrome) |
| `go vet` | clean | clean | |
| Smoke | passed | passed (canvas fit 0.844) | |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` | |

Both "before" numbers come from this tree before any edit: Go 78 from `go test ./... -v`, and Pester `Tests Passed: 98, Failed: 0`. The Pester baseline was still running when I began editing the spec, but the edits it could see (the spec text, not yet the version) leave its 98 unchanged. The "after" numbers come from one full `Invoke-Build` on the finished code, and Go was run once more afterwards. A first Pester run after the edits failed 3 tests because `build/win-x64/portal.exe` was still the 0.6.0 binary: Pester does not build. I stopped that run and ran the full `Invoke-Build` instead.

The later docs-only edits (design.md, CHANGELOG, checklist, this report) were followed by one more Pester run. Its result is below under Final Pester.

## 1. Location, and 410 for /api/layers

- The spec: `mountLayer`'s 201 now has a `required` `Location` header, `/<tenant>/layers/<name>`. `mountLayerUnscoped` (still `deprecated`) answers only `410` (new `Gone` response) or `401`. `internal/gen` was regenerated with `go generate ./api/...`.
- `MountLayer` sets `Location` to the layer's page (`layerPage` in `internal/server/server.go`). `MountLayerUnscoped` answers 410 with code `gone` and message `POST /api/layers is gone; mount with POST /api/tenants/<tenant>/layers`. It mounts nothing.
- **Location points at the page, `/<tenant>/layers/<name>`**, the route of item 2, not at an API resource.
- The bearer middleware still runs first, so an unauthenticated POST to `/api/layers` is 401 before it can be 410 (`TestAuthNoTokenReadsBadTokenIs401...` keeps that 401). See design.md 42.
- Tests that POSTed to `/api/layers`: the spec table in `TestEveryOperationAgainstSpec` had four calls expecting 201/400/404/422. They are now two calls expecting 410. No Pester test and no `.build.ps1` task POSTs there: Smoke only `GET`s `/api/tenants/default/layers`.
- Breaking a route is a minor bump: **0.7.0** in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`, and Pester's four pins. **Claude.Agent's Up must move to the tenant route.** That repo was not touched.

## 2. The layer page

- **Route.** `GET /{tenant}/layers/{name}` is a new spec operation, `getLayerPage` (tags page, layers). It serves `index.html` for a mounted layer, and 404 for an unknown layer or another tenant. chi prefers the literal `layers` segment over `getShellPage`'s `{service}`. So the merged-config check now refuses a menu service with the id `layers`, and a Go case covers it.
- **The layers list did not exist.** Nothing in the page listed layers; only the Explore toolbar named the open view's layer. I added it as a menu Page: **Graph/Layers** (`graph/layers`, `view: layers`, fourth under Graph). `web/layers.js` draws a row per mounted layer (name, module, nodes, edges, stamp, path), and the name links to `/<tenant>/layers/<name>`. `view` gained `layers` in `schema/portal.schema.json`.
- **The page.**
  - The shell maps `/<tenant>/layers/<name>` onto the Page whose view is `layers`, so the rail marks Graph/Layers and the crumbs read `default › Graph › Layers › filesystem`.
  - Title: the layer's name.
  - Toolbar: a badge `52 nodes · 51 edges · 2026-10-09T00:51:29Z` (module and path in its tooltip), then the row count.
  - Body: the layer's nodes from `GET .../layers/{name}/nodes`, drawn by `nodes.table`. That is the table builder the Graph pages (Kinds, Taxonomy) already used, now extracted from `nodes.mount` and shared: columns id, kind, label, updated.
  - The banner's search filters the rows. Node ids are plain text, because a layer's node has no item page.
- Checked headless (`--dump-dom`) in Pester and in checklist item 2.10. The click and the search box were not exercised by hand.

## 3. CLAUDE.md line 37

It now reads: `../Graph.Node`, `../GraphRenderer` (and `../Graph`, `../Graph.Emitter.FileSystem` for skills and fixtures, and the Portal.* seeds `../Portal.Tools`, `../Portal.Skills`, `../Portal.Workflows`, `../Portal.Emitters`, `../Portal.Ontology`). `../Portal.Agent` also exists beside them. It was not named, so it is not listed.

## 4. Port 18200

`Describe 'Up replaces a running portal'`'s `AfterEach` now reads `Stop-Portal -Artifacts $UpWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 }`. That is the one-line fix the feeds report named, and it matches the other blocks. The `Boot order` block's `AfterEach` has the same shape (the other race in that report) and was left alone.

## 5. Browser check 2.9

**It ran headless, in part.** Docker is not running here, so its precondition, item 3.7 (`Invoke-Build Feeds`), could not run. Instead, Chrome `--headless=new --dump-dom` read `/default/artifacts/feeds` from the portal already on 8200. That is your Dev portal: tenants/default in mode dev, a 0.6.0 binary, and cards in the ledger from the earlier pushes. It was a read-only GET, with no screenshot and no sign-in. Every Expect held:

- Feeds is first under Artifacts and current.
- There are two cards: `Cards (OCI)` (oci · private · `http://127.0.0.1:8201/cards`) and `Cards (NuGet)` (nuget · private · `http://127.0.0.1:8202/v3/index.json`).
- Each card has the row `Tool.Git · 0.1.0 · tool · afba6ccdf582 · <pushed> · dev-bootstrap`.
- The toolbar reads `2 feeds · 2 cards`.
- There are no buttons in the feeds pane.
- The amber DEV MODE bar is there.

The hover was not performed: what it would show is each hash cell's `title`, `sha256:afba6ccd…` plus `stored as` that feed's digest, read from the DOM.

## Docs

- **README.** Changed: the intro line, the What-is-here `web/` and `ontology.yaml` rows, Use the API (the mount line's Location, and the paragraph on `/api/layers` and the page routes), and Status (0.7.0, design.md 42). The What-is-here table, checked against `git ls-files`, has no row missing or extra (no new top-level path).
- **ONTOLOGY.md.** Changed: the version line, and the Data layer and Mount rows, which named `POST /api/layers` and now name the tenant route and the layer page. No Kind, edge or problem code changed.
- **docs/manual-check-list.md.**
  - Added 1.17 (Location and 410; run as written, its Expect the output seen) and 2.10 (Graph/Layers and the layer page; its Expect is what headless Chrome showed on that block's portal, without the click).
  - Changed 0.2's counts (79 Go, 100 Pester). Its "staged with mode dev" variant now says `99, Failed: 1`. That is inferred from 100, not seen.
  - Updated the Verified line. Not run: 3.7 (no Docker); 0.1, 0.3, 1.1 to 1.16, 3.1 to 3.6 and 3.8 (unchanged, or covered by Pester and Smoke); 2.1 to 2.8 (interactive).
- **CHANGELOG** 0.7.0; **design.md** 42; `portal.base.yaml`'s header comment names the `layers` view.

## Choices made without asking (design.md 42)

1. Location is the page URL, as the prompt said. There is no `GET /api/.../layers/{name}` resource.
2. The page route is its own spec operation at `/{tenant}/layers/{name}`, and a menu service with the id `layers` is refused.
3. The "layers list" is a new menu Page, Graph/Layers (`view: layers`), because none existed.
4. The layer page sits under Graph/Layers in the rail and crumbs. Its node ids are not links.
5. An unauthenticated POST to `/api/layers` is still 401 before 410. The middleware is not bypassed.
6. Version 0.7.0, because a route was removed.
7. Only the named `AfterEach` got the port wait. The `Boot order` one did not.
8. `../Portal.Agent` was not added to CLAUDE.md's list.

## Files

Staged:
- `api/openapi.yaml`, `internal/gen/api/api.gen.go`
- `internal/server/server.go`, `tenant.go`, `spec_test.go`, `tenant_test.go`
- `internal/tenant/tenant.go`, `tenant_test.go`
- `web/layers.js` (new), `web/nodes.js`, `web/portal.js`, `web/web.go`
- `portal.base.yaml`, `schema/portal.schema.json`
- `ontology.yaml`, `ontology.go`, `src/Portal/Portal.psd1`
- `tests/Portal.Tests.ps1`
- `CLAUDE.md`, `README.md`, `ONTOLOGY.md`, `CHANGELOG.md`, `docs/design.md`, `docs/manual-check-list.md`, this report

Unstaged: `tenants/default/portal.yaml` (your `mode: dev`), and nothing else. Nothing committed, pushed or tagged. No `artifacts/run-report.json` was written for this task.

Your Dev portal on 8200 was left running. `Invoke-Build` rebuilt `build/win-x64/portal.exe` underneath it, so it is still the 0.6.0 process, and `Invoke-Build Up` or `Dev` will start 0.7.0.

## Final Pester

After every edit above, from a fresh `pwsh -NoProfile -File .	ests\Invoke-Tests.ps1`: `Tests Passed: 100, Failed: 0, Skipped: 0, Inconclusive: 0, NotRun: 0`.
