# Portal manual check list

Verified for: 0.3.1 on 2026-10-09, Windows 11 (10.0.26200), PowerShell 7.6.6, Go 1.27.0, Pester 6.2.0, InvokeBuild 5.14.23: items 0.2, 1.1, 1.4, 2.1 were run for 0.3.1; 0.1, 1.2, 1.3, 1.5, 1.6, 3.1, 3.2, 3.3 were verified for 0.3.0 and not re-run. Item 2.1 was checked by a headless capture (Invoke-Build Screenshot), not by a person at a browser. Items 2.2, 2.3 and 2.4 were not run: they need a person at a browser. 2.4's seeded look is pinned by Pester's headless `--dump-dom` check, but no one clicked a section toggle.

Run each block by saving it to a temp `.ps1` and `pwsh -NoProfile -File <temp>.ps1` from the repo root. Never `pwsh -Command { }`. Items 1 to 3 need item 0.2 (or `Invoke-Build Build`) to have built `build/win-x64/`. Every block starts its portal on a fresh directory under `$env:TEMP` and on the test port 18200, one at a time (0 is refused; docs/design.md 24): a directory that holds snaps boots from the newest of them.

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
Expect: go test ends `ok` for `internal/server` and `internal/store` (30 tests); Smoke prints `smoke: seed 18 nodes; approved <a>, then <b> changing ui:theme:default; restart booted from <b>; canvas fit {"graphWidth":<g>,"panelWidth":<p>,"ratio":<r>}` with `<g>` above half of `<p>` (662 of 903 here), and `artifacts/smoke-report.json` has `checks.menu.restart.topBar` `ui:menu:graph`, `ui:menu:smoke`; Pester ends `Tests Passed: 46, Failed: 0, Skipped: 0`; then `Build succeeded. 6 tasks, 0 errors, 0 warnings`.

## 1. API

### 1.1 Dev seed and the filesystem layer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-1.1'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    $s = Get-PortalScreen -Url $p.Url
    "boot: $($p.Boot); nodes: $($s.nodes.Count), edges: $($s.edges.Count), root: $($s.root)"
    $s.nodes | Group-Object kind | Sort-Object Name | ForEach-Object { "$($_.Name): $($_.Count)" }
    $s.nodes | Where-Object id -Like 'ui:state:designer:*' | ForEach-Object { "$($_.id) collapsed $($_.properties.collapsed)" }
    $l = Invoke-RestMethod "$($p.Url)/api/layers"
    "layer: $($l[0].name) $($l[0].counts.nodes) nodes, ontology $($l[0].hasOntology)"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `boot: seed; nodes: 18, edges: 15, root: ui:root`, then `Category: 5`, `Container: 3`, `Layout: 1`, `State: 7`, `Theme: 1`, `Widget: 1`, then the seven section States in order, `ui:state:designer:approve collapsed False`, `...kinds collapsed True`, `...categories collapsed False`, and `add-panel`, `add-container`, `add-category`, `add-menu-entry` all `collapsed True`, then `layer: filesystem 52 nodes, ontology True`.

### 1.2 Display, Graph.Node shape and Category refusals
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-1.2'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    foreach ($body in '{"id":"ui:w","kind":"Widget","properties":{"color":"#ff0000"}}',
                      '{"id":"ui:w","kind":"Widget","display":{"color":"#ff0000"}}',
                      '{"id":"ui:w","kind":"Widget","properties":{"type":"Nope"}}',
                      '{"id":"ui:theme:x","kind":"Theme","properties":{"opacity":2}}',
                      '{"id":"ui:category:X","kind":"Category","name":"X"}') {
        $r = Invoke-WebRequest -Method Post -Uri "$($p.Url)/api/nodes" -Body $body -ContentType application/json -SkipHttpErrorCheck
        "$($r.StatusCode) $((($r.Content | ConvertFrom-Json).problems.code) -join ',')"
    }
    $null = Invoke-RestMethod -Method Post -Uri "$($p.Url)/api/nodes" -Body '{"id":"ui:w","kind":"Widget"}' -ContentType application/json
    $r = Invoke-WebRequest -Method Post -Uri "$($p.Url)/api/edges" -Body '{"kind":"is-a","from":"ui:w","to":"ui:category:Nope"}' -ContentType application/json -SkipHttpErrorCheck
    "$($r.StatusCode) $((($r.Content | ConvertFrom-Json).problems.code) -join ',')"
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `422 display`, `422 schema`, `422 unknown-category`, `422 property-type` (Theme opacity above 1), `422 property-type` (a Category with no properties, which Graph.Node 0.2.0 lets through), `422 dangling-edge,unknown-category`.

### 1.3 A layer that fails graphnode validate is refused
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-1.3'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    try { Mount-PortalLayer -Name bad -Path .\tests\fixtures\invalid.graph.json -Url $p.Url }
    catch { "refused: $((@($_.TargetObject.problems.code) | Sort-Object -Unique) -join ', ')" }
    $layers = Invoke-RestMethod "$($p.Url)/api/layers"
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
$dir = Join-Path $env:TEMP 'portal-check-1.5'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$exe = '.\build\win-x64\portal.exe'
$boot = { param($url) [string](Invoke-WebRequest "$url/api/screen").Headers['X-Portal-Boot'] }
$run = {
    param($port)
    $proc = Start-Process $exe -ArgumentList '-port', $port, '-artifacts', $dir, '-filesystem', '""' -PassThru -NoNewWindow -RedirectStandardOutput "$dir.out" -RedirectStandardError "$dir.err"
    foreach ($i in 1..100) { try { $null = Invoke-WebRequest "http://127.0.0.1:$port/api/screen"; break } catch { Start-Sleep -Milliseconds 100 } }
    $proc
}
$null = New-Item -ItemType Directory -Force $dir
$proc = & $run 18951
try {
    "boot: $(& $boot 'http://127.0.0.1:18951')"
    $a = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:18951/api/snap -ContentType application/json -Body '{"note":"first"}'
    "first: previous [$($a.previous)] diff [$($a.diff)]"
    $null = Invoke-RestMethod -Method Patch -Uri 'http://127.0.0.1:18951/api/nodes/ui:theme:default' -ContentType application/json -Body '{"properties":{"color":"#336699"}}'
    $b = Invoke-RestMethod -Method Post -Uri http://127.0.0.1:18951/api/snap -ContentType application/json -Body '{"note":"blue"}'
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
$dir = Join-Path $env:TEMP 'portal-check-1.6'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
try {
    $post = { param($path, $body) Invoke-WebRequest -Method Post -Uri "$($p.Url)$path" -Body $body -ContentType application/json -SkipHttpErrorCheck }
    (& $post '/api/nodes' '{"id":"ui:menu:designer","kind":"Container","name":"Designer","properties":{"order":1}}').StatusCode
    (& $post '/api/edges' '{"kind":"is-a","from":"ui:menu:designer","to":"ui:category:Menu"}').StatusCode
    (& $post '/api/edges' '{"kind":"opens","from":"ui:menu:designer","to":"ui:designer"}').StatusCode
    $r = & $post '/api/edges' '{"kind":"opens","from":"ui:menu:designer","to":"ui:layout:root"}'
    "$($r.StatusCode) $((($r.Content | ConvertFrom-Json).problems.code) -join ',')"
    $null = New-PortalSnap -Url $p.Url -Note 'menu'
    Stop-Portal -Artifacts $dir
    $p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem ''
    (Get-PortalScreen -Url $p.Url).edges | Where-Object kind -In 'is-a', 'opens' | Where-Object from -Like 'ui:menu:*' | ForEach-Object id
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: `201`, `201`, `201`, `422 edge-endpoints`, then the four menu edges `ui:menu:graph|ui:category:Menu|is-a`, `ui:menu:graph|ui:canvas|opens`, `ui:menu:designer|ui:category:Menu|is-a`, `ui:menu:designer|ui:designer|opens`.

## 2. Page

### 2.1 The FileSystem layer draws in colour
```powershell
# needs C:\__Code\Graph.Emitter.FileSystem\artifacts\layers (falls back to the fixture)
Invoke-Build Screenshot
```
Expect: `captured ...\docs\screenshots\portal.png; canvas coloured from ontology: 52/52`. The top bar holds the title, the menu (one entry, `Graph`) and the counts. The PNG shows the filesystem Canvas on the left (with the sibling's layer, whose ontology uses Graph.Node 0.2.0 display: Directories as blue rounded squares labelled `name/`, larger than Files, which are grey circles; with the fixture, Directories are amber rounded boxes and Files blue squares), and the Designer panel on the right in seven sections, each a heading with an `expand` or `collapse` button at its right: Approve (open: its note field and `Approve` button), Kinds (ontology.yaml) (collapsed), Categories (open, five Categories), then Add Panel, Add Container, Add Category and Add Menu Entry, collapsed. The graph fills most of its panel. No snap button in the top bar.

### 2.2 Approve from the Designer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-2.2'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process $p.Url
Read-Host 'Type a note in Approve and press approve; then in a second shell PATCH ui:theme:default opacity to 0.3, reload, approve again; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: the first approve shows `approved <stamp> · <note>` and `first snap: nothing to compare`; after the PATCH and a reload the panels are more opaque, and the second shows `against <first stamp>:` and `changed 1 nodes, 0 edges: ui:theme:default`.

### 2.3 Menu, Add Category and Add Menu Entry in the Designer
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-2.3'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process $p.Url
Read-Host 'Collapse the Canvas, then press Graph in the top bar; expand Add Category and add Tree refining Canvas; expand Add Menu Entry and add Designer opening designer; press Enter here to stop'
Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
```
Expect: Graph expands the Canvas and outlines it; after Add Category the Categories list shows `└ Tree (refines Canvas)` under Canvas and the category and refines dropdowns offer `· Tree`; after Add menu entry the top bar shows `Graph` then `Designer`; neither add reloads the page.

### 2.4 Designer sections collapse, and stay as left across Approve and a restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-2.4'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
Start-Process $p.Url
Read-Host 'Press expand on Add Panel and collapse on Categories, then Approve; press Enter here to restart'
Stop-Portal -Artifacts $dir
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    (Get-PortalScreen -Url $p.Url).nodes | Where-Object id -In 'ui:state:designer:add-panel', 'ui:state:designer:categories' | ForEach-Object { "$($_.id) collapsed $($_.properties.collapsed)" }
    Start-Process $p.Url
    Read-Host 'Reload the page; press Enter here to stop'
}
finally { Stop-Portal -Artifacts $dir; Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue }
```
Expect: the seeded page shows Approve and Categories open and the other five collapsed, every heading and button in Title Case and the field labels lower case; each toggle hides or shows only its section's body and the Canvas does not redraw; Approve's result is `first snap: nothing to compare`; after the restart the block prints `ui:state:designer:add-panel collapsed False` and `ui:state:designer:categories collapsed True`, and the reloaded page shows Add Panel open and Categories collapsed.

## 3. PowerShell

### 3.1 Approve, PATCH a Theme, approve, diff, restart
```powershell
Import-Module .\src\Portal\Portal.psd1 -Force
$dir = Join-Path $env:TEMP 'portal-check-3.1'
Remove-Item $dir -Recurse -Force -ErrorAction SilentlyContinue
$p = Start-Portal -Port 18200 -Artifacts $dir -Filesystem .\tests\fixtures\layers\filesystem
try {
    $a = New-PortalSnap -Url $p.Url -Note 'first'
    Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/nodes/ui:theme:default" -ContentType application/json -Body '{"properties":{"opacity":0.2}}' | Out-Null
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
