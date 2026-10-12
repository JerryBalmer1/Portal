# Actions: the ledger contract

Contract version 0.16.1 (Portal 0.16.1). Back to [README](../../README.md); design: [docs/design.md](../design.md) entries 50, 51 and 55.

Portal asks; Portal.Agent's Runner executes; the tenant's ledger records both. A request is one `action.request` line of `tenants/<tenant>/ledger.jsonl`, written by Portal (`POST /api/tenants/<tenant>/actions`). The Runner's answer is one `action.result` line of the same file. Both are appended under the same file lock, with the same `seq` and `prev`: one ledger, one chain, one seq space. Portal reads results from that file only.

## The two lines

A request, as Portal writes it, members in this order:

```json
{"kind":"action.request","id":"ledger:tenant/default/23","action":"logs","subject":"env:container/claude-agent-dev/ledger","principal":"ui:principal:dev-bootstrap","requested":"2026-10-12T00:44:55.935Z","status":"requested","seq":23,"prev":"<64 lower-case hex>"}
```

- `action`: `logs`, `stop` or `restart`. `subject`: a container, `env:container/<project>/<service>`. `principal`: who asked, from the token, `ui:principal:<name>`.
- `requested`: UTC, milliseconds, ending `Z`. `status`: `requested`.

A result, as the Runner writes it, members in this order:

```json
{"kind":"action.result","id":"ledger:tenant/default/24","request":"ledger:tenant/default/23","status":"done","started":"2026-10-12T00:48:13.800Z","finished":"2026-10-12T00:48:13.923Z","exit":0,"output":"...","source":{"emitter":"Emitter.Ledger","version":"0.4.0","target":"default","stamp":"2026-10-12T00:48:13.923Z"},"seq":24,"prev":"<64 lower-case hex>"}
```

- `request`: the `id` of the request it answers. `status`: `done` or `failed`. `exit` and `output` are always present (`""` is an output).
- `source`: Graph.Node's envelope source block, one object or a list of one, every member set.

Both lines:
- `seq`: the line's number in the file, from 1.
- `prev`: the SHA-256 of the line before it, as written, without its LF; 64 zeros on line 1.
- A line holds these members and no others. Portal refuses a line with an unknown member at boot.

## ids

From 0.16.1 every action line's `id`, and a result's `request`, is the tenant chain's id. This is Graph.Node's `ledger:` prefix in its tenant form, valid from Graph.Node 0.4.2:

```
ledger:tenant/<tenant>/<seq>
```

- `<tenant>` is the tenant's name, the directory under `tenants/`. It matches `^[a-z0-9][a-z0-9-]*$` (Portal's tenant name rule, and Graph.Node's slug rule).
- `<seq>` is a positive integer with no leading zero: the line's own `seq` for `id`, and the request line's `seq` for `request`.
- The whole id matches `^ledger:tenant/[a-z0-9][a-z0-9-]*/[1-9][0-9]*$`. `graphnode check-id ledger <id>` (Graph.Node 0.4.2 or later) exits 0 on it.
- `ledger:tenant/<tenant>`, with no seq, is the root of that tenant's chain. No line carries it.
- Example: line 24 of tenant `default`, answering line 23, is `"id":"ledger:tenant/default/24","request":"ledger:tenant/default/23"`.

`prev` is not an id and does not change form: it stays the SHA-256 of the line before (64 lower-case hex). `kind` is not an id either.

Why: the agent's own chain (Emitter.Ledger's agent ledger layer) is `ledger:<seq>`. Until 0.16.0 action lines used the same form, so line 23 of the tenant ledger and entry 23 of the agent chain had the same id, and the two could not be mounted in one graph. The tenant form keeps them apart, and Portal mounts the tenant ledger as the layer `tenant-actions`.

### The migration (Portal, at boot)

The first boot of Portal 0.16.1 or later rewrites, in place and under the file lock, every action line whose `id` (or a result's `request`) is still `ledger:<seq>`:

- `id` becomes `ledger:tenant/<tenant>/<seq>`, keeping the number. The old id is kept on the line as `legacyId`, immediately after `id`.
- A result's `request` becomes `ledger:tenant/<tenant>/<seq>`, keeping the number.
- A rewritten line has new bytes, so the next line's `prev` is rewritten to its new hash, and so on to the end of the file. A `prev` that did not name the line before it is left as it was, so a chain that was broken stays broken at the same line.
- Every other member, and the order of members, stays as written.
- The file is first copied, byte for byte, to `ledger.jsonl.<UTC stamp, colons as hyphens>.bak` beside it; a copy is never overwritten. A file with no line in the old form is not touched.

`legacyId` appears only on migrated lines. Neither Portal nor the Runner writes it on a new line.

### What Portal.Agent's Runner must write

- An `action.result` line's `id` is `ledger:tenant/<tenant>/<seq>`, `<seq>` being the `seq` it writes on the same line.
- Its `request` is the request line's `id`, exactly as that line carries it: `ledger:tenant/<tenant>/<seq>`. For a request written before 0.16.1 and not yet migrated, it is `ledger:tenant/<tenant>/<the request's seq>`, never the legacy form.
- `<tenant>` is the tenant whose ledger it appends to: the `target` of its own `source` block.
- `prev` and `seq` are computed as before. Nothing else in the line changes.
- The agent's own chain, the agent ledger, keeps `ledger:<seq>`. Only lines of `tenants/<tenant>/ledger.jsonl` take the tenant form.

Until the Runner writes this form, Portal still reads a result whose `id` or `request` is `ledger:<seq>`. It serves the result under its position's id and joins it to the request in the tenant form, and the next boot migrates the line.

## Where it shows

- `GET /api/tenants/<tenant>/actions`, `.../actions/<seq>` and `.../ledger/portal`: every entry is `ledger:tenant/<tenant>/<seq>`; a migrated line has `legacyId`.
- `POST /api/tenants/<tenant>/actions` returns the request as written, `id` in the tenant form. Every other member is as before 0.16.1.
- The layer `tenant-actions` (`GET .../layers`): the lines as LedgerEntry nodes plus the principals. Edges are `prev` (a line to the one before it), `request` (a result to its request), `subject` (a request to its container) and `principal` (a request to its principal). Its source block names Emitter.Ledger at the agent ledger layer's version.
- `GET /api/tenants/<tenant>/query?from=ledger:tenant/<tenant>/<seq of a result>&direction=out&depth=5` reaches the request, its subject, its principal and the lines before it ([query.md](query.md)).
