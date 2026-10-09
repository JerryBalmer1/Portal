---
name: manual-check-list
description: Maintain docs/manual-check-list.md, the paste-and-verify checklist for Portal's build, API, page and PowerShell wrappers. Use whenever an exported function or parameter, a cmd/portal flag, an API operation in api/openapi.yaml, a seed node, a base Category renderer, an Invoke-Build task, a fixture or the Pester count is added, changed or removed.
---

# Manual check list

Adapted from Graph.Node's skill of the same name. `docs/manual-check-list.md` is the human-run companion to Pester and the Go tests: every item is something Jerry copies, runs and eyeballs against an Expect line.

## When to update it

In the same task as any change to:
- an exported function or its parameters (`src/Portal/Public/<Verb-Noun>.ps1`)
- a `cmd/portal` flag, `PORTAL_MODE` or `PORTAL_SNAP`
- an operation in `api/openapi.yaml`, a problem code, or a status a route answers
- the dev seed (`internal/store/screen.go` `seed`) or a base Category renderer (`web/*.js`)
- an Invoke-Build task in `.build.ps1` or the task count; the Pester or go test counts item 0.2 quotes
- the fixtures under `tests/fixtures/`

If the task touches none of these, leave the file alone.

## File layout

```
# Portal manual check list

Verified for: <version> on <YYYY-MM-DD>, <OS>, PowerShell <x>, Go <x>, Pester <x>, InvokeBuild <x>: items <list>. <items not run and why>.

## 0. Toolchain
## 1. API
## 2. Page
## 3. PowerShell
```

Sections are fixed: 0 the build; 1 the API by curl-like calls; 2 the page in a browser; 3 the wrappers. One `###` item per behaviour, numbered `N.M`, never renumbered; a removed item keeps its number with "(removed in 0.x.0)".

## Item format

A fenced `powershell` block, then `Expect:` in one or two sentences naming what appears: status codes, problem codes, counts, a value, the Invoke-Build summary line. Every block runs from the repo root, starts with `Import-Module .\src\Portal\Portal.psd1 -Force` when it uses the module, starts its own portal on `-Port 18200` (the test port; 0 is refused since 0.3.0) under `$env:TEMP` and stops it in a `finally`, one portal at a time, and depends on no earlier item except 0.2 for the binaries. Never `pwsh -Command { }`.

## Procedure

1. Read docs/manual-check-list.md, the psd1 `FunctionsToExport`, `.build.ps1`, `api/openapi.yaml`.
2. Add or edit items. Run every block you add or edit from a temp `.ps1` with `pwsh -NoProfile -File` and confirm the output before writing the Expect. Fix the Expect, not the output.
3. Update the "Verified for" line; name every item not run and why.
4. Stage the file. In the report, list the items added, changed, removed, and not run.

## Do not

- Put checklist items anywhere else; link to them by number.
- Write an Expect you have not seen.
