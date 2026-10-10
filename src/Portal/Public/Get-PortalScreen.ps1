function Get-PortalScreen {
    <#
    .SYNOPSIS
        GET /api/tenants/{tenant}/screen: the screen graph, a Graph.Node envelope (module Portal), with stamps as strings.
    .EXAMPLE
        (Get-PortalScreen).nodes | Format-Table id, kind, name
    #>
    [CmdletBinding()]
    param([string] $Url)
    Invoke-PortalApi -Url (Resolve-PortalUrl $Url) -Path '/screen'
}
