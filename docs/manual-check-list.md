# Portal manual check list

Verified for: 0.4.0 on 2026-10-10, Windows 11 (10.0.26200), PowerShell 7.6.6, Go 1.27.0, Pester 6.2.0, InvokeBuild 5.14.23: items 0.1 not re-run (go.mod unchanged), 0.2 in full (Go 46 tests, Smoke passed, Pester 68 passed after the repos endpoint took a token; 1.10 re-run then), 0.3 not run (the hook refuses an agent's call quoting the commands; Pester runs the same six), 1.1 to 1.11 and 3.1 to 3.5 run as written and their Expects are the output seen, 2.1 run (three PNGs, canvas coloured 58/58 with the sibling's layer). 2.2 to 2.5 are interactive and were not run as written; the same paths were checked in Chrome against a live portal on another port (sign-in by token, Service connections Test showing `no_token` once signed in, Designer Sections and Export, the Workflows item tabs, Instances filter chips, rail collapse to 52 px).

Run each block by saving it to a temp `.ps1` and `pwsh -NoProfile -File <temp>.ps1` from the repo root. Never `pwsh -Command { }`. Items 1 to 3 need item 0.2 (or `Invoke-Build Build`) to have built `build/win-x64/`. Every block starts its portal on a fresh directory under `$env:TEMP` and on the test port 18200, one at a time (0 is refused; docs/design.md 24): a directory that holds snaps boots from the newest of them.

Since 0.4.0 every API path names the tenant and a write needs a bearer token (docs/design.md 36). A block that writes first makes a tenant under `$env:TEMP\portal-check-tenants` with a fresh token, `check`, in `PORTAL_TENANTS` and `PORTAL_TOKEN`, which `Start-Portal` and the module's calls read, and `$auth`, the header for a raw call. The repo's own `tenants/default/tokens.yaml` is never touched.

## 0. Toolchain

### 0.1 Go and modules
```powershell
go version
go mod verify
git status --short go.mod go.sum
```
Expect: `go version go1.27.0 windows/amd64` (1.27.0 or later), `all modules verified`, then `A  go.mod` and `A  go.sum` while staged and uncommitted, nothing once committed.

### 0.2 Full build
```powershell
Invoke-Build
```
Expect: go test ends `ok` for `cmd/portal`, `internal/server`, `internal/store` and `internal/tenant` (46 tests); Smoke prints `smoke: seed 20 nodes; approved <a>, then <b> changing ui:theme:default; restart booted from <b>; canvas fit {"graphWidth":<g>,"panelWidth":<p>,"graphHeight":<h>,"panelHeight":<q>,"ratio":<r>}` with `<r>` above 0.5 (0.853 here), and `artifacts/smoke-report.json` has `checks.auth` `{"anonymousWrite":401,"principal":"smoke"}` and `checks.rail.rail` the thirteen Services from `ui:rail:overview` to `ui:rail:settings`; Pester ends `Tests Passed: 68, Failed: 0, Skipped: 0`; then `Build succeeded. 6 tasks, 0 errors, 0 warnings`.

### 0.3 git-guard hook
Run from a shell that is not an agent's (the hook refuses an agent's call that quotes these commands).
```powershell
'{"tool_name":"Bash","tool_input":{"command":"git status"}}' | pwsh -NoProfile -File .claude/skills/git-guard/hook.ps1; "exit $LASTEXITCODE"
'{"tool_name":"Bash","tool_input":{"command":"echo hi && git push"}}' | pwsh -NoProfile -File .claude/skills/git-guard/hook.ps1; "exit $LASTEXITCODE"
```
Expect: `exit 0`, then `git-guard: agents edit and stage only; commits are run by Jerry from a separate shell` and `exit 2`.

## 1. API

### 1.1 Dev seed and the filesystem layer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.1'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    $s = Get-PortalScreen -Url $p.Url
    "boot: $($p.Boot); nodes: $($s.nodes.Count), edges: $($s.edges.Count), root: $($s.root)"
    $s.nodes | Group-Object kind | Sort-Object Name | ForEach-Object { "$($_.Name): $($_.Count)" }
    $s.nodes | Where-Object id -Like 'ui:state:designer:*' | ForEach-Object { "$($_.id) collapsed $($_.properties.collapsed)" }
    $l = Invoke-RestMethod "$($p.Url)/api/tenants/default/layers"
    "layer: $($l[0].name) $($l[0].counts.nodes) nodes, ontology $($l[0].hasOntology)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `boot: seed; nodes: 20, edges: 19, root: ui:root`, then `Category: 5`, `Container: 4`, `Layout: 1`, `State: 7`, `Theme: 1`, `Widget: 2`, then the seven section States in order, `ui:state:designer:approve collapsed False`, `...kinds collapsed True`, `...categories collapsed False`, and `add-panel`, `add-container`, `add-category`, `add-menu-entry` all `collapsed True`, then `layer: filesystem 52 nodes, ontology True`.

### 1.2 Display, Graph.Node shape and Category refusals
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.2'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    foreach ($body in '{"id":"ui:w","kind":"Widget","properties":{"color":"#ff0000"}}',
                      '{"id":"ui:w","kind":"Widget","display":{"color":"#ff0000"}}',
                      '{"id":"ui:w","kind":"Widget","properties":{"type":"Nope"}}',
                      '{"id":"ui:theme:x","kind":"Theme","properties":{"opacity":2}}',
                      '{"id":"ui:category:X","kind":"Category","name":"X"}') {
        $r = Invoke-WebRequest -Method Post -Uri "$($p.Url)/api/tenants/default/nodes" -Headers $auth -Body $body -ContentType application/json -SkipHttpErrorCheck
        "$($r.StatusCode) $((($r.Content | ConvertFrom-Json).problems.code) -join ',')"
    }
    $null = Invoke-RestMethod -Method Post -Uri "$($p.Url)/api/tenants/default/nodes" -Headers $auth -Body '{"id":"ui:w","kind":"Widget"}' -ContentType application/json
    $r = Invoke-WebRequest -Method Post -Uri "$($p.Url)/api/tenants/default/edges" -Headers $auth -Body '{"kind":"is-a","from":"ui:w","to":"ui:category:Nope"}' -ContentType application/json -SkipHttpErrorCheck
    "$($r.StatusCode) $((($r.Content | ConvertFrom-Json).problems.code) -join ',')"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `422 display`, `422 schema`, `422 unknown-category`, `422 property-type` (Theme opacity above 1), `422 property-type` (a Category with no properties, which Graph.Node 0.2.0 lets through), `422 dangling-edge,unknown-category`.

### 1.3 A layer that fails graphnode validate is refused
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.3'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    try { Mount-PortalLayer -Name bad -Path .\tests\fixtures\invalid.graph.json -Url $p.Url }
    catch { "refused: $((@($_.TargetObject.problems.code) | Sort-Object -Unique) -join ', ')" }
    $layers = Invoke-RestMethod "$($p.Url)/api/tenants/default/layers"
    "mounted: $(@($layers).Count)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `refused: dangling-edge, duplicate-node, edge-id, edgeid, kind, schema, stamp`, then `mounted: 0`.

### 1.4 Production hides the Designer and only the Designer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dev  = Join-Path $env:TEMP 'portal-check-1.4-dev'
$prod = Join-Path $env:TEMP 'portal-check-1.4-prod'
Remove-Item $dev, $prod -Recurse -Force -ErrorAction SilentlyContinue
$d = Start-Portal -Port 18200 -Artifacts $dev -Filesystem ''
try { $full = (Get-PortalScreen -Url $d.Url).nodes.id } finally { Stop-Portal -Artifacts $dev }
$p = Start-Portal -Port 18200 -Artifacts $prod -Filesystem '' -Production
try {
    $shown = (Get-PortalScreen -Url $p.Url).nodes.id
    "hidden: $(($full | Where-Object { $_ -notin $shown }) -join ', ')"
}
finally {
    Stop-Portal -Artifacts $prod
    Remove-Item $dev, $prod -Recurse -Force -ErrorAction SilentlyContinue
}
```
Expect: `hidden: ui:designer` (its section States stay, as every hidden node's State does; docs/design.md 3).

### 1.5 Approve, boot from the newest snap, PORTAL_SNAP
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.5'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$exe = '.\build\win-x64\portal.exe'
$boot = { param($url) [string](Invoke-WebRequest "$url/api/tenants/default/screen").Headers['X-Portal-Boot'] }
$run = {
    param($port)
    $proc = Start-Process $exe -ArgumentList '-port', $port, '-artifacts', $dir, '-tenants', $env:PORTAL_TENANTS, '-filesystem', '""' -PassThru -NoNewWindow -RedirectStandardOutput "$dir.out" -RedirectStandardError "$dir.err"
    foreach ($i in 1..100) { try { $null = Invoke-WebRequest "http://127.0.0.1:$port/api/tenants/default/screen"; break } catch { Start-Sleep -Milliseconds 100 } }
    $proc
}
$null = New-Item -ItemType Directory -Force $dir
$proc = & $run 18951
try {
    "boot: $(& $boot 'http://127.0.0.1:18951')"
    $a = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:18951/api/tenants/default/snap -Headers $auth -ContentType application/json -Body '{"note":"first"}'
    "first: previous [$($a.previous)] diff [$($a.diff)]"
    $null = Invoke-RestMethod -Method Patch -Uri 'http://127.0.0.1:18951/api/tenants/default/nodes/ui:theme:default' -Headers $auth -ContentType application/json -Body '{"properties":{"color":"#336699"}}'
    $b = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:18951/api/tenants/default/snap -Headers $auth -ContentType application/json -Body '{"note":"blue"}'
    "second: previous is first $($b.previous -eq $a.stamp); changed $($b.diff.changed.nodes -join ',')"
}
finally { Stop-Process -Id $proc.Id -Force; $proc.WaitForExit() }
$proc = & $run 18952
try { "restart boots from second: $((& $boot 'http://127.0.0.1:18952') -eq $b.stamp)" }
finally { Stop-Process -Id $proc.Id -Force; $proc.WaitForExit() }
$env:PORTAL_SNAP = $a.stamp
$proc = & $run 18953
try { "PORTAL_SNAP boots from first: $((& $boot 'http://127.0.0.1:18953') -eq $a.stamp)" }
finally { Stop-Process -Id $proc.Id -Force; $proc.WaitForExit(); Remove-Item Env:PORTAL_SNAP }
Remove-Item $dir, "$dir.out", "$dir.err" -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: `boot: seed`, `first: previous [] diff []`, `second: previous is first True; changed ui:theme:default`, `restart boots from second: True`, `PORTAL_SNAP boots from first: True`.

### 1.6 A menu entry is a node and two edges, and survives Approve and a restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.6'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $post = { param($path, $body) Invoke-WebRequest -Method Post -Uri "$($p.Url)/api/tenants/default$path" -Headers $auth -Body $body -ContentType application/json -SkipHttpErrorCheck }
    (& $post '/nodes' '{"id":"ui:menu:designer","kind":"Container","name":"Designer","properties":{"order":2}}').StatusCode
    (& $post '/edges' '{"kind":"is-a","from":"ui:menu:designer","to":"ui:category:Menu"}').StatusCode
    (& $post '/edges' '{"kind":"opens","from":"ui:menu:designer","to":"ui:designer"}').StatusCode
    $r = & $post '/edges' '{"kind":"opens","from":"ui:menu:designer","to":"ui:layout:root"}'
    "$($r.StatusCode) $((($r.Content | ConvertFrom-Json).problems.code) -join ',')"
    $null = New-PortalSnap -Url $p.Url -Note 'menu'
    Stop-Portal -Artifacts $dir
    $p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
    (Get-PortalScreen -Url $p.Url).edges | Where-Object kind -In 'is-a', 'opens' | Where-Object from -Like 'ui:menu:*' | ForEach-Object id
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `201`, `201`, `201`, `422 edge-endpoints`, then the six menu edges `ui:menu:graph|ui:category:Menu|is-a`, `ui:menu:graph|ui:canvas|opens`, `ui:menu:runs|ui:category:Menu|is-a`, `ui:menu:runs|ui:canvas:agent-runs|opens`, `ui:menu:designer|ui:category:Menu|is-a`, `ui:menu:designer|ui:designer|opens`.

### 1.7 A layer mounted by path is a Mount node, and an approval brings it back after a restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.7'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $l = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\filesystem -Url $p.Url
    "mounted: $($l.name) $($l.counts.nodes) nodes"
    $m = (Get-PortalScreen -Url $p.Url).nodes | Where-Object id -EQ 'ui:mount:agent-runs'
    "recorded: $($m.kind) $($m.properties.layer) $($m.properties.path)"
    $null = New-PortalSnap -Url $p.Url -Note 'agent-runs'
}
finally { Stop-Portal -Artifacts $dir }
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try { "after restart: $((Invoke-RestMethod "$($p.Url)/api/tenants/default/layers").name -join ',')" }
finally { Stop-Portal -Artifacts $dir }
```
Expect: `mounted: agent-runs 52 nodes`, `recorded: Mount agent-runs C:\__Code\Portal\tests\fixtures\layers\filesystem` (the directory as given, made absolute), then `after restart: agent-runs`, with no flag mounting anything.

### 1.8 No token reads, a write needs one, a bad token is refused
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-1.8'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $api = "$($p.Url)/api/tenants/default"
    $call = { param($method, $path, $headers) $r = Invoke-WebRequest -Method $method -Uri "$api$path" -Headers $headers -SkipHttpErrorCheck; "$method $path $($r.StatusCode) $(if ($r.StatusCode -ge 400) { ($r.Content | ConvertFrom-Json).code })" }
    & $call GET '/screen' @{}
    & $call POST '/snap' @{}
    & $call GET '/screen' @{ Authorization = 'Bearer wrong' }
    & $call POST '/snap' $auth
    "anonymous: $((Invoke-RestMethod "$api/auth/whoami") | ConvertTo-Json -Compress)"
    "token: $((Invoke-RestMethod "$api/auth/whoami" -Headers $auth) | ConvertTo-Json -Compress)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `GET /screen 200`, `POST /snap 401 unauthorized`, `GET /screen 401 unauthorized`, `POST /snap 201`, then `anonymous: {"name":"anonymous","anonymous":true,"writes":false,"tenant":"default","marker":null}` and `token: {"name":"check","anonymous":false,"writes":true,"tenant":"default","marker":null}`.

### 1.9 Config, menu and routes are the tenant's
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
$dir = Join-Path $env:TEMP 'portal-check-1.9'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    $api = "$($p.Url)/api/tenants/default"
    "product: $((Invoke-RestMethod "$api/config").product.name)"
    $m = Invoke-RestMethod "$api/menu"
    "menu: $(($m.nodes | Group-Object kind | Sort-Object Name | ForEach-Object { "$($_.Name) $($_.Count)" }) -join ', ')"
    "services: $((@($m.nodes | Where-Object kind -EQ 'Service').name) -join ', ')"
    "workflows/all tabs: $((@($m.edges | Where-Object from -EQ 'ui:rail:workflows:all' | ForEach-Object to)) -join ', ')"
    "categories: $(@((Invoke-RestMethod "$api/nodes?kind=Category").nodes).Count); directories: $(@((Invoke-RestMethod "$api/nodes?kind=Directory").nodes).Count)"
    $r = Invoke-WebRequest "$($p.Url)/" -MaximumRedirection 0 -SkipHttpErrorCheck -ErrorAction SilentlyContinue
    "/: $($r.StatusCode) $($r.Headers['Location'])"
    foreach ($path in '/default/settings/service-connections', '/default/workflows/all/wf-1', '/default/nope/x', '/acme/overview/summary', '/api/tenants/acme/screen') {
        "${path}: $((Invoke-WebRequest "$($p.Url)$path" -SkipHttpErrorCheck).StatusCode)"
    }
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `product: Portal`, `menu: Page 39, Service 13, State 13`, `services: Overview, Graph, Emitters, Designer, Workflows, Environments, Compliance, Policy, Ledger, Identity, Artifacts, Stack, Settings`, `workflows/all tabs: ui:rail:workflows:all:plan, ui:rail:workflows:all:runs, ui:rail:workflows:all:history`, `categories: 5; directories: 16`, `/: 302 /default/overview/summary`, then `200`, `200`, `404`, `404`, `404`.

### 1.10 Service connections list no secret; Test reports the error
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
Set-Content "$env:PORTAL_TENANTS\default\connections.yaml" "connections:`n  - { name: github, type: github, token_env: PORTAL_CHECK_GH }`n"
$dir = Join-Path $env:TEMP 'portal-check-1.10'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$env:PORTAL_CHECK_GH = $null
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $api = "$($p.Url)/api/tenants/default"
    "list: $((Invoke-RestMethod "$api/connections") | ConvertTo-Json -Compress)"
    $r = Invoke-WebRequest "$api/connections/github/repos" -SkipHttpErrorCheck
    "anonymous repos: $($r.StatusCode) $(($r.Content | ConvertFrom-Json).code)"
    $r = Invoke-WebRequest "$api/connections/github/repos" -Headers $auth -SkipHttpErrorCheck
    "repos: $($r.StatusCode) $(($r.Content | ConvertFrom-Json).message)"
    "nope: $((Invoke-WebRequest "$api/connections/nope/repos" -Headers $auth -SkipHttpErrorCheck).StatusCode)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir, "$env:PORTAL_TENANTS\default\connections.yaml" -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `list: {"name":"github","tokenSet":false,"token_env":"PORTAL_CHECK_GH","type":"github","url":"https://api.github.com"}` (one connection, so one object; the url is GitHub's default), `anonymous repos: 401 unauthorized` (the call spends the tenant's GitHub token, so it needs a bearer token though it is a GET), `repos: 422 token not set: connection github reads its token from PORTAL_CHECK_GH, which is not set`, `nope: 404`. With `PORTAL_CHECK_GH` set to a GitHub token before Start-Portal, `repos:` is `200` and the envelope holds one `git.repository` node per repository; a token GitHub refuses is `502` with GitHub's message (Go's `TestGitHubReposEndpoint` covers both against a fake GitHub).

### 1.11 A tenant overlay that fails the schema stops the binary
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.11-tenants'
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" 'theme: dark'
try { Start-Portal -Port 18200 -Artifacts (Join-Path $env:TEMP 'portal-check-1.11') -Filesystem '' -Tenants $t -TimeoutSec 5 } catch { "refused: $($_.Exception.Message)" }
finally { Remove-Item $t, (Join-Path $env:TEMP 'portal-check-1.11') -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `refused: The portal did not start on port 18200 within 5 s. ... portal: <path>\default\portal.yaml: /: additional properties 'theme' not allowed`.

## 2. Page

### 2.1 The shell, and the FileSystem layer in colour on Graph/Explore
```powershell
# needs C:\__Code\Graph.Emitter.FileSystem\artifacts\layers (falls back to the fixture); mounts C:\__Code\Claude.Agent\artifacts\layers as agent-runs when present
Invoke-Build Screenshot
```
Expect: `captured ...\docs\screenshots\summary.png`, `...service-connections.png`, `...explore.png`, then `canvas coloured from ontology: <n>/<n>`. Every PNG is dark: a near-black grid behind cards. The banner holds `Portal`, the breadcrumb (`default / Overview / Summary` and so on), a search box, `marker: none`, `Help` and the `anonymous` avatar. The rail lists Overview to Stack with icons, Settings at the foot above `Collapse`, and the open service's pages in place, the current one marked with the accent. summary.png shows the empty state `No summary yet`; service-connections.png one row, `github`, `https://api.github.com`, a red dot and `GITHUB_TOKEN`, and a `Test` button; explore.png the toolbar `filesystem · <n> nodes · <m> edges · <stamp>`, the badge `screen 20 · 19 · <stamp>` and `read-only: sign in to write`, above the filesystem Canvas filling the pane (with the sibling's layer Directories are blue rounded squares labelled `name/`, Files grey circles; with the fixture, Directories amber, Files blue).

### 2.2 Approve from the Designer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
"token: $env:PORTAL_TOKEN"
$dir = Join-Path $env:TEMP 'portal-check-2.2'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/default/designer/export"
Read-Host 'Give the avatar the token check (it is in $env:PORTAL_TOKEN), then type a note in Approve and press approve; then in a second shell PATCH ui:theme:default opacity to 0.3, reload, approve again; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: the avatar turns to `check` with the accent; the page shows only the Approve section; the first approve shows `approved <stamp> · <note>` and `first snap: nothing to compare`; after the PATCH and a reload the panels are more opaque, and the second shows `against <first stamp>:` and `changed 1 nodes, 0 edges: ui:theme:default`.

### 2.3 Add Category and Add Menu Entry in the Designer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
"token: $env:PORTAL_TOKEN"
$dir = Join-Path $env:TEMP 'portal-check-2.3'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/default/designer/sections"
Read-Host 'Give the avatar the printed token; expand Add Category and add Tree refining Canvas; expand Add Menu Entry and add Designer opening designer; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: after Add Category the Categories list shows `└ Tree (refines Canvas)` under Canvas and the category and refines dropdowns offer `· Tree`; Add Menu Entry answers with no error and the entry is a screen node (`GET /api/tenants/default/screen` holds `ui:menu:designer`), though since 0.4.0 the shell's rail is the tenant's menu and does not draw it (docs/design.md 36); neither add reloads the page. Without the token, either add shows `unauthorized`.

### 2.4 Designer sections collapse, and stay as left across Approve and a restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
"token: $env:PORTAL_TOKEN"
$dir = Join-Path $env:TEMP 'portal-check-2.4'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/default/designer/sections"
Read-Host 'Give the avatar the printed token; press expand on Add Panel and collapse on Categories, then Approve; press Enter here to restart'
Stop-Portal -Artifacts $dir
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    (Get-PortalScreen -Url $p.Url).nodes | Where-Object id -In 'ui:state:designer:add-panel', 'ui:state:designer:categories' | ForEach-Object { "$($_.id) collapsed $($_.properties.collapsed)" }
    Start-Process "$($p.Url)/default/designer/sections"
    Read-Host 'Reload the page; press Enter here to stop'
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the seeded page shows Approve and Categories open and the other five collapsed, every heading and button in Title Case and the field labels lower case; each toggle hides or shows only its section's body; Approve's result is `first snap: nothing to compare`; after the restart the block prints `ui:state:designer:add-panel collapsed False` and `ui:state:designer:categories collapsed True`, and the reloaded page shows Add Panel open and Categories collapsed.

### 2.5 The shell: rail, flyout, collapse, deep links, sign-in, Test
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Copy-Item .\tenants\default\connections.example.yaml "$env:PORTAL_TENANTS\default\connections.yaml" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
"token: $((New-PortalToken -Name check).Token)"
$dir = Join-Path $env:TEMP 'portal-check-2.5'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/"
Read-Host 'Click Graph, then Workflows; hover Policy; press Collapse and hover Ledger; Expand; open Settings > Service connections, press F5, press Test; give the avatar the printed token and press Test again; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir, "$env:PORTAL_TENANTS\default\connections.yaml" -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: the page opens at `/default/overview/summary` with Overview open in the rail. Clicking Graph opens its pages in place and closes Overview's, and Workflows does the same to Graph. Hovering a closed service shows its pages in a flyout beside it. Collapse leaves icons only, and a hover still shows the flyout. After F5 on Service connections the same page, breadcrumb and open Settings come back. Test before signing in shows `unauthorized: ...needs a bearer token` in red. After the avatar takes the token it reads `check` with accent initials, and Test then shows `no_token: ...GITHUB_TOKEN, which is not set` (a count of repositories when `GITHUB_TOKEN` is set). A wrong token is dropped with `unauthorized` in the avatar panel.

## 3. PowerShell

### 3.1 Approve, PATCH a Theme, approve, diff, restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-3.1'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    $a = New-PortalSnap -Url $p.Url -Note 'first'
    Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/tenants/default/nodes/ui:theme:default" -Headers $auth -ContentType application/json -Body '{"properties":{"opacity":0.2}}' | Out-Null
    $b = New-PortalSnap -Url $p.Url -Note 'lighter'
    "approval: $($b.note); changed $($b.diff.changed.nodes)"
    $d = Compare-PortalSnap $a $b -Url $p.Url
    "diff: changed $($d.changed.nodes.id); added $(@($d.added.nodes).Count); removed $(@($d.removed.nodes).Count)"
    "same: $(@((Compare-PortalSnap $a $a -Url $p.Url).changed.nodes).Count)"
    & .\build\win-x64\graphnode.exe validate $b.path | Out-Null; "exit $LASTEXITCODE"
    Stop-Portal -Artifacts $dir
    $p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
    "restart: boot is second $($p.Boot -eq $b.stamp); opacity $(((Get-PortalScreen -Url $p.Url).nodes | Where-Object id -EQ 'ui:theme:default').properties.opacity)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `approval: lighter; changed ui:theme:default`, `diff: changed ui:theme:default; added 0; removed 0`, `same: 0`, `exit 0`, `restart: boot is second True; opacity 0.2`.

### 3.2 Start-Portal -Snap boots an older snap
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-3.2'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
$a = New-PortalSnap -Url $p.Url
$null = New-PortalSnap -Url $p.Url
Stop-Portal -Artifacts $dir
try {
    $p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' -Snap $a.stamp
    "boot is first: $($p.Boot -eq $a.stamp)"
    Stop-Portal -Artifacts $dir
    try { Start-Portal -Port 18200 -Artifacts "$dir-x" -Filesystem '' -Snap 2000-01-01T00-00-00.000Z -TimeoutSec 5 } catch { "refused: $($_.Exception.Message -match 'no snap 2000-01-01T00-00-00.000Z')" }
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir, "$dir-x" -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `boot is first: True`, `refused: True`.

### 3.3 Fixed port: 0 refused, a busy port named, Status
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-3.3'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
try { Start-Portal -Port 0 -Artifacts $dir -Filesystem '' } catch { "zero: $($_.Exception.Message -match '0 is never picked')" }
$l = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 18200); $l.Start()
try { Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' } catch { "busy: $($_.Exception.Message)" } finally { $l.Stop() }
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try { $s = Get-PortalStatus -Artifacts $dir; "status: $($s.Running) $($s.Port) $($s.Url) $($s.Boot)" }
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `zero: True`, `busy: port 18200 is in use by pwsh (process <pid>); free it, or set PORTAL_PORT in .env or pass -Port`, `status: True 18200 http://127.0.0.1:18200 seed`.

### 3.4 Up replaces a running portal; -NoReplace throws
Up itself runs on 8200 over `artifacts/`; this drives the same private `Invoke-PortalUp` on the test port.
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
$dir = Join-Path $env:TEMP 'portal-check-3.4'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$m = Get-Module Portal
try {
    $a = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
    $snap = New-PortalSnap -Url $a.Url -Note 'check 3.4'
    $b = & $m { param($d) Invoke-PortalUp -Port 18200 -Artifacts $d -Filesystem '' } $dir
    "replaced $($b.Replaced) with $($b.Boot); new process: $($b.ProcessId -ne $a.ProcessId); old gone: $(-not (Get-Process -Id $a.ProcessId -ErrorAction SilentlyContinue))"
    try { & $m { param($d) Invoke-PortalUp -Port 18200 -Artifacts $d -Filesystem '' -NoReplace } $dir } catch { "noreplace: $($_.Exception.Message)" }
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `replaced seed with <stamp>; new process: True; old gone: True` (`<stamp>` the snap just taken), then `noreplace: A portal is already running at http://127.0.0.1:18200 (process <pid>). Stop it with Stop-Portal first.` The 10 s timeout path is Pester's (a held port, 1 s bound).

### 3.5 New-PortalToken: the hash on disk, the token once
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-3.5-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" '{}'
try {
    $k = New-PortalToken -Name jerry -Tenants $t
    "token: $($k.Token.Length) hex chars; on disk: $((Get-Content $k.Path -Raw).Contains($k.Token))"
    Get-Content $k.Path | Select-Object -Skip 1
    try { New-PortalToken -Name jerry -Tenants $t } catch { "again: $($_.Exception.Message)" }
}
finally { Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `token: 64 hex chars; on disk: False`, then `tokens:`, `  - name: 'jerry'`, `    sha256: <64 hex>`, then `again: tokens.yaml already holds a token named jerry (<path>). Remove its entry to replace it.`
