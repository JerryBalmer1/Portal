# Portal 0.12.1: alerts and environments read the env: and alert: prefixes only

**Task.** Five items:

1. Every match on alert or environment ids reads the new forms only, with no fallback and no mapping.
2. The lifted Graph.Node registry moves to 0.3.3.
3. The environment and alert fixtures are replaced with the real layers from Portal.Agent's latest Up.
4. Go and Pester move to the new ids, with one test proving `alert/<fp>` shows zero alerts.
5. A full Invoke-Build, version 0.12.1, the CHANGELOG, and a design.md note.

Decisions: docs/design.md 48. Edited and staged only. Nothing committed, pushed or tagged. `tenants/default/portal.yaml` is left as found: modified, unstaged, `mode: dev`.

## Counts

| | Before | After |
|---|---|---|
| Go top-level tests | 103 | **106**, all pass |
| Pester | 115 | **116** pass, 0 fail, 0 skipped (Chrome ran) |
| Smoke | ratio 0.844 | ratio 0.844 |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` |
| Fixture `agent-alerts/` | hand-made: 5 alerts under `alert/<16 hex>` + 3 copies, 10 edges | real: **1** alert under `alert:<64 hex>`, 0 copies, 2 edges |
| Fixture `agent-environment/` | real (10:08 Up): 4 `environment.instance` under bare ids, 5 edges | real (19:00 Up): **9** nodes under `env:` (4 instances, 3 images, 2 networks), 11 edges |
| Fixture `agent-environment-tagged/` | 4 nodes, 5 edges | 9 nodes, 11 edges, rebuilt from the new real layer |
| Lines matching `alert/` in server, store, web, Pester and spec | 32 | 9: comments, and the old-form ids in the two negative tests |
| Lines matching `alert:` there | 3 | 36 |
| Lines matching an `env:` form there | 0 | 52 |

- **Go before** is the number of `func Test` lines at HEAD. **Go after** is `go test -list` on the finished tree.
- **Pester before** is 0.12.0's CHANGELOG count.
- **Three full `Invoke-Build` runs.**
  - Run 1 failed one test of my own. Pester read `<fingerprint>` in its name as a template variable; I renamed it.
  - Run 2 passed 116.
  - Run 3, after the docs, ontology.yaml and the envgraph empty-state text, passed 116 again and ended `Build succeeded`.

## Every match point changed

**Alert ids.** All of these now go through `store.AlertFingerprint`: `alert:` then exactly 64 lower-case hex.

| Point | Where | Before | After |
|---|---|---|---|
| Alert reader (list, item, re-emit by fingerprint) | [internal/store/alerts.go](../../internal/store/alerts.go) `Alerts` | `alert/` prefix, any non-empty rest | `AlertFingerprint` |
| Copies-only guard | `store.IsAlertLayer`, used by `holdsLedger` (ledger.go), `EnvironmentGraph`, `NodesOfKind` (nodes.go), `HeldNodes` | Kind `alert` and `alert/` prefix | Kind `alert` and `AlertFingerprint` |
| 404 text of `getAlert` | [internal/server/alerts.go](../../internal/server/alerts.go) via `store.AlertIDPrefix` | `no alert alert/<fp>` | `no alert alert:<fp>` |
| Badge (open counts) | `listAlerts` root `open`, read by `web/portal.js` | | follows `Alerts`; no page change |
| Reverse "alerts" rows | `listAlerts?about=<id>`, read by `web/alerts.js` | | follows `Alerts`; no page change |
| Run's `alerts` count | `alertCounts` in `internal/server` | | follows `Alerts` |
| Alert row menu and item page | `web/alerts.js`, `nodemenu.detailsHref` | read `properties.fingerprint` | unchanged; the fingerprint is the 64 hex |

**Environment ids.**

| Point | Where | Before | After |
|---|---|---|---|
| Environments graph reader | [internal/store/environments.go](../../internal/store/environments.go) `EnvironmentGraph` | Kind starts `environment.` | `store.IsEnvironment`: Kind starts `environment.` and `graph.ValidID("env:", id)` |
| Graph page, own nodes | [web/envgraph.js](../../web/envgraph.js) `isEnvironment` | Kind only | `nodemenu.isEnvironment`: Kind and the four `env:` forms |
| Right-click on a table row | [web/nodes.js](../../web/nodes.js) `withMenu` | every row of an `environment.*` page | only rows that pass `nodemenu.isEnvironment` |
| Envgraph empty state | `web/envgraph.js` | names the Kinds | names the Kinds and the `env:` forms |

**Subject resolver** ([internal/server/resolve.go](../../internal/server/resolve.go)): no change was needed. It matches nothing on alert or environment ids: only Runs, Decisions, `ui:principal:` and `ledger:`, then any node held by a non-alert layer. Its alert-layer exclusion follows `IsAlertLayer`.

**Spec and ontology.** The text of `api/openapi.yaml` changed and `internal/gen` was regenerated; no operation or schema changed. `ontology.yaml`'s Alert `idRule` and `fingerprint` now say `alert:<fingerprint>`.

## Graph.Node lift

Portal does have its own registry: the lifted `internal/graphnode/graph/ids.go`. It moved to **v0.3.3** (`a536b0b`): `env:` owned by Claude.Agent.Orchestrator, `alert:<fingerprint>` accepted, `SharedWith` on `alert:` and `ledger:`, and an `about` edge from `alert:` allowed to dangle as a warning.

Everything is `git show v0.3.3:<path>`, with the Go import path rewritten:
- `internal/graphnode/schemas`
- `internal/graphnode/graph` (non-test files; `idformat.go` is new)
- `cmd/graphnode/main.go`
- `.claude/skills/graph-node` (now 0.3.3)
- `tests/fixtures/invalid.graph.json`

Pester's `$GraphNodeTag` is `v0.3.3`, and its "Lifted Graph.Node" and graph-node skill checks pass. I also edited CLAUDE.md: its two mentions of the pinned tag now say v0.3.3, following its own "recopy when a new tag is named" rule, and its fixtures line no longer calls the alert layer hand-made. Revert those lines if you'd rather keep CLAUDE.md to yourself.

## Fixtures

The source was `../Portal.Agent/agent/Orchestrator/artifacts`. The latest Up is `up.json`, started 2026-10-11T19:00:11Z.

- **`tests/fixtures/layers/agent-environment/2026-10-11T19-00-30Z.environment.graph.json`:** byte for byte from `environment-layers/`.
- **`tests/fixtures/layers/agent-alerts/2026-10-11T19-00-31Z.alerts.graph.json`:** byte for byte from `alert-layers/` (`cmp` equal).
- **`tests/fixtures/layers/agent-environment-tagged/2026-10-11T19-00-30Z.environment.graph.json`:** the new real layer with these changes:
  - `group` and `tags` added on compose, agent and mitm;
  - `env` dropped from the ledger keeper, to keep one `unset` node, since the real layer now gives every node `env: dev`;
  - hosts (and its edge id) pointed at the agent-runs fixture's run.
- **Deleted:** the three old files, `agent-alerts/2026-10-09T02-51-05Z.alert.graph.json` and `agent-environment{,-tagged}/2026-10-11T10-08-56Z.environment.graph.json`.
- **Not copied:** the folders' `ontology.yaml`, as before. The fixture directories hold the graph.json only.
- **The varied alerts live in the tests.** One real alert can't exercise severity order, chips, paging, badge colours or the subjects' reverse rows. Go (`variedAlerts`) and Pester (`Mount-AlEnvelope`) mount five alerts inline in the new form. They mean what the old hand-made set meant, and each fingerprint is SHA-256 of the rule, a newline and the subject. This is a hand-made set kept in test code, not a fixture file. It is flagged here because you asked for the old fixture to go.

## Tests

- **Go, new:**
  - `TestAlertsOfTheRealLayer`;
  - `TestOldAlertIDsAreIgnored` (item 4): a layer with `alert/<16 hex>` and `alert/<64 hex>` alerts mounts (201) and shows zero alerts, zero open, 404 on both fingerprints, nothing under `?about=` and no `alerts` on the run;
  - `TestEnvironmentGraphIgnoresBareIDs`.
- **Go, changed:**
  - `TestAlertsOfTheFixture` is now `TestAlertsOfTheVariedLayer`;
  - the alert, environment and spec tests use `alert:`/`env:` ids, the real alert and the new counts (9 environment nodes; edges `member-of`, `runs-image`, `attached-to`, `runs-as`, `hosts`).
- **Pester:**
  - the Environments graph block uses `env:` ids and the new counts, hulls and pinned sets;
  - the Alerts block mounts the varied set inline and matches `alert:<64 hex>` rows;
  - the orphan alert is in the new form;
  - new: an It mounting the real alert layer by path.

## Docs

- CHANGELOG 0.12.1; docs/design.md 48, and entry 4's pin.
- README: the lift, `internal/graphnode`, `tests/`, `.claude/skills/` and `ontology.yaml` rows, the API line and Status. The What-is-here table was not re-audited against `git ls-files`: no path was added or removed beyond the fixture file names it describes.
- ONTOLOGY.md: the Environments graph, Alert and Alert layer rows, the version, and two "Not here yet" items dropped, because images and networks are now emitted and the real alert layer has landed.
- Manual check list:
  - 1.21: Expect rewritten. Run as written; its output is what the Expect quotes.
  - 1.22: replaced, real layer plus an old-form layer. Run; output quoted.
  - 2.14: ids and counts updated, matching Pester's Chrome checks. Not clicked by hand.
  - 2.15: now over the real layer. The list, filtered list and item page were seen by `--dump-dom`; the menu as Pester's orphan alert checks it. Not clicked by hand.
- Version 0.12.1 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`.

## Still reading a bare id

Not changed, because they are generic rather than among the readers the brief named:

1. **Table pages** (`listNodes`; Environments/Instances, Images, Networks) list every node of their Kind from any mounted layer, bare-id `environment.instance` nodes included. Their rows no longer open the node menu.
2. **The subject resolver** and the environments graph's far ends serve any node a non-alert layer holds. An alert `about` a bare id is still linked if some layer holds that id.
3. **An old `alert/` layer is no longer an alert layer.** Copies it carries are read as ordinary nodes: a LedgerEntry copy could become the agent chain if its stamp is the latest, and an `env:` copy an environment. The real layers carry no copies, and `TestOldAlertIDsAreIgnored` mounts none.

Nothing else in `internal/` or `web/` reads `alert/` or a bare environment id. The 9 remaining `alert/` lines are comments and the negative tests.
