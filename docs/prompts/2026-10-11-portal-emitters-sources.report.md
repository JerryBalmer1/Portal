# Portal 0.13.0: emitters match by source block, never by name; the Sources page from the card

**Task.** Five items:

1. Recopy the lift and `graph-node` from Graph.Node v0.4.0, so pushes accept 0.4.0 cards.
2. Remove the `Emitter.X` → `Graph.Emitter.X` mapping. A layer belongs to the card its source block names; one with no source block is `unstamped` under its module.
3. The Catalogue is a roll call with statuses `ok`, `version-mismatch`, `no-layers`, `no-card` and `unstamped`, with chips by status and facet.
4. One Sources page per emitter at `/{t}/emitters/{name}`, built from the card.
5. The nine cards as fixtures, Go and Pester tests for each status, a full Invoke-Build, version 0.13.0, the CHANGELOG and a design.md entry.

Decisions: docs/design.md 49, "Emitters match by source block, never by name". Everything is edited and staged only; nothing is committed, pushed or tagged. `tenants/default/portal.yaml` is left as found: modified, unstaged, `mode: dev`. No screenshots or demo work were done.

## Counts

| | Before (0.12.1) | After (0.13.0) |
|---|---|---|
| Go top-level tests | 106 | **107**, all pass |
| Pester | 116 | **118** pass, 0 fail, 0 skipped (Chrome ran) |
| Smoke | ratio 0.844 | ratio 0.844 |
| `Invoke-Build` (default) | | `Build succeeded. 6 tasks, 0 errors, 0 warnings` |
| Emitter cards in fixtures | 1 (Emitter.Agent 0.1.3, Graph.Node 0.3 shape) | **9** (Portal.Emitters v0.2.0, Graph.Node 0.4.0) |
| Of the nine cards, how many push | 0: every one is 422 (the 0.3.x schema has no `module`, `repo`, `url`, `emits`) | **9**, 201 each |
| Rows in the roll call over the test set (nine cards, six layers) | 12: 9 cards (2 with a layer), 3 unknown | **11**: 2 ok, 1 version-mismatch, 6 no-layers, 1 no-card, 1 unstamped |
| The same over the fixtures as they are on disk (none stamped) | 12: 9 cards (2 with a layer), 3 unknown | 13: 9 no-layers, 4 unstamped |

The "before" roll call is 0.12.1's rule applied to the same set. The nine ledger lines are seeded, since 0.12.1 could not push them. The cards then become `Graph.Emitter.<X>`, and each layer is named by its ontology's module:

- `Graph.Emitter.Agent` (agent-runs) and `Graph.Emitter.FileSystem` (filesystem) matched cards by the mangled name.
- `Claude.Agent.Alerts`, `Claude.Agent.Orchestrator` and `Portal.Agent` (ledger, ledger-old) were unknown.
- Emitter.Environment, Emitter.Ledger and Emitter.Git matched nothing.

The FileSystem match was by name only: the layer says nothing about which card produced it, and after this change it does not join the card.

## Status per fixture emitter

The test set used by Go (`emittersPortal`) and Pester (the Emitters block) is the nine cards plus six layers. Five of the layers are copies of real fixtures with a source block inserted; the sixth, filesystem, is mounted as it is.

| Name | Status | Card | Layers (source.version) | Why |
|---|---|---|---|---|
| Emitter.Agent | **ok** | 0.1.4 | agent-runs (0.1.4) | |
| Emitter.AzureDevOps | **no-layers** | 0.3.1 | | no AzureDevOps layer in fixtures |
| Emitter.Environment | **version-mismatch** | 0.3.0 | agent-environment (0.3.0), agent-alerts (0.4.0) | New-AgentAlertLayer stamps Claude.Agent.Alerts' 0.4.0. Portal.Emitters' report item 7 flags the same. |
| Emitter.FileSystem | **no-layers** | 0.2.0 | | the filesystem fixture has no source block, so it is never matched |
| Emitter.Git | **no-layers** | 0.1.1 | | |
| Emitter.Ledger | **ok** | 0.3.0 | ledger (0.3.0) | |
| Emitter.Network | **no-layers** | 0.4.0 | | |
| Emitter.Reference | **no-layers** | 0.1.1 | | |
| Emitter.Terraform | **no-layers** | 0.17.1 | | |
| Emitter.Unlisted | **no-card** | | ledger-old (1.0.0) | test-made: no real layer names an emitter without a card |
| Graph.Emitter.FileSystem | **unstamped** | | filesystem | `ui:module:Graph.Emitter.FileSystem` |

With the fixtures exactly as on disk, nothing is stamped. All nine cards read `no-layers`, and the unstamped rows are `Claude.Agent.Orchestrator` (environment and alerts), `Graph.Emitter.Agent`, `Graph.Emitter.FileSystem` and `Portal.Agent` (the ledger layer's envelope module).

## Not matching the brief, or worth knowing

1. **No layer on disk carries a source block yet.** Portal.Agent's stamping commit (ce2a76f) and Emitter.Agent 0.1.4 have not run since. Every runs, ledger, environment and alert layer under `../Portal.Agent/agent/*/artifacts` and `../Portal.Emitters/emitters/Agent/artifacts` has `source: null`. I could not run them (CLAUDE.md: never build or run the siblings). The fixtures stay the real layers, byte for byte. The tests (Go `stamped`, Pester `New-StampedLayer`) copy a fixture and insert only the source block, with the values each emitter's code writes. Once Portal.Agent's next Up runs, those layers can replace the copies.
2. **The ledger now stores the card's `module`, `repo`, `url` and `emits`.** The ledger held only name, version, type and hash, so there was nothing to build a page "from the card". Reading the card back from the registry would make a page depend on a feed being up. Lines from before 0.13.0 lack these fields, so a real tenant's existing emitter cards show no module or emits. Re-pushing the same version is a 409, but the 0.4.0 cards are mostly new versions.
3. **"Sources" is the emitter's own page.** `/{t}/emitters/{name}` now draws the source page; the rail keeps Emitters/Catalogue current there. The menu page Emitters/Sources (`kind: emitter.source`) is unchanged.
4. **New id shape `ui:module:<module>`** for unstamped rows, so a module named like a card can never be a duplicate node. Such a row has no page of its own, since the name finds the card first (noted in ONTOLOGY.md, Not here yet).
5. **"Every id a link"**: the emitter, card and layers link. `mountedBy` stays the principal's name, since there is no per-principal page outside Identity/Agents.
6. **A 0.3-shaped emitter card is now 422** (Graph.Node 0.4.0 requires `module`, `repo`, `emits`). The `emitter-name` warning never refuses a push.
7. **The graph-node skill copy still says `go get …@v0.3.3`** in one line. That is upstream's own text at v0.4.0, and the copy is read-only.
8. **I edited CLAUDE.md**: its two pin mentions now say v0.4.0, as its rule asks, and its fixtures line names the nine cards. Revert if you'd rather keep that file to yourself.
9. **Pester's Emitters block has the tag `Emitters`**, the only tagged block, so it can run alone.

## Graph.Node lift

`internal/graphnode/schemas`, `internal/graphnode/graph` (non-test files), `cmd/graphnode/main.go`, `.claude/skills/graph-node` and `tests/fixtures/invalid.graph.json` are `git show v0.4.0:<path>` (b0fd7a1), the Go imports rewritten. The diff from v0.3.3 is exactly upstream's: the manifest schema and type, `ManifestEmit`, the `emitter-name` warning and the CLI version. Pester's "Lifted Graph.Node" and graph-node checks pass at `v0.4.0`.

## Tests

- **Go:**
  - `TestEmitterCardsOfGraphNode040Push`: the nine cards push through the fake registry, each ledger line and Card carrying module, repo, url and emits; a 0.3-shaped emitter card gets 422.
  - `TestEmittersMatchBySourceBlock`: the eleven rows and statuses above; `layerVersions`; every produced-by edge; the unstamped FileSystem layer never joining its card; the source page envelope.
  - `TestEmitterNamesAreNeverMapped`: a no-layers card is 200; card modules and unknown names are 404; an unstamped module named like a card is its own row after the card; a replaced last layer is 404.
  - The boot check is kept.
- **Pester (Emitters):**
  - the roll call over the API;
  - the Catalogue (each status row, the mismatch versions, the facets, both chip groups, `?status=` and `?facet=` filters, the card link landing on its feeds row, Sources keeping its path);
  - the source pages (module, repo, Open in source, card, emits table, mismatched layer row) for the mismatch, no-layers, no-card and unstamped emitters;
  - the produced-by line for ok, mismatch and unstamped.

## Docs

- `api/openapi.yaml`:
  - `listEmitters`, `getEmitter` and `getEmitterPage` describe the new rule;
  - `pushCard` names Graph.Node 0.4.0;
  - `LedgerEntry` gains `module`, `repo`, `url` and `emits`.

  `internal/gen` is regenerated.
- `ontology.yaml`:
  - Card gains `module`, `repo`, `url`, `emits`, `status` and `layerVersions`;
  - Emitter has `status` in place of `unknown`, plus `layerVersions`;
  - Layer gains `source`;
  - produced-by and has-layer have new wording.
- **ONTOLOGY.md:** the Emitter, Card and Data layer terms; the Card, Emitter and Layer Kind rows; `emitter-name` added to Graph.Node's codes; two "Not here yet" lines (the shadowed unstamped page, and no real layer stamped yet).
- **README:** the lift version (rows diagram, `internal/graphnode/`, `.claude/skills/`), the `internal/server/` test list, the `tests/` cards fixture, the push note, both emitters API lines, the ontology version, and Status 0.13.0. The What-is-here table still matches the tree: no path was added or removed.
- **docs/manual-check-list.md:**
  - 1.19 is rewritten (API) and run as written;
  - 2.12 is rewritten (page) and seen by --dump-dom from its own setup, but its colours were not seen by eye;
  - the counts in 0.2 are updated (they had been 103/115);
  - the Verified line is updated.

  No other item was run.
- **CHANGELOG 0.13.0; design.md 49.** Version 0.13.0 in `Portal.psd1`, `ontology.yaml`, `ontology.go`, `api/openapi.yaml` and the Pester version checks.
