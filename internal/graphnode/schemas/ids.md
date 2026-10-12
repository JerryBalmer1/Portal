# Node id prefixes

A node id is any non-empty string (node.schema.json). An id that starts with a registered prefix belongs to the module that owns the prefix. The registry is code too: `Get-GraphIdPrefix` and `graphnode prefixes` print this table, `Test-GraphId -Prefix <p>` and `graphnode check-id <p> <id>` check one id, and Pester checks that all three agree with this file (design.md 14).

GraphNode reserves the prefix and its owner. The id format belongs to the owner: the `idPattern` of the owner's ontology Kinds, when that `ontology.yaml` is reachable beside the repo (`<siblings>/<Owner>/ontology.yaml` or `<siblings>/<Owner>/src/<Owner>/ontology.yaml`, `module` equal to the owner, at least one `idPattern` starting with the prefix). The Shape column is the fallback, used otherwise (design.md 23). Shared with names modules that also write ids under the prefix, as edge endpoints or in their own layers, without owning it; the owner still decides the format (design.md 26). `Test-GraphId -SiblingRoot` and `graphnode check-id --siblings` choose the folder; `graphnode id-format` says which applied.

| Prefix | Owner | Shape (fallback) | Shared with |
|---|---|---|---|
| `fs:` | Graph.Emitter.FileSystem | `fs:` + path relative to the root, forward slashes; the root is `fs:.`; no trailing slash; case preserved | |
| `ui:` | Portal | `ui:` + a non-empty name; Portal's screen graph (Portal ontology.yaml idRule per Kind) | |
| `run:` | Graph.RunReport | `run:<repo folder name>:<startedAt UTC stamp ending Z>` | |
| `finding:` | Graph.RunReport | `finding:<n>`, n a positive integer | |
| `decision:` | Graph.RunReport | `decision:<n>`, n a positive integer | |
| `notrun:` | Graph.RunReport | `notrun:<n>`, n a positive integer | |
| `touched:` | Graph.RunReport | `touched:<path relative to the repo>`, forward slashes | |
| `alert:` | Claude.Agent.Alerts | `alert:<rule>:<seq>`, or `alert:<rule>:line-<n>` for a ledger line without seq; rule matches `^[a-z][a-z0-9-]*$`, seq and n positive integers; or `alert:<fingerprint>`, 64 lower-case hex, the SHA-256 of the rule and the subject id with a newline between (the alerts layer Claude.Agent.Alerts builds for Claude.Agent.Orchestrator) | Claude.Agent.Orchestrator |
| `ledger:` | Claude.Agent.Alerts | `ledger:<seq>`, or `ledger:line-<n>` for a ledger line without seq; seq and n positive integers; the bare `ledger:` is the chain root, Claude.Agent.Ledger's layer root, and the one registered prefix valid with nothing after the colon (design.md 28); `ledger:tenant/<tenant>/<seq>` is a tenant's own chain, `ledger:tenant/<tenant>` its root, tenant matching `^[a-z0-9][a-z0-9-]*$` (design.md 29, below) | Portal, Claude.Agent.Orchestrator, Claude.Agent.Ledger |
| `agent:` | Graph.Emitter.Agent | `agent:<path relative to the repo, forward slashes>` (AgentFile), then optionally `#<heading slug>` (Section) and `/rule-<n>` (Rule); slug matches `^[a-z0-9_-]+$`, n a positive integer. The run layer's `agent:run/<stamp>/step-<n>/...` is a path in this shape | |
| `runner:` | Claude.Agent.Runner | `runner:<plan>` (Plan), `runner:<plan>/<session id>` (Run), `runner:<plan>/<session id>/step-<n>` (Step); plan has no `.` or `/`, n a positive integer | |
| `git:` | Graph.Emitter.Git.Repository | `git:commit/<40 lower-case hex sha>`, `git:branch/<name>`, `git:tag/<name>`, `git:author/<email, lower-cased; the name when the email is empty>` | |
| `ref:` | Graph.Emitter.Reference | `ref:repo:<folder name>`, `ref:doc:<Repo>/<path relative to the root>`, `ref:section:<Repo>/design.md#<n>`, `ref:skill:<Repo>/<name>`, `ref:ext:<first 16 lower-case hex of the SHA-256 of host + path>` | |
| `env:` | Claude.Agent.Orchestrator | `env:compose/<project>` (the compose stack), `env:container/<project>/<service>`, `env:image/<image reference>` (`env:image/mitmproxy/mitmproxy:12.2.3`), `env:network/<name>` (the name compose gave it, project included); project, service and name have no `/`, backslash or space, the reference no backslash or space | |

## `ledger:tenant/` in full

`ledger:<seq>` is the agent's chain: one seq space, numbered by Claude.Agent.Ledger's writer. A tenant's action ledger (Portal's `tenants/<tenant>/ledger.jsonl`, appended by Portal and Portal.Agent's Runner) numbers its lines from 1 too. Mounted in one graph, line 23 of each would both be `ledger:23`: one node id for two lines, and an edge meant for one lands on the other. The namespaced form keeps them apart.

- `ledger:tenant/<tenant>/<seq>` is line seq of that tenant's chain; `ledger:tenant/<tenant>`, no seq, is that chain's root, accepted as the bare `ledger:` is.
- tenant matches `^[a-z0-9][a-z0-9-]*$`: lower-case letters, digits and hyphens, not starting with a hyphen. It is Portal's tenant name rule, so a tenant's name is its slug unchanged. seq is a positive integer with no leading zero.
- No other path form after `ledger:`. `ledger:default/23` (no `tenant/`), `ledger:tenant//23` (empty slug), `ledger:tenant/Default/23` (upper case), `ledger:tenant/default/0` (seq not positive), `ledger:tenant/default/line-3` and `ledger:tenant/default/23/4` all fail. The message names every accepted form: `must be ledger:<seq>, ledger:line-<n>, the bare ledger: (the chain root) or ledger:tenant/<tenant>/<seq> (ledger:tenant/<tenant> its root); seq and n positive integers, tenant matching ^[a-z0-9][a-z0-9-]*$`.
- Like the bare root, the tenant chain passes whichever format applies: an owner `idPattern` for `ledger:` describes the agent's chain and need not name it. Go `graph.LedgerTenantPrefix` is `ledger:tenant/`.
- `ledger:23` and `ledger:tenant/default/23` are different ids; one envelope may hold both.

## `fs:` in full

- `fs:` followed by the path relative to the root the emitter was pointed at. The root itself is `fs:.`.
- Separators are forward slashes on every platform; a backslash is never part of an `fs:` id.
- No leading slash, no trailing slash, no empty segment, no `.` or `..` segment.
- Case is preserved exactly as the file system reports it. Two ids that differ only in case are two ids.
- Any other character a file name can hold is allowed, `|` and `%` included. In an edge id they are percent-encoded as `%7C` and `%25` (design.md 15); the node id and the edge's `from` and `to` keep them raw.

## Edges across emitters

An emitter that draws an edge to a node another emitter owns uses that emitter's prefix and shape, so the edge meets the node when a merge brings the two layers together (design.md 11). Until then the endpoint is missing from the emitter's own envelope. Both validators report that `dangling-edge` as a warning, not an error, when the missing id carries a registered prefix whose owner is not the envelope's `module`. A missing id with the envelope's own module's prefix may be in another layer of that module: pure `Test-Graph` reports it as a warning, `Test-Graph -Strict`, `graph.Validate` and `graphnode validate` as an error (design.md 19). One exception, for alert layers, which point into every other layer: the `to` of an `about` edge whose `from` is an `alert:` id is a warning in every validator, strict included, when it carries any registered prefix, the envelope's own module's too (design.md 26). A missing id with no registered prefix is an error everywhere.

## Adding a prefix

A new prefix is a change to this file, `Get-GraphIdPrefix`, and `graph/ids.go` in the same task, with an owner that is a module name as it appears in envelopes. A prefix is never reused for a different owner. Changing a prefix's format is the owner's change, to its own `ontology.yaml` `idPattern`; the fallback shape here moves only when the owner asks.
