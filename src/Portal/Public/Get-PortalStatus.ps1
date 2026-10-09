function Get-PortalStatus {
    <#
    .SYNOPSIS
        The portal recorded in <Artifacts>/portal.json: Running, Port, Url, Boot (the stamp GET /api/screen's
        X-Portal-Boot header reports now, else the recorded one), ProcessId, Mode. With no record, Running is
        false and Port and Url are the fixed port Start-Portal would use.
    .EXAMPLE
        Get-PortalStatus | Format-List Running, Port, Url, Boot
    #>
    [CmdletBinding()]
    param([string] $Artifacts = $script:ArtifactsRoot)
    $Artifacts = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Artifacts))
    $record = Get-PortalRecord -Artifacts $Artifacts
    $alive = $record -and (Get-Process -Id $record.ProcessId -ErrorAction SilentlyContinue)
    if (-not $alive) {
        $port = Get-PortalPort
        return [pscustomobject]@{
            PSTypeName = 'Portal.Status'
            Running    = $false
            Port       = $port
            Url        = "http://127.0.0.1:$port"
            Boot       = $null
            ProcessId  = $null
            Mode       = $null
        }
    }
    $boot = $record.Boot
    try { $boot = [string](Invoke-WebRequest -Uri "$($record.Url)/api/screen" -TimeoutSec 5).Headers['X-Portal-Boot'] } catch { }
    [pscustomobject]@{
        PSTypeName = 'Portal.Status'
        Running    = $true
        Port       = [int]$record.Port
        Url        = $record.Url
        Boot       = $boot
        ProcessId  = $record.ProcessId
        Mode       = $record.Mode
    }
}
