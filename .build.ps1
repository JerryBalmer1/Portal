#Requires -Version 7.4
# Invoke-Build script. Run: Invoke-Build              (Modules, Build, Test, Smoke, Pester)
#                           Invoke-Build Up           (start the portal on its fixed port over artifacts/: -Port,
#                                                      else PORTAL_PORT from .env, else the environment, else 8200)
#                           Invoke-Build Status       (port, URL and boot stamp of the running portal)
#                           Invoke-Build Down         (stop it)
#                           Invoke-Build Generate     (regenerate internal/gen from api/openapi.yaml)
#                           Invoke-Build Screenshot   (docs/screenshots/portal.png)
param(
    # Up's port; absent means PORTAL_PORT from .env, else the environment, else 8200. 0 is refused.
    [int] $Port
)

$repo     = $PSScriptRoot
$buildDir = Join-Path $repo 'build'
$module   = Join-Path $repo 'src' 'Portal' 'Portal.psd1'
# Smoke, Pester and Screenshot run here, so a portal on the fixed port keeps running through a build.
$testPort = 18200

function Invoke-Go {
    param([Parameter(Mandatory)] [string[]] $Arguments, [hashtable] $Env = @{})
    $saved = @{}
    foreach ($k in $Env.Keys) { $saved[$k] = [Environment]::GetEnvironmentVariable($k); [Environment]::SetEnvironmentVariable($k, $Env[$k]) }
    try {
        Push-Location $repo
        & go @Arguments
        if ($LASTEXITCODE -ne 0) { throw "go $($Arguments -join ' ') failed with $LASTEXITCODE" }
    }
    finally {
        Pop-Location
        foreach ($k in $saved.Keys) { [Environment]::SetEnvironmentVariable($k, $saved[$k]) }
    }
}

# Go on PATH and the PowerShell modules the build uses. Nothing is installed: a missing one fails with the line to run.
task Modules {
    if (-not (Get-Command go -ErrorAction SilentlyContinue)) { throw 'go is not on PATH; install Go 1.27.0 or later' }
    foreach ($m in @(@{ Name = 'Pester'; Min = '5.5.0' }, @{ Name = 'InvokeBuild'; Min = '5.11.0' })) {
        $found = Get-Module -ListAvailable -Name $m.Name | Where-Object Version -GE ([version]$m.Min) | Sort-Object Version -Descending | Select-Object -First 1
        if (-not $found) { throw "$($m.Name) $($m.Min)+ is missing: Install-Module $($m.Name) -MinimumVersion $($m.Min) -Scope CurrentUser" }
        print Green "$($m.Name) $($found.Version)"
    }
    print Green (& go version)
}

# internal/gen from api/openapi.yaml with the pinned oapi-codegen (go.mod tool). The output is staged, never hand-edited.
task Generate { Invoke-Go @('generate', './api/...') }

# portal and graphnode for win-x64 into build/win-x64/, the only platform anything runs them on.
task Build {
    foreach ($cmd in 'portal', 'graphnode') {
        $out = Join-Path $buildDir 'win-x64' "$cmd.exe"
        Invoke-Go @('build', '-trimpath', '-ldflags', '-s -w', '-o', $out, "./cmd/$cmd") -Env @{ GOOS = 'windows'; GOARCH = 'amd64'; CGO_ENABLED = '0' }
        print Green "built $out"
    }
}

# go vet and go test over the whole module.
task Test {
    Invoke-Go @('vet', './...')
    Invoke-Go @('test', '-count=1', './...')
}

task Up Build, {
    Import-Module $module -Force
    $p = if ($Port) { Start-Portal -Port $Port } else { Start-Portal }
    print Green "portal at $($p.Url) (process $($p.ProcessId)), booted from $($p.Boot); Invoke-Build Down stops it"
}

task Status {
    Import-Module $module -Force
    $s = Get-PortalStatus
    $state = if ($s.Running) { "running (process $($s.ProcessId), $($s.Mode))" } else { 'not running' }
    print Green "port $($s.Port) · $($s.Url) · boot $(if ($s.Boot) { $s.Boot } else { '-' }) · $state"
}

task Down {
    Import-Module $module -Force
    Stop-Portal -Verbose
}

# The approve-and-reboot loop on a fresh artifacts/smoke over the FileSystem fixture, on port 18200: boot from the seed,
# add a menu entry through the existing POSTs, approve, change the seed Theme's opacity, approve, restart, and assert the
# restart booted from the second stamp with that opacity and the menu entry, that the top bar draws the entry (Chrome's
# --dump-dom; skipped and said so without Chrome) and that the second approval's diff is exactly the Theme node.
# Writes artifacts/smoke-report.json.
task Smoke Build, {
    Import-Module $module -Force
    $artifacts = Join-Path $repo 'artifacts' 'smoke'
    $fixture = Join-Path $repo 'tests' 'fixtures' 'layers' 'filesystem'
    Remove-Item -LiteralPath $artifacts -Recurse -Force -ErrorAction SilentlyContinue
    $report = [ordered]@{ startedAt = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'); result = 'failed'; checks = [ordered]@{} }
    $p = $null
    try {
        $p = Start-Portal -Port $testPort -Artifacts $artifacts -Filesystem $fixture
        if ($p.Boot -ne 'seed') { throw "a fresh artifacts/smoke booted from $($p.Boot), not seed" }
        $screen = Get-PortalScreen -Url $p.Url
        $ids = @($screen.nodes.id)
        foreach ($id in 'ui:root', 'ui:layout:root', 'ui:theme:default', 'ui:canvas', 'ui:designer', 'ui:category:Canvas', 'ui:category:Table', 'ui:category:Detail', 'ui:category:Designer', 'ui:category:Menu', 'ui:menu:graph') {
            if ($id -notin $ids) { throw "seed lacks $id" }
        }
        $report.checks.seed = [ordered]@{ boot = $p.Boot; nodes = $screen.nodes.Count; edges = $screen.edges.Count }
        $layers = Invoke-RestMethod "$($p.Url)/api/layers"
        $layer = $layers | Where-Object name -EQ 'filesystem'
        if (-not $layer -or -not $layer.hasOntology) { throw 'layer filesystem not mounted with its ontology' }
        $report.checks.layer = [ordered]@{ name = $layer.name; nodes = $layer.counts.nodes; hasOntology = $layer.hasOntology }

        # A menu entry is a node and two edges through the existing POSTs, as the Designer's form writes it.
        $entry = 'ui:menu:smoke'
        $null = Invoke-RestMethod -Method Post -Uri "$($p.Url)/api/nodes" -ContentType 'application/json' -Body (@{ id = $entry; kind = 'Container'; name = 'Smoke'; properties = @{ order = 1 } } | ConvertTo-Json -Compress)
        $null = Invoke-RestMethod -Method Post -Uri "$($p.Url)/api/edges" -ContentType 'application/json' -Body (@{ kind = 'is-a'; from = $entry; to = 'ui:category:Menu' } | ConvertTo-Json -Compress)
        $null = Invoke-RestMethod -Method Post -Uri "$($p.Url)/api/edges" -ContentType 'application/json' -Body (@{ kind = 'opens'; from = $entry; to = 'ui:designer' } | ConvertTo-Json -Compress)
        $report.checks.menu = [ordered]@{ added = $entry; edges = @("$entry|ui:category:Menu|is-a", "$entry|ui:designer|opens") }

        $a = New-PortalSnap -Url $p.Url -Note 'smoke: the seed and a menu entry'
        if ($a.PSObject.Properties['previous']) { throw "the first approval has a previous snap: $($a.previous)" }
        $patch = @{ properties = @{ opacity = 0.35 } } | ConvertTo-Json -Compress
        $null = Invoke-RestMethod -Method Patch -Uri "$($p.Url)/api/nodes/ui:theme:default" -Body $patch -ContentType 'application/json'
        $b = New-PortalSnap -Url $p.Url -Note 'smoke: lighter glass'
        $report.checks.approvals = @(
            [ordered]@{ stamp = $a.stamp; note = $a.note }
            [ordered]@{ stamp = $b.stamp; note = $b.note; previous = $b.previous; diff = $b.diff }
        )
        $changed = @($b.diff.changed.nodes)
        $others = @($b.diff.added.nodes) + @($b.diff.added.edges) + @($b.diff.removed.nodes) + @($b.diff.removed.edges) + @($b.diff.changed.edges)
        if ($b.previous -ne $a.stamp) { throw "the second approval's previous is $($b.previous), not $($a.stamp)" }
        if ($changed.Count -ne 1 -or $changed[0] -ne 'ui:theme:default' -or $others.Count -ne 0) {
            throw "the second approval's diff is not exactly the Theme node: $($b.diff | ConvertTo-Json -Depth 4 -Compress)"
        }

        Stop-Portal -Artifacts $artifacts
        $p = Start-Portal -Port $testPort -Artifacts $artifacts -Filesystem $fixture
        $after = Get-PortalScreen -Url $p.Url
        $opacity = ($after.nodes | Where-Object id -EQ 'ui:theme:default').properties.opacity
        $report.checks.restart = [ordered]@{ boot = $p.Boot; themeOpacity = $opacity }
        if ($p.Boot -ne $b.stamp) { throw "the restart booted from $($p.Boot), not the second approval $($b.stamp)" }
        if ($opacity -ne 0.35) { throw "the restart's Theme opacity is $opacity, not 0.35" }
        $d = Compare-PortalSnap $a $b -Url $p.Url
        if (@($d.changed.nodes).Count -ne 1 -or $d.changed.nodes[0].id -ne 'ui:theme:default') { throw 'GET /api/diff disagrees with the approval' }
        foreach ($id in $report.checks.menu.edges) { if ($id -notin @($after.edges.id)) { throw "the restart lacks the menu edge $id" } }
        $report.checks.menu.restart = Test-TopBarMenu -Url $p.Url -Entries @('ui:menu:graph', $entry)

        $report.result = 'passed'
        print Green "smoke: seed $($report.checks.seed.nodes) nodes; approved $($a.stamp), then $($b.stamp) changing $($changed -join ', '); restart booted from $($p.Boot)"
    }
    catch {
        $report.error = "$_"
        throw
    }
    finally {
        if ($p) { Stop-Portal -Artifacts $artifacts }
        $report.finishedAt = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $repo 'artifacts')
        $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $repo 'artifacts' 'smoke-report.json') -Encoding utf8NoBOM
    }
}

task Pester Build, {
    & pwsh -NoProfile -File (Join-Path $repo 'tests' 'Invoke-Tests.ps1')
    if ($LASTEXITCODE -ne 0) { throw "Pester failed: $LASTEXITCODE test(s)" }
}

# A headless capture of the dev seed over the newest FileSystem layer (the fixture when the sibling has none), with
# reduced motion. Also prints how many Canvas nodes took a colour from the layer ontology (data-coloured).
task Screenshot Build, {
    $browser = Find-HeadlessBrowser
    if (-not $browser) { print Yellow 'Screenshot skipped: neither msedge nor chrome found.'; return }
    Import-Module $module -Force
    $work = Join-Path ([IO.Path]::GetTempPath()) "Portal-shot-$([guid]::NewGuid())"
    $fs = 'C:\__Code\Graph.Emitter.FileSystem\artifacts\layers'
    if (-not (Test-Path -LiteralPath $fs)) { $fs = Join-Path $repo 'tests' 'fixtures' 'layers' 'filesystem' }
    $outDir = Join-Path $repo 'docs' 'screenshots'
    $null = New-Item -ItemType Directory -Force -Path $outDir, $work
    $png = Join-Path $outDir 'portal.png'
    try {
        $p = Start-Portal -Port $testPort -Artifacts (Join-Path $work 'artifacts') -Filesystem $fs
        $common = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', '--force-prefers-reduced-motion', '--virtual-time-budget=8000')
        Remove-Item -LiteralPath $png -ErrorAction SilentlyContinue
        $null = Start-Process -FilePath $browser -ArgumentList ($common + @("--screenshot=$png", '--window-size=1400,900', '--hide-scrollbars', "$($p.Url)/")) -Wait -NoNewWindow -PassThru `
            -RedirectStandardError (Join-Path $work 'b.err') -RedirectStandardOutput (Join-Path $work 'b.out')
        if (-not (Test-Path -LiteralPath $png)) { throw "Screenshot: $png not written; see $work" }
        # Edge's new headless mode prints no --dump-dom output on Windows; Chrome's does.
        $domBrowser = @(Find-HeadlessBrowser -Prefer chrome)[0]
        $dom = Join-Path $work 'dom.html'
        $null = Start-Process -FilePath $domBrowser -ArgumentList ($common + @('--dump-dom', "$($p.Url)/")) -Wait -NoNewWindow -PassThru `
            -RedirectStandardOutput $dom -RedirectStandardError (Join-Path $work 'd.err')
        $text = Get-Content -LiteralPath $dom -Raw -ErrorAction SilentlyContinue
        $coloured = if ($text) { [regex]::Match($text, 'data-coloured="([^"]*)"').Groups[1].Value } else { 'unknown (the browser printed no DOM)' }
        print Green "captured $png; canvas coloured from ontology: $coloured"
    }
    finally {
        Stop-Portal -Artifacts (Join-Path $work 'artifacts')
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Loads the page in headless Chrome (--dump-dom; Edge's new headless prints none on Windows) and throws unless the top
# bar draws exactly -Entries, in that order. Returns what it saw, or that it skipped and why when there is no Chrome.
function Test-TopBarMenu {
    param([Parameter(Mandatory)] [string] $Url, [Parameter(Mandatory)] [string[]] $Entries)
    $chrome = @(Find-HeadlessBrowser -Prefer chrome) | Where-Object { $_ -match 'chrome' } | Select-Object -First 1
    if (-not $chrome) { return [ordered]@{ topBar = 'skipped: no Chrome for --dump-dom' } }
    $work = Join-Path ([IO.Path]::GetTempPath()) "Portal-menu-$([guid]::NewGuid())"
    $null = New-Item -ItemType Directory -Force -Path $work
    try {
        $dom = Join-Path $work 'dom.html'
        $argv = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', '--virtual-time-budget=8000', '--dump-dom', "$Url/")
        $null = Start-Process -FilePath $chrome -ArgumentList $argv -Wait -NoNewWindow -PassThru `
            -RedirectStandardOutput $dom -RedirectStandardError (Join-Path $work 'd.err')
        $text = [string](Get-Content -LiteralPath $dom -Raw -ErrorAction SilentlyContinue)
        $bar = [regex]::Match($text, '<nav[^>]*id="menu"[^>]*>(.*?)</nav>', 'Singleline').Groups[1].Value
        $seen = @([regex]::Matches($bar, 'data-entry="([^"]+)"') | ForEach-Object { $_.Groups[1].Value })
        if (($seen -join ',') -ne ($Entries -join ',')) { throw "the top bar draws [$($seen -join ', ')], not [$($Entries -join ', ')]" }
        [ordered]@{ topBar = $seen }
    }
    finally { Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue }
}

# msedge, then chrome (chrome first with -Prefer chrome): PATH first, then the default install locations.
# Lifted from GraphRenderer's .build.ps1; -Prefer is the portal's.
function Find-HeadlessBrowser {
    param([ValidateSet('msedge', 'chrome')] [string] $Prefer = 'msedge')
    $names = if ($Prefer -eq 'chrome') { 'chrome', 'google-chrome', 'msedge', 'microsoft-edge' } else { 'msedge', 'microsoft-edge', 'chrome', 'google-chrome' }
    foreach ($n in $names) {
        $c = Get-Command $n -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($c -and (Test-Path -LiteralPath $c.Source)) { return $c.Source }
    }
    $known = @(
        "${env:ProgramFiles(x86)}\Microsoft\Edge\Application\msedge.exe"
        "$env:ProgramFiles\Microsoft\Edge\Application\msedge.exe"
        "$env:ProgramFiles\Google\Chrome\Application\chrome.exe"
        "${env:ProgramFiles(x86)}\Google\Chrome\Application\chrome.exe"
        "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe"
    )
    if ($Prefer -eq 'chrome') { $known = @($known | Where-Object { $_ -like '*chrome.exe' }) + @($known | Where-Object { $_ -notlike '*chrome.exe' }) }
    foreach ($k in $known) { if ($k -and (Test-Path -LiteralPath $k)) { return $k } }
    return $null
}

task . Modules, Build, Test, Smoke, Pester
