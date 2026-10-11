# Portal manual check list

Verified for: 0.13.0 on 2026-10-11, Windows 11 (10.0.26200), PowerShell 7.6.6, Go 1.27.0, Pester 6.2.0, InvokeBuild 5.14.23: item 0.2 (Go 107 tests, Smoke passed with ratio 0.844, Pester 118 passed, `Build succeeded. 6 tasks, 0 errors, 0 warnings`); item 1.19 run as written from its own `pwsh -NoProfile -File`, its Expect the output seen; 2.12's catalogue, filtered list, emitter page and two layer pages seen by --dump-dom from its own setup (its colours not seen by eye), and checked by Pester's Emitters block in headless Chrome. Every other item as verified for 0.12.1: item 0.2 (Go 106 tests, Smoke passed with ratio 0.844, Pester 116 passed, `Build succeeded. 6 tasks, 0 errors, 0 warnings`); items 1.21 and 1.22 run as written from their own `pwsh -NoProfile -File`, their Expect the output seen; 2.15's list, filtered list and item page seen by --dump-dom over the real alert layer on a throwaway demo-mode tenant, its menu as Pester's orphan alert checks it; 2.14 checked by Pester's Environments graph block in headless Chrome (counts, hulls, chips, selection, menus); nobody has clicked through 2.14 or 2.15 by hand. Other items were last verified as their own entries say, for 0.12.0 and earlier.

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
Expect: go test ends `ok` for `cmd/portal`, `internal/feed`, `internal/server`, `internal/store` and `internal/tenant` (107 tests); Smoke prints `smoke: seed 20 nodes; approved <a>, then <b> changing ui:theme:default; restart booted from <b>; canvas fit {"graphWidth":<g>,"panelWidth":<p>,"graphHeight":<h>,"panelHeight":<q>,"ratio":<r>}` with `<r>` above 0.5 (0.844 here), and `artifacts/smoke-report.json` has `checks.auth` `{"anonymousWrite":401,"principal":"smoke","role":"admin"}` (the smoke tenant's bootstrap admin) and `checks.rail.rail` the thirteen Services from `ui:rail:overview` to `ui:rail:settings`; Pester ends `Tests Passed: 118, Failed: 0, Skipped: 0`, then `Build succeeded. 6 tasks, 0 errors, 0 warnings`. While `tenants/default/portal.yaml` is staged or committed with mode dev, Pester ends `Tests Passed: 117, Failed: 1, Skipped: 0` instead, the failure being `tenants/default/portal.yaml is neither committed nor staged with mode dev`, and the build stops with `Pester failed: 1 test(s)` (docs/design.md 39).

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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Expect: `GET /screen 200`, `POST /snap 401 unauthorized`, `GET /screen 401 unauthorized`, `POST /snap 201`, then `anonymous: {"id":null,"name":"anonymous","type":null,"role":null,"anonymous":true,"legacy":false,"writes":false,"administers":false,"tenant":"default","marker":null}` and `token: {"id":"ui:principal:check","name":"check","type":"user","role":"admin","anonymous":false,"legacy":false,"writes":true,"administers":true,"tenant":"default","marker":null}` (since 0.5.0 the token belongs to the bootstrap admin, docs/design.md 38).

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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Expect: `list: {"name":"github","tokenSet":false,"token_env":"PORTAL_CHECK_GH","type":"github","url":"https://api.github.com","createdBy":null}` (one connection, so one object; the url is GitHub's default; `createdBy` null as the entry has no `created_by`), `anonymous repos: 401 unauthorized` (the call spends the tenant's GitHub token, so it needs a bearer token though it is a GET), `repos: 422 token not set: connection github reads its token from PORTAL_CHECK_GH, which is not set`, `nope: 404`. With `PORTAL_CHECK_GH` set to a GitHub token before Start-Portal, `repos:` is `200` and the envelope holds one `git.repository` node per repository; a token GitHub refuses is `502` with GitHub's message (Go's `TestGitHubReposEndpoint` covers both against a fake GitHub).

### 1.11 A tenant overlay that fails the schema stops the binary
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.11-tenants'
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" 'colours: dark'
try { Start-Portal -Port 18200 -Artifacts (Join-Path $env:TEMP 'portal-check-1.11') -Filesystem '' -Tenants $t -TimeoutSec 5 } catch { "refused: $($_.Exception.Message)" }
Set-Content "$t\default\portal.yaml" 'theme: { graph: { wheelZoomSpeed: 10 } }'
try { Start-Portal -Port 18200 -Artifacts (Join-Path $env:TEMP 'portal-check-1.11') -Filesystem '' -Tenants $t -TimeoutSec 5 } catch { "refused: $($_.Exception.Message)" }
finally { Remove-Item $t, (Join-Path $env:TEMP 'portal-check-1.11') -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `refused: The portal did not start on port 18200 within 5 s. ... portal: <path>\default\portal.yaml: /: additional properties 'colours' not allowed`, then the same with `/theme/graph/wheelZoomSpeed: got 10, want 5` (0.4.1: `theme` is a known key, held to 0.1..5, docs/design.md 37).

### 1.12 The config's wheel zoom speed, base and overlay; an anonymous collapse is 401
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.12-tenants'
$dir = Join-Path $env:TEMP 'portal-check-1.12'
$null = New-Item -ItemType Directory -Force "$t\default"; Copy-Item .\tenants\default\portal.yaml "$t\default" -Force
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem -Tenants $t
try {
    "base: $((Invoke-RestMethod "$($p.Url)/api/tenants/default/config").theme.graph.wheelZoomSpeed)"
    $r = Invoke-WebRequest "$($p.Url)/api/tenants/default/nodes/ui:canvas" -Method Patch -ContentType 'application/json' -Body '{"state":{"collapsed":true}}' -SkipHttpErrorCheck
    "anonymous collapse: $([int]$r.StatusCode) $(($r.Content | ConvertFrom-Json).code)"
}
finally { Stop-Portal -Artifacts $dir }
Set-Content "$t\default\portal.yaml" 'theme: { graph: { wheelZoomSpeed: 2.5 } }'
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem -Tenants $t
try { "overlay: $((Invoke-RestMethod "$($p.Url)/api/tenants/default/config").theme.graph.wheelZoomSpeed)" }
finally { Stop-Portal -Artifacts $dir; Remove-Item $t, $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `base: 1`, `anonymous collapse: 401 unauthorized`, `overlay: 2.5` (docs/design.md 37).

### 1.13 Roles: reader GET only, contributor no identity, admin all; attenuation; revoke
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$admin = @{ Authorization = "Bearer $((New-PortalToken -Name check -Bootstrap).Token)" }
$dir = Join-Path $env:TEMP 'portal-check-1.13'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $api = "$($p.Url)/api/tenants/default"
    $add = { param($name, $role, $h) $r = Invoke-WebRequest -Method Post "$api/principals" -Headers $h -ContentType application/json -Body (@{ name = $name; role = $role } | ConvertTo-Json) -SkipHttpErrorCheck; "add $name $role -> $($r.StatusCode)$(if ($r.StatusCode -ge 400) { ' ' + ($r.Content | ConvertFrom-Json).message })" }
    & $add rea reader $admin
    & $add con contributor $admin
    $reader = @{ Authorization = "Bearer $((New-PortalToken -Name rea).Token)" }
    $contrib = @{ Authorization = "Bearer $((New-PortalToken -Name con).Token)" }
    # Tokens minted after boot go live at the portal's next Add or Revoke, which re-read tokens.yaml.
    & $add ops reader $admin
    & $add nobody reader @{}
    foreach ($w in @(@{ n = 'reader'; h = $reader }, @{ n = 'contributor'; h = $contrib }, @{ n = 'admin'; h = $admin })) {
        $snap = Invoke-WebRequest -Method Post "$api/snap" -Headers $w.h -SkipHttpErrorCheck
        "$($w.n): GET $((Invoke-WebRequest "$api/screen" -Headers $w.h -SkipHttpErrorCheck).StatusCode), POST /snap $($snap.StatusCode)"
    }
    & $add boss admin $contrib
    & $add peer reader $contrib
    & $add peer reader $reader
    (Invoke-RestMethod "$api/principals").nodes | ForEach-Object { "principal $($_.name) $($_.properties.role) by $($_.properties.createdBy)" }
    $con = (Invoke-RestMethod "$api/tokens") | Where-Object name -EQ 'con'
    "revoke by contributor: $((Invoke-WebRequest -Method Delete "$api/tokens/$($con.id)" -Headers $contrib -SkipHttpErrorCheck).StatusCode)"
    "revoke by admin: $((Invoke-RestMethod -Method Delete "$api/tokens/$($con.id)" -Headers $admin) | ConvertTo-Json -Compress)"
    "contributor after: $((Invoke-WebRequest "$api/screen" -Headers $contrib -SkipHttpErrorCheck).StatusCode)"
    (Invoke-RestMethod "$api/tokens") | ForEach-Object { "token $($_.name) $($_.id)" }
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `add rea reader -> 201`, `add con contributor -> 201`, `add ops reader -> 201`, `add nobody reader -> 401 POST writes, and a write needs a bearer token; without one the portal reads only`, then `reader: GET 200, POST /snap 403`, `contributor: GET 200, POST /snap 201`, `admin: GET 200, POST /snap 201`, then `add boss admin -> 403 forbidden: attenuation: contributor con may create principals of role reader or contributor only, not admin`, `add peer reader -> 403 forbidden: creating a principal is identity, which needs role admin; con is contributor`, `add peer reader -> 403 POST writes, and rea's role reader reads only`, the four principals (`check admin by` nothing, then `rea`, `con`, `ops` each `by ui:principal:check`), `revoke by contributor: 403`, `revoke by admin: {"id":"<12 hex>","principal":"ui:principal:con","name":"con","created":"<time>","legacy":false}`, `contributor after: 401`, and the two tokens left, `check` and `rea` (docs/design.md 38).

### 1.14 A token from before 0.5.0 acts as admin and is named at boot
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes('old-token'))).ToLowerInvariant()
Set-Content "$env:PORTAL_TENANTS\default\tokens.yaml" "tokens:`n  - name: 'old-ci'`n    sha256: $hash`n"
$old = @{ Authorization = 'Bearer old-token' }
$dir = Join-Path $env:TEMP 'portal-check-1.14'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $api = "$($p.Url)/api/tenants/default"
    Get-Content "$($p.LogPath).err"
    "whoami: $((Invoke-RestMethod "$api/auth/whoami" -Headers $old) | ConvertTo-Json -Compress)"
    "snap: $((Invoke-WebRequest -Method Post "$api/snap" -Headers $old -SkipHttpErrorCheck).StatusCode)"
    $r = Invoke-WebRequest -Method Post "$api/principals" -Headers $old -ContentType application/json -Body '{"name":"x","role":"reader"}' -SkipHttpErrorCheck
    "add: $($r.StatusCode) $(($r.Content | ConvertFrom-Json).message)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: while `tenants/default/portal.yaml` is mode dev, first `portal: warning: tenant default is in mode dev: auto-minted admin, local sign-in shortcut; not for demo or production` (docs/design.md 39); then `portal: warning: tokens.yaml entries without a principal act as admin until rotated: old-ci`, then `whoami: {"id":null,"name":"old-ci","type":null,"role":"admin","anonymous":false,"legacy":true,"writes":true,"administers":true,"tenant":"default","marker":null}`, `snap: 201`, and `add: 403 forbidden: old-ci's token belongs to no principal (from before 0.5.0), so it cannot be recorded as createdBy; bootstrap a principal with New-PortalToken -Bootstrap` (docs/design.md 38).

### 1.15 Tenant mode; the dev token only in mode dev, and dev-bootstrap refused outside it
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.15-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" 'mode: dev'
$null = New-PortalToken -Name check -Bootstrap -Tenants $t
$dev = & (Get-Module Portal) { param($t) New-PortalDevToken -Tenants $t } $t
$dir = Join-Path $env:TEMP 'portal-check-1.15'
$saved = $env:PORTAL_TOKEN
try {
    foreach ($mode in 'dev', 'demo') {
        Set-Content "$t\default\portal.yaml" "mode: $mode"
        Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
        $env:PORTAL_TOKEN = $dev.Token
        $p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' -Tenants $t
        $env:PORTAL_TOKEN = $saved
        try {
            $api = "$($p.Url)/api/tenants/default"
            "${mode}: tenant $((Invoke-RestMethod $api) | ConvertTo-Json -Compress)"
            Get-Content "$($p.LogPath).err"
            $r = Invoke-WebRequest "$api/dev-token" -SkipHttpErrorCheck
            "${mode}: dev-token $($r.StatusCode) $(if ($r.StatusCode -eq 200) { 'same token: ' + (($r.Content | ConvertFrom-Json).token -eq $dev.Token) } else { ($r.Content | ConvertFrom-Json).message })"
            $w = Invoke-WebRequest "$api/auth/whoami" -Headers @{ Authorization = "Bearer $($dev.Token)" } -SkipHttpErrorCheck
            "${mode}: whoami with the dev token $($w.StatusCode) $(if ($w.StatusCode -eq 200) { ($w.Content | ConvertFrom-Json).role } else { ($w.Content | ConvertFrom-Json).message })"
        }
        finally { Stop-Portal -Artifacts $dir; & (Get-Module Portal) { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }
    }
}
finally { $env:PORTAL_TOKEN = $saved; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `dev: tenant {"mode":"dev","name":"default"}`, `portal: warning: tenant default is in mode dev: auto-minted admin, local sign-in shortcut; not for demo or production`, `dev: dev-token 200 same token: True`, `dev: whoami with the dev token 200 admin`; then `demo: tenant {"mode":"demo","name":"default"}`, `portal: warning: tenant default is in mode demo and tokens.yaml still holds 1 token(s) of dev-bootstrap, refused until revoked or mode dev`, `demo: dev-token 404 no dev token: it exists only in mode dev, from 127.0.0.1, on a portal started by Invoke-Build Dev` and `demo: whoami with the dev token 401 the bearer token belongs to dev-bootstrap, whose tokens work only while tenant default's mode is dev; it is demo` (docs/design.md 39).

### 1.16 Feeds as a graph; a push needs a writer and a feed the tenant has, and writes one ledger line
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.16-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" "feeds:`n  - { id: cards-oci, url: `"http://127.0.0.1:1/cards`" }"
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-1.16'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' -Tenants $t
$saved = $env:PORTAL_TOKEN
try {
    $env:PORTAL_TOKEN = $admin.Token
    $feeds = Invoke-RestMethod "$($p.Url)/api/tenants/default/feeds"
    "feeds: $(@($feeds.nodes | ForEach-Object { "$($_.id) $($_.properties.kind) $($_.properties.url)" }) -join '; ')"
    foreach ($case in @(
            @{ Name = 'anonymous'; Token = ''; Feed = 'cards-oci'; Card = '.\tests\fixtures\cards\Tool.Git' }
            @{ Name = 'admin, feed down'; Token = $admin.Token; Feed = 'cards-oci'; Card = '.\tests\fixtures\cards\Tool.Git' }
            @{ Name = 'admin, no such feed'; Token = $admin.Token; Feed = 'nope'; Card = '.\tests\fixtures\cards\Tool.Git' })) {
        $env:PORTAL_TOKEN = $case.Token
        try { $null = Publish-PortalCard -Feed $case.Feed -Path $case.Card -Url $p.Url; "$($case.Name): pushed" }
        catch { "$($case.Name): $(($_.Exception.Message -split "`n")[0])" }
    }
    Get-Content "$t\default\ledger.jsonl" | ForEach-Object { $e = $_ | ConvertFrom-Json; "ledger: $($e.result) $($e.feed) $($e.card) $($e.version) files $($e.files) by $($e.principal)" }
}
finally { $env:PORTAL_TOKEN = $saved; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `feeds: ui:feed:cards-oci oci http://127.0.0.1:1/cards; ui:feed:cards-nuget nuget http://127.0.0.1:8202/v3/index.json` (the overlay's url over the base's, the other feed as the base has it); `anonymous: ... -> 401: POST writes, and a write needs a bearer token; ...`; `admin, feed down: ... -> 502: feed cards-oci: Head "http://127.0.0.1:1/v2/cards/tool.git/manifests/0.1.0": dial tcp ...`; `admin, no such feed: ... -> 404: no such feed: nope; tenant default has cards-oci, cards-nuget`; and one line, `ledger: failed cards-oci Tool.Git 0.1.0 files 2 by ui:principal:check` (the 401 and the 404 write none; docs/design.md 40).

### 1.17 A mount answers Location at the layer's page; POST /api/layers is 410
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.17-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-1.17'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' -Tenants $t
try {
    $auth = @{ Authorization = "Bearer $($admin.Token)" }
    $body = @{ name = 'fs'; path = (Resolve-Path .\tests\fixtures\layers\filesystem).Path } | ConvertTo-Json
    $r = Invoke-WebRequest "$($p.Url)/api/tenants/default/layers" -Method POST -ContentType 'application/json' -Headers $auth -Body $body
    "mount: $([int]$r.StatusCode) Location $($r.Headers['Location'])"
    $page = Invoke-WebRequest "$($p.Url)$($r.Headers['Location'])" -SkipHttpErrorCheck
    "page: $([int]$page.StatusCode) $($page.Headers['Content-Type'])"
    "not mounted: $([int](Invoke-WebRequest "$($p.Url)/default/layers/nope" -SkipHttpErrorCheck).StatusCode)"
    $old = Invoke-WebRequest "$($p.Url)/api/layers" -Method POST -ContentType 'application/json' -Headers $auth -Body ($body -replace '"fs"', '"old"') -SkipHttpErrorCheck
    "old route: $([int]$old.StatusCode) $($old.Content.Trim())"
    "layers: $((Invoke-RestMethod "$($p.Url)/api/tenants/default/layers").name -join ',')"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `mount: 201 Location /default/layers/fs`; `page: 200 text/html; charset=utf-8`; `not mounted: 404`; `old route: 410 {"code":"gone","message":"POST /api/layers is gone; mount with POST /api/tenants/default/layers"}`; `layers: fs` (the old route mounted nothing; docs/design.md 42).

### 1.18 Agents: runs grouped by the principal that mounted them; a flag mount is dev-bootstrap's
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.18-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-1.18'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$runs = (Resolve-Path .\tests\fixtures\layers\agent-runs).Path
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' -Mount @{ 'flag-runs' = $runs } -Tenants $t
try {
    $u = "$($p.Url)/api/tenants/default"
    $show = { param($env) ($env.nodes | Where-Object kind -EQ 'Principal').name -join ',' }
    "at start: $(& $show (Invoke-RestMethod "$u/agents"))"
    $env:PORTAL_TOKEN = $admin.Token
    $l = Mount-PortalLayer -Name agent-runs -Path $runs -Url $p.Url
    "mounted by: $($l.mountedBy)"
    $a = Invoke-RestMethod "$u/agents"
    "after the mount: $(& $show $a)"
    $run = $a.nodes | Where-Object kind -EQ 'Run'
    "run: $($run.id) layer $($run.properties.layer) steps $($run.properties.steps) allow $($run.properties.allow) deny $($run.properties.deny) witness $($run.properties.witness)"
    $r = Invoke-RestMethod "$u/agents/check/runs"
    "decisions: $((@($r.nodes | Where-Object kind -EQ 'Decision') | ForEach-Object { "$($_.id -replace '^.*/(step-\d+/\w+)$', '$1') $($_.properties.effect)" }) -join '; ')"
    "no agent: $([int](Invoke-WebRequest "$u/agents/nobody/runs" -SkipHttpErrorCheck).StatusCode)"
    "pages: $([int](Invoke-WebRequest "$($p.Url)/default/agents" -SkipHttpErrorCheck).StatusCode) $([int](Invoke-WebRequest "$($p.Url)/default/agents/check" -SkipHttpErrorCheck).StatusCode) $([int](Invoke-WebRequest "$($p.Url)/default/agents/nobody" -SkipHttpErrorCheck).StatusCode)"
    "Mount node: $((Get-PortalScreen -Url $p.Url).nodes | Where-Object id -EQ 'ui:mount:agent-runs' | ForEach-Object { $_.properties.mountedBy })"
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `at start: dev-bootstrap` (a `-Mount` flag names no principal, and this tenant's principals.yaml lacks dev-bootstrap, so it is served with no `created`); `mounted by: ui:principal:check`; `after the mount: check`, because both layers hold the same Run id and the later mount's copy is the one served; `run: agent:run/2026-10-09T02:50:43.7976027Z layer agent-runs steps 3 allow 4 deny 1 witness 0`; `decisions: step-1/pre allow; step-1/post allow; step-2/pre allow; step-2/post allow; step-3/pre deny`; `no agent: 404`; `pages: 200 200 404`; `Mount node: ui:principal:check` (docs/design.md 43).

### 1.19 Emitters match by source block: ok, version-mismatch, no-card, unstamped; the card on the emitter; 404 for a module
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.19-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
# Two pushes as 0.13.0 writes them: the card's module, repo, url and emits on the line.
$line = '{"at":"2026-10-11T00:00:0{0}.000Z","principal":"ui:principal:check","name":"check","role":"admin","feed":"cards-oci","kind":"oci","card":"{1}","version":"{2}","type":"emitter","hash":"sha256:{3}","digest":null,"files":1,"result":"pushed","message":"{1} {2} to cards-oci","module":"{4}","repo":"{5}","url":"https://github.com/JerryBalmer1/{5}","emits":[{6}]}'
Set-Content "$t\default\ledger.jsonl" @(
    $line.Replace('{0}', '0').Replace('{1}', 'Emitter.Agent').Replace('{2}', '0.1.4').Replace('{3}', 'ab' * 32).Replace('{4}', 'Graph.Emitter.Agent').Replace('{5}', 'Portal.Emitters').Replace('{6}', '{"facet":"runs","kinds":["Run","Step","Action","PolicyDecision"]}')
    $line.Replace('{0}', '1').Replace('{1}', 'Emitter.Environment').Replace('{2}', '0.3.0').Replace('{3}', 'cd' * 32).Replace('{4}', 'Claude.Agent.Orchestrator').Replace('{5}', 'Portal.Agent').Replace('{6}', '{"facet":"alerts","kinds":["alert"]}'))
# Copies of real fixture layers with a source block inserted (no layer on disk carries one yet).
function Copy-Stamped([string] $Dir, [string] $Emitter, [string] $Version) {
    $to = Join-Path $t "stamped\$Dir"
    $null = New-Item -ItemType Directory -Force $to
    $f = Get-ChildItem ".\tests\fixtures\layers\$Dir" -Filter '*.graph.json'
    $text = Get-Content $f.FullName -Raw
    $stamp = [regex]::Match($text, '"layer"\s*:\s*"([^"]+)"').Groups[1].Value
    $i = $text.IndexOf('{')
    Set-Content (Join-Path $to $f.Name) -NoNewline ($text.Substring(0, $i + 1) + "`"source`":{`"emitter`":`"$Emitter`",`"version`":`"$Version`",`"target`":`"default`",`"stamp`":`"$stamp`"}," + $text.Substring($i + 1))
    Copy-Item ".\tests\fixtures\layers\$Dir\ontology.yaml" $to
    $to
}
$dir = Join-Path $env:TEMP 'portal-check-1.19'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem -Tenants $t
try {
    $u = "$($p.Url)/api/tenants/default"
    $env:PORTAL_TOKEN = $admin.Token
    $null = Mount-PortalLayer -Name agent-runs -Path (Copy-Stamped agent-runs Emitter.Agent 0.1.4) -Url $p.Url
    $null = Mount-PortalLayer -Name agent-alerts -Path (Copy-Stamped agent-alerts Emitter.Environment 0.4.0) -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path (Copy-Stamped ledger Emitter.Unlisted 1.0.0) -Url $p.Url
    $e = Invoke-RestMethod "$u/emitters"
    $e.nodes | Where-Object { $_.kind -in 'Card', 'Emitter' } | ForEach-Object { "$($_.kind) $($_.id): $($_.properties.status), card $($_.properties.version), layers $($_.properties.layerVersions -join '/')" }
    $e.edges | Where-Object kind -EQ 'produced-by' | ForEach-Object { "$($_.from) produced-by $($_.to)" }
    $one = (Invoke-WebRequest "$u/emitters/Emitter.Environment").Content | ConvertFrom-Json -DateKind String
    $c = $one.nodes | Where-Object id -EQ $one.root
    "card $($c.properties.module) $($c.properties.repo) $($c.properties.url) emits $($c.properties.emits.facet -join ',')"
    $one.nodes | Where-Object kind -EQ 'Layer' | ForEach-Object { "$($_.name): source $($_.properties.source.version) $($_.properties.source.target) $($_.properties.source.stamp)" }
    "no emitter: $(('Graph.Emitter.Agent', 'Graph.Emitter.Terraform' | ForEach-Object { [int](Invoke-WebRequest "$u/emitters/$_" -SkipHttpErrorCheck).StatusCode }) -join ' ')"
    "pages: $(('/default/emitters', '/default/emitters/Emitter.Environment', '/default/emitters/Graph.Emitter.FileSystem', '/default/emitters/sources', '/default/emitters/Graph.Emitter.Agent' | ForEach-Object { [int](Invoke-WebRequest "$($p.Url)$_" -SkipHttpErrorCheck).StatusCode }) -join ' ')"
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `Card ui:card:cards-oci/Emitter.Agent@0.1.4: ok, card 0.1.4, layers 0.1.4`, `Card ui:card:cards-oci/Emitter.Environment@0.3.0: version-mismatch, card 0.3.0, layers 0.4.0` (the alerts copy stamps Claude.Agent.Alerts' 0.4.0), `Emitter ui:emitter:Emitter.Unlisted: no-card, card , layers 1.0.0`, `Emitter ui:module:Graph.Emitter.FileSystem: unstamped, card , layers ` (no source block: under its module, never a card); the four produced-by lines to those ids; `card Claude.Agent.Orchestrator Portal.Agent https://github.com/JerryBalmer1/Portal.Agent emits alerts`; `agent-alerts: source 0.4.0 default 2026-10-11T19:00:31Z`; `no emitter: 404 404` (Graph.Emitter.Agent is a card's module, never mapped to it); `pages: 200 200 200 200 404`, Emitters/Sources keeping its path (docs/design.md 44, 49). A card with no layer would read `no-layers`; Pester and Go check it with all nine cards.

### 1.20 Ledger chains: the agent chain from a mounted layer, Portal's own; paging, a run's entries, one entry; a broken copy
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.20-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
Set-Content "$t\default\ledger.jsonl" ('{"at":"2026-10-11T00:00:00.000Z","principal":"ui:principal:check","name":"check","role":"admin","feed":"cards-oci","kind":"oci","card":"Emitter.Agent","version":"0.1.3","type":"emitter","hash":"sha256:' + ('ab' * 32) + '","digest":null,"files":1,"result":"pushed","message":"Emitter.Agent 0.1.3 to cards-oci"}')
$dir = Join-Path $env:TEMP 'portal-check-1.20'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Tenants $t
try {
    $u = "$($p.Url)/api/tenants/default"
    $env:PORTAL_TOKEN = $admin.Token
    "before a ledger layer: $(((Invoke-RestMethod "$u/ledger").nodes.id) -join ', '); agent $([int](Invoke-WebRequest "$u/ledger/agent" -SkipHttpErrorCheck).StatusCode)"
    $null = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\agent-runs -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path .\tests\fixtures\layers\ledger -Url $p.Url
    $show = { (Invoke-RestMethod "$u/ledger").nodes | ForEach-Object { $q = $_.properties; "$($_.id): count $($q.count) valid $($q.valid) $($q.first)..$($q.last) break $($q.breakAt) $($q.reason) unchained $($q.unchained)" } }
    & $show
    $page = Invoke-RestMethod "$u/ledger/agent?limit=3"
    "page: $(($page.nodes | Where-Object kind -EQ 'LedgerEntry').id -join ', '); next $($page.nodes[0].properties.next)"
    $run = Invoke-RestMethod "$u/ledger/agent?run=$([uri]::EscapeDataString('agent:run/2026-10-09T02:50:43.7976027Z'))"
    "run: $($run.nodes[0].properties.entries) entries in $(($run.nodes | Where-Object kind -EQ 'LedgerEntry' | Select-Object -First 1).properties.runLayer)"
    $one = Invoke-RestMethod "$u/ledger/agent/50"
    "entry: root $($one.root); $(($one.edges | Where-Object kind -NE 'has-entry' | ForEach-Object { "$($_.kind) $($_.to)" }) -join '; ')"
    "404s: $(('/ledger/nope', '/ledger/agent/99' | ForEach-Object { [int](Invoke-WebRequest "$u$_" -SkipHttpErrorCheck).StatusCode }) -join ' ')"
    $null = Mount-PortalLayer -Name ledger -Path .\tests\fixtures\layers\ledger-broken -Url $p.Url
    & $show
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `before a ledger layer: ui:ledger:portal; agent 404`; then `ui:ledger:agent: count 12 valid True 44..55 break   unchained ` and `ui:ledger:portal: count 1 valid True 1..1 break   unchained 1` (the seeded line has no seq or prev: the unchained head); `page: ledger:55, ledger:54, ledger:53; next 53`; `run: 9 entries in agent-runs`; `entry: root ledger:50; prev ledger:49; records agent:run/2026-10-09T02:50:43.7976027Z/step-2; records agent:run/2026-10-09T02:50:43.7976027Z/step-1` (entry 49 is served as the one before, with its own records edge); `404s: 404 404`; and, with the broken copy mounted as `ledger`, `ui:ledger:agent: count 12 valid False 44..55 break 51 prev-mismatch unchained ` (docs/design.md 45).

### 1.21 Environments graph: the real layer alone, then the tagged copy with the runs and ledger; one hop, edge kinds kept
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.21-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-1.21'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Tenants $t
try {
    $u = "$($p.Url)/api/tenants/default"
    $env:PORTAL_TOKEN = $admin.Token
    $show = { $g = Invoke-RestMethod "$u/environments/graph"; "nodes: $(($g.nodes | ForEach-Object { "$($_.kind) $($_.id)" }) -join ', ')"; "edges: $(($g.edges | ForEach-Object { "$($_.kind) $($_.from) -> $($_.to)" } | Sort-Object) -join '; ')" }
    "before: $((Invoke-RestMethod "$u/environments/graph").nodes.Count) nodes"
    $null = Mount-PortalLayer -Name agent-environment -Path .\tests\fixtures\layers\agent-environment -Url $p.Url
    & $show
    $null = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\agent-runs -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path .\tests\fixtures\layers\ledger -Url $p.Url
    $null = Mount-PortalLayer -Name agent-environment -Path .\tests\fixtures\layers\agent-environment-tagged -Url $p.Url
    & $show
    $run = (Invoke-RestMethod "$u/environments/graph").nodes | Where-Object kind -EQ 'Run'
    "run: layer $($run.properties.layer), ledger $($run.properties.ledger)"
    "page: $([int](Invoke-WebRequest "$($p.Url)/default/environments/graph" -SkipHttpErrorCheck).StatusCode)"
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `before: 0 nodes`; with the real layer, its nine nodes under `env:` ids and the principal: `nodes: environment.instance env:compose/claude-agent-dev, environment.instance env:container/claude-agent-dev/agent, environment.instance env:container/claude-agent-dev/ledger, environment.instance env:container/claude-agent-dev/mitm, environment.image env:image/claude-agent-ledger:dev, environment.image env:image/claude-agent:dev, environment.image env:image/mitmproxy/mitmproxy:12.2.3, environment.network env:network/claude-agent-dev_inside, environment.network env:network/claude-agent-dev_outside, Principal ui:principal:dev-bootstrap` and ten edges, sorted: three `attached-to` (agent to `_inside`, mitm to `_inside` and `_outside`), three `member-of` onto `env:compose/claude-agent-dev`, `runs-as env:compose/claude-agent-dev -> ui:principal:dev-bootstrap` and three `runs-image` (its hosts edge names a run no mounted layer holds, so it is left out); with the tagged copy, the runs and the ledger, the same nodes and `Run agent:run/2026-10-09T02:50:43.7976027Z`, the same edges and `hosts env:compose/claude-agent-dev -> agent:run/2026-10-09T02:50:43.7976027Z` after the `attached-to` ones, no Step, Decision or ledger entry; `run: layer agent-runs, ledger 9`; `page: 200` (docs/design.md 46, 48).

### 1.22 Alerts: the real layer's one alert under alert:<fingerprint>; a layer of alert/<fingerprint> ids mounts and shows none
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-1.22-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-1.22'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Tenants $t
try {
    $u = "$($p.Url)/api/tenants/default"
    $env:PORTAL_TOKEN = $admin.Token
    "before: $((Invoke-RestMethod "$u/alerts").nodes[0].properties.count) alerts"
    foreach ($m in @('agent-runs', 'ledger', 'agent-alerts')) { $null = Mount-PortalLayer -Name $m -Path ".\tests\fixtures\layers\$m" -Url $p.Url }
    $show = {
        $a = Invoke-RestMethod "$u/alerts"
        $root = $a.nodes | Where-Object id -EQ 'ui:alerts'
        "count $($root.properties.count); open: critical $($root.properties.open.critical), warning $($root.properties.open.warning), info $($root.properties.open.info)"
        $a.nodes | Where-Object kind -EQ 'Alert' | ForEach-Object { "$($_.id) $($_.properties.severity) $($_.properties.rule) $($_.properties.state) about $($_.properties.about) run $($_.properties.run)" }
        "edges: $(($a.edges | ForEach-Object { $_.kind }) -join ', ')"
    }
    & $show
    # The old form: an alert under alert/<fingerprint> mounts and is no alert.
    $old = @{ name = 'agent-alerts-old'; envelope = @{ schema = 'graph/1'; module = 'Claude.Agent.Alerts'; version = '0.2.0'; layer = '2026-10-09T02:51:05Z'; edges = @()
            nodes = @(@{ id = 'alert/3f9c2a7e5b1d4c08'; kind = 'alert'; properties = @{ severity = 'critical'; rule = 'policy-deny'; summary = 'old form'; stamp = '2026-10-09T02:50:49Z'; state = 'open'; fingerprint = '3f9c2a7e5b1d4c08' } }) } } | ConvertTo-Json -Depth 6
    "old: $([int](Invoke-WebRequest "$u/layers" -Method Post -ContentType 'application/json' -Body $old -Headers @{ Authorization = "Bearer $($admin.Token)" } -SkipHttpErrorCheck).StatusCode)"
    & $show
    "old alert: $([int](Invoke-WebRequest "$u/alerts/3f9c2a7e5b1d4c08" -SkipHttpErrorCheck).StatusCode)"
    "page: $([int](Invoke-WebRequest "$($p.Url)/default/compliance/alerts" -SkipHttpErrorCheck).StatusCode)"
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `before: 0 alerts`; `count 1; open: critical 0, warning 0, info 1`; `alert:e62d4da8120f3a864c3576cd6b3bbb838923641c71b01f9153d41e4973face6c info egress-attempt open about ledger:303 run agent:run/2026-10-11T19:00:22.5540487Z`; `edges: has-alert` (neither ledger:303 nor that run is in a mounted layer, so about and run name them and no edge leads there); `old: 201`, then the same three lines again (the alert/ node is no alert, and nothing maps it); `old alert: 404`; `page: 200` (docs/design.md 47, 48). The five varied alerts the page's order, chips and menus need are mounted inline by Go and Pester.

## 2. Page

### 2.1 The shell, and the FileSystem layer in colour on Graph/Explore
```powershell
# needs C:\__Code\Graph.Emitter.FileSystem\artifacts\layers (falls back to the fixture); mounts C:\__Code\Claude.Agent\artifacts\layers as agent-runs when present
Invoke-Build Screenshot
```
Expect: `captured ...\docs\screenshots\summary.png`, `...service-connections.png`, `...explore.png`, `...identity-users.png`, then `canvas coloured from ontology: <n>/<n>`. Every PNG is dark: a near-black grid behind cards. The banner holds `Portal`, the breadcrumb (`default / Overview / Summary` and so on), a search box, `marker: none`, `Help` and the `anonymous` avatar. The rail lists Overview to Stack with icons, Settings at the foot above `Collapse`, and the open service's pages in place, the current one marked with the accent. summary.png shows the empty state `No summary yet`; service-connections.png one row, `github`, `https://api.github.com`, a red dot and `GITHUB_TOKEN`, a `created by` of `example` and a `Test` button; identity-users.png the Add user form (name, `reader`, a greyed `Add`) above one row, `example`, `admin`, `2026-10-11T00:00:00.000Z`, `bootstrap`; explore.png the toolbar `filesystem · <n> nodes · <m> edges · <stamp>`, the badge `screen 20 · 19 · <stamp>` and `read-only: sign in to write`, above the filesystem Canvas filling the pane (with the sibling's layer Directories are blue rounded squares labelled `name/`, Files grey circles; with the fixture, Directories amber, Files blue).

### 2.2 Approve from the Designer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
"token: $env:PORTAL_TOKEN"
$dir = Join-Path $env:TEMP 'portal-check-2.3'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/default/designer/sections"
Read-Host 'Give the avatar the printed token; expand Add Category and add Tree refining Canvas; expand Add Menu Entry and add Designer opening designer; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: after Add Category the Categories list shows `└ Tree (refines Canvas)` under Canvas and the category and refines dropdowns offer `· Tree`; Add Menu Entry answers with no error and the entry is a screen node (`GET /api/tenants/default/screen` holds `ui:menu:designer`), though since 0.4.0 the shell's rail is the tenant's menu and does not draw it (docs/design.md 36); neither add reloads the page. Without the token every button in the Designer is disabled, with the tooltip `sign in to write` (since 0.4.1, docs/design.md 37).

### 2.4 Designer sections collapse, and stay as left across Approve and a restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
(Get-Content .\tenants\default\connections.example.yaml) -replace 'ui:principal:example', 'ui:principal:check' | Set-Content "$env:PORTAL_TENANTS\default\connections.yaml"
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
"token: $((New-PortalToken -Name check -Bootstrap).Token)"
$dir = Join-Path $env:TEMP 'portal-check-2.5'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/"
Read-Host 'Click Graph, then Workflows; hover Policy; press Collapse and hover Ledger; Expand; open Settings > Service connections, press F5, press Test; give the avatar the printed token and press Test again; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir, "$env:PORTAL_TENANTS\default\connections.yaml" -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: the page opens at `/default/overview/summary` with Overview open in the rail. Clicking Graph opens its pages in place and closes Overview's, and Workflows does the same to Graph. Hovering a closed service shows its pages in a flyout beside it. Collapse leaves icons only, and a hover still shows the flyout. After F5 on Service connections the same page, breadcrumb and open Settings come back. Test before signing in shows `unauthorized: ...needs a bearer token` in red. After the avatar takes the token it reads `check` with accent initials, and Test then shows `no_token: ...GITHUB_TOKEN, which is not set` (a count of repositories when `GITHUB_TOKEN` is set). A wrong token is dropped with `unauthorized` in the avatar panel.

### 2.6 Read-only for anonymous; refused writes are toasts; the wheel zoom speed
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"
Set-Content "$env:PORTAL_TENANTS\default\portal.yaml" 'theme: { graph: { wheelZoomSpeed: 3 } }'
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
"token: $((New-PortalToken -Name check -Bootstrap).Token)"
$dir = Join-Path $env:TEMP 'portal-check-2.6'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process "$($p.Url)/default/graph/explore"
Read-Host 'Hover collapse; open Designer > Sections and hover Approve; back on Graph > Explore, in DevTools remove the collapse button''s disabled attribute and press it; wheel over the Canvas; give the avatar the printed token and reload; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir, "$env:PORTAL_TENANTS\default\portal.yaml" -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: as anonymous the Canvas's `collapse` and every Designer button (section toggles, Approve, the four Adds) are greyed and show `sign in to write` on hover, and the toolbar says `read-only: sign in to write`. The collapse forced past `disabled` shows a toast at the bottom right, `unauthorized: PATCH writes, and a write needs a bearer token; without one the portal reads only`, for 5 s; the Canvas keeps drawing the graph and no error text appears in the panel. Signed in as `check`, the buttons are live and the badge is gone. With `wheelZoomSpeed: 3` the Canvas host carries `data-wheel-sensitivity="0.6"` (0.2 with the committed `{}`), and one wheel notch zooms about three times as far.

### 2.7 Identity/Users Add, Identity/Agents, Settings/Tokens Revoke
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
"admin token: $((New-PortalToken -Name check -Bootstrap).Token)"
$dir = Join-Path $env:TEMP 'portal-check-2.7'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
Start-Process "$($p.Url)/default/identity/users"
Read-Host 'Hover Add; give the avatar the admin token; Add "ann" as reader; open Identity > Agents; open Settings > Tokens and press Revoke twice on check''s row; press Enter here to stop'
Get-Content "$env:PORTAL_TENANTS\default\principals.yaml"
Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: as anonymous Users lists `check`, `admin`, its created time and `bootstrap`, and the Add button is greyed with `sign in to write`. Signed in, the avatar reads `check` and the role list offers reader, contributor and admin; Add shows `Added ann. Issue its token with New-PortalToken -Name ann.` and a row `ann`, `reader`, `<created>`, `check`. Agents shows `No agents yet` and `An agent is a principal holding runs: mount a Graph.Emitter.Agent runs layer (Mount-PortalLayer) and its runs list under the principal whose token mounted it.` Tokens lists `check` with a 12-digit hash and its created time; the first Revoke turns into `Revoke <hash>?`, the second removes it, and the page drops the now-refused token and reads as anonymous. The printed principals.yaml ends with ann's entry, `createdBy: 'ui:principal:check'`.

### 2.8 Dev mode in the browser: the amber bar, Use dev token, a reader cannot Add
```powershell
# Invoke-Build Dev runs Up: it stops a portal Up started and serves tenants/default on 8200, so it needs that tenant in mode dev.
Import-Module .\src\Portal\Portal.psd1 -Force
Invoke-Build Dev
Start-Process 'http://127.0.0.1:8200/default/identity/users'
Read-Host 'Read the bar; open the avatar and press Use dev token; Add "rea" as reader; in another PowerShell run New-PortalToken -Name rea; Add "rea2" (the portal re-reads tokens.yaml); sign out, paste rea''s token, Sign in, and hover Add; press Enter here to stop'
Invoke-Build Down
```
Expect: Invoke-Build prints `minted dev-bootstrap (admin) in <repo>\tenants\default\tokens.yaml; replaced <n> older dev-bootstrap token(s)`, the Up line, then two `DEV ONLY` lines, the second with the token. The page has an amber bar under the banner, `DEV MODE — auto-minted admin, local sign-in shortcut. Not for demo or production. See README: Before you demo.`, with no close button, and a `DEV` badge in the banner. The avatar panel offers `Use dev token`; pressing it signs in and the avatar reads `dev-bootstrap · admin`. Add `rea` shows its row, created by `dev-bootstrap`. After the Add of `rea2`, rea's token is live: signed in with it, the avatar reads `rea · reader` and Add is greyed, its tooltip `admin only: rea is reader`.

### 2.9 Artifacts/Feeds lists the feeds and the cards pushed to each
```powershell
# after item 3.7 (Invoke-Build Feeds), with that portal still on 8200
Start-Process "http://127.0.0.1:8200/default/artifacts/feeds"
```
Expect: under Artifacts, Feeds is the first page. Two cards, `Cards (OCI)` and `Cards (NuGet)`, each with its kind, `private` and its URL, and in each a row `Tool.Git · 0.1.0 · tool · <12 hex> · <pushed> · dev-bootstrap` (or whoever pushed); hovering the hash shows the whole `sha256:` and the feed's digest. The toolbar reads `2 feeds · 2 cards`. No button pushes or deletes. The amber DEV MODE bar and the avatar behave as in 2.8.

### 2.10 Graph/Layers links each layer to its page; the page shows its counts, stamp and nodes
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-2.10-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$dir = Join-Path $env:TEMP 'portal-check-2.10'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem -Tenants $t
try {
    Start-Process "$($p.Url)/default/graph/layers"
    Read-Host 'Read Graph/Layers, click filesystem, press Enter here to stop'
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: Graph/Layers is the fourth page under Graph and is marked current; the toolbar reads `1 layers`; one row, `filesystem · Graph.Emitter.FileSystem 0.1.0 · 52 · 51 · 2026-10-09T00:51:29Z · <repo>\tests\fixtures\layers\filesystem\2026-10-09T00-51-29Z.graph.json`. Clicking `filesystem` opens `/default/layers/filesystem` in place: the title `filesystem`, crumbs `default › Graph › Layers › filesystem`, Graph/Layers still current in the rail, a toolbar badge `52 nodes · 51 edges · 2026-10-09T00:51:29Z`, then `52 nodes`, and 52 rows of id, kind, label, updated, the first `fs:. · Directory · Graph.Emitter.FileSystem · 2026-10-09T00:51:29Z`.

### 2.11 Identity/Agents lists the agents; an agent's page lists its runs, deny in red, linking the layer and each decision
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-2.11-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-2.11'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem '' -Tenants $t
try {
    $env:PORTAL_TOKEN = $admin.Token
    $null = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\agent-runs -Url $p.Url
    Start-Process "$($p.Url)/default/agents"
    Read-Host 'Read Identity/Agents, click check, read its runs, click "step 3 pre deny", press Enter here to stop'
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the rail's Identity/Agents links to `/default/agents` and is marked current; the crumbs read `default › Identity › Agents`; the toolbar `1 agents`; one row, `check · user · admin · 1 · 1 · 2026-10-09T02:50:43Z`, the `1` under denied in red. Clicking `check` opens `/default/agents/check` in place: the title `check`, crumbs `default › Identity › Agents › check`, a badge `1 runs · 1 denied` (`user · admin` on hover), then `1 runs`, and one run row, `2026-10-09T02:50:43Z · 2026-10-09T02:50:56Z · 3 · 4 · 1 · 0 · 0 · agent-runs`, the deny `1` in red and `agent-runs` a link to `/default/layers/agent-runs`. Under it, five links, `step 1 pre allow` to `step 3 pre deny`, the last in red, each with the decision's name and reason on hover. Clicking `step 3 pre deny` opens the layer page with Graph/Layers current, and the row `agent:run/2026-10-09T02:50:43.7976027Z/step-3/pre · PolicyDecision · deny rules[2] (pre)` marked and scrolled into view (docs/design.md 43).

### 2.12 Emitters/Catalogue is a roll call with status and facet chips; an emitter's page is its source, built from the card; a layer page links back
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-2.12-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
# Two pushes as 0.13.0 writes them: the card's module, repo, url and emits on the line.
$line = '{"at":"2026-10-11T00:00:0{0}.000Z","principal":"ui:principal:check","name":"check","role":"admin","feed":"cards-oci","kind":"oci","card":"{1}","version":"{2}","type":"emitter","hash":"sha256:{3}","digest":null,"files":1,"result":"pushed","message":"{1} {2} to cards-oci","module":"{4}","repo":"{5}","url":"https://github.com/JerryBalmer1/{5}","emits":[{6}]}'
Set-Content "$t\default\ledger.jsonl" @(
    $line.Replace('{0}', '0').Replace('{1}', 'Emitter.Agent').Replace('{2}', '0.1.4').Replace('{3}', 'ab' * 32).Replace('{4}', 'Graph.Emitter.Agent').Replace('{5}', 'Portal.Emitters').Replace('{6}', '{"facet":"runs","kinds":["Run","Step","Action","PolicyDecision"]}')
    $line.Replace('{0}', '1').Replace('{1}', 'Emitter.Environment').Replace('{2}', '0.3.0').Replace('{3}', 'cd' * 32).Replace('{4}', 'Claude.Agent.Orchestrator').Replace('{5}', 'Portal.Agent').Replace('{6}', '{"facet":"alerts","kinds":["alert"]}'))
# Copies of real fixture layers with a source block inserted (no layer on disk carries one yet).
function Copy-Stamped([string] $Dir, [string] $Emitter, [string] $Version) {
    $to = Join-Path $t "stamped\$Dir"
    $null = New-Item -ItemType Directory -Force $to
    $f = Get-ChildItem ".\tests\fixtures\layers\$Dir" -Filter '*.graph.json'
    $text = Get-Content $f.FullName -Raw
    $stamp = [regex]::Match($text, '"layer"\s*:\s*"([^"]+)"').Groups[1].Value
    $i = $text.IndexOf('{')
    Set-Content (Join-Path $to $f.Name) -NoNewline ($text.Substring(0, $i + 1) + "`"source`":{`"emitter`":`"$Emitter`",`"version`":`"$Version`",`"target`":`"default`",`"stamp`":`"$stamp`"}," + $text.Substring($i + 1))
    Copy-Item ".\tests\fixtures\layers\$Dir\ontology.yaml" $to
    $to
}
$dir = Join-Path $env:TEMP 'portal-check-2.12'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem -Tenants $t
try {
    $env:PORTAL_TOKEN = $admin.Token
    $null = Mount-PortalLayer -Name agent-runs -Path (Copy-Stamped agent-runs Emitter.Agent 0.1.4) -Url $p.Url
    $null = Mount-PortalLayer -Name agent-alerts -Path (Copy-Stamped agent-alerts Emitter.Environment 0.4.0) -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path (Copy-Stamped ledger Emitter.Unlisted 1.0.0) -Url $p.Url
    Start-Process "$($p.Url)/default/emitters"
    Read-Host 'Read Emitters/Catalogue, click the chips, click Emitter.Environment and its layers, press Enter here to stop'
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the rail's Emitters/Catalogue is marked current; the toolbar `4 emitters · 1 ok · 1 version-mismatch · 1 no-card · 1 unstamped`, a status chip group (all, ok, version-mismatch, no-layers, no-card, unstamped) and a facet chip group (all, alerts, runs). Rows: `Emitter.Agent` with a green `ok`, `0.1.4 · 0.1.4 · runs · Emitter.Agent@0.1.4 · 1`; `Emitter.Environment` marked red with `version-mismatch`, `0.3.0`, `0.4.0` in red, `alerts`, its card link, `1`; `Emitter.Unlisted` in amber with `no-card` and `1.0.0`; `Graph.Emitter.FileSystem` muted, `unstamped`. A chip adds `?status=` or `?facet=` to the address and hides the other rows (`1 of 4 emitters · ...`). `Emitter.Environment` opens its page: badges `version-mismatch` and `1 layers`; module `Claude.Agent.Orchestrator`, version `0.3.0`, layer versions `0.4.0` in red, repo `Portal.Agent` and `Open in source` both opening https://github.com/JerryBalmer1/Portal.Agent in a new tab, card `Emitter.Environment@0.3.0` (opening its marked row on Artifacts/Feeds), the emits table `alerts · alert`; one layer row marked red, `agent-alerts · 0.4.0 · default · 2026-10-11T19:00:31Z · 1 · 2 · check`. `agent-alerts` opens its layer page: `produced by Emitter.Environment 0.4.0 (card 0.3.0) version-mismatch`, linking back; `/default/layers/filesystem` reads `produced by Graph.Emitter.FileSystem unstamped`. `/default/emitters/sources` still draws Emitters/Sources (docs/design.md 44, 49).

### 2.13 Ledger/Entries: chains valid in green or broken in red; a chain's entries link their runs; the agent page counts each run's entries
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-2.13-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
Set-Content "$t\default\ledger.jsonl" ('{"at":"2026-10-11T00:00:00.000Z","principal":"ui:principal:check","name":"check","role":"admin","feed":"cards-oci","kind":"oci","card":"Emitter.Agent","version":"0.1.3","type":"emitter","hash":"sha256:' + ('ab' * 32) + '","digest":null,"files":1,"result":"pushed","message":"Emitter.Agent 0.1.3 to cards-oci"}')
$dir = Join-Path $env:TEMP 'portal-check-2.13'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Tenants $t
try {
    $u = "$($p.Url)/api/tenants/default"
    $env:PORTAL_TOKEN = $admin.Token
    $null = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\agent-runs -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path .\tests\fixtures\layers\ledger -Url $p.Url
    Start-Process "$($p.Url)/default/ledger"
    Read-Host 'Read Ledger/Entries, click agent, click the run of entry 50 and come back, click 51; open /default/agents/check and click the ledger count; press Enter here to stop'
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the rail's Ledger/Entries links to `/default/ledger` and is marked current; the toolbar `2 chains`; the row `agent · valid (green) · 12 · 44 · 55 · 2026-10-09T02:51:00Z · layer ledger`, and `portal · valid (green) · 1 · 1 · 1 · 2026-10-11T00:00:00.000Z · ledger.jsonl · 1 before the chain`. `agent` opens `/default/ledger/agent`: the title `agent ledger`, a badge `12 entries · valid`, rows 55 down to 44, each seq linking its entry, entries 46 to 54 showing the run `agent:run/2026-10-09T02:50:43.7976027Z`, linking `/default/layers/agent-runs#<the Run's or Step's id>`, which marks that row (entry 50's marks `.../step-2`). `51` opens `/default/ledger/agent/51`: its fields, and `prev` with a link `entry 50`. On `/default/agents/check` the run row ends with a `ledger` column, `9`, linking `/default/ledger/agent?run=...`, which lists entries 54 to 46 under a badge `9 entries · valid · run agent:run/...`. With `ledger-broken` mounted as `ledger` instead, the agent row reads `broken at 51` in red, the toolbar `2 chains · 1 broken`, and entry 51's row is marked red (docs/design.md 45).

### 2.14 Environments/Graph: hulls by env, chips, # selects and pins; right-click on a node and an Instances row
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-2.14-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-2.14'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Tenants $t
try {
    $env:PORTAL_TOKEN = $admin.Token
    $null = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\agent-runs -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path .\tests\fixtures\layers\ledger -Url $p.Url
    $null = Mount-PortalLayer -Name agent-environment -Path .\tests\fixtures\layers\agent-environment-tagged -Url $p.Url
    Start-Process "$($p.Url)/default/environments/graph#env%3Acompose%2Fclaude-agent-dev"
    Read-Host 'Read the graph; drag agent, double-click it; click the dev chip, then a tag; reload; right-click the compose node and mitm (and Shift+F10 on a focused node); open Instances and right-click a row, then Show graph; press Enter here to stop'
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the rail lists Environments/Graph above Instances and marks it current. The toolbar reads `9 nodes · 2 one edge away · 11 edges · 1 without env`, then the env chips (all, dev, qa, prod, unset), the tag chips (all, agent, compose, egress) and `selected env:compose/claude-agent-dev · 5 neighbours pinned`. The graph settles at once: the compose node orange, the three containers blue circles, three images as squares, two networks as diamonds, dev-bootstrap and the run small, dim hexagons; `member-of` solid, `runs-as` dashed, `hosts` dotted, `runs-image` and `attached-to` as other kinds; an amber hull `env dev` holding the compose node, agent, mitm, the images and the networks, with dotted inner hulls `stack` and `egress`, and a grey dashed hull `env unset` round the ledger keeper; a legend below. The compose node is outlined and it and its five neighbours have dashed outlines (pinned). Dragging agent moves and pins it; double-clicking releases it. The dev chip hides the ledger keeper and its edge. A reload keeps `#env%3Acompose%2Fclaude-agent-dev` selected. Right-click on the compose node: Details (`/default/environments/instances/env%3Acompose%2Fclaude-agent-dev`), Show graph, Runs (`/default/layers/agent-runs#agent%3Arun%2F...`, that row marked) and Ledger (`/default/ledger/agent?run=...`, 9 entries); on mitm, Runs and Ledger greyed with `no hosts edge to a mounted run`. Arrow keys move, Escape closes and returns focus. On Instances, a row's right-click shows the same menu; Show graph opens the graph with that node selected (docs/design.md 46, 48).

### 2.15 Compliance/Alerts: severity colours, chips, an alert's page, the banner badge, alerts rows on the subjects, right-click
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-2.15-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"
Set-Content "$t\default\portal.yaml" 'mode: demo'
$admin = New-PortalToken -Name check -Bootstrap -Tenants $t
$dir = Join-Path $env:TEMP 'portal-check-2.15'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Tenants $t
try {
    $env:PORTAL_TOKEN = $admin.Token
    $null = Mount-PortalLayer -Name agent-runs -Path .\tests\fixtures\layers\agent-runs -Url $p.Url
    $null = Mount-PortalLayer -Name ledger -Path .\tests\fixtures\layers\ledger -Url $p.Url
    $null = Mount-PortalLayer -Name agent-environment -Path .\tests\fixtures\layers\agent-environment-tagged -Url $p.Url
    $null = Mount-PortalLayer -Name agent-alerts -Path .\tests\fixtures\layers\agent-alerts -Url $p.Url
    Start-Process "$($p.Url)/default/compliance/alerts"
    Read-Host 'Read the list; click the info chip, then the open state chip; open the alert; right-click its row (and Shift+F10 on a focused link); press Enter here to stop'
}
finally { $env:PORTAL_TOKEN = $null; Stop-Portal -Artifacts $dir; Remove-Item $dir, $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the rail lists Compliance/Alerts above Rules and marks it current; beside the DEMO badge no alerts badge, since the one open alert is info. The toolbar reads `1 alerts`, then the severity chips (all, critical, warning, info), the rule chips (all, egress-attempt) and the state chips (all, open). One row, info, muted bar: `egress-attempt`, its summary linking `/default/compliance/alerts/e62d4da8120f3a864c3576cd6b3bbb838923641c71b01f9153d41e4973face6c`, its subject `ledger:303` and its run `agent:run/2026-10-11T19:00:22.5540487Z` as plain text with why in their titles (no mounted layer holds them), stamp `2026-10-11T19:00:22.5540487Z`. The info chip and then the open chip keep the row and put `?severity=info&state=open` in the address. The alert's page lists id `alert:e62d...`, severity, rule, summary, state, stamp, fingerprint, layer `agent-alerts` (a link) and about and raised-in as plain text with why. A row's right-click offers Details as a link and Subject, Run and Ledger greyed with why, as Pester's orphan alert shows; arrows move, Escape closes and returns focus. The varied alerts behind the colours, the badge and the subjects' alerts rows are checked by Pester's Alerts block (docs/design.md 47, 48).

## 3. PowerShell

### 3.1 Approve, PATCH a Theme, approve, diff, restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$env:PORTAL_TENANTS = Join-Path $env:TEMP 'portal-check-tenants'
$null = New-Item -ItemType Directory -Force "$env:PORTAL_TENANTS\default"; Copy-Item .\tenants\default\portal.yaml "$env:PORTAL_TENANTS\default" -Force
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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
Remove-Item "$env:PORTAL_TENANTS\default\tokens.yaml", "$env:PORTAL_TENANTS\default\principals.yaml" -ErrorAction SilentlyContinue
$env:PORTAL_TOKEN = (New-PortalToken -Name check -Bootstrap).Token; $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
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

### 3.5 New-PortalToken: -Bootstrap once, then tokens for existing principals only
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-3.5-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" '{}'
try {
    try { New-PortalToken -Name jerry -Tenants $t } catch { "before bootstrap: $($_.Exception.Message)" }
    $k = New-PortalToken -Name jerry -Bootstrap -Tenants $t
    "token: $($k.Token.Length) hex chars; on disk: $((Get-Content $k.Path -Raw).Contains($k.Token)); $($k.Principal) $($k.Role)"
    Get-Content "$t\default\principals.yaml" | Select-Object -Skip 1
    Get-Content $k.Path | Select-Object -Skip 1
    try { New-PortalToken -Name eve -Bootstrap -Tenants $t } catch { "again: $($_.Exception.Message)" }
    "second token: $((New-PortalToken -Name jerry -Tenants $t).Token -ne $k.Token)"
    try { New-PortalToken -Name eve -Tenants $t } catch { "unknown: $($_.Exception.Message)" }
}
finally { Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `before bootstrap: No principal named jerry in <path>\principals.yaml. The tenant has no principals yet: make the first admin with -Bootstrap.`, then `token: 64 hex chars; on disk: False; ui:principal:jerry admin`, then principals.yaml's `principals:` with one entry (`id: 'ui:principal:jerry'`, `name: 'jerry'`, `type: user`, `role: admin`, `created: '<time>'`, `createdBy: null`), then tokens.yaml's `tokens:` with `- principal: 'ui:principal:jerry'`, `sha256: <64 hex>`, `created: '<time>'`, then `again: -Bootstrap makes a tenant's first admin, once; <path>\principals.yaml already holds 1 principal(s) (jerry). Issue a token for one with New-PortalToken -Name <principal>.`, `second token: True` and `unknown: No principal named eve in <path>\principals.yaml. An admin adds one on Identity/Users.`

### 3.6 Invoke-Build Dev refuses outside mode dev; Up never mints
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$t = Join-Path $env:TEMP 'portal-check-3.6-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" 'mode: demo'
$saved = $env:PORTAL_TENANTS
try {
    $env:PORTAL_TENANTS = $t
    try { Invoke-Build Dev } catch { "Dev: $($_.Exception.Message)" }
    "identity files written: $(@(Get-ChildItem "$t\default" -Filter *.yaml).Name -join ', ')"
    "Up mints: $([regex]::Match((Get-Content .\.build.ps1 -Raw), '(?ms)^task Up Build, \{(.*?)^\}').Groups[1].Value -match 'Token|Mint')"
}
finally { $env:PORTAL_TENANTS = $saved; Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: Invoke-Build's own `ERROR: Dev refuses: ...` and `Build FAILED. 2 tasks, 1 errors`, then `Dev: Dev refuses: tenant default is in mode demo, not dev. Dev mints an admin and a local sign-in shortcut, so it runs only for a tenant whose portal.yaml says 'mode: dev'. Use Invoke-Build Up, which never mints anything; README, Before you demo.`, `identity files written: portal.yaml` (no principals.yaml or tokens.yaml: nothing was built or minted) and `Up mints: False`.

### 3.7 Invoke-Build Feeds: the stock services up and the sample card in both feeds (Docker Desktop)
```powershell
# tenants/default in mode dev or demo; Docker Desktop running. Starts compose project portal-default and replaces the portal on 8200.
Invoke-Build Feeds
docker compose -p portal-default ps --format '{{.Service}} {{.State}} {{.Health}} {{.Ports}}'
(Invoke-RestMethod http://127.0.0.1:8201/v2/cards/tool.git/tags/list).tags
(Invoke-RestMethod http://127.0.0.1:8202/v3/package/tool.git/index.json).versions
Get-Content .\tenants\default\ledger.jsonl | Select-Object -Last 2
Invoke-Build Feeds   # again: both pushes are reported as 409, and the task still succeeds
```
Expect: `pushed Tool.Git 0.1.0 to cards-oci as <principal>: hash sha256:..., digest sha256:...`, the same for cards-nuget, then `Feeds: Tool.Git 0.1.0 in cards-oci (registry tag) and cards-nuget (feed version), listed by GET .../feeds` and `Build succeeded`. `ps` lists `nuget running`, `postgres running healthy` (no port) and `registry running healthy`, the two feeds on `127.0.0.1:8201` and `127.0.0.1:8202` only. The tags are `0.1.0`, the versions `0.1.0`, and the two ledger lines are `"result":"pushed"` with one `hash` between them. The second run prints `already holds it (409)` for each feed (docs/design.md 40).

### 3.8 Invoke-Build Feeds refuses in mode prod
```powershell
$t = Join-Path $env:TEMP 'portal-check-3.8-tenants'
Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
$null = New-Item -ItemType Directory -Force "$t\default"; Set-Content "$t\default\portal.yaml" 'mode: prod'
$saved = $env:PORTAL_TENANTS
try {
    $env:PORTAL_TENANTS = $t
    try { Invoke-Build Feeds } catch { "Feeds: $($_.Exception.Message)" }
    "written under the tenant: $(@(Get-ChildItem "$t\default").Name -join ', ')"
}
finally { $env:PORTAL_TENANTS = $saved; Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: Invoke-Build's own `ERROR: Feeds refuses: ...` and `Build FAILED. 2 tasks, 1 errors`, then `Feeds: Feeds refuses: tenant default is in mode prod. Feeds starts the stock registry, NuGet feed and Postgres in Docker and pushes a sample card, so it runs only for a tenant in mode dev or demo; a prod tenant's feeds are its own (portal.yaml feeds:).` and `written under the tenant: portal.yaml` (no `data/`, nothing started).
