function Start-Portal {
    <#
    .SYNOPSIS
        Starts the portal binary, waits until GET /api/screen answers, records it in <Artifacts>/portal.json and
        returns the record (ProcessId, Port, Url, Artifacts, Mode, Boot, LogPath).
    .DESCRIPTION
        The binary is build/win-x64/portal.exe at the repo root (Invoke-Build Build). The screen boots from the
        newest snap under <Artifacts>/snaps, or from the one -Snap names (PORTAL_SNAP), else from the dev seed
        (empty with -NoSeed); Boot in the record says which, as GET /api/screen's X-Portal-Boot header does.
        -Filesystem is mounted as layer "filesystem" when it exists (a directory's newest *.graph.json); -Mount
        adds more layers by name. -Production sets PORTAL_MODE=production for the process, which hides every
        node that is-a Designer. Output goes to <Artifacts>/portal.log, never to the console.
    .EXAMPLE
        Start-Portal
    .EXAMPLE
        Start-Portal -Port 0 -Artifacts $env:TEMP\portal -Snap 2026-10-09T01-02-03.456Z -Production
    #>
    [CmdletBinding()]
    param(
        [ValidateRange(0, 65535)] [int] $Port = 8080,
        [string] $Artifacts = $script:ArtifactsRoot,
        [AllowEmptyString()] [string] $Filesystem = $script:DefaultFilesystem,
        [hashtable] $Mount = @{},
        [string] $Snap,
        [switch] $Production,
        [switch] $NoSeed,
        [ValidateRange(1, 300)] [int] $TimeoutSec = 20
    )
    $Artifacts = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Artifacts))
    $running = Get-PortalRecord -Artifacts $Artifacts
    if ($running -and (Get-Process -Id $running.ProcessId -ErrorAction SilentlyContinue)) {
        throw "A portal is already running at $($running.Url) (process $($running.ProcessId)). Stop it with Stop-Portal first."
    }
    $exe = Get-PortalBinaryPath
    $null = New-Item -ItemType Directory -Force -Path $Artifacts
    if ($Port -eq 0) {
        $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
        $listener.Start()
        $Port = $listener.LocalEndpoint.Port
        $listener.Stop()
    }
    $all = @('-port', $Port, '-artifacts', $Artifacts, '-filesystem', $Filesystem)
    foreach ($k in $Mount.Keys) { $all += @('-mount', "$k=$($Mount[$k])") }
    if ($NoSeed) { $all += '-no-seed' }
    $quoted = foreach ($a in $all) {
        $s = [string]$a
        if ($s -eq '' -or $s -match '[\s"]') { '"' + $s.Replace('"', '\"') + '"' } else { $s }
    }
    $log = Join-Path $Artifacts 'portal.log'
    $mode = if ($Production) { 'production' } else { 'development' }
    # The child inherits the environment: set both variables for it, unset PORTAL_SNAP when -Snap is not given.
    $saved = @{ PORTAL_MODE = $env:PORTAL_MODE; PORTAL_SNAP = $env:PORTAL_SNAP }
    try {
        $env:PORTAL_MODE = $mode
        $env:PORTAL_SNAP = $Snap
        $process = Start-Process -FilePath $exe -ArgumentList ($quoted -join ' ') -PassThru -NoNewWindow `
            -RedirectStandardOutput $log -RedirectStandardError "$log.err"
    }
    finally {
        $env:PORTAL_MODE = $saved.PORTAL_MODE
        $env:PORTAL_SNAP = $saved.PORTAL_SNAP
    }
    $url = "http://127.0.0.1:$Port"
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    $boot = $null
    while (-not $boot -and [DateTime]::UtcNow -lt $deadline -and -not $process.HasExited) {
        try { $boot = [string](Invoke-WebRequest -Uri "$url/api/screen" -TimeoutSec 2).Headers['X-Portal-Boot'] }
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
        Artifacts  = $Artifacts
        Mode       = $mode
        Boot       = $boot
        LogPath    = $log
    }
    $record | ConvertTo-Json | Set-Content -LiteralPath (Join-Path $Artifacts 'portal.json') -Encoding utf8NoBOM
    $record
}
