function Get-PortalTenant {
    <#
    .SYNOPSIS
    The tenant the module talks to: PORTAL_TENANT from the environment, else default. Every API path names it
    (/api/tenants/<tenant>/...), and Start-Portal passes it to the binary.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param()
    if ($env:PORTAL_TENANT) { return $env:PORTAL_TENANT }
    'default'
}
