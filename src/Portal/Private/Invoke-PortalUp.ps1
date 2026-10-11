function Invoke-PortalUp {
    <#
    .SYNOPSIS
    Invoke-Build Up's start: when the portal recorded in <Artifacts>/portal.json is running, Stop-Portal it, wait
    up to -FreeTimeoutSec (10) for its port to free, then Start-Portal. Returns Start-Portal's record with Replaced,
    the old portal's boot stamp ($null when nothing was replaced). -NoReplace calls Start-Portal straight away, so a
    running portal throws as before. A port held by anything Portal did not record is never stopped: Start-Portal
    throws naming its owner (docs/design.md 32). PORTAL_NUGET_KEY, when the environment lacks it, is read from the repo's
    .env (Get-PortalEnvValue; read only, never written), so the portal can push to cards-nuget whether Up or Feeds
    started it (docs/design.md 40).
    #>
    [CmdletBinding()]
    param(
        [int] $Port,
        [string] $Artifacts = $script:ArtifactsRoot,
        [AllowEmptyString()] [string] $Filesystem = $script:DefaultFilesystem,
        [switch] $NoReplace,
        [ValidateRange(1, 300)] [int] $FreeTimeoutSec = 10,
        [string] $EnvFile = (Join-Path $script:RepoRoot '.env')
    )
    $old = $null
    if (-not $NoReplace) {
        $status = Get-PortalStatus -Artifacts $Artifacts
        if ($status.Running) {
            $old = $status.Boot
            Stop-Portal -Artifacts $Artifacts
            Wait-PortalPortFree -Port $status.Port -TimeoutSec $FreeTimeoutSec
        }
    }
    if (-not $env:PORTAL_NUGET_KEY) {
        $key = Get-PortalEnvValue -Name PORTAL_NUGET_KEY -EnvFile $EnvFile
        if ($key) { $env:PORTAL_NUGET_KEY = $key }
    }
    $start = @{ Artifacts = $Artifacts; Filesystem = $Filesystem }
    if ($PSBoundParameters.ContainsKey('Port')) { $start.Port = $Port }
    $p = Start-Portal @start
    $p | Add-Member -NotePropertyName Replaced -NotePropertyValue $old -PassThru
}
