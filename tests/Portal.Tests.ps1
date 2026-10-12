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

    # The Graph.Node version the lift and the graph-node skill are pinned to (docs/design.md 4, 40).
    $script:GraphNodeTag = 'v0.4.2'
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
    It 'is version 0.16.1 and needs PowerShell 7.4' {
        $m = Import-PowerShellDataFile $Manifest
        $m.ModuleVersion | Should -Be '0.16.1'
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
    It 'declares module Portal 0.16.1, the twenty-two Kinds, the twenty-six edge Kinds and the approved finding Kind' {
        $text = Get-Content (Join-Path $Repo 'ontology.yaml') -Raw
        $text | Should -Match '(?m)^module: Portal$'
        $text | Should -Match '(?m)^version: 0\.16\.1$'
        foreach ($k in 'Container', 'Widget', 'Layout', 'Category', 'State', 'Theme', 'Ref', 'Mount', 'Service', 'Page', 'Principal', 'Feed', 'Card', 'Run', 'Decision', 'Emitter', 'Layer', 'Ledger', 'LedgerEntry', 'Alerts', 'Alert', 'Actions') { $text | Should -Match "(?m)^  ${k}:" }
        foreach ($e in 'contains', 'laid-out-by', 'is-a', 'displays', 'has-state', 'themed-by', 'opens', 'has-page', 'has-card', 'has-run', 'has-decision', 'produced-by', 'has-layer', 'has-entry', 'prev', 'records', 'recorded-in', 'has-alert', 'about', 'raised-in', 'has-action', 'answered-by', 'answers', 'request', 'subject', 'principal') { $text | Should -Match "(?m)^  ${e}:" }
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
    AfterEach { Stop-Portal -Artifacts $UpWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }
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
        $s.version | Should -Be '0.16.1'
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
    It 'Graph/Explore''s query bar draws GET .../query from the URL, round-trips to the same URL, and the result passes graphnode validate' {
        # docs/design.md 54. The query string is the bar's fields in the API's order, as URLSearchParams writes them.
        $qs = 'from=fs%3Asrc&follow=contains&direction=out&depth=1&until=File'
        $api = Send $Url GET "/api/tenants/default/query?$qs"
        $api.Status | Should -Be 200
        @($api.Body.nodes.id) | Should -Be @('fs:src', 'fs:src/Graph.Emitter.FileSystem')
        @($api.Body.edges.id) | Should -Be @('fs:src|fs:src/Graph.Emitter.FileSystem|contains')
        @($api.Body.paths | ForEach-Object { $_ -join '>' }) | Should -Be @('fs:src>fs:src/Graph.Emitter.FileSystem')
        ($api.Body.query | ConvertTo-Json -Compress) | Should -Be '{"from":["fs:src"],"follow":["contains"],"direction":"out","depth":1,"until":"File","where":[],"limit":500}'
        $api.Body.truncated | Should -BeFalse
        # Test-Graph -Strict's schema check is the graphnode binary's validate, lifted at the pinned tag (docs/design.md 4).
        $file = Join-Path $TestDrive 'query.graph.json'
        Set-Content -LiteralPath $file -Encoding utf8NoBOM -NoNewline -Value (Invoke-WebRequest "$Url/api/tenants/default/query?$qs" -Headers $Auth).Content
        $out = & $GraphNodeBin validate $file
        $LASTEXITCODE | Should -Be 0 -Because ($out -join "`n")
        # A from no node has is 400, named; nothing reached is 200.
        $bad = Send $Url GET '/api/tenants/default/query?from=fs%3Anope'
        "$($bad.Status) $($bad.Body.message)" | Should -Be '400 from: no node fs:nope in the mounted layers'
        (Send $Url GET '/api/tenants/default/query?kind=Nope').Body.nodes | Should -BeNullOrEmpty

        $wait = 'const wait = async (f) => { for (let t = 0; t < 160; t++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 50)); } throw new Error("timed out: " + f); };'
        $r = Invoke-PageScript $Url "/default/graph/explore?$qs" '' ("(async () => { $wait " +
            'const s = await wait(() => document.querySelector("#query-stats[data-nodes]")); await wait(() => document.querySelector(".pt-graph-root[data-drawn]")); ' +
            'const f = document.querySelector("form[data-form=query]"); const out = { before: location.pathname + location.search, stats: s.textContent }; ' +
            'out.fields = [...f.elements].filter((x) => x.name).map((x) => `${x.name}=${x.value}`); ' +
            'out.paths = [...document.querySelectorAll(".pt-paths li")].map((li) => li.dataset.path + "|" + [...li.querySelectorAll("button.pt-path-id")].map((b) => b.dataset.id).join(",")); ' +
            'f.requestSubmit(); await new Promise((r) => setTimeout(r, 300)); out.after = location.pathname + location.search; ' +
            # A right-click on a drawn node is Query from here; it goes to Explore from that node.
            'const root = document.querySelector(".pt-graph-root"); root._cyreg.cy.getElementById("fs:src/Graph.Emitter.FileSystem").emit("cxttap"); ' +
            'const m = await wait(() => document.querySelector(".pt-nodemenu")); out.menu = [...m.querySelectorAll("[role=menuitem]")].map((x) => `${x.textContent}|${x.getAttribute("href")}`); ' +
            'm.querySelector("[data-item=query-from-here]").click(); await wait(() => location.search.includes("Graph.Emitter")); ' +
            'const t = await wait(() => document.querySelector("#query-stats[data-nodes]")); out.next = location.pathname + location.search + " " + t.dataset.nodes; ' +
            # No query in the URL: the bar over Explore's Canvases, as before.
            'document.querySelector("a.pt-page[data-page=\"ui:rail:graph:explore\"]").click(); await wait(() => document.querySelector(".pt-explore .gr-box[data-id]")); ' +
            'out.plain = location.pathname + location.search + " " + !!document.querySelector("form[data-form=query]") + " " + !document.querySelector("#query-stats"); ' +
            'return JSON.stringify(out); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        $o.before | Should -Be "/default/graph/explore?$qs"
        $o.after | Should -Be $o.before
        $o.stats | Should -Be '2 nodes · 1 edges · 1 paths'
        $o.fields | Should -Be @('from=fs:src', 'follow=contains', 'direction=out', 'depth=1', 'until=File')
        $o.paths | Should -Be @('fs:src>fs:src/Graph.Emitter.FileSystem|fs:src,fs:src/Graph.Emitter.FileSystem')
        $o.menu | Should -Be @('Query from here|/default/graph/explore?from=fs%3Asrc%2FGraph.Emitter.FileSystem')
        $n = @((Send $Url GET '/api/tenants/default/query?from=fs%3Asrc%2FGraph.Emitter.FileSystem').Body.nodes).Count
        $o.next | Should -Be "/default/graph/explore?from=fs%3Asrc%2FGraph.Emitter.FileSystem $n"
        $o.plain | Should -Be '/default/graph/explore True True'
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
    It 'a mount answers Location at the layer''s page, which serves the shell; POST /api/layers is 410 and names the tenant route' {
        $body = @{ name = 'fs-located'; path = (Resolve-Path $FsFixture).Path } | ConvertTo-Json
        $r = Invoke-WebRequest "$Url/api/tenants/default/layers" -Method POST -ContentType 'application/json' -Headers $Auth -Body $body
        [int]$r.StatusCode | Should -Be 201
        [string]$r.Headers['Location'] | Should -Be '/default/layers/fs-located'
        (Invoke-WebRequest "$Url/default/layers/fs-located").Content | Should -Match 'src="/static/portal\.js"'
        [int](Invoke-WebRequest "$Url/default/layers/nope" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        $gone = Invoke-WebRequest "$Url/api/layers" -Method POST -ContentType 'application/json' -Headers $Auth -Body ($body -replace 'fs-located', 'fs-gone') -SkipHttpErrorCheck
        [int]$gone.StatusCode | Should -Be 410
        ($gone.Content | ConvertFrom-Json).message | Should -BeLike '*POST /api/tenants/default/layers*'
        (Invoke-RestMethod "$Url/api/tenants/default/layers").name | Should -Not -Contain 'fs-gone'
    }
    It 'Graph/Layers links each layer to its page, which shows its counts, stamp and nodes' {
        $dom = Get-PageDom $Url '/default/graph/layers'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<tr data-layer="filesystem"><td><a href="/default/layers/filesystem">filesystem</a>'
        $fs = (Invoke-WebRequest "$Url/api/tenants/default/layers").Content | ConvertFrom-Json -DateKind String | Where-Object name -EQ 'filesystem'
        $dom = Get-PageDom $Url '/default/layers/filesystem'
        $dom | Should -Match '<h1 class="pt-title" id="title"[^>]*>filesystem</h1>'
        $dom | Should -Match ([regex]::Escape(">$($fs.counts.nodes) nodes · $($fs.counts.edges) edges · $($fs.stamp)</span>"))
        @([regex]::Matches($dom, '<tr data-id="')).Count | Should -Be $fs.counts.nodes
        $dom | Should -Match 'data-page="ui:rail:graph:layers"[^>]*aria-current="page"|aria-current="page"[^>]*data-page="ui:rail:graph:layers"'
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
    It 'Identity/Agents lists the principal that mounted runs; its page lists them, deny count in red, linking the layer and each decision' {
        $runs = Join-Path $Fixtures 'layers' 'agent-runs'
        $before = Get-ChildItem $runs -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName).Hash)" }
        $run = 'agent:run/2026-10-09T02:50:43.7976027Z'
        $l = Mount-PortalLayer -Name agent-runs -Path $runs -Url $Url
        $l.mountedBy | Should -Be 'ui:principal:pester'
        $api = Invoke-RestMethod "$Url/api/tenants/default/agents/pester/runs"
        ($api.nodes | Where-Object kind -EQ 'Run').id | Should -Be $run
        @($api.nodes | Where-Object kind -EQ 'Decision').Count | Should -Be 5
        [int](Invoke-WebRequest "$Url/default/agents/nobody" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        $dom = Get-PageDom $Url '/default/agents'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<tr data-agent="pester"><td><a href="/default/agents/pester">pester</a></td><td>user</td><td>admin</td><td>1</td><td><span class="pt-deny" data-deny="1">1</span></td><td>2026-10-09T02:50:43Z</td></tr>'
        $dom | Should -Match '<span class="pt-count">1 agents</span>'
        $dom | Should -Match 'href="/default/agents" class="pt-page" data-page="ui:rail:identity:agents" aria-current="page"'
        $dom = Get-PageDom $Url '/default/agents/pester'
        $dom | Should -Match '<h1 class="pt-title" id="title"[^>]*>pester</h1>'
        $dom | Should -Match '<span class="pt-badge" id="agent-stats"[^>]*>1 runs · 1 denied</span>'
        $dom | Should -Match ('<tr data-run="' + [regex]::Escape($run) + '" title="[^"]*"><td>2026-10-09T02:50:43Z</td><td>2026-10-09T02:50:56Z</td><td>3</td><td>4</td><td><span class="pt-deny" data-deny="1">1</span></td><td>0</td><td>0</td><td><a href="/default/layers/agent-runs">agent-runs</a></td></tr>')
        $links = @([regex]::Matches($dom, '<a href="/default/layers/agent-runs#([^"]+)" class="pt-decision pt-(allow|deny|witness)"') | ForEach-Object { "$([uri]::UnescapeDataString($_.Groups[1].Value)) $($_.Groups[2].Value)" })
        $links | Should -Be @("$run/step-1/pre allow", "$run/step-1/post allow", "$run/step-2/pre allow", "$run/step-2/post allow", "$run/step-3/pre deny")
        $dom = Get-PageDom $Url "/default/layers/agent-runs#$([uri]::EscapeDataString("$run/step-3/pre"))"
        $dom | Should -Match ('<tr data-id="' + [regex]::Escape("$run/step-3/pre") + '" class="pt-target" aria-current="true">')
        Get-ChildItem $runs -File | ForEach-Object { "$($_.Name) $((Get-FileHash $_.FullName).Hash)" } | Should -Be $before
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
        $agents | Should -Match 'An agent is a principal holding runs'
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

Describe 'Dev mode' {
    # docs/design.md 39. Each portal here has its own tenant directory, so the repo's identity files are never touched.
    BeforeAll {
        function New-ModeTenant([string] $Mode) {
            $dir = Join-Path $TestDrive "mode-$Mode-$([guid]::NewGuid().ToString('n'))"
            $null = New-Item -ItemType Directory -Force -Path (Join-Path $dir 'default')
            Set-Content -LiteralPath (Join-Path $dir 'default' 'portal.yaml') -Value "mode: $Mode" -Encoding utf8NoBOM
            $dir
        }
        # Side portals (other tenants, other modes) run on 18205, so the dev portal on 18200 runs throughout.
        function Start-SidePortal([string] $Tenants, [string] $Token) {
            $saved = $env:PORTAL_TOKEN
            try {
                $env:PORTAL_TOKEN = $Token
                Start-Portal -Port 18205 -Artifacts (Join-Path $TestDrive "side-$([guid]::NewGuid().ToString('n'))") -Filesystem '' -Tenants $Tenants
            }
            finally { $env:PORTAL_TOKEN = $saved }
        }
        function Stop-SidePortal($P) { if ($P) { Stop-Portal -Artifacts $P.Artifacts; InModuleScope Portal { Wait-PortalPortFree -Port 18205 -TimeoutSec 10 } } }
        # A tenant in mode dev with the real first admin (root), dev-bootstrap from the Dev mint, and a reader (reed)
        # that dev-bootstrap made, each with a token; and a portal started as Invoke-Build Dev starts it, with
        # PORTAL_TOKEN the dev token.
        $script:DevTenants = New-ModeTenant dev
        $script:DevRoot = New-PortalToken -Name root -Bootstrap -Tenants $DevTenants
        $script:Dev = InModuleScope Portal -Parameters @{ T = $DevTenants } { param($T) New-PortalDevToken -Tenants $T }
        Add-Content -LiteralPath (Join-Path $DevTenants 'default' 'principals.yaml') -Encoding utf8NoBOM -Value (
            "  - id: 'ui:principal:reed'`n    name: 'reed'`n    type: user`n    role: reader`n    created: '2026-10-11T00:00:00.000Z'`n    createdBy: 'ui:principal:dev-bootstrap'")
        $script:Reed = New-PortalToken -Name reed -Tenants $DevTenants
        $script:DevWork = Join-Path $TestDrive 'dev'
        $saved = $env:PORTAL_TOKEN
        try {
            $env:PORTAL_TOKEN = $Dev.Token
            $script:DevPortal = Start-Portal -Port 18200 -Artifacts $DevWork -Filesystem '' -Tenants $DevTenants
        }
        finally { $env:PORTAL_TOKEN = $saved }
        $script:DevUrl = $DevPortal.Url
    }
    AfterAll { Stop-Portal -Artifacts $DevWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'tenants/default/portal.yaml is neither committed nor staged with mode dev (README, Before you demo)' {
        # Fails on purpose while the default tenant is set to dev: a dev-mode tenant must never be committed.
        foreach ($rev in 'HEAD', '') {
            $text = (& git -C $Repo show "${rev}:tenants/default/portal.yaml" 2>$null) -join "`n"
            $where = if ($rev) { 'committed (HEAD)' } else { 'staged (the index)' }
            $text | Should -Not -Match '(?m)^mode:\s*[''"]?dev[''"]?\s*(#.*)?$' -Because "tenants/default/portal.yaml as $where says mode: dev; switch it to demo or prod before committing"
        }
    }
    It 'Get-PortalTenantMode reads <Text> as <Mode>' -ForEach @(
        @{ Text = '{}'; Mode = 'prod' }
        @{ Text = 'mode: dev'; Mode = 'dev' }
        @{ Text = "mode: 'demo'  # for the review"; Mode = 'demo' }
        @{ Text = "# mode: dev`nproduct: { name: X }"; Mode = 'prod' }
        @{ Text = '{ mode: dev }'; Mode = 'prod' }
    ) {
        $dir = Join-Path $TestDrive "read-$([guid]::NewGuid().ToString('n'))"
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $dir 'default')
        Set-Content -LiteralPath (Join-Path $dir 'default' 'portal.yaml') -Value $Text
        InModuleScope Portal -Parameters @{ T = $dir } { param($T) Get-PortalTenantMode -Tenants $T } | Should -Be $Mode
    }
    It 'the Dev mint makes dev-bootstrap once, replaces only its own token, and refuses outside mode dev' {
        $mt = New-ModeTenant dev
        $null = New-PortalToken -Name root -Bootstrap -Tenants $mt
        $first = InModuleScope Portal -Parameters @{ T = $mt } { param($T) New-PortalDevToken -Tenants $T }
        $first.Replaced | Should -Be 0
        $again = InModuleScope Portal -Parameters @{ T = $mt } { param($T) New-PortalDevToken -Tenants $T }
        $again.Token | Should -Not -Be $first.Token
        $again.Replaced | Should -Be 1
        $again.Role | Should -Be 'admin'
        $principals = InModuleScope Portal -Parameters @{ T = $mt } { param($T) Read-PortalYamlList -Path (Join-Path $T 'default' 'principals.yaml') -Key principals }
        @($principals | Where-Object name -CEQ 'dev-bootstrap').Count | Should -Be 1
        ($principals | Where-Object name -CEQ 'dev-bootstrap').createdBy | Should -BeNullOrEmpty
        $tokens = InModuleScope Portal -Parameters @{ T = $mt } { param($T) Read-PortalYamlList -Path (Join-Path $T 'default' 'tokens.yaml') -Key tokens }
        @($tokens | Where-Object principal -CEQ 'ui:principal:dev-bootstrap').Count | Should -Be 1
        @($tokens | Where-Object principal -CEQ 'ui:principal:root').Count | Should -Be 1
        ($tokens | Where-Object principal -CEQ 'ui:principal:dev-bootstrap').sha256 | Should -Be ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($again.Token))).ToLowerInvariant())
        (Get-Content -LiteralPath $again.Path -Raw) | Should -Match '(?m)^# Bearer tokens of tenant default'
        foreach ($mode in 'demo', 'prod') {
            $t = New-ModeTenant $mode
            { InModuleScope Portal -Parameters @{ T = $t } { param($T) New-PortalDevToken -Tenants $T } } | Should -Throw "*mode $mode, not dev*"
            Test-Path -LiteralPath (Join-Path $t 'default' 'tokens.yaml') | Should -BeFalse
        }
    }
    It '-Bootstrap appends the real first admin beside dev-bootstrap; the name dev-bootstrap is reserved' {
        $t = New-ModeTenant dev
        $null = InModuleScope Portal -Parameters @{ T = $t } { param($T) New-PortalDevToken -Tenants $T }
        $jerry = New-PortalToken -Name jerry -Bootstrap -Tenants $t
        $jerry.Role | Should -Be 'admin'
        $names = @((InModuleScope Portal -Parameters @{ T = $t } { param($T) Read-PortalYamlList -Path (Join-Path $T 'default' 'principals.yaml') -Key principals }).name)
        $names | Should -Be @('dev-bootstrap', 'jerry')
        { New-PortalToken -Name eve -Bootstrap -Tenants $t } | Should -Throw '*already holds 1 principal(s) (jerry)*'
        { New-PortalToken -Name dev-bootstrap -Tenants $t } | Should -Throw '*reserved*'
        # The binary loads what the module wrote.
        $p = $null
        try {
            $p = Start-SidePortal $t ''
            (Invoke-RestMethod "$($p.Url)/api/tenants/default/principals").nodes.name | Should -Be @('dev-bootstrap', 'jerry')
        }
        finally { Stop-SidePortal $p }
    }
    It 'Invoke-Build Dev refuses, building and minting nothing, for a tenant not in mode dev; Up never mints' {
        $t = New-ModeTenant demo
        $saved = @{ PORTAL_TENANTS = $env:PORTAL_TENANTS; PORTAL_TENANT = $env:PORTAL_TENANT }
        $err = $null
        try {
            $env:PORTAL_TENANTS = $t; $env:PORTAL_TENANT = $null
            try { Invoke-Build Dev -File (Join-Path $Repo '.build.ps1') *> $null } catch { $err = "$_" }
        }
        finally { foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) } }
        $err | Should -Match 'Dev refuses: tenant default is in mode demo, not dev'
        Test-Path -LiteralPath (Join-Path $t 'default' 'tokens.yaml') | Should -BeFalse
        Test-Path -LiteralPath (Join-Path $t 'default' 'principals.yaml') | Should -BeFalse
        $build = Get-Content -LiteralPath (Join-Path $Repo '.build.ps1') -Raw
        $up = [regex]::Match($build, '(?ms)^task Up Build, \{(.*?)^\}').Groups[1].Value
        $up | Should -Not -BeNullOrEmpty
        $up | Should -Not -Match 'Token|Mint'
        $build | Should -Match '(?m)^task Dev DevGate, DevMint, Up, \{'
    }
    It 'the binary says mode dev at boot, GET /api/tenants/default says dev, and dev-token hands out the Dev token' {
        Get-Content -LiteralPath $DevPortal.LogPath -Raw | Should -Match 'tenant default from .*, mode dev \('
        Get-Content -LiteralPath "$($DevPortal.LogPath).err" -Raw | Should -Match 'warning: tenant default is in mode dev'
        (Invoke-RestMethod "$DevUrl/api/tenants/default").mode | Should -Be 'dev'
        $t = Invoke-RestMethod "$DevUrl/api/tenants/default/dev-token"
        $t.token | Should -Be $Dev.Token
        $t.principal | Should -Be 'ui:principal:dev-bootstrap'
        (Invoke-RestMethod "$DevUrl/api/tenants/default/auth/whoami" -Headers @{ Authorization = "Bearer $($t.token)" }).role | Should -Be 'admin'
    }
    It 'the page: the amber bar, then Use dev token signs in as dev-bootstrap, admin, and Add works' {
        $js = @'
(async () => {
  const wait = async (f, what) => { for (let i = 0; i < 100; i++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 100)); } throw new Error('timed out waiting for ' + what); };
  const avatar = document.getElementById('avatar');
  await wait(() => avatar.dataset.principal, 'the avatar');
  await wait(() => document.querySelector('form[data-form="add-user"]'), 'the Add user form');
  const bar = document.getElementById('modebar');
  const out = { bar: bar.hidden ? null : bar.textContent, barButtons: bar.querySelectorAll('button').length, badge: document.getElementById('mode').textContent, before: avatar.dataset.principal,
    addBefore: document.querySelector('form[data-form="add-user"] button[type="submit"]').disabled };
  avatar.click();
  const use = await wait(() => document.querySelector('#account button[data-dev-token]'), 'Use dev token');
  use.click();
  await wait(() => avatar.dataset.role === 'admin', 'the admin avatar');
  await new Promise((r) => setTimeout(r, 300));
  out.avatar = avatar.textContent;
  out.principal = avatar.dataset.principal;
  out.stored = (sessionStorage.getItem('portal.token') || '').length;
  const form = await wait(() => document.querySelector('form[data-form="add-user"]'), 'the Add user form');
  const btn = form.querySelector('button[type="submit"]');
  out.addDisabled = btn.disabled;
  form.querySelector('input[name="name"]').value = 'ann';
  form.querySelector('select[name="role"]').value = 'reader';
  btn.click();
  const row = await wait(() => document.querySelector('tr[data-principal="ui:principal:ann"]'), 'the added row');
  out.added = [...row.cells].map((c) => c.textContent);
  return JSON.stringify(out);
})()
'@
        $raw = Invoke-PageScript -Url $DevUrl -Route '/default/identity/users' -Token '' -Script $js
        if ($null -eq $raw) { Set-ItResult -Skipped -Because 'no Chrome for the DevTools protocol'; return }
        $seen = $raw | ConvertFrom-Json
        $seen.bar | Should -BeExactly 'DEV MODE — auto-minted admin, local sign-in shortcut. Not for demo or production. See README: Before you demo.'
        $seen.barButtons | Should -Be 0
        $seen.badge | Should -Be 'dev'
        $seen.before | Should -Be 'anonymous'
        $seen.addBefore | Should -BeTrue
        $seen.principal | Should -Be 'dev-bootstrap'
        $seen.avatar | Should -Match 'dev-bootstrap · admin'
        $seen.stored | Should -Be 64
        $seen.addDisabled | Should -BeFalse
        $seen.added[0] | Should -Be 'ann'
        $seen.added[3] | Should -Be 'dev-bootstrap'
    }
    It 'the page: a reader signed in through the box cannot click Add' {
        $js = @'
(async () => {
  const wait = async (f, what) => { for (let i = 0; i < 100; i++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 100)); } throw new Error('timed out waiting for ' + what); };
  const avatar = document.getElementById('avatar');
  await wait(() => avatar.dataset.principal, 'the avatar');
  avatar.click();
  const f = await wait(() => document.querySelector('#account form.pt-token-form'), 'the sign-in form');
  f.querySelector('input[name="token"]').value = '__READER__';
  f.requestSubmit();
  await wait(() => avatar.dataset.role === 'reader', 'the reader avatar');
  await new Promise((r) => setTimeout(r, 300));
  const form = await wait(() => document.querySelector('form[data-form="add-user"]'), 'the Add user form');
  const btn = form.querySelector('button[type="submit"]');
  const out = { avatar: avatar.textContent, disabled: btn.disabled, title: btn.title };
  form.querySelector('input[name="name"]').value = 'bob';
  btn.click();
  await new Promise((r) => setTimeout(r, 800));
  out.bob = !!document.querySelector('tr[data-principal="ui:principal:bob"]');
  avatar.click();
  avatar.click();
  out.signOut = !![...document.querySelectorAll('#account button')].find((b) => b.textContent === 'Sign out');
  out.devOffered = !!document.querySelector('#account button[data-dev-token]');
  return JSON.stringify(out);
})()
'@
        $raw = Invoke-PageScript -Url $DevUrl -Route '/default/identity/users' -Token '' -Script $js.Replace('__READER__', $Reed.Token)
        if ($null -eq $raw) { Set-ItResult -Skipped -Because 'no Chrome for the DevTools protocol'; return }
        $seen = $raw | ConvertFrom-Json
        $seen.avatar | Should -Match 'reed · reader'
        $seen.disabled | Should -BeTrue
        $seen.title | Should -Match 'admin only: reed is reader'
        $seen.bob | Should -BeFalse
        $seen.signOut | Should -BeTrue
        $seen.devOffered | Should -BeTrue
        (Invoke-RestMethod "$DevUrl/api/tenants/default/principals").nodes.name | Should -Not -Contain 'bob'
    }
    It 'in mode demo: the DEMO bar, no Use dev token, dev-token is 404 and dev-bootstrap''s token is refused' {
        $t = New-ModeTenant demo
        $null = New-PortalToken -Name root -Bootstrap -Tenants $t
        # A dev-bootstrap left from mode dev: written as the mint writes it, then the tenant moved to demo.
        Set-Content -LiteralPath (Join-Path $t 'default' 'portal.yaml') -Value 'mode: dev'
        $old = InModuleScope Portal -Parameters @{ T = $t } { param($T) New-PortalDevToken -Tenants $T }
        Set-Content -LiteralPath (Join-Path $t 'default' 'portal.yaml') -Value 'mode: demo'
        $p = $null
        try {
            $p = Start-SidePortal $t $old.Token
            Get-Content -LiteralPath "$($p.LogPath).err" -Raw | Should -Match 'mode demo and tokens\.yaml still holds 1 token\(s\) of dev-bootstrap'
            (Invoke-RestMethod "$($p.Url)/api/tenants/default").mode | Should -Be 'demo'
            [int](Invoke-WebRequest "$($p.Url)/api/tenants/default/dev-token" -SkipHttpErrorCheck).StatusCode | Should -Be 404
            $r = Invoke-WebRequest "$($p.Url)/api/tenants/default/auth/whoami" -Headers @{ Authorization = "Bearer $($old.Token)" } -SkipHttpErrorCheck
            [int]$r.StatusCode | Should -Be 401
            $r.Content | Should -Match 'work only while tenant default''s mode is dev'
            $js = @'
(async () => {
  const wait = async (f, what) => { for (let i = 0; i < 100; i++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 100)); } throw new Error('timed out waiting for ' + what); };
  const avatar = document.getElementById('avatar');
  await wait(() => avatar.dataset.principal, 'the avatar');
  const bar = document.getElementById('modebar');
  const out = { bar: bar.hidden ? null : bar.textContent, badge: document.getElementById('mode').textContent, principal: avatar.dataset.principal, stored: sessionStorage.getItem('portal.token') };
  avatar.click();
  await wait(() => document.querySelector('#account form.pt-token-form'), 'the sign-in form');
  out.devOffered = !!document.querySelector('#account button[data-dev-token]');
  return JSON.stringify(out);
})()
'@
            # The tab still holds the dev token from mode dev: the page drops it and reads as anonymous.
            $raw = Invoke-PageScript -Url $p.Url -Route '/default/identity/users' -Token $old.Token -Script $js
            if ($null -eq $raw) { Set-ItResult -Skipped -Because 'no Chrome for the DevTools protocol'; return }
            $seen = $raw | ConvertFrom-Json
            $seen.bar | Should -BeExactly 'DEMO'
            $seen.badge | Should -Be 'demo'
            $seen.principal | Should -Be 'anonymous'
            $seen.stored | Should -BeNullOrEmpty
            $seen.devOffered | Should -BeFalse
        }
        finally { Stop-SidePortal $p }
    }
    It 'in mode prod the page shows no bar, no badge and no Use dev token' {
        $t = New-ModeTenant prod
        $p = $null
        try {
            $p = Start-SidePortal $t ''

            (Invoke-RestMethod "$($p.Url)/api/tenants/default").mode | Should -Be 'prod'
            $dom = Get-PageDom $p.Url '/default/identity/users'
            if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
            $dom | Should -Match '<p class="pt-modebar" id="modebar" role="status" hidden="" data-mode="prod"></p>'
            $dom | Should -Match '<span class="pt-mode" id="mode" hidden="" data-mode="prod"'
            $js = @'
(async () => {
  const wait = async (f, what) => { for (let i = 0; i < 100; i++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 100)); } throw new Error('timed out waiting for ' + what); };
  const avatar = document.getElementById('avatar');
  await wait(() => avatar.dataset.principal, 'the avatar');
  avatar.click();
  await wait(() => document.querySelector('#account form.pt-token-form'), 'the sign-in form');
  return JSON.stringify({ devOffered: !!document.querySelector('#account button[data-dev-token]'), devBox: !!document.querySelector('#account .pt-dev-signin') });
})()
'@
            $seen = Invoke-PageScript -Url $p.Url -Route '/default/identity/users' -Token '' -Script $js | ConvertFrom-Json
            $seen.devOffered | Should -BeFalse
            $seen.devBox | Should -BeFalse
        }
        finally { Stop-SidePortal $p }
    }
}

Describe 'Feeds' {
    # docs/design.md 40. A tenant of its own: admin root (-Bootstrap) and reader reed, its two feeds pointed at a port
    # nothing listens on (so a push is 502 whatever runs on 8201 and 8202), and a ledger that already holds one push.
    BeforeAll {
        $script:FeedTenants = Join-Path $TestDrive 'feed-tenants'
        $td = Join-Path $FeedTenants 'default'
        $null = New-Item -ItemType Directory -Force -Path $td
        Set-Content -LiteralPath (Join-Path $td 'portal.yaml') -Encoding utf8NoBOM -Value @'
feeds:
  - { id: cards-oci, url: "http://127.0.0.1:1/cards" }
  - { id: cards-nuget, url: "http://127.0.0.1:1/v3/index.json" }
'@
        $script:FeedRoot = New-PortalToken -Name root -Bootstrap -Tenants $FeedTenants
        Add-Content -LiteralPath (Join-Path $td 'principals.yaml') -Encoding utf8NoBOM -Value (
            "  - id: 'ui:principal:reed'`n    name: 'reed'`n    type: user`n    role: reader`n    created: '2026-10-11T00:00:00.000Z'`n    createdBy: 'ui:principal:root'")
        $script:FeedReed = New-PortalToken -Name reed -Tenants $FeedTenants
        $script:Ledger = Join-Path $td 'ledger.jsonl'
        $script:SeedHash = 'sha256:' + ('ab' * 32)
        Set-Content -LiteralPath $Ledger -Encoding utf8NoBOM -Value ('{"at":"2026-10-11T00:00:00.000Z","principal":"ui:principal:root","name":"root","role":"admin","feed":"cards-oci","kind":"oci",' +
            '"card":"Tool.Git","version":"0.1.0","type":"tool","hash":"' + $SeedHash + '","digest":"sha256:' + ('cd' * 32) + '","files":2,"result":"pushed","message":"Tool.Git 0.1.0 to cards-oci"}')
        # A card folder: the fixture's two files, a nested one, and a dot file Publish-PortalCard leaves out.
        $script:CardDir = Join-Path $TestDrive 'card'
        Copy-Item -LiteralPath (Join-Path $Fixtures 'cards' 'Tool.Git') -Destination $CardDir -Recurse
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $CardDir 'docs')
        Set-Content -LiteralPath (Join-Path $CardDir 'docs' 'read me.md') -Value '# Tool.Git' -Encoding utf8NoBOM
        Set-Content -LiteralPath (Join-Path $CardDir '.notes') -Value 'not sent' -Encoding utf8NoBOM
        (Get-Content -LiteralPath (Join-Path $CardDir 'manifest.yaml') -Raw).Replace('version: 0.1.0', 'version: 0.2.0') |
            Set-Content -LiteralPath (Join-Path $CardDir 'manifest.yaml') -NoNewline -Encoding utf8NoBOM
        $script:FeedWork = Join-Path $TestDrive 'feeds'
        # The portal starts without the NuGet key, whatever the runner's environment holds.
        $script:SavedNuGetKey = $env:PORTAL_NUGET_KEY
        $env:PORTAL_NUGET_KEY = $null
        $script:FeedPortal = Start-Portal -Port 18200 -Artifacts $FeedWork -Filesystem '' -Tenants $FeedTenants
        $script:FeedUrl = $FeedPortal.Url
        function Invoke-AsToken([string] $Token, [scriptblock] $Block) {
            $saved = $env:PORTAL_TOKEN
            try { $env:PORTAL_TOKEN = $Token; & $Block } finally { $env:PORTAL_TOKEN = $saved }
        }
        function Get-LedgerLines { @(Get-Content -LiteralPath $Ledger | Where-Object { $_ } | ForEach-Object { $_ | ConvertFrom-Json -DateKind String }) }
    }
    AfterAll {
        Stop-Portal -Artifacts $FeedWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 }
        $env:PORTAL_NUGET_KEY = $SavedNuGetKey
    }

    It 'GET .../feeds is the two feeds of the merged config and the card the ledger holds, as a graph' {
        $env = Invoke-RestMethod "$FeedUrl/api/tenants/default/feeds"
        $env.module | Should -Be 'Portal'
        @($env.nodes | Where-Object kind -EQ 'Feed' | ForEach-Object id) | Should -Be @('ui:feed:cards-oci', 'ui:feed:cards-nuget')
        $oci = $env.nodes | Where-Object id -EQ 'ui:feed:cards-oci'
        $oci.properties.kind | Should -Be 'oci'
        $oci.properties.url | Should -Be 'http://127.0.0.1:1/cards'
        $oci.properties.visibility | Should -Be 'private'
        ($env.nodes | Where-Object id -EQ 'ui:feed:cards-nuget').properties.tokenEnv | Should -Be 'PORTAL_NUGET_KEY'
        $card = $env.nodes | Where-Object id -EQ 'ui:card:cards-oci/Tool.Git@0.1.0'
        $card.properties.hash | Should -Be $SeedHash
        @($env.edges.id) | Should -Be @('ui:feed:cards-oci|ui:card:cards-oci/Tool.Git@0.1.0|has-card')
        $file = Join-Path $TestDrive 'feeds.graph.json'
        $env | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $file -Encoding utf8NoBOM
        & $GraphNodeBin validate $file | Out-Null
        $LASTEXITCODE | Should -Be 0
    }
    It 'Publish-PortalCard is 401 without a token, 403 for a reader and 404 for a feed the tenant lacks, writing no ledger line' {
        $before = (Get-LedgerLines).Count
        Invoke-AsToken '' { { Publish-PortalCard -Feed cards-oci -Path $CardDir -Url $FeedUrl } | Should -Throw '*-> 401*' }
        Invoke-AsToken $FeedReed.Token { { Publish-PortalCard -Feed cards-oci -Path $CardDir -Url $FeedUrl } | Should -Throw '*-> 403*reader*' }
        Invoke-AsToken $FeedRoot.Token { { Publish-PortalCard -Feed nope -Path $CardDir -Url $FeedUrl } | Should -Throw '*-> 404*no such feed*' }
        Invoke-AsToken $FeedRoot.Token { Publish-PortalCard -Feed cards-oci -Path $CardDir -Url $FeedUrl -WhatIf }
        (Get-LedgerLines).Count | Should -Be $before
    }
    It 'Publish-PortalCard sends the card''s files by path; a feed that is down is 502 and one failed ledger line' {
        Invoke-AsToken $FeedRoot.Token { { Publish-PortalCard -Feed cards-oci -Path $CardDir -Url $FeedUrl } | Should -Throw '*-> 502*cards-oci*' }
        $e = (Get-LedgerLines)[-1]
        $e.result | Should -Be 'failed'
        $e.principal | Should -Be 'ui:principal:root'
        $e.role | Should -Be 'admin'
        $e.feed | Should -Be 'cards-oci'
        $e.card | Should -Be 'Tool.Git'
        $e.version | Should -Be '0.2.0'
        $e.files | Should -Be 3 -Because 'manifest.yaml, install.yaml and docs/read me.md; .notes is left out'
        $e.hash | Should -Match '^sha256:[0-9a-f]{64}$'
        $e.digest | Should -BeNullOrEmpty
        @($e.PSObject.Properties.Name) | Should -Be @('at', 'principal', 'name', 'role', 'feed', 'kind', 'card', 'version', 'type', 'hash', 'digest', 'files', 'result', 'message', 'seq', 'prev')
        # Chained (docs/design.md 45): seq is the line number, prev the SHA-256 of the line before.
        $lines = @(Get-Content -LiteralPath $Ledger | Where-Object { $_ })
        $e.seq | Should -Be $lines.Count
        $e.prev | Should -Be ([Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($lines[-2]))).ToLowerInvariant())
    }
    It 'Publish-PortalCard sends a non-ASCII path as .NET encodes it, and the portal takes it as the path (docs/design.md 52)' {
        $uni = Join-Path $TestDrive 'unicode-card'
        Copy-Item -LiteralPath (Join-Path $Fixtures 'cards' 'Tool.Git') -Destination $uni -Recurse
        $hcl = Join-Path $uni 'tests' 'fixtures' 'hcl' 'enc-日本'
        $null = New-Item -ItemType Directory -Force -Path $hcl
        Set-Content -LiteralPath (Join-Path $hcl 'main.tf') -Value 'terraform {}' -Encoding utf8NoBOM
        (Get-Content -LiteralPath (Join-Path $uni 'manifest.yaml') -Raw).Replace('version: 0.1.0', 'version: 0.2.1') |
            Set-Content -LiteralPath (Join-Path $uni 'manifest.yaml') -NoNewline -Encoding utf8NoBOM
        # What .NET puts on the wire for that filename: the encoded word Push-AllCards was refused for in 0.14.1.
        $cd = [System.Net.Http.Headers.ContentDispositionHeaderValue]::new('form-data')
        $cd.FileName = '"tests/fixtures/hcl/enc-日本/main.tf"'
        "$cd" | Should -Match 'filename="=\?utf-8\?B\?dGVzdHMvZml4dHVyZXMvaGNsL2VuYy3ml6XmnKwvbWFpbi50Zg==\?="'
        Invoke-AsToken $FeedRoot.Token { { Publish-PortalCard -Feed cards-oci -Path $uni -Url $FeedUrl } | Should -Throw '*-> 502*cards-oci*' }
        $e = (Get-LedgerLines)[-1]
        $e.result | Should -Be 'failed' -Because 'the card passed its checks and only the feed, which is down, failed it'
        $e.version | Should -Be '0.2.1'
        $e.files | Should -Be 3 -Because 'manifest.yaml, install.yaml and tests/fixtures/hcl/enc-日本/main.tf'
    }
    It 'a card that fails Graph.Node''s manifest schema is 422 with its problems, and an invalid ledger line' {
        $bad = Join-Path $TestDrive 'bad-card'
        $null = New-Item -ItemType Directory -Force -Path $bad
        Set-Content -LiteralPath (Join-Path $bad 'manifest.yaml') -Value "name: Tool.Git`nversion: one`ntype: tool`ndescription: x`ncolour: red" -Encoding utf8NoBOM
        $saved = $env:PORTAL_TOKEN
        $err = $null
        try {
            $env:PORTAL_TOKEN = $FeedRoot.Token
            try { Publish-PortalCard -Feed cards-nuget -Path $bad -Url $FeedUrl } catch { $err = $_ }
        }
        finally { $env:PORTAL_TOKEN = $saved }
        "$err" | Should -Match '-> 422'
        "$err" | Should -Match 'schema manifest\.yaml:/version'
        $e = (Get-LedgerLines)[-1]
        $e.result | Should -Be 'invalid'
        $e.feed | Should -Be 'cards-nuget'
        $e.hash | Should -BeNullOrEmpty
    }
    It 'pushing the version the ledger holds is 409 before the feed is asked' {
        $dup = Join-Path $TestDrive 'dup-card'
        Copy-Item -LiteralPath (Join-Path $Fixtures 'cards' 'Tool.Git') -Destination $dup -Recurse
        Invoke-AsToken $FeedRoot.Token { { Publish-PortalCard -Feed cards-oci -Path $dup -Url $FeedUrl } | Should -Throw '*-> 409*already in cards-oci*' }
        (Get-LedgerLines)[-1].result | Should -Be 'duplicate'
    }
    It 'Artifacts/Feeds draws each feed and the cards it holds, from the graph, with no write control' {
        $dom = Get-PageDom $FeedUrl '/default/artifacts/feeds'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match 'data-feed="ui:feed:cards-oci"'
        $dom | Should -Match 'data-feed="ui:feed:cards-nuget"'
        $dom | Should -Match '<tr data-card="ui:card:cards-oci/Tool.Git@0.1.0"><td>Tool.Git</td><td>0.1.0</td><td>tool</td><td><code[^>]*>abababababab</code></td>'
        $dom | Should -Match 'No cards pushed yet\.'
        $dom | Should -Match '2 feeds · 1 cards'
        $feeds = [regex]::Match($dom, '(?s)<div class="pt-feeds">.*').Value
        $feeds | Should -Not -Match '<button'
    }
    It 'ledger.jsonl and the feeds'' data/ are gitignored' {
        foreach ($f in 'tenants/default/ledger.jsonl', 'tenants/default/data/registry/x') {
            & git -C $Repo check-ignore -q $f
            $LASTEXITCODE | Should -Be 0 -Because $f
        }
    }
    It 'compose.yaml pins every image, publishes only the two feeds and only on 127.0.0.1, and keeps data under the tenant' {
        $text = Get-Content -LiteralPath (Join-Path $Repo 'compose.yaml') -Raw
        $images = @([regex]::Matches($text, '(?m)^\s+image:\s*(\S+)\s*$') | ForEach-Object { $_.Groups[1].Value })
        $images | Should -Be @('registry:2.8.3', 'bagetter/bagetter:1.6.5', 'postgres:16.15-alpine')
        $ports = @([regex]::Matches($text, '(?m)^\s+- "([^"/]*:\d+)"\s*$') | ForEach-Object { $_.Groups[1].Value })
        $ports | Should -Be @('127.0.0.1:${PORTAL_REGISTRY_PORT:-8201}:5000', '127.0.0.1:${PORTAL_NUGET_PORT:-8202}:8080')
        $postgres = [regex]::Match($text, '(?s)\n  postgres:\r?\n(.*)$').Groups[1].Value
        $postgres | Should -Not -Match 'ports:'
        $postgres | Should -Match 'pg_isready'
        $postgres | Should -Match 'POSTGRES_DB: portal'
        @([regex]::Matches($text, '(?m)^\s+- "\$\{PORTAL_FEEDS_DATA[^}]*\}/\w+:')).Count | Should -Be 3
        $text | Should -Match '(?m)^name: portal-\$\{PORTAL_TENANT:-default\}\s*$'
    }
    It 'Invoke-Build Feeds refuses in mode prod before it builds, starts or writes anything' {
        $t = Join-Path $TestDrive "feeds-prod-$([guid]::NewGuid().ToString('n'))"
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $t 'default')
        Set-Content -LiteralPath (Join-Path $t 'default' 'portal.yaml') -Value 'mode: prod' -Encoding utf8NoBOM
        $envFile = Join-Path $Repo '.env'
        $envBefore = if (Test-Path -LiteralPath $envFile) { (Get-FileHash -LiteralPath $envFile).Hash } else { 'absent' }
        $saved = @{ PORTAL_TENANTS = $env:PORTAL_TENANTS; PORTAL_TENANT = $env:PORTAL_TENANT }
        $err = $null
        try {
            $env:PORTAL_TENANTS = $t; $env:PORTAL_TENANT = $null
            try { Invoke-Build Feeds -File (Join-Path $Repo '.build.ps1') *> $null } catch { $err = "$_" }
        }
        finally { foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) } }
        $err | Should -Match 'Feeds refuses: tenant default is in mode prod'
        Join-Path $t 'default' 'data' | Should -Not -Exist
        $envAfter = if (Test-Path -LiteralPath $envFile) { (Get-FileHash -LiteralPath $envFile).Hash } else { 'absent' }
        $envAfter | Should -Be $envBefore
        (Get-Content -LiteralPath (Join-Path $Repo '.build.ps1') -Raw) | Should -Match '(?m)^task Feeds FeedsGate, Build, \{'
    }
    It 'Initialize-PortalFeedEnv keeps the secrets .env holds and writes only those it lacks' {
        $envFile = Join-Path $TestDrive 'feeds.env'
        Set-Content -LiteralPath $envFile -Value "PORTAL_PORT=8200`nPORTAL_NUGET_KEY=kept" -NoNewline -Encoding utf8NoBOM
        $saved = @{ PORTAL_NUGET_KEY = $env:PORTAL_NUGET_KEY; PORTAL_POSTGRES_PASSWORD = $env:PORTAL_POSTGRES_PASSWORD; PORTAL_TENANT = $env:PORTAL_TENANT; PORTAL_FEEDS_DATA = $env:PORTAL_FEEDS_DATA }
        try {
            $env:PORTAL_NUGET_KEY = $null; $env:PORTAL_POSTGRES_PASSWORD = $null
            $r = InModuleScope Portal -Parameters @{ E = $envFile; T = (Join-Path $TestDrive 'fe-tenants') } { param($E, $T) Initialize-PortalFeedEnv -Tenant acme -Tenants $T -EnvFile $E }
            @($r.Written) | Should -Be @('PORTAL_POSTGRES_PASSWORD')
            $env:PORTAL_NUGET_KEY | Should -Be 'kept'
            $env:PORTAL_POSTGRES_PASSWORD | Should -Match '^[0-9a-f]{64}$'
            $env:PORTAL_FEEDS_DATA | Should -Be (Join-Path $TestDrive 'fe-tenants' 'acme' 'data')
            Join-Path $TestDrive 'fe-tenants' 'acme' 'data' | Should -Exist
            @(Get-Content -LiteralPath $envFile) | Should -Be @('PORTAL_PORT=8200', 'PORTAL_NUGET_KEY=kept', "PORTAL_POSTGRES_PASSWORD=$env:PORTAL_POSTGRES_PASSWORD")
        }
        finally { foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) } }
    }

    # Last in the block: it replaces the block's portal on 18200 (same artifacts) with one Invoke-PortalUp starts.
    It 'Up reads PORTAL_NUGET_KEY from .env as Feeds does, never writing it: a NuGet push goes from 422 no_token to the feed' {
        Invoke-AsToken $FeedRoot.Token { { Publish-PortalCard -Feed cards-nuget -Path $CardDir -Url $FeedUrl } | Should -Throw '*-> 422*PORTAL_NUGET_KEY, which is not set*' }
        $envFile = Join-Path $TestDrive 'up.env'
        Set-Content -LiteralPath $envFile -Value "PORTAL_PORT=8200`nexport PORTAL_NUGET_KEY='up-key'" -Encoding utf8NoBOM
        $before = (Get-FileHash -LiteralPath $envFile).Hash
        $saved = @{ PORTAL_TENANTS = $env:PORTAL_TENANTS; PORTAL_NUGET_KEY = $env:PORTAL_NUGET_KEY }
        try {
            $env:PORTAL_TENANTS = $FeedTenants; $env:PORTAL_NUGET_KEY = $null
            $p = InModuleScope Portal -Parameters @{ A = $FeedWork; E = $envFile } { param($A, $E) Invoke-PortalUp -Port 18200 -Artifacts $A -Filesystem '' -EnvFile $E }
            $p.Replaced | Should -Not -BeNullOrEmpty
            $env:PORTAL_NUGET_KEY | Should -Be 'up-key'
        }
        finally { foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) } }
        (Get-FileHash -LiteralPath $envFile).Hash | Should -Be $before
        InModuleScope Portal -Parameters @{ E = $envFile } { param($E) Get-PortalEnvValue -Name PORTAL_POSTGRES_PASSWORD -EnvFile $E } | Should -BeNullOrEmpty
        Invoke-AsToken $FeedRoot.Token { { Publish-PortalCard -Feed cards-nuget -Path $CardDir -Url $p.Url } | Should -Throw '*-> 502*cards-nuget*' }
        (Get-LedgerLines)[-1].message | Should -Not -Match 'not set'
    }
}

Describe 'Emitters' -Tag 'Emitters' {
    # docs/design.md 44, 49. A tenant of its own: admin root (-Bootstrap) and a ledger of nine chained pushes to cards-oci,
    # one per emitter card of tests/fixtures/cards (Portal.Emitters v0.2.0), each line carrying the card's module, repo,
    # url and emits as a 0.13.0 push writes them. The FileSystem fixture is mounted at start as it is, with no source
    # block: unstamped, under its module, never the card Emitter.FileSystem whose module that is. root mounts copies of
    # four real fixtures with Graph.Node's source block inserted as each emitter's code writes it (no layer on disk is
    # stamped yet): agent-runs Emitter.Agent 0.1.4 and ledger Emitter.Ledger 0.3.0 (ok), agent-environment
    # Emitter.Environment 0.3.0 and agent-alerts Emitter.Environment 0.4.0 (version-mismatch), and the broken ledger copy
    # as ledger-old, Emitter.Unlisted 1.0.0, which no card names (no-card). The other six cards have no layer (no-layers).
    BeforeAll {
        $script:EmTenants = Join-Path $TestDrive 'em-tenants'
        $td = Join-Path $EmTenants 'default'
        $null = New-Item -ItemType Directory -Force -Path $td
        Set-Content -LiteralPath (Join-Path $td 'portal.yaml') -Encoding utf8NoBOM -Value '{}'
        $script:EmRoot = New-PortalToken -Name root -Bootstrap -Tenants $EmTenants
        # Reads a card's top-level scalars and its emits, which every fixture card writes as one flow map per line.
        function Read-Card([string] $Name) {
            $text = Get-Content -LiteralPath (Join-Path $Fixtures 'cards' $Name 'manifest.yaml') -Raw
            $card = [ordered]@{}
            foreach ($k in 'name', 'version', 'type', 'module', 'repo', 'url') { $card[$k] = [regex]::Match($text, "(?m)^${k}: (\S+)\r?$").Groups[1].Value }
            $block = [regex]::Match($text, '(?ms)^emits:\r?\n(.*?)(?=^\S)').Groups[1].Value
            $card.emits = @($block -split '\r?\n' | Where-Object { $_ } | ForEach-Object {
                    $m = [regex]::Match($_, '^  - \{ facet: ([a-z0-9-]+), kinds: \[([^\]]+)\] \}$')
                    $m.Success | Should -BeTrue -Because "$Name emits line '$_'"
                    [ordered]@{ facet = $m.Groups[1].Value; kinds = @($m.Groups[2].Value -split ', ') }
                })
            $card
        }
        $script:EmCards = [ordered]@{ 'Emitter.Agent' = '0.1.4'; 'Emitter.AzureDevOps' = '0.3.1'; 'Emitter.Environment' = '0.3.0'; 'Emitter.FileSystem' = '0.2.0'
            'Emitter.Git' = '0.1.1'; 'Emitter.Ledger' = '0.3.0'; 'Emitter.Network' = '0.4.0'; 'Emitter.Reference' = '0.1.1'; 'Emitter.Terraform' = '0.17.1' }
        $lines = [System.Collections.Generic.List[string]]::new()
        $prev = '0' * 64
        $seq = 0
        foreach ($name in $EmCards.Keys) {
            $c = Read-Card $name
            $c.name | Should -Be $name
            $c.version | Should -Be $EmCards[$name]
            $c.type | Should -Be 'emitter'
            $seq++
            $line = [ordered]@{ at = "2026-10-11T00:00:0$($seq - 1).000Z"; principal = 'ui:principal:root'; name = 'root'; role = 'admin'; feed = 'cards-oci'; kind = 'oci'
                card = $name; version = $c.version; type = 'emitter'; hash = 'sha256:' + ('{0:x2}' -f $seq) * 32; digest = 'sha256:' + ('cd' * 32); files = 1; result = 'pushed'
                message = "$name $($c.version) to cards-oci"; seq = $seq; prev = $prev; module = $c.module; repo = $c.repo; url = $c.url; emits = $c.emits } | ConvertTo-Json -Compress -Depth 5
            $lines.Add($line)
            $prev = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($line))).ToLowerInvariant()
        }
        Set-Content -LiteralPath (Join-Path $td 'ledger.jsonl') -Encoding utf8NoBOM -Value $lines
        # Copies a fixture layer folder to TestDrive with the source block inserted after the envelope's opening brace;
        # its stamp is the layer's own, as Portal.Agent's builders write it. Nothing else in the file changes.
        function New-StampedLayer([string] $Dir, [string] $Emitter, [string] $Version, [string] $Target) {
            $from = Join-Path $Fixtures 'layers' $Dir
            $layer = @(Get-ChildItem -LiteralPath $from -Filter '*.graph.json')
            $layer.Count | Should -Be 1
            $text = Get-Content -LiteralPath $layer[0].FullName -Raw
            $text | Should -Not -Match '"source"\s*:\s*\{'
            $stamp = [regex]::Match($text, '"layer"\s*:\s*"([^"]+)"').Groups[1].Value
            $src = [ordered]@{ emitter = $Emitter; version = $Version; target = $Target; stamp = $stamp } | ConvertTo-Json -Compress
            $to = Join-Path $TestDrive 'stamped' $Dir
            $null = New-Item -ItemType Directory -Force -Path $to
            $i = $text.IndexOf('{')
            Set-Content -LiteralPath (Join-Path $to $layer[0].Name) -NoNewline -Encoding utf8NoBOM -Value ($text.Substring(0, $i + 1) + '"source":' + $src + ',' + $text.Substring($i + 1))
            Copy-Item -LiteralPath (Join-Path $from 'ontology.yaml') -Destination $to
            $to
        }
        $script:EmWork = Join-Path $TestDrive 'emitters'
        $script:EmPortal = Start-Portal -Port 18200 -Artifacts $EmWork -Filesystem $FsFixture -Tenants $EmTenants
        $script:EmUrl = $EmPortal.Url
        $saved = $env:PORTAL_TOKEN
        try {
            $env:PORTAL_TOKEN = $EmRoot.Token
            $script:EmLayer = Mount-PortalLayer -Name agent-runs -Path (New-StampedLayer agent-runs Emitter.Agent 0.1.4 ledger.jsonl) -Url $EmUrl
            $null = Mount-PortalLayer -Name ledger -Path (New-StampedLayer ledger Emitter.Ledger 0.3.0 default) -Url $EmUrl
            $null = Mount-PortalLayer -Name agent-environment -Path (New-StampedLayer agent-environment Emitter.Environment 0.3.0 default) -Url $EmUrl
            $null = Mount-PortalLayer -Name agent-alerts -Path (New-StampedLayer agent-alerts Emitter.Environment 0.4.0 default) -Url $EmUrl
            $null = Mount-PortalLayer -Name ledger-old -Path (New-StampedLayer ledger-broken Emitter.Unlisted 1.0.0 default) -Url $EmUrl
        }
        finally { $env:PORTAL_TOKEN = $saved }
        $script:EnvCard = 'ui:card:cards-oci/Emitter.Environment@0.3.0'
    }
    AfterAll { Stop-Portal -Artifacts $EmWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'the roll call has one row per card or source.emitter name, and the unstamped module, each with its status' {
        $EmLayer.mountedBy | Should -Be 'ui:principal:root'
        $api = Invoke-RestMethod "$EmUrl/api/tenants/default/emitters"
        @($api.nodes | Where-Object { $_.kind -in 'Card', 'Emitter' } | ForEach-Object { "$($_.name) $($_.properties.status) $($_.properties.layers) $($_.id)" }) | Should -Be @(
            'Emitter.Agent ok 1 ui:card:cards-oci/Emitter.Agent@0.1.4'
            'Emitter.AzureDevOps no-layers 0 ui:card:cards-oci/Emitter.AzureDevOps@0.3.1'
            "Emitter.Environment version-mismatch 2 $EnvCard"
            'Emitter.FileSystem no-layers 0 ui:card:cards-oci/Emitter.FileSystem@0.2.0'
            'Emitter.Git no-layers 0 ui:card:cards-oci/Emitter.Git@0.1.1'
            'Emitter.Ledger ok 1 ui:card:cards-oci/Emitter.Ledger@0.3.0'
            'Emitter.Network no-layers 0 ui:card:cards-oci/Emitter.Network@0.4.0'
            'Emitter.Reference no-layers 0 ui:card:cards-oci/Emitter.Reference@0.1.1'
            'Emitter.Terraform no-layers 0 ui:card:cards-oci/Emitter.Terraform@0.17.1'
            'Emitter.Unlisted no-card 1 ui:emitter:Emitter.Unlisted'
            'Graph.Emitter.FileSystem unstamped 1 ui:module:Graph.Emitter.FileSystem')
        $envCardNode = $api.nodes | Where-Object id -EQ $EnvCard
        $envCardNode.properties.layerVersions | Should -Be @('0.3.0', '0.4.0')
        $envCardNode.properties.module | Should -Be 'Claude.Agent.Orchestrator'
        @($envCardNode.properties.emits.facet) | Should -Be @('environment', 'alerts')
        @($api.edges | Where-Object kind -EQ 'produced-by' | ForEach-Object { "$($_.from) $($_.to)" } | Sort-Object) | Should -Be @(
            "ui:layer:agent-alerts $EnvCard"
            "ui:layer:agent-environment $EnvCard"
            'ui:layer:agent-runs ui:card:cards-oci/Emitter.Agent@0.1.4'
            'ui:layer:filesystem ui:module:Graph.Emitter.FileSystem'
            'ui:layer:ledger ui:card:cards-oci/Emitter.Ledger@0.3.0'
            'ui:layer:ledger-old ui:emitter:Emitter.Unlisted')
        # Nothing maps a module onto a card: Graph.Emitter.Agent is the card's module, not an emitter's name.
        [int](Invoke-WebRequest "$EmUrl/api/tenants/default/emitters/Graph.Emitter.Agent" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        [int](Invoke-WebRequest "$EmUrl/default/emitters/Graph.Emitter.Terraform" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        [int](Invoke-WebRequest "$EmUrl/default/emitters/Emitter.Terraform" -SkipHttpErrorCheck).StatusCode | Should -Be 200
    }
    It 'Emitters/Catalogue draws each status, the versions on both sides of a mismatch, the facets and the card links, with status and facet chips' {
        $dom = Get-PageDom $EmUrl '/default/emitters'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match 'href="/default/emitters" class="pt-page" data-page="ui:rail:emitters:catalogue" aria-current="page"'
        $dom | Should -Match '<span class="pt-count">11 emitters · 2 ok · 1 version-mismatch · 6 no-layers · 1 no-card · 1 unstamped</span>'
        $dom | Should -Match ('<tr data-emitter="Emitter.Environment" data-status="version-mismatch"><td><a href="/default/emitters/Emitter.Environment">Emitter.Environment</a></td>' +
            '<td><span class="pt-status" data-status="version-mismatch" title="[^"]+">version-mismatch</span></td><td>0.3.0</td><td class="pt-mismatch">0.3.0, 0.4.0</td><td>environment, alerts</td>' +
            '<td><a href="/default/artifacts/feeds#' + [regex]::Escape([uri]::EscapeDataString($EnvCard)) + '" class="pt-card-link" data-card="' + [regex]::Escape($EnvCard) + '">Emitter.Environment@0.3.0</a></td><td>2</td><td>10</td></tr>')
        $dom | Should -Match '<tr data-emitter="Emitter.Agent" data-status="ok">.*?<td>0.1.4</td><td>0.1.4</td><td>instructions, runs</td>'
        $dom | Should -Match '<tr data-emitter="Emitter.Terraform" data-status="no-layers">.*?<td>0.17.1</td><td></td><td>schema, resources, variables, modules</td>.*?<td>0</td><td>0</td></tr>'
        $dom | Should -Match '<tr data-emitter="Emitter.Unlisted" data-status="no-card"><td><a href="/default/emitters/Emitter.Unlisted">Emitter.Unlisted</a></td><td><span class="pt-status" data-status="no-card"[^>]*>no-card</span></td><td></td><td>1.0.0</td><td></td><td></td><td>1</td><td>12</td></tr>'
        $dom | Should -Match '<tr data-emitter="Graph.Emitter.FileSystem" data-status="unstamped"><td><a href="/default/emitters/Graph.Emitter.FileSystem">Graph.Emitter.FileSystem</a></td><td><span class="pt-status" data-status="unstamped"[^>]*>unstamped</span></td><td></td><td></td><td></td><td></td><td>1</td><td>0</td></tr>'
        $dom | Should -Match '<div class="pt-chips" role="group" aria-label="filter by status" data-chips="status">'
        $dom | Should -Match '<div class="pt-chips" role="group" aria-label="filter by facet" data-chips="facet">'
        foreach ($f in 'alerts', 'environment', 'filesystem', 'host', 'instructions', 'ledger', 'modules', 'organization', 'references', 'repository', 'resources', 'runs', 'schema', 'variables') {
            $dom | Should -Match "<button type=`"button`" class=`"pt-chip`" data-value=`"$f`" aria-pressed=`"false`">$f</button>"
        }
        # A chip's filter is in the URL: the list drawn from it shows the matching rows only.
        $dom = Get-PageDom $EmUrl '/default/emitters?status=no-layers'
        $dom | Should -Match '<span class="pt-count">6 of 11 emitters · '
        $dom | Should -Match 'data-value="no-layers" aria-pressed="true"'
        $dom | Should -Match '<tr data-emitter="Emitter.Agent" data-status="ok" hidden="">'
        $dom | Should -Match '<tr data-emitter="Emitter.Git" data-status="no-layers">'
        $dom = Get-PageDom $EmUrl '/default/emitters?facet=alerts'
        $dom | Should -Match '<span class="pt-count">1 of 11 emitters · '
        $dom | Should -Match '<tr data-emitter="Emitter.Environment" data-status="version-mismatch">'
        # The card link lands on its row of the feeds page, marked.
        $dom = Get-PageDom $EmUrl "/default/artifacts/feeds#$([uri]::EscapeDataString($EnvCard))"
        $dom | Should -Match ('<tr data-card="' + [regex]::Escape($EnvCard) + '" class="pt-target" aria-current="true">')
        # The emitters service's other pages keep their paths.
        $dom = Get-PageDom $EmUrl '/default/emitters/sources'
        $dom | Should -Match '<h1 class="pt-title" id="title"[^>]*>Sources</h1>'
    }
    It 'an emitter''s page is its source, built from the card, with its layers'' source version, target and stamp' {
        $dom = Get-PageDom $EmUrl '/default/emitters/Emitter.Environment'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<h1 class="pt-title" id="title"[^>]*>Emitter.Environment</h1>'
        $dom | Should -Match '<span class="pt-badge" id="emitter-stats"[^>]*>2 layers</span>'
        $dom | Should -Match '<dd data-field="module">Claude.Agent.Orchestrator</dd>'
        # Show graph beside the version: the tenant graph filtered to the card's name (docs/design.md 53).
        $dom | Should -Match '<dd data-field="version">0\.3\.0<a href="/default/graph\?source=Emitter\.Environment" class="pt-show-graph" data-source="Emitter\.Environment" title="10 nodes in the tenant graph from the layers whose source block names Emitter\.Environment">Show graph</a></dd>'
        $dom | Should -Match '<dd data-field="layer versions"><span class="pt-mismatch">0.3.0, 0.4.0</span></dd>'
        $dom | Should -Match '<dd data-field="repo"><a href="https://github.com/JerryBalmer1/Portal.Agent" target="_blank" rel="noopener noreferrer">Portal.Agent</a></dd>'
        $dom | Should -Match '<dd data-field="url"><a href="https://github.com/JerryBalmer1/Portal.Agent" target="_blank" rel="noopener noreferrer" class="pt-open-source">Open in source</a></dd>'
        $dom | Should -Match ('<dd data-field="card"><a href="/default/artifacts/feeds#[^"]+" class="pt-card-link" data-card="' + [regex]::Escape($EnvCard) + '">Emitter.Environment@0.3.0</a></dd>')
        $dom | Should -Match '<tr data-facet="environment"><td>environment</td><td><span class="pt-kind">environment.instance</span><span class="pt-kind">environment.image</span><span class="pt-kind">environment.network</span></td></tr>'
        $dom | Should -Match '<tr data-facet="alerts"><td>alerts</td><td><span class="pt-kind">alert</span></td></tr>'
        $dom | Should -Match '<tr data-layer="agent-alerts" title="Claude.Agent.Orchestrator 0.3.0" class="pt-mismatch"><td><a href="/default/layers/agent-alerts">agent-alerts</a></td><td class="pt-mismatch" title="The card is 0.3.0.">0.4.0</td><td>default</td><td>2026-10-11T19:00:31Z</td><td>\d+</td><td>\d+</td><td>root</td></tr>'
        $dom | Should -Match '<tr data-layer="agent-environment" title="Claude.Agent.Orchestrator 0.3.0"><td><a href="/default/layers/agent-environment">agent-environment</a></td><td>0.3.0</td><td>default</td><td>2026-10-11T19:00:30Z</td>'
        $dom | Should -Match 'href="/default/emitters" class="pt-page" data-page="ui:rail:emitters:catalogue" aria-current="page"'
        $dom = Get-PageDom $EmUrl '/default/emitters/Emitter.Terraform'
        $dom | Should -Match '<dd data-field="module">Graph.Emitter.Terraform</dd>'
        $dom | Should -Match '<dd data-field="version">0\.17\.1<a href="/default/graph\?source=Emitter\.Terraform" class="pt-show-graph" data-source="Emitter\.Terraform" title="0 nodes[^"]*">Show graph</a></dd>'
        $dom | Should -Match '<tr data-facet="variables"><td>variables</td>'
        $dom | Should -Match '<h2>No layers mounted</h2>'
        $dom = Get-PageDom $EmUrl '/default/emitters/Emitter.Unlisted'
        $dom | Should -Match '<dd data-field="emitter">Emitter.Unlisted<a href="/default/graph\?source=Emitter\.Unlisted" class="pt-show-graph" data-source="Emitter\.Unlisted" title="12 nodes[^"]*">Show graph</a></dd><dt>status</dt><dd data-field="status"><span class="pt-status" data-status="no-card"'
        $dom | Should -Match '<tr data-layer="ledger-old" title="Portal.Agent 0.1.0"><td><a href="/default/layers/ledger-old">ledger-old</a></td><td>1.0.0</td><td>default</td>'
        $dom = Get-PageDom $EmUrl '/default/emitters/Graph.Emitter.FileSystem'
        $dom | Should -Match '<dd data-field="module">Graph.Emitter.FileSystem</dd><dt>status</dt><dd data-field="status"><span class="pt-status" data-status="unstamped"'
        $dom | Should -Not -Match 'pt-show-graph'
        $dom | Should -Match '<tr data-layer="filesystem" title="Graph.Emitter.FileSystem 0.1.0"><td><a href="/default/layers/filesystem">filesystem</a></td><td></td><td></td><td>2026-10-09T00:51:29Z</td><td>\d+</td><td>\d+</td><td></td></tr>'
    }
    It 'the roll call counts each emitter''s nodes as GET .../graph?source= serves them, edges pruned to the filtered set' {
        $api = Invoke-RestMethod "$EmUrl/api/tenants/default/emitters"
        @($api.nodes | Where-Object { $_.kind -in 'Card', 'Emitter' } | ForEach-Object { "$($_.name) $($_.properties.nodes)" }) | Should -Be @(
            'Emitter.Agent 12', 'Emitter.AzureDevOps 0', 'Emitter.Environment 10', 'Emitter.FileSystem 0', 'Emitter.Git 0', 'Emitter.Ledger 12',
            'Emitter.Network 0', 'Emitter.Reference 0', 'Emitter.Terraform 0', 'Emitter.Unlisted 12', 'Graph.Emitter.FileSystem 0')
        foreach ($name in 'Emitter.Agent', 'Emitter.Environment', 'Emitter.Ledger', 'Emitter.Unlisted', 'Emitter.Terraform') {
            $g = Invoke-RestMethod "$EmUrl/api/tenants/default/graph?source=$name"
            $n = ($api.nodes | Where-Object { $_.kind -in 'Card', 'Emitter' -and $_.name -eq $name }).properties.nodes
            @($g.nodes).Count | Should -Be $n -Because $name
            $ids = @{}; foreach ($x in $g.nodes) { $ids[$x.id] = $true }
            foreach ($e in $g.edges) { ($ids[$e.from] -and $ids[$e.to]) | Should -BeTrue -Because "$name edge $($e.id)" }
        }
        # Edge pruning: the ledger's records edges into Emitter.Agent's run are in the whole graph, not in Emitter.Ledger's.
        $records = 'ledger:46|agent:run/2026-10-09T02:50:43.7976027Z|records'
        @((Invoke-RestMethod "$EmUrl/api/tenants/default/graph").edges.id) | Should -Contain $records
        $led = Invoke-RestMethod "$EmUrl/api/tenants/default/graph?source=Emitter.Ledger"
        "$(@($led.nodes).Count) $(@($led.edges).Count)" | Should -Be '12 11'
        @($led.edges.id) | Should -Not -Contain $records
        # Exact and case-sensitive.
        @((Invoke-RestMethod "$EmUrl/api/tenants/default/graph?source=emitter.environment").nodes).Count | Should -Be 0
    }
    It 'Catalogue rows open a menu whose Show graph is the tenant graph filtered to the card name, drawn with the filter as a chip that clears it' {
        $wait = 'const wait = async (f) => { for (let t = 0; t < 160; t++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 50)); } throw new Error("timed out: " + f); };'
        $items = 'const items = (m) => [...m.querySelectorAll("[role=menuitem]")].map((x) => `${x.textContent}|${x.getAttribute("href") || ""}|${x.getAttribute("aria-disabled") || ""}`);'
        $r = Invoke-PageScript $EmUrl '/default/emitters' '' ("(async () => { $wait $items " +
            'const menu = async (name) => { const row = await wait(() => document.querySelector(`tr[data-emitter="${name}"]`)); const b = row.cells[1].getBoundingClientRect(); ' +
            'row.cells[1].dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 4, clientY: b.y + 4 })); return wait(() => document.querySelector(".pt-nodemenu")); }; ' +
            'const out = {}; let m = await menu("Graph.Emitter.FileSystem"); out.unstamped = items(m); out.why = [...m.querySelectorAll("[aria-disabled]")].map((x) => x.title); ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); ' +
            'm = await menu("Emitter.Unlisted"); out.unlisted = items(m); m.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); ' +
            'm = await menu("Emitter.Environment"); out.env = items(m); out.label = m.getAttribute("aria-label"); ' +
            'm.querySelector("[data-item=show-graph]").click(); ' +
            'let s = await wait(() => document.querySelector("#graph-stats[data-nodes]")); await wait(() => document.querySelector(".pt-graph-root[data-drawn]")); ' +
            'out.at = location.pathname + location.search; out.stats = s.textContent; out.title = document.getElementById("title").textContent; ' +
            'const chip = document.querySelector(".pt-filter[data-filter=source]"); out.chip = chip ? chip.dataset.value + "|" + chip.querySelector("a").getAttribute("href") : ""; ' +
            'out.rail = document.querySelector("a.pt-page[aria-current=page]").dataset.page; ' +
            'chip.querySelector("a").click(); s = await wait(() => { const x = document.querySelector("#graph-stats[data-nodes]"); return x && !document.querySelector(".pt-filter") ? x : null; }); ' +
            'out.cleared = location.pathname + location.search; out.all = Number(s.dataset.nodes); return JSON.stringify(out); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        $o.env | Should -Be @('Show graph|/default/graph?source=Emitter.Environment|', 'Open card|/default/emitters/Emitter.Environment|')
        $o.label | Should -Be 'Emitter.Environment (Card)'
        $o.unlisted | Should -Be @('Show graph|/default/graph?source=Emitter.Unlisted|', 'Open card||true')
        $o.unstamped | Should -Be @('Show graph||true', 'Open card||true')
        $o.why | Should -Be @('unstamped: its layers carry no source block, so no source filter matches them', 'unstamped: no card is matched to a module')
        "$($o.at) | $($o.stats) | $($o.title) | $($o.chip) | $($o.rail)" | Should -Be '/default/graph?source=Emitter.Environment | 10 nodes · 9 edges | Nodes | Emitter.Environment|/default/graph | ui:rail:graph:nodes'
        $o.cleared | Should -Be '/default/graph'
        $o.all | Should -Be @((Invoke-RestMethod "$EmUrl/api/tenants/default/graph").nodes).Count
        # No match is the normal empty state, with the chip: a link, not an error.
        $dom = Get-PageDom $EmUrl '/default/graph?source=Emitter.Terraform'
        $dom | Should -Match '<span class="pt-count" id="graph-stats" data-nodes="0" data-edges="0">0 nodes · 0 edges</span><span class="pt-filter" data-filter="source" data-value="Emitter.Terraform">source: <b>Emitter.Terraform</b><a class="pt-filter-clear" href="/default/graph" title="clear the source filter" aria-label="clear the source filter Emitter.Terraform">×</a></span>'
        $dom | Should -Match '<div class="pt-card pt-empty-state"><h2>No nodes</h2><p>No mounted layer''s source block names Emitter.Terraform\.'
        $dom | Should -Not -Match 'pt-error-card'
        $dom | Should -Match 'href="/default/graph" class="pt-page" data-page="ui:rail:graph:nodes" aria-current="page"'
    }
    It 'a layer page names the emitter its source block names, with that version and the status, or its module unstamped' {
        $dom = Get-PageDom $EmUrl '/default/layers/agent-runs'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<p class="pt-produced-by" data-emitter="Emitter.Agent">produced by <a href="/default/emitters/Emitter.Agent">Emitter.Agent</a> 0.1.4 <span class="pt-status" data-status="ok"[^>]*>ok</span></p>'
        $dom = Get-PageDom $EmUrl '/default/layers/agent-alerts'
        $dom | Should -Match '<p class="pt-produced-by" data-emitter="Emitter.Environment">produced by <a href="/default/emitters/Emitter.Environment">Emitter.Environment</a> 0.4.0 \(card 0.3.0\) <span class="pt-status" data-status="version-mismatch"[^>]*>version-mismatch</span></p>'
        $dom = Get-PageDom $EmUrl '/default/layers/filesystem'
        $dom | Should -Match '<p class="pt-produced-by" data-emitter="Graph.Emitter.FileSystem">produced by <a href="/default/emitters/Graph.Emitter.FileSystem">Graph.Emitter.FileSystem</a> <span class="pt-status" data-status="unstamped"[^>]*>unstamped</span></p>'
    }
}
Describe 'Ledger' {
    # docs/design.md 45. A tenant of its own: admin root (-Bootstrap) and a push ledger of one line from before 0.10.0
    # (no seq, no prev: the unchained head). root mounts the agent-runs fixture and the ledger fixture, twelve entries of
    # the agent's ledger, seq 44 to 55, nine of them in the run; then the broken copy under the same name, whose entry 50
    # was altered, so the chain breaks at 51.
    BeforeAll {
        $script:LgTenants = Join-Path $TestDrive 'lg-tenants'
        $td = Join-Path $LgTenants 'default'
        $null = New-Item -ItemType Directory -Force -Path $td
        Set-Content -LiteralPath (Join-Path $td 'portal.yaml') -Encoding utf8NoBOM -Value '{}'
        $script:LgRoot = New-PortalToken -Name root -Bootstrap -Tenants $LgTenants
        Set-Content -LiteralPath (Join-Path $td 'ledger.jsonl') -Encoding utf8NoBOM -Value ('{"at":"2026-10-11T00:00:00.000Z","principal":"ui:principal:root","name":"root","role":"admin","feed":"cards-oci","kind":"oci",' +
            '"card":"Emitter.Agent","version":"0.1.3","type":"emitter","hash":"sha256:' + ('ab' * 32) + '","digest":"sha256:' + ('cd' * 32) + '","files":1,"result":"pushed","message":"Emitter.Agent 0.1.3 to cards-oci"}')
        $script:LgWork = Join-Path $TestDrive 'ledger'
        $script:LgPortal = Start-Portal -Port 18200 -Artifacts $LgWork -Tenants $LgTenants
        $script:LgUrl = $LgPortal.Url
        $script:LgRun = 'agent:run/2026-10-09T02:50:43.7976027Z'
        function Mount-AsRoot([string] $Name, [string] $Dir) {
            $saved = $env:PORTAL_TOKEN
            try { $env:PORTAL_TOKEN = $LgRoot.Token; Mount-PortalLayer -Name $Name -Path (Join-Path $Fixtures 'layers' $Dir) -Url $LgUrl }
            finally { $env:PORTAL_TOKEN = $saved }
        }
        $null = Mount-AsRoot agent-runs agent-runs
        $null = Mount-AsRoot ledger ledger
    }
    AfterAll { Stop-Portal -Artifacts $LgWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'Ledger/Entries lists both chains with their counts, the agent chain valid in green; the altered copy is red, broken at 51' {
        $api = Invoke-RestMethod "$LgUrl/api/tenants/default/ledger"
        @($api.nodes | ForEach-Object { "$($_.id) $($_.properties.count) $($_.properties.valid)" }) | Should -Be @('ui:ledger:agent 12 True', 'ui:ledger:portal 1 True')
        $dom = Get-PageDom $LgUrl '/default/ledger'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match ('<tr data-source="agent"><td><a href="/default/ledger/agent">agent</a></td><td><span class="pt-valid" data-valid="true">valid</span></td>' +
            '<td>12</td><td>44</td><td>55</td><td>2026-10-09T02:51:00Z</td><td>layer <a href="/default/layers/ledger">ledger</a></td></tr>')
        $dom | Should -Match ('<tr data-source="portal"><td><a href="/default/ledger/portal">portal</a></td><td><span class="pt-valid" data-valid="true">valid</span></td>' +
            '<td>1</td><td>1</td><td>1</td><td>2026-10-11T00:00:00.000Z</td><td>ledger.jsonl · 1 before the chain</td></tr>')
        $dom | Should -Match '<span class="pt-count">2 chains</span>'
        $dom | Should -Match 'href="/default/ledger" class="pt-page" data-page="ui:rail:ledger:entries" aria-current="page"'
        # The ledger service's other pages keep their paths.
        (Get-PageDom $LgUrl '/default/ledger/verify') | Should -Match '<h1 class="pt-title" id="title"[^>]*>Verify</h1>'

        $null = Mount-AsRoot ledger ledger-broken
        try {
            $a = (Invoke-RestMethod "$LgUrl/api/tenants/default/ledger").nodes | Where-Object id -EQ 'ui:ledger:agent'
            "$($a.properties.valid) $($a.properties.breakAt) $($a.properties.reason) $($a.properties.claimed)" | Should -Be 'False 51 prev-mismatch True'
            $dom = Get-PageDom $LgUrl '/default/ledger'
            $dom | Should -Match '<tr data-source="agent"><td><a href="/default/ledger/agent">agent</a></td><td><span class="pt-broken" data-valid="false" title="prev-mismatch">broken at 51</span></td><td>12</td>'
            $dom | Should -Match '<span class="pt-count">2 chains · 1 broken</span>'
            $dom = Get-PageDom $LgUrl '/default/ledger/agent'
            $dom | Should -Match '<span class="pt-badge" id="ledger-stats" data-source="agent">12 entries · <span class="pt-broken" data-valid="false" title="prev-mismatch">broken at 51</span></span>'
            $dom | Should -Match '<tr data-seq="51" class="pt-break" title="the chain breaks here: prev-mismatch">'
            # The broken line's colour is the chain's red.
            (Get-Content -LiteralPath (Join-Path $Repo 'web' 'base.css') -Raw) | Should -Match '\.pt-broken \{ color: var\(--gr-bad\);'
        }
        finally { $null = Mount-AsRoot ledger ledger }
    }
    It 'a chain''s page lists its entries newest first, each run linking to the runs layer page by id after #; the agent''s run counts its entries' {
        [int](Invoke-WebRequest "$LgUrl/default/ledger/nope" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        [int](Invoke-WebRequest "$LgUrl/default/ledger/agent/99" -SkipHttpErrorCheck).StatusCode | Should -Be 404
        $dom = Get-PageDom $LgUrl '/default/ledger/agent'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<span class="pt-badge" id="ledger-stats" data-source="agent">12 entries · <span class="pt-valid" data-valid="true">valid</span></span>'
        @([regex]::Matches($dom, '<tr data-seq="(\d+)"') | ForEach-Object { [int]$_.Groups[1].Value }) | Should -Be (55..44)
        $step2 = [regex]::Escape([uri]::EscapeDataString("$LgRun/step-2"))
        $dom | Should -Match ('<tr data-seq="50"><td><a href="/default/ledger/agent/50">50</a></td><td>2026-10-09T02:50:47Z</td><td>policy.pre</td>' +
            '<td><a href="/default/layers/agent-runs#' + $step2 + '" class="pt-run-link" data-run="' + [regex]::Escape("$LgRun/step-2") + '">' + [regex]::Escape($LgRun) + '</a></td>')
        $dom | Should -Match '<tr data-seq="44"><td><a href="/default/ledger/agent/44">44</a></td><td>2026-10-09T02:50:41Z</td><td>policy.load</td><td></td>'
        $dom | Should -Match '<li><a href="/default/ledger">Entries</a></li><li><span aria-current="page">agent</span></li>'
        # The run link lands on the runs layer's page, its row marked (0.8.0's highlight).
        $dom = Get-PageDom $LgUrl "/default/layers/agent-runs#$([uri]::EscapeDataString("$LgRun/step-2"))"
        $dom | Should -Match ('<tr data-id="' + [regex]::Escape("$LgRun/step-2") + '"[^>]*class="pt-target" aria-current="true"')
        # One entry, linking the one before it.
        $dom = Get-PageDom $LgUrl '/default/ledger/agent/51'
        $dom | Should -Match '<dd data-field="seq">51</dd>'
        $dom | Should -Match '<dd data-field="prev"><span>[0-9a-f]{64} · <a href="/default/ledger/agent/50">entry 50</a></span></dd>'
        # The agent's page counts the run's entries, linking to them.
        $runQ = [regex]::Escape([uri]::EscapeDataString($LgRun))
        $dom = Get-PageDom $LgUrl '/default/agents/root'
        $dom | Should -Match ('<th>layer</th><th>ledger</th>')
        $dom | Should -Match ('<td><a href="/default/ledger/agent\?run=' + $runQ + '" class="pt-ledger-count" data-run="' + [regex]::Escape($LgRun) + '">9</a></td></tr>')
        $dom = Get-PageDom $LgUrl "/default/ledger/agent?run=$([uri]::EscapeDataString($LgRun))"
        @([regex]::Matches($dom, '<tr data-seq="(\d+)"') | ForEach-Object { [int]$_.Groups[1].Value }) | Should -Be (54..46)
        $dom | Should -Match ('9 entries · <span class="pt-valid" data-valid="true">valid</span> · run ' + [regex]::Escape($LgRun) + '</span>')
        # Portal's own chain: the line from before 0.10.0, unchained.
        $dom = Get-PageDom $LgUrl '/default/ledger/portal'
        $dom | Should -Match '<tr data-seq="1"><td><a href="/default/ledger/portal/1">1</a></td><td>2026-10-11T00:00:00.000Z</td><td>pushed \(unchained\)</td><td>Emitter.Agent@0.1.3</td><td>root</td>'
    }
}

Describe 'Environments graph' {
    # docs/design.md 46, 48. A tenant of its own (admin root, -Bootstrap). root mounts the agent-runs and ledger fixtures,
    # then agent-environment: first the real layer from Portal.Agent's Up (nine nodes under env:, every one env dev, no
    # tags; its hosts edge names a run no fixture holds), later the tagged copy (groups stack and egress and tags on three
    # instances, no env on the ledger keeper, hosts onto the runs fixture's run) under the same name.
    BeforeAll {
        $script:EgTenants = Join-Path $TestDrive 'eg-tenants'
        $td = Join-Path $EgTenants 'default'
        $null = New-Item -ItemType Directory -Force -Path $td
        Set-Content -LiteralPath (Join-Path $td 'portal.yaml') -Encoding utf8NoBOM -Value '{}'
        $script:EgRoot = New-PortalToken -Name root -Bootstrap -Tenants $EgTenants
        $script:EgWork = Join-Path $TestDrive 'envgraph'
        $script:EgPortal = Start-Portal -Port 18200 -Artifacts $EgWork -Tenants $EgTenants
        $script:EgUrl = $EgPortal.Url
        $script:EgRun = 'agent:run/2026-10-09T02:50:43.7976027Z'
        function Mount-EgAsRoot([string] $Name, [string] $Dir) {
            $saved = $env:PORTAL_TOKEN
            try { $env:PORTAL_TOKEN = $EgRoot.Token; Mount-PortalLayer -Name $Name -Path (Join-Path $Fixtures 'layers' $Dir) -Url $EgUrl }
            finally { $env:PORTAL_TOKEN = $saved }
        }
        $null = Mount-EgAsRoot agent-runs agent-runs
        $null = Mount-EgAsRoot ledger ledger
        $null = Mount-EgAsRoot agent-environment agent-environment
        # Waits in the page: wait(f) resolves f's first truthy value, polling, or throws after 6 s.
        $script:EgWait = 'const wait = async (f) => { for (let t = 0; t < 120; t++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 50)); } throw new Error("timed out: " + f); };'
        # The node menu's items as label|href|disabled, one string each.
        $script:EgItems = 'const items = (m) => [...m.querySelectorAll("[role=menuitem]")].map((x) => `${x.textContent}|${x.getAttribute("href") || ""}|${x.getAttribute("aria-disabled") || ""}`);'
    }
    AfterAll { Stop-Portal -Artifacts $EgWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'Environments/Graph sits first under Environments and draws the real layer: nine env: nodes, dev-bootstrap one runs-as edge away, every node a link to graph#id' {
        $api = Invoke-RestMethod "$EgUrl/api/tenants/default/environments/graph"
        @($api.nodes | ForEach-Object { "$($_.kind) $($_.id)" }) | Should -Be @('environment.instance env:compose/claude-agent-dev', 'environment.instance env:container/claude-agent-dev/agent',
            'environment.instance env:container/claude-agent-dev/ledger', 'environment.instance env:container/claude-agent-dev/mitm', 'environment.image env:image/claude-agent-ledger:dev',
            'environment.image env:image/claude-agent:dev', 'environment.image env:image/mitmproxy/mitmproxy:12.2.3', 'environment.network env:network/claude-agent-dev_inside',
            'environment.network env:network/claude-agent-dev_outside', 'Principal ui:principal:dev-bootstrap')
        @($api.edges.kind | Sort-Object) | Should -Be @('attached-to', 'attached-to', 'attached-to', 'member-of', 'member-of', 'member-of', 'runs-as', 'runs-image', 'runs-image', 'runs-image')
        $dom = Get-PageDom $EgUrl '/default/environments/graph'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match ('<a href="/default/environments/graph" class="pt-page" data-page="ui:rail:environments:graph" aria-current="page">Graph</a></li>' +
            '<li><a href="/default/environments/instances" class="pt-page" data-page="ui:rail:environments:instances">Instances</a>')
        $dom | Should -Match '<a href="/default/environments/graph#env%3Acompose%2Fclaude-agent-dev" class="pt-eg-node" data-id="env:compose/claude-agent-dev" data-kind="environment.instance" data-shape="circle" data-ikind="compose" data-env="dev"'
        $dom | Should -Match '<a href="/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fmitm" class="pt-eg-node" data-id="env:container/claude-agent-dev/mitm" data-kind="environment.instance" data-shape="circle" data-ikind="container" data-env="dev"'
        $dom | Should -Match '<a href="/default/environments/graph#env%3Aimage%2Fclaude-agent%3Adev" class="pt-eg-node" data-id="env:image/claude-agent:dev" data-kind="environment.image" data-shape="square"'
        $dom | Should -Match '<a href="/default/environments/graph#env%3Anetwork%2Fclaude-agent-dev_inside" class="pt-eg-node" data-id="env:network/claude-agent-dev_inside" data-kind="environment.network" data-shape="diamond"'
        $dom | Should -Match '<a href="/default/environments/graph#ui%3Aprincipal%3Adev-bootstrap" class="pt-eg-node pt-eg-neighbour" data-id="ui:principal:dev-bootstrap" data-kind="Principal" data-shape="hexagon"'
        [regex]::Matches($dom, '<a href="[^"]*" class="pt-eg-node').Count | Should -Be 10
        [regex]::Matches($dom, '<line class="pt-eg-edge" data-kind="member-of"').Count | Should -Be 3
        [regex]::Matches($dom, '<line class="pt-eg-edge" data-kind="runs-image"').Count | Should -Be 3
        [regex]::Matches($dom, '<line class="pt-eg-edge" data-kind="attached-to"').Count | Should -Be 3
        [regex]::Matches($dom, '<line class="pt-eg-edge" data-kind="runs-as"').Count | Should -Be 1
        $dom | Should -Not -Match 'data-kind="hosts" data-from'
        $dom | Should -Match '<span class="pt-count" data-unset="0">9 nodes · 1 one edge away · 10 edges</span>'
        # The env chips are Instances' chips; every node carries env dev, and unset is offered all the same.
        $dom | Should -Match ('<div class="pt-chips" role="group" aria-label="filter by env" data-chips="env"><button type="button" class="pt-chip" data-value="" aria-pressed="true">all</button>' +
            '<button type="button" class="pt-chip" data-value="dev" aria-pressed="false">dev</button><button type="button" class="pt-chip" data-value="qa" aria-pressed="false">qa</button>' +
            '<button type="button" class="pt-chip" data-value="prod" aria-pressed="false">prod</button><button type="button" class="pt-chip" data-value="unset" aria-pressed="false">unset</button></div>')
        $dom | Should -Match '<span class="pt-muted" data-chips="tags">no tags</span>'
        @([regex]::Matches($dom, '<g class="pt-eg-hull[^"]*" data-env="([^"]+)"') | ForEach-Object { $_.Groups[1].Value }) | Should -Be @('dev')
        $dom | Should -Match '<svg [^>]*data-settled="true"'
        # Line styles by edge kind, colour by the instance's kind.
        $css = Get-Content -LiteralPath (Join-Path $Repo 'web' 'base.css') -Raw
        $css | Should -Match '\.pt-eg-edge\[data-kind="member-of"\] \{ stroke-dasharray: none; \}'
        $css | Should -Match '\.pt-eg-edge\[data-kind="runs-as"\] \{ stroke-dasharray: 6 4; \}'
        $css | Should -Match '\.pt-eg-edge\[data-kind="hosts"\] \{ stroke-dasharray: 0\.5 4\.5;'
        foreach ($k in 'compose', 'container', 'kubernetes', 'host') { $css | Should -Match ('\.pt-eg-node\[data-ikind="' + $k + '"\] \.pt-eg-shape \{ fill: var\(--eg-' + $k + '\); \}') }
        # Shape by Kind, from the page's own module.
        $shapes = Invoke-PageScript $EgUrl '/default/environments/graph' '' ('import("/static/envgraph.js").then((m) => ["environment.instance", "environment.image", "environment.network", "environment.volume", "Run"]' +
            '.map((kind) => m.shapeOf({ kind })).join(" "))')
        $shapes | Should -Be 'circle square diamond hexagon hexagon'
    }

    It 'the tagged copy draws env and group hulls and tag chips; the node after # is selected with its neighbours pinned, and survives a reload' {
        $null = Mount-EgAsRoot agent-environment agent-environment-tagged
        $api = Invoke-RestMethod "$EgUrl/api/tenants/default/environments/graph"
        $run = $api.nodes | Where-Object id -EQ $EgRun
        "$($run.kind) $($run.properties.layer) $($run.properties.ledger)" | Should -Be 'Run agent-runs 9'
        @($api.edges | Where-Object kind -EQ 'hosts' | ForEach-Object { "$($_.from) $($_.to)" }) | Should -Be @("env:compose/claude-agent-dev $EgRun")
        $dom = Get-PageDom $EgUrl '/default/environments/graph#env%3Acompose%2Fclaude-agent-dev'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<span class="pt-count" data-unset="1">9 nodes · 2 one edge away · 11 edges · 1 without env</span>'
        $dom | Should -Match 'data-id="env:compose/claude-agent-dev" data-kind="environment.instance" data-shape="circle" data-ikind="compose" data-env="dev" data-group="stack" data-tags="agent compose"'
        $dom | Should -Match 'data-id="env:container/claude-agent-dev/ledger" data-kind="environment.instance" data-shape="circle" data-ikind="container" data-env="unset"'
        $hulls = [regex]::Matches($dom, '<g class="(pt-eg-hull[^"]*)" data-env="([^"]+)"(?: data-group="([^"]+)")?>') | ForEach-Object { "$($_.Groups[2].Value)/$($_.Groups[3].Value)" }
        @($hulls | Sort-Object) | Should -Be @('dev/', 'dev/egress', 'dev/stack', 'unset/')
        $dom | Should -Match ('aria-label="filter by tag" data-chips="tags"><button type="button" class="pt-chip" data-value="" aria-pressed="true">all</button>' +
            '<button type="button" class="pt-chip" data-value="agent" aria-pressed="false">agent</button><button type="button" class="pt-chip" data-value="compose" aria-pressed="false">compose</button>' +
            '<button type="button" class="pt-chip" data-value="egress" aria-pressed="false">egress</button></div>')
        $dom | Should -Match '<svg [^>]*data-settled="true"'
        $dom | Should -Match '<svg [^>]*data-selected="env:compose/claude-agent-dev"'
        $dom | Should -Match 'data-id="env:compose/claude-agent-dev"[^>]*aria-current="true"'
        # Selected and pinned: the compose node and its five neighbours (three containers, dev-bootstrap, the run).
        @([regex]::Matches($dom, '<a [^>]*data-id="([^"]+)"[^>]*data-pinned="true"') | ForEach-Object { $_.Groups[1].Value } | Sort-Object) |
            Should -Be (@('env:compose/claude-agent-dev', 'env:container/claude-agent-dev/agent', 'env:container/claude-agent-dev/ledger', 'env:container/claude-agent-dev/mitm', $EgRun, 'ui:principal:dev-bootstrap') | Sort-Object)
        $dom | Should -Match '<span class="pt-eg-status" aria-live="polite">selected env:compose/claude-agent-dev · 5 neighbours pinned</span>'
        # A pasted item route becomes the graph with the node after #; the dev chip hides the unset keeper and its edges.
        $r = Invoke-PageScript $EgUrl "/default/environments/graph/$([uri]::EscapeDataString('env:container/claude-agent-dev/mitm'))" '' ("(async () => { $EgWait " +
            'const svg = await wait(() => document.querySelector("svg.pt-eg[data-settled=true]")); const at = location.pathname + location.hash; ' +
            'document.querySelector("[data-chips=env] button[data-value=dev]").click(); ' +
            'const keeper = document.querySelector("a.pt-eg-node[data-id=\"env:container/claude-agent-dev/ledger\"]").hasAttribute("hidden"); ' +
            'const edge = document.querySelector("line[data-from=\"env:container/claude-agent-dev/ledger\"]").hasAttribute("hidden"); ' +
            'return [at, svg.dataset.selected, svg.dataset.shown, keeper, edge].join(" "); })()')
        $r | Should -Be '/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fmitm env:container/claude-agent-dev/mitm 8 true true'
    }

    It 'a drag pins a node and a double-click releases it' {
        $r = Invoke-PageScript $EgUrl '/default/environments/graph' '' ("(async () => { $EgWait " +
            'const svg = await wait(() => document.querySelector("svg.pt-eg[data-settled=true]")); ' +
            'const a = document.querySelector("a.pt-eg-node[data-id=\"env:container/claude-agent-dev/agent\"]"); const b = a.getBoundingClientRect(); ' +
            'const x = b.x + b.width / 2; const y = b.y + b.height / 2; const o = { bubbles: true, pointerId: 7, button: 0, isPrimary: true }; ' +
            'a.querySelector(".pt-eg-shape").dispatchEvent(new PointerEvent("pointerdown", { ...o, clientX: x, clientY: y })); ' +
            'svg.dispatchEvent(new PointerEvent("pointermove", { ...o, clientX: x + 60, clientY: y + 30 })); ' +
            'svg.dispatchEvent(new PointerEvent("pointerup", { ...o, clientX: x + 60, clientY: y + 30 })); ' +
            'const pinned = await wait(() => a.dataset.pinned); const clickKept = !location.hash; ' +
            'a.querySelector(".pt-eg-shape").dispatchEvent(new MouseEvent("dblclick", { bubbles: true, cancelable: true })); ' +
            'await wait(() => !a.dataset.pinned); return [pinned, clickKept, a.dataset.pinned === undefined].join(" "); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $r | Should -Be 'true true true'
    }

    It 'right-click on a graph node and on an Instances row opens Details, Show graph, Runs, Ledger and Query from here as links, greyed where no edge leads, keyboard too' {
        $runQ = [uri]::EscapeDataString($EgRun)
        $q = '/default/graph/explore?from='
        # The keyboard's contextmenu (Shift+F10, the menu key) comes at 0,0: the menu opens beside the focused node.
        $r = Invoke-PageScript $EgUrl '/default/environments/graph' '' ("(async () => { $EgWait $EgItems " +
            'const svg = await wait(() => document.querySelector("svg.pt-eg[data-settled=true]")); const out = {}; ' +
            'const node = (id) => document.querySelector(`a.pt-eg-node[data-id="${id}"]`); ' +
            'const c = node("env:compose/claude-agent-dev"); const b = c.getBoundingClientRect(); ' +
            'c.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 4, clientY: b.y + 4 })); ' +
            'let m = await wait(() => document.querySelector(".pt-nodemenu")); out.compose = items(m); out.first = document.activeElement.textContent; ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true })); out.second = document.activeElement.textContent; ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true })); out.last = document.activeElement.textContent; ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); out.closed = !document.querySelector(".pt-nodemenu"); out.back = document.activeElement.dataset.id; ' +
            'const k = node("env:container/claude-agent-dev/mitm"); k.focus(); k.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 0, clientY: 0 })); ' +
            'm = await wait(() => document.querySelector(".pt-nodemenu")); out.mitm = items(m); out.titles = [...m.querySelectorAll("[aria-disabled]")].map((x) => x.title); ' +
            'm.querySelector("[data-item=show-graph]").click(); out.selected = await wait(() => svg.dataset.selected); out.hash = location.hash; out.path = location.pathname; ' +
            'node("ui:principal:dev-bootstrap").dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 9, clientY: 9 })); ' +
            'm = await wait(() => document.querySelector(".pt-nodemenu")); out.principal = items(m); ' +
            "node('$EgRun')" + '.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 9, clientY: 9 })); ' +
            'm = await wait(() => document.querySelector(".pt-nodemenu")); out.run = items(m); ' +
            # Query from here: Explore with the query bar from the node (docs/design.md 54).
            'm.querySelector("[data-item=query-from-here]").click(); const qs = await wait(() => document.querySelector("#query-stats[data-nodes]")); ' +
            'out.query = location.pathname + location.search + " " + document.querySelector("form[data-form=query] input[name=from]").value + " " + qs.dataset.nodes + " " + document.getElementById("title").textContent; ' +
            'return JSON.stringify(out); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        $o.compose | Should -Be @('Details|/default/environments/instances/env%3Acompose%2Fclaude-agent-dev|', 'Show graph|/default/environments/graph#env%3Acompose%2Fclaude-agent-dev|',
            "Runs|/default/layers/agent-runs#$runQ|", "Ledger|/default/ledger/agent?run=$runQ|", "Query from here|${q}env%3Acompose%2Fclaude-agent-dev|")
        "$($o.first) / $($o.second) / $($o.last) / $($o.closed) / $($o.back)" | Should -Be 'Details / Show graph / Query from here / True / env:compose/claude-agent-dev'
        # A container also gets Logs, Stop and Restart (docs/design.md 50), greyed here: the page is anonymous.
        $o.mitm | Should -Be @('Details|/default/environments/instances/env%3Acontainer%2Fclaude-agent-dev%2Fmitm|', 'Show graph|/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fmitm|', 'Runs||true', 'Ledger||true',
            "Query from here|${q}env%3Acontainer%2Fclaude-agent-dev%2Fmitm|", 'Logs||true', 'Stop||true', 'Restart||true')
        $o.titles | Should -Be @('no hosts edge to a mounted run', 'no hosts edge to a mounted run', 'sign in to write', 'sign in to write', 'sign in to write')
        "$($o.path)$($o.hash) $($o.selected)" | Should -Be '/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fmitm env:container/claude-agent-dev/mitm'
        # dev-bootstrap holds no run here (root mounted them): no has-run edge, so its pages are greyed.
        $o.principal | Should -Be @('Details||true', 'Show graph|/default/environments/graph#ui%3Aprincipal%3Adev-bootstrap|', 'Runs||true', 'Ledger||true', "Query from here|${q}ui%3Aprincipal%3Adev-bootstrap|")
        $o.run | Should -Be @("Details|/default/layers/agent-runs#$runQ|", "Show graph|/default/environments/graph#$runQ|", "Runs|/default/layers/agent-runs#$runQ|", "Ledger|/default/ledger/agent?run=$runQ|", "Query from here|$q$runQ|")
        $n = @((Invoke-RestMethod "$EgUrl/api/tenants/default/query?from=$runQ").nodes).Count
        $o.query | Should -Be "$q$runQ $EgRun $n Explore"

        # The Instances row: the same menu; Show graph opens the graph with that node selected.
        $r = Invoke-PageScript $EgUrl '/default/environments/instances' '' ("(async () => { $EgWait $EgItems " +
            'const row = await wait(() => document.querySelector("tr[data-id=\"env:container/claude-agent-dev/agent\"]")); const out = {}; const b = row.getBoundingClientRect(); ' +
            'row.cells[1].dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 30, clientY: b.y + 4 })); ' +
            'const m = await wait(() => document.querySelector(".pt-nodemenu")); out.agent = items(m); out.label = m.getAttribute("aria-label"); ' +
            'm.querySelector("[data-item=show-graph]").click(); ' +
            'const svg = await wait(() => document.querySelector("svg.pt-eg[data-settled=true]")); out.at = location.pathname + location.hash; out.selected = svg.dataset.selected; ' +
            'out.title = document.getElementById("title").textContent; return JSON.stringify(out); })()')
        $o = $r | ConvertFrom-Json
        $o.agent | Should -Be @('Details|/default/environments/instances/env%3Acontainer%2Fclaude-agent-dev%2Fagent|', 'Show graph|/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fagent|', 'Runs||true', 'Ledger||true',
            "Query from here|${q}env%3Acontainer%2Fclaude-agent-dev%2Fagent|", 'Logs||true', 'Stop||true', 'Restart||true')
        $o.label | Should -Be 'agent (claude-agent:dev) (environment.instance)'
        "$($o.at) $($o.selected) $($o.title)" | Should -Be '/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fagent env:container/claude-agent-dev/agent Graph'
    }
}

Describe 'Actions' -Tag 'Actions' {
    # docs/design.md 50. A tenant of its own (admin root, -Bootstrap) whose ledger.jsonl is the actions fixture, one chain:
    # three requests as root (1 logs on the agent container, 2 restart on mitm, 3 stop on the ledger keeper), then the
    # Runner's results, 4 answering 1 (done) and 5 answering 2 (failed, exit 1); 3 is the gap. root mounts the agent-runs,
    # ledger and agent-environment fixtures, which answer nothing: results are read from ledger.jsonl only. Portal asks;
    # it never runs anything.
    BeforeAll {
        $script:AcTenants = Join-Path $TestDrive 'ac-tenants'
        $script:AcTd = Join-Path $AcTenants 'default'
        $null = New-Item -ItemType Directory -Force -Path $AcTd
        Set-Content -LiteralPath (Join-Path $AcTd 'portal.yaml') -Encoding utf8NoBOM -Value '{}'
        $script:AcRoot = New-PortalToken -Name root -Bootstrap -Tenants $AcTenants
        Copy-Item -LiteralPath (Join-Path $Fixtures 'ledger' 'actions.ledger.jsonl') -Destination (Join-Path $AcTd 'ledger.jsonl')
        $script:AcWork = Join-Path $TestDrive 'actions'
        $script:AcPortal = Start-Portal -Port 18200 -Artifacts $AcWork -Tenants $AcTenants
        $script:AcUrl = $AcPortal.Url
        $script:AcAuth = @{ Authorization = "Bearer $($AcRoot.Token)" }
        foreach ($m in @('agent-runs', 'ledger', 'agent-environment')) {
            $saved = $env:PORTAL_TOKEN
            try { $env:PORTAL_TOKEN = $AcRoot.Token; $null = Mount-PortalLayer -Name $m -Path (Join-Path $Fixtures 'layers' $m) -Url $AcUrl }
            finally { $env:PORTAL_TOKEN = $saved }
        }
        function Send-Ask($Body, [hashtable] $Headers = $AcAuth) {
            $r = Invoke-WebRequest -Uri "$AcUrl/api/tenants/default/actions" -Method POST -Headers $Headers -ContentType 'application/json' -SkipHttpErrorCheck `
                -Body (($Body -is [string]) ? $Body : ($Body | ConvertTo-Json -Compress))
            [pscustomobject]@{ Status = [int]$r.StatusCode; Body = ([string]$r.Content | ConvertFrom-Json) }
        }
        $script:AcWait = 'const wait = async (f) => { for (let t = 0; t < 120; t++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 50)); } throw new Error("timed out: " + f); };'
        $script:AcItems = 'const items = (m) => [...m.querySelectorAll("[role=menuitem]")].map((x) => `${x.textContent}|${x.tagName}|${x.getAttribute("aria-disabled") || ""}`);'
    }
    AfterAll { Stop-Portal -Artifacts $AcWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'POST .../actions appends a request as the token''s principal; a principal in the body, another action, a non-container and no token are refused, writing nothing' {
        (Send-Ask @{ action = 'logs'; subject = 'env:container/claude-agent-dev/agent'; principal = 'ui:principal:someone' }).Status | Should -Be 400
        foreach ($bad in @(@{ action = 'nuke'; subject = 'env:container/claude-agent-dev/agent' }, @{ action = 'stop'; subject = 'env:compose/claude-agent-dev' },
                @{ action = 'stop'; subject = 'env:image/claude-agent:dev' }, @{ action = 'stop'; subject = 'claude-agent-dev/agent' })) {
            $r = Send-Ask $bad
            "$($r.Status) $($r.Body.code)" | Should -Be '422 invalid'
        }
        (Send-Ask @{ action = 'logs'; subject = 'env:container/claude-agent-dev/agent' } @{}).Status | Should -Be 401
        @(Get-Content -LiteralPath (Join-Path $AcTd 'ledger.jsonl')).Count | Should -Be 5

        $r = Send-Ask @{ action = 'logs'; subject = 'env:container/claude-agent-dev/agent' }
        $r.Status | Should -Be 201
        "$($r.Body.kind) $($r.Body.id) $($r.Body.action) $($r.Body.subject) $($r.Body.principal) $($r.Body.status) $($r.Body.seq)" |
            Should -Be 'action.request ledger:tenant/default/6 logs env:container/claude-agent-dev/agent ui:principal:root requested 6'
        # The line as written is the line returned, in the contract's member order, chained to line 5, the Runner's.
        $lines = @(Get-Content -LiteralPath (Join-Path $AcTd 'ledger.jsonl'))
        $lines.Count | Should -Be 6
        $lines[5] | Should -Match '^\{"kind":"action\.request","id":"ledger:tenant/default/6","action":"logs","subject":"env:container/claude-agent-dev/agent","principal":"ui:principal:root","requested":"[^"]+","status":"requested","seq":6,"prev":"[0-9a-f]{64}"\}$'
        $sha = [Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($lines[4]))
        $r.Body.prev | Should -Be ([Convert]::ToHexString($sha).ToLowerInvariant())
        $c = (Invoke-RestMethod "$AcUrl/api/tenants/default/ledger").nodes | Where-Object id -EQ 'ui:ledger:portal'
        "$($c.properties.count) $($c.properties.valid)" | Should -Be '6 True'
    }

    It 'GET .../actions joins each request to its result in the one chain: done, failed, and the gap still requested' {
        $api = Invoke-RestMethod "$AcUrl/api/tenants/default/actions"
        $root = $api.nodes | Where-Object id -EQ 'ui:actions'
        "$($root.properties.count) $($root.properties.requested) $($root.properties.done) $($root.properties.failed)" | Should -Be '4 2 1 1'
        @($api.edges | Where-Object kind -EQ 'has-action' | ForEach-Object to) | Should -Be @('ledger:tenant/default/6', 'ledger:tenant/default/3', 'ledger:tenant/default/2', 'ledger:tenant/default/1')
        @($api.nodes | Where-Object { $_.properties.PSObject.Properties['event']?.Value -eq 'action.request' } | ForEach-Object { "$($_.properties.seq) $($_.properties.outcome) $($_.properties.PSObject.Properties['resultId']?.Value)" }) |
            Should -Be @('6 requested ', '3 requested ', '2 failed ledger:tenant/default/5', '1 done ledger:tenant/default/4')
        $res = $api.nodes | Where-Object id -EQ 'ledger:tenant/default/5'
        "$($res.properties.source) $($res.properties.request) $($res.properties.status) $($res.properties.exit) $($res.properties.writer[0].emitter) $($res.source[0].tool) $($res.source[0].path):$($res.source[0].line)" |
            Should -Be 'portal ledger:tenant/default/2 failed 1 Emitter.Ledger Emitter.Ledger ledger.jsonl:5'
        @($api.edges | Where-Object kind -In 'answered-by', 'answers' | ForEach-Object id | Sort-Object) |
            Should -Be @('ledger:tenant/default/1|ledger:tenant/default/4|answered-by', 'ledger:tenant/default/2|ledger:tenant/default/5|answered-by', 'ledger:tenant/default/4|ledger:tenant/default/1|answers', 'ledger:tenant/default/5|ledger:tenant/default/2|answers')
        (Invoke-RestMethod "$AcUrl/api/tenants/default/actions/2").root | Should -Be 'ledger:tenant/default/2'
        foreach ($n in 4, 9) { [int](Invoke-WebRequest "$AcUrl/api/tenants/default/actions/$n" -SkipHttpErrorCheck).StatusCode | Should -Be 404 }
    }

    It 'Environments/Actions at /default/actions lists them newest first, each status in its colour, every id a link and the request''s id last; a row opens Explore from it' {
        $dom = Get-PageDom $AcUrl '/default/actions'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match 'href="/default/actions" class="pt-page" data-page="ui:rail:environments:actions" aria-current="page">Actions</a>'
        $dom | Should -Match '<span class="pt-count">4 actions · 2 requested · 1 done · 1 failed</span>'
        $dom | Should -Match '<th>action</th><th>subject</th><th>principal</th><th>requested</th><th>status</th><th>result</th><th>id</th>'
        @([regex]::Matches($dom, '<tr data-seq="(\d+)" data-id="([^"]+)" data-outcome="(\w+)"') | ForEach-Object { "$($_.Groups[1].Value) $($_.Groups[2].Value) $($_.Groups[3].Value)" }) |
            Should -Be @('6 ledger:tenant/default/6 requested', '3 ledger:tenant/default/3 requested', '2 ledger:tenant/default/2 failed', '1 ledger:tenant/default/1 done')
        # docs/design.md 55: each row's query is Explore from its request, out, five hops.
        $dom | Should -Match '<tr data-seq="2" data-id="ledger:tenant/default/2" data-outcome="failed" class="pt-action-row" data-explore="/default/graph/explore\?from=ledger%3Atenant%2Fdefault%2F2&amp;direction=out&amp;depth=5"'
        $dom | Should -Match ('<td><a href="/default/ledger/portal/2">restart</a></td><td><a href="/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fmitm">env:container/claude-agent-dev/mitm</a></td>' +
            '<td><a href="/default/identity/users#ui%3Aprincipal%3Aroot">root</a></td><td>2026-10-09T02:50:57.000Z</td><td><span class="pt-outcome" data-outcome="failed">failed</span></td>' +
            '<td><a href="/default/ledger/portal/5">ledger:tenant/default/5</a></td><td class="pt-hash">ledger:tenant/default/2</td></tr>')
        $dom | Should -Match '<td><span class="pt-outcome" data-outcome="requested">requested</span></td><td></td><td class="pt-hash">ledger:tenant/default/6</td></tr>'
        $css = Get-Content -LiteralPath (Join-Path $Repo 'web' 'base.css') -Raw
        $css | Should -Match '\.pt-outcome\[data-outcome="done"\] \{ color: var\(--gr-ok\); \}'
        $css | Should -Match '\.pt-outcome\[data-outcome="failed"\] \{ color: var\(--gr-bad\); \}'
        # A click on the row (not a link) opens Explore querying from the request out to depth 5 in the tenant-actions
        # layer: the result, the lines before it, its subject and its principal (docs/design.md 55); the principal's link
        # marks its row on Users.
        $r = Invoke-PageScript $AcUrl '/default/actions' '' ("(async () => { $AcWait " +
            'const row = await wait(() => document.querySelector("tr[data-seq=\"1\"]")); row.cells[3].click(); ' +
            'const s = await wait(() => document.querySelector("#query-stats[data-nodes]")); ' +
            'const f = document.querySelector("form[data-form=query]"); ' +
            'return JSON.stringify({ at: location.pathname + location.search, fields: [...f.elements].filter((x) => x.name).map((x) => `${x.name}=${x.value}`) }); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        $o.at | Should -Be '/default/graph/explore?from=ledger%3Atenant%2Fdefault%2F1&direction=out&depth=5'
        $o.fields | Should -Contain 'from=ledger:tenant/default/1'
        $o.fields | Should -Contain 'direction=out'
        $o.fields | Should -Contain 'depth=5'
        # The same query from the API: request 1 reaches its subject, its principal and the line before it; its result,
        # line 4, points at it, so out does not reach it.
        $q = Invoke-RestMethod "$AcUrl/api/tenants/default/query?from=ledger%3Atenant%2Fdefault%2F1&direction=out&depth=5"
        @($q.nodes.id) | Should -Contain 'env:container/claude-agent-dev/agent'
        @($q.nodes.id) | Should -Contain 'ui:principal:root'
        @($q.edges.id) | Should -Contain 'ledger:tenant/default/1|ui:principal:root|principal'
        (Get-PageDom $AcUrl '/default/identity/users#ui%3Aprincipal%3Aroot') | Should -Match '<tr data-principal="ui:principal:root" class="pt-target" aria-current="true">'
    }

    It 'a request''s entry links its result and a result''s links its request, the output in a pre; the gap says nothing answered it' {
        $dom = Get-PageDom $AcUrl '/default/ledger/portal/1'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<dd data-field="action">logs</dd>'
        $dom | Should -Match '<dd data-field="subject"><a href="/default/environments/graph#env%3Acontainer%2Fclaude-agent-dev%2Fagent">env:container/claude-agent-dev/agent</a></dd>'
        $dom | Should -Match '<dd data-field="outcome"><span class="pt-outcome" data-outcome="done">done</span></dd>'
        $dom | Should -Match '<dd data-field="answered by"><a href="/default/ledger/portal/4">ledger:tenant/default/4</a></dd>'
        $dom | Should -Match '<dd data-field="principal"><a href="/default/identity/users#ui%3Aprincipal%3Aroot">ui:principal:root</a></dd>'
        $dom = Get-PageDom $AcUrl '/default/ledger/portal/5'
        $dom | Should -Match '<dd data-field="answers"><a href="/default/ledger/portal/2">ledger:tenant/default/2</a></dd>'
        $dom | Should -Match '<dd data-field="status"><span class="pt-outcome" data-outcome="failed">failed</span></dd>'
        $dom | Should -Match '<dd data-field="exit">1</dd>'
        $dom | Should -Match '<dd data-field="output"><pre class="pt-output">Error response from daemon: container claude-agent-dev-mitm-1 is marked for removal and cannot be started\s*</pre></dd>'
        $dom | Should -Match '<dd data-field="written by">Emitter\.Ledger 0\.4\.0 · for default · 2026-10-09T02:50:59\.400Z</dd>'
        $dom | Should -Match '<dd data-field="line">ledger\.jsonl:5</dd>'
        $dom = Get-PageDom $AcUrl '/default/ledger/portal/3'
        $dom | Should -Match '<dd data-field="answered by">nothing yet: no later action\.result in the tenant''s ledger names ledger:tenant/default/3</dd>'
        # The portal chain's list says what each request asked and what each result answers.
        $dom = Get-PageDom $AcUrl '/default/ledger/portal'
        $dom | Should -Match '<tr data-seq="2" data-event="action.request"><td><a href="/default/ledger/portal/2">2</a></td><td>2026-10-09T02:50:57.000Z</td><td>restart · failed</td><td>env:container/claude-agent-dev/mitm</td><td>root</td>'
        $dom | Should -Match '<tr data-seq="5" data-event="action.result"><td><a href="/default/ledger/portal/5">5</a></td><td>2026-10-09T02:50:59.400Z</td><td>failed · answers ledger:tenant/default/2</td><td></td><td>Emitter.Ledger</td>'
    }

    It 'right-click offers Logs, Stop and Restart on a container only; Logs asks at once, Stop and Restart ask first, and the notice links the new line' {
        $r = Invoke-PageScript $AcUrl '/default/environments/graph' $AcRoot.Token ("(async () => { $AcWait $AcItems " +
            'await wait(() => document.querySelector("svg.pt-eg[data-settled=true]")); const out = {}; ' +
            'const menuOf = async (id) => { const a = document.querySelector(`a.pt-eg-node[data-id="${id}"]`); const b = a.getBoundingClientRect(); ' +
            'a.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 4, clientY: b.y + 4 })); return wait(() => document.querySelector(".pt-nodemenu")); }; ' +
            'let m = await menuOf("env:container/claude-agent-dev/agent"); out.container = items(m); out.sep = !!m.querySelector("[role=separator]"); ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "End", bubbles: true })); out.last = document.activeElement.textContent; ' +
            'm.querySelector("[data-action=logs]").click(); out.dialog = !!document.querySelector(".pt-confirm"); ' +
            'const t1 = await wait(() => document.querySelector(".pt-asked")); out.asked = t1.dataset.asked; out.link = t1.querySelector("a").getAttribute("href"); ' +
            'm = await menuOf("env:compose/claude-agent-dev"); out.compose = items(m); ' +
            'm = await menuOf("env:image/claude-agent:dev"); out.image = items(m); ' +
            'm = await menuOf("env:container/claude-agent-dev/mitm"); m.querySelector("[data-action=restart]").click(); ' +
            'let d = await wait(() => document.querySelector(".pt-confirm")); out.question = d.querySelector("h2").textContent; out.focus = document.activeElement.dataset.confirm; ' +
            'd.querySelector("[data-confirm=cancel]").click(); await new Promise((r) => setTimeout(r, 300)); out.cancelled = document.querySelectorAll(".pt-asked").length; ' +
            'm = await menuOf("env:container/claude-agent-dev/mitm"); m.querySelector("[data-action=stop]").click(); ' +
            'd = await wait(() => document.querySelector(".pt-confirm")); d.querySelector("[data-confirm=ok]").click(); ' +
            'await wait(() => document.querySelectorAll(".pt-asked").length === 2); out.second = document.querySelectorAll(".pt-asked")[1].dataset.asked; ' +
            'document.querySelectorAll(".pt-asked")[1].querySelector("a").click(); await wait(() => document.querySelector("[data-field=\"answered by\"]")); out.at = location.pathname; ' +
            'return JSON.stringify(out); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        $o.container | Should -Be @('Details|A|', 'Show graph|A|', 'Runs|A|true', 'Ledger|A|true', 'Query from here|A|', 'Logs|BUTTON|', 'Stop|BUTTON|', 'Restart|BUTTON|')
        "$($o.sep) $($o.last) $($o.dialog) $($o.asked) $($o.link)" | Should -Be 'True Restart False ledger:tenant/default/7 /default/ledger/portal/7'
        $o.compose | Should -Be @('Details|A|', 'Show graph|A|', 'Runs|A|true', 'Ledger|A|true', 'Query from here|A|')
        $o.image | Should -Be @('Details|A|', 'Show graph|A|', 'Runs|A|true', 'Ledger|A|true', 'Query from here|A|')
        "$($o.question) / $($o.focus) / $($o.cancelled) / $($o.second) / $($o.at)" | Should -Be 'Restart mitm (mitmproxy/mitmproxy:12.2.3)? / cancel / 1 / ledger:tenant/default/8 / /default/ledger/portal/8'
        # Cancel wrote nothing: line 7 is the logs, line 8 the stop, both as root.
        $lines = @(Get-Content -LiteralPath (Join-Path $AcTd 'ledger.jsonl') | ForEach-Object { $_ | ConvertFrom-Json })
        @($lines | Select-Object -Skip 6 | ForEach-Object { "$($_.id) $($_.action) $($_.subject) $($_.principal)" }) |
            Should -Be @('ledger:tenant/default/7 logs env:container/claude-agent-dev/agent ui:principal:root', 'ledger:tenant/default/8 stop env:container/claude-agent-dev/mitm ui:principal:root')
    }

    It 'anonymous sees the three actions greyed with why, on the graph and on an Instances row' {
        $r = Invoke-PageScript $AcUrl '/default/environments/instances' '' ("(async () => { $AcWait $AcItems " +
            'const row = await wait(() => document.querySelector("tr[data-id=\"env:container/claude-agent-dev/ledger\"]")); const b = row.getBoundingClientRect(); ' +
            'row.cells[1].dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 30, clientY: b.y + 4 })); ' +
            'const m = await wait(() => document.querySelector(".pt-nodemenu")); ' +
            'return JSON.stringify({ items: items(m), titles: [...m.querySelectorAll("button[aria-disabled]")].map((x) => x.title) }); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        @($o.items | Select-Object -Last 3) | Should -Be @('Logs|BUTTON|true', 'Stop|BUTTON|true', 'Restart|BUTTON|true')
        $o.titles | Should -Be @('sign in to write', 'sign in to write', 'sign in to write')
    }

    It 'a result the Runner appends while Portal runs is read without a restart, and the next ask is chained to it' {
        # The Runner's line answering request 7, in its shape (source one object, as Runner 0.2.0 writes it), appended
        # with its LF behind the running portal's back.
        $file = Join-Path $AcTd 'ledger.jsonl'
        $lines = @(Get-Content -LiteralPath $file)
        $hash = { param($l) [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($l))).ToLowerInvariant() }
        $n = $lines.Count + 1
        $result = '{"kind":"action.result","id":"ledger:' + $n + '","request":"ledger:7","status":"done","started":"2026-10-11T22:00:00.000Z","finished":"2026-10-11T22:00:01.000Z","exit":0,"output":"agent: stopped\n","source":{"emitter":"Emitter.Ledger","version":"0.4.0","target":"default","stamp":"2026-10-11T22:00:01.000Z"},"seq":' + $n + ',"prev":"' + (& $hash $lines[-1]) + '"}'
        [IO.File]::AppendAllText($file, $result + "`n", [Text.UTF8Encoding]::new($false))
        $api = Invoke-RestMethod "$AcUrl/api/tenants/default/actions"
        $q = $api.nodes | Where-Object id -EQ 'ledger:tenant/default/7'
        "$($q.properties.outcome) $($q.properties.resultId)" | Should -Be "done ledger:tenant/default/$n"
        $root = $api.nodes | Where-Object id -EQ 'ui:actions'
        "$($root.properties.count) $($root.properties.requested) $($root.properties.done) $($root.properties.failed)" | Should -Be '6 3 2 1'
        $r = Send-Ask @{ action = 'restart'; subject = 'env:container/claude-agent-dev/mitm' }
        "$($r.Status) $($r.Body.id) $($r.Body.prev)" | Should -Be "201 ledger:tenant/default/$($n + 1) $(& $hash $result)"
        $c = (Invoke-RestMethod "$AcUrl/api/tenants/default/ledger").nodes | Where-Object id -EQ 'ui:ledger:portal'
        "$($c.properties.count) $($c.properties.valid)" | Should -Be "$($n + 1) True"
    }
}

Describe 'Alerts' {
    # docs/design.md 47, 48. A tenant of its own (admin root, -Bootstrap). root mounts the agent-runs, ledger and tagged
    # environment fixtures, then the varied alerts inline (the real alert layer holds one, so they are Go's variedAlerts):
    # five alerts under alert:<fingerprint> raised in the runs fixture's run, about step 3's pre deny (critical, open),
    # ledger entry 52 (critical, resolved), the run (warning, acknowledged), ledger entry 46 (warning, open) and the
    # compose node (info, open). The real alert layer is mounted by path last.
    BeforeAll {
        $script:AlTenants = Join-Path $TestDrive 'al-tenants'
        $td = Join-Path $AlTenants 'default'
        $null = New-Item -ItemType Directory -Force -Path $td
        Set-Content -LiteralPath (Join-Path $td 'portal.yaml') -Encoding utf8NoBOM -Value '{}'
        $script:AlRoot = New-PortalToken -Name root -Bootstrap -Tenants $AlTenants
        $script:AlWork = Join-Path $TestDrive 'alerts'
        $script:AlPortal = Start-Portal -Port 18200 -Artifacts $AlWork -Tenants $AlTenants
        $script:AlUrl = $AlPortal.Url
        $script:AlRun = 'agent:run/2026-10-09T02:50:43.7976027Z'
        # Each fingerprint is SHA-256 of the rule and the subject id with a newline between, as Graph.Node 0.3.3 registers it.
        $script:AlFp = @{ Decision = 'b9885880ee651752e2b1dfa400d8238803c67f927ff74e8fd774ca0f19fa5027'; Ledger52 = '5af632474080efde717583489cc8334ed38b593f90ef18bba42f684edcf92118'
            RunDenied = 'f433234ac4a8ea943e4769832fdb4d8ecd194cfac4920e5c5e71006a259f66ee'; Ledger46 = 'ad50168288b7cacc483d2e3d7e56611506c8e51f11d4530d646f87f0586b46f2'
            Env = 'a133d7bec5d0ce29d8fef760b9eea4894a0dc073f170420468d55cd29c33779d' }
        $script:AlOrder = @($AlFp.Ledger52, $AlFp.Decision, $AlFp.RunDenied, $AlFp.Ledger46, $AlFp.Env)
        $script:AlCompose = 'env:compose/claude-agent-dev'
        $script:AlComposeQ = [uri]::EscapeDataString($AlCompose)
        function Mount-AlAsRoot([string] $Name, [string] $Dir) {
            $saved = $env:PORTAL_TOKEN
            try { $env:PORTAL_TOKEN = $AlRoot.Token; Mount-PortalLayer -Name $Name -Path (Join-Path $Fixtures 'layers' $Dir) -Url $AlUrl }
            finally { $env:PORTAL_TOKEN = $saved }
        }
        $null = Mount-AlAsRoot agent-runs agent-runs
        $null = Mount-AlAsRoot ledger ledger
        $null = Mount-AlAsRoot agent-environment agent-environment-tagged
        function New-AlAlert([string] $Fp, [string] $Severity, [string] $Rule, [string] $Summary, [string] $Stamp, [string] $State) {
            [ordered]@{ id = "alert:$Fp"; kind = 'alert'; name = "$Rule $($Fp.Substring(0, 8))"
                properties = [ordered]@{ severity = $Severity; rule = $Rule; summary = $Summary; stamp = $Stamp; state = $State; fingerprint = $Fp } }
        }
        # A mount by envelope, module Claude.Agent.Orchestrator; each edge is from, to, kind.
        function Mount-AlEnvelope([string] $Name, [string] $Stamp, [object[]] $Nodes, [object[]] $Edges) {
            $es = @($Edges | ForEach-Object { [ordered]@{ id = "$($_[0])|$($_[1])|$($_[2])"; kind = $_[2]; from = $_[0]; to = $_[1] } })
            $body = [ordered]@{ name = $Name; envelope = [ordered]@{ schema = 'graph/1'; module = 'Claude.Agent.Orchestrator'; version = '0.3.0'; layer = $Stamp; nodes = @($Nodes); edges = $es } } |
                ConvertTo-Json -Depth 8 -Compress
            $null = Invoke-RestMethod "$AlUrl/api/tenants/default/layers" -Method Post -ContentType 'application/json' -Body $body -Headers @{ Authorization = "Bearer $($AlRoot.Token)" }
        }
        $about = [ordered]@{ $AlFp.Decision = "$AlRun/step-3/pre"; $AlFp.Ledger52 = 'ledger:52'; $AlFp.Ledger46 = 'ledger:46'; $AlFp.RunDenied = $AlRun; $AlFp.Env = $AlCompose }
        Mount-AlEnvelope agent-alerts-varied '2026-10-09T02:51:05Z' @(
            New-AlAlert $AlFp.Decision critical policy-deny 'Step 3''s pre hook denied the action the agent asked for.' '2026-10-09T02:50:49Z' open
            New-AlAlert $AlFp.Ledger52 critical policy-deny 'Ledger entry 52 records a denied pre decision.' '2026-10-09T02:50:49Z' resolved
            New-AlAlert $AlFp.Ledger46 warning egress-check 'The egress check ran before the run''s first step.' '2026-10-09T02:50:44Z' open
            New-AlAlert $AlFp.RunDenied warning run-denied 'The run stopped with a denied step.' '2026-10-09T02:50:52Z' acknowledged
            New-AlAlert $AlFp.Env info image-by-tag 'The compose stack runs the agent image by tag, not by digest.' '2026-10-09T02:50:43Z' open
            [ordered]@{ id = 'ledger:46'; kind = 'LedgerEntry'; name = '46 agent.egress-check'; properties = [ordered]@{ seq = 46; stamp = '2026-10-09T02:50:43Z'; event = 'agent.egress-check'; run = $AlRun } }
            [ordered]@{ id = 'ledger:52'; kind = 'LedgerEntry'; name = '52 policy.pre'; properties = [ordered]@{ seq = 52; stamp = '2026-10-09T02:50:49Z'; event = 'policy.pre'; run = $AlRun } }
            [ordered]@{ id = $AlCompose; kind = 'environment.instance'; name = 'claude-agent-dev (compose on MSI)'; properties = [ordered]@{ kind = 'compose'; state = 'running'; composeProject = 'claude-agent-dev'; env = 'dev' } }
        ) @($about.Keys | ForEach-Object { , @("alert:$_", $about[$_], 'about'); , @("alert:$_", $AlRun, 'raised-in') })
        $script:AlWait = 'const wait = async (f) => { for (let t = 0; t < 120; t++) { const v = f(); if (v) return v; await new Promise((r) => setTimeout(r, 50)); } throw new Error("timed out: " + f); };'
        $script:AlItems = 'const items = (m) => [...m.querySelectorAll("[role=menuitem]")].map((x) => `${x.textContent}|${x.getAttribute("href") || ""}|${x.getAttribute("aria-disabled") || ""}`);'
    }
    AfterAll { Stop-Portal -Artifacts $AlWork; InModuleScope Portal { Wait-PortalPortFree -Port 18200 -TimeoutSec 10 } }

    It 'Compliance/Alerts sits first under Compliance and lists the alerts critical first, newest first, severity in colour, subject and run linking their pages' {
        $api = Invoke-RestMethod "$AlUrl/api/tenants/default/alerts"
        @($api.nodes | Where-Object kind -EQ 'Alert' | ForEach-Object { $_.properties.fingerprint }) | Should -Be $AlOrder
        $root = $api.nodes | Where-Object id -EQ 'ui:alerts'
        "$($root.properties.count) $($root.properties.open.critical) $($root.properties.open.warning) $($root.properties.open.info)" | Should -Be '5 1 1 1'
        $dom = Get-PageDom $AlUrl '/default/compliance/alerts'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match ('<a href="/default/compliance/alerts" class="pt-page" data-page="ui:rail:compliance:alerts" aria-current="page">Alerts</a></li>' +
            '<li><a href="/default/compliance/rules" class="pt-page" data-page="ui:rail:compliance:rules">Rules</a>')
        @([regex]::Matches($dom, '<tr data-id="alert:([0-9a-f]{64})"') | ForEach-Object { $_.Groups[1].Value }) | Should -Be $AlOrder
        $runQ = [regex]::Escape([uri]::EscapeDataString($AlRun))
        $dom | Should -Match ('<tr data-id="alert:' + $AlFp.Decision + '" data-severity="critical"><td><span class="pt-sev" data-severity="critical">critical</span></td><td>policy-deny</td>' +
            '<td><a href="/default/compliance/alerts/' + $AlFp.Decision + '">Step 3''s pre hook denied the action the agent asked for\.</a></td>' +
            '<td><a href="/default/layers/agent-runs#' + [regex]::Escape([uri]::EscapeDataString("$AlRun/step-3/pre")) + '" class="pt-subject-link" data-id="' + [regex]::Escape("$AlRun/step-3/pre") + '" title="Decision [^"]+">deny rules\[2\] \(pre\)</a></td>' +
            '<td><a href="/default/layers/agent-runs#' + $runQ + '" class="pt-run-link" data-id="' + [regex]::Escape($AlRun) + '" title="Run [^"]+">[^<]+</a></td><td>2026-10-09T02:50:49Z</td></tr>')
        # The ledger entry, from the ledger layer (never the alert layer's copy), and the compose node, from the environment layer.
        $dom | Should -Match '<td><a href="/default/ledger/agent/52" class="pt-subject-link" data-id="ledger:52" title="LedgerEntry ledger:52">52 policy.pre</a></td>'
        $dom | Should -Match ('<td><a href="/default/environments/instances/' + $AlComposeQ + '" class="pt-subject-link" data-id="' + $AlCompose + '" title="environment.instance ' + $AlCompose + '">claude-agent-dev \(compose on MSI\)</a></td>')
        # An alert no longer open is dimmed.
        $dom | Should -Match ('<tr data-id="alert:' + $AlFp.Ledger52 + '" data-severity="critical" class="pt-closed">')
        $dom | Should -Match '<span class="pt-count">5 alerts</span>'
        $css = Get-Content -LiteralPath (Join-Path $Repo 'web' 'base.css') -Raw
        $css | Should -Match '\.pt-sev\[data-severity="critical"\], \.pt-alert-link\[data-severity="critical"\] \{ color: var\(--gr-bad\); \}'
        $css | Should -Match '\.pt-sev\[data-severity="warning"\], \.pt-alert-link\[data-severity="warning"\] \{ color: rgb\(245,166,35\); \}'
    }

    It 'severity, rule and state chips filter through the URL; an alert''s page shows its fields and its two edges as links' {
        $dom = Get-PageDom $AlUrl '/default/compliance/alerts?severity=critical'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        @([regex]::Matches($dom, '<tr data-id="alert:([0-9a-f]{64})"') | ForEach-Object { $_.Groups[1].Value }) | Should -Be @($AlFp.Ledger52, $AlFp.Decision)
        $dom | Should -Match '<span class="pt-count">2 alerts · 5 in all</span>'
        $dom | Should -Match 'data-chips="severity"><button type="button" class="pt-chip" data-value="" aria-pressed="false">all</button><button type="button" class="pt-chip" data-value="critical" aria-pressed="true">critical</button>'
        $dom | Should -Match ('aria-label="filter by state" data-chips="state"><button type="button" class="pt-chip" data-value="" aria-pressed="true">all</button>' +
            '<button type="button" class="pt-chip" data-value="acknowledged" aria-pressed="false">acknowledged</button><button type="button" class="pt-chip" data-value="open" aria-pressed="false">open</button>' +
            '<button type="button" class="pt-chip" data-value="resolved" aria-pressed="false">resolved</button></div>')
        # A chip keeps the other filters and moves to the first page.
        $r = Invoke-PageScript $AlUrl '/default/compliance/alerts?severity=warning' '' ("(async () => { $AlWait " +
            '(await wait(() => document.querySelector("[data-chips=state] button[data-value=open]"))).click(); ' +
            'await wait(() => location.search.includes("state") && document.querySelectorAll("tr[data-id]").length === 1); ' +
            'return location.search + " " + document.querySelector("tr[data-id]").dataset.id; })()')
        $r | Should -Be "?severity=warning&state=open alert:$($AlFp.Ledger46)"
        $dom = Get-PageDom $AlUrl "/default/compliance/alerts/$($AlFp.Decision)"
        $dom | Should -Match ('<h1 class="pt-title" id="title" data-page="ui:rail:compliance:alerts">' + $AlFp.Decision + '</h1>')
        $dom | Should -Match ('<dd data-field="id">alert:' + $AlFp.Decision + '</dd><dt>severity</dt><dd data-field="severity"><span class="pt-sev" data-severity="critical">critical</span></dd><dt>rule</dt><dd data-field="rule">policy-deny</dd>')
        $dom | Should -Match ('<dd data-field="state">open</dd><dt>stamp</dt><dd data-field="stamp">2026-10-09T02:50:49Z</dd><dt>fingerprint</dt><dd data-field="fingerprint">' + $AlFp.Decision + '</dd><dt>layer</dt><dd data-field="layer"><a href="/default/layers/agent-alerts-varied">agent-alerts-varied</a></dd>')
        $dom | Should -Match ('<dd data-field="about"><a href="/default/layers/agent-runs#' + [regex]::Escape([uri]::EscapeDataString("$AlRun/step-3/pre")) + '" class="pt-subject-link"')
        $dom | Should -Match ('<dd data-field="raised-in"><a href="/default/layers/agent-runs#' + [regex]::Escape([uri]::EscapeDataString($AlRun)) + '" class="pt-run-link"')
        $dom | Should -Match ('<li><a href="/default/compliance/alerts">Alerts</a></li><li><span aria-current="page">' + $AlFp.Decision + '</span></li>')
        (Get-PageDom $AlUrl '/default/compliance/alerts/nope') | Should -Match 'not_found: no alert alert:nope in tenant default; its mounted layers hold 5'
    }

    It 'the banner badge beside the mode counts the open alerts at their top severity: red for critical, amber for warning, none for info only' {
        $dom = Get-PageDom $AlUrl '/default/overview/summary'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match ('<span class="pt-mode" id="mode"[^>]*>[^<]*</span>\s*<a class="pt-alerts-badge" id="alerts-badge" href="/default/compliance/alerts\?severity=critical&amp;state=open" ' +
            'data-severity="critical" title="open alerts: 1 critical · 1 warning · 1 info"[^>]*>1 critical</a>')
        $r = Invoke-PageScript $AlUrl '/default/overview/summary' '' ('import("/static/alerts.js").then((m) => [{ critical: 1, warning: 2, info: 3 }, { critical: 0, warning: 2, info: 1 }, { info: 4 }, {}]' +
            '.map((o) => { const b = m.badgeOf(o); return b ? `${b.severity} ${b.count}` : "none"; }).join(", "))')
        $r | Should -Be 'critical 1, warning 2, none, none'
        $css = Get-Content -LiteralPath (Join-Path $Repo 'web' 'base.css') -Raw
        $css | Should -Match '\.pt-alerts-badge\[data-severity="critical"\] \{ background: var\(--gr-bad\);'
        $css | Should -Match '\.pt-alerts-badge\[data-severity="warning"\] \{ background: rgb\(245,166,35\);'
    }

    It 'a ledger entry, a run, a decision and an environment instance show an alerts row linking back; the agent page counts a run''s alerts beside its ledger count' {
        $dom = Get-PageDom $AlUrl '/default/ledger/agent/52'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match ('<dd data-field="alerts"><span class="pt-alerts-about" data-about="ledger:52"><a href="/default/compliance/alerts/' + $AlFp.Ledger52 + '" class="pt-alert-link" data-severity="critical" ' +
            'title="[^"]*">critical policy-deny \(resolved\)</a></span></dd>')
        (Get-PageDom $AlUrl '/default/ledger/agent/50') | Should -Not -Match 'data-field="alerts"'
        $dom = Get-PageDom $AlUrl "/default/layers/agent-runs#$([uri]::EscapeDataString("$AlRun/step-3/pre"))"
        $dom | Should -Match ('<tr class="pt-alerts-row" data-alerts-of="' + [regex]::Escape("$AlRun/step-3/pre") + '"><td colspan="4">alerts: <span class="pt-alerts-about"[^>]*>' +
            '<a href="/default/compliance/alerts/' + $AlFp.Decision + '" class="pt-alert-link" data-severity="critical"[^>]*>critical policy-deny</a></span></td></tr>')
        $dom = Get-PageDom $AlUrl "/default/layers/agent-runs#$([uri]::EscapeDataString($AlRun))"
        $dom | Should -Match ('data-alerts-of="' + [regex]::Escape($AlRun) + '"><td colspan="4">alerts: <span class="pt-alerts-about"[^>]*><a href="/default/compliance/alerts/' + $AlFp.RunDenied + '" class="pt-alert-link" data-severity="warning"[^>]*>warning run-denied \(acknowledged\)</a>')
        $dom = Get-PageDom $AlUrl "/default/environments/instances/$AlComposeQ"
        $dom | Should -Match ('<dt>alerts</dt><dd data-field="alerts"><span class="pt-alerts-about" data-about="' + $AlCompose + '"><a href="/default/compliance/alerts/' + $AlFp.Env + '" class="pt-alert-link" data-severity="info"[^>]*>info image-by-tag</a>')
        $dom = Get-PageDom $AlUrl '/default/agents/root'
        $runQ = [regex]::Escape([uri]::EscapeDataString($AlRun))
        $dom | Should -Match '<th>layer</th><th>ledger</th><th>alerts</th>'
        $dom | Should -Match ('class="pt-ledger-count" data-run="[^"]+">9</a></td><td><a href="/default/compliance/alerts\?run=' + $runQ + '" class="pt-alert-count" data-run="' + [regex]::Escape($AlRun) + '">5</a></td></tr>')
    }

    It 'right-click on an alert row opens Details, Subject, Run and Ledger as links, greyed with why where no edge leads, keyboard too' {
        $runQ = [uri]::EscapeDataString($AlRun)
        $r = Invoke-PageScript $AlUrl '/default/compliance/alerts' '' ("(async () => { $AlWait $AlItems " +
            'const row = await wait(() => document.querySelector("tr[data-id=\"alert:' + $AlFp.Decision + '\"]")); const out = {}; const b = row.getBoundingClientRect(); ' +
            'row.cells[1].dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 30, clientY: b.y + 4 })); ' +
            'let m = await wait(() => document.querySelector(".pt-nodemenu")); out.items = items(m); out.first = document.activeElement.textContent; ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true })); out.second = document.activeElement.textContent; ' +
            'm.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true })); out.closed = !document.querySelector(".pt-nodemenu"); out.back = document.activeElement.getAttribute("href"); ' +
            'const env = row.parentElement.querySelector("tr[data-id=\"alert:' + $AlFp.Env + '\"] a"); env.focus(); env.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 0, clientY: 0 })); ' +
            'm = await wait(() => document.querySelector(".pt-nodemenu")); out.env = items(m); ' +
            'm.querySelector("[data-item=subject]").click(); await wait(() => location.pathname.includes("instances")); out.path = location.pathname; ' +
            'return JSON.stringify(out); })()')
        if ($null -eq $r) { Set-ItResult -Skipped -Because 'no Chrome'; return }
        $o = $r | ConvertFrom-Json
        $o.items | Should -Be @("Details|/default/compliance/alerts/$($AlFp.Decision)|", "Subject|/default/layers/agent-runs#$([uri]::EscapeDataString("$AlRun/step-3/pre"))|",
            "Run|/default/layers/agent-runs#$runQ|", "Ledger|/default/ledger/agent?run=$runQ|")
        "$($o.first) / $($o.second) / $($o.closed) / $($o.back)" | Should -Be "Details / Subject / True / /default/compliance/alerts/$($AlFp.Decision)"
        $o.env[1] | Should -Be "Subject|/default/environments/instances/$AlComposeQ|"
        $o.path | Should -Be "/default/environments/instances/$AlComposeQ"

        # An alert with no about edge, raised in a run no layer holds: Subject, Run and Ledger greyed, each saying why.
        $orphan = '00000000000000000000000000000000000000000000000000000000deadbeef'
        Mount-AlEnvelope agent-alerts-orphan '2026-10-09T03:00:00Z' @(New-AlAlert $orphan info orphan 'An alert raised in a run no layer holds.' '2026-10-09T03:00:00Z' open) @(
            , @("alert:$orphan", 'agent:run/2026-01-01T00:00:00Z', 'raised-in'))
        $r = Invoke-PageScript $AlUrl '/default/compliance/alerts?rule=orphan' '' ("(async () => { $AlWait $AlItems " +
            'const row = await wait(() => document.querySelector("tr[data-id=\"alert:' + $orphan + '\"]")); const b = row.getBoundingClientRect(); ' +
            'row.cells[0].dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: b.x + 4, clientY: b.y + 4 })); ' +
            'const m = await wait(() => document.querySelector(".pt-nodemenu")); ' +
            'return JSON.stringify({ items: items(m), titles: [...m.querySelectorAll("[aria-disabled]")].map((x) => x.title), run: row.cells[4].innerHTML }); })()')
        $o = $r | ConvertFrom-Json
        $o.items | Should -Be @("Details|/default/compliance/alerts/$orphan|", 'Subject||true', 'Run||true', 'Ledger||true')
        $o.titles | Should -Be @('no about edge', 'no mounted runs layer holds agent:run/2026-01-01T00:00:00Z', 'no raised-in edge to a mounted run')
        $o.run | Should -Be '<span class="pt-muted" title="no mounted layer Portal reads holds it">agent:run/2026-01-01T00:00:00Z</span>'
    }

    It 'the real alert layer from Portal.Agent mounts by path and shows its one alert, its id alert: and the fingerprint' {
        $null = Mount-AlAsRoot agent-alerts agent-alerts
        $api = Invoke-RestMethod "$AlUrl/api/tenants/default/alerts?rule=egress-attempt"
        $a = @($api.nodes | Where-Object kind -EQ 'Alert')
        @($a.id) | Should -Be @('alert:e62d4da8120f3a864c3576cd6b3bbb838923641c71b01f9153d41e4973face6c')
        "$($a.properties.severity) $($a.properties.state) $($a.properties.about) $($a.properties.run) $($a.properties.layer)" |
            Should -Be 'info open ledger:303 agent:run/2026-10-11T19:00:22.5540487Z agent-alerts'
        $dom = Get-PageDom $AlUrl '/default/compliance/alerts?rule=egress-attempt'
        if (-not $dom) { Set-ItResult -Skipped -Because 'no Chrome for --dump-dom'; return }
        $dom | Should -Match '<tr data-id="alert:e62d4da8120f3a864c3576cd6b3bbb838923641c71b01f9153d41e4973face6c" data-severity="info">'
    }

    It 'a Compliance page alerts with any other view stops the boot' {
        $dir = Join-Path $TestDrive 'al-bad'
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $dir 'default')
        Set-Content -LiteralPath (Join-Path $dir 'default' 'portal.yaml') -Value "menu:`n  - id: compliance`n    children: [{ id: alerts, view: table, kind: alert }]"
        { Start-Portal -Port 18201 -Artifacts (Join-Path $TestDrive 'al-bad-art') -Filesystem '' -Tenants $dir -TimeoutSec 5 } |
            Should -Throw '*compliance/alerts is the alerts page*its view is alerts, not "table"*'
    }
}
