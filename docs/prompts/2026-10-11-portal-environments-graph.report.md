# Portal environments graph report (2026-10-11)

Task: five items.

1. A page Environments → Graph at `/{t}/environments/graph`, above Instances: a force layout, drag to pin, double-click to release, every `environment.*` node and what is one edge away, edge kinds and node kinds drawn differently.
2. Grouping by the `env`, `group` and `tags` the nodes carry, in hulls and chips.
3. A right-click menu (Details, Show graph, Runs, Ledger) on the graph and on Instances rows, greyed where no edge leads, reachable by keyboard.
4. Every node and item a link, with the selected node at `#<id>`.
5. Go and Pester tests, a fixture from the real agent-environment layer, and the boot rule.

Decisions: docs/design.md 46. Everything is staged; nothing is committed. `tenants/default/portal.yaml` (your `mode: dev`) is left unstaged, as I found it.

## Counts

| Suite | Before | After | New |
|---|---|---|---|
| Go top-level tests | 91 pass, 0 fail | **96 pass**, 0 fail | `TestEnvironmentGraphIsEmptyWithoutAnEnvironmentLayer`, `TestEnvironmentGraphOfTheRealLayer`, `TestEnvironmentGraphOneHopNeighbours`, `TestEnvironmentGraphFirstLayerWins`, `TestEnvironmentGraphJoinsThePrincipalToTheRunsItHolds` |
| Pester | 105 pass | **109 pass**, 0 fail, 0 skipped | four in `Environments graph`, all in headless Chrome |
| Smoke | | passed (canvas fit 0.844) | |

- **Go before** was run on this tree before any edit (`go test ./... -count=1 -json`: 91 top-level passes). **Go after** is the same command on the finished code.
- **Pester before** was `Invoke-Build Pester` on the untouched tree: 105 passed.
- **Pester runs after the change.** The final run, `pwsh -NoProfile -File .\tests\Invoke-Tests.ps1`, passed 109 of 109. The runs before it:
  1. Two of my own tests failed. Under `--dump-dom` the animated layout had not settled when the DOM was dumped. The fix: the first layout now settles synchronously before the first paint, within a 150 ms budget.
  2. All four new tests passed. "Production mode" failed in its BeforeAll because port 18200 was still taken.
  3. This was the default `Invoke-Build`. Go and Smoke passed. Pester's "Boot order" failed on the same port race.
- **The port race is old and is not from this change.** It is the race 0.7.0's report named: "Boot order" and "Section States on an old snap" do not wait for 18200 to come free. It hit in two of four runs today, so it is worth the one-line fix in a task of its own. I did not make that fix here.

## Asked, and your answers

- **Details.** No quick-details drawer exists. **Your answer: link to the item page.** Details is the node's item page, `/{t}/environments/instances/<id>`. For a Run it is its row on the runs layer page; for a principal, its agent page.
- **"Boot refuses a page named like a kind."** No such rule existed. **Your answer: reserve `graph`.** An Environments page with the id `graph` and a view other than `environments-graph` stops the boot. Covered in `TestBadOverlayFailsBoot`.

## What was built

- **API: `GET /api/tenants/{t}/environments/graph` (`getEnvironmentGraph`)**, written in the spec first, then generated.
  - **Nodes.** Every `environment.*` node of the mounted layers, under its layer's id with its properties. The first id wins, as on `listNodes`.
  - **Edges.** Every layer edge with one of those nodes at an end, its kind kept.
  - **The node one hop away.**
    - A Run comes as `listAgents` has it, with `layer` and `ledger`.
    - A Principal comes as `listPrincipals` has it; dev-bootstrap is made up when the file lacks it.
    - Anything else comes as its layer emits it.
  - **`has-run`.** A served principal holding a served run is joined to it by `has-run`. I added this when I found that a principal's Details and Runs would otherwise link an agent page that is 404 for a principal holding no run.
  - **Dropped edges.** An edge to a node nothing serves is left out: the real layer's `hosts` edge, until its run is mounted.
- **The page** is `web/envgraph.js`, drawn in SVG so that every node is an `<a>`.
  - **Layout.** The force layout has repulsion (doubled across envs), a link distance by kind and a pull toward its env.
  - **Pins and selection.** A drag pins a node and a double-click releases it. The node after `#` is selected and pinned with its neighbours. That survives a reload, and Back changes the selection in place through a new `onHash` hook in the shell.
  - **Look.** Colour follows the instance's kind; shape follows the Kind. A neighbour is smaller and dimmer. `member-of` is solid, `runs-as` dashed, `hosts` dotted. A legend explains them.
  - **Grouping.** Hulls are drawn per env, with group hulls inside; `unset` is grey and dashed. The env chips (dev, qa, prod, unset and any other value) and the tag chips are Instances' chips.
- **The menu** is `web/nodemenu.js`. It opens on graph nodes and on the rows of any `environment.*` table page: Instances now, Images and Networks once they fill.
  - Every item is a link. One with no edge to follow has no href, has `aria-disabled`, and says why in its title.
  - The keys are arrows, Home, End and Escape. Shift+F10 opens the menu beside the focused node.

## Fixtures

- **`tests/fixtures/layers/agent-environment/`.** The real layer and its ontology, byte for byte.
  - **Where it came from.** It is `../Portal.Agent/agent/Orchestrator/artifacts/environment-layers/2026-10-11T10-08-56Z.environment.graph.json`. The brief said `ledger-layers/`, but that folder holds the ledger layer.
  - **Reading Portal.Agent.** CLAUDE.md does not list `../Portal.Agent` as readable. I read it because the brief asks for the real layer. I did not widen the rule.
- **`agent-environment-tagged/`.** A derived copy:
  - `env: dev` on three nodes;
  - groups `stack` and `egress`;
  - tags;
  - the ledger keeper left unset, to show the gap;
  - `hosts` pointing at the agent-runs fixture's run;
  - the three fields declared in its ontology as fixture only.

## Docs

- design.md 46.
- README: intro, the What-is-here rows, Use the API, the routes, Status 0.11.0 and two terms on line 1.
- ONTOLOGY.md: the terms "Environments graph" and "Node menu", and a Not here yet line.
- manual-check-list: new item 1.21, which I ran (Expect is the output I saw), and new item 2.14. Item 0.2's counts and the Verified line are updated.
- CHANGELOG 0.11.0.
- CLAUDE.md: the fixtures in the tests/ layout line.
- Version 0.11.0 in the psd1, `ontology.yaml`, `ontology.go` and the spec.

## Worth a second look

- **The real ledger layer is not the shape of entry 45's fixture.** Its module is Claude.Agent.Ledger and its root is `ledger.jsonl`, a `LedgerChain`, with 236 entries. Portal finds it by its `ledger:<seq>` entries, so it should mount, but I did not mount it. The fixture should be replaced by a copy.
- **No "ledger root" one edge away.** Neither real layer has an edge from an environment node to the ledger. The Ledger item therefore follows `hosts` to the run and links that run's entries. A ledger node would show in the graph only once some layer emits such an edge.
- **Proposal for you:** add `../Portal.Agent` (or its `artifacts/`) to CLAUDE.md's read-only siblings, since the last three briefs have needed it.
- **Not checked by hand.** Item 2.14 was verified by Pester's DevTools script and by a headless screenshot I took on a throwaway demo-mode tenant. That screenshot was for my own check and was not for showing. Nobody has clicked through it with a real mouse.

## Not done

- No PowerShell wrapper for the new route.
- No keyboard drag, and no way to release a pin by keyboard.
- No kubernetes or host instances in any fixture: their colours are checked only in the stylesheet.
