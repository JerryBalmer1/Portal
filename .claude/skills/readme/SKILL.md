---
name: readme
description: Keep README.md, the usage door to Portal, in step with the code. Use whenever a path the What-is-here table names is added, moved or removed, an exported function or parameter changes, a cmd/portal flag or API operation changes, an Invoke-Build task or a requirement changes, or the version changes, and before reporting any task that touched one.
---

# README

Adapted from Graph.Node's skill of the same name. `README.md` is the door for people who run or build on the portal: what is in the repo, what the build needs, how to start it, the API at a glance, and a pasteable PowerShell example. Terms live in `ONTOLOGY.md` (skill `ontology-doc`).

## When to update it

In the same task as any change to a path the What-is-here table names or a new top-level directory; the psd1 `FunctionsToExport` or a parameter; a `cmd/portal` flag; an operation in `api/openapi.yaml`; an Invoke-Build task; a requirement (`go.mod`'s go line, `#Requires`, Pester `-MinimumVersion`); the psd1 `ModuleVersion`.

## Rules

- The first prose line is one `>` line naming the terms and linking `ONTOLOGY.md`. A prose line is any line that is not blank, an HTML tag, an image or a heading; everything above it is presentation only (HTML `<p>`/`<img>` blocks, blank lines, the H1). Pester checks both.
- Sections in order: rows diagram, What is here, Requirements, Build, Use from PowerShell, Use the API, Status.
- The What-is-here table matches the tree; the `src/Portal/` row lists exactly `FunctionsToExport`, in order; the `cmd/portal/` row lists every flag `main.go` defines; the `cmd/graphnode/` row every subcommand.
- Build lists the Invoke-Build tasks exactly as `.build.ps1` defines them and says where binaries go (`build/win-x64/`).
- Use from PowerShell is an example you have run; output comments are what you saw. Use the API lists every operation in the spec except the page's.
- Status names the psd1 version and points at design.md entries by number. No release steps.

## Procedure

1. Read the psd1, `.build.ps1`, `go.mod`, `cmd/portal/main.go`, `api/openapi.yaml`.
2. Check the What-is-here table against `git ls-files`; report rows missing, extra or out of order.
3. Edit. Run every example you changed from a temp `.ps1` with `pwsh -NoProfile -File`; paste only output you saw.
4. Run `Invoke-Build`; it must end `Build succeeded`.
5. Stage README.md. In the report, list the sections changed and the result of step 2.
