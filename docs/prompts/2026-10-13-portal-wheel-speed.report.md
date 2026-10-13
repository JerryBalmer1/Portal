# Portal 0.16.4: the Wheel control on graph pages

Prompt: `[WORKSPACE: Portal] — graph pages: mouse-wheel zoom speed control, default from portal.yaml, adjustable per page`.

Result: done, with one deviation from the brief: the config key (below).
- `Invoke-Build` is green: Go ok (127 tests), Smoke passed (ratio 0.819; `wheel {"speed":1.0,"sensitivity":0.2,"pages":["graph/explore","graph/nodes"]}`), Pester 170 passed (166 + 4), `Build succeeded. 6 tasks, 0 errors, 0 warnings`. An earlier Pester run failed one test, the ontology version regex I had missed (`0\.16\.3`); it was fixed before this run.
- Manual check item 2.19 (new) was run headlessly: its non-interactive parts by `--dump-dom`, its interactive parts by a DevTools script on the same portal. Its Expect is that output.
- Every changed file is staged by name. `tenants/default/portal.yaml` carries the key and stays unstaged, as the brief says (it is also mode dev, modified before this task).
- Nothing was committed, pushed or tagged.

## The deviation: `theme.graph.wheelZoomSpeed`, not `ui.graph.wheelSpeed`

The brief asks to add `ui.graph.wheelSpeed` (default 1.0, range 0.1 to 5.0) to the config, its schema and the endpoint that serves UI settings. That setting already exists, since 0.4.1 (docs/design.md 37), as `theme.graph.wheelZoomSpeed`:
- default 1.0 in `portal.base.yaml`;
- `minimum` 0.1 and `maximum` 5 in `schema/portal.schema.json` and the spec's `Config`;
- overridable per tenant, and refused at boot when out of range;
- served by `GET /api/tenants/<tenant>/config`;
- already read by the Canvas.

Adding a second key for the same setting would make a tenant set it twice. Renaming it would break every overlay that sets it. I kept it, and recorded why in docs/design.md 58. If you do want the new name, it is a rename in the schema, base, spec, `wheel-speed.js`'s `configured`, and the tests: say so.

No endpoint or spec operation changed. `go generate ./api/...` was run for the version and the `Config` description.

## What changed (docs/design.md 58)

| Brief item | Files | Outcome |
|---|---|---|
| Config | `portal.base.yaml`, `schema/portal.schema.json`, `api/openapi.yaml`, `tenants/default/portal.yaml` (unstaged) | The existing key, see above. Descriptions now say every graph page. The default tenant names `wheelZoomSpeed: 1.0` with a one-line comment, and so does the base. |
| Every graph canvas reads it | `web/canvas.js`, `web/graph.js`, `web/query.js` | Graph/Nodes, Explore's Canvases and Explore's query result all take the speed from `wheel-speed.js`. The product step is still 0.2 × speed, so 1.0 is exactly today's behaviour. Environments/Graph is SVG with no wheel zoom, so it gets no control. |
| 3. One shared component | `web/wheel-speed.js` (new), `web/portal.js`, `web/base.css`, `web/web.go` | It holds a range slider labelled Wheel (0.1 to 5, step 0.1), the value (`1.0×`) and a `↺` reset, disabled with no override. It sits at the right end of the toolbar and changes every graph on the page in place through `cy.renderer().wheelSensitivity`, with no redraw. |
| 4. Per-page override | `web/wheel-speed.js` | The override is kept under `portal:<tenant>:<page>:wheelSpeed`, where `<page>` is the menu path (`graph/explore`). Every storage access is in try/catch. An absent or invalid value falls back to the config. Reset removes the key. There is no server call. |
| 5. Positions kept | `web/base.css` | `order: 1; margin-left: auto`. The stats, source chip and truncated chip stay left, and the query bar is in the content. The headless DOM shows the control as the toolbar's last child on every graph page. |
| 6. Tests | `internal/tenant/tenant_test.go`, `tests/Portal.Tests.ps1`, `.build.ps1` | Go: `5.0` accepted and `0.05` refused at `/theme/graph/wheelZoomSpeed`. Pester +4 (166 to 170), listed below. Smoke: `checks.wheelSpeed` loads every menu page that draws a Cytoscape graph and fails unless the control and the step match the config. |
| 7. Docs and version | `docs/design.md` 58, `docs/tenant-ui-settings.md` (new), `CHANGELOG.md`, `README.md`, `docs/manual-check-list.md`, `Portal.psd1`, `ontology.yaml`, `ontology.go`, `ONTOLOGY.md`, `api/openapi.yaml`, `internal/gen` | Version 0.16.4. The design entry's title is the brief's sentence. |

The four new Pester tests:
- the default tenant's key is in range, and `GET .../config` serves it;
- the "every page serving a graph canvas loads wheel-speed.js" check: each module that calls `cytoscape({` imports and uses it, and it is served;
- the control is present on Nodes, Explore and an Explore query;
- the override, its invalid-value fallback and Reset, driven in Chrome.

## Choices made without asking (each in docs/design.md 58)

1. The existing key was kept, not a new `ui.graph.wheelSpeed` (above).
2. "Graph canvas" means a Cytoscape graph. Environments/Graph (SVG, no wheel zoom) is left out.
3. `<page>` is the menu page's path. Explore with and without a query share one speed. Non-menu pages (a layer's, an agent's, an item's) get no control.
4. The 0.2 step and the 0.1..5 clamp remain JS constants. The step is what speed 1 means, and the clamp mirrors the schema. The speed itself comes only from config or the viewer.
5. Moving the slider back to the default keeps the key until Reset.
6. There is no demo tenant config and no tenant template in the repo. A fresh tenant merges over `portal.base.yaml`, which already carries the key, so nothing else was created or staged for it.
7. The Smoke check finds graph pages from the menu and the screen graph (view `graph`, or a screen node that is-a Canvas), rather than a fixed list.

## Docs

- README: the What-is-here rows `portal.base.yaml`, `web/` and `docs/`; Requirements (Chrome for Smoke's wheel check); the Build Smoke line; Status 0.16.4. The step-2 check on the table: the one new path, `web/wheel-speed.js`, is in the `web/` row; `docs/tenant-ui-settings.md` is in the `docs/` row. No rows are missing or extra.
- Manual check list: 2.19 is new; 2.6's Expect is updated (no longer the committed `{}`); 0.2's counts; the Verified line. Item 0.2 was run as the full `Invoke-Build`; item 2.19 was run as described above. The others were not run: 0.16.4 changes no other API, page or wrapper.
- ONTOLOGY.md: the version only, since no Kind or term changed.

## Not done

- Nothing was shown by hand in a browser. The control's look was checked from the DOM and the CSS, not from a screenshot. Any screenshot would need a demo-mode tenant: the default tenant is mode dev.
