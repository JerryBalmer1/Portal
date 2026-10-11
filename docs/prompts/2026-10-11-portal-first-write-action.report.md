# Portal 0.14.0: right-click actions on containers, with the principal and the ledger in the path

**Task.** On Environments/Graph, right-clicking a container offers Logs, Stop and Restart. Choosing one appends an `action.request` line to the tenant ledger as the signed-in principal and shows the id of the request. The page shows the result once the Runner has written one. Portal asks; it never executes. The seven items:

1. The API: `POST`/`GET .../actions` and `GET .../actions/{seq}`.
2. The menu items, on containers only, with a confirm for Stop and Restart.
3. The Actions page.
4. Ledger entry pages that link a request and its result both ways.
5. Copies only.
6. Fixtures and tests.
7. A design.md entry, version 0.14.0.

Decisions: docs/design.md 50. **Updated for 0.14.1** (docs/design.md 51): one ledger, one seq space; see the section at the end. Where this report names 0.14.0's split chains, ids or fixture, 0.14.1 replaced them. Everything is edited and staged only; nothing is committed, pushed or tagged. `tenants/default/portal.yaml` is left as found: modified and unstaged. No screenshots or demo were made, so the dev-mode rule did not apply. Every manual check here runs on a throwaway tenant in mode `demo`.

## The one question asked

**Which ledger holds the action lines, and how do the two `ledger:<seq>` spaces stay apart?** The contract's `id: ledger:<seq>` for a request can only be the seq of the tenant's `ledger.jsonl`, the one file Portal may write. Item 5 says action lines are read from the ledger layer, and the agent ledger's entries there are `ledger:<seq>` in another seq space. So `ledger:7` could name two lines.

Jerry chose **split chains**:
- The request line carries `id: ledger:<seq>` exactly as the contract says. Graph.Node 0.3.3 shares `ledger:` with Portal.
- Results are read from the agent ledger layer only.
- Served nodes keep their own chain ids: the request is `ui:ledger:portal/<seq>`, with `lineId: ledger:<seq>`, and the result is `ledger:<seq>`. No envelope holds one id twice.

## The four roles, as they now stand in code

| Role | Who | Where in code |
|---|---|---|
| Asks, as the principal | Portal | `server.PostAction` takes `{action, subject}` only; the principal is the middleware's, from the token. `tenant.RequestAction` appends through `record`, the push's write path. |
| Executes | Portal.Agent's Runner | Not in this repo. Nothing in Portal starts, stops or reads a container. |
| Records | the two ledgers | The request is a line of `tenants/<t>/ledger.jsonl`, chained. The result is an entry of the agent's ledger, read only through the mounted ledger layer (`store.AgentLedger`, alert layers excluded). |
| Shows the gap | the graph | `server.joinActions` gives each request an `outcome`: `requested` until a result names it, then the result's status. `answered-by` and `answers` join the two. |

## Counts

| | Before (0.13.0) | After (0.14.0) |
|---|---|---|
| Go top-level tests | 107 | **112**, all pass |
| Pester | 118 | **124** pass, 0 fail, 0 skipped (Chrome ran) |
| Smoke | ratio 0.844 | ratio 0.844 |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` |
| API operations | | +3 (`listActions`, `postAction`, `getAction`) and the page route `getActionsPage` |
| Ontology | 21 Kinds, 20 edges | 22 Kinds (`Actions`), 23 edges (`has-action`, `answered-by`, `answers`) |
| Ledger layer fixture | 12 entries, 44 to 55 | 14 entries, 44 to 57 (56 done, 57 failed) |

## Not matching the brief, or worth knowing

- **The join keys on the id alone**, which is enough since 0.14.1: request and result are lines of one chain, so a reset of `ledger.jsonl` drops both sides and no old result is left to answer a reused `ledger:1`.
- **The three items also appear on an Instances row's menu**, not only on the graph. It is the same node menu (design 46), and the items come from `nodemenu.items`.
- **The three items are `<button>`s, not `<a>`s**, since they go nowhere. Design 46's "every item a link" still holds for the four that link.
- **The confirm is an in-page dialog**, not `window.confirm()`, with Cancel focused and Escape as Cancel. Headless Chrome tests can drive it; a native confirm would block them.
- **Status `requested` is amber, the warning colour.** The brief named no colours. Done is green and failed is red.
- **A body that names `principal` is refused (400), not ignored.**
- **A legacy token (from before 0.5.0) is 403**: the line must name a principal, and the token has none.
- **Not added:** paging on `/actions`; cancelling a request; a pending mark drawn on the graph node itself; a PowerShell wrapper. The brief asks for routes, a page and tests.
- **CLAUDE.md** changed in one phrase: the ledger line is now "one appended line per push or action request (docs/design.md 50: Portal asks, it never executes)". Design entry 2 says the same. The binary writes the same three files as before.

## API

- `POST /api/tenants/{t}/actions` takes `{action, subject}`.
  - 201 with the line as written.
  - 400 for a malformed body, or one with any other member such as `principal`.
  - 401 without a token.
  - 403 for a reader or a legacy token.
  - 422 for another action or a subject that is no `env:container/<project>/<service>`.
- `GET /api/tenants/{t}/actions` serves the requests newest first under `ui:actions`, each with its result and both edges.
- `GET /api/tenants/{t}/actions/{seq}` serves one request and its result. It is 404 when that line is no request, and 400 when `seq` is not an integer.
- `getLedgerEntry` now serves the other half of an action beside the entry. `getLedger` carries the joined properties.
- The line on disk, as Pester and manual check 1.23 saw it:
  `{"kind":"action.request","id":"ledger:4","action":"restart","subject":"env:container/claude-agent-dev/mitm","principal":"ui:principal:check","requested":"2026-10-11T21:48:00.792Z","status":"requested","seq":4,"prev":"1193b6…"}`

## Tests

- **Go** (`internal/server/actions_test.go`), five new tests:
  - validation: 401; 403 for a reader and for a legacy token; 422 for eight bad asks; 400 for a principal in the body, writing nothing;
  - the principal from the token, and the line on disk;
  - the join: done, failed and the gap, both entry pages;
  - an alert layer's copy answering nothing;
  - a restart reading requests among pushes, and a bad request line stopping the boot;
  - the menu's containers being the API's subjects over the real environment layer.

  The spec test exercises the four new operations, and client parity includes the three new ROUTES. The ledger, alerts and menu-shape tests follow the fixture and the new page.
- **Pester**: a new `Actions` block (tag `Actions`, six Its). The Ledger, Environments graph, Module and Ontology Its are updated.

## Docs

- **README**: line 1's terms, the summary, the What-is-here rows (`internal/server`, `internal/store`, `internal/tenant`, `web`, `ontology.yaml`, `tests`), Use the API (three lines), the page routes, and Status 0.14.0. Its table checked against `git ls-files`: no row missing or extra; the new files fall under rows that already exist.
- **ONTOLOGY.md**:
  - its version line moves to 0.14.0 (it said 0.12.1, already stale);
  - the id note, Page views, Ledger and LedgerEntry are updated;
  - Action, Action request, Action result and Runner are new terms;
  - the LedgerEntry row and a new Actions row in the Kinds table;
  - one Not-here-yet line.

  `graphnode validate-ontology ontology.yaml` returns `[]`.
- **docs/manual-check-list.md**:
  - New: 1.23 (API, run, Expect as seen) and 2.16 (page).
  - Changed: 1.20 (re-run, new counts), 2.13 and 2.14 (Expects for 14 entries and the container's three items).
  - The Verified line is updated.
- **CHANGELOG** has a 0.14.0 entry. Version 0.14.0 is in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`, and `internal/gen` is regenerated.

## What to click (check-list)

Start the page with manual check **2.16**: a demo tenant, the actions fixture as its ledger, and the runs, ledger and environment layers mounted. It prints a token for the avatar. Then (ids as of 0.14.1):

- [ ] **Environments → Actions** (last in the rail) at `/default/actions`.
  - The toolbar reads `3 actions · 1 requested · 1 done · 1 failed`.
  - Rows 3, 2, 1: `stop … requested` (amber, no result), `restart … failed` (red bar, result `ledger:5`), `logs … done` (green, result `ledger:4`).
- [ ] Click the **subject** of row 2. The graph opens with mitm selected.
- [ ] Back, then click **`root`**. Identity/Users opens with root's row marked.
- [ ] Back, then click **row 2 anywhere but a link**. `/default/ledger/portal/2` opens: action `restart`, outcome `failed`, `answered by` `ledger:5`.
- [ ] Click **`ledger:5`**. `answers` links `ledger:2`, status `failed` in red, exit `1`, the output in a monospace box, `written by` `Emitter.Ledger 0.4.0 · for default · 2026-10-09T02:50:59.400Z` and `line` `ledger.jsonl:5`.
- [ ] Before signing in, open **Environments → Graph** and right-click the **agent** container. You should see Details, Show graph, Runs, Ledger, a rule, then **Logs, Stop, Restart greyed**. Hover shows `sign in to write`.
- [ ] Right-click the **compose node**, an **image** and a **network**: there are no action items on any of them.
- [ ] Give the avatar the printed token. Right-click **agent → Logs**: there is no dialog, and a notice says `asked: logs env:container/claude-agent-dev/agent · ledger:6`. Its link opens the entry, with `answered by: nothing yet: …`.
- [ ] Right-click **mitm → Restart**: a dialog `Restart mitm (…)?` opens with Cancel focused. Press **Escape** or click **Cancel**: there is no notice and no new line.
- [ ] Right-click **mitm → Stop**, then choose **Stop**. The notice reads `… · ledger:7`. Environments → Actions now reads `5 actions · 3 requested · 1 done · 1 failed`.
- [ ] **Keyboard:** focus a container, press **Shift+F10**, then **End**. Focus lands on **Restart**; **Escape** closes the menu.
- [ ] **Environments → Instances**: right-click a container row. The same three items appear.
- [ ] The ledger file `%TEMP%\portal-check-2.16-tenants\default\ledger.jsonl` (while the check runs) has lines 6 and 7 naming `ui:principal:root`, in the contract's member order.
- [ ] **API**: run manual check **1.23** and compare with its Expect.

Nothing in these steps stops or restarts a container: no Runner reads these requests here.

## Full runs

`Invoke-Build`, the default chain of Modules, Build, Test, Smoke and Pester, ends `Build succeeded. 6 tasks, 0 errors, 0 warnings`:
- Go passes, with 112 top-level tests.
- Smoke passes, with a canvas fit ratio of 0.844.
- Pester shows `Tests Passed: 124, Failed: 0, Skipped: 0`.

The `Actions` block's first run caught two of its own assertions failing under strict mode (absent properties) and one regex on a newline. The tests were fixed; nothing in the product changed for them.

Manual checks 1.20 and 1.23 were run as written; their Expects are what I saw. Check 2.16 (the page) has not been clicked by hand. Its Expect is what Pester's `Actions` block asserts in headless Chrome, and its by-hand steps are the check-list above.

## 0.14.1: one ledger, one seq space

Portal.Agent's committed shape puts `action.request` and `action.result` in `tenants/<t>/ledger.jsonl`, one chain both writers append to under the same lock. Portal now serves every line there as `ledger:<seq>`, reads results from that file only, and reads the file's tail before every read and under the lock before every append (docs/design.md 51). Go 114, Pester 125, `Invoke-Build` `Build succeeded. 6 tasks, 0 errors, 0 warnings`.

### Click-through: the result path

With manual check **2.16** running (the page above), play the Runner by hand:

- [ ] Right-click **agent → Logs** as root. The notice reads `asked: … · ledger:6`; its entry says `answered by: nothing yet: no later action.result in the tenant's ledger names ledger:6`.
- [ ] Append the Runner's answer to `%TEMP%\portal-check-2.16-tenants\default\ledger.jsonl` while the portal runs, as manual check **1.23** does: an `action.result` line with `"id":"ledger:7"`, `"request":"ledger:6"`, `"seq":7` and `prev` the SHA-256 of line 6, ending in an LF.
- [ ] **Reload** `/default/actions`, with no restart. Row 6 reads `done` in green with result `ledger:7`, and the toolbar counts one more done.
- [ ] Click **`ledger:7`**. `answers` links `ledger:6`; `written by` names the emitter, version, tenant and stamp from the line's `source`; `line` reads `ledger.jsonl:7`.
- [ ] **Ledger → Entries → portal**: line 7 reads `done · answers ledger:6`, and the chain is still `valid`.
- [ ] Ask once more: the notice reads `ledger:8`, chained to the Runner's line (its `prev` is line 7's hash).

Manual check 1.23 runs the same path over the API and was run as written; its Expect is the output seen. Nobody has clicked this by hand; Pester's `Actions` block asserts the page side in headless Chrome.
