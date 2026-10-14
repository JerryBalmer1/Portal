function Get-PortalDevToken {
    <#
    .SYNOPSIS
    The dev token the portal at -Url serves now, from GET /api/tenants/<tenant>/dev-token (loopback, mode dev;
    docs/design.md 39). Refresh calls it immediately before each API call it makes (docs/design.md 59): every portal
    start mints a new one, so a token read earlier may be dead.
    .DESCRIPTION
    Sends no Authorization header: the route is anonymous, and a stale PORTAL_TOKEN would be refused with 401 there
    too. A 404 means the portal serves no dev token (started with Invoke-Build Up, or the tenant is not in mode dev,
    or its dev-bootstrap token was replaced since it started), and throws saying so. Any other failure throws with
    the status. Returns the token.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)] [string] $Url,
        [string] $Tenant = (Get-PortalTenant)
    )
    $uri = "$($Url.TrimEnd('/'))/api/tenants/$Tenant/dev-token"
    $response = try { Invoke-WebRequest -Uri $uri -SkipHttpErrorCheck -TimeoutSec 10 }
    catch { throw "GET $uri failed: $($_.Exception.Message); is the portal up?" }
    $status = [int]$response.StatusCode
    if ($status -eq 404) {
        throw "GET $uri answered 404: the portal serves no dev token. It was started with Invoke-Build Up (or tenant $Tenant is not in mode dev), and Refresh needs the portal Invoke-Build Dev starts"
    }
    if ($status -ne 200) { throw "GET $uri answered $status`: $($response.Content)" }
    $token = ([string]$response.Content | ConvertFrom-Json).token
    if (-not $token) { throw "GET $uri answered 200 with no token" }
    $token
}
