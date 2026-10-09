#Requires -Version 7.4
# Run in a fresh process only: pwsh -NoProfile -File .\tests\Invoke-Tests.ps1
# Needs the binaries (Invoke-Build Build; Invoke-Build Pester builds first).

BeforeDiscovery {
    $script:Code      = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
    $script:GraphNode = Join-Path $Code 'Graph.Node'
    $script:Graph     = Join-Path $Code 'Graph'
    $script:HasGraphNode = Test-Path -LiteralPath (Join-Path $GraphNode 'src' 'GraphNode' 'GraphNode.psd1')
    $script:HasGraph     = Test-Path -LiteralPath (Join-Path $Graph 'src' 'Graph' 'Graph.psd1')
}

BeforeAll {
    $script:Repo      = Split-Path $PSScriptRoot -Parent
    $script:Code      = Split-Path $Repo -Parent
    $script:Manifest  = Join-Path $Repo 'src' 'Portal' 'Portal.psd1'
    $script:Fixtures  = Join-Path $PSScriptRoot 'fixtures'
    $script:FsFixture = Join-Path $Fixtures 'layers' 'filesystem'
    $script:SkillRoot = Join-Path $Repo '.claude' 'skills'
    Import-Module $Manifest -Force
    $script:GraphNodeBin = Join-Path $Repo 'build' 'win-x64' 'graphnode.exe'
    # git show prints UTF-8; read it as UTF-8 so a byte comparison holds for non-ASCII text.
    [Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)

    # The Graph.Node version the lift and the graph-node skill are pinned to (docs/design.md 4).
    $script:GraphNodeTag = 'v0.2.0'
    # The one edit the lift carries: the ui: row of the id prefix registry names Portal (docs/design.md 4).
    $script:Registered = @{
        'graph/ids.go'   = @('{Prefix: "ui:", Owner: "GraphRenderer", Shape: "reserved; ui: + a non-empty name"},',
                             '{Prefix: "ui:", Owner: "Portal", Shape: "ui: + a non-empty name; Portal''s screen graph (Portal ontology.yaml idRule per Kind)"},')
        'schemas/ids.md' = @('| `ui:` | GraphRenderer | reserved; `ui:` + a non-empty name |',
                             '| `ui:` | Portal | `ui:` + a non-empty name; Portal''s screen graph (Portal ontology.yaml idRule per Kind) |')
    }

    function Get-FrontVersion([string] $Path) {
        $text = Get-Content -LiteralPath $Path -Raw
        $front = [regex]::Match($text, '\A---\r?\n(.*?)\r?\n---', 'Singleline').Groups[1].Value
        [regex]::Match($front, '(?m)^version:\s*(\S+)\s*$').Groups[1].Value
    }
    # A file as committed at a revision of a sibling: the lift follows Graph.Node's tag, not its working tree, where
    # another session may be mid-change (docs/design.md 4).
    function Get-SiblingAt([string] $Sibling, [string] $Rev, [string] $RelativePath) {
        $text = & git -C (Join-Path $Code $Sibling) show "${Rev}:$RelativePath" 2>$null
        if ($LASTEXITCODE -ne 0) { throw "git show ${Rev}:$RelativePath failed in $Sibling" }
        ($text -join "`n") + "`n"
    }
    function Get-GraphNodeAtTag([string] $RelativePath) { Get-SiblingAt 'Graph.Node' $GraphNodeTag $RelativePath }
    function Get-Lf([string] $Path) { (Get-Content -LiteralPath $Path -Raw -Encoding utf8) -replace "`r`n", "`n" }
    # One raw call: status, headers and parsed body, never a throw.
    function Send([string] $Url, [string] $Method, [string] $Path, $Body) {
        $req = @{ Uri = "$Url$Path"; Method = $Method; SkipHttpErrorCheck = $true }
        if ($null -ne $Body) { $req.ContentType = 'application/json'; $req.Body = ($Body -is [string]) ? $Body : ($Body | ConvertTo-Json -Depth 20 -Compress) }
        $r = Invoke-WebRequest @req
        [pscustomobject]@{ Status = [int]$r.StatusCode; Headers = $r.Headers; Body = ([string]$r.Content | ConvertFrom-Json -Depth 64) }
    }
}

Describe 'Module' {
    It 'is version 0.3.0 and needs PowerShell 7.4' {
        $m = Import-PowerShellDataFile $Manifest
        $m.ModuleVersion | Should -Be '0.3.0'
        $m.PowerShellVersion | Should -Be '7.4'
    }
    It 'exports exactly the functions the manifest lists' {
        $m = Import-PowerShellDataFile $Manifest
        @((Get-Command -Module Portal).Name | Sort-Object) | Should -Be @($m.FunctionsToExport | Sort-Object)
    }
    It 'has one file under Public per exported function, each defining exactly that function' {
        $public = Get-ChildItem (Join-Path $Repo 'src' 'Portal' 'Public') -Filter '*.ps1'
        @($public.BaseName | Sort-Object) | Should -Be @((Get-Command -Module Portal).Name | Sort-Object)
        foreach ($file in $public + (Get-ChildItem (Join-Path $Repo 'src' 'Portal' 'Private') -Filter '*.ps1')) {
            $ast = [System.Management.Automation.Language.Parser]::ParseFile($file.FullName, [ref]$null, [ref]$null)
            $fns = @($ast.FindAll({ param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $false))
            $fns.Count | Should -Be 1 -Because $file.Name
            $fns[0].Name | Should -BeExactly $file.BaseName
        }
    }
    It 'shares no command name with the Graph module' -Skip:(-not $HasGraph) {
        $graph = Import-PowerShellDataFile (Join-Path $Code 'Graph' 'src' 'Graph' 'Graph.psd1')
        $mine = (Import-PowerShellDataFile $Manifest).FunctionsToExport
        @($mine | Where-Object { $_ -in $graph.FunctionsToExport }) | Should -BeNullOrEmpty
    }
}

Describe 'Two doors' {
    It 'README first line links ONTOLOGY.md' {
        Get-Content (Join-Path $Repo 'README.md') -First 1 | Should -Match 'ONTOLOGY\.md'
    }
    It 'ONTOLOGY.md first line links README.md' {
        Get-Content (Join-Path $Repo 'ONTOLOGY.md') -First 1 | Should -Match 'README\.md'
    }
    It 'design.md entry 1 is the principle: everything on screen is a graph' {
        (Get-Content (Join-Path $Repo 'docs' 'design.md') -Raw) | Should -Match '(?m)^## 1\. Everything on screen is a graph'
    }
    It 'names the repo Portal: no tracked or new file outside the fixtures says the 0.1.0 name' {
        $old = 'Graph' + '.Portal'
        $files = @(& git -C $Repo ls-files --cached --others --exclude-standard) | Where-Object { $_ -notmatch '^(tests/fixtures/|web/vendor/)|\.png$' }
        $hits = foreach ($f in $files) {
            $path = Join-Path $Repo $f
            if ((Test-Path -LiteralPath $path -PathType Leaf) -and (Select-String -LiteralPath $path -SimpleMatch $old -Quiet)) { $f }
        }
        @($hits) | Should -BeNullOrEmpty
    }
}

Describe 'Skills' {
    It 'has the five skills' {
        foreach ($s in 'manual-check-list', 'ontology-doc', 'readme', 'graph-node', 'run-report') {
            Join-Path $SkillRoot $s 'SKILL.md' | Should -Exist
        }
    }
    It 'graph-node (copied from Graph.Node, read-only here) is Graph.Node''s at the pinned tag, version and files' -Skip:(-not $HasGraphNode) {
        $psd1 = Get-GraphNodeAtTag 'src/GraphNode/GraphNode.psd1'
        $tagged = [regex]::Match($psd1, "ModuleVersion\s*=\s*'([^']+)'").Groups[1].Value
        Get-FrontVersion (Join-Path $SkillRoot 'graph-node' 'SKILL.md') | Should -Be $tagged -Because 'recopy .claude/skills/graph-node from Graph.Node at the pinned tag'
        foreach ($f in @(& git -C (Join-Path $Code 'Graph.Node') ls-tree -r --name-only $GraphNodeTag .claude/skills/graph-node)) {
            Get-Lf (Join-Path $Repo $f) | Should -BeExactly (Get-GraphNodeAtTag $f) -Because $f
        }
    }
    # Since Graph 0.2.3 the skill's own version: tracks the run report's envelope and ontology, not Graph's
    # ModuleVersion (Graph design.md 9), so the copy is compared with Graph's committed skill, version and files.
    It 'run-report (maintained in Graph, read-only here) is Graph''s committed skill, version and files' -Skip:(-not $HasGraph) {
        $committed = Get-SiblingAt 'Graph' 'HEAD' '.claude/skills/run-report/SKILL.md'
        $version = [regex]::Match($committed, '(?m)^version:\s*(\S+)\s*$').Groups[1].Value
        Get-FrontVersion (Join-Path $SkillRoot 'run-report' 'SKILL.md') | Should -Be $version -Because 'recopy .claude/skills/run-report from Graph when its committed version: changes'
        foreach ($f in @(& git -C (Join-Path $Code 'Graph') ls-tree -r --name-only HEAD .claude/skills/run-report)) {
            Get-Lf (Join-Path $Repo $f) | Should -BeExactly (Get-SiblingAt 'Graph' 'HEAD' $f) -Because $f
        }
    }
    It 'run-report reference ontology passes graphnode validate-ontology' {
        $out = & $GraphNodeBin validate-ontology (Join-Path $SkillRoot 'run-report' 'reference' 'ontology.yaml')
        $LASTEXITCODE | Should -Be 0 -Because ($out -join ' ')
    }
}

Describe 'Lifted Graph.Node' -Skip:(-not $HasGraphNode) {
    It 'schemas are Graph.Node''s at the pinned tag, byte for byte but for the registered ui: row' {
        $files = @(& git -C (Join-Path $Code 'Graph.Node') ls-tree -r --name-only $GraphNodeTag schemas)
        $files.Count | Should -BeGreaterThan 0
        foreach ($f in $files) {
            $mine = Join-Path $Repo 'internal' 'graphnode' $f
            $mine | Should -Exist
            $want = Get-GraphNodeAtTag $f
            if ($Registered[$f]) { $want = $want.Replace($Registered[$f][0], $Registered[$f][1]) }
            Get-Lf $mine | Should -BeExactly $want -Because $f
        }
    }
    It 'graph/ and cmd/graphnode differ from Graph.Node''s at the pinned tag only in the import path and the registered ui: row' {
        $graphFiles = @(& git -C (Join-Path $Code 'Graph.Node') ls-tree -r --name-only $GraphNodeTag graph) | Where-Object { $_ -notlike '*_test.go' }
        @((Get-ChildItem (Join-Path $Repo 'internal' 'graphnode' 'graph') -Filter '*.go').Name | Sort-Object) |
            Should -Be @($graphFiles | ForEach-Object { Split-Path $_ -Leaf } | Sort-Object)
        foreach ($f in @('cmd/graphnode/main.go') + $graphFiles) {
            $to = if ($f -like 'cmd/*') { Join-Path $Repo $f } else { Join-Path $Repo 'internal' 'graphnode' $f }
            $want = (Get-GraphNodeAtTag $f) -replace 'github\.com/JerryBalmer1/GraphNode/', 'github.com/JerryBalmer1/Portal/internal/graphnode/'
            if ($Registered[$f]) { $want = $want.Replace($Registered[$f][0], $Registered[$f][1]) }
            Get-Lf $to | Should -BeExactly $want -Because $to
        }
    }
    It 'registers ui: as Portal''s own' {
        $prefixes = & $GraphNodeBin prefixes | ConvertFrom-Json
        ($prefixes | Where-Object prefix -EQ 'ui:').owner | Should -Be 'Portal'
    }
    It 'tests/fixtures/invalid.graph.json is Graph.Node''s at the pinned tag' {
        Get-Lf (Join-Path $Fixtures 'invalid.graph.json') | Should -BeExactly (Get-GraphNodeAtTag 'tests/fixtures/invalid.graph.json')
    }
}

Describe 'Ontology and fixtures' {
    It 'ontology.yaml passes graphnode validate-ontology' {
        $out = & $GraphNodeBin validate-ontology (Join-Path $Repo 'ontology.yaml')
        $LASTEXITCODE | Should -Be 0 -Because ($out -join ' ')
    }
    It 'declares module Portal 0.3.0, the seven Kinds, the seven edge Kinds and the approved finding Kind' {
        $text = Get-Content (Join-Path $Repo 'ontology.yaml') -Raw
        $text | Should -Match '(?m)^module: Portal$'
        $text | Should -Match '(?m)^version: 0\.3\.0$'
        foreach ($k in 'Container', 'Widget', 'Layout', 'Category', 'State', 'Theme', 'Ref') { $text | Should -Match "(?m)^  ${k}:" }
        foreach ($e in 'contains', 'laid-out-by', 'is-a', 'displays', 'has-state', 'themed-by', 'opens') { $text | Should -Match "(?m)^  ${e}:" }
        $text | Should -Match '(?m)^  approved:'
    }
    It 'the FileSystem fixture passes graphnode validate and has its ontology beside it' {
        $layer = Get-ChildItem $FsFixture -Filter '*.graph.json' | Select-Object -First 1
        & $GraphNodeBin validate $layer.FullName | Out-Null
        $LASTEXITCODE | Should -Be 0
        Join-Path $FsFixture 'ontology.yaml' | Should -Exist
    }
}

Describe 'Build output' {
    It 'is win-x64 only' {
        Join-Path $Repo 'build' 'linux-x64' | Should -Not -Exist
        (Get-Content (Join-Path $Repo '.build.ps1') -Raw) | Should -Not -Match 'linux'
    }
}

Describe 'Fixed port' {
    BeforeAll { $script:TestPort = 18200 }
    It '.env.example sets PORTAL_PORT=8200 and .env is gitignored' {
        (Get-Content (Join-Path $Repo '.env.example')) | Should -Contain 'PORTAL_PORT=8200'
        (Get-Content (Join-Path $Repo '.gitignore')) | Should -Contain '.env'
    }
    It 'resolves -Port, then PORTAL_PORT from .env, then the environment, then 8200' {
        $envFile = Join-Path $TestDrive 'port.env'
        $saved = $env:PORTAL_PORT
        try {
            InModuleScope Portal -Parameters @{ EnvFile = $envFile } {
                param($EnvFile)
                $env:PORTAL_PORT = $null
                Remove-Item -LiteralPath $EnvFile -ErrorAction SilentlyContinue
                Get-PortalPort -EnvFile $EnvFile | Should -Be 8200
                $env:PORTAL_PORT = '8300'
                Get-PortalPort -EnvFile $EnvFile | Should -Be 8300
                Set-Content -LiteralPath $EnvFile -Value "# comment`nPORTAL_PORT=8400"
                Get-PortalPort -EnvFile $EnvFile | Should -Be 8400
                Get-PortalPort -Port 8500 -EnvFile $EnvFile | Should -Be 8500
            }
        }
        finally { $env:PORTAL_PORT = $saved }
    }
    It 'refuses port 0, from -Port and from .env, and never starts the binary' {
        { Start-Portal -Port 0 -Artifacts (Join-Path $TestDrive 'zero') -Filesystem '' } | Should -Throw '*0*refused*'
        Join-Path $TestDrive 'zero' 'portal.json' | Should -Not -Exist
        $envFile = Join-Path $TestDrive 'zero.env'
        Set-Content -LiteralPath $envFile -Value 'PORTAL_PORT=0'
        InModuleScope Portal -Parameters @{ EnvFile = $envFile } { param($EnvFile) { Get-PortalPort -EnvFile $EnvFile } | Should -Throw '*refused*' }
    }
    It 'fails on a busy port naming the port and the owning process, never picking another' {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $TestPort)
        $listener.Start()
        try {
            { Start-Portal -Port $TestPort -Artifacts (Join-Path $TestDrive 'busy') -Filesystem '' } | Should -Throw "*port $TestPort is in use by pwsh (process $PID)*"
            Join-Path $TestDrive 'busy' 'portal.json' | Should -Not -Exist
        }
        finally { $listener.Stop() }
    }
    It 'the binary refuses -port 0 itself' {
        $out = & (Join-Path $Repo 'build' 'win-x64' 'portal.exe') -port 0 -artifacts (Join-Path $TestDrive 'bin0') -filesystem '' 2>&1
        $LASTEXITCODE | Should -Not -Be 0
        ($out -join ' ') | Should -Match '0 is refused'
    }
    It 'Get-PortalStatus gives the port, URL and boot stamp, running or not' {
        $work = Join-Path $TestDrive 'status'
        $idle = Get-PortalStatus -Artifacts $work
        $idle.Running | Should -BeFalse
        $idle.Port | Should -BeOfType [int]
        $p = Start-Portal -Port $TestPort -Artifacts $work -Filesystem ''
        try {
            $s = Get-PortalStatus -Artifacts $work
            $s.Running | Should -BeTrue
            $s.Port | Should -Be $TestPort
            $s.Url | Should -Be "http://127.0.0.1:$TestPort"
            $s.Boot | Should -Be 'seed'
            $s.ProcessId | Should -Be $p.ProcessId
        }
        finally { Stop-Portal -Artifacts $work }
    }
}

Describe 'Live portal' {
    BeforeAll {
        $script:Work = Join-Path $TestDrive 'live'
        $script:P = Start-Portal -Port 18200 -Artifacts $Work -Filesystem $FsFixture
        $script:Url = $P.Url
    }
    AfterAll { Stop-Portal -Artifacts $Work }

    It 'Start-Portal records the process, boots from the seed and says so' {
        Join-Path $Work 'portal.json' | Should -Exist
        $P.Boot | Should -Be 'seed'
        (Send $Url GET '/api/screen').Headers['X-Portal-Boot'] | Should -Be 'seed'
        $s = Get-PortalScreen -Url $Url
        $s.schema | Should -Be 'graph/1'
        $s.module | Should -Be 'Portal'
        $s.version | Should -Be '0.3.0'
        $s.layer | Should -BeOfType [string]
        $byId = @{}; foreach ($n in $s.nodes) { $byId[$n.id] = $n }
        $byId['ui:root'].kind | Should -Be 'Container'
        $byId['ui:layout:root'].properties.mode | Should -Be 'grid'
        $byId['ui:theme:default'].kind | Should -Be 'Theme'
        $byId['ui:canvas'].properties.source | Should -Be 'filesystem'
        @($s.nodes | Where-Object kind -EQ 'Category').name | Sort-Object | Should -Be @('Canvas', 'Designer', 'Detail', 'Menu', 'Table')
        $s.edges.id | Should -Contain 'ui:canvas|ui:category:Canvas|is-a'
        $s.edges.id | Should -Contain 'ui:root|ui:theme:default|themed-by'
        Join-Path $Work 'state' | Should -Not -Exist
    }
    It 'seeds the menu as nodes: one Container "Graph" that is-a Menu and opens the Canvas, contained by nothing' {
        $s = Get-PortalScreen -Url $Url
        $entry = $s.nodes | Where-Object id -EQ 'ui:menu:graph'
        $entry.kind | Should -Be 'Container'
        $entry.name | Should -Be 'Graph'
        $s.edges.id | Should -Contain 'ui:menu:graph|ui:category:Menu|is-a'
        $s.edges.id | Should -Contain 'ui:menu:graph|ui:canvas|opens'
        @($s.edges | Where-Object { $_.kind -eq 'contains' -and $_.to -eq 'ui:menu:graph' }) | Should -BeNullOrEmpty
        (Send $Url POST '/api/edges' @{ kind = 'opens'; from = 'ui:menu:graph'; to = 'ui:layout:root' }).Body.problems.code | Should -Contain 'edge-endpoints'
    }
    It 'refuses a POST with display properties on a plain node (display lives in State and Theme nodes only)' {
        $r = Send $Url POST '/api/nodes' @{ id = 'ui:w1'; kind = 'Widget'; properties = @{ color = '#ff0000' } }
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'display'
        $r = Send $Url POST '/api/nodes' '{"id":"ui:w1","kind":"Widget","display":{"color":"#ff0000"}}'
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'schema'
        (Send $Url POST '/api/nodes' @{ id = 'ui:theme:dark'; kind = 'Theme'; properties = @{ color = '#203040'; opacity = 0.2 } }).Status | Should -Be 201
        $r = Send $Url POST '/api/nodes' @{ id = 'ui:theme:bad'; kind = 'Theme'; properties = @{ opacity = 2 } }
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'property-type'
    }
    It 'refuses an is-a edge to an unknown Category' {
        (Send $Url POST '/api/nodes' @{ id = 'ui:w2'; kind = 'Widget' }).Status | Should -Be 201
        $r = Send $Url POST '/api/edges' @{ kind = 'is-a'; from = 'ui:w2'; to = 'ui:category:Nope' }
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'unknown-category'
        $r.Body.problems.severity | Should -Contain 'error'
    }
    It 'returns the validated node from every write path' {
        $n = Send $Url POST '/api/nodes' @{ id = 'ui:w3'; kind = 'Widget'; name = 'w3'; properties = @{ type = 'Table'; source = 'filesystem?kind=File' } }
        $n.Status | Should -Be 201
        $n.Body.id | Should -Be 'ui:w3'
        $e = Send $Url POST '/api/edges' @{ kind = 'contains'; from = 'ui:root'; to = 'ui:w3' }
        $e.Body.id | Should -Be 'ui:root|ui:w3|contains'
        $p = Send $Url PATCH '/api/nodes/ui:w3' @{ state = @{ collapsed = $true } }
        $p.Body.node.id | Should -Be 'ui:w3'
        $p.Body.state.id | Should -Be 'ui:state:w3'
        $d = Send $Url DELETE '/api/nodes/ui:w3'
        $d.Body.node.id | Should -Be 'ui:w3'
        $d.Body.edges | Should -Contain 'ui:root|ui:w3|contains'
    }
    It 'New-PortalSnap writes a snap that round-trips through graphnode validate and carries its note' {
        $snap = New-PortalSnap -Url $Url -Note 'round trip'
        $snap.path | Should -Exist
        $snap.note | Should -Be 'round trip'
        & $GraphNodeBin validate $snap.path | Out-Null
        $LASTEXITCODE | Should -Be 0
        $onDisk = Get-Content -LiteralPath $snap.path -Raw | ConvertFrom-Json -Depth 64
        ($onDisk.nodes | ConvertTo-Json -Depth 64 -Compress) | Should -Be ((Get-PortalScreen -Url $Url).nodes | ConvertTo-Json -Depth 64 -Compress)
        $onDisk.findings.kind | Should -Be 'approved'
        $onDisk.findings.message | Should -Be 'round trip'
    }
    It 'Compare-PortalSnap of identical snaps is empty' {
        $a = New-PortalSnap -Url $Url
        $d = Compare-PortalSnap $a $a -Url $Url
        @($d.added.nodes).Count + @($d.added.edges).Count + @($d.removed.nodes).Count + @($d.removed.edges).Count + @($d.changed.nodes).Count + @($d.changed.edges).Count | Should -Be 0
    }
    It 'an approval after one Theme PATCH reports exactly the Theme node against the snap before it' {
        $a = New-PortalSnap -Url $Url
        (Send $Url PATCH '/api/nodes/ui:theme:default' @{ properties = @{ opacity = 0.25 } }).Status | Should -Be 200
        $b = New-PortalSnap -Url $Url -Note 'lighter'
        $b.previous | Should -Be $a.stamp
        @($b.diff.changed.nodes) | Should -Be @('ui:theme:default')
        @($b.diff.added.nodes) + @($b.diff.added.edges) + @($b.diff.removed.nodes) + @($b.diff.removed.edges) + @($b.diff.changed.edges) | Should -BeNullOrEmpty
        $d = Compare-PortalSnap $a.stamp $b.stamp -Url $Url
        @($d.changed.nodes).Count | Should -Be 1
        $d.changed.nodes[0].id | Should -Be 'ui:theme:default'
    }
    It 'Mount-PortalLayer refuses a layer that fails validation, with the problem codes' {
        $err = $null
        try { Mount-PortalLayer -Name bad -Path (Join-Path $Fixtures 'invalid.graph.json') -Url $Url } catch { $err = $_ }
        $err | Should -Not -BeNullOrEmpty
        $codes = @($err.TargetObject.problems.code)
        foreach ($c in 'schema', 'duplicate-node', 'dangling-edge', 'edge-id', 'edgeid', 'kind', 'stamp') { $codes | Should -Contain $c }
        (Invoke-RestMethod "$Url/api/layers").name | Should -Not -Contain 'bad'
    }
    It 'Mount-PortalLayer mounts by path and by envelope; the layer ontology is served' {
        $l = Mount-PortalLayer -Name fs2 -Path $FsFixture -Url $Url
        $l.hasOntology | Should -BeTrue
        $l.counts.nodes | Should -BeGreaterThan 0
        $inline = Mount-PortalLayer -Name inline -Envelope '{"schema":"graph/1","module":"M","layer":"2026-10-09T00:00:00Z","nodes":[{"id":"a","kind":"K"}],"edges":[]}' -Url $Url
        $inline.counts.nodes | Should -Be 1
        $inline.hasOntology | Should -BeFalse
        (Invoke-RestMethod "$Url/api/ontology").layers.fs2.kinds.Directory.display.color | Should -Not -BeNullOrEmpty
    }
    It 'never writes to a mounted layer' {
        $before = Get-ChildItem $FsFixture -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName).Hash)" }
        $null = Invoke-RestMethod "$Url/api/layers/filesystem/nodes?kind=File"
        $null = New-PortalSnap -Url $Url
        Get-ChildItem $FsFixture -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName).Hash)" } | Should -Be $before
    }
}

Describe 'Boot order' {
    BeforeAll {
        $script:BootWork = Join-Path $TestDrive 'boot'
        $p = Start-Portal -Port 18200 -Artifacts $BootWork -Filesystem ''
        try {
            $script:First = New-PortalSnap -Url $p.Url -Note 'first'
            $null = Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/nodes/ui:theme:default" -ContentType 'application/json' -Body '{"properties":{"color":"#336699"}}'
            $script:Second = New-PortalSnap -Url $p.Url -Note 'second'
            # Not approved: gone after a restart.
            $null = Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/nodes/ui:theme:default" -ContentType 'application/json' -Body '{"properties":{"color":"#000000"}}'
        }
        finally { Stop-Portal -Artifacts $BootWork }
    }
    AfterEach { Stop-Portal -Artifacts $BootWork }

    It 'a restart boots from the newest snap, without the unapproved change' {
        $p = Start-Portal -Port 18200 -Artifacts $BootWork -Filesystem ''
        $p.Boot | Should -Be $Second.stamp
        ((Get-PortalScreen -Url $p.Url).nodes | Where-Object id -EQ 'ui:theme:default').properties.color | Should -Be '#336699'
    }
    It 'Start-Portal -Snap (PORTAL_SNAP) forces an older snap' {
        $p = Start-Portal -Port 18200 -Artifacts $BootWork -Filesystem '' -Snap $First.stamp
        $p.Boot | Should -Be $First.stamp
        ((Get-PortalScreen -Url $p.Url).nodes | Where-Object id -EQ 'ui:theme:default').properties.color | Should -Be '#f09648'
    }
    It 'a PORTAL_SNAP that names no snap stops the binary' {
        { Start-Portal -Port 18200 -Artifacts $BootWork -Filesystem '' -Snap '2000-01-01T00-00-00.000Z' -TimeoutSec 5 } | Should -Throw '*no snap 2000-01-01T00-00-00.000Z*'
    }
}

Describe 'Production mode' {
    # One test port: the development screen is read and stopped before the production one starts.
    BeforeAll {
        $dev = Start-Portal -Port 18200 -Artifacts (Join-Path $TestDrive 'dev') -Filesystem $FsFixture
        try { $script:Full = Get-PortalScreen -Url $dev.Url } finally { Stop-Portal -Artifacts (Join-Path $TestDrive 'dev') }
        $script:Prod = Start-Portal -Port 18200 -Artifacts (Join-Path $TestDrive 'prod') -Filesystem $FsFixture -Production
    }
    AfterAll {
        Stop-Portal -Artifacts (Join-Path $TestDrive 'prod')
    }
    It 'hides the Designer and only the Designer' {
        $full = $Full
        $shown = Get-PortalScreen -Url $Prod.Url
        @($full.nodes.id | Where-Object { $_ -notin $shown.nodes.id }) | Should -Be @('ui:designer')
        @($shown.edges | Where-Object { $_.from -eq 'ui:designer' -or $_.to -eq 'ui:designer' }).Count | Should -Be 0
        $Prod.Mode | Should -Be 'production'
    }
    It 'serves a screen that still passes graphnode validate' {
        $f = Join-Path $TestDrive 'prod-screen.json'
        Get-PortalScreen -Url $Prod.Url | ConvertTo-Json -Depth 64 | Set-Content -LiteralPath $f -Encoding utf8NoBOM
        & $GraphNodeBin validate $f | Out-Null
        $LASTEXITCODE | Should -Be 0
    }
}
