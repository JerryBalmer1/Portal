function Resolve-PortalUrl {
    <#
    .SYNOPSIS
    The given -Url, else the URL of the portal recorded under the default artifacts directory.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param([string] $Url)
    if ($Url) { return $Url.TrimEnd('/') }
    $record = Get-PortalRecord
    if (-not $record) { throw 'No portal is recorded in artifacts/portal.json. Start one with Start-Portal, or pass -Url.' }
    $record.Url
}
