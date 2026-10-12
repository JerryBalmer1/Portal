# The tenant graph query

`GET /api/tenants/{tenant}/query` (operationId `queryTenantGraph`) answers multi-hop questions across every mounted layer of a tenant at once. It walks breadth-first and in memory, and returns a normal Graph.Node envelope. The decisions behind it are in [design.md entry 54](../design.md). The schema is `GraphQuery` in [api/openapi.yaml](../../api/openapi.yaml).

The walk reads the same graph `GET .../graph` serves. A node id seen in two layers keeps its first in mount order. A node with no stamp gets `stamp.layer` from its layer. An alert layer's copies of its subjects are left out. Edges are kept once. `GET .../graph?source=` is this walk with every node a start, the source set and depth 0.

## Parameters

| Parameter | Default | Meaning |
|---|---|---|
| `from` | none | Start node ids, comma-separated. Required unless `kind` is given. An id that no node has is 400, and the message names it. |
| `kind` | none | Start at every node of this Kind instead. `from` and `kind` together is 400. A Kind that no node has is an empty 200. |
| `follow` | every kind | Edge kinds to walk, comma-separated. |
| `direction` | `out` | `out` walks the edges from a node, `in` the edges to it, `both` either. |
| `depth` | 3 | Hops at most, 0 to 10. 0 is the starts alone. |
| `until` | none | Stop at nodes of this Kind: they are in the result, their neighbours are not. A start is always expanded. |
| `where` | none | Repeatable `key=value`; every one must match for a node to enter the result. A node that fails is neither in the result nor expanded, and that includes a start. The key `id`, `kind` or `name` reads the node's own field. Any other key is a dotted path into `properties` (`facet.key`). `properties.<path>` reads a property that shares a name with a node field: `where=properties.kind=container` matches an instance whose `properties.kind` is `container`, while `where=kind=container` asks for the node's Kind. The match is exact against a string, or against a number or boolean as JSON writes it (`n=1`, `internal=true`). A list or a map never matches. |
| `source` | every layer | Only the layers whose Graph.Node source block names this emitter (`source.emitter`, exact and case-sensitive). A `from` id outside them is 400. |
| `limit` | 500 | Nodes at most, 1 to 5000. When the next node would pass the limit, the walk stops: `truncated` is `true` and `truncatedAt` is the hop it stopped on (0 when the starts alone pass it). |

## Response

The response is an envelope (`schema`, `module: Portal`, `version`, `layer`, `nodes`, `edges`, `findings`, `counts`) with these members beside it:
- `paths`: for each start, the id chain to each node the walk stopped at. The walk stops at a node when `until` matched, when depth ran out, or when nothing new lay beyond it. A start that reaches nothing is a path of one.
- `query`: the parameters as resolved, defaults filled in.
- `truncated`, and `truncatedAt` when it is true.

The result's edges are every edge between two of its nodes, whichever way the walk went. The envelope passes `graphnode validate` (the schema check `Test-Graph -Strict` runs): Graph.Node's envelope schema allows members beside its own.

Failures:
- 400 `unknown_node`: a `from` id no node has.
- 400 `bad_request`: no `from` and no `kind`, both, a `direction` other than `out`, `in` or `both`, `depth` outside 0 to 10, `limit` outside 1 to 5000, or a `where` that is not `key=value`.
- 404: a tenant this process does not serve.

On the page, Graph/Explore's query bar writes these parameters into its own URL, so a query is a link: `/default/graph/explore?from=env%3Acontainer%2Fclaude-agent-dev%2Fmitm&follow=attached-to&direction=both&depth=2`. Right-click any node of a graph page and choose Query from here to open Explore from that node.

## Worked examples against the dev tenant

Run on 2026-10-12 over the dev tenant's eleven mounted layers: the same files, in the same mount order, as `GET http://127.0.0.1:8200/api/tenants/default/layers` listed them. They were served by this build (0.16.0) on port 18260 from a throwaway tenant, so the running dev portal was not restarted. Every 200 below passed `graphnode validate` (exit 0). The outputs are the real responses. After the first, they are cut to ids, Kinds, names, edges, paths and the truncation; `properties`, `source` and `stamp` are left out.

### 1. A container's network neighbours

The brief asked for this from Emitter.Network. In the dev tenant, Emitter.Network's layer holds two `RemoteHost` nodes and no container. The containers and their networks are Emitter.Environment's (`agent-environment`, Portal.Agent's environment layer). So the walk runs over every layer, and `source=Emitter.Network` is shown at the end for contrast.

The container's networks, one hop along `attached-to`:

```
GET /api/tenants/default/query?from=env%3Acontainer%2Fclaude-agent-dev%2Fmitm&follow=attached-to&depth=1
```

```json
{
 "schema": "graph/1",
 "module": "Portal",
 "version": "0.16.0",
 "layer": "2026-10-12T03:15:23.525Z",
 "nodes": [
  {
   "id": "env:container/claude-agent-dev/mitm",
   "kind": "environment.instance",
   "name": "mitm (mitmproxy/mitmproxy:12.2.3)",
   "properties": {
    "composeProject": "claude-agent-dev",
    "containerId": "6e20843a757454e2070f9b009c49947bc344f47ab4a3f9a8559d6a15c09450b2",
    "env": "dev",
    "image": "mitmproxy/mitmproxy:12.2.3",
    "imageId": "sha256:90d20030c6789da3a4164407a6934ecd30f1b88e7941161e6fe9c703c91d589d",
    "kind": "container",
    "networks": ["inside", "outside"],
    "service": "mitm",
    "started": "2026-10-12T02:47:33.665982398Z",
    "state": "running",
    "updated": "2026-10-12T02:47:51Z"
   },
   "source": [{ "tool": "Claude.Agent.Orchestrator", "path": "docker inspect (compose project claude-agent-dev)" }],
   "stamp": { "layer": "2026-10-12T02:47:51Z" }
  },
  {
   "id": "env:network/claude-agent-dev_inside",
   "kind": "environment.network",
   "name": "claude-agent-dev_inside (bridge, internal)",
   "properties": { "composeProject": "claude-agent-dev", "driver": "bridge", "env": "dev", "internal": true, "subnet": "172.18.0.0/16", "updated": "2026-10-12T02:47:51Z" },
   "source": [{ "tool": "Claude.Agent.Orchestrator", "path": "docker inspect (compose project claude-agent-dev)" }],
   "stamp": { "layer": "2026-10-12T02:47:51Z" }
  },
  {
   "id": "env:network/claude-agent-dev_outside",
   "kind": "environment.network",
   "name": "claude-agent-dev_outside (bridge)",
   "properties": { "composeProject": "claude-agent-dev", "driver": "bridge", "env": "dev", "internal": false, "subnet": "172.19.0.0/16", "updated": "2026-10-12T02:47:51Z" },
   "source": [{ "tool": "Claude.Agent.Orchestrator", "path": "docker inspect (compose project claude-agent-dev)" }],
   "stamp": { "layer": "2026-10-12T02:47:51Z" }
  }
 ],
 "edges": [
  { "id": "env:container/claude-agent-dev/mitm|env:network/claude-agent-dev_inside|attached-to", "kind": "attached-to", "from": "env:container/claude-agent-dev/mitm", "to": "env:network/claude-agent-dev_inside" },
  { "id": "env:container/claude-agent-dev/mitm|env:network/claude-agent-dev_outside|attached-to", "kind": "attached-to", "from": "env:container/claude-agent-dev/mitm", "to": "env:network/claude-agent-dev_outside" }
 ],
 "findings": [],
 "counts": { "nodes": 3, "edges": 2, "byKind": { "environment.instance": 1, "environment.network": 2 } },
 "paths": [
  ["env:container/claude-agent-dev/mitm", "env:network/claude-agent-dev_inside"],
  ["env:container/claude-agent-dev/mitm", "env:network/claude-agent-dev_outside"]
 ],
 "query": { "from": ["env:container/claude-agent-dev/mitm"], "follow": ["attached-to"], "direction": "out", "depth": 1, "where": [], "limit": 500 },
 "truncated": false
}
```

The containers that share a network with it, two hops along `attached-to` in both directions:

```
GET /api/tenants/default/query?from=env%3Acontainer%2Fclaude-agent-dev%2Fmitm&follow=attached-to&direction=both&depth=2
```

```
nodes:  env:container/claude-agent-dev/mitm      environment.instance  mitm (mitmproxy/mitmproxy:12.2.3)
        env:network/claude-agent-dev_inside      environment.network   claude-agent-dev_inside (bridge, internal)
        env:network/claude-agent-dev_outside     environment.network   claude-agent-dev_outside (bridge)
        env:container/claude-agent-dev/agent     environment.instance  agent (claude-agent:dev)
edges:  env:container/claude-agent-dev/mitm|env:network/claude-agent-dev_inside|attached-to
        env:container/claude-agent-dev/mitm|env:network/claude-agent-dev_outside|attached-to
        env:container/claude-agent-dev/agent|env:network/claude-agent-dev_inside|attached-to
paths:  mitm > claude-agent-dev_outside
        mitm > claude-agent-dev_inside > agent
truncated: false
```

The agent container is mitm's only neighbour, on the internal network. Nothing else is attached to `_outside`.

`where` stops the walk as well as filtering the result. With `where=properties.kind=container`, the networks fail the filter, so they are not expanded, and the agent is never reached:

```
GET /api/tenants/default/query?from=env%3Acontainer%2Fclaude-agent-dev%2Fmitm&follow=attached-to&direction=both&depth=2&where=properties.kind=container
```

```
nodes:  env:container/claude-agent-dev/mitm      environment.instance
edges:  (none)
paths:  mitm
```

Emitter.Network's own layer holds two hosts and the edge between them:

```
GET /api/tenants/default/query?kind=RemoteHost&source=Emitter.Network&direction=both&depth=1
```

```
nodes:  dns:localhost   RemoteHost  localhost
        127.0.0.1       RemoteHost  localhost
edges:  127.0.0.1|dns:localhost|ResolvesTo
paths:  dns:localhost > 127.0.0.1
        127.0.0.1 > dns:localhost
query:  {"from":[],"kind":"RemoteHost","follow":[],"direction":"both","depth":1,"where":[],"source":"Emitter.Network","limit":500}
```

Asking from the container inside Emitter.Network's layers is 400, since no layer of that emitter holds it:

```
GET /api/tenants/default/query?from=env%3Acontainer%2Fclaude-agent-dev%2Fmitm&source=Emitter.Network
400 {"code":"unknown_node","message":"from: no node env:container/claude-agent-dev/mitm in the mounted layers"}
```

### 2. A Terraform module's resources

Emitter.Terraform 0.17.2's dev layer joins modules by `Calls` edges. It has no edge from a module to its resources: a resource names its module in `properties.Module`. So a module's resources are a `where` on that property, and its submodules are a walk along `Calls`.

The resources of `module.network`:

```
GET /api/tenants/default/query?kind=Resource&where=Module=module.network&depth=0
```

```
nodes:  module.network/resource/null_resource.subnet   Resource  subnet
        properties: {"File":"main.tf","Line":9,"Module":"module.network","ProviderAddress":"registry.terraform.io/hashicorp/null",
                     "ProviderLocalName":"null","Reason":"NoSchemaGraph","ResourceAddress":"module.network.null_resource.subnet",
                     "SchemaId":"registry.terraform.io/hashicorp/null/resource/null_resource","SchemaMatched":false,"Type":"null_resource"}
edges:  (none)
paths:  module.network/resource/null_resource.subnet
query:  {"from":[],"kind":"Resource","follow":[],"direction":"out","depth":0,"where":["Module=module.network"],"limit":500}
```

Its submodules:

```
GET /api/tenants/default/query?from=module.network&follow=Calls&depth=10
```

```
nodes:  module.network                    Module  network
        module.network.module.endpoint    Module  endpoint
edges:  module.network|module.network.module.endpoint|Calls
paths:  module.network > module.network.module.endpoint
```

The whole call tree from the root, within Emitter.Terraform's layers:

```
GET /api/tenants/default/query?from=root&follow=Calls&depth=10&source=Emitter.Terraform
```

```
nodes:  root                              Module  root
        module.network                    Module  network
        module.network.module.endpoint    Module  endpoint
edges:  root|module.network|Calls
        module.network|module.network.module.endpoint|Calls
paths:  root > module.network > module.network.module.endpoint
```

### 3. The ledger chain from one entry, with direction=in

This example walks Emitter.Ledger's agent chain (`agent-ledger`): LedgerEntry nodes under `ledger:<seq>`, each with a `prev` edge to the entry before it. Example 4 walks the tenant's own action chain.

`prev` points from the newer entry to the older one. So `direction=in` walks forwards in time, to the entries that chain onto this one. `direction=out` walks backwards, to the entries it chains onto. This example starts from the policy decision `ledger:25`, a deny.

Forwards, `direction=in`:

```
GET /api/tenants/default/query?from=ledger%3A25&follow=prev&direction=in&depth=3
```

```
nodes:  ledger:25  LedgerEntry  policy.deny pre File.delete /work/policy-smoke.txt
        ledger:26  LedgerEntry  agent.run exit 0
        ledger:27  LedgerEntry  agent.stop
        ledger:28  LedgerEntry  agent.egress-check directBlocked True
edges:  ledger:26|ledger:25|prev
        ledger:27|ledger:26|prev
        ledger:28|ledger:27|prev
paths:  ledger:25 > ledger:26 > ledger:27 > ledger:28
query:  {"from":["ledger:25"],"follow":["prev"],"direction":"in","depth":3,"where":[],"limit":500}
```

Backwards, `direction=out`:

```
GET /api/tenants/default/query?from=ledger%3A25&follow=prev&depth=3
```

```
nodes:  ledger:25  LedgerEntry  policy.deny pre File.delete /work/policy-smoke.txt
        ledger:24  LedgerEntry  policy.allow post File.write /work/policy-smoke.txt
        ledger:23  LedgerEntry  policy.allow pre File.write /work/policy-smoke.txt
        ledger:22  LedgerEntry  policy.allow post File.read /etc/hostname
edges:  ledger:25|ledger:24|prev
        ledger:24|ledger:23|prev
        ledger:23|ledger:22|prev
paths:  ledger:25 > ledger:24 > ledger:23 > ledger:22
```

The same walk backwards to depth 10, capped at 4 nodes. It stops on hop 4, and the response says so:

```
GET /api/tenants/default/query?from=ledger%3A25&follow=prev&depth=10&limit=4
```

```
nodes:  ledger:25, ledger:24, ledger:23, ledger:22
truncated: true, truncatedAt: 4
query:  {"from":["ledger:25"],"follow":["prev"],"direction":"out","depth":10,"where":[],"limit":4}
```

### 4. An action result back to its request, its container and its principal

Since 0.16.1 the tenant's `ledger.jsonl` is mounted as the layer `tenant-actions` (design.md entry 55). Its lines are LedgerEntry nodes under `ledger:tenant/<tenant>/<seq>`, apart from the agent chain's `ledger:<seq>`. A result has a `request` edge to its request. A request has a `subject` edge to its container and a `principal` edge to whoever asked. Every line has a `prev` edge to the line before it. The ids and the line contract are in [actions.md](actions.md).

These are the dev tenant's layers on 2026-10-12, with `agent-ledger` and `agent-environment` mounted from Portal.Agent's latest layers. Line 24 is the Runner's `action.result`, the newest action line. It answers request 23, a `logs` on the ledger keeper, asked by `dev-bootstrap`. All of a request's edges point away from it, so `direction=out` is the walk that reaches the request, the container (and its compose stack and image, from the environment layer) and the principal:

```
GET /api/tenants/default/query?from=ledger%3Atenant%2Fdefault%2F24&direction=out&depth=5
```

```
nodes:  ledger:tenant/default/24                         LedgerEntry            done ledger:tenant/default/23
        ledger:tenant/default/23                         LedgerEntry            logs env:container/claude-agent-dev/ledger
        ledger:tenant/default/22                         LedgerEntry            pushed Emitter.Terraform 0.17.1 to cards-oci
        env:container/claude-agent-dev/ledger            environment.instance   ledger (claude-agent-ledger:dev)
        ui:principal:dev-bootstrap                       Principal              dev-bootstrap
        ledger:tenant/default/21                         LedgerEntry            duplicate Emitter.Reference 0.1.1 to cards-oci
        env:compose/claude-agent-dev                     environment.instance   claude-agent-dev (compose on MSI)
        env:image/claude-agent-ledger:dev                environment.image      claude-agent-ledger:dev
        ledger:tenant/default/20                         LedgerEntry            duplicate Emitter.Network 0.4.0 to cards-oci
        ledger:tenant/default/19                         LedgerEntry            duplicate Emitter.Ledger 0.4.0 to cards-oci
edges:  ledger:tenant/default/24|ledger:tenant/default/23|prev
        ledger:tenant/default/24|ledger:tenant/default/23|request
        ledger:tenant/default/23|ledger:tenant/default/22|prev
        ledger:tenant/default/23|env:container/claude-agent-dev/ledger|subject
        ledger:tenant/default/23|ui:principal:dev-bootstrap|principal
        ledger:tenant/default/22|ledger:tenant/default/21|prev
        env:container/claude-agent-dev/ledger|env:compose/claude-agent-dev|member-of
        env:container/claude-agent-dev/ledger|env:image/claude-agent-ledger:dev|runs-image
        ledger:tenant/default/21|ledger:tenant/default/20|prev
        env:compose/claude-agent-dev|ui:principal:dev-bootstrap|runs-as
        ledger:tenant/default/20|ledger:tenant/default/19|prev
paths:  ledger:tenant/default/24 > ledger:tenant/default/23 > ui:principal:dev-bootstrap
        ledger:tenant/default/24 > ledger:tenant/default/23 > env:container/claude-agent-dev/ledger > env:compose/claude-agent-dev
        ledger:tenant/default/24 > ledger:tenant/default/23 > env:container/claude-agent-dev/ledger > env:image/claude-agent-ledger:dev
        ledger:tenant/default/24 > ledger:tenant/default/23 > ledger:tenant/default/22 > ledger:tenant/default/21 > ledger:tenant/default/20 > ledger:tenant/default/19
query:  {"from":["ledger:tenant/default/24"],"follow":[],"direction":"out","depth":5,"where":[],"limit":500}
```

`direction=in` walks to the lines that chain onto line 24, the newer ones:

```
GET /api/tenants/default/query?from=ledger%3Atenant%2Fdefault%2F24&direction=in&depth=5
```

```
nodes:  ledger:tenant/default/24                         LedgerEntry            done ledger:tenant/default/23
        ledger:tenant/default/25                         LedgerEntry            duplicate Emitter.Agent 0.1.4 to cards-oci
        ledger:tenant/default/26                         LedgerEntry            pushed Emitter.AzureDevOps 0.3.2 to cards-oci
        ledger:tenant/default/27                         LedgerEntry            duplicate Emitter.Environment 0.3.0 to cards-oci
        ledger:tenant/default/28                         LedgerEntry            pushed Emitter.FileSystem 0.2.1 to cards-oci
        ledger:tenant/default/29                         LedgerEntry            pushed Emitter.Git 0.1.2 to cards-oci
edges:  ledger:tenant/default/25|ledger:tenant/default/24|prev
        ledger:tenant/default/26|ledger:tenant/default/25|prev
        ledger:tenant/default/27|ledger:tenant/default/26|prev
        ledger:tenant/default/28|ledger:tenant/default/27|prev
        ledger:tenant/default/29|ledger:tenant/default/28|prev
paths:  ledger:tenant/default/24 > ledger:tenant/default/25 > ledger:tenant/default/26 > ledger:tenant/default/27 > ledger:tenant/default/28 > ledger:tenant/default/29
query:  {"from":["ledger:tenant/default/24"],"follow":[],"direction":"in","depth":5,"where":[],"limit":500}
```

The ids no longer collide. `ledger:24` is the agent chain's entry 24, and the tenant's line 24 is `ledger:tenant/default/24`:

```
GET /api/tenants/default/query?from=ledger%3A24&depth=0
```

```
nodes:  ledger:24                                        LedgerEntry            policy.allow post File.write /work/policy-smoke.txt
paths:  ledger:24
query:  {"from":["ledger:24"],"follow":[],"direction":"out","depth":0,"where":[],"limit":500}
```

Environments/Actions opens this walk for any request: a click on its row goes to Explore at `?from=<request id>&direction=out&depth=5`.
