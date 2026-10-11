# Portal 0.6.0: feeds

Brief: feeds and Postgres as stock services, push through the API, a ledger entry per push.
1. Compose: an OCI registry, a NuGet v3 feed and Postgres 16, with tenant-scoped volumes and no new host ports.
2. `feeds:` in `portal.base.yaml`, overridable per tenant.
3. Every feed is a Graph.Node node, and Artifacts → Feeds lists them from the graph.
4. `POST /api/tenants/<t>/feeds/<feed>/push`: a manifest card, validated with Graph.Node 0.3.0, contributor or admin only, pushed to the feed, one ledger entry, 409 on a duplicate.
5. A read-only Feeds page.
6. `Invoke-Build Feeds`, which refuses in mode prod.
7. Tests in Go and Pester.
8. Version 0.6.0, design.md, README "Publishing a card".

Everything is edited and staged; nothing is committed, pushed or tagged. Only Portal was edited. Graph.Node and Portal.Tools were read only: the lift came from `git show v0.3.0:<path>` and the sample card from `git show HEAD:tools/Git/<file>`. `tenants/default/portal.yaml` says `mode: dev` in the working tree and is **not staged**; the index still holds `HEAD`'s `mode: demo`. No screenshots were taken.

## Asked, and answered

| Clash | Jerry chose |
|---|---|
| design.md 2 and CLAUDE.md let the binary write only `-artifacts` and the two identity files. A push sends a card to a feed, and the ledger needs a home. | `tenants/<t>/ledger.jsonl`, append-only and gitignored, beside the identity files. Entry 2 and CLAUDE.md line 9 now name the ledger and the outbound push. |
| "No host ports except the Portal API's", but the binary runs on the host and must reach the feeds. | Publish the registry and the NuGet feed on **127.0.0.1 only** (8201, 8202). Postgres stays unpublished. |
| Docker Desktop was not running. | Jerry starts it; Feeds runs end to end at the end. The first follow-up still found no Docker on this machine (5 minutes of polling). The second found engine 29.5.3, and Feeds ran end to end (see "End to end"). |

Premises that did not hold, found and worked around (design.md 40):
- Portal had no compose file, so there were no "existing" services. `compose.yaml` is new.
- The rail had no Artifacts → Feeds item; Artifacts had Exports and Bundles only. Feeds is added as its first page.
- `docs/examples` does not exist in Portal. The sample card is Portal.Tools' `tools/Git` (`c330712`), copied to `tests/fixtures/cards/Tool.Git/`.

## Image choices

| Service | Image | Why |
|---|---|---|
| OCI registry | `registry:2.8.3` | The brief's `registry:2`, pinned to its last release (2025-02-14 rebuild, about 10 MB). Distribution's reference registry: one binary, stores OCI image manifests, and needs no database. |
| NuGet v3 feed | `bagetter/bagetter:1.6.5` | The smallest well-maintained v3 server. BaGetter is the maintained fork of BaGet, whose last release was 2021; 1.6.5 shipped 2025-12-07. One container, about 90 MB, SQLite and files under `/data`, API-key push, service index at `/v3/index.json`. Passed over: NuGet.Server (v2 only, .NET Framework, Windows only), Sleet (a static feed with no push endpoint), ProGet, Nexus and Forgejo (hundreds of MB, built for many formats). |
| Postgres | `postgres:16.15-alpine` | The newest 16.x (Alpine build 2026-09-21), Alpine for size. Database and user `portal`, a `pg_isready` health check, `PGDATA` in a subdirectory of the volume. Not published, and not used by the binary yet. |

Volumes are bind mounts under `<tenants>/<tenant>/data/{registry,nuget,postgres}` (gitignored), passed as `PORTAL_FEEDS_DATA`. The Compose project is `portal-<tenant>`. `PORTAL_NUGET_KEY` and `PORTAL_POSTGRES_PASSWORD` come from `.env`; the first `Invoke-Build Feeds` writes them there, random. Each tag was checked on Docker Hub's tag API before pinning.

## Feed envelope shape

`GET /api/tenants/default/feeds`: one envelope, checked at boot with Graph.Node and the portal ontology, like the menu.

```json
{
  "schema": "graph/1", "module": "Portal", "version": "0.6.0", "layer": "2026-10-11T04:19:26.603Z", "ontology": "ontology.yaml",
  "nodes": [
    { "id": "ui:feed:cards-oci", "kind": "Feed", "name": "Cards (OCI)",
      "properties": { "kind": "oci", "url": "http://127.0.0.1:8201/cards", "visibility": "private", "order": 0 } },
    { "id": "ui:feed:cards-nuget", "kind": "Feed", "name": "Cards (NuGet)",
      "properties": { "kind": "nuget", "url": "http://127.0.0.1:8202/v3/index.json", "visibility": "private", "order": 1, "tokenEnv": "PORTAL_NUGET_KEY" } },
    { "id": "ui:card:cards-oci/Tool.Git@0.1.0", "kind": "Card", "name": "Tool.Git",
      "properties": { "feed": "cards-oci", "version": "0.1.0", "type": "tool",
                      "hash": "sha256:afba6ccdf582911204a6e0d49941bd925edd6e28cbce34c7b494cb4e4bd23293",
                      "digest": "sha256:0deb0f06e0400642e9a736faafe6853bd57227d3e8486831e7c1d0f65cd96829", "pushed": "2026-10-11T04:43:23.731Z", "pushedBy": "dev-bootstrap" } }
  ],
  "edges": [ { "id": "ui:feed:cards-oci|ui:card:cards-oci/Tool.Git@0.1.0|has-card", "kind": "has-card",
               "from": "ui:feed:cards-oci", "to": "ui:card:cards-oci/Tool.Git@0.1.0" } ],
  "counts": { "nodes": 3, "edges": 1, "byKind": { "Card": 1, "Feed": 2 } }
}
```

- **The prefix is `ui:`.** Portal issues only `ui:` ids, and the id registry is in the read-only lift. Portal's own Kinds are "registered" in `ontology.yaml`, so `Feed`, `Card` and the edge `has-card` are declared there.
- **The edge is `has-card`, not `holds`.** An existing test uses `holds` as its example of an unknown edge Kind.
- **Card nodes come from the ledger's `pushed` lines.** The config gives the feeds; nothing else is stored.

## Ledger entry example

One line of `tenants/default/ledger.jsonl` per push that names a feed the tenant has. These are the two lines the end-to-end run wrote, one per feed. The content hash is the same in both; the digests are each feed's own:

```json
{"at":"2026-10-11T04:43:23.731Z","principal":"ui:principal:dev-bootstrap","name":"dev-bootstrap","role":"admin","feed":"cards-oci","kind":"oci","card":"Tool.Git","version":"0.1.0","type":"tool","hash":"sha256:afba6ccdf582911204a6e0d49941bd925edd6e28cbce34c7b494cb4e4bd23293","digest":"sha256:0deb0f06e0400642e9a736faafe6853bd57227d3e8486831e7c1d0f65cd96829","files":2,"result":"pushed","message":"Tool.Git 0.1.0 to cards-oci"}
{"at":"2026-10-11T04:43:24.491Z","principal":"ui:principal:dev-bootstrap","name":"dev-bootstrap","role":"admin","feed":"cards-nuget","kind":"nuget","card":"Tool.Git","version":"0.1.0","type":"tool","hash":"sha256:afba6ccdf582911204a6e0d49941bd925edd6e28cbce34c7b494cb4e4bd23293","digest":"sha256:8ac52784459123e6ce7f109a6e73e19e113f7a95ef0c38e243c3374261e9326d","files":2,"result":"pushed","message":"Tool.Git 0.1.0 to cards-nuget"}
```

- **Every key is on every line.** One that does not apply is `null`: `principal` for a legacy token; `hash`, `digest` and sometimes `card` and `version` for an invalid card.
- **`result`** is `pushed`, `duplicate`, `invalid` or `failed`.
- **Refusals before the feed is known write no line.** That covers 401, 403, an unknown feed (404) and a malformed body (400).
- **At boot the ledger is read and checked.** A bad line stops the binary, naming the line.

## Push packaging

- **OCI feed: an OCI artifact in the shape ORAS writes,** so `oras pull 127.0.0.1:8201/cards/tool.git:0.1.0` restores the folder and any OCI client can read it.
  - The image manifest has `artifactType: application/vnd.graphnode.card.v1` and the empty config (`application/vnd.oci.empty.v1+json`).
  - Each file is one layer (`application/vnd.graphnode.card.file.v1`), titled by its path; `manifest.yaml` comes first.
  - Annotations carry the name, version, description, type, content hash, author and created time.
  - The repository is the feed URL's path plus the lower-cased card name (`cards/tool.git`). The tag is the version, with `+` written `_`.
  - It is plain distribution-API calls on `net/http`, with no new dependency. The digest recorded is the manifest's.
- **NuGet feed: a `.nupkg`,** the only thing a v3 feed accepts.
  - `<name>.nuspec` holds id = card name, version, authors = the pusher, description, and tags `graphnode-card <type> <tags>`.
  - `[Content_Types].xml`, then every card file at its own path, `manifest.yaml` at the root, so an extracted package is the card folder.
  - There is no `lib/`, so nothing installs it as a library.
  - The zip is deterministic (fixed times). The push is `PUT` to the feed's `PackagePublish/2.0.0` with `X-NuGet-ApiKey`, and the digest is the `.nupkg`'s SHA-256.
- **Content hash.** It is separate from the digest: `sha256` over `path TAB sha256(file) LF` lines sorted by path. A card pushed to both feeds carries one hash, while the two digests differ.

## Tests

| Suite | Before (0.5.1, this tree) | After | New |
|---|---|---|---|
| Go top-level tests | 63 pass | **78 pass** | 15. `internal/feed` 7: the card checked with Graph.Node, paths, content hash, OCI ref and manifest, `.nupkg`, push to a fake registry and a fake NuGet feed with 409. `internal/tenant` 4: the feeds merge, bad feeds stop the boot, one ledger line per push with every key and a reload, a bad ledger stops the boot. `internal/server` 4: push auth (401, 403, contributor, admin, legacy), 409, the feeds envelope against Graph.Node and the portal ontology, the ledger file's shape. Both new operations are in the spec test, the multipart request validated by kin-openapi. |
| Pester | 87 pass | **98 pass** | 11, block `Feeds`: Up reading `PORTAL_NUGET_KEY` from `.env` (follow-up); the envelope through `graphnode validate`; `Publish-PortalCard` 401, 403, 404 and `-WhatIf` with no line; 502 with a `failed` line naming every file; 422 with Graph.Node problems; 409 from the ledger; the page in Chrome; compose pins, loopback ports and volumes; the gitignore; Feeds refusing in prod and writing nothing; `.env` secrets. |
| `go vet` | clean | clean | |
| `Invoke-Build Feeds` (live) | (did not exist) | `Build succeeded. 3 tasks`; rerun `Build succeeded` with two 409s | |
| Smoke | passed | passed (canvas fit 0.844) | |
| `Invoke-Build` (default) | `Build succeeded. 6 tasks` | `Build succeeded. 6 tasks, 0 errors, 0 warnings` | |
| Manual items | | 1.16 and 3.8 run as written; their Expects are what I saw | 1.16, 2.9, 3.7, 3.8 new |

- **Tests changed, with the reason.**
  - `TestMenuShape` expects Artifacts' pages `Feeds, Exports, Bundles`.
  - The Pester pins moved from 0.5.1 to 0.6.0, and the ontology check to 13 Kinds and 9 edges.
  - `$GraphNodeTag` is `v0.3.0`.
- **Port 18200 races, twice.** "a PORTAL_SNAP that names no snap stops the binary" once found 18200 still held by the previous block's portal. After the live Feeds run, "throws when the port is still held after the wait" once could not bind 18200, because its block's `AfterEach` stops a portal without waiting for the port. Neither test nor the code it covers changed in 0.6.0, and each passed on the next run. The final full `Invoke-Build`, after Feeds: Go 78 `ok`, Smoke passed, `Tests Passed: 98, Failed: 0`, `Build succeeded. 6 tasks, 0 errors, 0 warnings`. A `Wait-PortalPortFree` in that `AfterEach` would close the second race; not done here.

## End to end

Run on 2026-10-11 against `tenants/default` (mode dev), Docker engine 29.5.3, `PORTAL_TOKEN` unset (so Feeds minted a dev-bootstrap token, DEV ONLY):

- **`Invoke-Build Feeds`** pulled the three pinned images, wrote `PORTAL_NUGET_KEY` and `PORTAL_POSTGRES_PASSWORD` to `.env`, and brought up `portal-default`. `--wait` saw nuget, registry and postgres Healthy. It replaced the portal on 8200 and pushed `Tool.Git 0.1.0` to both feeds:

  ```
  pushed Tool.Git 0.1.0 to cards-oci as dev-bootstrap: hash sha256:afba6ccdf582911204a6e0d49941bd925edd6e28cbce34c7b494cb4e4bd23293, digest sha256:0deb0f06e0400642e9a736faafe6853bd57227d3e8486831e7c1d0f65cd96829
  pushed Tool.Git 0.1.0 to cards-nuget as dev-bootstrap: hash sha256:afba6ccdf582911204a6e0d49941bd925edd6e28cbce34c7b494cb4e4bd23293, digest sha256:8ac52784459123e6ce7f109a6e73e19e113f7a95ef0c38e243c3374261e9326d
  Feeds: Tool.Git 0.1.0 in cards-oci (registry tag) and cards-nuget (feed version), listed by GET .../feeds; http://127.0.0.1:8200/default/artifacts/feeds
  Build succeeded. 3 tasks, 0 errors, 0 warnings 00:00:37.7388556
  ```

  The content hash matches the one computed offline from the fixture's two files.
- **`docker compose -p portal-default ps`:** `nuget running 127.0.0.1:8202->8080/tcp`, `postgres running healthy 5432/tcp` (not published), `registry running healthy 127.0.0.1:8201->5000/tcp`.
- **Registry.** Tags list is `0.1.0`. `Docker-Content-Digest` is `sha256:0deb0f06e0400642e9a736faafe6853bd57227d3e8486831e7c1d0f65cd96829`, the ledger's digest. The stored manifest has `artifactType: application/vnd.graphnode.card.v1`, the empty config `sha256:44136fa3...` (size 2), and layers titled `manifest.yaml` (731 bytes) and `install.yaml` (269 bytes). It also has every card annotation. So Distribution 2.8.3 accepts the image-spec 1.1 artifact shape as written.
- **NuGet.** The flat container lists version `0.1.0` for `tool.git`.
- **Postgres.** It initialised on the Windows bind mount (`tenants/default/data/postgres`); no named-volume fallback was needed. `psql -U portal -d portal` returned `portal|PostgreSQL 16.15 on x86_64-pc-linux-musl ...`.
- **Rerun.** `Invoke-Build Feeds` a second time printed `cards-oci already holds it (409): ... duplicate: Tool.Git 0.1.0 is already in cards-oci: pushed 2026-10-11T04:43:23.731Z by dev-bootstrap, sha256:afba6ccd...`, the same for cards-nuget, then `Build succeeded` in about 5 s. The ledger then held 4 lines: 2 `pushed`, then 2 `duplicate`.
- **Not checked by hand:** `oras pull` (oras is not installed here) and the Feeds page in a browser (item 2.9, interactive; and no screenshots of a dev-mode tenant). Pester's Chrome test covers the page's markup.
- **One harness note.** My check script ended with `docker compose exec -T postgres psql ...`, which printed its answer and then did not return. The pwsh process holding it was stopped after its output was read. Feeds itself never calls `exec`.

The stock services are still running (project `portal-default`), and the portal on 8200 is the one Feeds started. `Invoke-Build Down` and `docker compose -p portal-default down` stop them; the data stays under `tenants/default/data/`.

## Decisions without asking (design.md 40 has each)

- The lift and `graph-node` move to Graph.Node `v0.3.0` (`fa8fbf3`). There is still no registered edit.
- The feed's name is `id`, so the `id`-keyed merge (entry 36) overrides it per tenant. `visibility` is recorded, not enforced.
- `view: feeds` is a page renderer like `users` and `tokens`, not a panel type.
- The duplicate check runs ledger first, then the feed (HEAD or the version list), then the feed's own 409. A feed holding a version the ledger lacks is also 409.
- Pushes run one at a time per tenant, each bounded at 2 minutes.
- New problem code `file` for paths a card cannot carry. Manifest problems keep Graph.Node's `schema` code, at paths `manifest.yaml:/...`.
- `Publish-PortalCard` is the ninth export; every task's work lives in the module (entry 24).
- Feeds' token is `PORTAL_TOKEN`. When that is unset in mode dev, Feeds mints a dev-bootstrap token as DevMint does; in demo the token must be set. Up still never mints.
- Feeds reruns safely: a 409 is reported, not failed.
- `.env` gets the two secrets once; it is the user's gitignored file.

## Worth checking

- **CLAUDE.md's lift tag** said `v0.2.1`; the follow-up fixed it to `v0.3.0`. Its line 39 still says Pester checks graph-node against Graph's committed skill, untrue since 0.3.6 (entry 34); not changed.
- **Up reads `PORTAL_NUGET_KEY`** from `.env` since the follow-up, through `Get-PortalEnvValue`, which Feeds' `Initialize-PortalFeedEnv` uses too. It only reads: Up still writes and mints nothing. Until Feeds has run once, `.env` holds no key, and a NuGet push from an Up portal is 422 `no_token`, as before.
- **The base config hardcodes the stock ports** (8201, 8202). Compose takes `PORTAL_REGISTRY_PORT` and `PORTAL_NUGET_PORT`, but moving them also needs a `feeds:` override, and Feeds' checks use the defaults.
- **The Ledger rail service's Entries page still lists Kind `ledger.entry`,** which nothing emits. The push ledger is not shown there.
- **The README's Terraform example under "Publishing a card" has not been run yet.** It needs the live feeds.

## Open questions for Portal.Agent (the main pusher)

1. **Principal type.** Portal.Agent should push as `type: agent`, but `POST .../principals` refuses agents ("minted via Marker, later"). Until then it needs a user principal with role contributor, made by an admin, and a token from `New-PortalToken -Name`. Is that acceptable, or should agent principals come first?
2. **Re-push semantics.** 409 is final: there is no overwrite and no unlist. Should Portal.Agent treat a 409 with an equal `hash` as success (idempotent) and a different `hash` as an error to surface? The 409 message carries the earlier line's hash; a structured field would be better.
3. **Which feed.** Should the agent push every card to both feeds, or choose by card `type` (skills to OCI, tools to NuGet)? Or should the config say which feed is the default per type?
4. **Pull side.** Portal.Agent will install from these feeds. Pull by OCI tag, or by the content hash (`dev.graphnode.card.hash` annotation)? A pull by hash needs a lookup Portal does not serve yet: `GET .../feeds` lists cards but not by hash.
5. **Ledger access.** The ledger has no API of its own; only its pushed lines show, as Card nodes. Does the agent need `GET .../ledger` (failures included), or the Ledger service's Entries page wired to it?
6. **Ledger ownership.** `ledger:` ids belong to Claude.Agent.Alerts. Should the push ledger move there (signed, verifiable), with Portal writing to it, or stay a Portal tenant file?
7. **Visibility.** What should `public` mean once enforced: anonymous pulls from the feed, the card listed to other tenants, or both?
8. **Batch pushes.** Portal.Agent pushes a whole repo's `index.yaml`. One call per card (today), or a batch endpoint that pushes an index atomically?
