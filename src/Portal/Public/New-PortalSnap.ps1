function New-PortalSnap {
    <#
    .SYNOPSIS
        POST /api/tenants/{tenant}/snap: approves the screen. Writes it to artifacts/snaps/<stamp>.graph.json with -Note as its
        approved finding, and returns stamp, layer, path, counts, note and, after the first snap, previous (the
        snap before) and diff (ids added, removed and changed against it). The next start boots from it.
    .EXAMPLE
        $a = New-PortalSnap -Note 'first layout'; $a.stamp
    .EXAMPLE
        (New-PortalSnap -Note 'lighter glass').diff.changed.nodes      # ui:theme:default
    #>
    [CmdletBinding()]
    param(
        [ValidateLength(0, 2000)] [string] $Note,
        [string] $Url
    )
    $call = @{ Url = (Resolve-PortalUrl $Url); Path = '/snap'; Method = 'POST' }
    if ($PSBoundParameters.ContainsKey('Note')) { $call.Body = @{ note = $Note } }
    Invoke-PortalApi @call
}
