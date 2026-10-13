# Portal: README screenshots (2026-10-12)

Task: replace README's one screenshot (`docs/screenshots/summary.png`, Overview → Summary) with `nodes.png`, `ledger.png` and `alerts.png`, each captioned from what it shows.

## Dev-mode stop and override

- `tenants/default/portal.yaml` is `mode: dev` in the working copy (committed as `demo`). CLAUDE.md says to stop before preparing anything for showing to others while a tenant is in dev, so I stopped and reported it first.
- All three new captures show the DEV badge and the amber DEV MODE bar, with the user as `anonymous`.
- The user explicitly overrode the stop: they had seen the bar, chose to publish the captures as they are because the README describes a work in progress, and said to leave `portal.yaml` alone. `portal.yaml` is untouched and unstaged.

## Changes

- `README.md:18`: the Summary image is replaced by three images, each with a one-line italic caption:
  - `nodes.png`: Graph → Nodes, 1615 nodes and 2813 edges. Agent run layers are on the left and the Portal repos on the right.
  - `ledger.png`: Ledger → Entries. It shows two valid chains: the agent's 344 entries from the agent-ledger layer and Portal's 33 entries from `ledger.jsonl`.
  - `alerts.png`: Compliance → Alerts. It shows two info alerts from agent run 339..344: a refused PORTAL_TOKEN that was retried, and a blocked direct egress attempt.
- `README.md` `docs/` row (was line 43): `screenshots/` now lists `summary.png` and the three new files. The four `Invoke-Build Screenshot` captures are named as its output; the three new ones are marked as hand captures shown above.
- `README.md` `Invoke-Build Screenshot` line (was line 73): it now names the three new files as hand captures that the task does not write. Listing them as the task's output would be false: `.build.ps1` captures only summary, service-connections, explore and identity-users.

## Kept

- `summary.png` is kept as the user asked. It is still referenced in the README, and `Invoke-Build Screenshot` writes it.

## Checks

- Every `docs/screenshots/*.png` path in README.md resolves on disk: alerts, ledger, nodes and summary.
- No test or skill references README screenshots (checked with `git grep` over `tests/` and `.claude/skills/readme`).
- Pester was not run, because the change is docs-only.

## Staged

`README.md`, `docs/screenshots/nodes.png`, `docs/screenshots/ledger.png`, `docs/screenshots/alerts.png`, and this report. Not staged: `tenants/default/portal.yaml`. Nothing was removed. No version bump, no CHANGELOG.

## Choices made without asking

- Lines 43/73: I put the three new files beside the four existing ones and labelled them as hand captures, rather than presenting them as `Invoke-Build Screenshot` output, because the task does not write them. This is a docs wording choice, so it has no design.md entry.
