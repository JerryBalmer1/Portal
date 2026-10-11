# Portal 0.5.0: identity

Brief: principals, roles, and tokens that belong to a principal.
1. A node Kind `Principal`, served under the tenant: `name`, `type` (user or agent), `role` (reader, contributor or admin), `created`, `createdBy`.
2. Tokens belong to a principal. `New-PortalToken -Name` issues one for an existing principal; `-Bootstrap` makes the first admin, once. Old tokens work as admin, with a warning at boot.
3. Roles are enforced (reader GET only, contributor no identity or settings, admin all), refusals are 403 toasts, and attenuation is enforced server-side.
4. Pages: Identity/Users (Add for admin), Identity/Agents (empty state), Settings/Tokens (Revoke for admin).
5. Service connections record `createdBy`.
6. Tests in Go and Pester.
7. Docs.

All of it is edited and staged; nothing is committed. Graph.Node and GraphRenderer were not touched.

## Asked, and answered

The brief clashed with design.md 2 and CLAUDE.md, under which the binary writes only under `-artifacts`. Add principal and Revoke are binary writes that must persist, or a revoked token comes back after a restart. Jerry chose the tenant's identity files: `tenants/<tenant>/principals.yaml` and `tokens.yaml` are now the only files the binary writes outside `-artifacts`. Each is written whole, atomically. Entry 2 and the CLAUDE.md line were amended to say so (docs/design.md 38).

## What landed

| Brief item | Where | State |
|---|---|---|
| 1. Principal | `ontology.yaml` (Kind `Principal`), `internal/tenant/identity.go`, `api/openapi.yaml` (`listPrincipals`, `postPrincipal`), `internal/server/tenant.go` | Done. `ui:principal:<name>`, served as an envelope of nodes. `type: agent` is in the schema, the file check and the ontology, and the API refuses to create it (422). |
| 2. Tokens and principals | `internal/tenant/identity.go`, `src/Portal/Public/New-PortalToken.ps1`, `src/Portal/Private/Read-PortalYamlList.ps1` (new), `cmd/portal/main.go` | Done. A token entry is `{principal, sha256, created}`. `-Name` throws when no principal has that name. `-Bootstrap` works once. A legacy `{name, sha256}` entry acts as admin, and stderr says `portal: warning: tokens.yaml entries without a principal act as admin until rotated: <names>`. |
| 3. Roles | `internal/server/tenant.go` (middleware and identity handlers), `internal/tenant/auth.go`, every write in the spec documents 403 | Done. Anonymous writes stay 401. A reader writing is 403. Identity writes are admin only. Attenuation is checked before admin, in the server and again at boot over the file. |
| 4. Rail | `portal.base.yaml` (views `users`, `agents`, `tokens`), `schema/portal.schema.json`, `web/principals.js` and `web/tokens.js` (new), `web/portal.js`, `web/toast.js` (`refusal`), `web/base.css` | Done. Users has an Add form (name and role; roles above yours are disabled). Agents shows "Agents are minted via Marker, later." Tokens lists principal, 12-digit hash prefix and created, with Revoke confirmed by a second click. Marker holders is unchanged. |
| 5. Connections | `internal/tenant/connections.go`, `web/connections.js`, `connections.example.yaml` | Done. Optional `created_by`, which must name a principal; served as `createdBy` (null when absent) and shown as a column. |
| 6. Tests | `internal/server/identity_test.go` (new), `spec_test.go`, `tenant_test.go` in both packages, `tests/Portal.Tests.ps1`, `.build.ps1` Smoke | Done. See Results. |
| 7. Docs | `docs/design.md` 38 (and 2 amended), `ONTOLOGY.md`, `README.md` ("First user"), `CHANGELOG.md` 0.5.0, `docs/manual-check-list.md`, `CLAUDE.md` line 9 | Done. |

Results:
- **Build.** `Invoke-Build` from cold (`build/` removed first) ended `Build succeeded. 6 tasks, 0 errors, 0 warnings`.
- **Go.** 55 tests (47 before), `ok` for `cmd/portal`, `internal/server`, `internal/store` and `internal/tenant`. New:
  - `TestRoleMatrix`: anonymous, reader, contributor and admin, each against read, write, identity and revoke.
  - `TestAttenuationRefusal` and `TestAttenuation`: a contributor asking for an admin is refused with a message naming attenuation; the file check refuses it too.
  - `TestBootstrapOnceOnly`: a second null `createdBy`, a root that is not first, or a root that is not an admin stops the load.
  - `TestLegacyTokenIsAdminUntilRotated`, `TestAddAndRevokePersist`, `TestTokensBelongToPrincipals`, `TestIdentityExamplesLoad`.
- **Pester.** 73 passed (69 before), 0 failed, 0 skipped. Chrome was present.
  - New: `New-PortalToken -Bootstrap` once, and `-Name` only for an existing principal, with a portal reading what the module wrote.
  - New: the boot warning; the anonymous Users, Agents and Tokens pages.
  - New: the Users page Add flow, driven in headless Chrome over the DevTools protocol. The admin fills the form; the row appears; `principals.yaml` records `createdBy: 'ui:principal:root'`.
  - New: a token minted for the added user goes live at the next revoke and is refused a write (403).
- **Smoke.** Bootstraps its admin. `checks.auth` is `{"anonymousWrite":401,"principal":"smoke","role":"admin"}`, and the canvas fit is 0.853.
- **Manual items.** 0.3, 1.1 to 1.14 and 3.1 to 3.5 were run as written, and their Expects are the output seen. `Invoke-Build Screenshot` was run: it added `identity-users.png` and refreshed the other three.
- **README.** The PowerShell example was rerun, and its output comments are what it printed.

## Docs, per the skills

- **ONTOLOGY.md.** Version 0.5.0. Terms added: Principal (now a Kind, also who a request is), Bootstrap, Role, Attenuation, Legacy token, Identity files. Terms changed: Token, Page (`view`), Service connection (`created_by`), Read-only. A Kinds row was added for `Principal` (**`type`**, **`role`**, **`created`**, `createdBy`; no edges). No problem codes changed. "Not here yet" gained agents, issuing tokens from the page, role changes and removals, and settings writes. `graphnode validate-ontology ontology.yaml` passed.
- **README.md.** Changed: line 1's terms, the intro, What is here (`internal/server/`, `internal/tenant/`, `cmd/portal/`, `tenants/default/`, `web/`, `ontology.yaml`, `docs/`), Build (Smoke, Screenshot), Use from PowerShell (`-Bootstrap`, `.role`, and a new "First user" subsection), Use the API (403, `whoami`, principals, tokens, `createdBy`), and Status. Checked against `git ls-files`: no row missing or extra once the new files are staged. "First user" is a `###` under Use from PowerShell, because the readme skill fixes the section order.
- **docs/manual-check-list.md.**
  - Added: 1.13 (roles, attenuation, revoke), 1.14 (legacy token) and 2.7 (Users Add, Agents, Tokens Revoke; interactive).
  - Rewritten: 3.5 (`-Bootstrap` once, unknown principal).
  - Changed: 0.2 (counts, `checks.auth`), 1.8 (`whoami`), 1.10 (`createdBy`), 2.1 (`identity-users.png`). Every item that mints a token now bootstraps first and removes `principals.yaml` too. 2.5 copies the connection example with `created_by` pointed at `check`.
  - Not run: 0.1, and 2.1 to 2.7, which are interactive.

## What was guessed

Each guess is a bullet in docs/design.md 38.

1. **Principal names** match `^[a-z0-9][a-z0-9._-]{0,63}$`, and the id is `ui:principal:<name>`. `New-PortalToken -Name` used to take spaces and capitals; it no longer does.
2. **`createdBy` is a property, not an edge.** The first admin's node leaves it out, because the portal refuses null properties; the file says `createdBy: null`.
3. **Attenuation is checked before the admin check**, so it can show through the API at all. `tenant.AddPrincipal` enforces both checks itself.
4. **"Settings" guards no route today**: nothing writes settings through the API. Identity means `POST /principals` and `DELETE /tokens/{id}`.
5. **Reads stay open to anonymous**, including the principal and token lists (names, roles, hash prefixes, dates; never a secret). The GitHub repos call still needs only a principal, so a reader may make it.
6. **A legacy token is admin but cannot add principals**: it has no id to record as `createdBy`, so it gets a 403 pointing at `-Bootstrap`. It may revoke tokens.
7. **A principal may hold several tokens.** `New-PortalToken` no longer refuses a second token for the same name; rotating means mint, then revoke. The API names a token by the first 12 hex digits of its hash.
8. **Add and Revoke re-read both identity files** before writing, so a token minted since boot goes live at the next identity write, not only at a restart.
9. **Both writers use one fixed block format**, so the module reads it with a small private parser and no YAML module.
10. **Users, Agents and Tokens get page views of their own** (`users`, `agents`, `tokens`) instead of table views of Kinds nothing emits. They are page renderers like `connections`, not panel types.
11. **Revoke confirms by a second click**, not a browser dialog. Read-only tooltips now name the role (`<name> is reader, which reads only`); anonymous keeps `sign in to write`.
12. **`created_by` in connections.yaml** is optional and snake case, like `token_env`. The committed examples now form a consistent set: an `example` admin, its token, and its connection.
13. **Pester drives Chrome over the DevTools protocol** on port 18203 for the Add flow.
14. **Version and dates.** Version 0.5.0. The CHANGELOG date is 2026-10-11, the brief's report date (UTC); the check-list's "Verified for" date is local, 2026-10-10.
15. **Screenshot.** I added `identity-users.png` to `Invoke-Build Screenshot` and reran it, which also refreshed `service-connections.png` (new "created by" column) and `explore.png`.

## Not done, and open

- **Port race on 18200.** `Stop-Portal` waits for the process to exit, but Windows can still list its listener for a moment, owned by a process that is gone. A test that starts a portal straight after then fails with `port 18200 is in use by unknown`. I saw it three times in eight Pester runs. Once was right after my new test, and once each in the existing "-NoReplace" and "Boot order" tests. My new stops now wait for the port (`Wait-PortalPortFree`, in Pester only). `Stop-Portal` itself is unchanged: fixing it there would also change `Invoke-PortalUp`'s timing and its held-port test, which I did not want to change without asking. Making `Stop-Portal` wait for the recorded port to free is the fix to decide on.
- **Revoke in the page is not automated.** Clicking Revoke, and the page dropping the viewer's own revoked token, are covered only by manual item 2.7, which was not run. The API path is tested in Go and Pester.
- **Live and minted tokens.** A token minted with `New-PortalToken` while the portal runs is refused until a restart or the next Add or Revoke. This is documented, but it may surprise people; a file watch would remove it.
- **Agents, issuing a token from the page, changing a principal's role or removing a principal, and settings writes** are listed in ONTOLOGY.md's "Not here yet".
- **Carried from 0.4.x, still Jerry's to decide.** The CLAUDE.md lift line says v0.2.1 while the pin is v0.2.3. The CLAUDE.md layout block lacks `internal/tenant/`, `portal.base.yaml`, `schema/` and `tenants/`. Only CLAUDE.md line 9 was edited in this task, as agreed.
