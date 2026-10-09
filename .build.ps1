#Requires -Version 7.4
# Invoke-Build script. Run: Invoke-Build              (Modules, Build, Test, Smoke, Pester)
#                           Invoke-Build Up           (start the portal on :8080 over artifacts/)
#                           Invoke-Build Down         (stop it)
#                           Invoke-Build Generate     (regenerate internal/gen from api/openapi.yaml)
#                           Invoke-Build Screenshot   (docs/screenshots/portal.png)
param()

$repo     = $PSScriptRoot
$buildDir = Join-Path $repo 'build'
$module   = Join-Path $repo 'src' 'Portal' 'Portal.psd1'

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
    $p = Start-Portal
    print Green "portal at $($p.Url) (process $($p.ProcessId)); Invoke-Build Down stops it"
}

task Down {
    Import-Module $module -Force
    Stop-Portal -Verbose
}

# The approve-and-reboot loop on a fresh artifacts/smoke over the FileSystem fixture: boot from the seed, approve, change
# the seed Theme's opacity, approve, restart, and assert the restart booted from the second stamp with that opacity and
# that the second approval's diff is exactly the Theme node. Writes artifacts/smoke-report.json.
task Smoke Build, {
    Import-Module $module -Force
    $artifacts = Join-Path $repo 'artifacts' 'smoke'
    $fixture = Join-Path $repo 'tests' 'fixtures' 'layers' 'filesystem'
    Remove-Item -LiteralPath $artifacts -Recurse -Force -ErrorAction SilentlyContinue
    $report = [ordered]@{ startedAt = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'); result = 'failed'; checks = [ordered]@{} }
    $p = $null
    try {
        $p = Start-Portal -Port 0 -Artifacts $artifacts -Filesystem $fixture
        if ($p.Boot -ne 'seed') { throw "a fresh artifacts/smoke booted from $($p.Boot), not seed" }
        $screen = Get-PortalScreen -Url $p.Url
        $ids = @($screen.nodes.id)
        foreach ($id in 'ui:root', 'ui:layout:root', 'ui:theme:default', 'ui:canvas', 'ui:designer', 'ui:category:Canvas', 'ui:category:Table', 'ui:category:Detail', 'ui:category:Designer') {
            if ($id -notin $ids) { throw "seed lacks $id" }
        }
        $report.checks.seed = [ordered]@{ boot = $p.Boot; nodes = $screen.nodes.Count; edges = $screen.edges.Count }
        $layers = Invoke-RestMethod "$($p.Url)/api/layers"
        $layer = $layers | Where-Object name -EQ 'filesystem'
        if (-not $layer -or -not $layer.hasOntology) { throw 'layer filesystem not mounted with its ontology' }
        $report.checks.layer = [ordered]@{ name = $layer.name; nodes = $layer.counts.nodes; hasOntology = $layer.hasOntology }

        $a = New-PortalSnap -Url $p.Url -Note 'smoke: the seed'
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
        $p = Start-Portal -Port 0 -Artifacts $artifacts -Filesystem $fixture
        $after = Get-PortalScreen -Url $p.Url
        $opacity = ($after.nodes | Where-Object id -EQ 'ui:theme:default').properties.opacity
        $report.checks.restart = [ordered]@{ boot = $p.Boot; themeOpacity = $opacity }
        if ($p.Boot -ne $b.stamp) { throw "the restart booted from $($p.Boot), not the second approval $($b.stamp)" }
        if ($opacity -ne 0.35) { throw "the restart's Theme opacity is $opacity, not 0.35" }
        $d = Compare-PortalSnap $a $b -Url $p.Url
        if (@($d.changed.nodes).Count -ne 1 -or $d.changed.nodes[0].id -ne 'ui:theme:default') { throw 'GET /api/diff disagrees with the approval' }

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
        $p = Start-Portal -Port 0 -Artifacts (Join-Path $work 'artifacts') -Filesystem $fs
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
