function Get-PortalStatus {
    <#
    .SYNOPSIS
        The portal serving <Artifacts>: Running, Recorded, Port, Url, Boot (the stamp GET /api/tenants/{tenant}/screen's
        X-Portal-Boot header reports now, else the recorded one), ProcessId, Mode. With none, Running is false and
        Port and Url are the fixed port Start-Portal would use.
    .DESCRIPTION
        The recorded portal in <Artifacts>/portal.json counts only while its process is this repo's portal serving
        <Artifacts>. Without one, a portal this repo's binary runs over <Artifacts> unrecorded is reported with
        Recorded false, its Port from its command line and Mode $null (docs/design.md 57), so Up replaces it and
        Down stops it.
    .EXAMPLE
        Get-PortalStatus | Format-List Running, Recorded, Port, Url, Boot
    #>
    [CmdletBinding()]
    param([string] $Artifacts = $script:ArtifactsRoot)
    $Artifacts = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Artifacts))
    $record = Get-PortalRecord -Artifacts $Artifacts
    $running = @(Get-PortalProcess -Artifacts $Artifacts)
    $recorded = $record -and $record.ProcessId -in @($running | ForEach-Object { $_.ProcessId })
    if ($recorded) {
        # A record from before 0.4.0 has no Tenant; that portal had none either.
        $tenant = if ($record.PSObject.Properties['Tenant']) { $record.Tenant } else { Get-PortalTenant }
        $port, $url, $boot, $id, $mode = [int]$record.Port, $record.Url, $record.Boot, $record.ProcessId, $record.Mode
    }
    elseif ($running) {
        $p = $running[0]
        $port = if ($p.Port) { $p.Port } else { Get-PortalPort }
        $tenant, $url, $boot, $id, $mode = (Get-PortalTenant), "http://127.0.0.1:$port", $null, $p.ProcessId, $null
    }
    else {
        $port = Get-PortalPort
        return [pscustomobject]@{
            PSTypeName = 'Portal.Status'
            Running    = $false
            Recorded   = $false
            Port       = $port
            Url        = "http://127.0.0.1:$port"
            Boot       = $null
            ProcessId  = $null
            Mode       = $null
        }
    }
    try { $boot = [string](Invoke-WebRequest -Uri "$url/api/tenants/$tenant/screen" -TimeoutSec 5).Headers['X-Portal-Boot'] } catch { }
    [pscustomobject]@{
        PSTypeName = 'Portal.Status'
        Running    = $true
        Recorded   = [bool]$recorded
        Port       = $port
        Url        = $url
        Boot       = $boot
        ProcessId  = $id
        Mode       = $mode
    }
}
