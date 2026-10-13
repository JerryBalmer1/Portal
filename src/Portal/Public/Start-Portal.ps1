function Start-Portal {
    <#
    .SYNOPSIS
        Starts the portal binary, waits until GET /api/tenants/<tenant>/screen answers, records it in
        <Artifacts>/portal.json and returns the record (ProcessId, Port, Url, Tenant, Artifacts, Mode, Boot, LogPath).
    .DESCRIPTION
        The binary is build/win-x64/portal.exe at the repo root (Invoke-Build Build). The port is fixed: -Port,
        else PORTAL_PORT from the repo's .env, else the PORTAL_PORT environment variable, else 8200. 0 is refused,
        and a busy port throws naming the port and the process that owns it, and the Stop-Portal -Artifacts that
        frees it when the owner is this repo's portal; another port is never picked. A portal already serving
        -Artifacts, recorded or not (Get-PortalStatus), throws too. The
        screen boots from the
        newest snap under <Artifacts>/snaps, or from the one -Snap names (PORTAL_SNAP), else from the dev seed
        (empty with -NoSeed); Boot in the record says which, as the screen's X-Portal-Boot header does.
        It serves one tenant, -Tenant (PORTAL_TENANT, else default), from -Tenants: <Tenants>/<Tenant>/portal.yaml
        merged over the product's portal.base.yaml, and that tenant's tokens.yaml and connections.yaml when present.
        -Tenants defaults to PORTAL_TENANTS, else the repo's tenants directory. A tenant file that fails its check
        stops the binary, and the throw carries its message.
        -Filesystem is mounted as layer "filesystem" when it exists (a directory's newest *.graph.json); -Mount
        adds more layers by name. -Production sets PORTAL_MODE=production for the process, which hides every
        node that is-a Designer. Output goes to <Artifacts>/portal.log, never to the console.
    .EXAMPLE
        Start-Portal
    .EXAMPLE
        Start-Portal -Port 18200 -Artifacts $env:TEMP\portal -Snap 2026-10-09T01-02-03.456Z -Production
    #>
    [CmdletBinding()]
    param(
        [int] $Port,
        [string] $Artifacts = $script:ArtifactsRoot,
        [AllowEmptyString()] [string] $Filesystem = $script:DefaultFilesystem,
        [hashtable] $Mount = @{},
        [string] $Snap,
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')] [string] $Tenant = (Get-PortalTenant),
        [string] $Tenants = $(if ($env:PORTAL_TENANTS) { $env:PORTAL_TENANTS } else { Join-Path $script:RepoRoot 'tenants' }),
        [switch] $Production,
        [switch] $NoSeed,
        [ValidateRange(1, 300)] [int] $TimeoutSec = 20
    )
    $Artifacts = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Artifacts))
    $running = Get-PortalStatus -Artifacts $Artifacts
    if ($running.Running) {
        $how = if ($running.Recorded) { '' } else { ', unrecorded' }
        throw "A portal is already running at $($running.Url) (process $($running.ProcessId)$how) over $Artifacts. Stop it with Stop-Portal first."
    }
    $Port = Get-PortalPort -Port ($PSBoundParameters.ContainsKey('Port') ? $Port : $null)
    $exe = Get-PortalBinaryPath
    try { Assert-PortalPortFree -Port $Port }
    catch {
        # This repo's portal over other artifacts: name the Stop-Portal that frees the port (docs/design.md 57).
        $holder = @(Get-PortalProcess | Where-Object Port -EQ $Port) | Select-Object -First 1
        if (-not $holder) { throw }
        throw "$($_.Exception.Message). It is this repo's portal serving $($holder.Artifacts): Stop-Portal -Artifacts '$($holder.Artifacts)' stops it."
    }
    $null = New-Item -ItemType Directory -Force -Path $Artifacts
    $Tenants = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Tenants))
    $all = @('-port', $Port, '-artifacts', $Artifacts, '-tenants', $Tenants, '-filesystem', $Filesystem)
    foreach ($k in $Mount.Keys) { $all += @('-mount', "$k=$($Mount[$k])") }
    if ($NoSeed) { $all += '-no-seed' }
    $quoted = foreach ($a in $all) {
        $s = [string]$a
        if ($s -eq '' -or $s -match '[\s"]') { '"' + $s.Replace('"', '\"') + '"' } else { $s }
    }
    $log = Join-Path $Artifacts 'portal.log'
    $mode = if ($Production) { 'production' } else { 'development' }
    # The child inherits the environment: set both variables for it, unset PORTAL_SNAP when -Snap is not given.
    $saved = @{ PORTAL_MODE = $env:PORTAL_MODE; PORTAL_SNAP = $env:PORTAL_SNAP; PORTAL_TENANT = $env:PORTAL_TENANT }
    try {
        $env:PORTAL_MODE = $mode
        $env:PORTAL_SNAP = $Snap
        $env:PORTAL_TENANT = $Tenant
        $process = Start-Process -FilePath $exe -ArgumentList ($quoted -join ' ') -PassThru -NoNewWindow `
            -RedirectStandardOutput $log -RedirectStandardError "$log.err"
    }
    finally {
        $env:PORTAL_MODE = $saved.PORTAL_MODE
        $env:PORTAL_SNAP = $saved.PORTAL_SNAP
        $env:PORTAL_TENANT = $saved.PORTAL_TENANT
    }
    $url = "http://127.0.0.1:$Port"
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    $boot = $null
    while (-not $boot -and [DateTime]::UtcNow -lt $deadline -and -not $process.HasExited) {
        try { $boot = [string](Invoke-WebRequest -Uri "$url/api/tenants/$Tenant/screen" -TimeoutSec 2).Headers['X-Portal-Boot'] }
        catch { Start-Sleep -Milliseconds 100 }
    }
    if (-not $boot) {
        if (-not $process.HasExited) { Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue }
        $err = if (Test-Path -LiteralPath "$log.err") { Get-Content -LiteralPath "$log.err" -Raw } else { '' }
        throw "The portal did not start on port $Port within $TimeoutSec s. $err".Trim()
    }
    $record = [pscustomobject]@{
        PSTypeName = 'Portal.Process'
        ProcessId  = $process.Id
        Port       = $Port
        Url        = $url
        Tenant     = $Tenant
        Artifacts  = $Artifacts
        Mode       = $mode
        Boot       = $boot
        LogPath    = $log
    }
    $record | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $Artifacts 'portal.json') -Encoding utf8NoBOM
    $record
}
