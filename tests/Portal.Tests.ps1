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
    $script:GraphNodeTag = 'v0.2.3'
    # Edits the lift carries, file => (upstream text, Portal's text). Up to v0.2.1 the ui: row of the id prefix
    # registry was one (docs/design.md 4, 31); since v0.2.3 Graph.Node registers ui: to Portal itself, so the lift is
    # upstream's but for the import path (docs/design.md 34). A new edit goes here and in design.md.
    $script:Registered = @{}

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
    # A tenant directory for every portal the tests start (PORTAL_TENANTS, which Start-Portal reads): the committed
    # empty overlay and one principal, "pester", the first admin (New-PortalToken -Bootstrap), whose token is
    # PORTAL_TOKEN, which the module's calls send. Outside the repo, so the repo's own gitignored principals.yaml and
    # tokens.yaml are never touched.
    $script:TenantsDir = Join-Path ([IO.Path]::GetTempPath()) "Portal-tenants-$([guid]::NewGuid())"
    $null = New-Item -ItemType Directory -Force -Path (Join-Path $TenantsDir 'default')
    Copy-Item -LiteralPath (Join-Path $Repo 'tenants' 'default' 'portal.yaml') -Destination (Join-Path $TenantsDir 'default')
    $script:Saved = @{ PORTAL_TENANTS = $env:PORTAL_TENANTS; PORTAL_TOKEN = $env:PORTAL_TOKEN; PORTAL_TENANT = $env:PORTAL_TENANT }
    $env:PORTAL_TENANTS = $TenantsDir
    $env:PORTAL_TENANT = $null
    $env:PORTAL_TOKEN = (New-PortalToken -Name pester -Bootstrap).Token
    $script:Auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }

    # One raw call with the test token: status, headers and parsed body, never a throw.
    function Send([string] $Url, [string] $Method, [string] $Path, $Body) {
        $req = @{ Uri = "$Url$Path"; Method = $Method; SkipHttpErrorCheck = $true; Headers = $Auth }
        if ($null -ne $Body) { $req.ContentType = 'application/json'; $req.Body = ($Body -is [string]) ? $Body : ($Body | ConvertTo-Json -Depth 20 -Compress) }
        $r = Invoke-WebRequest @req
        [pscustomobject]@{ Status = [int]$r.StatusCode; Headers = $r.Headers; Body = ([string]$r.Content | ConvertFrom-Json -Depth 64) }
    }
    # The Designer's sections in order: slug, heading, open on seed (docs/design.md 28).
    $script:Sections = @(
        @{ Slug = 'approve'; Title = 'Approve'; Open = $true }
        @{ Slug = 'kinds'; Title = 'Kinds (ontology.yaml)'; Open = $false }
        @{ Slug = 'categories'; Title = 'Categories'; Open = $true }
        @{ Slug = 'add-panel'; Title = 'Add Panel'; Open = $false }
        @{ Slug = 'add-container'; Title = 'Add Container'; Open = $false }
        @{ Slug = 'add-category'; Title = 'Add Category'; Open = $false }
        @{ Slug = 'add-menu-entry'; Title = 'Add Menu Entry'; Open = $false }
    )
    # The page as headless Chrome draws it (--dump-dom; Edge's new headless prints none on Windows), or $null without Chrome.
    function Get-PageDom([string] $Url, [string] $Route = '/') {
        $chrome = @("$env:ProgramFiles\Google\Chrome\Application\chrome.exe", "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
            "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe") | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
        if (-not $chrome) { return $null }
        $work = Join-Path $TestDrive "dom-$([guid]::NewGuid())"
        $null = New-Item -ItemType Directory -Force -Path $work
        $argv = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', '--virtual-time-budget=8000', '--window-size=1400,900', '--dump-dom', "$Url$Route")
        $null = Start-Process -FilePath $chrome -ArgumentList $argv -Wait -NoNewWindow -PassThru -RedirectStandardOutput (Join-Path $work 'dom.html') -RedirectStandardError (Join-Path $work 'err.txt')
        [string](Get-Content -LiteralPath (Join-Path $work 'dom.html') -Raw -ErrorAction SilentlyContinue)
    }
    # Drives headless Chrome over the DevTools protocol, which --dump-dom cannot: opens $Url/static/base.css to put
    # $Token in the tab's sessionStorage (where the page keeps it), opens $Url$Route, runs $Script (an expression, a
    # promise awaited) in the page and returns its value. $null without Chrome.
    function Invoke-PageScript([string] $Url, [string] $Route, [string] $Token, [string] $Script) {
        $chrome = @("$env:ProgramFiles\Google\Chrome\Application\chrome.exe", "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe",
            "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe") | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
        if (-not $chrome) { return $null }
        $work = Join-Path $TestDrive "cdp-$([guid]::NewGuid())"
        $null = New-Item -ItemType Directory -Force -Path $work
        $port = 18203
        $argv = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', "--remote-debugging-port=$port", '--window-size=1400,900', 'about:blank')
        $proc = Start-Process -FilePath $chrome -ArgumentList $argv -PassThru -RedirectStandardOutput (Join-Path $work 'out.txt') -RedirectStandardError (Join-Path $work 'err.txt')
        $ws = [System.Net.WebSockets.ClientWebSocket]::new()
        $cts = [Threading.CancellationTokenSource]::new([TimeSpan]::FromSeconds(60))
        $state = @{ id = 0 }
        $buf = [byte[]]::new(1MB)
        # One DevTools command; with -Event, also waits for that event (it may come before or after the answer).
        $call = {
            param([string] $Method, [hashtable] $Params = @{}, [string] $Event)
            $state.id++
            $msg = [Text.Encoding]::UTF8.GetBytes((@{ id = $state.id; method = $Method; params = $Params } | ConvertTo-Json -Depth 10 -Compress))
            $null = $ws.SendAsync([ArraySegment[byte]]::new($msg), 'Text', $true, $cts.Token).GetAwaiter().GetResult()
            $answer = $null
            $seen = -not $Event
            while (-not $answer -or -not $seen) {
                $sb = [Text.StringBuilder]::new()
                do {
                    $r = $ws.ReceiveAsync([ArraySegment[byte]]::new($buf), $cts.Token).GetAwaiter().GetResult()
                    $null = $sb.Append([Text.Encoding]::UTF8.GetString($buf, 0, $r.Count))
                } until ($r.EndOfMessage)
                $m = $sb.ToString() | ConvertFrom-Json -Depth 64
                if ($m.PSObject.Properties['id'] -and $m.id -eq $state.id) {
                    if ($m.PSObject.Properties['error']) { throw "DevTools $Method failed: $($m.error.message)" }
                    $answer = $m
                }
                elseif ($Event -and $m.PSObject.Properties['method'] -and $m.method -eq $Event) { $seen = $true }
            }
            $answer.result
        }
        try {
            $page = $null
            $deadline = [datetime]::UtcNow.AddSeconds(20)
            while (-not $page -and [datetime]::UtcNow -lt $deadline) {
                # Invoke-RestMethod hands back the JSON list as one object: unroll it before filtering.
                try { $list = Invoke-RestMethod "http://127.0.0.1:$port/json/list" -TimeoutSec 2; $page = $list | Where-Object type -EQ 'page' | Select-Object -First 1 }
                catch { Start-Sleep -Milliseconds 200 }
            }
            if (-not $page) { throw "Chrome's DevTools did not answer on port $port" }
            $null = $ws.ConnectAsync([uri]$page.webSocketDebuggerUrl, $cts.Token).GetAwaiter().GetResult()
            $null = & $call 'Page.enable'
            $null = & $call 'Page.navigate' @{ url = "$Url/static/base.css" } 'Page.loadEventFired'
            $null = & $call 'Runtime.evaluate' @{ expression = "sessionStorage.setItem('portal.token', '$Token')" }
            $null = & $call 'Page.navigate' @{ url = "$Url$Route" } 'Page.loadEventFired'
            $r = & $call 'Runtime.evaluate' @{ expression = $Script; awaitPromise = $true; returnByValue = $true }
            if ($r.PSObject.Properties['exceptionDetails']) { throw "page script failed: $($r.exceptionDetails.exception.description)" }
            $r.result.value
        }
        finally {
            try { $null = & $call 'Browser.close' } catch { }
            $ws.Dispose()
            if (-not $proc.WaitForExit(5000)) { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue }
        }
    }
}

AfterAll {
    foreach ($k in $Saved.Keys) { [Environment]::SetEnvironmentVariable($k, $Saved[$k]) }
    Remove-Item -LiteralPath $TenantsDir -Recurse -Force -ErrorAction SilentlyContinue
}

Describe 'Module' {
    It 'is version 0.5.0 and needs PowerShell 7.4' {
        $m = Import-PowerShellDataFile $Manifest
        $m.ModuleVersion | Should -Be '0.5.0'
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
        $files = @(& git -C $Repo ls-files --cached --others --exclude-standard) | Where-Object { $_ -notmatch '^(tests/fixtures/|web/vendor/|\.claude/skills/graph-node/)|\.png$' }
        $hits = foreach ($f in $files) {
            $path = Join-Path $Repo $f
            if ((Test-Path -LiteralPath $path -PathType Leaf) -and (Select-String -LiteralPath $path -SimpleMatch $old -Quiet)) { $f }
        }
        @($hits) | Should -BeNullOrEmpty
    }
}

Describe 'Skills' {
    It 'has the six skills' {
        foreach ($s in 'manual-check-list', 'ontology-doc', 'readme', 'graph-node', 'run-report', 'git-guard') {
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
    # git-guard follows the same rule since 0.3.2 (docs/design.md 30). graph-node was checked here too from 0.3.3 to
    # 0.3.5 (docs/design.md 31); since 0.3.6 it follows Graph.Node's pinned tag, which Graph's copy may lag behind but
    # never lead (docs/design.md 34).
    It 'graph-node is never older than Graph''s committed copy' -Skip:(-not $HasGraph) {
        $committed = Get-SiblingAt 'Graph' 'HEAD' '.claude/skills/graph-node/SKILL.md'
        $theirs = [regex]::Match($committed, '(?m)^version:\s*(\S+)\s*$').Groups[1].Value
        $theirs | Should -Not -BeNullOrEmpty
        [version](Get-FrontVersion (Join-Path $SkillRoot 'graph-node' 'SKILL.md')) | Should -BeGreaterOrEqual ([version]$theirs) -Because 'Graph has a newer graph-node: name a newer Graph.Node tag and recopy'
    }
    It '<_> (maintained in Graph, read-only here) is Graph''s committed skill, version and files' -Skip:(-not $HasGraph) -ForEach 'run-report', 'git-guard' {
        $committed = Get-SiblingAt 'Graph' 'HEAD' ".claude/skills/$_/SKILL.md"
        $version = [regex]::Match($committed, '(?m)^version:\s*(\S+)\s*$').Groups[1].Value
        $version | Should -Not -BeNullOrEmpty
        Get-FrontVersion (Join-Path $SkillRoot $_ 'SKILL.md') | Should -Be $version -Because "recopy .claude/skills/$_ from Graph when its committed version: changes"
        $files = @(& git -C (Join-Path $Code 'Graph') ls-tree -r --name-only HEAD ".claude/skills/$_")
        $files.Count | Should -BeGreaterThan 0
        foreach ($f in $files) {
            Get-Lf (Join-Path $Repo $f) | Should -BeExactly (Get-SiblingAt 'Graph' 'HEAD' $f) -Because $f
        }
        @(Get-ChildItem -LiteralPath (Join-Path $SkillRoot $_) -Recurse -File).Count | Should -Be $files.Count -Because "the copy holds only Graph's committed files"
    }
    It '.claude/settings.json wires the git-guard hook for Bash and PowerShell' {
        $settings = Get-Content -LiteralPath (Join-Path $Repo '.claude' 'settings.json') -Raw | ConvertFrom-Json
        $fragment = Get-Content -LiteralPath (Join-Path $SkillRoot 'git-guard' 'settings.fragment.json') -Raw | ConvertFrom-Json
        $want = $fragment.hooks.PreToolUse[0]
        $entry = @($settings.hooks.PreToolUse | Where-Object { $_.matcher -eq $want.matcher -and $_.hooks.command -contains $want.hooks[0].command })
        $entry.Count | Should -Be 1
    }
    It 'git-guard hook exits <Code> for <Command>' -ForEach @(
        @{ Command = 'git status'; Code = 0 }
        @{ Command = 'git add -A'; Code = 0 }
        @{ Command = 'git commit -m x'; Code = 2 }
        @{ Command = 'git push'; Code = 2 }
        @{ Command = 'git tag v1'; Code = 2 }
        @{ Command = 'gh pr merge 1'; Code = 2 }
    ) {
        $json = @{ tool_name = 'Bash'; tool_input = @{ command = $Command } } | ConvertTo-Json -Compress
        $err = $json | & pwsh -NoProfile -File (Join-Path $SkillRoot 'git-guard' 'hook.ps1') 2>&1
        $LASTEXITCODE | Should -Be $Code
        if ($Code -eq 2) { ($err -join ' ') | Should -Match '^git-guard:' }
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
    It 'declares module Portal 0.5.0, the eleven Kinds, the eight edge Kinds and the approved finding Kind' {
        $text = Get-Content (Join-Path $Repo 'ontology.yaml') -Raw
        $text | Should -Match '(?m)^module: Portal$'
        $text | Should -Match '(?m)^version: 0\.5\.0$'
        foreach ($k in 'Container', 'Widget', 'Layout', 'Category', 'State', 'Theme', 'Ref', 'Mount', 'Service', 'Page', 'Principal') { $text | Should -Match "(?m)^  ${k}:" }
        foreach ($e in 'contains', 'laid-out-by', 'is-a', 'displays', 'has-state', 'themed-by', 'opens', 'has-page') { $text | Should -Match "(?m)^  ${e}:" }
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

# Invoke-Build Up's start, Invoke-PortalUp, on the test port over its own artifacts (docs/design.md 32).
Describe 'Up replaces a running portal' {
    BeforeAll { $script:UpWork = Join-Path $TestDrive 'up' }
    AfterEach { Stop-Portal -Artifacts $UpWork }
    It 'Up stops the running portal, waits for the port and starts another, naming both boot stamps' {
        $first = Start-Portal -Port 18200 -Artifacts $UpWork -Filesystem ''
        $snap = New-PortalSnap -Url $first.Url -Note 'up: replace'
        $p = InModuleScope Portal -Parameters @{ Work = $UpWork } { param($Work) Invoke-PortalUp -Port 18200 -Artifacts $Work -Filesystem '' }
        $p.Replaced | Should -Be 'seed'
        $p.Boot | Should -Be $snap.stamp
        $p.ProcessId | Should -Not -Be $first.ProcessId
        Get-Process -Id $first.ProcessId -ErrorAction SilentlyContinue | Should -BeNullOrEmpty
        (Get-PortalStatus -Artifacts $UpWork).ProcessId | Should -Be $p.ProcessId
    }
    It 'Up with nothing running starts one and replaces nothing' {
        $p = InModuleScope Portal -Parameters @{ Work = $UpWork } { param($Work) Invoke-PortalUp -Port 18200 -Artifacts $Work -Filesystem '' }
        $p.Replaced | Should -BeNullOrEmpty
        (Get-PortalStatus -Artifacts $UpWork).Running | Should -BeTrue
    }
    It '-NoReplace keeps the throw and leaves the running portal alone' {
        $first = Start-Portal -Port 18200 -Artifacts $UpWork -Filesystem ''
        { InModuleScope Portal -Parameters @{ Work = $UpWork } { param($Work) Invoke-PortalUp -Port 18200 -Artifacts $Work -Filesystem '' -NoReplace } } |
            Should -Throw '*already running*Stop-Portal first*'
        Get-Process -Id $first.ProcessId -ErrorAction SilentlyContinue | Should -Not -BeNullOrEmpty
        (Get-PortalStatus -Artifacts $UpWork).ProcessId | Should -Be $first.ProcessId
    }
    It 'throws when the port is still held after the wait, and starts nothing' {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 18200)
        $listener.Start()
        try {
            InModuleScope Portal {
                Mock Get-PortalStatus { [pscustomobject]@{ Running = $true; Port = 18200; Boot = 'seed' } }
                Mock Stop-Portal { }
                Mock Start-Portal { throw 'Start-Portal must not run' }
                $t = [Diagnostics.Stopwatch]::StartNew()
                { Invoke-PortalUp -Port 18200 -Artifacts (Join-Path $TestDrive 'held') -FreeTimeoutSec 1 } |
                    Should -Throw "*port 18200 is still held 1 s after Stop-Portal*in use by pwsh (process $PID)*"
                $t.Elapsed.TotalSeconds | Should -BeLessThan 5
                Should -Invoke Stop-Portal -Times 1 -Exactly
                Should -Invoke Start-Portal -Times 0 -Exactly
            }
        }
        finally { $listener.Stop() }
    }
    It 'waits 10 s by default and Invoke-Build Up passes -NoReplace and logs the replacement' {
        InModuleScope Portal { (Get-Command Invoke-PortalUp).Parameters['FreeTimeoutSec'] } | Should -Not -BeNullOrEmpty
        (Get-Content (Join-Path $Repo 'src' 'Portal' 'Private' 'Invoke-PortalUp.ps1') -Raw) | Should -Match '\[int\] \$FreeTimeoutSec = 10\b'
        $build = Get-Content (Join-Path $Repo '.build.ps1') -Raw
        $build | Should -Match '\[switch\] \$NoReplace'
        $build | Should -Match 'Invoke-PortalUp'
        $build | Should -Match '"replaced \$\(\$p\.Replaced\) with \$\(\$p\.Boot\)"'
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
        (Send $Url GET '/api/tenants/default/screen').Headers['X-Portal-Boot'] | Should -Be 'seed'
        $s = Get-PortalScreen -Url $Url
        $s.schema | Should -Be 'graph/1'
        $s.module | Should -Be 'Portal'
        $s.version | Should -Be '0.5.0'
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
    It 'seeds the menu as nodes: Containers "Graph" and "Runs" that are-a Menu and open their Canvases, contained by nothing' {
        $s = Get-PortalScreen -Url $Url
        foreach ($m in @(
                @{ Id = 'ui:menu:graph'; Name = 'Graph'; Order = 0; Opens = 'ui:canvas'; Source = 'filesystem' }
                @{ Id = 'ui:menu:runs'; Name = 'Runs'; Order = 1; Opens = 'ui:canvas:agent-runs'; Source = 'agent-runs' })) {
            $entry = $s.nodes | Where-Object id -EQ $m.Id
            $entry.kind | Should -Be 'Container'
            $entry.name | Should -Be $m.Name
            $entry.properties.order | Should -Be $m.Order
            $s.edges.id | Should -Contain "$($m.Id)|ui:category:Menu|is-a"
            $s.edges.id | Should -Contain "$($m.Id)|$($m.Opens)|opens"
            @($s.edges | Where-Object { $_.kind -eq 'contains' -and $_.to -eq $m.Id }) | Should -BeNullOrEmpty
            $target = $s.nodes | Where-Object id -EQ $m.Opens
            $target.properties.type | Should -Be 'Canvas'
            $target.properties.source | Should -Be $m.Source
            $s.edges.id | Should -Contain "ui:root|$($m.Opens)|contains"
        }
        # The seed records no Mount: agent-runs is drawn once Claude.Agent mounts it (docs/design.md 35).
        @($s.nodes | Where-Object kind -EQ 'Mount') | Should -BeNullOrEmpty
        (Send $Url POST '/api/tenants/default/edges' @{ kind = 'opens'; from = 'ui:menu:graph'; to = 'ui:layout:root' }).Body.problems.code | Should -Contain 'edge-endpoints'
    }
    It 'seeds a State per Designer section, joined by has-state: Approve and Categories open, the rest collapsed' {
        $s = Get-PortalScreen -Url $Url
        foreach ($sec in $Sections) {
            $id = "ui:state:designer:$($sec.Slug)"
            $n = $s.nodes | Where-Object id -EQ $id
            $n.kind | Should -Be 'State'
            $n.properties.collapsed | Should -Be (-not $sec.Open) -Because $id
            $s.edges.id | Should -Contain "ui:designer|$id|has-state"
        }
    }
    It 'draws the seven Designer sections with Title Case headings and buttons, collapsed as their States say' {
        $dom = Get-PageDom $Url '/default/designer/sections'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $designer = [regex]::Match($dom, '<div class="pt-designer">.*', 'Singleline').Value
        @([regex]::Matches($designer, '<h4>([^<]*)</h4>') | ForEach-Object { $_.Groups[1].Value }) | Should -Be @($Sections.Title)
        @([regex]::Matches($designer, '<button class="gr-btn" type="submit"[^>]*>([^<]*)</button>') | ForEach-Object { $_.Groups[1].Value }) |
            Should -Be @('Approve', 'Add Panel', 'Add Container', 'Add Category', 'Add Menu Entry')
        # Headless Chrome reads as anonymous: every write button is disabled with the read-only tooltip (docs/design.md 37).
        @([regex]::Matches($designer, '<button [^>]*>') | Where-Object { $_.Value -notmatch '\bdisabled\b' -or $_.Value -notmatch 'title="sign in to write"' }) |
            Should -BeNullOrEmpty
        foreach ($sec in $Sections) {
            $tag = [regex]::Match($designer, "<section class=""([^""]*)"" data-section=""$($sec.Slug)"">").Groups[1].Value
            ($tag -split ' ') -contains 'pt-collapsed' | Should -Be (-not $sec.Open) -Because $sec.Slug
        }
        # Field labels stay lower case.
        $designer | Should -Match '<label class="gr-form-row">name<'
    }
    It 'heads Graph/Explore with its Canvas''s layer, the screen graph''s counts in a badge beside' {
        $dom = Get-PageDom $Url '/default/graph/explore'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $fs = (Invoke-WebRequest "$Url/api/tenants/default/layers").Content | ConvertFrom-Json -DateKind String | Where-Object name -EQ 'filesystem'
        $s = Get-PortalScreen -Url $Url
        $stats = [regex]::Match($dom, '<span class="pt-mode" id="stats" data-layer="([^"]*)"[^>]*>([^<]*)</span>')
        $stats.Groups[1].Value | Should -Be 'filesystem'
        $stats.Groups[2].Value | Should -Be "filesystem · $($fs.counts.nodes) nodes · $($fs.counts.edges) edges · $($fs.stamp)"
        $dom | Should -Match ([regex]::Escape(">screen $(@($s.nodes).Count) · $(@($s.edges).Count) · "))
    }
    It 'an anonymous collapse is 401 and the canvas still draws the graph, its collapse disabled' {
        $r = Invoke-WebRequest "$Url/api/tenants/default/nodes/ui:canvas" -Method PATCH -ContentType 'application/json' `
            -Body '{"state":{"collapsed":true}}' -SkipHttpErrorCheck
        [int]$r.StatusCode | Should -Be 401
        ($r.Content | ConvertFrom-Json).code | Should -Be 'unauthorized'
        $s = Get-PortalScreen -Url $Url
        @($s.nodes | Where-Object { $_.id -eq 'ui:state:canvas' -and $_.properties.collapsed }) | Should -BeNullOrEmpty
        $dom = Get-PageDom $Url '/default/graph/explore'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $box = [regex]::Match($dom, '<div class="gr-box pt-widget"[^>]*data-id="ui:canvas"[^>]*>.*', 'Singleline').Value
        $box | Should -Not -BeNullOrEmpty
        $box | Should -Not -Match 'pt-collapsed'
        $box | Should -Match '<button class="gr-btn" disabled="" title="sign in to write" data-readonly="true">collapse</button>'
        $drawn = [regex]::Match($box, 'data-coloured="(\d+)/(\d+)"')
        $drawn.Success | Should -BeTrue
        [int]$drawn.Groups[2].Value | Should -BeGreaterThan 0
        $box | Should -Not -Match 'class="gr-error"'
        # The base's theme.graph.wheelZoomSpeed 1.0 keeps the product's wheel step.
        $box | Should -Match 'data-wheel-sensitivity="0.2"'
    }
    It 'a section State PATCH round-trips through New-PortalSnap and back' {
        $null = New-PortalSnap -Url $Url
        (Send $Url PATCH '/api/tenants/default/nodes/ui:state:designer:kinds' @{ properties = @{ collapsed = $false } }).Status | Should -Be 200
        $snap = New-PortalSnap -Url $Url -Note 'kinds open'
        @($snap.diff.changed.nodes) | Should -Be @('ui:state:designer:kinds')
        $onDisk = Get-Content -LiteralPath $snap.path -Raw | ConvertFrom-Json -Depth 64
        ($onDisk.nodes | Where-Object id -EQ 'ui:state:designer:kinds').properties.collapsed | Should -BeFalse
        (Send $Url PATCH '/api/tenants/default/nodes/ui:state:designer:kinds' @{ properties = @{ collapsed = $true } }).Status | Should -Be 200
    }
    It 'refuses a POST with display properties on a plain node (display lives in State and Theme nodes only)' {
        $r = Send $Url POST '/api/tenants/default/nodes' @{ id = 'ui:w1'; kind = 'Widget'; properties = @{ color = '#ff0000' } }
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'display'
        $r = Send $Url POST '/api/tenants/default/nodes' '{"id":"ui:w1","kind":"Widget","display":{"color":"#ff0000"}}'
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'schema'
        (Send $Url POST '/api/tenants/default/nodes' @{ id = 'ui:theme:dark'; kind = 'Theme'; properties = @{ color = '#203040'; opacity = 0.2 } }).Status | Should -Be 201
        $r = Send $Url POST '/api/tenants/default/nodes' @{ id = 'ui:theme:bad'; kind = 'Theme'; properties = @{ opacity = 2 } }
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'property-type'
    }
    It 'refuses an is-a edge to an unknown Category' {
        (Send $Url POST '/api/tenants/default/nodes' @{ id = 'ui:w2'; kind = 'Widget' }).Status | Should -Be 201
        $r = Send $Url POST '/api/tenants/default/edges' @{ kind = 'is-a'; from = 'ui:w2'; to = 'ui:category:Nope' }
        $r.Status | Should -Be 422
        $r.Body.problems.code | Should -Contain 'unknown-category'
        $r.Body.problems.severity | Should -Contain 'error'
    }
    It 'returns the validated node from every write path' {
        $n = Send $Url POST '/api/tenants/default/nodes' @{ id = 'ui:w3'; kind = 'Widget'; name = 'w3'; properties = @{ type = 'Table'; source = 'filesystem?kind=File' } }
        $n.Status | Should -Be 201
        $n.Body.id | Should -Be 'ui:w3'
        $e = Send $Url POST '/api/tenants/default/edges' @{ kind = 'contains'; from = 'ui:root'; to = 'ui:w3' }
        $e.Body.id | Should -Be 'ui:root|ui:w3|contains'
        $p = Send $Url PATCH '/api/tenants/default/nodes/ui:w3' @{ state = @{ collapsed = $true } }
        $p.Body.node.id | Should -Be 'ui:w3'
        $p.Body.state.id | Should -Be 'ui:state:w3'
        $d = Send $Url DELETE '/api/tenants/default/nodes/ui:w3'
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
        (Send $Url PATCH '/api/tenants/default/nodes/ui:theme:default' @{ properties = @{ opacity = 0.25 } }).Status | Should -Be 200
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
        (Invoke-RestMethod "$Url/api/tenants/default/layers").name | Should -Not -Contain 'bad'
    }
    It 'Mount-PortalLayer mounts by path and by envelope; the layer ontology is served' {
        $l = Mount-PortalLayer -Name fs2 -Path $FsFixture -Url $Url
        $l.hasOntology | Should -BeTrue
        $l.counts.nodes | Should -BeGreaterThan 0
        $inline = Mount-PortalLayer -Name inline -Envelope '{"schema":"graph/1","module":"M","layer":"2026-10-09T00:00:00Z","nodes":[{"id":"a","kind":"K"}],"edges":[]}' -Url $Url
        $inline.counts.nodes | Should -Be 1
        $inline.hasOntology | Should -BeFalse
        (Invoke-RestMethod "$Url/api/tenants/default/ontology").layers.fs2.kinds.Directory.display.color | Should -Not -BeNullOrEmpty
        # A mount by path is recorded on screen for the next approval; an inline one is not.
        $s = Get-PortalScreen -Url $Url
        $m = $s.nodes | Where-Object id -EQ 'ui:mount:fs2'
        $m.kind | Should -Be 'Mount'
        $m.properties.path | Should -Be (Resolve-Path $FsFixture).Path
        $s.nodes.id | Should -Not -Contain 'ui:mount:inline'
        $s.nodes.id | Should -Not -Contain 'ui:mount:filesystem'
    }
    It 'whoami names the token''s principal; without a token the module reads and a write is 401; a bad token is 401 on a read' {
        $me = (Send $Url GET '/api/tenants/default/auth/whoami').Body
        $me.name | Should -Be 'pester'
        $me.id | Should -Be 'ui:principal:pester'
        $me.role | Should -Be 'admin'
        $me.writes | Should -BeTrue
        $me.administers | Should -BeTrue
        $saved = $env:PORTAL_TOKEN
        try {
            $env:PORTAL_TOKEN = $null
            (Get-PortalScreen -Url $Url).module | Should -Be 'Portal'
            $err = $null
            try { New-PortalSnap -Url $Url -Note 'anonymous' } catch { $err = $_ }
            $err.TargetObject.code | Should -Be 'unauthorized'
            $env:PORTAL_TOKEN = 'not-a-token'
            $err = $null
            try { Get-PortalScreen -Url $Url } catch { $err = $_ }
            $err.TargetObject.code | Should -Be 'unauthorized'
        }
        finally { $env:PORTAL_TOKEN = $saved }
    }
    It 'serves the menu and the config under the tenant, and the shell at a deep link; / goes to the first page' {
        $menu = (Send $Url GET '/api/tenants/default/menu').Body
        @($menu.nodes | Where-Object kind -EQ 'Service').name | Should -Be @('Overview', 'Graph', 'Emitters', 'Designer', 'Workflows', 'Environments', 'Compliance', 'Policy', 'Ledger', 'Identity', 'Artifacts', 'Stack', 'Settings')
        (Send $Url GET '/api/tenants/default/config').Body.product.name | Should -Be 'Portal'
        $r = Invoke-WebRequest "$Url/" -MaximumRedirection 0 -SkipHttpErrorCheck -ErrorAction SilentlyContinue
        [int]$r.StatusCode | Should -Be 302
        [string]$r.Headers['Location'] | Should -Be '/default/overview/summary'
        (Invoke-WebRequest "$Url/default/settings/service-connections").Content | Should -Match 'src="/static/portal\.js"'
        [int](Invoke-WebRequest "$Url/other/overview/summary" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        [int](Invoke-WebRequest "$Url/api/tenants/other/screen" -SkipHttpErrorCheck).StatusCode | Should -Be 404
    }
    It 'draws the rail from the tenant''s menu, Settings docked at the foot, and the page''s empty state' {
        $dom = Get-PageDom $Url '/default/overview/summary'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $top = [regex]::Match($dom, '<ul class="pt-services">(.*?)</ul>\s*<ul class="pt-services pt-docked">(.*?)</ul>\s*</nav>', 'Singleline')
        $top.Success | Should -BeTrue
        @([regex]::Matches($top.Groups[1].Value, 'data-service="ui:rail:([^"]+)"') | ForEach-Object { $_.Groups[1].Value }) |
            Should -Be @('overview', 'graph', 'emitters', 'designer', 'workflows', 'environments', 'compliance', 'policy', 'ledger', 'identity', 'artifacts', 'stack')
        $top.Groups[2].Value | Should -Match 'data-service="ui:rail:settings"'
        $dom | Should -Match 'data-page="ui:rail:overview:summary"[^>]*aria-current="page"|aria-current="page"[^>]*data-page="ui:rail:overview:summary"'
        $dom | Should -Match 'data-kind="portal\.summary"'
        $dom | Should -Match 'No summary yet'
        $dom | Should -Match 'data-principal="pester"|data-principal="anonymous"'
    }
    It 'never writes to a mounted layer' {
        $before = Get-ChildItem $FsFixture -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName).Hash)" }
        $null = Invoke-RestMethod "$Url/api/tenants/default/layers/filesystem/nodes?kind=File"
        $null = New-PortalSnap -Url $Url
        Get-ChildItem $FsFixture -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName).Hash)" } | Should -Be $before
    }
}

Describe 'Tenants and tokens' {
    It 'principals.yaml, tokens.yaml and connections.yaml are gitignored and their examples committed' {
        foreach ($f in 'principals', 'tokens', 'connections') {
            & git -C $Repo check-ignore -q "tenants/default/$f.yaml"
            $LASTEXITCODE | Should -Be 0 -Because "$f.yaml holds a tenant's own"
            Join-Path $Repo 'tenants' 'default' "$f.example.yaml" | Should -Exist
        }
        Join-Path $Repo 'tenants' 'default' 'portal.yaml' | Should -Exist
        Join-Path $Repo 'portal.base.yaml' | Should -Exist
        Join-Path $Repo 'schema' 'portal.schema.json' | Should -Exist
    }
    It 'New-PortalToken -Bootstrap makes the first admin once; -Name issues tokens only for a principal that exists' {
        $dir = Join-Path $TestDrive 'tok'
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $dir 'acme')
        Set-Content -LiteralPath (Join-Path $dir 'acme' 'portal.yaml') -Value '{}'
        { New-PortalToken -Name ci -Tenant acme -Tenants $dir } | Should -Throw '*No principal named ci*-Bootstrap*'
        $t = New-PortalToken -Name ci -Bootstrap -Tenant acme -Tenants $dir
        $t.Token | Should -Match '^[0-9a-f]{64}$'
        $t.Principal | Should -Be 'ui:principal:ci'
        $t.Role | Should -Be 'admin'
        $p = Get-Content -LiteralPath (Join-Path $dir 'acme' 'principals.yaml') -Raw
        $p | Should -Match "(?m)^  - id: 'ui:principal:ci'$"
        $p | Should -Match '(?m)^    role: admin$'
        $p | Should -Match '(?m)^    createdBy: null$'
        $text = Get-Content -LiteralPath $t.Path -Raw
        $text | Should -Not -Match $t.Token
        $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($t.Token))).ToLowerInvariant()
        $text | Should -Match "(?m)^  - principal: 'ui:principal:ci'\r?\n    sha256: $hash\r?\n    created: '\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z'"
        # Once: a second -Bootstrap, of any name, is an error and changes nothing.
        { New-PortalToken -Name other -Bootstrap -Tenant acme -Tenants $dir } | Should -Throw '*-Bootstrap makes a tenant''s first admin, once*'
        { New-PortalToken -Name ci -Bootstrap -Tenant acme -Tenants $dir } | Should -Throw '*once*'
        Get-Content -LiteralPath (Join-Path $dir 'acme' 'principals.yaml') -Raw | Should -Be $p
        # An existing principal may hold more than one token; an unknown one none.
        $again = New-PortalToken -Name ci -Tenant acme -Tenants $dir
        $again.Token | Should -Not -Be $t.Token
        { New-PortalToken -Name nobody -Tenant acme -Tenants $dir } | Should -Throw '*No principal named nobody*Identity/Users*'
        { New-PortalToken -Name ci -Tenant nope -Tenants $dir } | Should -Throw '*No tenant nope*'
        # What the module writes, the binary reads: a portal on this tenant knows ci as its admin.
        $work = Join-Path $TestDrive 'tok-art'
        $portal = Start-Portal -Port 18200 -Artifacts $work -Filesystem '' -Tenant acme -Tenants $dir
        try {
            $me = Invoke-RestMethod "$($portal.Url)/api/tenants/acme/auth/whoami" -Headers @{ Authorization = "Bearer $($again.Token)" }
            $me.id | Should -Be 'ui:principal:ci'
            $me.role | Should -Be 'admin'
            @((Invoke-RestMethod "$($portal.Url)/api/tenants/acme/tokens").principal) | Should -Be @('ui:principal:ci', 'ui:principal:ci')
        }
        # Windows can list the listener a moment after the process exits, and the next test starts on the same port.
        finally { Stop-Portal -Artifacts $work; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }
    }
    It 'a tenant overlay that fails the schema stops the binary, naming the file' {
        $dir = Join-Path $TestDrive 'badtenant'
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $dir 'default')
        Set-Content -LiteralPath (Join-Path $dir 'default' 'portal.yaml') -Value 'colours: dark'
        { Start-Portal -Port 18200 -Artifacts (Join-Path $TestDrive 'badtenant-art') -Filesystem '' -Tenants $dir -TimeoutSec 5 } |
            Should -Throw "*portal.yaml*'colours' not allowed*"
        # theme.graph.wheelZoomSpeed is held to 0.1..5 in an overlay too (docs/design.md 37).
        Set-Content -LiteralPath (Join-Path $dir 'default' 'portal.yaml') -Value 'theme: { graph: { wheelZoomSpeed: 10 } }'
        { Start-Portal -Port 18200 -Artifacts (Join-Path $TestDrive 'badtenant-art2') -Filesystem '' -Tenants $dir -TimeoutSec 5 } |
            Should -Throw '*portal.yaml*/theme/graph/wheelZoomSpeed*'
    }
}

Describe 'Identity' {
    # A tenant of its own: the first admin "root" (-Bootstrap) and a token from before 0.5.0, "old-ci", which names no
    # principal (docs/design.md 38).
    BeforeAll {
        $script:IdTenants = Join-Path $TestDrive 'id-tenants'
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $IdTenants 'default')
        Copy-Item -LiteralPath (Join-Path $Repo 'tenants' 'default' 'portal.yaml') -Destination (Join-Path $IdTenants 'default')
        $script:Root = New-PortalToken -Name root -Bootstrap -Tenants $IdTenants
        $script:LegacyToken = 'pester-legacy-token'
        $legacyHash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($LegacyToken))).ToLowerInvariant()
        Add-Content -LiteralPath $Root.Path -Value "  - name: 'old-ci'`n    sha256: $legacyHash" -Encoding utf8NoBOM
        $script:LegacyId = $legacyHash.Substring(0, 12)
        $script:IdWork = Join-Path $TestDrive 'id'
        $script:IdPortal = Start-Portal -Port 18200 -Artifacts $IdWork -Filesystem '' -Tenants $IdTenants
        $script:IdUrl = $IdPortal.Url
        $script:RootAuth = @{ Authorization = "Bearer $($Root.Token)" }
        $script:RootId = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($Root.Token))).ToLowerInvariant().Substring(0, 12)
    }
    AfterAll { Stop-Portal -Artifacts $IdWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'boot warns, naming each token that belongs to no principal; such a token acts as admin' {
        Get-Content -LiteralPath "$($IdPortal.LogPath).err" -Raw | Should -Match 'warning: tokens\.yaml entries without a principal act as admin until rotated: old-ci'
        $me = Invoke-RestMethod "$IdUrl/api/tenants/default/auth/whoami" -Headers @{ Authorization = "Bearer $LegacyToken" }
        $me.legacy | Should -BeTrue
        $me.role | Should -Be 'admin'
        $me.id | Should -BeNullOrEmpty
    }
    It 'draws Users with the bootstrap admin and Add disabled for anonymous, Agents as its empty state, Tokens by principal' {
        $users = Get-PageDom $IdUrl '/default/identity/users'
        if (-not $users) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $users | Should -Match '<tr data-principal="ui:principal:root"><td>root</td><td>admin</td><td>[^<]+</td><td>bootstrap</td></tr>'
        $users | Should -Match '<button type="submit" class="gr-btn" disabled="" title="sign in to write" data-readonly="true">Add</button>'
        $agents = Get-PageDom $IdUrl '/default/identity/agents'
        $agents | Should -Match 'No agents yet'
        $agents | Should -Match 'minted via Marker, later'
        $agents | Should -Not -Match 'data-form="add-user"'
        $tokens = Get-PageDom $IdUrl '/default/settings/tokens'
        $tokens | Should -Match "<tr data-token=""$RootId""><td>root</td>"
        $tokens | Should -Match "<tr data-token=""$LegacyId""><td>old-ci <span class=""pt-badge"">legacy: admin until rotated</span></td>"
        $tokens | Should -Match 'disabled="" title="sign in to write" data-readonly="true">Revoke</button>'
    }
    It 'the Users page Add flow: the admin adds a reader in the page; principals.yaml records it, createdBy the admin' {
        $js = @'
(async () => {
  const wait = async (f, what) => { for (let i = 0; i < 100; i++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 100)); } throw new Error('timed out waiting for ' + what); };
  const form = await wait(() => document.querySelector('form[data-form="add-user"]'), 'the Add user form');
  await wait(() => document.querySelector('tr[data-principal="ui:principal:root"]'), 'the users table');
  const btn = form.querySelector('button[type="submit"]');
  const role = form.querySelector('select[name="role"]');
  const out = { disabled: btn.disabled, title: btn.title, roles: [...role.options].filter((o) => !o.disabled).map((o) => o.value), avatar: document.getElementById('avatar').dataset.role };
  form.querySelector('input[name="name"]').value = 'ann';
  role.value = 'reader';
  form.requestSubmit();
  const row = await wait(() => document.querySelector('tr[data-principal="ui:principal:ann"]'), 'the added row');
  out.cells = [...row.cells].map((c) => c.textContent);
  out.status = form.querySelector('.pt-status').textContent;
  return JSON.stringify(out);
})()
'@
        $raw = Invoke-PageScript -Url $IdUrl -Route '/default/identity/users' -Token $Root.Token -Script $js
        if ($null -eq $raw) { Set-ItResult -Skipped -Because 'no Chrome for the DevTools protocol'; return }
        $seen = $raw | ConvertFrom-Json
        $seen.avatar | Should -Be 'admin'
        $seen.disabled | Should -BeFalse
        $seen.title | Should -BeNullOrEmpty
        @($seen.roles) | Should -Be @('reader', 'contributor', 'admin')
        $seen.cells[0] | Should -Be 'ann'
        $seen.cells[1] | Should -Be 'reader'
        $seen.cells[3] | Should -Be 'root'
        $seen.status | Should -Be 'Added ann. Issue its token with New-PortalToken -Name ann.'
        $file = Get-Content -LiteralPath (Join-Path $IdTenants 'default' 'principals.yaml') -Raw
        $file | Should -Match "(?m)^  - id: 'ui:principal:ann'\r?\n    name: 'ann'\r?\n    type: user\r?\n    role: reader\r?\n    created: '[^']+'\r?\n    createdBy: 'ui:principal:root'$"
        $file | Should -Match '(?m)^# Principals of tenant default'
        $n = (Invoke-RestMethod "$IdUrl/api/tenants/default/principals").nodes | Where-Object id -EQ 'ui:principal:ann'
        $n.properties.createdBy | Should -Be 'ui:principal:root'
    }
    It 'New-PortalToken issues a token for a principal the portal added; an admin''s Revoke reloads tokens.yaml, so it goes live, and a reader is 403' {
        $ann = New-PortalToken -Name ann -Tenants $IdTenants
        $ann.Role | Should -Be 'reader'
        $annAuth = @{ Authorization = "Bearer $($ann.Token)" }
        # Minted after boot: refused until the portal next reads tokens.yaml.
        [int](Invoke-WebRequest "$IdUrl/api/tenants/default/auth/whoami" -Headers $annAuth -SkipHttpErrorCheck).StatusCode | Should -Be 401
        $gone = Invoke-RestMethod -Method Delete "$IdUrl/api/tenants/default/tokens/$LegacyId" -Headers $RootAuth
        $gone.name | Should -Be 'old-ci'
        $gone.legacy | Should -BeTrue
        [int](Invoke-WebRequest "$IdUrl/api/tenants/default/auth/whoami" -Headers @{ Authorization = "Bearer $LegacyToken" } -SkipHttpErrorCheck).StatusCode | Should -Be 401
        (Get-Content -LiteralPath $Root.Path -Raw) | Should -Not -Match 'old-ci'
        (Invoke-RestMethod "$IdUrl/api/tenants/default/auth/whoami" -Headers $annAuth).role | Should -Be 'reader'
        $r = Invoke-WebRequest -Method Post "$IdUrl/api/tenants/default/snap" -Headers $annAuth -SkipHttpErrorCheck
        [int]$r.StatusCode | Should -Be 403
        ($r.Content | ConvertFrom-Json).code | Should -Be 'forbidden'
        $r = Invoke-WebRequest -Method Post "$IdUrl/api/tenants/default/principals" -Headers $RootAuth -ContentType 'application/json' -Body '{"name":"ann","role":"reader"}' -SkipHttpErrorCheck
        [int]$r.StatusCode | Should -Be 409
    }
}

Describe 'Service connections' {
    # A fake GitHub on localhost answering GET /user/repos with two repos for the right token, and a tenant of its
    # own whose github connection points at it, its token in PORTAL_PESTER_GH, which the portal inherits.
    BeforeAll {
        $script:FakePort = 18201
        $script:Fake = Start-ThreadJob -ArgumentList $FakePort -ScriptBlock {
            param($Port)
            $l = [System.Net.HttpListener]::new()
            $l.Prefixes.Add("http://localhost:$Port/")
            $l.Start()
            try {
                while ($l.IsListening) {
                    $c = $l.GetContext()
                    if ($c.Request.Url.AbsolutePath -eq '/stop') { $c.Response.Close(); break }
                    $ok = $c.Request.Url.AbsolutePath -eq '/user/repos' -and $c.Request.Headers['Authorization'] -eq 'Bearer gh-pester'
                    $body = if ($ok) { '[{"full_name":"octo/one","default_branch":"main","updated_at":"2026-10-01T00:00:00Z"},{"full_name":"octo/two","default_branch":"main","updated_at":"2026-10-02T00:00:00Z"}]' } else { '{"message":"Bad credentials"}' }
                    $c.Response.StatusCode = if ($ok) { 200 } else { 401 }
                    $bytes = [Text.Encoding]::UTF8.GetBytes($body)
                    $c.Response.ContentType = 'application/json'
                    $c.Response.OutputStream.Write($bytes, 0, $bytes.Length)
                    $c.Response.Close()
                }
            }
            finally { $l.Close() }
        }
        $script:ConnTenants = Join-Path $TestDrive 'conn-tenants'
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $ConnTenants 'default')
        Copy-Item -LiteralPath (Join-Path $Repo 'tenants' 'default' 'portal.yaml') -Destination (Join-Path $ConnTenants 'default')
        Set-Content -LiteralPath (Join-Path $ConnTenants 'default' 'connections.yaml') -Value "connections:`n  - { name: github, type: github, url: 'http://localhost:$FakePort', token_env: PORTAL_PESTER_GH }"
        $script:ConnToken = (New-PortalToken -Name pester -Bootstrap -Tenants $ConnTenants).Token
        $script:SavedGh = $env:PORTAL_PESTER_GH
        $env:PORTAL_PESTER_GH = 'gh-pester'
        $script:ConnWork = Join-Path $TestDrive 'conn'
        $script:ConnPortal = Start-Portal -Port 18200 -Artifacts $ConnWork -Filesystem '' -Tenants $ConnTenants
    }
    AfterAll {
        Stop-Portal -Artifacts $ConnWork
        $env:PORTAL_PESTER_GH = $SavedGh
        # The listener blocks in GetContext, so Stop-Job would wait for ever: /stop ends its loop instead.
        try { $null = Invoke-WebRequest "http://localhost:$FakePort/stop" -TimeoutSec 2 -SkipHttpErrorCheck } catch { }
        $null = Wait-Job $Fake -Timeout 5
        Remove-Job $Fake -Force -ErrorAction SilentlyContinue
    }
    It 'the repos endpoint needs a token though it is a GET: no token 401, good token 200 with the repositories' {
        $uri = "$($ConnPortal.Url)/api/tenants/default/connections/github/repos"
        $anon = Invoke-WebRequest $uri -SkipHttpErrorCheck
        [int]$anon.StatusCode | Should -Be 401
        ($anon.Content | ConvertFrom-Json).code | Should -Be 'unauthorized'
        $good = Invoke-WebRequest $uri -Headers @{ Authorization = "Bearer $ConnToken" } -SkipHttpErrorCheck
        [int]$good.StatusCode | Should -Be 200 -Because $good.Content
        $env = $good.Content | ConvertFrom-Json
        @($env.nodes.id) | Should -Be @('ui:conn:github/octo/one', 'ui:conn:github/octo/two')
        @($env.nodes.kind | Sort-Object -Unique) | Should -Be @('git.repository')
        $good.Content | Should -Not -Match 'gh-pester'
    }
}

Describe 'Boot order' {
    BeforeAll {
        $script:BootWork = Join-Path $TestDrive 'boot'
        $p = Start-Portal -Port 18200 -Artifacts $BootWork -Filesystem ''
        try {
            $script:First = New-PortalSnap -Url $p.Url -Note 'first'
            $null = Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/tenants/default/nodes/ui:theme:default" -Headers $Auth -ContentType 'application/json' -Body '{"properties":{"color":"#336699"}}'
            $script:Second = New-PortalSnap -Url $p.Url -Note 'second'
            # Not approved: gone after a restart.
            $null = Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/tenants/default/nodes/ui:theme:default" -Headers $Auth -ContentType 'application/json' -Body '{"properties":{"color":"#000000"}}'
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

Describe 'Section States on an old snap' {
    BeforeAll {
        $script:OldWork = Join-Path $TestDrive 'old'
        $p = Start-Portal -Port 18200 -Artifacts $OldWork -Filesystem ''
        try {
            foreach ($sec in $Sections) { $null = Invoke-RestMethod -Method Delete -Headers $Auth -Uri "$($p.Url)/api/tenants/default/nodes/ui:state:designer:$($sec.Slug)" }
            $script:Old = New-PortalSnap -Url $p.Url -Note '0.3.0 shape'
        }
        finally { Stop-Portal -Artifacts $OldWork }
    }
    AfterAll { Stop-Portal -Artifacts $OldWork }
    It 'a snap without the Designer''s section States (0.3.0''s shape) boots with the defaults, visible in the graph' {
        $old = $Old
        (Get-Content -LiteralPath $old.path -Raw) | Should -Not -Match 'ui:state:designer:'
        $p = Start-Portal -Port 18200 -Artifacts $OldWork -Filesystem ''
        $p.Boot | Should -Be $old.stamp
        $s = Get-PortalScreen -Url $p.Url
        foreach ($sec in $Sections) {
            ($s.nodes | Where-Object id -EQ "ui:state:designer:$($sec.Slug)").properties.collapsed | Should -Be (-not $sec.Open)
        }
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
