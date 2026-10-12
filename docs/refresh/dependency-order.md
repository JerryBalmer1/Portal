# Refresh: the dependency order across the Portal repos

This file holds the order that `Invoke-Build Refresh` follows: every repo under `C:\__Code` named `Portal` or `Portal.*`, what each depends on, and the conflicts between them. The machine copy is [order.psd1](order.psd1) beside it. Refresh reads only that file, and Pester fails when the two disagree (docs/design.md 56).

Found on 2026-10-12 by reading each repo's build script, README, CLAUDE.md and docs/. Six dependents are on disk: Portal.Agent, Portal.Emitters, Portal.Ontology, Portal.Skills, Portal.Tools and Portal.Workflows. None has a `.build.ps1`. Two have a build script named after the repo (`Portal.Agent.build.ps1`, `Portal.Emitters.build.ps1`). The other four have none.

## What each repo does

| Repo | Build file | Invoke-Build tasks | Portal endpoints it calls | Imports Graph.Node from `..\Graph.Node\src` | Restarts Portal | Layers it pushes |
|---|---|---|---|---|---|---|
| Portal | `.build.ps1` | Modules, Generate, Build, Test, Up, DevGate, DevMint, Dev, FeedsGate, Feeds, Status, Down, Smoke, Pester, Screenshot, **Refresh** (new) | it is the API | no: Graph.Node is lifted into `internal/graphnode` and `cmd/graphnode` at v0.4.2 | Up, Dev and Feeds replace a running portal (Invoke-PortalUp); Down stops it | none on 8200; Smoke and Screenshot run their own portal on 18200 |
| Portal.Agent | `Portal.Agent.build.ps1` (lane; each `agent/<X>/.build.ps1` below it) | Preflight, Test, Build, **Up**, Smoke, **Down**, Status | `GET /api/tenants/{t}` (mode), `GET .../dev-token` (after a 401, mode dev, loopback only), `POST .../layers` ×4 on Up (+1 after a Runner pass that called docker), `POST .../layers` on Down (environment layer marked stopped). It also appends `action.result` lines to `tenants/{t}/ledger.jsonl` directly (Runner) | yes: Preflight wants GraphNode 0.3.2+ there, and Orchestrator Modules copies `src/GraphNode` into `build/host-modules` | no | `agent-runs`, `agent-ledger`, `agent-environment`, `agent-alerts`, each a **path mount of a folder** under `agent/Orchestrator/artifacts/` (`layers`, `ledger-layers`, `environment-layers`, `alert-layers`) |
| Portal.Emitters | `Portal.Emitters.build.ps1` | Test, Emit, **Mount** | `POST .../layers` ×6, `POST .../snap`, `GET .../layers`, `GET .../emitters`. The module's `Push-Card` / `Push-AllCards` call `POST .../feeds/{feed}/push`, but no task does | yes: Enter-Build imports `..\Graph.Node\src\GraphNode\GraphNode.psd1` | **yes: Mount runs Portal's `Down` then `Dev` twice** (before mounting, and after its snap) | `emitter.azuredevops`, `emitter.filesystem`, `emitter.git`, `emitter.network`, `emitter.reference`, `emitter.terraform`: path mounts of `artifacts/layers/<card>`. Mount approves them itself. Agent, Environment and Ledger are `mount: live` and skipped |
| Portal.Ontology | none | none | none | tests only | no | none (rule cards; `index.yaml`) |
| Portal.Skills | none | none | none | tests only | no | none (skill cards) |
| Portal.Tools | none | none | none | tests only | no | none (tool cards). Portal's `tests/fixtures/cards/Tool.Git` is a copy, and Portal's own `Feeds` pushes it |
| Portal.Workflows | none | none | none | tests only | no | none (workflow cards). Its tests read Portal.Skills, Portal.Emitters and Portal.Tools |

**File-backed or live.** Every layer a dependent pushes is a path mount of a folder on disk, so in that sense every one is file-backed. Portal still holds the mount in memory only. It records the mount as a `ui:mount:<layer>` node, and that node reaches disk only through an approved snap (design.md 14, 20, 33). So the rule the brief names holds: until a snap is approved after it, a pushed layer is **live** and is gone after any restart. Once a snap holds its Mount node, each boot remounts the folder's newest `*.graph.json`. Portal's own `tenant-actions` layer is derived from the ledger at every boot (design.md 55) and needs nothing.

## Depends on

| Repo | Depends on | Why |
|---|---|---|
| Portal | Graph.Node (lifted at v0.4.2, read-only) | Its validator and schemas are compiled in. The checkout at `..\Graph.Node` is not read at run time |
| Portal.Emitters | Portal (up, mode dev), Graph.Node checkout | Mount drives Portal's `Down` and `Dev` and posts to its API. Every emitter imports GraphNode from the checkout |
| Portal.Agent | Portal (up for Down and Up), Graph.Node checkout, Portal.Emitters checkout, Docker | Down re-posts the environment layer to a running portal. Up mounts four layers on it. Up imports `Graph.Emitter.Agent` from `..\Portal.Emitters\emitters\Agent`, and Modules copies GraphNode |
| Portal.Workflows | Portal.Skills, Portal.Emitters, Portal.Tools (tests only) | Its called-verb check reads their cards. Nothing to run in a Refresh |
| Portal.Ontology, Portal.Skills, Portal.Tools | Graph.Node checkout (tests only) | Card checks and `New-GraphIndex`. Nothing to run in a Refresh |

## The Refresh sequence

| # | Step | Repo | Task | Phase | Why here |
|---|---|---|---|---|---|
| 1 | agent-down | Portal.Agent | Down | down | Before Portal's Down, because it re-posts the environment layer (stopped) to a running portal. Skipped when the Docker engine is not answering: the lane's Down throws then, and with no engine there is nothing to stop |
| 2 | portal-down | Portal | Down | down | Last down. Nothing after it needs a portal until Dev |
| 3 | portal-dev | Portal | Dev | up | First up: a portal booted from the newest snap, with a fresh dev-bootstrap token |
| 4 | emitters-mount | Portal.Emitters | Mount | up | Restarts Portal twice, so it runs before any step that pushes a live layer. Its own snap saves its six Mount nodes |
| 5 | agent-up | Portal.Agent | Up | up | Pushes the four live agent layers after the last restart, so nothing wipes them before step 6 |
| 6 | approve | Portal | `POST .../snap` | up | After the last live push: saves the agent Mount nodes, so the next restart keeps them |
| 7 | catalogue | Portal | `GET .../emitters` | check | Last: the roll call after everything is mounted, compared with the previous Refresh |

Portal.Ontology, Portal.Skills, Portal.Tools and Portal.Workflows have no step. They have no build script, no Down and no up task. Refresh prints one line for each saying so. A repo that is not on disk is skipped with one line, and so is its every step.

## Conflicts found, and how each is resolved

1. **Portal.Emitters `Mount` restarts Portal mid-sequence (twice).** Each restart drops every layer not held by an approved snap, so Mount would wipe Portal.Agent's four layers if Agent's `Up` ran first. Its `Emit` task does not restart, but using it would mean Refresh does Mount's mount, snap and verify itself, copying Portal.Emitters' code into Portal. **Resolved by order.** Refresh calls `Mount` unchanged, before any step that pushes a live layer (rule R3), and Agent's `Up` runs after Mount's last restart. Nothing is re-pushed. Mount's own snap already saves its six Mount nodes, and step 6 saves the agent's four.
2. **Portal.Agent `Down` needs a running portal.** It posts the environment layer as stopped before removing containers. **Resolved by order:** every dependent's Down runs before Portal's Down (rule R2).
3. **Portal.Agent's lane `Down` throws when the Docker engine is down** (Assert-DockerEngine), and that would stop the whole Refresh. **Resolved by a skip:** Refresh asks `docker info` first and skips the step with one line when there is no engine. `Up`'s Preflight starts Docker Desktop itself.
4. **Every Portal restart re-mints the dev token** (DevMint replaces every dev-bootstrap token), so any token a step holds goes stale at the next restart. Agent `Up` recovers through `GET .../dev-token` after a 401 and writes it to its `.env`, and Emitters `Mount` takes the token its own `Dev` sets. Agent `Down` does neither, so a stale `.env` token gets a warning there, not a failure (prompt B in the report). **Resolved for Refresh's own writes:** each step runs in a child `pwsh`, so no token passes from one step to the next. Before its snap, Refresh fetches the current dev token from `GET .../dev-token` on loopback and never prints it.
5. **Portal's own `Feeds` replaces a running portal** (Invoke-PortalUp). **Resolved by leaving it out.** The Catalogue reads pushed cards from the tenant ledger, not from the running registry (design.md 49), so Refresh does not need Feeds. Run `Invoke-Build Feeds` before Refresh when the stock feeds themselves are wanted, never between its steps.
6. **Portal's `Down` before `Dev` is redundant:** Dev's Up replaces a running portal anyway. Kept, because the brief names it and it makes the down phase total. It costs one stop.
7. **Graph.Node pins disagree; there is one checkout.** Portal is lifted at v0.4.2. Portal.Emitters pins v0.4.1. Ontology, Skills, Tools and Workflows pin v0.3.0. Agent wants 0.3.2 or later. `..\Graph.Node` is at v0.4.2. Refresh runs no card-repo tests, so this cannot break a Refresh. It is reported rather than resolved (prompt C), and Refresh prints the checkout's GraphNode version on its first line.
8. **Portal.Emitters' live cards name stale layer names.** `emitters/Environment/mount.yaml` says `environment` and `emitters/Ledger/mount.yaml` says `ledger`. Portal.Agent mounts `agent-environment` and `agent-ledger` (and `agent-alerts`, which no card names). Only Mount's table label is affected, since those rows are `live` and skipped. Reported (prompt D).

## Rules Refresh checks before running anything

`Test-PortalRefreshOrder` checks these rules over [order.psd1](order.psd1). Pester checks them on the shipped file and on broken copies, one per rule.

- **R1 Phases.** Every `down` step comes before every `up` step, and `check` steps come last.
- **R2 Portal bounds.** `portal-down` is the last down and `portal-dev` the first up. A step that needs a portal runs in the down phase before `portal-down`, or in the up phase after `portal-dev`.
- **R3 No restart after a live push.** No step that restarts Portal comes after a step that pushes a live layer.
- **R4 Approve after the last push.** An `approve` step comes after the last step that pushes a live layer, and before the `catalogue` step.
- **R5 Known repos only.** Every step's repo is `Portal` or `Portal.<Name>`, and every step id is unique.
- **R6 Catalogue last.** Exactly one `catalogue` step, and it is the final step.

## The Catalogue check

The last step runs `GET /api/tenants/{t}/emitters` and prints Name, Kind, Status, Version, Layers and LayerVersions for every Card and Emitter row. It compares the result with `artifacts/refresh/last.json` (gitignored with `artifacts/`). Refresh fails, naming each such card, when a card that read `ok` last time now reads `no-layers` or `version-mismatch`. `last.json` is written only after a Refresh passes, so a regression keeps failing until it is fixed. The failing roll call goes to `artifacts/refresh/failed.json` (design.md 56).
