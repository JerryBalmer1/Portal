# Portal tidy after 0.6.0 — report

Task: three small things, no new features. Avatar → Use dev token; CLAUDE.md line 39 and any other stale version or path claim in CLAUDE.md and README; one line in README "Before you demo". Graph.Node, feeds and compose were not touched. No version change. Decisions: docs/design.md 41.

## What changed

| File | Change |
|---|---|
| `tests/Portal.Tests.ps1` | The prod-mode test now opens the avatar's box and asserts that neither `button[data-dev-token]` nor `.pt-dev-signin` is there. It is renamed `in mode prod the page shows no bar, no badge and no Use dev token`. |
| `CLAUDE.md` | Line 7 (lift) and line 39 (skills) rewritten. Layout gains `internal/tenant/`, `internal/feed/`, the Tool.Git fixture, and the Feeds and Status tasks. |
| `README.md` | One line under Before you demo, after the numbered steps: revoke `jerry`'s token and re-mint it with `New-PortalToken -Name jerry`, because a bootstrap token was pasted in chat on 2026-10-10. Status names design.md 41. |
| `docs/design.md` | Entry 41. |

### 1. Avatar → Use dev token

Already shipped in 0.5.1 (design.md 39), so no page code changed:

- `devSignIn` in `web/portal.js` returns early unless `mode === 'dev'`.
- The click calls `client.getDevToken()` (`GET /api/tenants/{tenant}/dev-token`, 127.0.0.1 only), then `setToken`, which writes `sessionStorage['portal.token']`. That is the bearer `client.js` already sends on every write.
- The avatar then reads `dev-bootstrap · admin`.

Coverage after this task:

| Mode | Test | Checks |
|---|---|---|
| dev | `the page: the amber bar, then Use dev token signs in as dev-bootstrap, admin, and Add works` | The button is there, the click stores a 64-character bearer, and the avatar reads `dev-bootstrap · admin`. A following POST (Add user `ann`) succeeds with createdBy `dev-bootstrap`, which shows the next POST used the bearer. |
| dev | `the page: a reader signed in through the box cannot click Add` | Still offered to a non-dev-bootstrap principal. |
| demo | `in mode demo: ... no Use dev token ...` | Not rendered; `dev-token` returns 404. |
| prod | `in mode prod the page shows no bar, no badge and no Use dev token` | **New assertion:** not rendered. Before this task it was checked only by a DOM dump that never opened the box. |
| Go | `internal/server/devmode_test.go` | The route answers only in dev, only from loopback (unchanged). |

### 2. Stale claims, checked against the tree

Found and fixed (CLAUDE.md):

| Line | Was | Now / why |
|---|---|---|
| 39 | Pester checks graph-node, run-report and git-guard against Graph's committed skill, and graph-node also against Graph.Node's tag | run-report and git-guard are checked against Graph's committed skill (version, every file, nothing extra). graph-node is checked against Graph.Node v0.3.0 (`version:` equals the tag's ModuleVersion 0.3.0, every file) and must never be older than Graph's copy (Graph has 0.2.1). The lift is checked separately in "Lifted Graph.Node". Stale since 0.3.6. |
| 7 | "the copy carries one registered edit, `ui:` owned by Portal" | `$Registered = @{}`: since v0.2.3 the copy differs only in the import path (design.md 4, 34). |
| 31 | Tasks "Modules, Generate, Build, Test, Up, Dev, Down, Smoke, Pester, Screenshot" | Feeds and Status added; both exist in `.build.ps1`. The gates are internal and stay out. |
| layout | No `internal/tenant/`, `internal/feed/`; fixtures without the card | Added. |

Found, not fixed:

- **CLAUDE.md line 37 (read-only siblings).** The rule lists `../Graph.Node`, `../GraphRenderer`, `../Graph` and `../Graph.Emitter.FileSystem`. But 0.6.0 copied `tests/fixtures/cards/Tool.Git` from `../Portal.Tools`, and README's Publishing example reads `..\Portal.Tools`. Widening the rule is your call.
- **0.6.0 was committed during this task** (`4e46337`, not by this task). It also took `docs/manual-check-list.md` and `docs/prompts/2026-10-11-portal-feeds.report.md`, which had unstaged changes at the start. The changes listed here are staged on top of that commit.

Checked and held:

| Claim | Where | Found in the tree |
|---|---|---|
| oapi-codegen v2.8.0 | CLAUDE 5, README | `go.mod` tool line `v2.8.0` |
| Graph.Node tag v0.3.0 | CLAUDE 7, README rows/table/status | Tag exists; `GraphNodeTag = 'v0.3.0'` in Pester; ModuleVersion 0.3.0 |
| Cytoscape 3.34.3 | CLAUDE 41 | `web/vendor/cytoscape.min.js` contains 3.34.3 |
| Go 1.27.0 | README | `go.mod` `go 1.27.0` |
| `go generate ./api/...` | CLAUDE 5 | `.build.ps1` Generate; `api/codegen/generate.go` |
| design.md 1, 2, 4, 5, 20, 22, 24, 34, 36, 38, 39, 40 | both | Headings exist with matching subjects |
| `pwsh -NoProfile -File .\tests\Invoke-Tests.ps1` | both | File exists; `.build.ps1` Pester uses it |
| Layout paths (api/codegen, internal/{gen/api,server,store,graphnode}, cmd/{portal,graphnode}, web, ontology.yaml, src/Portal Public/Private, tests, docs) | CLAUDE | All exist |
| `ontology.yaml` module Portal 0.6.0 | README | `module: Portal`, `version: 0.6.0` |
| Nine exports | README | `src/Portal/Public/` holds exactly those nine files |
| Sixteen `web/` files | README | All exist |
| Screenshots | README | All four `.png` exist |
| Fixtures `layers/filesystem/`, `invalid.graph.json`, `cards/Tool.Git/` (c330712) | README | Exist; c330712 matches design.md 40 |
| `tenants/default/*.example.yaml` | README | All three exist |
| Compose images `registry:2.8.3`, `bagetter/bagetter:1.6.5`, `postgres:16.15-alpine` | README | `compose.yaml` |
| Skills graph-node 0.3.0, run-report 0.2.2, git-guard 0.1.0; Graph HEAD 0.2.8 | README | `version:` lines; Graph's committed skills 0.2.2 / 0.1.0; Graph.psd1 0.2.8 |
| Default task Modules, Build, Test, Smoke, Pester | README | `task . Modules, Build, Test, Smoke, Pester` |

### 3. Before you demo

The steps keep their order and numbering. The new line comes after the paragraph that explains the order, so it does not split the steps. `jerry` is in `tenants/default/principals.yaml`.

## Counts

| | Before (0.6.0 follow-up, manual-check-list 0.2) | After (this run) |
|---|---|---|
| Go | 78 tests, 5 packages `ok` | 78 tests, 5 packages `ok` |
| Smoke | passed (canvas fit 0.844 / 0.853) | passed (canvas fit 0.844) |
| Pester | 98 passed | 98 passed, 0 failed, 0 skipped |
| Build | 6 tasks, 0 errors, 0 warnings | 6 tasks, 0 errors, 0 warnings |

The Pester count stays the same because the prod assertion went into an existing test. manual-check-list needs no change.

## Staging

Staged: `CLAUDE.md`, `README.md`, `docs/design.md`, `tests/Portal.Tests.ps1`, this report. Unstaged: `tenants/default/portal.yaml` (mode dev) only. Nothing committed, pushed or tagged. No `artifacts/run-report.json` was written for this task.
