# Portal 0.5.1: dev mode — report

Prompt: `[WORKSPACE: Portal]` dev mode, auto-minted token, browser sign-in, visible warning. Run 2026-10-10 (local; 2026-10-11 UTC). Everything is staged; nothing is committed, pushed or tagged. Only Portal was edited. While this task ran, 0.5.0 was committed (`29ff1d6`), so the index now holds 0.5.1's 32 files only.

## Token flow, in five lines

1. `tenants/<t>/portal.yaml` says `mode: dev`, or `Invoke-Build Dev` stops at DevGate, before it builds or writes anything.
2. DevMint writes principal `dev-bootstrap` (admin) and one fresh token hash into the gitignored `principals.yaml` and `tokens.yaml`, dropping older dev-bootstrap tokens, and sets `PORTAL_TOKEN`.
3. Up starts `portal.exe`, which inherits `PORTAL_TOKEN` and keeps it only if it authenticates as dev-bootstrap while the tenant is in mode dev. The token is printed once, DEV ONLY.
4. In the page: avatar, then "Use dev token", then `GET /api/tenants/<t>/dev-token`. It returns 404 unless the mode is dev, the caller and Host are loopback, and the token is still live. The page keeps the token in sessionStorage.
5. Every call sends it as the bearer token. Outside mode dev, dev-bootstrap's tokens are 401, and the page drops a refused token and reads as anonymous.

## What landed

1. **Mode.**
   - `mode: dev | demo | prod` in `schema/portal.schema.json`, prod when absent.
   - `tenant.Mode()`. The merged config always carries the mode.
   - New `GET /api/tenants/{tenant}` returns `{name, mode}` (spec first, `internal/gen` regenerated).
   - The boot line names the mode, and the banner shows a mode badge.
2. **`Invoke-Build Dev`.** It is `task Dev DevGate, DevMint, Up, {...}`, so it reuses Up as is.
   - The refusal prints why and stops before Build.
   - The mint is a private `New-PortalDevToken`, which refuses outside dev itself. It writes only the two gitignored files and keeps every other token.
   - Up never mints anything; Pester checks Up's body for that.
3. **Sign-in.**
   - The avatar box (paste, Sign in, Sign out, sessionStorage, bearer header) already existed from 0.4.0.
   - New: the avatar reads `<name> · <role>`, and in mode dev a "Use dev token" button reads from `GET .../dev-token`.
   - That route answers only in mode dev, to a loopback caller with a loopback Host, while the binary holds a live dev-bootstrap token. Everything else is 404, with one message for every case.
4. **Bars.**
   - Mode dev: an amber bar under the banner with the exact text from the brief.
   - Mode demo: a thin blue `DEMO` bar, and no "Use dev token".
   - Mode prod: neither. No bar has a dismiss control.
5. **Agent guard.** The sentence is in CLAUDE.md verbatim. The graph-node skill was not changed: Portal's copy is read-only (Pester compares it with Graph's), and the source skill in Graph.Node has no Portal section.
6. **README.**
   - "Dev mode" and "Before you demo", as `###` under Use from PowerShell, because the readme skill fixes the section order.
   - The two routes added to the API list.
   - Build lists Dev; Status is 0.5.1.
   - Pester fails while `tenants/default/portal.yaml` is committed or staged with mode dev.
7. **Tests.**
   - Pester drives Chrome over DevTools. There is no Playwright in the repo; this is the harness 0.5.0 added. It covers: sign in through Use dev token, then `dev-bootstrap · admin` and an Add; a reader signed in through the box, with Add disabled and its click doing nothing; the DEMO bar with no shortcut; no bar in prod.
   - Go: the dev token is 404 in non-dev modes and for a non-loopback caller or Host.
8. **Docs.** design.md 39, CHANGELOG 0.5.1, ONTOLOGY.md (terms Tenant mode and dev-bootstrap; Bootstrap updated), manual items 1.15, 2.8 and 3.6 (new) and 1.14 (updated). Version 0.5.1 in the psd1, the ontology and the spec.

`tenants/default/portal.yaml` is `mode: dev`, staged.

## Test counts

- **Go:** 63 top-level tests pass (8 new), `go vet` is clean.
- **Smoke:** passed (`artifacts/smoke-report.json`).
- **Pester:** `Tests Passed: 86, Failed: 1, Skipped: 0`. The one failure is the commit guard, as asked:

  ```
  [-] tenants/default/portal.yaml is neither committed nor staged with mode dev (README, Before you demo)
      ... because tenants/default/portal.yaml as staged (the index) says mode: dev; switch it to demo or prod before committing
  ```

  So `Invoke-Build` ends `Pester failed: 1 test(s)`. Before staging, with the same code, it was `Tests Passed: 87, Failed: 0` and `Build succeeded. 6 tasks, 0 errors, 0 warnings`.
- **Manual items:** I ran 1.14, 1.15 and 3.6 as written. Their Expects are what I saw.
- **Not run:**
  - The interactive items 2.1 to 2.8.
  - `Invoke-Build Dev` against the real `tenants/default`: your portal from Up was running on 8200 (process 113760), and Dev's Up would have replaced it.
  - Screenshots: Screenshot now refuses in mode dev, and CLAUDE.md forbids them.

## Decisions without asking (design.md 39 has each)

- **The served token is the one the binary was started with.** `tokens.yaml` holds only hashes. No new variable is used: the binary reads `PORTAL_TOKEN`, and a non-dev token in it is never served.
- **The dev-token route also checks the Host header**, against DNS rebinding, and sends `Cache-Control: no-store`.
- **dev-bootstrap is refused outside mode dev** (401 naming the mode), and the boot warns while its tokens remain. So demo is safe even if the revoke step is forgotten.
- **dev-bootstrap gets its own file rule.** It may have `createdBy: null` anywhere in `principals.yaml`, and the once-only bootstrap rule counts only the other principals. Without this, a dev mint into `tenants/default`, which already has `jerry`, would stop the next boot. `-Bootstrap` appends beside dev-bootstrap, and the name is reserved.
- **"Before you demo" is reordered**: mint the real admin, revoke dev-bootstrap as that admin, switch to demo, then check. In your order, once the tenant is in demo, dev-bootstrap can no longer sign in to revoke itself. All five steps are kept.
- **The guard reads `git show HEAD:` and `git show :`**, so it fails only when dev is committed or staged, not for a local unstaged edit.
- **`Invoke-Build Screenshot` refuses while `tenants/default` is mode dev.** It would capture the amber bar into `docs/screenshots/`.
- **The page drops a refused token at boot** rather than stopping at the error card. Without that, a tab still holding a dev token after the switch to demo would not load.

## Worth checking

- **The default build is red until the mode changes.** That is the guard working, but every `Invoke-Build` now stops at Pester. Flip `tenants/default/portal.yaml` to demo or prod before you commit.
- **Your own `tenants/default` already has `jerry` as its first admin.** My earlier status check had no `principals.yaml`; it appeared at 02:22Z, outside this session. The first `Invoke-Build Dev` will append dev-bootstrap beside it. I did not touch either file.
- **Demo does not hide the Designer.** `PORTAL_MODE=production` is separate. Say if demo should imply it.
- **A mode written in flow style reads as prod in PowerShell.** `Get-PortalTenantMode` reads only a block line `mode: dev`, so `{ mode: dev }` reads as prod and Dev refuses; the binary reads it correctly.
- **Smoke and Pester now run in mode dev**, because they copy `tenants/default/portal.yaml`. Everything passes there. The dev tests set their own modes.
- **"Use dev token" is also offered to a signed-in non-dev principal**, so you can switch to the dev admin. Hide it there if you'd rather.
