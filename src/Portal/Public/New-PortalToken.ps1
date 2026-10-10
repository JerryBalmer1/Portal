function New-PortalToken {
    <#
    .SYNOPSIS
        Makes a bearer token that may write to a tenant: 32 random bytes as hex. Adds its SHA-256 under -Name to
        <Tenants>/<Tenant>/tokens.yaml (created when absent; gitignored) and returns Name, Token and Path. The token
        is shown this once and never stored; a portal reads tokens.yaml at start, so restart it to use the token.
    .DESCRIPTION
        Give the token to a client as PORTAL_TOKEN (this module's calls send it), or paste it into the page's avatar
        menu. A name already in the file is refused: remove its entry to replace it.
    .EXAMPLE
        $t = New-PortalToken -Name jerry; $env:PORTAL_TOKEN = $t.Token
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory, Position = 0)]
        [ValidatePattern('^[A-Za-z0-9][A-Za-z0-9 ._-]*$')]
        [string] $Name,
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')] [string] $Tenant = (Get-PortalTenant),
        [string] $Tenants = $(if ($env:PORTAL_TENANTS) { $env:PORTAL_TENANTS } else { Join-Path $script:RepoRoot 'tenants' })
    )
    $Tenants = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Tenants))
    $dir = Join-Path $Tenants $Tenant
    if (-not (Test-Path -LiteralPath (Join-Path $dir 'portal.yaml'))) {
        throw "No tenant $Tenant under ${Tenants}: $(Join-Path $dir 'portal.yaml') is missing."
    }
    $file = Join-Path $dir 'tokens.yaml'
    $text = if (Test-Path -LiteralPath $file) { Get-Content -LiteralPath $file -Raw } else { '' }
    $quoted = "'" + $Name.Replace("'", "''") + "'"
    if ($text -match "(?m)^\s*-\s*name:\s*($([regex]::Escape($Name))|$([regex]::Escape($quoted)))\s*$") {
        throw "tokens.yaml already holds a token named $Name ($file). Remove its entry to replace it."
    }
    $bytes = [byte[]]::new(32)
    [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $token = [Convert]::ToHexString($bytes).ToLowerInvariant()
    $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($token))).ToLowerInvariant()
    if (-not $text.Trim()) {
        $text = "# Bearer tokens that may write to tenant $Tenant, as the SHA-256 of each token (New-PortalToken).`ntokens:`n"
    }
    elseif (-not $text.EndsWith("`n")) { $text += "`n" }
    $text += "  - name: $quoted`n    sha256: $hash`n"
    Set-Content -LiteralPath $file -Value $text -NoNewline -Encoding utf8NoBOM
    [pscustomobject]@{ PSTypeName = 'Portal.Token'; Name = $Name; Token = $token; Path = $file }
}
