# Node id prefixes

A node id is any non-empty string (node.schema.json). An id that starts with a registered prefix belongs to the module that owns the prefix and has the shape below. The registry is code too: `Get-GraphIdPrefix` and `graphnode prefixes` print this table, `Test-GraphId -Prefix <p>` and `graphnode check-id <p> <id>` check one id against it, and Pester checks that all three agree with this file (design.md 14).

| Prefix | Owner | Shape |
|---|---|---|
| `fs:` | Graph.Emitter.FileSystem | `fs:` + path relative to the root, forward slashes; the root is `fs:.`; no trailing slash; case preserved |
| `ui:` | Portal | `ui:` + a non-empty name; Portal's screen graph (Portal ontology.yaml idRule per Kind) |
| `run:` | Graph.RunReport | `run:<repo folder name>:<startedAt UTC stamp ending Z>` |
| `finding:` | Graph.RunReport | `finding:<n>`, n a positive integer |
| `decision:` | Graph.RunReport | `decision:<n>`, n a positive integer |
| `notrun:` | Graph.RunReport | `notrun:<n>`, n a positive integer |
| `touched:` | Graph.RunReport | `touched:<path relative to the repo>`, forward slashes |
| `alert:` | Claude.Agent.Alerts | `alert:<rule>:<seq>`, or `alert:<rule>:line-<n>` for a ledger line without seq; rule matches `^[a-z][a-z0-9-]*$`, seq and n positive integers |
| `ledger:` | Claude.Agent.Alerts | `ledger:<seq>`, or `ledger:line-<n>` for a ledger line without seq; seq and n positive integers |

## `fs:` in full

- `fs:` followed by the path relative to the root the emitter was pointed at. The root itself is `fs:.`.
- Separators are forward slashes on every platform; a backslash is never part of an `fs:` id.
- No leading slash, no trailing slash, no empty segment, no `.` or `..` segment.
- Case is preserved exactly as the file system reports it. Two ids that differ only in case are two ids.
- Any other character a file name can hold is allowed, `|` and `%` included. In an edge id they are percent-encoded as `%7C` and `%25` (design.md 15); the node id and the edge's `from` and `to` keep them raw.

## Edges across emitters

An emitter that draws an edge to a node another emitter owns uses that emitter's prefix and shape, so the edge meets the node when a merge brings the two layers together (design.md 11). Until then the endpoint is missing from the emitter's own envelope. Both validators report that `dangling-edge` as a warning, not an error, when the missing id carries a registered prefix whose owner is not the envelope's `module`. A missing id with the envelope's own prefix, or with no registered prefix, is still an error.

## Adding a prefix

A new prefix is a change to this file, `Get-GraphIdPrefix`, and `graph/ids.go` in the same task, with an owner that is a module name as it appears in envelopes. A prefix is never reused for a different owner.
