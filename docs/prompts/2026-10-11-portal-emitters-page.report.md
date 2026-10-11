# Portal 0.9.0, emitters and the layers they produced: report

Task: five items. (1) Emitter as the existing feed Card, with edges `produced-by` (layer to emitter) and `has-layer`; an emitter with no card shown by name, flagged unknown. (2) `GET .../emitters` and `.../emitters/{name}`. (3) The pages `/{t}/emitters` and `/{t}/emitters/{name}` on the existing Emitters menu entry, and a produced-by line on the layer page. (4) A fixture card, plus Go tests that produced-by resolves to it and that an unmounted name is 404. (5) Go tests for both routes, and Pester for both pages. Decisions: docs/design.md 44.

## Counts

| Suite | Before | After | New |
|---|---|---|---|
| Go top-level tests | 83 pass, 0 fail | **86 pass**, 0 fail | `TestEmitterOfALayerIsItsFeedCard`, `TestUnmountedEmitterIs404`, `TestEmitterCardNamedLikeAPageStopsTheBoot` |
| Pester | 101 pass | **103 pass**, 0 fail, 0 skipped | `Emitters/Catalogue lists the card of the agent-runs emitter with its layer count, and the FileSystem emitter in amber, unknown`; `an emitter's page lists its layers with their counts, linking each layer page, which links back by produced-by` (both in headless Chrome) |
| `go vet` | clean | clean | |
| Smoke | | passed (canvas fit 0.844) | |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` | |

- **Go before** is from this tree before any edit (`go test ./... -count=1 -v`: 83 `--- PASS`, 0 fail). **Go after** is the same command on the finished code (86, 0 fail).
- **Pester before** is `tests/testResults.xml` as I found it: total 101, 0 failures, written at 01:48 on 2026-10-11, the run behind 0.8.0's report. I did not rerun it on the untouched tree.
- **Three full `Invoke-Build` runs.**
  - Run 1 came after the code and before the docs: 103 passed.
  - Run 2 came after the docs and failed 1 test: `Boot order` › `Start-Portal -Snap (PORTAL_SNAP) forces an older snap`, with `port 18200 is in use by unknown (process 35700)`. That is the port race in that block's `AfterEach` that design.md 42 named and left. Nothing this task touched runs there.
  - Run 3, unchanged, passed 103 and ended `Build succeeded`. The race is still in the suite.

## Asked, and your answer

- **The clash.** The brief asked for a boot refusal of a menu service named emitters, but portal.base.yaml has one. So the product would not boot, and `/{t}/emitters/{name}` would have captured `/{t}/emitters/catalogue`.
- **Your answer.** Keep the id and drop the refusal. Catalogue becomes the list (`view: emitters`) at `/{t}/emitters`. Sources, Runs and Schemas keep their paths. An emitter card named like one of those pages is a boot error.
- **Done as answered.**
  - `getEmitterPage` serves the shell for any page the menu has under `emitters/`. Its literal route wins in chi, and a 4-segment item route such as `/default/emitters/runs/<id>` still falls through to `getShellItem` (Go checks both).
  - The page's router makes the same choice.
  - At boot, a pushed ledger line of type `emitter` whose card name, or emitter name, equals one of those page ids stops the binary. The error names the entry and the page.
- **Found while testing.** A push cannot create that clash. Graph.Node's manifest schema requires a dotted card name, and a menu id (`^[a-z0-9][a-z0-9-]*$`) has no dot. I first added a push-time refusal as well. It could never fire, so I removed it. The boot check stays, as you asked; it catches a hand-edited ledger. Go covers both facts: a single-word card is 422 `schema`, and a hand-seeded `runs` emitter line stops `tenant.Load`.

## 1. The ontology

- **Known emitter = the Card.** No new Kind, served under the card's own id `ui:card:<feed>/<name>@<version>` with every listFeeds property. It also gets two new optional Card properties: `emitter` (its name) and `layers` (a count).
- **Unknown emitter = new Kind `Emitter`.** Why: a Card is a push to a feed, so `feed`, `version`, `hash` and `pushed` are required, and an emitter known only from a layer has none of them. Id `ui:emitter:<name>`, under Portal's own `ui:`, so there is no new prefix. Properties `unknown` (always true) and `layers`.
- **New Kind `Layer`** for the edges' other end: `ui:layer:<name>` with `emitter`, `module`, `version`, `stamp`, `nodes`, `edges`, `mountedBy` and `path`. Mount was not reused: it is a screen record of a path mount, with `path` required and a remount at boot.
- **Edges.** `produced-by` (Layer to Card or Emitter) and `has-layer` (the reverse).
- **The emitter's name.**
  - It is the `module` of the layer's ontology (the file the envelope's `ontology` field names), or the envelope's `module` when no ontology could be read.
  - Portal.Emitters names its cards `Emitter.<X>` for the module `Graph.Emitter.<X>` (all seven at cd862d9). So the card `Emitter.Agent` is the emitter `Graph.Emitter.Agent`.
  - Your list's "Graph.Emitter.Git.Repo" is read as the module `Graph.Emitter.Git.Repository`, whose card is `Emitter.Git.Repository`.
- **Several cards for one emitter** (versions or feeds): the newest push wins.
- `ontology.yaml` is 0.9.0: 17 Kinds, 13 edge Kinds. Page's `view` now lists `layers` (missing before) and `emitters`. It passes `graphnode validate-ontology` (Pester).

## 2. The API

- **`GET /api/tenants/{t}/emitters`** (`listEmitters`) is an envelope, module Portal. It holds every emitter by name, each followed by its Layer nodes newest first, with `produced-by` and `has-layer` edges.
- **`GET .../emitters/{name}`** (`getEmitter`) is the same for one emitter, rooted at its node.
- **"version, feed, card link, layer count"** are the Card's `version`, `feed`, id and `layers`. No URL is served: the page links the card id to its row on Artifacts/Feeds.
- **404** for a name that is no emitter. That includes the card's own name (`Emitter.Agent`), and an emitter whose last layer was replaced while no card names it.
- **Validation.** Both envelopes pass `graph.ValidateBytes` and the portal ontology in Go.
- **Spec.** Four new operations. `internal/gen` is regenerated, and `web/client.js` has `listEmitters` and `getEmitter`. Route, client and every-operation parity pass.

## 3. The pages

- **`/{t}/emitters`** (`web/emitters.js`, Emitters/Catalogue): one row per emitter, with name (linking its page), version, feed, card (linking `/{t}/artifacts/feeds#<card id>`) and layers.
  - Unknown emitters are amber (the dev bar's hue), with an `unknown` badge and empty version, feed and card.
  - The toolbar reads `2 emitters · 1 unknown`.
  - The rail links Catalogue to `/{t}/emitters` and marks it current.
- **`/{t}/emitters/{name}`**: a badge `<n> layers`, the card's version, feed and link (or the unknown badge), then its layers newest first: name (linking the layer page), nodes, edges, stamp and principal.
- **The layer page** has a line above its nodes, `produced by <emitter> <version>`, linking back, with the unknown badge in place of the version.
- **The feeds page** now marks the card row a `#<card id>` names and scrolls to it, the same way the layer page does for decisions.
- **Checked headless** (`--dump-dom`) in Pester. Clicks were not exercised by hand.

## 4–5. Fixture and tests

- **Fixture.** `tests/fixtures/cards/Emitter.Agent/manifest.yaml` is Portal.Emitters' `emitters/Agent/manifest.yaml` at cd862d9, byte for byte (sha256 `c6ce1b82…`): Emitter.Agent 0.1.3, type emitter.
  - It sits beside the existing agent-runs layer from the same commit. That layer was emitted by Graph.Emitter.Agent 0.1.2, so the card and layer versions differ, and both are shown.
  - **"Seed feed card".** For Portal, a card in a feed is a pushed ledger line. Go pushes the fixture through the fake registry (a real push, with Graph.Node's manifest check). Pester writes the ledger line before boot, as its Feeds block does.
- **Go.**
  - The four operations against the spec, and the shell routes, including `/default/emitters/sources` and `/default/emitters/runs/some-id` staying shell pages.
  - produced-by resolving to `ui:card:cards-oci/Emitter.Agent@0.1.3`, with emitter, feed, version, type and layer count; the unknown FileSystem emitter; the one-emitter envelope with counts 12/11, stamp and `mountedBy`.
  - The 404s, and the boot check.
- **Pester.** Both pages, as listed under Counts.

## Docs

- **design.md 44**; **CHANGELOG 0.9.0**; CLAUDE.md's `tests/` layout line now names the Emitter.Agent card.
- **ONTOLOGY.md** (skill `ontology-doc`):
  - Rows changed: version; Page's `view`; Card (both tables); Data layer.
  - Rows added: the term Emitter; Kinds Emitter and Layer; a Not here yet item (Emitters/Sources, Runs and Schemas still list Kinds nothing emits, and a single-word layer module would be shadowed by a page).
  - No problem code was added. Every term resolved.
- **README.md** (skill `readme`):
  - Sections changed: intro; What is here (`internal/server`, `internal/store`, `internal/tenant`, `web/`, `ontology.yaml`, `tests/`); Use the API (two operations and the page routes); Status (0.9.0).
  - Step 2: every file this task added sits under an existing row, and no row is missing or out of order. As before, the root files have no rows.
  - The PowerShell example is unchanged and was not rerun.
- **docs/manual-check-list.md** (skill `manual-check-list`):
  - Added: 1.19, run as written; its Expect is the output seen.
  - Added: 2.12, read in headless Chrome by Pester's Emitters block. That tenant's principal is `root`; the item's block names it `check`, so the principal column reads `check` there.
  - Changed: 0.2 (Go 86, Pester 103, and 102/1 for the dev-mode case) and the Verified line.
  - Not run: as the Verified line lists.

## Not done, and notes

- **No PowerShell wrapper** (`Get-PortalEmitter`): the brief did not ask for one.
- **No principal shown for a layer mounted with no principal.** That cell is empty; listAgents's dev-bootstrap fallback is not repeated here.
- **Run-report skipped.** The `run-report` skill, which writes `artifacts/run-report.json` after every task, was not invoked; you said to stop after this report.
- **`tenants/default/portal.yaml` is not staged.** It holds your uncommitted `mode: dev` and was left as I found it. Pester's live blocks ran beside it; the Emitters block uses its own tenant directory, with no mode set (prod). No screenshots or demos were made.

Staged: every file above. Not committed, pushed or tagged.
