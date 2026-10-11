# Portal 0.4.1: read-only page for anonymous, wheel zoom speed

Brief: two fixes, stage only. 1) With an anonymous principal, every control that would write is disabled with the "sign in to write" tooltip; a 401 that still happens is a toast, never an error body in a panel; Pester: an anonymous collapse is 401 and the canvas still renders. 2) Theme key `graph.wheelZoomSpeed` (default 1.0, range 0.1 to 5) in `portal.base.yaml`, validated by `schema/portal.schema.json`, overridable by a tenant overlay, read by the Canvas to scale its wheel zoom step; documented in design.md and ONTOLOGY.md. Edited and staged only; nothing committed. Graph.Node and GraphRenderer were not touched.

## What landed

| Brief item | Where | State |
|---|---|---|
| 1. Read-only UI | `web/toast.js` (new), `web/screen.js`, `web/designer.js`, `web/portal.js`, `web/base.css`, `web/web.go` | While whoami's `writes` is false, these buttons are `disabled` with `title="sign in to write"`: a panel's collapse/expand, the Designer's section toggles, Approve, and the four Add buttons. A refused write is a toast (`code: message`, plus any problems), and the panel keeps drawing. |
| 1. Pester | `tests/Portal.Tests.ps1` | New: "an anonymous collapse is 401 and the canvas still draws the graph, its collapse disabled". The Designer DOM test now also checks that every Designer button is disabled with the tooltip. |
| 2. Zoom speed | `portal.base.yaml`, `schema/portal.schema.json`, `api/openapi.yaml` (`Config.theme`), `internal/gen` regenerated, `web/canvas.js` | `theme.graph.wheelZoomSpeed: 1.0`. The schema holds it to 0.1 to 5, with no other key allowed under `theme` or `graph`. An overlay overrides it, and a value out of range stops the binary. The Canvas uses `wheelSensitivity = 0.2 × speed` and records it as `data-wheel-sensitivity`. |
| 2. Docs | `docs/design.md` 37, `ONTOLOGY.md` (terms Shell theme, Read-only), `README.md`, `docs/manual-check-list.md` | Below. |
| Report, CHANGELOG | this file, `CHANGELOG.md` 0.4.1 | |

Results: `Invoke-Build` (Modules, Build, Test, Smoke, Pester) ended `Build succeeded. 6 tasks, 0 errors, 0 warnings`. Go has 47 tests (46 before), `ok` for `cmd/portal`, `internal/server`, `internal/store` and `internal/tenant`. Pester passed 69 of 69 with none skipped (68 before; Chrome was present, so the DOM tests ran). Smoke's canvas fit was 0.853.

Manual items 1.11 and 1.12 were run as written, and their Expects are the output seen. Item 2.6 was driven by script in Chrome against a live portal with an overlay of `wheelZoomSpeed: 3`:
- As anonymous, the Canvas's collapse and all 13 Designer buttons were disabled with the tooltip.
- A collapse forced past `disabled` toasted `unauthorized: PATCH writes, and a write needs a bearer token; without one the portal reads only`. The Canvas stayed drawn (52/52 coloured) with no error in the panel.
- `data-wheel-sensitivity` was 0.6.
- Signed in with the test token, all 13 buttons were live.

## What was guessed

Each guess is a bullet in docs/design.md 37:

1. **Read-only means `writes` is false**, not only `anonymous`. A failed whoami counts as read-only too. The toolbar badge follows the same test.
2. **Every refused write is a toast, not only a 401.** The Designer's status line used to print refusals (validation problems included). Now they are toasts, and the status line keeps only Approve's result. A panel's read errors (a layer that fails to load, no renderer) still draw in the panel.
3. **Form fields stay editable**; only the button that sends is disabled.
4. **Service connections' Test is left alone.** It is a GET, not a write, and it still shows `unauthorized` in its row.
5. **A refused collapse no longer redraws the page.** Before this change it printed the error and then redrew anyway.
6. **`theme` in config is shell settings, not node display**, so entry 5 (display only on State and Theme nodes) stands. The key is not a node and does not appear in snaps or diffs. The spec's `Config` schema now documents it.
7. **The product's wheel step is still 0.2** (Cytoscape `wheelSensitivity`). The page also clamps to 0.1 to 5 and falls back to 1.
8. **The unknown-key example in tests and in manual item 1.11 is now `colours: dark`**, because `theme` is a known key. `theme: dark` is now a type error.
9. **Version 0.4.1.** The brief named none. `ontology.yaml` was bumped, but no Kind changed.

## Not done, and open

- No automated test clicks a disabled button or sees the toast: `--dump-dom` cannot click. That path was driven by script in Chrome (manual item 2.6) and has no Pester test.
- A real mouse-wheel notch was not tried. Only the configured sensitivity was checked.
- Manual items 2.2 to 2.5 were not re-run for this change.
- Carried from 0.4.0 and still Jerry's to decide: the CLAUDE.md lift line still says v0.2.1, but the pin is v0.2.3. The CLAUDE.md layout block lacks `internal/tenant/`, `portal.base.yaml`, `schema/` and `tenants/`.
