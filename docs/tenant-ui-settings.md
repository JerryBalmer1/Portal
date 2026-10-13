# Tenant UI settings

Back to [README](../README.md); design: [docs/design.md](design.md) entries 37 and 58.

A tenant's UI settings are the shell's own settings: how the page behaves, not how a node looks. They live in the config's `theme` block. How a node looks belongs to State and Theme nodes (docs/design.md 5), and the portal refuses display properties anywhere else.

The rule (docs/design.md 58): **graph interaction defaults live in tenant config; per-page overrides live in the browser, never the server.**

## Where they come from

1. `portal.base.yaml` holds the product defaults. It is embedded in the binary and never edited by a tenant.
2. `tenants/<tenant>/portal.yaml` is the tenant's overlay. It is deep-merged over the base, so it names only what it changes.
3. `schema/portal.schema.json` validates both at boot. A value out of range stops the binary, naming the file and the key, for example `/theme/graph/wheelZoomSpeed`.
4. `GET /api/tenants/<tenant>/config` serves the merged config to the page (schema `Config` in [api/openapi.yaml](../api/openapi.yaml)). The page reads it once, when it loads.

A change to a tenant's `portal.yaml` takes effect when the portal restarts (`Invoke-Build Up`, or `Dev` for a dev-mode tenant).

## Settings

| Key | Default | Range | What it does |
|---|---|---|---|
| `theme.graph.wheelZoomSpeed` | `1.0` | `0.1` to `5` | Scales the mouse-wheel zoom step of every graph page: Graph/Nodes, Graph/Explore's Canvases and Explore's query result. `1.0` is the product's step (Cytoscape's `wheelSensitivity` 0.2), `2` twice as fast, `0.5` half. It is the default of each graph page's Wheel control. |

Environments/Graph is drawn in SVG and has no wheel zoom, so the key does not reach it.

```yaml
# tenants/<tenant>/portal.yaml
theme:
  graph:
    # Mouse-wheel zoom speed of every graph page, the default of each page's Wheel control (0.1 to 5, 1.0 the product's step).
    wheelZoomSpeed: 1.5
```

## Per-page overrides in the browser

Every page that draws a Cytoscape graph has a **Wheel** control at the right end of its toolbar: a slider from 0.1 to 5, the current speed (`1.0×`), and a reset button (`↺`).

- Moving the slider changes the speed of every graph on that page at once. Layout, zoom and pan stay as they were.
- The value is kept only in this browser's `localStorage`, under `portal:<tenant>:<page>:wheelSpeed`, where `<page>` is the menu page's path: for example `portal:default:graph/explore:wheelSpeed`. It is never sent to the server, and no other viewer, device or browser sees it.
- Opening the page again reads the key. A key that is absent, not a number, or outside 0.1..5 is ignored, and the tenant's default applies.
- Reset removes the key, and the page goes back to the tenant's default. Reset is disabled while the page has no override. Its tooltip names the default.
- With storage blocked (a private window, blocked site data), a change still holds until you leave the page.
- The control is not a write, so it works for an anonymous viewer too.

To clear every override in a browser, remove the `portal:*:wheelSpeed` keys in DevTools (Application, Local Storage).

## Adding a setting

A new UI setting is a key under `theme` in `schema/portal.schema.json` (with `additionalProperties: false`, a range and a default), in `portal.base.yaml` with a one-line comment, and in the spec's `Config` schema. It needs a numbered entry in docs/design.md and a row in the table above. If viewers may change it per page, the override goes in the browser as above, never in an API.
