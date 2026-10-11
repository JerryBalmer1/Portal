# Portal 0.10.0, ledger chains verified and joined to runs: report

Task: five items. (1) Kind LedgerEntry under `ledger:<seq>` with no new prefix, edges `records` and `recorded-in`, and Portal's own push ledger as a second chain under source `portal`. (2) `GET .../ledger`, `.../ledger/{source}` (paged, default 100) and `.../ledger/{source}/{seq}`, with 404s. (3) Pages on the existing Ledger menu entry, run links to the runs layer page by `#<id>`, a ledger count per run on the agent page, and page names reserved in the ledger service. (4) A good and a broken ledger fixture, with a Go test for each and one that every `records` edge resolves to the runs fixture. (5) Go for the routes and Pester for the pages. Decisions: docs/design.md 45.

## Counts

| Suite | Before | After | New |
|---|---|---|---|
| Go top-level tests | 86 pass, 0 fail | **91 pass**, 0 fail | `TestKnownGoodLedgerVerifies`, `TestAlteredLedgerEntryBreaksTheChain`, `TestEveryRecordsEdgeResolvesToTheRunsLayer`, `TestLedgerEntriesNewestFirstInPages`, `TestPortalLedgerChainsItsPushes` |
| Pester | 103 pass | **105 pass**, 0 fail, 0 skipped | `Ledger/Ledger/Entries lists both chains with their counts, the agent chain valid in green; the altered copy is red, broken at 51`; `a chain's page lists its entries newest first, each run linking to the runs layer page by id after #; the agent's run counts its entries` (both in headless Chrome) |
| `go vet` | clean | clean | |
| Smoke | | passed (canvas fit 0.844) | |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` | |

- **Go before** is from this tree before any edit (`go test ./... -count=1 -v`: 86 `--- PASS`, 0 fail). **Go after** is the same command on the finished code (91, 0 fail).
- **Pester before** is `tests/testResults.xml` as I found it: total 103, 0 failures, written at 02:15 on 2026-10-11, the run behind 0.9.0's report. I did not rerun it on the untouched tree.
- **Two full `Invoke-Build` runs.**
  - Run 1 failed 1 test, my own: the run-filtered entries badge read the chain's 12 where the test wanted the run's 9. The page was wrong, not the test; `web/ledger.js` now shows the run's count when `?run=` is given.
  - Run 2, after the docs, passed 105 and ended `Build succeeded`.

## Asked, and your answers

- **Portal's push ledger had no chain.** Its lines carried no `seq`, `prev` or hash.
  - **Your answer: chain new lines.** Every line now gets `seq` (its line number) and `prev`: the SHA-256 of the line before, 64 zeros for the first. That is the agent chain's own rule, as Portal.Emitters' `Read-AgentLedger.ps1` reads it.
  - Lines from before 0.10.0 are an unchained head: counted, never a break.
- **Portal cannot mint `ledger:`.** Graph.Node registers it to Claude.Agent.Alerts, and `ledger:<seq>` would collide with the agent's entries.
  - **Your answer: `ui:ledger:portal/<seq>`.** The chain nodes are `ui:ledger:agent` and `ui:ledger:portal`.

## 1. The ontology

- **`ontology.yaml` 0.10.0** has 19 Kinds and 17 edge Kinds, and passes `graphnode validate-ontology`.
- **Kind `Ledger`, a chain.** Properties: `source`, `count`, `valid`, `first`, `last`, `breakAt`, `reason`, `stamp`, `layer`, `claimed` and `unchained`. One chain's page also gets `entries`, `run` and `next`.
- **Kind `LedgerEntry`.**
  - Every entry: `source`, `seq`, `stamp`, `hash` and `prev`.
  - Agent entries: `event`, `run` and `runLayer`.
  - Portal entries: `by`, `feed`, `card`, `version`, `result`, `message`, `unchained` and `written`.
- **Edges.** `has-entry`, `prev`, `records` (LedgerEntry to Run or Step) and `recorded-in` (the reverse). `recorded-in` is declared but not served; see Not done.
- **Run** gains optional `ledger`, its count of entries.
- **The portal check** takes `ledger:` ids for a LedgerEntry the API serves, the way 0.8.0 lets Run keep `agent:` ids. A Portal entry stays `ui:`.
- **The real Portal.Agent layer was not read**, because CLAUDE.md does not list `../Portal.Agent`. Two consequences:
  - Portal finds the agent's ledger as any mounted layer holding LedgerEntry nodes under `ledger:` ids, whatever its name or module.
  - Portal reads the verify from whatever node the envelope's `root` names. In the fixture, that root is the newest entry, `ledger:55`: `ledger:` allows no other id shape.

## 2. The API

- **`listLedgers`.** Agent first while a ledger layer is mounted, then portal, always.
- **`getLedger`.** Newest first. `limit` is 1 to 1000 (absent or 0 means 100; otherwise 400). `before` pages by seq, and the root's `next` gives the next value. `run` filters to one run.
- **`getLedgerEntry`.** The entry, its chain, and the entry before it, so its `prev` edge always lands. A seq that is no integer is 400.
- **404s.** An unknown source (agent too, while no ledger layer is mounted), or an unknown seq.
- **Verify.**
  - An entry follows the one before when its seq is one more and its `prev` is that entry's `hash`. `breakAt` is the seq of the first entry that does not follow.
  - `reason` is `prev-mismatch`, `seq-gap`, `unchained` or `layer`.
  - The layer root's own verdict is served as `claimed`. It can make the chain broken, never valid: the broken fixture's root still says valid, and Portal reports the break.
- **`records` edges are cross-layer.** They are Graph.Node warnings, resolved at merge. Each entry instead carries `runLayer`, so the page can link to the run.
- **The push's `LedgerEntry` schema** gains `seq` and `prev`. `TestLedgerEntryShape` checks both.
- **Spec and generated code.** Six new operations (three API, three page). `internal/gen` is regenerated, and `web/client.js` has `listLedgers`, `getLedger` and `getLedgerEntry`. The parity tests pass.

## 3. The pages

- **`/{t}/ledger`** (Ledger/Entries, now `view: ledger`, drawn by `web/ledger.js`).
  - One row per chain: source (linking its entries), verify, entries, first, last, stamp, and where it comes from.
  - Verify is `valid` in green, or `broken at <seq>` in red with the reason as its title. The toolbar reads `2 chains · 1 broken`.
- **`/{t}/ledger/{source}`.** Entries newest first, the entry the chain breaks at marked red, and an `older entries` link for paging.
  - Each run id links to `/{t}/layers/<runLayer>#<id>`, where the layer page marks the row. For a `policy.*` entry the link goes to its Step.
  - `?run=` shows one run's entries.
- **`/{t}/ledger/{source}/{seq}`.** One entry: its fields, with `prev` linking the entry before.
- **The agent page** gets a `ledger` column, but only while a ledger layer is mounted, so 0.8.0's row assertions still hold. Its count (9 for the fixture run) links to `/{t}/ledger/agent?run=<id>`.
- **Reservation, as in 0.9.0.**
  - The literal routes serve the ledger service's own pages (Verify, Keys) and their items.
  - The sources are fixed names, so the refusal runs the other way from 0.9.0's: boot refuses a ledger page with the id `agent` or `portal`. Go covers it in the bad-overlay table.
- **Checked headless** (`--dump-dom`) in Pester. Clicks were not exercised by hand.

## 4–5. Fixtures and tests

- **`tests/fixtures/layers/ledger/`.** Twelve entries, seq 44 to 55, module `Portal.Agent` 0.1.0, with an ontology beside it.
  - The lines are synthetic, in the stream-contract shape, with real SHA-256 `prev` and `hash`.
  - Entries 46 to 54 record the agent-runs fixture's run, or its Steps for `policy.pre` and `policy.post`.
  - It passes `graphnode validate --ontology`; the only warnings are the cross-layer `records` edges.
- **`tests/fixtures/layers/ledger-broken/`.** The same, with entry 50's `event` altered and re-hashed. 51's `prev` no longer matches, so the chain is broken at 51 with `prev-mismatch`.
- **Go.**
  - Both fixtures' verify.
  - Every `records` edge of both fixtures resolving to an id in the runs fixture (9 each).
  - Paging, the run filter, the `prev`, `records` and `has-entry` edges, and one entry.
  - Portal's chain: an unchained head, a hand edit breaking it at the next line, and the next push chaining on.
  - The run's `ledger` count, the boot refusal, and the six operations in the every-operation test.
- **Pester.**
  - The two pages, as listed under Counts.
  - The Feeds push test now checks `seq` and `prev` against the line before.
  - The version and ontology pins moved.

## Docs

- **design.md 45.** It also adds one note to entry 40's line "the lines are not nodes", pointing at 45.
- **CHANGELOG 0.10.0.**
- **CLAUDE.md:** the `tests/` layout line names the ledger fixtures.
- **ONTOLOGY.md** (skill `ontology-doc`):
  - Rows changed: version and prefix note; Page's `view`; Run; Ledger; `id-prefix`; Not here yet (the 0.6.0 ledger item split, and a 0.10.0 item added).
  - Rows added: terms Ledger chain, Verify and LedgerEntry; Kinds Ledger and LedgerEntry.
  - Every term resolved.
- **README.md** (skill `readme`):
  - Sections changed: line 1 terms; What is here (`internal/server`, `internal/store`, `internal/tenant`, `web/`, `ontology.yaml`, `tests/`); Use the API (three operations, the push's `seq`/`prev`, page routes); Status (0.10.0).
  - Step 2: every new file sits under an existing row, and no row is missing or out of order. As before, root files have no rows.
  - The PowerShell example is unchanged and was not rerun.
- **docs/manual-check-list.md** (skill `manual-check-list`):
  - Added 1.20, run as written; its Expect is the output seen.
  - Added 2.13; its Expect is the DOM read headless, without the clicks.
  - Changed 0.2 (Go 91, Pester 105, and 104/1 for the dev-mode case) and the Verified line.
  - Not run: as the Verified line lists.

## Not done, and notes

- **For Graph.Node, not made here.** `ledger:` is registered to Claude.Agent.Alerts. If Portal.Agent's layer issues `ledger:<seq>` ids as the brief says, the registry should name the prefix as shared, or `graphnode check-id` will name the wrong owner.
- **Replace the fixture when the real layer exists.** It is built to the brief's shape, not copied, since Portal.Agent is outside the readable siblings.
- **What verify cannot see.**
  - For the agent chain Portal has hashes, not lines, so an edit that leaves `hash` alone is invisible.
  - For its own ledger, an edit to the last line is invisible until the next push.
  - Neither chain has a MAC check.
- **`recorded-in` is declared, not served.** Serving it would need the Run or Step node in Portal's envelope.
- **Ledger/Verify and Keys** still list Kinds nothing emits.
- **No PowerShell wrapper** (`Get-PortalLedger`): the brief did not ask for one.
- **Run-report skipped.** The `run-report` skill was not invoked; you said to stop after this report.
- **`tenants/default/portal.yaml` is not staged.** It holds your uncommitted `mode: dev` and was left as I found it. The Ledger Pester block and item 1.20 use their own tenants (prod and demo). No screenshots or demos were made.

Staged: every file above. Not committed, pushed or tagged.
