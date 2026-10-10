function Compare-PortalSnap {
    <#
    .SYNOPSIS
        GET /api/tenants/{tenant}/diff: what snap B added, removed and changed against snap A (nodes and edges), and the read-only
        Graph.Node design 11 merge of the two. Takes stamps or the objects New-PortalSnap returns.
    .EXAMPLE
        $a = New-PortalSnap; <change something>; $b = New-PortalSnap
        (Compare-PortalSnap $a $b).changed.nodes.id
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory, Position = 0)] $A,
        [Parameter(Mandatory, Position = 1)] $B,
        [string] $Url
    )
    $stamp = { param($s) if ($s -is [string]) { $s } else { [string]$s.stamp } }
    $query = 'a={0}&b={1}' -f [uri]::EscapeDataString((& $stamp $A)), [uri]::EscapeDataString((& $stamp $B))
    Invoke-PortalApi -Url (Resolve-PortalUrl $Url) -Path "/diff?$query"
}
