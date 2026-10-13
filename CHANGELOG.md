# Changelog

## 0.16.4 — 2026-10-13

- **The Wheel control on every graph page** (docs/design.md 58). Graph/Nodes, Graph/Explore and Explore's query result each have a Wheel control at the right end of the toolbar:
  - a slider from 0.1 to 5;
  - the speed (`1.0×`);
  - a reset to the tenant default (`↺`).

  Moving the slider changes the mouse-wheel zoom speed of every graph on the page at once, without a redraw. The stats, source chip and query bar keep their places.
- **A per-page override stays in the browser.** It is kept in `localStorage` under `portal:<tenant>:<page>:wheelSpeed` (for example `portal:default:graph/explore:wheelSpeed`) and read when the page opens. An absent or invalid value falls back to the tenant default. Reset removes the key. Nothing goes to the server.
- **The default is the tenant config's `theme.graph.wheelZoomSpeed`** (0.4.1, docs/design.md 37), which now reaches every graph page, not only the Canvas. 1.0 keeps today's step exactly. The brief named a new `ui.graph.wheelSpeed`. This existing key already had the same default, range, schema check and `GET .../config`, so it was kept rather than duplicated (docs/design.md 58).
- **`web/wheel-speed.js`** (new) holds the product's step and the page's speed. `canvas.js` and `graph.js` no longer keep their own copies. Graph/Nodes and query results now record `data-wheel-sensitivity` too.
- **`docs/tenant-ui-settings.md`** (new): the tenant's UI settings, where they come from, and the per-page overrides.
- **`tenants/default/portal.yaml`** names `theme.graph.wheelZoomSpeed: 1.0` with a one-line comment. It is left unstaged (mode dev). `portal.base.yaml` gains the same one-line comment.
- **Tests:**
  - Go `TestWheelZoomSpeed`: `5.0` accepted, `0.05` refused.
  - Pester 166 to 170: the default tenant's key in range and served by `GET .../config`; every module that draws Cytoscape takes its speed from `wheel-speed.js`; the control on Nodes, Explore and a query; the override, its fallback and Reset, driven in Chrome.
  - `Invoke-Build Smoke` adds `checks.wheelSpeed` over every menu page that draws a graph.
- **docs/manual-check-list.md**: new item 2.19; item 2.6's Expect updated.
- No API operation or Kind change. The spec's `Config` description and version move, so `internal/gen` is regenerated.

## 0.16.3 — 2026-10-12

- **A portal that lost its record is found** (docs/design.md 57). The 2026-10-12 Refresh's `Down` passed with nothing recorded while this repo's portal still held 8200 over `artifacts/`, so `Dev` failed. A portal is now this repo's `build/win-x64/portal.exe`, or `portal.exe~` as `go build` renames a binary in use, whose `-artifacts` is the same directory. That narrows entry 32: any other port owner is still never stopped.
  - `Stop-Portal` (and so `Down`) stops the recorded portal and every unrecorded one over its artifacts, saying which with `-Verbose`.
  - `Get-PortalStatus` reports an unrecorded one: `Running` true, new property `Recorded` false, its port from its command line. `Up`, `Dev` and `Feeds` replace it; `Status` prints `unrecorded`.
- **`Stop-Portal` keeps the record when the stop fails.** A portal still running 10 s after `Stop-Process` keeps `portal.json`, and Stop-Portal throws naming it. A recorded id that is not such a portal (exited, or reused by another program) is never stopped; the record is dropped.
- **`Start-Portal`** refuses a second portal over the same artifacts, recorded or not. When this repo's portal over other artifacts holds the port, the message names the `Stop-Portal -Artifacts` that frees it.
- **Refresh** checks the port after Portal's `Down`, and fails that step naming the holder.
- **`Build`** removes a `build/win-x64/*.exe~` that no process runs.
- **Pester** 159 to 166: an `Unrecorded portals` block (found, stopped, replaced, left alone over other artifacts, a stale id never stopped, a survivor keeping its record, the matcher over fake process rows) and a Refresh case for the port check.
- **docs/manual-check-list.md**: item 0.4's two paths, broken by an escaped `\r` and `\a` in 0.16.2, are repaired. New item 3.9.
- No API, Kind or binary change. The spec's version moves, so `internal/gen` is regenerated.

## 0.16.2 — 2026-10-12

- **`Invoke-Build Refresh`** (docs/design.md 56): everything down and back up in one declared order.
  - The steps: Portal.Agent `Down`, Portal `Down` and `Dev`, Portal.Emitters `Mount`, Portal.Agent `Up`, one approved snap, then `GET .../emitters` printed as the Catalogue table.
  - One line per step. Each step runs in a child pwsh, its output in `artifacts/refresh/<stamp>/<n>-<id>.log`.
  - A repo not on disk is skipped and said so. So are the four card-only repos, which have nothing to run.
  - `-DryRun` prints the plan and runs nothing.
- **The order is derived and declared.** `docs/refresh/dependency-order.md` records:
  - what each Portal repo runs, calls, imports, restarts and pushes;
  - what each depends on;
  - the sequence;
  - eight conflicts, each resolved. The main one: Portal.Emitters `Mount` restarts Portal twice, so it runs before Portal.Agent pushes its live layers.

  `docs/refresh/order.psd1` holds the sequence as data, and Refresh reads only that file. Before it runs anything, it checks the order against six rules (R1 to R6).
- **The Catalogue check fails, naming the card,** when one that read `ok` in the last Refresh reads `no-layers` or `version-mismatch`. The last passing roll call is `artifacts/refresh/last.json` (gitignored), and a failing one is `failed.json`.
- **`scripts/Refresh-Portal.ps1`** runs the same from any directory.
- **README**: a Build subsection, "Refresh: run this after any change in any repo".
- **Pester** 129 to 159: a `Refresh` block (tag `Refresh`), covering:
  - the order against the doc and the repos on disk;
  - each rule on a broken copy;
  - the run loop over a fake workspace: order, skips, no Docker, the first failure, the dry run;
  - the child-process runner on a scratch build file;
  - the Catalogue rows and every regression case.
- No API, Kind or binary change. The spec's version moves, so `internal/gen` is regenerated.

## 0.16.1 — 2026-10-12

- **The tenant's action chain under its own ids** (docs/design.md 55). Action lines of `tenants/<tenant>/ledger.jsonl` carry `ledger:tenant/<tenant>/<seq>` (Graph.Node 0.4.2's tenant form of `ledger:`), and a result's `request` names that form. They no longer share strings with the agent chain's `ledger:<seq>`. A line still in the old form is read as this tenant's.
- **Repinned to Graph.Node v0.4.2.** `internal/graphnode`, `cmd/graphnode` and the `graph-node` skill are recopied byte for byte from the tag (import path aside); the Pester tag, CLAUDE.md and README name it. `graphnode check-id ledger ledger:tenant/default/23` passes from Portal's own binary.
- **The boot migrates the old ids in place**, under the file lock (`internal/tenant/migrate.go`). Each action line's `id`, and a result's `request`, become the tenant form, keeping the number. `legacyId` (the old id) goes right after `id`. The next line's `prev` is rewritten to the new hash, so the chain still verifies; a link that was already broken stays broken. Every other member and the member order are kept. The file is first copied to `ledger.jsonl.<stamp>.bak` beside it (gitignored). A ledger with nothing to migrate is not touched.
- **The tenant ledger is mounted as `tenant-actions`**, a layer Portal builds itself (`store.Derived`). It is rebuilt whenever the file grows, Portal's appends and the Runner's alike, on the next read of the layers, and it is mounted while the ledger holds an action line.
  - Nodes are the lines (LedgerEntry) and the principals they name. Edges are `prev` (newer to older), `request` (result to request), `subject` (request to its container) and `principal` (request to who asked).
  - Its source block names Emitter.Ledger at the mounted agent ledger layer's version, else the newest Emitter.Ledger card's.
  - The name is reserved (400). The agent chain never reads `ledger:tenant/...` nodes.
- **The query reaches an action's request, container and principal**: `GET .../query?from=ledger:tenant/default/24&direction=out&depth=5` on the dev tenant (`docs/api/query.md` example 4).
- **Environments/Actions** gets an `id` column. A row opens Graph/Explore at `?from=<request id>&direction=out&depth=5`; `queryHref(id, more)` takes the bar's other fields.
- **`docs/api/actions.md`**: the line contract, versioned. Its `ids` section gives the exact format, the migration rule, and what Portal.Agent's Runner must write.
- **Ontology** 0.16.1: edge Kinds `request`, `subject` and `principal`, and property `legacyId` on LedgerEntry. The spec describes the new ids, `legacyId` on ActionRequest and the reserved layer name; `internal/gen` is regenerated. No new operation.
- **Go** 119 to 127:
  - `TestMigrationOfThreeLinesKeepsTheChain`, `TestMigrationLeavesABrokenLinkBroken`, `TestMigrationLeavesAnUnreadableLedgerToTheBoot`.
  - `TestLedgerIDsAreGraphNodeIDs`, `TestTenantActionsLayerMountsWithFourEdgeKinds` (strict validate, every id `ValidID`, both appends without a restart, the reserved name).
  - `TestQueryFromAResultReachesItsRequestSubjectAndPrincipal`, `TestTenantActionsLayerSourceAndTheRollCall`, `TestNoActionLineNoTenantActionsLayer`.
  - The actions and ledger tests expect the tenant form.
- **Pester** 129, the same count. The Actions block runs on the migrated fixture. Its page check reads the `id` column and each row's Explore link, clicks a row through to Explore (from, direction and depth in the bar), and checks the same query from the API reaching the subject and the principal. The ontology check counts 26 edge Kinds.

## 0.16.0 — 2026-10-12

- **The tenant graph query** (docs/design.md 54). `GET .../query` (`queryTenantGraph`) walks every mounted layer at once, breadth-first and in memory, and answers with an envelope (module Portal) plus `paths` and `query`.
  - Starts are `from` (ids, comma-separated) or every node of `kind`. The walk follows the edge kinds `follow` names, `out` (the default), `in` or `both`, to `depth` hops (3; 0 to 10).
  - A node of Kind `until` is kept and not expanded. A node enters the result only when every `where` (`key=value`: `id`, `kind`, `name` or a dotted path into `properties`) matches it, and only then is it expanded.
  - `source` keeps one emitter's layers, entry 53's rule. `limit` caps the nodes (500; 1 to 5000) and sets `truncated` and `truncatedAt`.
  - Edges are every edge between two result nodes. `paths` runs from each start to each node the walk stopped at.
  - Empty is 200. An unknown `from` is 400 `unknown_node`, naming the id. A bad parameter is 400 `bad_request`.
- **One walk, two calls.** `store.RunWalk` reads the graph through `store.Adjacency`: `Neighbours(id, direction, follow)` and `NodesOfKind(kind)`. Those two calls are what a SQLite or graph-store adapter implements. `memGraph` is the in-memory one.
- **`GET .../graph?source=` is the query** with every node a start, the source set and depth 0. The behaviour is unchanged, and its tests are unchanged.
- **Graph/Explore's query bar** (`web/query.js`, `query: true` on the Page). The bar's fields are `from`, `follow`, `direction`, `depth` and `until`; the URL holds them, so a query is a link and round-trips.
  - While the URL holds a query, the result is drawn with the tenant graph's renderer, its paths listed under it (an id selects and centres its node). Without one, the Canvases are drawn as before.
- **Query from here** on every graph page. It is the node menu's fifth item, on the environments graph and environment rows. On a right-click of any Cytoscape node (Graph/Nodes, Explore's Canvases and Explore's result), it is a menu of its own.
- **`docs/api/query.md`**: every parameter, and worked examples against the dev tenant's layers. The examples are a container's network neighbours, a Terraform module's resources, and the agent ledger chain in both directions. The tenant ledger's `action.result` lines are no mounted layer, so the walk cannot reach them.
- **Go** 117 to 119: `TestQueryWalksTheTenantGraph` (a chain a → b → c → d at depth 1, 2 and 3, `until`, `direction=in` and `both`, `where`, two starts, `kind`, truncation, `source`, the 400s, empty 200) and `TestTenantGraphIsTheQueryAtDepthZero`. The spec walk covers the new operation.
- **Pester** 128 to 129: the Live portal block checks Explore's bar. It runs the API call and `graphnode validate`, the page's result and paths, the bar's round trip to the same URL, a Cytoscape right-click's Query from here, and plain Explore. The Environments graph and Actions blocks' menus gain Query from here, clicked through to Explore.

## 0.15.0 — 2026-10-11

- **The tenant graph** (docs/design.md 53). `GET .../graph` (`getTenantGraph`) serves every node and edge of the mounted layers as one envelope, module Portal, the nodes keeping their layers' Kinds; the first id wins in mount order, and an edge is kept when both its ends are.
- **`?source=<emitter>`** keeps the layers whose Graph.Node source block names that emitter (`source.emitter`, exact and case-sensitive), their nodes, and only the edges between them. No match is an empty envelope, never 404. The filter keys on the emitter's name, not an id prefix: one emitter can own several prefixes, and the source block is what the roll call already matches by.
- **Graph/Nodes at `/<tenant>/graph`** (`getGraphPage`, `view: graph`, `web/graph.js`), drawn with the vendored Cytoscape. `?source=` is shown as a chip whose X links to `/<tenant>/graph`; the filter is the URL, never page state. No match is the empty state with the chip.
- **Emitters/Catalogue** gets a `nodes` column (the roll call's new `nodes` property, the count `?source=<name>` serves; 0 for an unstamped module) and a right-click menu on each row, the container menu's code: Show graph (`/<tenant>/graph?source=<name>`) and Open card (`/<tenant>/emitters/<name>`), each greyed with why where it leads nowhere. An emitter's page has Show graph beside its version.
- **Go** 116 to 117: `TestTenantGraphFiltersBySourceEmitter` (match, no match, edge pruning, the roll call's nodes); the spec walk covers both new operations.
- **Pester** 126 to 128: the `Emitters` block checks the nodes column in every row, the counts against the API with edge pruning, the menu's hrefs and greyed items, Show graph through to the chip and its X, the empty state, and the Show graph links on the emitter pages.

## 0.14.2 — 2026-10-11

- **Card files with non-ASCII names** (docs/design.md 52). `POST .../feeds/{feed}/push` decodes a filename sent as RFC 2047 encoded words, as .NET's multipart (PowerShell's) sends a non-ASCII one. Portal.Emitters' Emitter.Terraform push was refused for `=?utf-8?B?...?=`, its `tests/fixtures/hcl/enc-日本/main.tf`.
- **A path segment may hold Unicode letters, decimal digits and (after the first character) combining marks.** The first-character and separator rules are kept. Control characters, `..`, empty segments and invalid UTF-8 are refused.
- **Go** 114 to 116: `TestNonASCIIFilenamesArePushedAsTheirPath` and `TestNewCardTakesUnicodeSegments`.
- **Pester** 125 to 126: the `Feeds` block pushes a card holding `enc-日本/main.tf` with `Publish-PortalCard`.

## 0.14.1 — 2026-10-11

- **One ledger, one seq space** (docs/design.md 51). Portal.Agent's committed shape: `action.request` and `action.result` both live in `tenants/<t>/ledger.jsonl` as one chain, appended by Portal and by the Runner with the same lock, `seq` and `prev`. 0.14.0's split chains are undone.
  - A request is served under its own id, `ledger:<seq>`; every line of the portal chain is `ledger:<position>`. `ui:ledger:portal/<seq>` and `lineId` are gone.
  - The loader reads `action.result` lines (`kind`, `id`, `request`, `status`, `started`, `finished`, `exit`, `output`, `source`, `seq`, `prev`; nothing else). `source` is a one-item list, or the one object Runner 0.2.0 writes, read as the list of one. A result is served with `writer` (that source) and a source block naming its emitter, `ledger.jsonl` and the line.
  - Results come from `ledger.jsonl` only, never from a mounted layer or an alert layer's copy. A result answers only a request before it.
- **`record` takes the Runner's lock** (no sharing on Windows, `flock` on Unix, 30 s at most) and reads the file's tail under it before computing `seq` and `prev`. It ends a torn last line with an LF before its own.
- **Reads see lines appended after boot.** Every ledger read takes the lines after the last one it read, up to the last LF; a shorter or missing file is a reset. The ledger page, the actions page and the join show the Runner's results without a restart.
- **Pages.** A result's entry page shows `written by` (emitter, version, tenant, stamp) and `line`; the portal chain's list shows `<status> · answers <request>`. A request's page loses `line id`.
- **Fixtures.** The ledger layer fixture is 0.10.0's again (12 entries, 44 to 55, no action lines). `actions.ledger.jsonl` gains the Runner's results 4 (answering 1, done) and 5 (answering 2, failed).
- **Go** 112 to 114: `TestLinesAppendedAfterBootAreReadAndChainedTo` and `TestRecordWaitsForTheRunnersLock` are new; `TestActionResultsAreReadFromTheLedgerLayerOnly` is now `TestActionResultsAreReadFromTheTenantLedgerOnly`.
- **Pester** 124 to 125: the `Actions` block gains an It for a result the Runner appends while Portal runs.

## 0.14.0 — 2026-10-11

- **Actions on containers: Portal asks, the Runner executes, the ledger records, the graph shows the gap** (docs/design.md 50).
  - `POST .../actions` with `{action, subject}` appends one `action.request` line to `tenants/<t>/ledger.jsonl` and returns it. Its members are `kind`, `id` (`ledger:<seq>`), `action`, `subject`, `principal`, `requested`, `status`, `seq` and `prev`.
  - The principal comes from the token, never the body: a body naming one is 400. A reader or a legacy token is 403.
  - Only logs, stop or restart, on an `env:container/` subject; anything else is 422.
  - Portal executes nothing.
- **Results come from the ledger layer only.** An agent ledger entry with `kind: action.result` (`request`, `status`, `started`, `finished`, `exit`, `output`, its source block) answers the request whose `ledger:<seq>` it names, the newest when several do. An alert layer's copy answers nothing.
  - **Split chains**, as Jerry chose: the request stays `ui:ledger:portal/<seq>`, with `lineId` carrying the contract id, and the result stays `ledger:<seq>`.
- **`GET .../actions`** serves the requests newest first under `ui:actions` (Kind `Actions`: `count`, `requested`, `done`, `failed`). Each request has an `outcome` and joins its result through `answered-by` and `answers`. `GET .../actions/{seq}` serves one request.
- **The menu.** A container's right-click menu, on the graph and on an Instances row, gains Logs, Stop and Restart:
  - Stop and Restart ask first in an in-page dialog.
  - A notice links the new id to its ledger entry.
  - The three are greyed with why while the viewer may not write.
- **Environments/Actions** at `/<t>/actions` has the columns action, subject, principal, requested, status (amber requested, green done, red failed) and result. A row opens the request's ledger entry.
- **Ledger entry pages** link a request to its result and back. A result's output is shown in a `pre` block.
- **Fixtures.**
  - The ledger layer fixture gains entries 56 (logs, done) and 57 (restart, failed, exit 1), so it now holds 14 entries, 44 to 57.
  - `tests/fixtures/ledger/actions.ledger.jsonl` holds three requests; the third is unanswered.
- **Go** 107 to 112: `TestPostActionValidatesAndTakesThePrincipalFromTheToken`, `TestActionsJoinTheirResults`, `TestActionResultsAreReadFromTheLedgerLayerOnly`, `TestActionLinesSurviveARestartAndABadOneStopsIt` and `TestActionSubjectsAreTheGraphsContainers`.
- **Pester** 118 to 124: a new `Actions` block (tag `Actions`) of six Its. The Ledger and Environments graph blocks are updated for the fixture and the new menu items.

## 0.13.0 — 2026-10-11

- **Emitters match by source block, never by name** (docs/design.md 49).
  - An emitter is its card name. A layer belongs to it when its Graph.Node source block's `emitter` is that name. `tenant.EmitterName` (`Emitter.<X>` → `Graph.Emitter.<X>`) is gone.
  - A layer with no source block is listed under its envelope's `module` as `unstamped`, `ui:module:<module>`. It is never matched to a card, even one whose `module` is that name.
- **Roll call.** `GET .../emitters` has one row per card or `source.emitter` name, each with a `status`:
  - `ok`, `version-mismatch` and `no-layers` on a Card, with `layerVersions` so both versions show.
  - `no-card` on an Emitter, `ui:emitter:<name>`.
  - `unstamped` on an Emitter, `ui:module:<module>`.

  Emitter's `unknown` is replaced by `status`. A Layer node carries its `source` block.
- **Emitters/Catalogue** shows a status badge per row, the card's version beside the layers' (red on a mismatch), facets, the card link and the layer count. Status and facet chips keep their filter in the URL (`?status=`, `?facet=`).
- **The emitter's page is its source, built from the card.** It shows module, version, repo, url ("Open in source"), the card link and an emits table (facet, kinds). Its layers table gives each layer's source version, target and stamp, with a mismatched row marked red. The layer page reads `produced by <emitter> <version> (card <v>) <status>`.
- **Lift at Graph.Node v0.4.0.** `internal/graphnode`, `cmd/graphnode`, `.claude/skills/graph-node` (0.4.0) and `tests/fixtures/invalid.graph.json` are recopied from v0.4.0, so 0.4.0 cards push. An emitter card without `module`, `repo` and `emits` is now 422. Pester's pin and CLAUDE.md say v0.4.0.
- **Ledger.** A push writes the card's `module`, `repo`, `url` and `emits` on its line, and the Card node serves them. `LedgerEntry` in the spec gains the four optional properties.
- **Fixtures.** `tests/fixtures/cards/` holds Portal.Emitters v0.2.0's nine emitter cards, byte for byte. `Emitter.Agent` moves from 0.1.3 to 0.1.4. No layer on disk carries a source block yet, so the tests stamp copies of the real layers as each emitter's code would.
- **Go** 106 to 107:
  - `TestEmitterOfALayerIsItsFeedCard` and `TestUnmountedEmitterIs404` are replaced by `TestEmitterCardsOfGraphNode040Push`, `TestEmittersMatchBySourceBlock` (every status, the unstamped FileSystem layer never joining its card) and `TestEmitterNamesAreNeverMapped`.
  - `TestEmitterCardNamedLikeAPageStopsTheBoot` is kept.
- **Pester** 116 to 118. The Emitters block (tag `Emitters`) has four Its: the roll call, the Catalogue with chips, the emitter's source page and the produced-by line.

## 0.12.1 — 2026-10-11

- **New prefixes only, no mapping.** Alerts and environments read the `env:` and `alert:` prefixes only (docs/design.md 48).
  - **Alerts:** a node of Kind `alert` under `alert:<fingerprint>`, 64 lower-case hex (`store.AlertFingerprint`). This covers the reader, the item, the badge's open counts, `?about=` behind the reverse rows, the run's `alerts`, and the copies-only guard `IsAlertLayer`.
  - **Environments:** an `environment.*` Kind under `env:compose/`, `env:container/`, `env:image/` or `env:network/`, checked by the lifted registry (`store.IsEnvironment`). On the page, `nodemenu.isEnvironment` decides which graph nodes are the page's own and which table rows open the node menu.
  - **Dropped:** `alert/<fingerprint>`, `alert:<rule>:<seq>` and bare environment ids are ignored, with no fallback.
- **Lift at Graph.Node v0.3.3.** `internal/graphnode`, `cmd/graphnode`, `.claude/skills/graph-node` (0.3.3) and `tests/fixtures/invalid.graph.json` are recopied from v0.3.3. That release registers `env:` to Claude.Agent.Orchestrator, accepts `alert:<fingerprint>`, shares `alert:` and `ledger:`, and lets an alert's `about` edge dangle as a warning. Pester's pin is `v0.3.3`.
- **Fixtures are real.**
  - `agent-environment/` and `agent-alerts/` are Portal.Agent's environment and alert layers from the Up of 2026-10-11 19:00, byte for byte: nine `env:` nodes, and one info alert about `ledger:303`.
  - `agent-environment-tagged/` is rebuilt from the new layer (group and tags on three instances, no env on the ledger keeper, hosts onto the agent-runs run).
  - The old files are deleted. Go and Pester mount five varied alerts inline in the new form.
- **Go** 103 to 106:
  - New: `TestAlertsOfTheRealLayer`, `TestOldAlertIDsAreIgnored` (an `alert/<fp>` layer mounts and shows zero alerts) and `TestEnvironmentGraphIgnoresBareIDs`.
  - `TestAlertsOfTheFixture` is now `TestAlertsOfTheVariedLayer`.
  - The environment and spec tests use the `env:` ids and the real alert.
- **Pester** 115 to 116:
  - The Environments graph and Alerts blocks use the new ids and counts.
  - New: an It mounting the real alert layer by path.
  - The version pins and the Graph.Node tag moved.
- **Docs.**
  - The spec's text, `ontology.yaml`'s Alert idRule, ONTOLOGY.md, README, CLAUDE.md's pinned tag, and manual checks 1.21, 1.22, 2.14 and 2.15.
  - The envgraph empty state names the `env:` forms.
- **Version.** 0.12.1 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 48.

## 0.12.0 — 2026-10-11

- **Alerts.** `GET /api/tenants/{tenant}/alerts` (`listAlerts`) serves the alerts of every mounted layer: nodes of Kind `alert` under `alert/<fingerprint>`, from Portal.Agent's agent-alerts layer. They come as Kind `Alert`, critical first, newest first, 100 a page (`after`), under `ui:alerts` (Kind `Alerts`: `count`, `open` by severity, `rules`, `states`, `next`). Filters are `severity`, `rule`, `state`, `run` and `about`. `GET .../alerts/{fingerprint}` (`getAlert`) serves one alert. An alert's subject (`about`) and Run (`raised-in`) are served as Portal serves them. The later stamp wins on a fingerprint.
- **One resolver** for the environments graph's ends and an alert's subject and run (`internal/server/resolve.go`). It also serves a Decision and an agent ledger entry. `nodemenu.detailsHref` is its page side.
- **Alert layers are copies.** A layer holding alerts is never read as the agent's ledger, as environments or on a table page. Its other nodes are copies of what its alerts are about, there so that its edges do not dangle.
- **Compliance/Alerts** at `/<tenant>/compliance/alerts`, first under Compliance (`view: alerts`, `web/alerts.js`).
  - **Rows:** severity in colour, rule, summary, subject and run linking their pages, stamp.
  - **Chips:** severity, rule and state, through the URL.
  - **Item page:** `/<tenant>/compliance/alerts/<fingerprint>`, with `about` and `raised-in` as links.
  - **Menu:** right-click Details, Subject, Run and Ledger, greyed with why.
- **Badge.** The banner shows the open count at its top severity beside the mode: red for critical, amber for warning, none for info only.
- **Reverse links.** Ledger entries, run and decision rows, and item pages get an alerts row. Run gains `alerts`, and the agent page an alerts column.
- **Boot.** A Compliance page `alerts` with another view stops the boot.
- **Ontology.** Kinds `Alerts` and `Alert`, edges `has-alert`, `about`, `raised-in`.
- **Fixture.** `tests/fixtures/layers/agent-alerts/` is hand-made in the brief's shape.
- **Go** 96 to 103: `TestAlertsAreEmptyWithoutAnAlertLayer`, `TestAlertsOfTheFixture`, `TestGetAlert`, `TestAlertsFiltersAndPages`, `TestAlertLayerIsNotTheLedgerNorAnEnvironment`, `TestAlertReEmitWins`, `TestAgentRunsCountTheirAlerts`. The spec test exercises both operations and the page. The bad-overlay table covers `compliance/alerts`. The menu test expects Alerts first.
- **Pester** 109 to 115: the Alerts block, in Chrome. The version pins and ontology counts moved.
- **Docs.** Manual check item 2.14's fixture paths are repaired.
- **Version.** 0.12.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 47.

## 0.11.0 — 2026-10-11

- **The environments graph.** `GET /api/tenants/{tenant}/environments/graph` (`getEnvironmentGraph`) serves every `environment.*` node of the mounted layers, with its layer's id, Kind and properties (first id wins). It also serves every layer edge touching one, with its kind kept (`member-of`, `runs-as`, `hosts`), and the node one hop away. A Run comes as `listAgents` serves it, with `layer` and `ledger`. A Principal comes as `listPrincipals` serves it, with dev-bootstrap made up. `has-run` joins a served principal to a served run it holds. An edge to a node nothing serves is left out. The response is empty, not 404, without an environment layer. No Kind is added to `ontology.yaml`.
- **Environments/Graph** at `/<tenant>/environments/graph`, first under Environments (`view: environments-graph`, `web/envgraph.js`).
  - It is drawn in SVG, so every node is a link to `#<id>`. A force layout settles before the first paint. A drag pins a node and a double-click releases it.
  - Colour is the instance's kind and shape is the node's Kind. A node one edge away is smaller and dimmer. `member-of` is solid, `runs-as` dashed, `hosts` dotted.
  - Hulls group nodes by the `env` they carry (`unset` without one), with `group` hulls inside them. The env and tag chips are Instances' chips.
  - The node after `#` is selected and pinned with its neighbours, across a reload and Back. `/<tenant>/environments/graph/<id>` becomes `#<id>`.
- **The node menu** (`web/nodemenu.js`). Right-click, Shift+F10 or the menu key on a graph node or on a row of an `environment.*` page opens Details (the item page), Show graph, Runs and Ledger. Each item is a link, greyed with why when no edge leads to it. Arrow keys, Home, End and Escape work in it.
- **Boot.** An Environments page with the id `graph` and a view other than `environments-graph` stops the boot.
- **Shell.** Back or Forward that changes only the fragment goes to a page's `onHash` when the page has one, instead of drawing the page again. `nodes.chips` takes an aria-label.
- **Fixtures.** `tests/fixtures/layers/agent-environment/` is Portal.Agent's real agent-environment layer from Up, byte for byte. `agent-environment-tagged/` is the same layer with `env`, `group` and `tags` added, and its `hosts` edge pointing at the agent-runs run.
- **Go** 91 to 96: `TestEnvironmentGraphIsEmptyWithoutAnEnvironmentLayer`, `TestEnvironmentGraphOfTheRealLayer`, `TestEnvironmentGraphOneHopNeighbours`, `TestEnvironmentGraphFirstLayerWins`, `TestEnvironmentGraphJoinsThePrincipalToTheRunsItHolds`. The spec test exercises the operation and the page. The bad-overlay table covers the `graph` page. The menu tests expect Graph before Instances.
- **Pester** 105 to 109: the Environments graph block in Chrome. The version pins moved.
- **Version.** 0.11.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 46.

## 0.10.0 — 2026-10-11

- **Ledger chains.** Kinds `Ledger` (`ui:ledger:<source>`: `count`, `valid`, `first`, `last`, `breakAt`, `reason`, `stamp`; `layer` and `claimed` for the agent's, `unchained` for Portal's) and `LedgerEntry` (`seq`, `stamp`, `prev`, `hash`). Edges `has-entry`, `prev`, `records` (an entry to its Run or Step) and `recorded-in`. Run gains optional `ledger`. No new id prefix: the agent's entries keep their layer's `ledger:<seq>` ids, and Portal's are `ui:ledger:portal/<seq>`.
- **The agent chain** is read from the mounted layer holding LedgerEntry nodes under `ledger:` ids. Portal checks each entry's `prev` against the hash of the entry before it, and that seq has no gap. The layer root's own verify can only make the chain broken, never valid.
- **Portal's push ledger is a chain.** Each new line of `tenants/<tenant>/ledger.jsonl` carries `seq` (its line number) and `prev` (the SHA-256 of the line before; 64 zeros for the first). Lines from before 0.10.0 are an unchained head. A hand edit breaks the chain at the next line. The boot does not stop for it.
- **API.** `GET /api/tenants/{tenant}/ledger` (`listLedgers`), `GET .../ledger/{source}` (`getLedger`: newest first, `limit` default 100, `before`, `run`) and `GET .../ledger/{source}/{seq}` (`getLedgerEntry`). 404 for an unknown source or seq. The push's `LedgerEntry` gains `seq` and `prev`.
- **Pages.** `GET /{tenant}/ledger`, `/{tenant}/ledger/{source}` and `/{tenant}/ledger/{source}/{seq}`.
  - Ledger/Entries is now `view: ledger` (`web/ledger.js`). Each chain shows valid in green, or broken in red with the seq it breaks at.
  - A chain's entries link each run to the runs layer's page, with the Run's or Step's id after `#`.
  - The agent page's runs gain a `ledger` count linking to that run's entries.
  - Ledger/Verify and Keys keep their paths. A ledger page with the id `agent` or `portal` stops the boot.
- **Fixtures.** `tests/fixtures/layers/ledger/` (a good chain, seq 44 to 55, recording the agent-runs fixture's run and steps) and `ledger-broken/` (entry 50 altered, so the chain breaks at 51).
- **Go** 86 to 91: `TestKnownGoodLedgerVerifies`, `TestAlteredLedgerEntryBreaksTheChain`, `TestEveryRecordsEdgeResolvesToTheRunsLayer`, `TestLedgerEntriesNewestFirstInPages`, `TestPortalLedgerChainsItsPushes`. The spec test exercises the six new operations; `TestLedgerEntryShape` checks `seq` and `prev`; the bad-overlay table covers a ledger page named `portal`.
- **Pester** 103 to 105: the ledger pages in Chrome. The version and ontology pins moved (19 Kinds, 17 edge Kinds), and the Feeds push test checks `seq` and `prev`.
- **Version.** 0.10.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 45.

## 0.9.0 — 2026-10-11

- **Every layer traces back to its emitter.** An emitter is a card of type `emitter` in one of the tenant's feeds, under the card's own id. It is named by the module its layers' ontology names: the card `Emitter.<X>` is the emitter `Graph.Emitter.<X>`. An emitter a mounted layer names with no card in any feed is Kind `Emitter` (`ui:emitter:<name>`, `unknown`, `layers`). New Kind `Layer` (`ui:layer:<name>`). New edges `produced-by` (Layer to Card or Emitter) and `has-layer` (the reverse). Card gains optional `emitter` and `layers`. No new id prefix.
- **API.** `GET /api/tenants/{tenant}/emitters` (`listEmitters`) and `GET .../emitters/{name}` (`getEmitter`, its layers newest first; 404 for a name that is no emitter).
- **Pages.** `GET /{tenant}/emitters` (`getEmittersPage`) and `GET /{tenant}/emitters/{name}` (`getEmitterPage`).
  - Emitters/Catalogue is now `view: emitters` (`web/emitters.js`), linked at `/<tenant>/emitters`. Unknown emitters show in amber. A card links to its row on the feeds page, which marks the row a `#<card id>` names.
  - An emitter's page lists its layers, each linking its layer page. The layer page has a produced-by line linking back.
  - Emitters/Sources, Runs and Schemas keep their paths under `/<tenant>/emitters/`. A ledger line pushing an emitter card named like one of those pages stops the boot.
- **Fixture.** `tests/fixtures/cards/Emitter.Agent/`: Portal.Emitters' Agent card at cd862d9.
- **Go** 83 to 86: `TestEmitterOfALayerIsItsFeedCard`, `TestUnmountedEmitterIs404`, `TestEmitterCardNamedLikeAPageStopsTheBoot`. The spec test exercises the four new operations; `TestShellRoutes` covers the emitters routes.
- **Pester** 101 to 103: the emitters pages and the produced-by line in Chrome. The version and ontology pins moved (17 Kinds, 13 edge Kinds).
- **Version.** 0.9.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 44.

## 0.8.0 — 2026-10-11

- **Agents are principals.** There is no Agent Kind. An agent is a principal of type `agent`, or one whose token mounted a layer whose module is Graph.Emitter.Agent and which holds a Run. Runs mounted with no principal's token (a token from before 0.5.0, or a `-mount` flag at start) are dev-bootstrap's. When principals.yaml lacks dev-bootstrap, it is served with no `created`.
- **Runs keep the emitter's ids.** New Kinds `Run` (`agent:run/<stamp>`: `layer`, stamps, `steps`, `allow`, `deny`, `witness`) and `Decision` (`agent:run/<stamp>/step-<n>/<pre|post>`: `layer`, `effect`, `phase`, `rule`, `step`, `reason`). New edges `has-run` (Principal to Run) and `has-decision` (Run to Decision). Portal mints nothing under `agent:`. The portal check takes those ids only in what the API serves; the screen refuses them (`id-prefix`).
- **API.** `GET /api/tenants/{tenant}/agents` (`listAgents`) and `GET .../agents/{name}/runs` (`listAgentRuns`, runs newest first with their decisions; 404 for a name that is no agent). `LayerSummary` gains `mountedBy`.
- **Pages.** `GET /{tenant}/agents` (`getAgentsPage`) and `GET /{tenant}/agents/{name}` (`getAgentPage`, 404 for a name that is no agent). Identity/Agents (`view: agents`) is now `web/agents.js`, linked at `/<tenant>/agents`. An agent's page lists its runs, the deny count in red, the layer linking to its page and each decision to its row there: the layer page marks the node a `#<id>` names. `principals.js` draws Identity/Users only. A menu service with the id `agents` is refused at boot.
- **A mount records who made it.** The Mount node gains `mountedBy`, so a path mount's runs keep their agent after an approval and a restart.
- **Fixture.** `tests/fixtures/layers/agent-runs/`: Portal.Emitters' `emitters/Agent/artifacts/layers` runs layer and its ontology at cd862d9.
- **Go** 79 to 83: `TestAgentsGroupRunsByTheMountingPrincipal`, `TestAgentsLegacyMountAndASharedRunID`, `TestAgentOfAPathMountSurvivesApproveAndRestart`, `TestScreenRefusesABorrowedID`. The spec test exercises the four new operations; `TestBadOverlayFailsBoot` covers the `agents` service id.
- **Pester** 100 to 101: the agents pages in Chrome (counts, the red deny, the layer link, each decision link and the marked row). The Identity/Agents empty state's text changed.
- **docs/manual-check-list.md**: items 1.18 and 2.11. Three lines from 0.7.0 (items 1.17 and 2.10) held control characters where `\t`, `\f` and `\2026` had been read as escapes; they are repaired.
- **Version.** 0.8.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 43.

## 0.7.0 — 2026-10-11

- **A layer has an address.** `POST /api/tenants/{tenant}/layers` answers 201 with `Location: /<tenant>/layers/<name>`, the layer's page.
- **The layer page.** `GET /{tenant}/layers/{name}` (`getLayerPage`, new) serves the shell for a mounted layer and is 404 for any other name. The page shows the layer's name as its title, a toolbar badge `<n> nodes · <m> edges · <stamp>`, and its nodes in the table the Graph pages use (`nodes.table`, now shared). The banner's search filters the rows.
- **Graph/Layers** (new, `view: layers`, `web/layers.js`) lists the mounted layers, each linking to its page. A menu service with the id `layers` is refused at boot.
- **`POST /api/layers` is gone.** It answers 410 `gone`, naming `POST /api/tenants/<tenant>/layers`, and mounts nothing. A write without a token is still 401 first.
- **CLAUDE.md** lists the Portal.* seeds (Tools, Skills, Workflows, Emitters, Ontology) among the read-only siblings.
- **Pester.** Block "Up replaces a running portal" waits for port 18200 after each test. New tests: Location and 410; Graph/Layers and the layer page in Chrome.
- **Go.** New test `TestMountLocationIsTheLayerPageAndTheOldRouteIsGone`. The spec test exercises `getLayerPage` and expects 410 from `POST /api/layers`. `TestMenuShape` expects Graph's pages `Explore, Kinds, Taxonomy, Layers`, `TestShellRoutes` covers the layer page, and `TestBadOverlayFailsBoot` covers the `layers` service id.
- **Version.** 0.7.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated. docs/design.md 42.

## 0.6.0 — 2026-10-11

- **Stock services.** `compose.yaml` (new), project `portal-<tenant>`, data under `<tenants>/<tenant>/data/` (gitignored):
  - `registry:2.8.3`, an OCI registry, on 127.0.0.1:8201;
  - `bagetter/bagetter:1.6.5`, a NuGet v3 feed, on 127.0.0.1:8202;
  - `postgres:16.15-alpine`, database `portal` with a `pg_isready` health check, not published and not used yet.

  `PORTAL_NUGET_KEY` and `PORTAL_POSTGRES_PASSWORD` live in `.env`.
- **Feeds in the config.** `feeds:` in `portal.base.yaml` (`cards-oci`, `cards-nuget`): `id`, `label`, `kind` oci or nuget, `url`, `visibility` private or public, `token_env`. A tenant's `portal.yaml` overrides a feed by id or adds one; a bad feed stops the binary.
- **Feeds as a graph.** Kinds `Feed` (`ui:feed:<id>`) and `Card` (`ui:card:<feed>/<name>@<version>`) and the edge `has-card` in `ontology.yaml`. `GET /api/tenants/{tenant}/feeds` serves them, the cards taken from the ledger.
- **Push.** `POST /api/tenants/{tenant}/feeds/{feed}/push` takes multipart: `manifest` and one `file` part per file.
  - It is contributor or admin only (401 and 403 from the existing middleware).
  - The manifest is checked with Graph.Node 0.3.0's `graph.ValidateManifest` (422 with `schema` problems), and file paths with the new code `file`.
  - The card goes on as an OCI artifact (ORAS-compatible, one layer per file) or a `.nupkg` (the card's files at the package root).
  - A duplicate name and version is 409; a feed that fails is 502; an unset key is 422 `no_token`.
- **Ledger.** `tenants/<tenant>/ledger.jsonl` (gitignored) gets one appended line per push: `at`, `principal`, `name`, `role`, `feed`, `kind`, `card`, `version`, `type`, `hash` (the content hash), `digest`, `files`, `result` (pushed, duplicate, invalid, failed), `message`. design.md 2 and CLAUDE.md name it, and the push, as the binary's only writes beyond `-artifacts` and the identity files.
- **Page.** Artifacts/Feeds (new, first under Artifacts; `view: feeds`, `web/feeds.js`) shows each feed and its cards with version and hash. It is read-only.
- **PowerShell.** `Publish-PortalCard -Feed -Path [-Url]` (ninth export) pushes a card folder and returns its ledger line. The private `Initialize-PortalFeedEnv` sets the Compose environment and writes the secrets to `.env` once. `Invoke-Build Up` reads `PORTAL_NUGET_KEY` from `.env` the same way (private `Get-PortalEnvValue`, read only), so a portal Up starts can push to cards-nuget.
- **Invoke-Build Feeds.** It refuses in mode prod before building. Otherwise it brings up `compose.yaml` for the tenant, runs Up, pushes `tests/fixtures/cards/Tool.Git` (Portal.Tools' card) to both feeds and checks them.
- **CLAUDE.md** names the lift's pinned tag as v0.3.0, and the ledger and push in the data-layer rule.
- **Lift at Graph.Node v0.3.0.** `internal/graphnode`, `cmd/graphnode` (adds `validate-manifest`, `validate-index`, `index`) and the `graph-node` skill (0.3.0) are recopied; Pester's `$GraphNodeTag` is `v0.3.0`.
- **Version.** 0.6.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated.
- **Tests.**
  - Go: new `internal/feed` tests and the `feedtest` fakes; `internal/tenant/feeds_test.go`; `internal/server/feeds_test.go`; both new operations in the spec test.
  - Pester: a new `Feeds` block.

  The before and after counts are in the 0.6.0 report.

## 0.5.1 — 2026-10-11

- **Tenant mode.** `mode: dev | demo | prod` in `tenants/<tenant>/portal.yaml`, prod when absent (`schema/portal.schema.json`).
  - Served by the new `GET /api/tenants/{tenant}` (`{name, mode}`) and in the merged config.
  - Printed at boot, and shown in the banner as a badge.
  - Mode dev adds a persistent amber bar: "DEV MODE — auto-minted admin, local sign-in shortcut. Not for demo or production. See README: Before you demo." Mode demo adds a thin blue "DEMO" bar. Neither has a dismiss control.
  - `tenants/default` is set to `mode: dev` for now.
- **`Invoke-Build Dev`.**
  - It refuses, building and minting nothing, unless the tenant's mode is dev.
  - It makes principal `dev-bootstrap` (admin, `createdBy` null) in the gitignored `principals.yaml`, replaces every earlier dev-bootstrap token in `tokens.yaml` with a new one, sets `PORTAL_TOKEN`, runs Up and prints the token once, DEV ONLY.
  - `Invoke-Build Up` is unchanged and never mints.
  - The minting is a private `New-PortalDevToken`; mode is read by a private `Get-PortalTenantMode`.
- **Browser sign-in.**
  - The avatar shows the principal's name and role (`jerry · admin`).
  - In mode dev, the avatar's box adds "Use dev token", read from the new `GET /api/tenants/{tenant}/dev-token`. That route answers only in mode dev, to a loopback caller with a loopback Host, while the binary holds a live dev-bootstrap token from `PORTAL_TOKEN`; anything else is 404.
  - At boot the page drops a token the portal refuses and reads as anonymous.
- **dev-bootstrap is refused outside mode dev.** Its tokens get 401 naming the mode, and the boot warns while any is left.
  - It may sit anywhere in `principals.yaml` with `createdBy` null.
  - `-Bootstrap` ignores it, and appends the real first admin beside it.
  - The name is reserved for `POST .../principals` (422) and `New-PortalToken -Name`.
- **Guards.**
  - CLAUDE.md: stop before screenshot or demo work while any tenant is in mode dev.
  - README: "Dev mode" and "Before you demo".
  - Pester fails while `tenants/default/portal.yaml` is committed or staged with mode dev, so it fails now, as asked.
  - `Invoke-Build Screenshot` refuses while that file is mode dev.
- **Version.** 0.5.1 in `Portal.psd1`, `ontology.yaml`, `ontology.go` and `api/openapi.yaml`; `internal/gen` regenerated.
- **Tests.**
  - Go 63 tests, all passing (8 new: `internal/tenant/devmode_test.go`, `internal/server/devmode_test.go`).
  - Pester 87: 86 passed and 1 failed, the commit guard, by design while mode dev is staged (14 new, `Describe 'Dev mode'`; the reader and dev-token paths are driven in Chrome over DevTools).
  - Smoke passed.
- **Manual items.** 1.15, 2.8 and 3.6 are new; the 1.14 Expect is updated (docs/design.md 39).

## 0.5.0 — 2026-10-11

- Identity. New Kind `Principal` (`ui:principal:<name>`): `type` (user or agent), `role` (reader, contributor or admin), `created`, and `createdBy`, absent only on the first admin. Kept in `tenants/<tenant>/principals.yaml` (gitignored; example committed) and served by `GET /api/tenants/{tenant}/principals` as an envelope. An admin adds a user with `POST .../principals`; agents are refused, to be minted via Marker later (docs/design.md 38).
- Tokens belong to a principal: a `tokens.yaml` entry is `{principal, sha256, created}`.
  - `New-PortalToken -Name <principal>` issues a token for an existing principal and throws for an unknown one. A principal may hold several tokens.
  - `New-PortalToken -Name <name> -Bootstrap` makes the first admin (`createdBy` null), once; a second is an error.
  - `GET .../tokens` lists tokens by principal, the first 12 hex digits of the hash, and created. `DELETE .../tokens/{id}` revokes one (admin only).
- Roles are enforced: reader is GET only, contributor makes every write but identity and settings, admin makes all. A write the role does not allow is 403 `forbidden`, a toast on the page; anonymous writes stay 401. Attenuation: no principal creates one of a role above its own. The server checks this first, and the boot checks it over the file.
- An old `tokens.yaml` entry with `name` and no principal works as admin until revoked, and the binary names it in a warning at boot. It cannot add users.
- The binary now writes the tenant's two identity files, atomically, on Add and Revoke, and nothing else outside `-artifacts`; design.md 2 and CLAUDE.md say so. Each such write re-reads both files, so a token minted since boot goes live with it.
- `whoami` adds `id`, `type`, `role`, `legacy` and `administers`. Service connections record `created_by` (served as `createdBy`, shown on the page).
- Page: Identity/Users lists users and has Add (name and role; admin only; roles above yours disabled). Identity/Agents is an empty state ("minted via Marker, later"). Settings/Tokens has Revoke (admin only; a second click confirms). Read-only tooltips name the role. New screenshot `identity-users.png`.
- Go tests 55, Pester 73. New manual items 1.13, 1.14 and 2.7; 3.5 rewritten; every item that mints a token bootstraps first.

## 0.4.1 — 2026-10-10

- Read-only page for an anonymous viewer. While whoami says the principal may not write, every control that would write is disabled with the tooltip `sign in to write`: a panel's collapse/expand, the Designer's section toggles, Approve, Add Panel, Add Container, Add Category and Add Menu Entry. A write the portal still refuses (401, or any refusal) is a toast at the bottom right (`web/toast.js`), never an error body inside a panel; the collapse toggle used to print the error JSON into the panel, and the Designer's status line printed refusals. A refused collapse no longer redraws the page (docs/design.md 37).
- Config `theme.graph.wheelZoomSpeed` (default 1.0 in `portal.base.yaml`, 0.1 to 5 in `schema/portal.schema.json` and the spec's `Config`). A tenant overlay may override it; a value outside the range stops the binary. The Canvas sets Cytoscape's `wheelSensitivity` to 0.2 × the speed (docs/design.md 37).
- `theme` is now a known top-level config key, so the bad-overlay example in tests and manual item 1.11 is `colours: dark`. New manual items 1.12 and 2.6.
- Go tests 47, Pester 69.

## 0.4.0 — 2026-10-10

- The page is a DevOps-style shell. The banner holds the product, a breadcrumb that mirrors the URL, search, the marker (`none`), help and the avatar. The left rail holds the services: a click opens a service's pages in place and closes the others, a hover on a closed service shows a flyout, the chevron collapses the rail to icons, and Settings is pinned at the foot. The main pane has a title, a toolbar and the content. Routes are `/<tenant>/<service>/<page>[/<id>]` and deep links survive a refresh. Dark theme: cards on a near-black grid (docs/design.md 36).
- Config split. `portal.base.yaml` (embedded) holds product defaults and the menu; `tenants/<tenant>/portal.yaml` is the tenant's overlay, deep-merged over it. Both are validated against `schema/portal.schema.json` at boot, and a failure stops the binary. `PORTAL_TENANT` (default `default`) and `-tenants` say which tenant a process serves.
- API: every path is `/api/tenants/{tenant}/...`. New: `config`, `menu` (an envelope of the new Kinds `Service` and `Page`, edge `has-page`, icons in State `icon`), `auth/whoami`, `connections`, `connections/{name}/repos` (GitHub, as `git.repository` nodes; needs a token though it is a GET) and `nodes?kind=` (one Kind across the screen and the mounted layers). `/` redirects to the first page. The unscoped `POST /api/layers` is kept, deprecated, for Claude.Agent.
- Bearer tokens. `tenants/<tenant>/tokens.yaml` (gitignored) holds SHA-256 hashes. No token reads only, a write without one is 401, and a token the tenant does not hold is 401 on any request. New `New-PortalToken`. The module sends `PORTAL_TOKEN`, and the page takes a token in the avatar panel.
- Service connections in `tenants/<tenant>/connections.yaml` (gitignored; example committed). Settings/Service connections lists them without secrets and has a Test button per connection.
- Every menu leaf is a page. Table pages list their Kind or show an empty state. Graph/Explore draws the Canvas with its layer's counts in the toolbar. Designer/Sections and Designer/Export draw the Designer. The top bar's screen menu and stats are gone. Screenshots are now `summary.png`, `service-connections.png` and `explore.png`.
- Go tests 46, Pester 68.

## 0.3.6 — 2026-10-09

- Graph.Node lift at tag v0.2.3 (1807fd7): `internal/graphnode/schemas`, `internal/graphnode/graph` and `cmd/graphnode` (reports 0.2.3) recopied. The lift brings the `agent:`, `runner:`, `git:` and `ref:` id prefixes, the `triangle` and `star` shapes, display `when` rules and the envelope `source` block. Upstream now registers `ui:` to Portal with Portal's shape, so the lift carries no edit beyond the import path (docs/design.md 34).
- `graph-node` skill recopied at 0.2.3 from Graph.Node v0.2.3. Graph's committed copy is still 0.2.1, so Pester now checks graph-node against the pinned tag, and checks only that Graph's copy is not newer. `run-report` and `git-guard` are still compared with Graph's committed files in full (docs/design.md 34).
- Dev seed: the Runs menu entry `ui:menu:runs` (order 1) opens `ui:canvas:agent-runs`, a Canvas on the `agent-runs` layer below the filesystem Canvas, so a fresh checkout has the Runs view. The seed holds no Mount node: the layer is drawn once Claude.Agent mounts it. The seed is now 20 nodes and 19 edges (docs/design.md 35).
- Smoke expects Graph, Runs and Smoke in the top bar, with its own entry at order 2. `Invoke-Build Screenshot` mounts Claude.Agent's newest agent-runs layer when that sibling has one. Screenshot refreshed.
- Go tests 35, Pester 61.

## 0.3.5 — 2026-10-09

- A mount by path is a Mount node. `POST /api/layers` with `{name, path}` records `ui:mount:<name>` (`layer`, and `path` as given, made absolute) on the screen, so an approved snap mounts the layer again at boot. A directory path mounts its newest `*.graph.json` each time. Mounting the same path again leaves the screen and its stamp unchanged. An inline mount removes the node. Flag mounts (`-filesystem`, `-mount`) are never recorded and win over a recorded layer of the same name. A recorded layer that is gone at boot is printed and skipped (docs/design.md 33).
- Top bar: the stats show the open view's data layer, `<layer> · <n> nodes · <m> edges · <stamp>`, from `GET /api/layers`. The open view is the target of the last menu entry chosen, else the first entry's. The screen graph's counts and stamp move to a `screen …` badge beside it.
- `ontology.yaml` 0.3.5: Kind `Mount`. Go tests 35, Pester 61.

## 0.3.4 — 2026-10-09

- `Invoke-Build Up` replaces a running portal: when `artifacts/portal.json` records a live portal it calls `Stop-Portal`, waits for that port to free (10 s at most, else it throws naming the owner and starts nothing), then starts, printing `replaced <old stamp> with <new stamp>`. `-NoReplace` keeps 0.3.3's throw. The logic is the private `Invoke-PortalUp` and `Wait-PortalPortFree` (docs/design.md 32).
- CLAUDE.md: the lift pin reads Graph.Node v0.2.1, and the read-only skill line names all three copies and what Pester checks them against.
- Pester: replace, nothing to replace, `-NoReplace` and the timeout path, plus a check that Up wires them. Go tests 30, Pester 60.

## 0.3.3 — 2026-10-09

0.3.3: git-guard hook; run-report 0.2.2; graph-node 0.2.1

- `graph-node` skill recopied at 0.2.1. Graph's committed copy is byte for byte the copy at Graph.Node tag v0.2.1. `git-guard` 0.1.0 and `run-report` 0.2.2 already matched Graph's committed files. The `.claude/settings.json` git-guard entry was already in place (docs/design.md 31).
- Graph.Node lift moved from tag v0.2.0 to v0.2.1. The lift covers `internal/graphnode/schemas`, `internal/graphnode/graph` and `cmd/graphnode` (now reports 0.2.1). It brings the `alert:` and `ledger:` id prefixes and their shape checks. The one registered edit still names Portal as the `ui:` owner.
- Pester: `$GraphNodeTag` is now v0.2.1 and the registered edit matches upstream's new `ui:` row. graph-node is also checked against Graph's committed skill (version, full text, no extra files). "names the repo Portal" now skips the read-only graph-node copy. Go tests 30, Pester 55.

## 0.3.2 — 2026-10-09

- `git-guard` skill (0.1.0) copied read-only from Graph's committed HEAD (0.2.8) and wired in a new `.claude/settings.json` as a PreToolUse hook on the Bash and PowerShell tools: an agent's commit, push, tag, `--amend`, `gh pr merge` and `gh release create` are refused (docs/design.md 30).
- `run-report` skill recopied at Graph's committed 0.2.2: the report is written only to this repo's `artifacts/run-report.json`, never to `C:\__Code\Graph\artifacts`. Portal's `.build.ps1` has no Report task and no doc said otherwise, so nothing else needed correcting.
- Pester: both copied skills match Graph's committed copy (`version:` line and full text of every file, no extra files); `.claude/settings.json` holds the git-guard entry once; the hook exits 0, 0, 2, 2, 2, 2 for status, add, commit, push, tag and `gh pr merge`. Go tests 30, Pester 54.

## 0.3.1 — 2026-10-09

- Designer sections collapsible: Approve, Kinds (ontology.yaml), Categories, Add Panel, Add Container, Add Category and Add Menu Entry each have a head row (heading left, the panels' `collapse`/`expand` button right) and a body that collapsing hides. Open or closed is a State node per section, `ui:state:designer:<slug>`, joined from `ui:designer` by `has-state`, so it is in the graph and in snaps. The seed has all seven, Approve and Categories open, the rest collapsed (18 nodes, 15 edges). A snap without them boots with them added at those defaults. A toggle writes the State and updates the section in place without redrawing the page. The page reads a node's own State by id, since the Designer now holds eight (docs/design.md 28).
- Title Case in the Designer: every heading and button, each button saying its section (`Approve`, `Add Panel`, `Add Container`, `Add Category`, `Add Menu Entry`); field labels stay lower case.
- Canvas fit: `cy.resize()` and `cy.fit()` on the next frame after layout and from a debounced `ResizeObserver` on the panel body (which also fires when a collapsed panel expands), only once the container has a size. Smoke loads the page in headless Chrome and fails unless the drawn graph is wider than half its panel: 662 of 903 px on the fixture (docs/design.md 29).
- `ontology.yaml` 0.3.1: State's description and idRule name the section States (prose only). Go tests 30, Pester 46. Screenshot refreshed.

## 0.3.0 — 2026-10-09

- Fixed port 8200: `Start-Portal` and `Invoke-Build Up` take `-Port`, else `PORTAL_PORT` from `.env` (`.env.example` at the root; `.env` gitignored), else the environment, else 8200. Port 0 is refused by the wrappers and by the binary (`-port` now defaults to 8200); a busy port fails naming the port and its owning process, and another is never picked. Tests, Smoke and Screenshot use 18200. New `Invoke-Build Status` and `Get-PortalStatus`: port, URL and boot stamp (docs/design.md 24).
- Menu as nodes: a seeded `Menu` Category; a menu entry is a Container that `is-a` Menu and `opens` the Container or Widget it shows (new edge Kind `opens`). The seed's one entry, "Graph" (`ui:menu:graph`), opens the filesystem Canvas `ui:canvas`. The top bar draws the entries from `GET /api/screen`, knowing only the name Menu; choosing one expands and scrolls to its target. The Designer's "Add menu entry" form is one node and two edges through the existing POSTs, shows in the top bar without a reload, and Approve snaps it (docs/design.md 25).
- Designer lists refresh after every add from `GET /api/screen`: Categories lists every Category as a tree by refines depth, the category and refines dropdowns follow it, and the panel list offers only Categories that reach a renderer. Pinned by a Go test: a Category added refining Canvas is on the screen with its `is-a` edge (docs/design.md 26).
- `run-report` skill recopied from Graph's committed 0.2.3 (skill `version: 0.2.1`); Pester now compares the copy's version and files with Graph's committed skill, not Graph's ModuleVersion (docs/design.md 27).
- Smoke: seed, add a menu entry, approve, Theme opacity, approve, restart, and the entry is in the restarted top bar (headless Chrome `--dump-dom`). design.md 24–27.

## 0.2.0 — 2026-10-09

- Renamed to `Portal` everywhere: Go module `github.com/JerryBalmer1/Portal`, envelope and ontology `module`, spec and page title, docs, skills. PowerShell module `src/Portal` with `Start-Portal`, `Stop-Portal`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`; none clashes with the Graph module (docs/design.md 18).
- Graph.Node lifted again at tag `v0.2.0` (ee4df2c): schemas, `graph` (now with `ids.go` and `ontology.go`), `cmd/graphnode` (adds `prefixes`, `check-id`, `validate --ontology`). The copy's id registry names `Portal` as the owner of `ui:`. Pester compares the lift, the `graph-node` skill (0.2.0) and `tests/fixtures/invalid.graph.json` with the tag (docs/design.md 4).
- Problems carry Graph.Node's `severity`; only errors refuse a write or a mount, and a dangling edge on screen stays an error whatever its prefix (docs/design.md 19). New Graph.Node code `edgeid` for a malformed edge id. The Category-without-properties gap is still Portal's own check, noted for Graph.Node 0.2.1 (docs/design.md 10).
- Boot order: the newest `artifacts/snaps/*.graph.json`, or the one `PORTAL_SNAP=<stamp>` names, else the dev seed. `GET /api/screen` answers `X-Portal-Boot` with the stamp, `seed` or `empty`; `Start-Portal` records it as `Boot` and takes `-Snap`. The screen lives in memory; `artifacts/state/` is no longer written (docs/design.md 20).
- Approve: the Designer's Approve form calls `POST /api/snap` with `{note}`; the snap carries the note as an `approved` envelope finding, and the response adds `note`, `previous` and a `diff` summary against the previous snap. `New-PortalSnap -Note` (docs/design.md 21).
- The Canvas draws Graph.Node 0.2.0 display: `circle`, `square`, `size`, edge `style` (docs/design.md 19).
- Theme Kind (`color`, `opacity`, `font`) joined from a Container by `themed-by`; the seed's `ui:theme:default` on `ui:root` holds base.css's values; the page applies a Theme as the stylesheet tokens of its Container. Theme is a display Kind with State (docs/design.md 5, 22).
- Removed (docs/design.md 23): linux-x64 build output and target; the live state file; the `X-Portal-Mode` header; the `-host` and `-dev` flags; the top bar's snap button; `web.StaticNames`; `vendor/LICENSE` from the embed; an unused CSS rule.
- Smoke: boot from seed, approve, change the Theme's opacity, approve, restart, and assert the restart booted from the second stamp and that the second approval's diff is exactly the Theme node.
- `run-report` skill recopied at Graph's committed 0.2.2. design.md 18–23.

## 0.1.0 — 2026-10-09

- Principle (docs/design.md 1): everything on screen is a graph in the Graph.Node envelope; no UI model of its own; new panel types are Category nodes with `is-a` edges.
- `ontology.yaml`, module `Portal` (its 0.2.0 name), prefix `ui:`: Kinds Container, Widget, Layout, Category, State, Ref; edges `contains`, `laid-out-by`, `is-a`, `displays`, `has-state`; finding Kind `absent`. Passes `graphnode validate-ontology`.
- `api/openapi.yaml` and the server `cmd/portal` generated from it (oapi-codegen v2.8.0, chi v5.3.2): `GET /api/screen`; `POST /api/nodes`, `POST /api/edges`; `PATCH`, `DELETE /api/nodes/{id}`; `GET`, `POST /api/layers`; `GET /api/layers/{name}/nodes`; `POST /api/snap`; `GET /api/diff`; `GET /api/ontology`; `GET /` and `GET /static/{file}` for the page.
- Every write validates the whole next screen with Graph.Node and the touched elements with the portal ontology; 422 with problem codes. Display properties are refused on everything but State nodes.
- Data layers mount read-only from a path or an envelope and are refused with Graph.Node's problem codes when they fail; never written.
- Snaps under `artifacts/snaps/` with millisecond UTC stamps; the diff is Graph.Node design 11's merge, read-only.
- `PORTAL_MODE=production` hides nodes that `is-a` Designer and nothing else.
- Page (`web/`): draws the screen recursively by Layout (grid, flow, canvas), each panel by the base Category its `is-a` chain reaches: Canvas (GraphRenderer's Cytoscape drawing, colour from the layer ontology, grey fallback, label templates), Table, Detail, Designer (lists Kinds, adds panels, containers and Categories through the API). Collapse writes State.
- Dev seed on an empty state: root Container, grid Layout, four Categories, a Canvas Widget on layer `filesystem`, the Designer Container. `filesystem` is the newest Graph.Emitter.FileSystem layer, mounted at startup when present; a copy is in `tests/fixtures/layers/filesystem/`.
- Graph.Node 0.1.0 schemas, `graph` package and `cmd/graphnode` lifted into `internal/graphnode` and `cmd/graphnode`; Pester compares them with Graph.Node's committed HEAD.
- PowerShell module (0.2.0 names: `Portal`, `Start-Portal`, `Stop-Portal`, `Get-PortalScreen`, `New-PortalSnap`, `Compare-PortalSnap`, `Mount-PortalLayer`): start, stop, read the screen, snap, diff, mount a layer.
- Invoke-Build: Modules, Generate, Build, Test, Up, Down, Smoke (writes `artifacts/smoke-report.json`), Pester, Screenshot; default Modules, Build, Test, Smoke, Pester.
- Skills: `manual-check-list`, `ontology-doc`, `readme` (adapted from Graph.Node's), `graph-node` (Graph.Node 0.1.0) and `run-report` (Graph 0.2.0) copied read-only with version-match tests.
- design.md 1–17.
