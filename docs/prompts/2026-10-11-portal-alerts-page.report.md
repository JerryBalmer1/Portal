# Portal alerts page report (2026-10-11)

Task: five items.

1. A page Compliance → Alerts at `/{t}/compliance/alerts`, first under Compliance: a table, chips, the sort order, and an item page per fingerprint.
2. A badge in the header with the open count by top severity, and the API (`listAlerts`, `getAlert`), written in the spec first.
3. An "Alerts" row on the pages of the things an alert is about, and an alert count on the agent page's runs.
4. A right-click menu on alert rows: Details, Subject, Run, Ledger.
5. The boot rule; Go and Pester tests on a hand-made fixture; design.md 47.

Decisions: docs/design.md 47. Everything is staged; nothing is committed. `tenants/default/portal.yaml` (your `mode: dev`) is left unstaged, as I found it.

## Counts

| Suite | Before | After | New |
|---|---|---|---|
| Go top-level tests | 96 pass, 0 fail | **103 pass**, 0 fail | `TestAlertsAreEmptyWithoutAnAlertLayer`, `TestAlertsOfTheFixture`, `TestGetAlert`, `TestAlertsFiltersAndPages`, `TestAlertLayerIsNotTheLedgerNorAnEnvironment`, `TestAlertReEmitWins`, `TestAgentRunsCountTheirAlerts` |
| Pester | 109 pass | **115 pass**, 0 fail, 0 skipped | six in `Alerts`, five of them in headless Chrome |
| Smoke | | passed (canvas fit 0.844) | |

- **Go before** was run on this tree before any edit (`go test ./... -count=1 -json`, top-level passes). **Go after** is the same command on the finished code.
- **Pester before** was `Invoke-Build Pester` on the untouched tree: 109 passed.
- **Pester runs after the change.**
  1. The first run failed one of my own tests. It was a JS precedence slip in the test script (`await wait(...).click()`), not the page.
  2. After the fix, `pwsh -NoProfile -File .\tests\Invoke-Tests.ps1` passed 115 of 115.
  3. The final default `Invoke-Build` (Modules, Build, Test, Smoke, Pester) ended `Build succeeded. 6 tasks, 0 errors, 0 warnings`. The port race from the last two reports did not hit this time.

## What was built

- **API: `listAlerts` and `getAlert`**, written in the spec first, then generated.
  - **Root.** `GET .../alerts` serves one page under `ui:alerts` (Kind `Alerts`). It carries `count`, `alerts` (the number matching), `open` by severity, `rules` and `states` (for the chips), and `next`.
  - **Order and paging.** Critical first, then newest first, then by fingerprint. Pages of 100; `after=<fingerprint>` gives the next page.
  - **Filters.** `severity`, `rule` and `state`, as you asked. I added two more: `run` (raised in that run) and `about` (about that id). See "Worth a second look".
  - **Alerts.** Each is Kind `Alert` under the layer's own id, `alert/<fingerprint>`. `about` and `raised-in` join it to its subject and its run, when Portal serves them.
  - **One alert.** `GET .../alerts/{fingerprint}` serves the alert, its subject and its run.
- **One resolver** (`internal/server/resolve.go`). The environments graph's `environmentEnds` became it, so the alerts use "the same resolver the environments graph uses".
  - **Old cases:** a Run, a Principal, or any node as its layer holds it.
  - **New cases:** a Decision, and an agent ledger entry.
  - **Page side:** `nodemenu.detailsHref`, which the env menu's Details now uses too. The env graph's output and tests are unchanged.
- **The page** is `web/alerts.js`.
  - **Rows:** severity in colour (critical red, warning amber, info muted), rule, summary (a link to the alert's page), subject, run and stamp. An alert that is no longer open is dimmed.
  - **Chips:** severity, rule and state. A chip sets the URL (`?severity=critical`), so a filter is a link and applies across pages.
  - **Item page:** `/{t}/compliance/alerts/<fingerprint>`, with the fields and the `about` and `raised-in` edges as links.
- **The badge** sits next to the mode badge. It shows `1 critical` in red, or `<n> warning` in amber, and nothing when only info alerts are open. Its title holds every open count. It links to the list filtered to that severity and `state=open`.
- **Reverse links: an "alerts" row** on each of these pages, linking each alert about it:
  - a ledger entry's page;
  - the marked row of a Run or Decision on its runs layer's page, which is those nodes' item page today;
  - any table page's item page, which covers environment instances.

  The agent page's runs get an `alerts` column after `ledger`, linking `?run=`.
- **The menu** is the node menu with its own items: Details, Subject, Run and Ledger. It uses the same greyed-with-why rule as the environments menu. `nodemenu.open` now takes an item list.
- **The boot rule.** A Compliance page with id `alerts` and any other view stops the boot. It is the twin of entry 46's rule for `environments/graph`.
- **Version** 0.12.0.

## The fixture, and a problem the real layer will meet

`tests/fixtures/layers/agent-alerts/` is hand-made. It holds five alerts, all raised in the agent-runs fixture's run. Their subjects are:

- the step-3 pre deny (a Decision);
- `ledger:52` and `ledger:46`;
- the run itself;
- `claude-agent-dev`.

The severities and states vary, so the badge counts one critical, one warning and one info as open.

**A layer of the brief's exact shape will not mount.** Graph.Node lets an edge dangle only when its end's id has another module's prefix. `about` edges to `claude-agent-dev` (a plain id) and to `ledger:<seq>` (Claude.Agent.Alerts' own prefix) are `dangling-edge` errors. So the fixture carries copies of those three subjects, as Portal.Agent's `Export-LedgerAlert` already does for ledger entries.

That forced a new rule in Portal: **a layer holding alerts is read as copies only.** It is never the agent's ledger, never environments and never on a table page, and its copies are never served as a subject. Without the rule, the alert layer's later stamp took the agent chain from the ledger layer, leaving a two-entry chain. `TestAlertLayerIsNotTheLedgerNorAnEnvironment` covers it.

Portal.Agent will have to solve the same problem: carry copies of the subjects, or use ids that may dangle.

## Worth a second look

- **Portal.Agent's alerts code is not the brief's shape.** `agent/Alerts` today emits:
  - Kind `Finding`;
  - ids `alert:<rule>:<seq>`, the shape Graph.Node registers for `alert:`;
  - severities info, warn and error;
  - a `raised-by` edge.

  Portal reads only the brief's shape, so a layer like that mounts and shows no alerts. It also carries `ledger:` copies without an `alert` node, so it is not an alert layer. If mounted after the ledger layer, it would still take the agent chain. That was already true before this change.
- **Two filters you did not name.** I added `run` and `about` because the agent page's count and the subject pages' rows need them.
- **The agent page's count uses `raised-in`.** It counts the alerts raised in the run, not those about it. A run's own page row shows the alerts about it. So the run in the fixture counts 5 on the agent page, and its row lists the one alert about it.
- **No state counts as open.** An alert with no `state` is counted as open.
- **CLAUDE.md now lists `../Portal.Agent` as readable**, with `agent/*/artifacts/` and the future `alert-layers/`, as you asked.
- **I fixed manual check item 2.14.** Its fixture paths held control characters (`\t`, `\f`, `\a` eaten by an earlier script). I made the same mistake once in this session, writing cp1252 into `web/portal.js`; I caught and fixed it, and scanned every touched file. No other file has stray control characters or bad UTF-8.
- **Not checked by hand.** Item 2.15 was verified by Pester's Chrome tests and by `--dump-dom` of each page on a throwaway tenant in mode demo, for my own check only. Nobody has clicked through it with a real mouse.

## Docs

- design.md 47.
- README: intro, the What-is-here rows, Use the API, the routes, Status 0.12.0, and three terms on line 1.
- ONTOLOGY.md:
  - the terms Alert, Alert layer and Alerts badge;
  - the Kinds rows `Alerts` and `Alert`, and `alerts` on Run;
  - a Not here yet line;
  - its stale version (0.10.0) corrected to 0.12.0.
- manual-check-list:
  - new item 1.22, which I ran (its Expect is the output I saw);
  - new item 2.15;
  - item 0.2's counts and the Verified line updated;
  - item 2.14's paths repaired.
- CHANGELOG 0.12.0.
- CLAUDE.md: the readable list and the fixtures in the tests/ layout line.

## Not done

- No PowerShell wrapper for the two routes.
- No way to acknowledge or resolve an alert from the page: Portal never writes one.
- No filter takes more than one value.
- The fixture is not yet replaced by the real layer, which has not landed.
