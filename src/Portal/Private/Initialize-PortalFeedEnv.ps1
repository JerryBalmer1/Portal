function Initialize-PortalFeedEnv {
    <#
    .SYNOPSIS
    The environment compose.yaml and the portal need for the stock feeds (docs/design.md 40), set in this process:
    PORTAL_TENANT, PORTAL_FEEDS_DATA (<Tenants>/<Tenant>/data, made when absent), and PORTAL_NUGET_KEY and
    PORTAL_POSTGRES_PASSWORD, each from the process environment, else from the repo's .env, else 32 random bytes in hex
    appended to .env (gitignored) so the next run and compose agree. Returns which of the secrets were written.
    #>
    [CmdletBinding()]
    param(
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')] [string] $Tenant = (Get-PortalTenant),
        [string] $Tenants = $(if ($env:PORTAL_TENANTS) { $env:PORTAL_TENANTS } else { Join-Path $script:RepoRoot 'tenants' }),
        [string] $EnvFile = (Join-Path $script:RepoRoot '.env')
    )
    $data = Join-Path ([IO.Path]::GetFullPath($Tenants)) $Tenant 'data'
    $null = New-Item -ItemType Directory -Force -Path $data
    $env:PORTAL_TENANT = $Tenant
    $env:PORTAL_FEEDS_DATA = $data
    $written = @()
    foreach ($name in 'PORTAL_NUGET_KEY', 'PORTAL_POSTGRES_PASSWORD') {
        if ([Environment]::GetEnvironmentVariable($name)) { continue }
        $value = Get-PortalEnvValue -Name $name -EnvFile $EnvFile
        if (-not $value) {
            $value = [Convert]::ToHexString([Security.Cryptography.RandomNumberGenerator]::GetBytes(32)).ToLowerInvariant()
            $text = if (Test-Path -LiteralPath $EnvFile -PathType Leaf) { [IO.File]::ReadAllText($EnvFile) } else { '' }
            $lead = if ($text -and -not $text.EndsWith("`n")) { [Environment]::NewLine } else { '' }
            [IO.File]::AppendAllText($EnvFile, "$lead$name=$value$([Environment]::NewLine)")
            $written += $name
        }
        [Environment]::SetEnvironmentVariable($name, $value)
    }
    [pscustomobject]@{ Tenant = $Tenant; Data = $data; Written = $written }
}
