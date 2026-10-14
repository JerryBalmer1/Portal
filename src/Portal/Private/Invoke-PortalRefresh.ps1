function Invoke-PortalRefresh {
    <#
    .SYNOPSIS
    Invoke-Build Refresh (docs/design.md 56): every step of docs/refresh/order.psd1 in order, one line each, after
    checking the order against rules R1 to R6 (Test-PortalRefreshOrder). Returns one result per step.
    .DESCRIPTION
    An invoke step runs `Invoke-Build <Task> -File <Repo>\<File>` in a child pwsh whose output goes to
    artifacts/refresh/<stamp>/<n>-<id>.log, through files rather than a pipe: a portal the step starts inherits the
    child's handles, and an inherited pipe would never close. A repo or build file not on disk, and a
    SkipWithoutDocker step with no Docker engine, are skipped with one line. Portal is this repo; a dependent is the
    folder of that name beside it. approve POSTs .../snap and catalogue GETs .../emitters, each with the dev token
    fetched from GET .../dev-token immediately before its call, never earlier and never PORTAL_TOKEN (docs/design.md
    59: every Portal start mints a new one); a portal started with Up (no dev token, 404) fails the step naming it.
    Each line names the token's first 8 characters, and artifacts/refresh/<stamp>/refresh.log keeps every step's
    line. An invoke step still running prints its elapsed seconds every -ProgressSeconds. catalogue prints the
    table and fails, naming each
    card, when one that read ok in artifacts/refresh/last.json reads no-layers or version-mismatch; last.json is
    written only when it passes, failed.json when it does not. -DryRun prints what each step would do and runs
    nothing. After Portal's own down step, the port Up would use must be free, or that step fails naming the
    owner (docs/design.md 57): a Down that stopped nothing while 8200 is held fails there, not at the next step.
    -Handlers replaces the invoke, approve, catalogue, docker, portfree, url, token or api handler (Pester).
    #>
    [CmdletBinding()]
    param(
        [string] $OrderPath = (Join-Path $script:RepoRoot 'docs' 'refresh' 'order.psd1'),
        [string] $Workspace = (Split-Path $script:RepoRoot -Parent),
        [string] $Artifacts = (Join-Path $script:ArtifactsRoot 'refresh'),
        [switch] $DryRun,
        [ValidateRange(1, 3600)] [int] $ProgressSeconds = 15,
        [hashtable] $Handlers = @{}
    )
    $order = Get-PortalRefreshOrder -Path $OrderPath
    $problems = @(Test-PortalRefreshOrder -Steps $order.Steps)
    if ($problems) { throw "$OrderPath breaks the Refresh rules: $($problems -join '; ')" }
    $tenant = Get-PortalTenant
    $stamp = [datetime]::UtcNow.ToString("yyyy-MM-dd'T'HH-mm-ss'Z'", [cultureinfo]::InvariantCulture)
    $logDir = Join-Path $Artifacts $stamp

    $h = @{
        docker    = {
            if (-not (Get-Command docker -ErrorAction SilentlyContinue)) { return $false }
            $null = & docker info --format '{{.ServerVersion}}' 2>&1
            $LASTEXITCODE -eq 0
        }
        invoke    = {
            param($Step, $File, $Log)
            $tick = { param($Seconds) Write-Host "$Seconds s... " -NoNewline -ForegroundColor DarkGray }
            Invoke-PortalRefreshStep -Task $Step.Task -File $File -Log $Log -ProgressSeconds $ProgressSeconds -Progress $tick
        }
        portfree  = { param($Step) Assert-PortalPortFree -Port (Get-PortalPort) }
        url       = { Resolve-PortalUrl }
        token     = { param($Url) Get-PortalDevToken -Url $Url -Tenant $tenant }
        api       = {
            param($Url, $Token, $Method, $Path, $Body)
            $call = @{ Url = $Url; Path = $Path; Method = $Method; Token = $Token }
            if ($null -ne $Body) { $call.Body = $Body }
            Invoke-PortalApi @call
        }
        approve   = {
            param($Step)
            $c = & $callApi 'POST' '/snap' @{ note = "Invoke-Build Refresh $stamp" }
            $snap = $c.Data
            $changed = 0
            if ($snap.PSObject.Properties['diff'] -and $snap.diff) {
                foreach ($part in 'added', 'removed', 'changed') {
                    $d = $snap.diff.PSObject.Properties[$part]
                    if ($d -and $d.Value) { foreach ($k in 'nodes', 'edges') { $q = $d.Value.PSObject.Properties[$k]; if ($q) { $changed += @($q.Value).Count } } }
                }
            }
            $since = if ($snap.PSObject.Properties['previous'] -and $snap.previous) { "$changed ids changed since $($snap.previous)" } else { 'the first snap' }
            [pscustomobject]@{ Ok = $true; Line = "approved snap $($snap.stamp) ($since); dev token $($c.Prefix)"; After = $null }
        }
        catalogue = {
            param($Step)
            $c = & $callApi 'GET' '/emitters' $null
            $url = $c.Url
            $rows = @(ConvertTo-PortalCatalogueRow -Envelope $c.Data)
            $last = Join-Path $Artifacts 'last.json'
            $previous = if (Test-Path -LiteralPath $last) { @((Get-Content -LiteralPath $last -Raw | ConvertFrom-Json).rows) } else { @() }
            $regressed = @(Compare-PortalCatalogue -Previous $previous -Current $rows)
            $record = [ordered]@{ finished = [datetime]::UtcNow.ToString('o'); tenant = $tenant; url = $url; refresh = $stamp; rows = $rows }
            $null = New-Item -ItemType Directory -Force -Path $Artifacts
            $json = $record | ConvertTo-Json -Depth 5
            $table = ($rows | Format-Table Name, Kind, Status, Version, Layers, LayerVersions -AutoSize | Out-String -Width 200).TrimEnd()
            $counts = ($rows | Group-Object Status | Sort-Object Name | ForEach-Object { "$($_.Count) $($_.Name)" }) -join ', '
            if ($regressed) {
                Set-Content -LiteralPath (Join-Path $Artifacts 'failed.json') -Value $json -Encoding utf8NoBOM
                $named = ($regressed | ForEach-Object { "$($_.Name) ok -> $($_.Now)" }) -join '; '
                return [pscustomobject]@{ Ok = $false; Line = "REGRESSED since the last Refresh: $named; dev token $($c.Prefix)"; After = $table }
            }
            Set-Content -LiteralPath $last -Value $json -Encoding utf8NoBOM
            $was = if ($previous) { 'compared with the last Refresh' } else { 'no last Refresh to compare with' }
            [pscustomobject]@{ Ok = $true; Line = "$($rows.Count) rows ($counts); $was; wrote $last; dev token $($c.Prefix)"; After = $table }
        }
    }
    foreach ($k in $Handlers.Keys) { $h[$k] = $Handlers[$k] }
    # One API call of a step: the dev token is fetched here, immediately before the call, every time (docs/design.md
    # 59). Never PORTAL_TOKEN, which this process may have inherited from an older Dev, and never a token an earlier
    # step held: a step before this one may have restarted the portal, and every start mints a new token.
    $callApi = {
        param([string] $Method, [string] $Path, $Body)
        $url = & $h.url
        $token = & $h.token $url
        $prefix = "$(([string]$token).Substring(0, [math]::Min(8, ([string]$token).Length)))..."
        try { $data = & $h.api $url $token $Method $Path $Body }
        catch { throw "$($_.Exception.Message) (sent dev token $prefix, fetched from GET /api/tenants/$tenant/dev-token just before)" }
        [pscustomobject]@{ Data = $data; Prefix = $prefix; Url = $url }
    }

    $say = { param([string] $Text, [string] $Color = 'Gray') Write-Host $Text -ForegroundColor $Color }
    $runLog = Join-Path $logDir 'refresh.log'
    $record = { param([string] $Text) if (-not $DryRun) { $null = New-Item -ItemType Directory -Force -Path $logDir; Add-Content -LiteralPath $runLog -Value $Text -Encoding utf8NoBOM } }
    & $record "Refresh $stamp`: $($order.Steps.Count) steps from $OrderPath; tenant $tenant; workspace $Workspace"
    & $say "Refresh $stamp$(if ($DryRun) { ' (dry run)' }): $($order.Steps.Count) steps from $OrderPath; tenant $tenant; workspace $Workspace" Cyan
    $gn = Join-Path $Workspace 'Graph.Node' 'src' 'GraphNode' 'GraphNode.psd1'
    if (Test-Path -LiteralPath $gn) { & $say "Graph.Node checkout: GraphNode $((Import-PowerShellDataFile -LiteralPath $gn).ModuleVersion) at $(Split-Path (Split-Path (Split-Path $gn)))" }
    else { & $say "Graph.Node checkout: not on disk ($gn); the dependents that import it will fail" Yellow }
    $named = @($order.Steps.Repo) + @($order.CardOnly) | Sort-Object -Unique
    foreach ($repo in $order.CardOnly) {
        $dir = Join-Path $Workspace $repo
        if (-not (Test-Path -LiteralPath $dir)) { & $say "$repo`: not on disk, skipped"; continue }
        $builds = @(Get-ChildItem -LiteralPath $dir -File -Filter '*.build.ps1' -Force -ErrorAction SilentlyContinue)
        if ($builds) { & $say "$repo`: has $($builds.Name -join ', ') now but no step in order.psd1; rerun the discovery in docs/refresh/dependency-order.md" Yellow }
        else { & $say "$repo`: no build script, nothing to refresh" }
    }
    foreach ($dir in @(Get-ChildItem -LiteralPath $Workspace -Directory -Filter 'Portal.*' -ErrorAction SilentlyContinue)) {
        if ($dir.Name -notin $named) { & $say "$($dir.Name): on disk but not in order.psd1; rerun the discovery in docs/refresh/dependency-order.md" Yellow }
    }

    $results = [System.Collections.Generic.List[object]]::new()
    $n = $order.Steps.Count
    for ($i = 0; $i -lt $n; $i++) {
        $step = $order.Steps[$i]
        $root = if ($step.Repo -eq 'Portal') { $script:RepoRoot } else { Join-Path $Workspace $step.Repo }
        $what = switch ($step.Action) {
            'invoke' { "Invoke-Build $($step.Task)" }
            'approve' { "POST /api/tenants/$tenant/snap" }
            'catalogue' { "GET /api/tenants/$tenant/emitters" }
        }
        $head = '[{0}/{1}] {2,-15} {3,-16} {4}: ' -f ($i + 1), $n, $step.Id, $step.Repo, $what
        $file = if ($step.Action -eq 'invoke') { Join-Path $root $step.File }
        $skip = if (-not (Test-Path -LiteralPath $root)) { "skipped: $($step.Repo) not on disk ($root)" }
        elseif ($file -and -not (Test-Path -LiteralPath $file)) { "skipped: no $($step.File) in $($step.Repo)" }
        elseif ($step.SkipWithoutDocker -and -not (& $h.docker)) { 'skipped: the Docker engine is not answering, so there is nothing to stop' }
        if ($skip) {
            & $say ($head + $(if ($DryRun) { "would be $skip" } else { $skip })) Yellow
            & $record ($head + $skip)
            $results.Add([pscustomobject]@{ Id = $step.Id; Result = 'skipped'; Line = $skip })
            continue
        }
        if ($DryRun) {
            $plan = switch ($step.Action) {
                'invoke' { "would run Invoke-Build $($step.Task) -File $file" }
                'approve' { 'would fetch the dev token and approve a snap' }
                'catalogue' { "would fetch the dev token, print the Catalogue and compare it with $(Join-Path $Artifacts 'last.json')" }
            }
            & $say ($head + $plan)
            $results.Add([pscustomobject]@{ Id = $step.Id; Result = 'planned'; Line = $plan })
            continue
        }
        Write-Host $head -NoNewline -ForegroundColor Gray
        $clock = [Diagnostics.Stopwatch]::StartNew()
        $r = try {
            switch ($step.Action) {
                'invoke' {
                    $null = New-Item -ItemType Directory -Force -Path $logDir
                    & $h.invoke $step $file (Join-Path $logDir ('{0:d2}-{1}.log' -f ($i + 1), $step.Id))
                }
                'approve' { & $h.approve $step }
                'catalogue' { & $h.catalogue $step }
            }
        }
        catch { [pscustomobject]@{ Ok = $false; Line = "FAILED: $($_.Exception.Message)"; After = $null } }
        if (-not $r) { $r = [pscustomobject]@{ Ok = $false; Line = 'FAILED: the step returned no result'; After = $null } }
        if ($r.Ok -and $step.Repo -eq 'Portal' -and $step.Phase -eq 'down') {
            try { & $h.portfree $step }
            catch { $r = [pscustomobject]@{ Ok = $false; Line = "FAILED after $($step.Task): $($_.Exception.Message) ($($r.Line))"; After = $null } }
        }
        $seconds = [math]::Round($clock.Elapsed.TotalSeconds, 1)
        Write-Host "$($r.Line) ($seconds s)" -ForegroundColor $(if ($r.Ok) { 'Green' } else { 'Red' })
        & $record "$head$($r.Line) ($seconds s)"
        if ($r.After) { Write-Host $r.After }
        $results.Add([pscustomobject]@{ Id = $step.Id; Result = $(if ($r.Ok) { 'ok' } else { 'failed' }); Line = $r.Line })
        if (-not $r.Ok) { throw "Refresh stopped at step $($i + 1), $($step.Id) ($($step.Repo) $what): $($r.Line)" }
    }
    $results.ToArray()
}
