# Portal 0.8.0, agents and their runs: report

Task: four items. (1) Node types Agent, Run and Decision in Portal's ontology, with ids shared with Graph.Emitter.Agent's runs layer. (2) `GET .../agents` and `.../agents/{name}/runs`, read from every mounted Graph.Emitter.Agent layer. (3) The pages `/{t}/agents` and `/{t}/agents/{name}`, wired to an existing menu entry. (4) Go tests for the routes on a fixture layer, and Pester for the page's counts. Decisions: docs/design.md 43.

## Counts

| Suite | Before | After | New |
|---|---|---|---|
| Go top-level tests | 79 pass | **83 pass**, 0 fail | `TestAgentsGroupRunsByTheMountingPrincipal`, `TestAgentsLegacyMountAndASharedRunID`, `TestAgentOfAPathMountSurvivesApproveAndRestart`, `TestScreenRefusesABorrowedID` |
| Pester | 100 pass | **101 pass**, 0 fail, 0 skipped | `Identity/Agents lists the principal that mounted runs; its page lists them, deny count in red, linking the layer and each decision` (headless Chrome) |
| `go vet` | clean | clean | |
| Smoke | passed | passed (canvas fit 0.844) | |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` | |

- **Go before** is from this tree before any edit (`go test ./... -count=1 -v`, 79 `--- PASS`, 0 fail).
- **Pester before** is `tests/testResults.xml` as I found it: total 100, 0 failures, written at 00:16 on 2026-10-11, the run behind 0.7.0's report (commit at 00:29). I did not rerun it on the untouched tree.
- **After** is from one full `Invoke-Build` on the finished code, after the last code edit.
- **One failed run.** The first Pester run after the edits failed 1 test: my own rail assertion had the anchor's attributes in the wrong order (`class` sits between `href` and `data-page`). I fixed the regex and the next run passed 101.

## 1. The ontology: no Agent Kind (your answer), Run and Decision

- **Asked.** `agent:` is registered to Graph.Emitter.Agent in Graph.Node's id registry. `agent:<name>` is already the shape of its AgentFile ids (`agent:CLAUDE.md`). And the graph-node skill forbids minting under another module's prefix. My first question offered `ui:agent:<name>`. Your answer: no Agent Kind. The agent is the 0.5.0 principal, `ui:principal:<name>`. Runs group by the principal whose token posted the layer, else under dev-bootstrap. Run and Decision keep the emitter's `agent:run/...` ids. design.md 43 records that `agent:` belongs to Graph.Emitter.Agent and Portal never mints under it.
- **`ontology.yaml` 0.8.0.**
  - Kind `Run`, idRule the emitter's `agent:run/<stamp>`. Properties: `layer`, `startedAt`, `stopped`, `finishedAt`, `exit`, `steps`, `allow`, `deny`, `witness`.
  - Kind `Decision`, idRule the emitter's `agent:run/<stamp>/step-<n>/<pre|post>`. Properties: `layer`, `effect`, `phase`, `rule`, `step`, `reason`.
  - Edges `has-run` (Principal to Run) and `has-decision` (Run to Decision).
  - Mount gains optional `mountedBy`.
  - Principal's `created` is no longer required. It is absent only on a dev-bootstrap made up because principals.yaml lacks it.
- **The ids really are shared.** A Go test checks that every Decision id equals a PolicyDecision id that `GET .../layers/agent-runs/nodes` returns. The Run is the emitter's Run node id.
- **The portal check.** Its `id-prefix` rule demands `ui:`. Run and Decision are excepted only in envelopes the API serves (`CheckEnvelope`), and there the id must pass `graph.ValidID("agent:", id)`. The screen still refuses them. `TestScreenRefusesABorrowedID` posts a Run with an `agent:` id and gets 422 `id-prefix`.

## 2. The API

- **`GET /api/tenants/{t}/agents`** (`listAgents`) is an envelope, module Portal. It holds every agent (Principal node), its Run nodes, and `has-run` edges.
- **`GET .../agents/{name}/runs`** (`listAgentRuns`) is rooted at the principal. It holds the agent's runs newest first by `startedAt`, each with its Decision nodes (`has-decision`). A name that is no agent is 404.
- **What counts as an agent.** Each principal of type `agent` (none exist until Marker), plus each principal holding a run, in principals.yaml order.
- **dev-bootstrap** gets runs whose layer was mounted with no principal: a legacy token, a `-mount` or `-filesystem` flag at start, or a principal principals.yaml no longer holds. When the file lacks dev-bootstrap, it is served as `user`/`admin` with no `created`.
- **Which layers.** "Whose ontology is Graph.Emitter.Agent" is read as the envelope's `module`.
- **Counts follow the emitter's edges.** A Run `contains` its Steps (`steps`), a Step `contains` its Action, and an Action `decided` its PolicyDecisions, counted by `effect` into `allow`, `deny` and `witness`, pre and post both.
- **A Run id in two layers is served once,** from the later stamp, the later mount on a tie.
- **Who mounted a layer.** `mountLayer` now records its caller (`store.Mount.By`, which the body cannot set). It is served on LayerSummary as `mountedBy` and written to the Mount node, so a path mount keeps its agent across approval and restart.
- **Both envelopes pass** `graph.ValidateBytes` and the portal ontology in the Go tests.
- The spec has four new operations. `internal/gen` is regenerated, and `web/client.js` has `listAgents` and `listAgentRuns`. Route, client and every-operation parity pass.

## 3. The pages, wired to Identity → Agents

- **Wired to Identity → Agents.** Its `view: agents` used to draw principals.js's list of type-agent principals, which was always empty. It now draws `web/agents.js`. The rail links the entry to `/default/agents`.
- **Routing.** The shell maps `/<t>/agents` and `/<t>/agents/<name>` onto that Page, as it maps layer pages onto Graph/Layers, so the rail marks Identity/Agents and the crumbs read `default › Identity › Agents › check`. `/<t>/identity/agents` still draws the same list. principals.js now draws Identity/Users only.
- **New page operations.** `getAgentsPage` and `getAgentPage` (404 for a name that is no agent). The literal `agents` segment wins over `{service}`, so a menu service with id `agents` is now refused at boot (Go case added).
- **The list** has one row per agent: name (linking to its page), type, role, runs, denied (red when not zero), last run.
- **The agent page** shows a badge `<n> runs · <d> denied` and its runs newest first: started, finished, steps, allow, deny (red when not zero), witness, exit, and the layer, linking to `/<t>/layers/<layer>`.
  - Under each run there is one link per decision, `step 3 pre deny` (a deny in red), going to `/<t>/layers/<layer>#<id>`.
  - A layer node has no item page, so the layer page now marks the row the fragment names (`pt-target`, `aria-current`) and scrolls to it.
- **Checked headless** (`--dump-dom`), in Pester and in checklist item 2.11.
  - Pester asserts on the list: the row `pester · user · admin · 1 · 1 (red) · 2026-10-09T02:50:43Z` and the rail current. On the agent page: the badge `1 runs · 1 denied`, the run row `3 · 4 · 1 (red) · 0 · 0 · agent-runs`, the five decision links in order with step-3/pre as the only deny, and the marked row on the layer page.
  - Clicks were not exercised by hand.

## 4. Tests and fixture

- **Fixture.** `tests/fixtures/layers/agent-runs/` holds Portal.Emitters' `emitters/Agent/artifacts/layers/2026-10-09T02-50-56Z.runs.graph.json` and the `ontology.yaml` beside it (Graph.Emitter.Agent 0.1.2, Portal.Emitters at cd862d9). It has one run, three steps, four allow and one deny, and passes `graphnode validate --ontology`. CLAUDE.md does not list `../Graph.Emitter.Agent`, so I did not read it.
- **Go** covers:
  - the four operations against the spec;
  - grouping: a flag mount goes to a made-up dev-bootstrap, and a contributor's inline layer of two runs to contrib, newest first;
  - counts and stamps, and Decision ids equal to the layer's;
  - a legacy-token mount going to dev-bootstrap, and a shared Run id served once, from the later mount;
  - `mountedBy` surviving approve and restart;
  - the screen refusing a borrowed id;
  - the `agents` service id refused at boot.
- **Pester** covers the pages (above). Its version pins moved to 0.8.0, the ontology test expects 15 Kinds and 11 edges, and the Identity test's Agents text is the new empty state.

## Docs

- **ONTOLOGY.md** (skill `ontology-doc`):
  - Rows changed: version; the prefix note; Page's `view` (users and agents split); Principal (`created` optional; `has-run`); Mount (`mountedBy`); `id-prefix`.
  - Rows added: terms Agent, Run, Decision; Kinds Run, Decision.
  - Every term resolved.
- **README.md** (skill `readme`):
  - Sections changed: line 1 terms; intro; What is here (`internal/server`, `internal/store`, `web/`, `ontology.yaml`, `tests/`); Use the API (two operations; mount's `mountedBy`; page routes); Status (0.8.0).
  - Step 2 (the table against `git ls-files`): no row is missing for a path this task added, and none is out of order. As before this task, the root files (`.build.ps1`, `CHANGELOG.md`, `CLAUDE.md`, `config.go`, `ontology.go`, `go.mod`, `go.sum`, `.gitignore`, `.gitattributes`, `Portal.code-workspace`) have no row.
  - The Use from PowerShell example is unchanged and was not rerun.
- **docs/manual-check-list.md** (skill `manual-check-list`):
  - Added: 1.18 (run as written; its Expect is the output seen) and 2.11 (read with headless Chrome).
  - Changed: 0.2 (Go 83, Pester 101; run in full), 2.7's Agents Expect (not rerun, as it is interactive), and the Verified line.
  - Repaired: three lines in 1.17 and 2.10 held a tab, form feeds and a stray `0x82` byte where `\tests`, `\fixtures`, `\filesystem` and `\2026` were meant. They were in HEAD from 0.7.0, and the file is now clean UTF-8.
  - Not run: as the Verified line lists.
- **design.md 43**, **CHANGELOG 0.8.0**, and CLAUDE.md's `tests/` layout line (the Agent runs layer).

## Not done, and notes

- **No PowerShell wrapper** (`Get-PortalAgent`): the brief did not ask, and `Mount-PortalLayer` already returns `mountedBy`. design.md 43.
- **Proposal for Graph.Node**, not made here: if agents ever need ids of their own under `agent:`, Graph.Node would register a shape such as `agent:principal/<name>`.
- **Old Mount nodes.** A Mount node from before 0.8.0 has no `mountedBy`, so an approved snap's path mounts list their runs under dev-bootstrap until they are mounted again.
- **`tenants/default/portal.yaml` is not staged.** It holds your uncommitted `mode: dev`, so it was left exactly as I found it. Pester's live blocks therefore ran in mode dev; no screenshots or demos were made.
- `../Graph/artifacts/layers/Graph.Emitter.Agent/` holds another emit of the same ledger: the same stamp and version (0.1.2), different bytes. The fixture is the Portal.Emitters copy.

Staged: every file above. Not committed, pushed or tagged.
