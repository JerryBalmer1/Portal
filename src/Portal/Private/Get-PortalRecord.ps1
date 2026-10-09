function Get-PortalRecord {
    <#
    .SYNOPSIS
    The running portal recorded in <Artifacts>/portal.json by Start-Portal, or $null.
    #>
    [CmdletBinding()]
    param([string] $Artifacts = $script:ArtifactsRoot)
    $file = Join-Path $Artifacts 'portal.json'
    if (-not (Test-Path -LiteralPath $file)) { return $null }
    Get-Content -LiteralPath $file -Raw | ConvertFrom-Json
}
