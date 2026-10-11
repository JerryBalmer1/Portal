function Get-PortalEnvValue {
    <#
    .SYNOPSIS
    One variable as the portal's tools see it: the process environment, else the last `NAME=value` line (an optional
    `export`, quotes trimmed) of the repo's .env, else $null. Reads only; never writes .env. Initialize-PortalFeedEnv
    (Invoke-Build Feeds) and Invoke-PortalUp (Invoke-Build Up) both read PORTAL_NUGET_KEY through it, so a portal
    started either way holds the key Feeds wrote (docs/design.md 40).
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [Parameter(Mandatory)] [ValidatePattern('^[A-Za-z_][A-Za-z0-9_]*$')] [string] $Name,
        [string] $EnvFile = (Join-Path $script:RepoRoot '.env')
    )
    $value = [Environment]::GetEnvironmentVariable($Name)
    if ($value) { return $value }
    $value = $null
    if (Test-Path -LiteralPath $EnvFile -PathType Leaf) {
        foreach ($line in Get-Content -LiteralPath $EnvFile) {
            if ($line -match "^\s*(?:export\s+)?$Name\s*=\s*(.*?)\s*$") { $value = $Matches[1].Trim('"', "'") }
        }
    }
    if ($value) { $value } else { $null }
}
