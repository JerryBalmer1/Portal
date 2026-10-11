function Get-PortalTenantMode {
    <#
    .SYNOPSIS
    A tenant's mode from <Tenants>/<Tenant>/portal.yaml: dev, demo or prod, prod when the file names none
    (docs/design.md 39). Reads a top-level block line "mode: <value>" (quoted or not, a trailing comment allowed); a
    mode written any other way (inside a flow mapping) reads as prod, so Invoke-Build Dev refuses rather than mints.
    The binary reads the same key with a YAML decoder and refuses an unknown value at boot. Throws when the tenant has
    no portal.yaml.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')] [string] $Tenant = (Get-PortalTenant),
        [string] $Tenants = $(if ($env:PORTAL_TENANTS) { $env:PORTAL_TENANTS } else { Join-Path $script:RepoRoot 'tenants' })
    )
    $file = Join-Path $Tenants $Tenant 'portal.yaml'
    if (-not (Test-Path -LiteralPath $file)) { throw "No tenant $Tenant under ${Tenants}: $file is missing." }
    $m = [regex]::Match((Get-Content -LiteralPath $file -Raw), '(?m)^mode:[ \t]*([''"]?)([A-Za-z]+)\1[ \t]*(#.*)?\r?$')
    if ($m.Success) { $m.Groups[2].Value } else { 'prod' }
}
