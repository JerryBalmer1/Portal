#Requires -Version 7.4
# Invoke-Build script. Run: Invoke-Build              (Modules, Build, Test, Smoke, Pester)
#                           Invoke-Build Up           (start the portal on its fixed port over artifacts/: -Port,
#                                                      else PORTAL_PORT from .env, else the environment, else 8200;
#                                                      a running one is stopped and replaced; -NoReplace throws instead)
#                           Invoke-Build Dev          (only for a tenant in mode dev: mint dev-bootstrap, an admin,
#                                                      set PORTAL_TOKEN to its token and run Up; prints the token once,
#                                                      DEV ONLY. Up itself never mints anything)
#                           Invoke-Build Feeds        (not in mode prod: the stock feeds and Postgres up in Docker for the
#                                                      tenant, compose.yaml; then Up, and one sample card pushed to each
#                                                      feed end to end; docs/design.md 40)
#                           Invoke-Build Status       (port, URL and boot stamp of the running portal)
#                           Invoke-Build Down         (stop it)
#                           Invoke-Build Generate     (regenerate internal/gen from api/openapi.yaml)
#                           Invoke-Build Screenshot   (docs/screenshots/: summary.png, service-connections.png,
#                                                      explore.png, identity-users.png)
param(
    # Up's port; absent means PORTAL_PORT from .env, else the environment, else 8200. 0 is refused.
    [int] $Port,
    # Up throws when a portal is already running instead of stopping and replacing it.
    [switch] $NoReplace
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

# A portal already recorded in artifacts/ is stopped, its port waited on (10 s at most) and a new one started in its
# place (docs/design.md 32).
task Up Build, {
    $m = Import-Module $module -Force -PassThru
    $up = @{ NoReplace = [bool]$NoReplace }
    if ($Port) { $up.Port = $Port }
    $p = & $m { param($a) Invoke-PortalUp @a } $up
    if ($p.Replaced) { print Green "replaced $($p.Replaced) with $($p.Boot)" }
    print Green "portal at $($p.Url) (process $($p.ProcessId)), booted from $($p.Boot); Invoke-Build Down stops it"
}

# Dev mode (docs/design.md 39). DevGate refuses, before anything is built or minted, unless the tenant Up serves
# (PORTAL_TENANT, else default, under PORTAL_TENANTS, else tenants/) says mode: dev in its portal.yaml. DevMint makes
# principal dev-bootstrap (admin) in that tenant's gitignored principals.yaml when absent, replaces every dev-bootstrap
# token in tokens.yaml with one new one, and sets PORTAL_TOKEN for this process, which the portal Up starts inherits:
# in mode dev the binary serves that token at GET .../dev-token to 127.0.0.1, the page's "Use dev token".
task DevGate {
    $m = Import-Module $module -Force -PassThru
    $tenant = & $m { Get-PortalTenant }
    $mode = & $m { Get-PortalTenantMode }
    if ($mode -ne 'dev') {
        throw "Dev refuses: tenant $tenant is in mode $mode, not dev. Dev mints an admin and a local sign-in shortcut, so it runs only for a tenant whose portal.yaml says 'mode: dev'. Use Invoke-Build Up, which never mints anything; README, Before you demo."
    }
}

task DevMint DevGate, {
    $m = Import-Module $module -Force -PassThru
    $script:DevMinted = & $m { New-PortalDevToken }
    $env:PORTAL_TOKEN = $script:DevMinted.Token
    print Yellow "minted dev-bootstrap (admin) in $($script:DevMinted.Path); replaced $($script:DevMinted.Replaced) older dev-bootstrap token(s)"
}

task Dev DevGate, DevMint, Up, {
    print Yellow 'DEV ONLY. dev-bootstrap is an admin. Not for demo or production; README, Before you demo.'
    print Yellow "DEV ONLY token (PORTAL_TOKEN for this process; the page's avatar offers it as Use dev token): $($script:DevMinted.Token)"
}

# Feeds (docs/design.md 40). FeedsGate refuses, before anything is built, started or written, when the tenant Up serves
# is in mode prod: the stock services are for dev and demo, as Dev is for dev only. Feeds then sets the compose
# environment (Initialize-PortalFeedEnv: the tenant's data directory, and the NuGet key and Postgres password from .env,
# written there at random the first time), runs `docker compose -p portal-<tenant> up -d --wait` over compose.yaml,
# waits for the NuGet service index, runs Up so the portal holds PORTAL_NUGET_KEY, and pushes the sample card
# (tests/fixtures/cards/Tool.Git, copied from Portal.Tools) to cards-oci and cards-nuget with Publish-PortalCard.
# A card already there (409) is reported, not failed, so Feeds can run again. Then it checks that GET .../feeds lists
# the card in both, that the registry has the tag and that the NuGet feed has the version. The token is PORTAL_TOKEN,
# else in mode dev a fresh dev-bootstrap token (as DevMint mints it); in mode demo PORTAL_TOKEN must be set.
task FeedsGate {
    $m = Import-Module $module -Force -PassThru
    $tenant = & $m { Get-PortalTenant }
    $mode = & $m { Get-PortalTenantMode }
    if ($mode -eq 'prod') {
        throw "Feeds refuses: tenant $tenant is in mode prod. Feeds starts the stock registry, NuGet feed and Postgres in Docker and pushes a sample card, so it runs only for a tenant in mode dev or demo; a prod tenant's feeds are its own (portal.yaml feeds:)."
    }
    if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { throw 'Feeds needs docker (Docker Desktop) on PATH' }
}

task Feeds FeedsGate, Build, {
    $m = Import-Module $module -Force -PassThru
    $mode = & $m { Get-PortalTenantMode }
    $fe = & $m { Initialize-PortalFeedEnv }
    if ($fe.Written) { print Yellow "wrote $($fe.Written -join ' and ') to .env (gitignored)" }
    $compose = Join-Path $repo 'compose.yaml'
    exec { docker compose -p "portal-$($fe.Tenant)" -f $compose up -d --wait }
    print Green "portal-$($fe.Tenant): registry, nuget, postgres up; data under $($fe.Data)"
    $deadline = [datetime]::UtcNow.AddSeconds(60)
    while ($true) {
        try { $null = Invoke-RestMethod 'http://127.0.0.1:8202/v3/index.json' -TimeoutSec 3; break }
        catch { if ([datetime]::UtcNow -gt $deadline) { throw "the NuGet feed did not answer http://127.0.0.1:8202/v3/index.json within 60 s: $_" }; Start-Sleep -Milliseconds 500 }
    }
    if (-not $env:PORTAL_TOKEN) {
        if ($mode -ne 'dev') { throw "Feeds pushes with PORTAL_TOKEN, a contributor's or admin's token for tenant $($fe.Tenant); set it (mode $mode mints nothing)" }
        $env:PORTAL_TOKEN = (& $m { New-PortalDevToken }).Token
        print Yellow 'PORTAL_TOKEN was unset: minted a dev-bootstrap token (DEV ONLY), as Invoke-Build Dev does'
    }
    $up = @{ NoReplace = [bool]$NoReplace }
    if ($Port) { $up.Port = $Port }
    $p = & $m { param($a) Invoke-PortalUp @a } $up
    print Green "portal at $($p.Url) (process $($p.ProcessId)) holds PORTAL_NUGET_KEY"
    $card = Join-Path $repo 'tests' 'fixtures' 'cards' 'Tool.Git'
    foreach ($feed in 'cards-oci', 'cards-nuget') {
        try {
            $e = Publish-PortalCard -Feed $feed -Path $card -Url $p.Url
            print Green "pushed $($e.card) $($e.version) to $feed as $($e.name): hash $($e.hash), digest $($e.digest)"
        }
        catch {
            if ($_.FullyQualifiedErrorId -notlike 'Portal.409*') { throw }
            print Yellow "$feed already holds it (409): $($_.Exception.Message)"
        }
    }
    $listed = Invoke-RestMethod "$($p.Url)/api/tenants/$($fe.Tenant)/feeds"
    foreach ($feed in 'cards-oci', 'cards-nuget') {
        if (-not ($listed.nodes | Where-Object { $_.id -eq "ui:card:$feed/Tool.Git@0.1.0" })) { throw "GET .../feeds lists no Tool.Git 0.1.0 in $feed" }
    }
    $tags = Invoke-RestMethod 'http://127.0.0.1:8201/v2/cards/tool.git/tags/list'
    if ('0.1.0' -notin @($tags.tags)) { throw "the registry has no cards/tool.git:0.1.0 (tags: $($tags.tags -join ', '))" }
    $versions = Invoke-RestMethod 'http://127.0.0.1:8202/v3/package/tool.git/index.json'
    if ('0.1.0' -notin @($versions.versions)) { throw "the NuGet feed has no Tool.Git 0.1.0 (versions: $($versions.versions -join ', '))" }
    print Green "Feeds: Tool.Git 0.1.0 in cards-oci (registry tag) and cards-nuget (feed version), listed by GET .../feeds; $($p.Url)/$($fe.Tenant)/artifacts/feeds"
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

# The approve-and-reboot loop on a fresh artifacts/smoke over the FileSystem fixture, on port 18200, as tenant default
# from artifacts/smoke/tenants with one principal and token, "smoke" (New-PortalToken -Bootstrap, so an admin): boot
# from the seed, check an anonymous write
# is refused, add a menu entry through the existing POSTs, approve, change the seed Theme's opacity, approve, restart,
# and assert the restart booted from the second stamp with that opacity and the menu entry, that the shell's rail draws
# the tenant menu's services (Chrome's --dump-dom; skipped and said so without Chrome), that the Canvas on Graph/Explore
# fits its panel after load (docs/design.md 29, 36) and that the second approval's diff is exactly the Theme node.
# Writes artifacts/smoke-report.json.
task Smoke Build, {
    Import-Module $module -Force
    $artifacts = Join-Path $repo 'artifacts' 'smoke'
    $fixture = Join-Path $repo 'tests' 'fixtures' 'layers' 'filesystem'
    Remove-Item -LiteralPath $artifacts -Recurse -Force -ErrorAction SilentlyContinue
    $report = [ordered]@{ startedAt = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ'); result = 'failed'; checks = [ordered]@{} }
    $p = $null
    $tenants = Join-Path $artifacts 'tenants'
    $null = New-Item -ItemType Directory -Force -Path (Join-Path $tenants 'default')
    Copy-Item -LiteralPath (Join-Path $repo 'tenants' 'default' 'portal.yaml') -Destination (Join-Path $tenants 'default')
    $savedEnv = @{ PORTAL_TOKEN = $env:PORTAL_TOKEN; PORTAL_TENANT = $env:PORTAL_TENANT; PORTAL_TENANTS = $env:PORTAL_TENANTS }
    $env:PORTAL_TENANT = $null
    $env:PORTAL_TENANTS = $tenants
    $env:PORTAL_TOKEN = (New-PortalToken -Name smoke -Bootstrap -Tenants $tenants).Token
    $auth = @{ Authorization = "Bearer $env:PORTAL_TOKEN" }
    $api = '/api/tenants/default'
    try {
        $p = Start-Portal -Port $testPort -Artifacts $artifacts -Filesystem $fixture
        if ($p.Boot -ne 'seed') { throw "a fresh artifacts/smoke booted from $($p.Boot), not seed" }
        $screen = Get-PortalScreen -Url $p.Url
        $ids = @($screen.nodes.id)
        foreach ($id in 'ui:root', 'ui:layout:root', 'ui:theme:default', 'ui:canvas', 'ui:designer', 'ui:category:Canvas', 'ui:category:Table', 'ui:category:Detail', 'ui:category:Designer', 'ui:category:Menu', 'ui:menu:graph', 'ui:canvas:agent-runs', 'ui:menu:runs') {
            if ($id -notin $ids) { throw "seed lacks $id" }
        }
        $report.checks.seed = [ordered]@{ boot = $p.Boot; nodes = $screen.nodes.Count; edges = $screen.edges.Count }
        $anon = Invoke-WebRequest -Method Post -Uri "$($p.Url)$api/snap" -SkipHttpErrorCheck
        if ([int]$anon.StatusCode -ne 401) { throw "an anonymous POST /snap answered $([int]$anon.StatusCode), not 401" }
        $me = Invoke-RestMethod "$($p.Url)$api/auth/whoami" -Headers $auth
        if ($me.role -ne 'admin' -or $me.id -ne 'ui:principal:smoke') { throw "the bootstrap token is $($me.id) $($me.role), not ui:principal:smoke admin" }
        $report.checks.auth = [ordered]@{ anonymousWrite = [int]$anon.StatusCode; principal = $me.name; role = $me.role }
        $layers = Invoke-RestMethod "$($p.Url)$api/layers"
        $layer = $layers | Where-Object name -EQ 'filesystem'
        if (-not $layer -or -not $layer.hasOntology) { throw 'layer filesystem not mounted with its ontology' }
        $report.checks.layer = [ordered]@{ name = $layer.name; nodes = $layer.counts.nodes; hasOntology = $layer.hasOntology }

        # A menu entry is a node and two edges through the existing POSTs, as the Designer's form writes it.
        $entry = 'ui:menu:smoke'
        $null = Invoke-RestMethod -Method Post -Headers $auth -Uri "$($p.Url)$api/nodes" -ContentType 'application/json' -Body (@{ id = $entry; kind = 'Container'; name = 'Smoke'; properties = @{ order = 2 } } | ConvertTo-Json -Compress)
        $null = Invoke-RestMethod -Method Post -Headers $auth -Uri "$($p.Url)$api/edges" -ContentType 'application/json' -Body (@{ kind = 'is-a'; from = $entry; to = 'ui:category:Menu' } | ConvertTo-Json -Compress)
        $null = Invoke-RestMethod -Method Post -Headers $auth -Uri "$($p.Url)$api/edges" -ContentType 'application/json' -Body (@{ kind = 'opens'; from = $entry; to = 'ui:designer' } | ConvertTo-Json -Compress)
        $report.checks.menu = [ordered]@{ added = $entry; edges = @("$entry|ui:category:Menu|is-a", "$entry|ui:designer|opens") }

        $a = New-PortalSnap -Url $p.Url -Note 'smoke: the seed and a menu entry'
        if ($a.PSObject.Properties['previous']) { throw "the first approval has a previous snap: $($a.previous)" }
        $patch = @{ properties = @{ opacity = 0.35 } } | ConvertTo-Json -Compress
        $null = Invoke-RestMethod -Method Patch -Headers $auth -Uri "$($p.Url)$api/nodes/ui:theme:default" -Body $patch -ContentType 'application/json'
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
        $services = @((Invoke-RestMethod "$($p.Url)$api/menu").nodes | Where-Object kind -EQ 'Service' | ForEach-Object id)
        $report.checks.rail = Test-Rail -Url $p.Url -Services $services
        $report.checks.canvasFit = Test-CanvasFit -Url $p.Url

        $report.result = 'passed'
        print Green "smoke: seed $($report.checks.seed.nodes) nodes; approved $($a.stamp), then $($b.stamp) changing $($changed -join ', '); restart booted from $($p.Boot); canvas fit $($report.checks.canvasFit | ConvertTo-Json -Compress)"
    }
    catch {
        $report.error = "$_"
        throw
    }
    finally {
        if ($p) { Stop-Portal -Artifacts $artifacts }
        foreach ($k in $savedEnv.Keys) { [Environment]::SetEnvironmentVariable($k, $savedEnv[$k]) }
        $report.finishedAt = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ')
        $null = New-Item -ItemType Directory -Force -Path (Join-Path $repo 'artifacts')
        $report | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $repo 'artifacts' 'smoke-report.json') -Encoding utf8NoBOM
    }
}

task Pester Build, {
    & pwsh -NoProfile -File (Join-Path $repo 'tests' 'Invoke-Tests.ps1')
    if ($LASTEXITCODE -ne 0) { throw "Pester failed: $LASTEXITCODE test(s)" }
}

# Headless captures of the shell at 1400x900 with reduced motion, over the newest FileSystem layer (the fixture when the
# sibling has none) and, when Claude.Agent has one, its newest agent-runs layer (docs/design.md 35): Overview/Summary,
# Settings/Service connections (the tenant holds connections.example.yaml's github connection), Graph/Explore and
# Identity/Users (principals.example.yaml's admin). The tenant is a temporary copy of tenants/default, so the repo's own
# principals.yaml, tokens.yaml and connections.yaml are never read.
# Also prints how many Canvas nodes took a colour from the layer ontology (data-coloured).
task Screenshot Build, {
    # Screenshots are shown to others: never of a dev-mode tenant (CLAUDE.md, docs/design.md 39).
    $shotMode = & (Import-Module $module -Force -PassThru) { param($t) Get-PortalTenantMode -Tenant default -Tenants $t } (Join-Path $repo 'tenants')
    if ($shotMode -eq 'dev') {
        throw "Screenshot refuses: tenants/default/portal.yaml is mode dev, and docs/screenshots are shown to others. Switch it to demo first (README, Before you demo)."
    }
    $browser = Find-HeadlessBrowser
    if (-not $browser) { print Yellow 'Screenshot skipped: neither msedge nor chrome found.'; return }
    Import-Module $module -Force
    $work = Join-Path ([IO.Path]::GetTempPath()) "Portal-shot-$([guid]::NewGuid())"
    $fs = 'C:\__Code\Graph.Emitter.FileSystem\artifacts\layers'
    if (-not (Test-Path -LiteralPath $fs)) { $fs = Join-Path $repo 'tests' 'fixtures' 'layers' 'filesystem' }
    $outDir = Join-Path $repo 'docs' 'screenshots'
    $null = New-Item -ItemType Directory -Force -Path $outDir, $work
    $tenants = Join-Path $work 'tenants'
    $null = New-Item -ItemType Directory -Force -Path (Join-Path $tenants 'default')
    Copy-Item -LiteralPath (Join-Path $repo 'tenants' 'default' 'portal.yaml') -Destination (Join-Path $tenants 'default')
    Copy-Item -LiteralPath (Join-Path $repo 'tenants' 'default' 'connections.example.yaml') -Destination (Join-Path $tenants 'default' 'connections.yaml')
    Copy-Item -LiteralPath (Join-Path $repo 'tenants' 'default' 'principals.example.yaml') -Destination (Join-Path $tenants 'default' 'principals.yaml')
    try {
        $runs = 'C:\__Code\Claude.Agent\artifacts\layers'
        $mount = if (Test-Path -LiteralPath $runs) { @{ 'agent-runs' = $runs } } else { @{} }
        $p = Start-Portal -Port $testPort -Artifacts (Join-Path $work 'artifacts') -Filesystem $fs -Mount $mount -Tenant default -Tenants $tenants
        $common = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', '--force-prefers-reduced-motion', '--virtual-time-budget=8000')
        foreach ($shot in @(
                @{ Name = 'summary.png'; Route = '/default/overview/summary' }
                @{ Name = 'service-connections.png'; Route = '/default/settings/service-connections' }
                @{ Name = 'explore.png'; Route = '/default/graph/explore' }
                @{ Name = 'identity-users.png'; Route = '/default/identity/users' })) {
            $png = Join-Path $outDir $shot.Name
            Remove-Item -LiteralPath $png -ErrorAction SilentlyContinue
            $null = Start-Process -FilePath $browser -ArgumentList ($common + @("--screenshot=$png", '--window-size=1400,900', '--hide-scrollbars', "$($p.Url)$($shot.Route)")) -Wait -NoNewWindow -PassThru `
                -RedirectStandardError (Join-Path $work 'b.err') -RedirectStandardOutput (Join-Path $work 'b.out')
            if (-not (Test-Path -LiteralPath $png)) { throw "Screenshot: $png not written; see $work" }
            print Green "captured $png"
        }
        # Edge's new headless mode prints no --dump-dom output on Windows; Chrome's does.
        $domBrowser = @(Find-HeadlessBrowser -Prefer chrome)[0]
        $dom = Join-Path $work 'dom.html'
        $null = Start-Process -FilePath $domBrowser -ArgumentList ($common + @('--dump-dom', "$($p.Url)/default/graph/explore")) -Wait -NoNewWindow -PassThru `
            -RedirectStandardOutput $dom -RedirectStandardError (Join-Path $work 'd.err')
        $text = Get-Content -LiteralPath $dom -Raw -ErrorAction SilentlyContinue
        $coloured = if ($text) { [regex]::Match($text, 'data-coloured="([^"]*)"').Groups[1].Value } else { 'unknown (the browser printed no DOM)' }
        print Green "canvas coloured from ontology: $coloured"
    }
    finally {
        Stop-Portal -Artifacts (Join-Path $work 'artifacts')
        Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Loads Overview/Summary in headless Chrome (--dump-dom; Edge's new headless prints none on Windows) and throws unless
# the rail draws exactly -Services (the menu's Service ids), in that order. Returns what it saw, or that it skipped and
# why when there is no Chrome.
function Test-Rail {
    param([Parameter(Mandatory)] [string] $Url, [Parameter(Mandatory)] [string[]] $Services)
    $chrome = @(Find-HeadlessBrowser -Prefer chrome) | Where-Object { $_ -match 'chrome' } | Select-Object -First 1
    if (-not $chrome) { return [ordered]@{ rail = 'skipped: no Chrome for --dump-dom' } }
    $work = Join-Path ([IO.Path]::GetTempPath()) "Portal-rail-$([guid]::NewGuid())"
    $null = New-Item -ItemType Directory -Force -Path $work
    try {
        $dom = Join-Path $work 'dom.html'
        $argv = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', '--virtual-time-budget=8000', '--dump-dom', "$Url/default/overview/summary")
        $null = Start-Process -FilePath $chrome -ArgumentList $argv -Wait -NoNewWindow -PassThru `
            -RedirectStandardOutput $dom -RedirectStandardError (Join-Path $work 'd.err')
        $text = [string](Get-Content -LiteralPath $dom -Raw -ErrorAction SilentlyContinue)
        $rail = [regex]::Match($text, '<nav[^>]*id="rail"[^>]*>(.*?)</nav>', 'Singleline').Groups[1].Value
        $seen = @([regex]::Matches($rail, 'data-service="([^"]+)"') | ForEach-Object { $_.Groups[1].Value })
        if (($seen -join ',') -ne ($Services -join ',')) { throw "the rail draws [$($seen -join ', ')], not [$($Services -join ', ')]" }
        [ordered]@{ rail = $seen }
    }
    finally { Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue }
}

# Loads Graph/Explore in headless Chrome at 1400x900 and throws unless the Canvas's drawn graph (Cytoscape's rendered
# bounding box, which canvas.js records as data-fit="<graph>/<panel>" for width and data-fit-height for height after each
# fit) fills more than half its panel one way: a page-wide panel is fitted by its height (docs/design.md 29, 36). Returns
# the numbers, or that it skipped and why when there is no Chrome.
function Test-CanvasFit {
    param([Parameter(Mandatory)] [string] $Url)
    $chrome = @(Find-HeadlessBrowser -Prefer chrome) | Where-Object { $_ -match 'chrome' } | Select-Object -First 1
    if (-not $chrome) { return [ordered]@{ canvasFit = 'skipped: no Chrome for --dump-dom' } }
    $work = Join-Path ([IO.Path]::GetTempPath()) "Portal-fit-$([guid]::NewGuid())"
    $null = New-Item -ItemType Directory -Force -Path $work
    try {
        $dom = Join-Path $work 'dom.html'
        $argv = @('--headless=new', "--user-data-dir=$(Join-Path $work 'profile')", '--no-first-run', '--virtual-time-budget=8000', '--window-size=1400,900', '--dump-dom', "$Url/default/graph/explore")
        $null = Start-Process -FilePath $chrome -ArgumentList $argv -Wait -NoNewWindow -PassThru `
            -RedirectStandardOutput $dom -RedirectStandardError (Join-Path $work 'd.err')
        $text = [string](Get-Content -LiteralPath $dom -Raw -ErrorAction SilentlyContinue)
        $m = [regex]::Match($text, 'data-fit="(\d+)/(\d+)"')
        $mh = [regex]::Match($text, 'data-fit-height="(\d+)/(\d+)"')
        if (-not $m.Success -or -not $mh.Success) { throw 'the Canvas recorded no fit (data-fit): it never had a size' }
        $graph, $panel = [int]$m.Groups[1].Value, [int]$m.Groups[2].Value
        $graphH, $panelH = [int]$mh.Groups[1].Value, [int]$mh.Groups[2].Value
        if ($graph -le $panel / 2 -and $graphH -le $panelH / 2) { throw "the Canvas draws its graph $graph x $graphH px in a $panel x $panelH px panel: not above half either way" }
        [ordered]@{ graphWidth = $graph; panelWidth = $panel; graphHeight = $graphH; panelHeight = $panelH; ratio = [math]::Round([math]::Max($graph / $panel, $graphH / $panelH), 3) }
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
