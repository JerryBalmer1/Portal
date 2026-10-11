---
name: graph-node
description: How to produce, declare, validate and read graph/1 envelopes with GraphNode, the base graph shape shared by every graph module (emitters such as Graph.Emitter.FileSystem, Graph.Emitter.Network, Graph.Emitter.Terraform, Graph.Emitter.AzureDevOps; consumers such as GraphRenderer). Use any time this repo writes or reads a graph.json envelope, an ontology.yaml, a manifest.yaml card or an index.yaml, builds nodes, edges or node ids, or reports GraphNode problem codes.
version: 0.3.3
---

# GraphNode for emitters and consumers

GraphNode owns the shape: one Node, one Edge, one Envelope (`schema: graph/1`), the schema your `ontology.yaml` must satisfy, the registry of node id prefixes, the `manifest.yaml` card every packaged thing carries and the `index.yaml` derived from the cards, and a validator in PowerShell and Go. Your repo produces or reads that shape; it never redefines it. If the shape does not fit what you need, propose a change to GraphNode; do not work around it locally.

`reference/` beside this file holds complete examples to open: `envelope.example.json` (a valid envelope), `ontology.example.yaml` (the ontology it names; in your repo the file is `ontology.yaml`, which is the name the example envelope uses) and `manifest.example.yaml` (a valid card; in your repo the file is `manifest.yaml`).

## When to use

Any time a repo produces or reads graph/1 envelopes: building nodes and edges from a source, choosing node ids, writing a layer, writing or changing `ontology.yaml`, validating output in tests or CI, loading layers to draw, diff or merge them, or writing the `manifest.yaml` card of a packaged emitter, skill, tool, rule or workflow and deriving its repo's `index.yaml`.

## Depend on GraphNode

PowerShell (7.4+). Until GraphNode is on a feed, import it from the sibling checkout (the module inside is still named GraphNode):

```powershell
Import-Module C:\__Code\Graph.Node\src\GraphNode\GraphNode.psd1
```

Once it is on a feed, declare it in your psd1 instead: `RequiredModules = @(@{ ModuleName = 'GraphNode'; ModuleVersion = '0.3.3' })`. Do not copy GraphNode's files into your module.

Go (1.24+). Import the package and pin it to a tag in `go.mod`, never `latest` or a branch:

```
go get github.com/JerryBalmer1/GraphNode/graph@v0.3.3
```

The package embeds the schemas, so `graph.Validate` needs no files on disk.

Cross-repo fixtures pin to a tag, never HEAD. Anything your repo copies from GraphNode or another sibling (this skill and its `reference/`, a schema, a fixture) is copied from a tag and checked against that tag: `git -C ..\Graph.Node archive v0.3.2 .claude/skills/graph-node` to copy, `git -C ..\Graph.Node show v0.3.2:<path>` in a test to compare. Never read a sibling at `HEAD`, a branch or its working tree in a test or fixture: those move under you, and your build then passes or fails on someone else's uncommitted work. Keep the tag in one place in the test, and when the sibling has no tag yet, skip the check with a reason naming the missing tag rather than falling back to `HEAD`.

## Produce an envelope

PowerShell:

```powershell
Import-Module C:\__Code\Graph.Node\src\GraphNode\GraphNode.psd1
$layer = Get-Date -AsUTC -Format 'yyyy-MM-ddTHH:mm:ssZ'
$vnet  = New-GraphNode -Id 'vnet:a' -Kind VNet -Name vnet-a -Properties @{ cidr = '10.0.0.0/16' } -Source @{ tool = 'az'; command = 'az network vnet show -n vnet-a' }
$snet  = New-GraphNode -Id 'snet:a' -Kind Subnet -Name snet-a -Properties @{ cidr = '10.0.1.0/24' }
$edge  = New-GraphEdge -From $vnet -To $snet -Kind contains
$env   = New-GraphEnvelope -Module Example -Version 0.1.0 -Layer $layer -Ontology 'ontology.yaml' -Nodes $vnet, $snet -Edges $edge
Export-Graph $env -Path ".\layers\$($layer -replace ':', '-')\graph.json"
```

- `New-GraphNode -Id -Kind` are required; `-Name`, `-Properties`, `-Source`, `-Findings`, `-Layer` (sets `stamp.layer`) are optional and omitted from the JSON when not given. `New-GraphNode`, `New-GraphEdge` and `New-GraphEnvelope` write any `[datetime]` or `[DateTimeOffset]` given as `-Layer` or as a top-level `-Properties` value as a UTC stamp `yyyy-MM-ddTHH:mm:ssZ` (whole seconds), so `-Layer (Get-Date)` is safe. Strings are written as given. A list value stays a list, whatever its length: `-Properties @{ names = @('a') }` writes `["a"]` and `@()` writes `[]`.
- `New-GraphEdge -From -To -Kind` takes node objects or id strings and sets the edge id for you, escaping as below.
- `New-GraphEnvelope -Module -Layer` are required; it sets `schema: graph/1` and fills `counts` (`nodes`, `edges`, `byKind`). `-Version`, `-Root`, `-Ontology`, `-Findings` (envelope-level findings) and `-Source` are optional. `-Source @{ emitter = 'Example'; version = '0.1.0'; target = 'C:/repo'; stamp = (Get-Date) }` writes the envelope's `source` block (below), the stamp as UTC ending `Z`.
- `Export-Graph` runs `Test-Graph` first and throws on any error (warnings do not block) unless `-Force`; it writes UTF-8 without BOM, LF line endings, and creates the directory.

Go: `env := graph.NewEnvelope(module, version, layer)`, append `graph.Node{ID: ..., Kind: ...}` and `graph.NewEdge(from, to, kind)`, then `env.Recount()` and `graph.Save(path, env)`. `NewEnvelope` keeps the lists non-nil so they serialise as `[]`.

Rules:
- Edge id is `from|to|kind`, with every `%` and `|` inside `from` and `to` percent-encoded as `%25` and `%7C`: `vnet:a|snet:a|contains`, and for a file named `a|b.txt`, `fs:a%7Cb.txt|fs:.|contains`. The edge's `from` and `to`, and the node ids, keep the raw characters. Ids without `%` or `|` are unchanged. Build edge ids with `New-GraphEdge` or `graph.NewEdge` / `graph.EdgeID`, never by hand; split one with `graph.ParseEdgeID`. A well-formed id naming the wrong endpoints is `edge-id`; one that does not split into three parts or holds any other `%` sequence is `edgeid`.
- Node id stability across layers is your job. The same real thing must get the same id in every layer, or a diff sees a delete and an add. Write the rule for each Kind as `idRule` in your `ontology.yaml` (`"vnet:<name>"`) and follow it.
- Kind names match `^[A-Za-z][A-Za-z0-9_.-]*$`. `New-GraphNode` and `New-GraphEdge` reject anything else.
- One envelope per layer: each run of your emitter writes one complete envelope of what it saw, never a patch. `layer` is required and must sort in time order as a string. Stamps are UTC ending `Z`: `layer`, `stamp.layer`, `stamp.firstSeen`, `stamp.lastSeen` and ActionRun `startedAt` and `finishedAt` must match `^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?Z$` (`2026-10-08T00:00:00Z`); a `+hh:mm` offset or no zone is a `stamp` problem. Keep one width across layers (whole seconds, as the builders write) so string order is time order. Put the same value in `stamp.layer` (`-Layer`) if you stamp nodes.
- Required: Node `id`, `kind`; Edge `id`, `kind`, `from`, `to`; Envelope `schema`, `module`, `layer`, `nodes`, `edges`. Everything else is optional and tolerated when absent, so leave out what you do not know rather than writing empty values.
- On a node or edge, `source` is a list of `{tool, command, path, file, line}`, all optional. On the envelope, `source` is one optional object `{emitter, version, target, stamp}`: what wrote the envelope, its version, what it read (a path, URL or name) and when (UTC ending `Z`). Leave it out, or give all four and nothing else; a bad stamp is `stamp`, anything else `schema`. Go: `env.Source = &graph.EnvelopeSource{...}`. `findings` is a list of `{kind, severity, message, target}`; `kind` and `message` are required by the Go types.

## Node id prefixes

Some id prefixes are registered to a module; GraphNode's `schemas/ids.md` is the table. `Get-GraphIdPrefix` (PowerShell), `graph.IDPrefixes()` (Go) and `graphnode prefixes` return it as `Prefix`, `Owner`, `Shape`, `SharedWith` (Go `SharedWith`, JSON `sharedWith`, absent when empty): the modules that also write ids under a prefix, as edge endpoints or in their own layers, without owning it. GraphNode reserves the prefix and its owner only; the id format belongs to the owner. `Shape` is the fallback format, used when the owner's `ontology.yaml` cannot be reached (below):

| Prefix | Owner | Shape |
|---|---|---|
| `fs:` | Graph.Emitter.FileSystem | `fs:` + path relative to the root, forward slashes; the root is `fs:.`; no trailing slash; case preserved |
| `ui:` | Portal | `ui:` + a non-empty name |
| `run:`, `finding:`, `decision:`, `notrun:`, `touched:` | Graph.RunReport | the run report's id rules |
| `alert:` | Claude.Agent.Alerts; shared with Claude.Agent.Orchestrator | `alert:<rule>:<seq>` or `alert:<rule>:line-<n>`, rule `^[a-z][a-z0-9-]*$`; or `alert:<fingerprint>`, 64 lower-case hex (SHA-256 of the rule and the subject id with a newline between) |
| `ledger:` | Claude.Agent.Alerts; shared with Portal, Claude.Agent.Orchestrator and Claude.Agent.Ledger | `ledger:<seq>` or `ledger:line-<n>` |
| `agent:` | Graph.Emitter.Agent | `agent:<path relative to the repo>`, then optionally `#<slug>` (`^[a-z0-9_-]+$`) and `/rule-<n>` |
| `runner:` | Claude.Agent.Runner | `runner:<plan>`, `runner:<plan>/<session id>`, `runner:<plan>/<session id>/step-<n>`; plan has no `.` or `/` |
| `git:` | Graph.Emitter.Git.Repository | `git:commit/<40 lower-case hex>`, `git:branch/<name>`, `git:tag/<name>`, `git:author/<email, lower-cased, or name>` |
| `ref:` | Graph.Emitter.Reference | `ref:repo:<folder>`, `ref:doc:<Repo>/<path>`, `ref:section:<Repo>/design.md#<n>`, `ref:skill:<Repo>/<name>`, `ref:ext:<16 lower-case hex>` |
| `env:` | Claude.Agent.Orchestrator | `env:compose/<project>`, `env:container/<project>/<service>`, `env:image/<image reference>`, `env:network/<name>`; project, service and name have no `/`, backslash or space, the reference no backslash or space |

- If your module owns a prefix, declare its format in your `ontology.yaml`: give each Kind whose ids carry the prefix an `idPattern`, a regular expression the whole id must match, starting with the prefix (`idPattern: "fs:([.]|[^/]+(/[^/]+)*)"`). Use the syntax Go and .NET share (RE2: no lookaround, no backreferences); the checker anchors it as `^(?:<pattern>)$`, so a leading `^` or trailing `$` is harmless. A pattern that does not compile is a `schema` problem in `validate-ontology`. Until you declare one, the registry's `Shape` is your format.
- Check an id: `Test-GraphId -Prefix fs -Id 'fs:src/main.go'` (one `[bool]` per id, pipeline input allowed; `-Verbose` says why one fails and where the format came from), `graphnode check-id [--siblings <dir>] fs <id>`, or Go `graph.ValidIDIn(siblings, "fs", id)`. Where the format comes from: the owner's ontology when it is reachable under the siblings folder, at `<siblings>/<Owner>/ontology.yaml` or `<siblings>/<Owner>/src/<Owner>/ontology.yaml`, its `module` equals the owner, and at least one Kind has an `idPattern` starting with the prefix; then the id must match one of those patterns. Otherwise the registry's shape. An owner `idPattern` that does not compile is an error (`Test-GraphId` throws, `check-id` exits 2), never a silent fallback; an unregistered prefix throws, or exits 1.
- `Test-GraphId -SiblingRoot` defaults to the folder holding the GraphNode checkout (`C:\__Code` when it is `C:\__Code\Graph.Node`), where its siblings live; `-SiblingRoot ''` uses the registry alone. Reading the ontology needs the `graphnode` binary; without it `Test-GraphId` uses the registry. The CLI and Go look nowhere unless given `--siblings` or a non-empty `siblings`; `graph.ValidID(prefix, id)` is the registry alone. In tests, pass a folder of fixtures (pinned, per "Depend on GraphNode"), not the live siblings.
- `graphnode id-format [--siblings <dir>] <prefix>` prints where the format comes from as JSON `{prefix, owner, source, ontology, patterns, shape}`, `source` `ontology` or `registry`. Go: `f, err := graph.ResolveIDFormat(siblings, prefix)` returns that `graph.IDFormat` (constants `graph.IDFormatOntology`, `graph.IDFormatRegistry`), `f.Check(id)` checks one id, `errors.Is(err, graph.ErrUnregisteredPrefix)` tells an unknown prefix from a broken ontology, and `graph.OwnerOntologyPaths(siblings, owner)` lists the two places looked.
- To edge to a node another module owns (a Terraform resource to the `.tf` file it came from), use that module's prefix and shape, `fs:modules/net/main.tf`, so the edge meets the node when a merge brings the layers together. In your own envelope that endpoint is missing: both validators report the `dangling-edge` as a warning, not an error, when the missing id carries a registered prefix whose owner is not your envelope's `module`. A missing id with your own module's prefix is a cross-layer edge within your module (from one of your layers into another): pure `Test-Graph` reports it as a warning, so `Export-Graph` writes the layer, but `Test-Graph -Strict`, `graph.Validate` and `graphnode validate` report it as an error, so strict CI fails on it. One exception: the `to` of an `about` edge whose `from` is an `alert:` id is a warning everywhere, `-Strict` and Go included, when it carries any registered prefix, your own module's too, because an alert layer points into every other layer (an alert written by Claude.Agent.Orchestrator about `env:container/<project>/<service>`). A missing id with no registered prefix is an error everywhere.
- To register a prefix, propose it to GraphNode; never issue ids with a prefix another module owns for your own nodes.

## Declare an ontology

Your repo ships one `ontology.yaml` that declares every Kind, edge Kind and finding Kind you emit. It is validated against GraphNode's `ontology.schema.json`; unknown keys fail.

```yaml
module: Example
version: 0.2.3
kinds:
  VNet:
    description: A virtual network.
    idRule: "vnet:<name>"
    idPattern: "vnet:[a-z0-9-]+"
    properties: [cidr]
    display: { color: "#4a90e2", shape: square, size: 3 }
  Subnet:
    idRule: "snet:<name>"
    properties:
      - { name: cidr, type: string, required: true }
    display: { color: "#2ec4b6", shape: hexagon, icon: network }
    actions:
      - name: attach
        description: Attach a VM to this subnet.
        inputs:
          - { name: vm, type: VM, required: true }
          - { name: primary, type: boolean }
        returns: { type: string, description: id of the new NIC }
        trigger: manual
        permissions: [network.write]
  VM:
    idRule: "vm:<name>"
    properties: [power]
    display:
      color: "#e07a9a"
      shape: circle
      when:
        - { properties: { power: stopped }, color: "#9e9e9e", shape: triangle }
edgeKinds:
  contains: { from: [VNet], to: [Subnet], display: { color: "#4a90e2", style: solid } }
findingKinds:
  unused-subnet: { severity: info }
```

- `kinds` is required. Each Kind may have `description`, `idRule` (how its id is built, in words), `idPattern` (the same as a regular expression, checked; see "Node id prefixes"), `properties` (names, or `{name, type, required, description}` with `type` one of string, integer, number, boolean, object, array), `display` and `actions`.
- Kind `display` is `{color, shape, icon, size, label, collapsed, when}`: `color` is `#rrggbb`; `shape` is `circle`, `square`, `diamond`, `hexagon`, `triangle` or `star`; `size` is an integer of 1 or more; `icon` is a name; `label` is a template where `{name}` and `{properties.x}` substitute. The 0.1.0 shape names `round-rectangle`, `rectangle`, `ellipse` and `octagon` are accepted with a `display` warning until 0.3.0; move to the six.
- `when` overrides the Kind's display by property value: a list of rules, each `{properties: {<name>: <value>, ...}, <display keys>}`, e.g. `{ properties: { verdict: deny }, color: "#cc0000" }`. A renderer uses the first rule whose every pair equals the node's property (string, number or boolean, exact) and lays its keys over the Kind's; no match leaves the Kind's display. A rule needs at least one pair and at least one display key, with the same rules as above (`display` error otherwise). A rule keyed on a property the Kind does not list in `properties` is a `display` warning; when the Kind lists none, it is not checked. Per-Kind display stays the default; do not split a Kind just to colour some of its nodes.
- Edge Kind `display` is `{color, style}`: `style` is `solid`, `dashed` or `dotted`. No `shape`, `icon` or `size` on edge Kinds.
- Display lives here, per Kind and edge Kind, and never on a node or edge: the node and edge schemas reject a `display` key (`schema`). A renderer resolves it by `node.kind` / `edge.kind` from the ontology, so changing a colour is an ontology change, not a re-emit.
- `actions` are `{name, description, inputs, returns, trigger, permissions}`. `inputs` is a list of `{name, type, required, description}`; `returns` is `{type, description}`. A `type` is a JSON type (string, integer, number, boolean, object, array) or a Kind name, meaning the value is that node's id. The 0.1.0 `inputs: { name: type }` map is accepted with an `action` warning until 0.3.0. `trigger` is `manual`, `on-change`, `schedule:<cron>` or `event:<name>`. Actions are declared only; running one is the ledger's job and produces an ActionRun node.
- `edgeKinds` give `from` and `to` as lists of Kind names, plus optional `description` and `display`.
- `findingKinds` give an optional `description` and `severity` (`info`, `warning`, `error`).
- If you emit a base Kind (ActionRun), declare it under `kinds` too so it has a display.
- `envelope.ontology` names the file (`"ontology": "ontology.yaml"`), as a path or URL. A relative path is relative to the directory of the envelope file, so `ontology.yaml` means the file beside `graph.json`, and `../ontology.yaml` one level up. Set it with `New-GraphEnvelope -Ontology`. `schema` stays `graph/1` regardless of your ontology's version.

## Validate

Every problem has a severity, `error` or `warning`. An envelope or ontology is valid when it has no errors; warnings are reported and never block.

PowerShell:
- `Test-Graph $env` or `Test-Graph -Path .\graph.json` runs the pure-PowerShell checks and returns `Valid`, `Problems` (each `Code`, `Path`, `Message`, `Severity`) and `Strict`. No binary needed.
- `Test-Graph ... -Strict` also runs the `graphnode` binary for full JSON Schema validation, including the base Kind schemas. It needs the binary built in the GraphNode checkout (`src\GraphNode\bin\win-x64\graphnode.exe` or `bin/linux-x64/graphnode`); if it is not built, `-Strict` throws `Test-Graph -Strict needs the graphnode binary: build the graphnode binary; see README` (GraphNode's README); on a platform with no binary (macOS today) it throws `no graphnode binary for this platform (win-x64, linux-x64)`. CI should run `-Strict` (or `graphnode validate`).
- `Test-Graph ... -Ontology .\ontology.yaml` also validates that ontology (display blocks, actions) and checks every ActionRun's `inputs` and `result` against the action its `action` names. Ontology problems have paths starting `ontology:`. It reads YAML through the binary, so it implies `-Strict` and needs the binary. Run it on request (in CI, say); it is not part of the default check.

Go and CI:
- `graph.Validate(env)` or `graph.ValidateBytes(data)` returns `[]graph.Problem` (`Code`, `Path`, `Message`, `Severity`), nil when there is nothing to report; `graph.HasErrors(ps)` is the validity test. `graph.ValidateOntology(doc)` checks a decoded ontology. `graph.ValidateWithOntology(data, doc)` does both plus `graph.CheckActionRuns(env, doc)`.
- `graphnode validate <graph.json> [--ontology <ontology.yaml>]` and `graphnode validate-ontology <ontology.yaml>` print a JSON array of `{code, path, message, severity}` (`[]` when clean) and exit 0 when there is no error (warnings only, or nothing), 1 on any error, 2 on a usage or read error. `graphnode prefixes` prints the id prefix registry; `graphnode check-id [--siblings <dir>] <prefix> <id>` exits 1 and prints why when the id does not fit; `graphnode id-format [--siblings <dir>] <prefix>` says where the format comes from. `graphnode schema` lists the schema names; `graphnode schema <name>` prints one; `graphnode version` prints the version.

Problem codes (the same in both validators; a code one reports and the other does not is a GraphNode bug, report it; the codes for cards and indexes are under "Write a manifest card"):

| Code | Means |
|---|---|
| `schema` | A required field is missing or wrong (`schema` not `graph/1`, no `module`, `layer`, `nodes` or `edges`, a node or edge without its required fields, a node or edge Kind name that does not match the pattern, a `display` key on a node or edge, an envelope `source` block that is not exactly `{emitter, version, target, stamp}` with non-empty strings), or, with `-Strict` / Go, any other JSON Schema violation; Go checks the full base Kind schemas, so it can find more `schema` problems than pure `Test-Graph`. Also the code for a file that is not JSON, and, in an ontology, for a Kind `idPattern` that is empty or does not compile. |
| `duplicate-node` | Two nodes in the envelope have the same `id`. |
| `duplicate-edge` | Two edges in the envelope have the same `id`. |
| `dangling-edge` | An edge's `from` or `to` is not the id of a node in the same envelope. A warning when the missing id carries another module's registered prefix; with the envelope's own module's prefix, a warning in pure `Test-Graph` and an error with `-Strict` / Go, except the `to` of an `about` edge from an `alert:` id, a warning everywhere; otherwise an error. |
| `edge-id` | An edge's `id` is well formed but is not `from|to|kind` for its own `from`, `to` and `kind`. |
| `edgeid` | An edge's `id` is malformed: not three `|`-separated parts (an unescaped `|` in an id), or a `%` that does not start `%25` or `%7C`. |
| `counts` | `counts.nodes` or `counts.edges` disagrees with the lists. |
| `kind` | A base Kind node is missing what its schema requires (ActionRun `action`, `startedAt`; Category `label`). |
| `stamp` | `layer`, `source.stamp`, `stamp.layer`, `stamp.firstSeen`, `stamp.lastSeen`, or ActionRun `startedAt` or `finishedAt` is not a UTC ISO-8601 stamp ending `Z`. |
| `display` | In an ontology: a display block or `when` rule that breaks its rules (colour not `#rrggbb`, unknown shape or style, size below 1, an unknown key, a rule without `properties` or without a display key); a warning for a 0.1.0 shape name, and for a `when` rule keyed on a property the Kind does not declare. |
| `action` | In an ontology: an action declaration that breaks its rules, or a warning for a 0.1.0 `inputs` map. With `-Ontology` / `--ontology`: an ActionRun missing a required input, with an undeclared or wrongly typed input, or with a `result` of the wrong type; a warning for an ActionRun whose action the ontology does not declare. |

What the validators do not check yet: that a node's or edge's Kind is declared in your `ontology.yaml`, and that an edge's endpoints match its edge Kind's `from` / `to`. Those rules are yours to keep.

## Read an envelope

- `Import-Graph .\graph.json` returns the envelope with `nodes`, `edges` and `findings` always present (empty arrays when missing). Validate it with `Test-Graph` before trusting it.
- `Import-Graph` (PowerShell's JSON reader) turns ISO-8601 strings such as `layer`, `startedAt` and `finishedAt` into `[datetime]`. A valid `Z` stamp exports back unchanged and `Test-Graph` accepts it; an offset stamp (invalid anyway) would come back rewritten in the local time zone. Compare layers as the strings in the file.
- `Get-GraphSchema envelope` (or `node`, `edge`, `ontology`, `manifest`, `index`, `action-run`, `category`) returns a schema as a hashtable; `-Raw` returns the JSON text; `-List` lists all eight.
- Go: `graph.Load(path)` or `graph.Decode(r)` returns `*graph.Envelope`, filling empty lists, with `Source` (`*graph.EnvelopeSource`) nil when the envelope has no `source` block; `env.NodeIndex()` maps id to index. `graph.SchemaNames()` and `graph.SchemaBytes(name)` expose the embedded schemas. `graph.ParseEdgeID(id)` returns the decoded from, to and kind; `graph.PrefixOf(id)` returns the registered prefix an id carries.
- Tolerate every optional field being absent. Resolve display, edge rules and actions from the ontology the envelope names, not from the nodes; for display, take the Kind's block, then lay over it the first `when` rule whose `properties` all equal the node's. An edge whose far end has another module's prefix points into that module's layer.

## Base Kinds you may emit

| Kind | When | Properties | Edges |
|---|---|---|---|
| `ActionRun` | One execution of an action declared in an ontology, recorded by whatever ran it. Emit it only when your repo actually runs actions. | Required `action` (`<Kind>.<action name>`, e.g. `Subnet.attach`), `startedAt`; optional `finishedAt`, `status` (`running`, `succeeded`, `failed`, `cancelled`), `actor`, `inputs` (name to value, matching the action's declared `inputs`), `result` (matching its `returns.type`). `startedAt` and `finishedAt` are UTC stamps ending `Z`. | `performed-on` to each target node; `produced` to each node it created or changed. |
| `Category` | Never from an emitter. Only the taxonomy module writes it. | Required `label`; optional `description`. | Kinds join it by `is-a` edges. |

Any other Kind is yours and is declared in your `ontology.yaml`. Never add your own Kinds to GraphNode.

## Write a manifest card

Every packaged thing, an emitter, a skill, a tool, a rule or a workflow (one folder each in Portal.Emitters, Portal.Skills, Portal.Tools, Portal.Ontology and Portal.Workflows), carries a card, `manifest.yaml`, in its own folder. It says what the thing is, what it needs and what it can do, and is checked against GraphNode's `manifest.schema.json`. `reference/manifest.example.yaml` is a complete one:

```yaml
name: Emitter.Example
version: 0.1.0
type: emitter
description: Reads an example source and writes one graph/1 envelope per run.
tags: [example, network]
needs:
  - { name: PowerShell, version-range: ">=7.4", os: any }
  - { name: Graph.Node, version-range: ^0.3.1 }
  - { name: example-cli, version-range: ">=1.2 <2.0", os: windows }
verbs:
  - { name: discover, command: Invoke-ExampleDiscover, description: Walk the source and write one envelope to the output folder., effect: read }
  - { name: tag, command: Set-ExampleTag, description: Write a tag onto each source item., effect: write }
  - { name: Remove-ExampleCache, description: Delete the local cache., effect: destroy, os: [windows, linux] }
pins:
  Graph.Node: 0.3.1
```

A rule that builds on another and ships a file, and a workflow:

```yaml
name: Overlay.Example
version: 0.1.0
type: rule
description: Renames two base kinds to the words one organisation uses.
tags: [overlay]
extends: Base.Infrastructure
files: [vocabulary.yaml]
verbs:
  - { name: apply, description: Lay the renames over the base vocabulary., effect: read, targets: [Base.Infrastructure] }
```

```yaml
name: Workflow.Onboard
version: 0.1.0
type: workflow
description: Onboards one target by planning, running and reporting a read-only discovery.
files: [steps.yaml]
needs:
  - { name: Skill.Discover, version-range: ^0.1.0 }
verbs:
  - { name: plan, description: List what run would do; runs nothing., effect: read }
  - { name: run, command: Invoke-Onboard, description: Discover the target and validate every envelope., effect: read, os: [windows, linux] }
  - { name: report, description: Summarise a finished run without reaching the target again., effect: read }
```

- Required: `name`, `version`, `type`, `description`. Optional: `tags`, `needs`, `verbs`, `extends`, `files`, `pins`. No other key, at the top or inside a need or verb: an unknown key fails rather than being ignored.
- `name` is dotted: two or more segments, each a letter then letters, digits or hyphens (`Emitter.Terraform`, `Emitter.Git.Repository`). No spaces, no leading or trailing dot. It must be unique among the cards of the repo; the index fails on a repeat.
- `version` is Semantic Versioning 2.0.0, three numbers with optional `-pre` and `+build` (`0.17.1`, `1.0.0-rc.1`): no `v`, no leading zeros, never two parts (`0.3` is not a version, and YAML reads it as a number). Use the version the thing itself reports (a psd1 `ModuleVersion`, a skill's `version:`).
- `type` is `emitter`, `skill`, `tool`, `rule` or `workflow`. Anything else is `schema` at `/type` with the message `value must be one of 'emitter', 'skill', 'tool', 'rule', 'workflow'`. A workflow's verbs are the phases it runs (`plan`, `run`, `report`); its steps live in a file beside the card, listed in `files`, and are not part of the card. A base vocabulary, an overlay and a policy are all `rule`: an overlay or policy says what it builds on with `extends` and what its verbs act on with `targets`, and a tag (`base`, `overlay`, `policy`) names its layer. `description` is one non-empty line.
- `tags` are strings with no repeats.
- `needs` lists what it needs to run: another packaged thing by its dotted name, or a runtime or tool (`PowerShell`, `terraform`). `name` is required; `version-range` is a free string (`>=7.4`, `^0.3.0`, `>=1.2 <2.0`), absent meaning any; `os` is `windows`, `linux` or `any`, absent meaning any. Quote a range that starts with `>` or `<` in YAML. Build-only tools (a compiler, Docker) are not needs.
- `verbs` lists what it can do. `name` and `effect` are required; `description`, `command`, `os` and `targets` are optional.
  - `name` is the verb a caller asks for (`discover`, `plan`, `fetch`), or the command's own name when there is no separate verb.
  - `command` is the cmdlet or executable that runs it (`Invoke-FileSystemDiscover`, `git fetch`), a non-empty string; leave it out when `name` already is the command. Put the command here, never in `description`, which says what the verb does in words.
  - `effect` is the strongest thing the verb can do outside its own output: `read` (changes nothing), `write` (creates or changes something) or `destroy` (can delete). A command that only reads unless a switch deletes something is `destroy`. Writing the card's own output (the envelope, report or index it was asked to produce, to the folder it was given), local refs (`git fetch` updating remote-tracking refs), the plan file and ledger lines does not count as an effect. So `discover`, `plan`, `run`, `report` and `fetch` verbs declare `read` unless they change something outside their own output: the target, a remote, another repo's files, a cloud resource.
  - `os` lists where the verb runs, a non-empty list of `windows`, `linux` and `macos` with no repeats; absent means all. Note the difference from a need's `os`, which is one value and allows `any`.
  - `targets` lists the names of the cards the verb applies to (`[Base.Infrastructure]`), a non-empty list of dotted names with no repeats. Each is checked as a well-formed name only, not that such a card exists.
- `extends` is the name of the card this one builds on (an overlay's base, a policy's vocabulary), a dotted name. It is checked as a well-formed name only, not that it resolves; the base may live in another repo. It is not a need: `needs` says what must be present to run, `extends` what this card is a layer over. Give both when both hold.
- `files` lists the files the card ships, relative to the card's folder (`vocabulary.yaml`, `steps/onboard.yaml`): forward slashes, no repeats, never absolute, no empty, `.` or `..` segment, no backslash or colon. That the files exist is not checked.
- `pins` maps a dotted name to the exact version it was built against or copied from (`Graph.Node: 0.3.1`, the GraphNode whose `graph-node` skill you carry). An exact semver, never a range; ranges go in `needs`.

Check a card:
- PowerShell: `Test-GraphManifest -Path .\manifest.yaml` (or the folder holding it; Get-ChildItem output pipes in) returns `Valid`, `Problems` (each `Code`, `Path`, `Message`, `Severity`) and `Path`.
- Go: `m, problems, err := graph.LoadManifest(path)` returns a `*graph.Manifest` (`Name`, `Version`, `Type`, `Description`, `Tags`, `Needs []graph.ManifestNeed`, `Verbs []graph.ManifestVerb` with `Name`, `Description`, `Command`, `Effect`, `OS`, `Targets`, `Extends`, `Files`, `Pins`), nil when there is an error problem; `err` is for a file that cannot be read. `graph.ValidateManifest(doc)` checks a decoded card; `graph.DecodeYAML(data)` decodes YAML into the plain form the validators take.
- CLI: `graphnode validate-manifest <manifest.yaml>` prints the problems as JSON and exits 0 clean, 1 on an error, 2 on a file it cannot read.

Every card problem is `schema`, with a JSON pointer into the card: `/` for a missing required key or an unknown key (the message names it), `/type` for an unknown type, `/needs/0/os`, `/verbs/0/command`, `/verbs/0/os/1`, `/verbs/0/targets/0`, `/extends`, `/files/0`, `/pins/Graph.Node`. A file that is not YAML is `schema` at `/`. A bad key under `pins` is reported at `/`, with the key in the message.

Derive the index. A repo's `index.yaml` sits at its root and is never written by hand:
- `New-GraphIndex -Root <repo>`, or `graphnode index <root>`, or `graph.BuildIndex(root, time.Now())` then `graph.SaveIndex(filepath.Join(root, graph.IndexFile), idx)`, walks the root for every `manifest.yaml` (skipping `.git` and `node_modules`), validates each, and writes `index.yaml`: `repo` (the root's folder name), `generated` (UTC ending `Z`) and `entries`, one `{path, name, version, type, tags}` per card, sorted by `path`, the card's folder relative to the root with forward slashes (`emitters/Terraform`; `.` for a card at the root). `New-GraphIndex` returns the file.
- Any invalid card, or a second card with a name already taken, fails it: nothing is written and an existing `index.yaml` is left as it was. `New-GraphIndex` throws `index not written; <n> problem(s): ...`, `graphnode index` prints the problems and exits 1. Each problem's path is the card's file relative to the root, a colon, then the pointer: `emitters/Terraform/manifest.yaml:/version`.
- Regenerate it in the same change as any card change. Read it with any YAML reader; check it with `graphnode validate-index <index.yaml>` or `graph.ValidateIndex(doc)` (Go types `graph.Index`, `graph.IndexEntry`).

`Test-GraphManifest` and `New-GraphIndex` read YAML through the `graphnode` binary, so they always need it built in the GraphNode checkout, and throw `Test-GraphManifest needs the graphnode binary: build the graphnode binary; see README` (or `New-GraphIndex needs ...`) without it. There is no pure-PowerShell card check.

| Code | Means |
|---|---|
| `schema` | A card breaks `manifest.schema.json` (a missing or unknown key, a bad name, version, type, os, effect, command, target, extends, file or pin), or is not YAML; or an index breaks `index.schema.json`. |
| `duplicate-name` | Building an index: this card's `name` is already the name of an earlier card (by path); the message names it. |
| `stamp` | An index's `generated` is not a UTC stamp ending `Z`. |

## What not to do

- No per-emitter node types. One Node shape with a `kind` string; what a Kind carries goes in `properties` and is declared in `ontology.yaml`.
- No display on nodes or edges (`color`, `shape`, `icon`, ...): the schema rejects it. Display lives in `ontology.yaml`.
- No edge to a missing node except one into another module's registered prefix, or an `about` edge from an `alert:` id into any registered prefix; those are warnings until a merge resolves them. Inside your own ids, a missing endpoint is an error.
- No hand-built edge ids. Use `New-GraphEdge`, `graph.NewEdge` or `graph.EdgeID`.
- No Kind, edge Kind or finding Kind that is not declared in your `ontology.yaml`.
- No `Category` nodes from an emitter, and no edits to another module's layers.
- No hand-written or hand-edited `index.yaml`; derive it with `New-GraphIndex` or `graphnode index`.
- No cmdlet or executable names in a verb's `description`; they go in `command`.
- No test or fixture that reads a sibling repo at `HEAD`, a branch or its working tree; pin to a tag, never HEAD.

## Changes

- 0.1.0: initial; stamps (`layer`, `stamp.*`, ActionRun `startedAt`, `finishedAt`) must be UTC ending `Z`, builders convert `[datetime]`, new code `stamp`; `display` on a node or edge is rejected (`schema`); pure `Test-Graph` checks the edge Kind pattern; a relative `envelope.ontology` path is relative to the envelope file's directory; `-Strict` errors for an unbuilt binary or an unsupported platform.
- 0.2.0: problems carry `severity` (`error`, `warning`) and only errors fail; Kind `display` checked (`#rrggbb` colour, shape circle|square|diamond|hexagon, integer `size`), edge Kind `display` is `{color, style}`, new code `display`; id prefix registry (`schemas/ids.md`), `Get-GraphIdPrefix`, `Test-GraphId`, `graphnode prefixes` and `check-id`, `graph.IDPrefixes`, `ValidID`, `PrefixOf`, and a dangling edge to another module's prefix is a warning; `%` and `|` in edge id parts are percent-encoded, new code `edgeid`, `graph.ParseEdgeID`; action `inputs` as `[{name, type, required}]` and `returns: {type}`, ActionRun `result`, `Test-Graph -Ontology` and `graphnode validate --ontology` check ActionRuns, new code `action`, `graph.ValidateWithOntology`, `CheckActionRuns`, `HasErrors`. 0.1.0 shapes and `inputs` maps are warnings until 0.3.0.
- 0.2.1: `ui:` is registered to Graph.Portal, not GraphRenderer; `alert:` and `ledger:` are registered to Claude.Agent.Alerts, with their shapes checked by `Test-GraphId`, `graph.ValidID` and `graphnode check-id` (`schemas/ids.md`, `Get-GraphIdPrefix`, `graph.IDPrefixes`, `graphnode prefixes`).
- 0.2.3: `agent:` (Graph.Emitter.Agent), `runner:` (Claude.Agent.Runner), `git:` (Graph.Emitter.Git.Repository) and `ref:` (Graph.Emitter.Reference) registered with shape checks; `ui:` owned by Portal; shapes `triangle` and `star`; Kind `display.when` overrides display by property value; a dangling edge onto the envelope's own module's prefix is a warning in pure `Test-Graph`, an error with `-Strict` / Go; envelope `source` block `{emitter, version, target, stamp}`, `New-GraphEnvelope -Source`, `graph.EnvelopeSource`; list-valued properties stay lists in the builders.
- 0.3.0: manifest cards: `manifest.yaml` per packaged emitter, skill, tool or rule (`manifest.schema.json`: `name`, `version`, `type`, `description`, optional `tags`, `needs`, `verbs`, `pins`) and the derived repo `index.yaml` (`index.schema.json`); `Test-GraphManifest`, `New-GraphIndex`; `graphnode validate-manifest`, `validate-index`, `index`; Go `Manifest`, `ManifestNeed`, `ManifestVerb`, `Index`, `IndexEntry`, `ManifestFile`, `IndexFile`, `DecodeYAML`, `LoadManifest`, `ValidateManifest`, `ValidateIndex`, `BuildIndex`, `MarshalIndex`, `SaveIndex`; new code `duplicate-name`; `reference/manifest.example.yaml`.
- 0.3.1: card fields, additive (every 0.3.0 card stays valid): type `workflow`; verb `command` (the cmdlet or executable), `os` (`[windows, linux, macos]`, absent means all) and `targets` (card names); card `extends` (a card name, not resolved) and `files` (paths relative to the card); `effect` wording (own output, local refs, the plan file and ledger lines are not effects; discover, plan, run, report and fetch are `read`); Go `Manifest.Extends`, `Files`, `ManifestVerb.Command`, `OS`, `Targets`. Id formats: GraphNode reserves prefix and owner, the format is the owner's ontology Kind `idPattern` when reachable beside the repo, else the registry's shape; `Test-GraphId -SiblingRoot`, `graphnode check-id --siblings`, `graphnode id-format`, Go `IDFormat`, `ResolveIDFormat`, `ValidIDIn`, `OwnerOntologyPaths`, `ErrUnregisteredPrefix`, `IDFormatOntology`, `IDFormatRegistry`; a bad `idPattern` is `schema`. Rule: cross-repo fixtures pin to a tag, never HEAD. `reference/` refreshed.
- 0.3.2: prefix `env:` (Claude.Agent.Orchestrator: `env:compose/<project>`, `env:container/<project>/<service>`, `env:image/<reference>`, `env:network/<name>`); `alert:` also takes `alert:<fingerprint>` (64 lower-case hex); `ledger:` shared with Portal and Claude.Agent.Orchestrator, a new registry column `SharedWith` (`Get-GraphIdPrefix`, Go `IDPrefix.SharedWith`, `graphnode prefixes` `sharedWith`); the `to` of an `about` edge from an `alert:` id onto any registered prefix is a `dangling-edge` warning in every validator.
- 0.3.3: `SharedWith` is descriptive only: `alert:` shared with Claude.Agent.Orchestrator; `ledger:` shared with Portal, Claude.Agent.Orchestrator and Claude.Agent.Ledger. No check changes.
