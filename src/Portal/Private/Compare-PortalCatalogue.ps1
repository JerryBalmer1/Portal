function Compare-PortalCatalogue {
    <#
    .SYNOPSIS
    The Catalogue rows that regressed between two Refreshes (docs/design.md 56): a name that read ok in -Previous and
    reads no-layers or version-mismatch in -Current, as Name, Was and Now. A name new to -Current, or one that was
    not ok before, is no regression.
    #>
    [CmdletBinding()]
    param(
        [AllowNull()] [AllowEmptyCollection()] [object[]] $Previous,
        [AllowNull()] [AllowEmptyCollection()] [object[]] $Current
    )
    $was = @{}
    foreach ($r in @($Previous | Where-Object { $_ })) { $was[[string]$r.Name] = [string]$r.Status }
    foreach ($r in @($Current | Where-Object { $_ })) {
        $name = [string]$r.Name
        if ($was[$name] -eq 'ok' -and $r.Status -in 'no-layers', 'version-mismatch') {
            [pscustomobject]@{ Name = $name; Was = 'ok'; Now = [string]$r.Status }
        }
    }
}
