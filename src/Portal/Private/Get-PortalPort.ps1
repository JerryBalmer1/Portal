function Get-PortalPort {
    <#
    .SYNOPSIS
    The portal's fixed port: -Port when given, else PORTAL_PORT from the repo's .env, else the PORTAL_PORT
    environment variable, else 8200 (docs/design.md 24). Throws on 0 or anything that is not a port number.
    #>
    [CmdletBinding()]
    [OutputType([int])]
    param(
        [object] $Port,
        [string] $EnvFile = (Join-Path $script:RepoRoot '.env')
    )
    $value = $null
    $from = $null
    if ($null -ne $Port -and "$Port" -ne '') {
        $value = "$Port"
        $from = '-Port'
    }
    if (-not $from -and (Test-Path -LiteralPath $EnvFile -PathType Leaf)) {
        foreach ($line in Get-Content -LiteralPath $EnvFile) {
            if ($line -match '^\s*(?:export\s+)?PORTAL_PORT\s*=\s*(.*?)\s*$') {
                $value = $Matches[1].Trim('"', "'")
                $from = $EnvFile
            }
        }
    }
    if (-not $from -and $env:PORTAL_PORT) {
        $value = $env:PORTAL_PORT
        $from = 'the PORTAL_PORT environment variable'
    }
    if (-not $from) { return 8200 }
    $n = 0
    if (-not [int]::TryParse($value, [ref]$n) -or $n -lt 1 -or $n -gt 65535) {
        throw "port '$value' from $from is refused: the portal serves on one fixed port, 1-65535 (default 8200); 0 is never picked for you"
    }
    $n
}
