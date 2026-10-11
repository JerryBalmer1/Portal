function New-PortalDevToken {
    <#
    .SYNOPSIS
    Invoke-Build Dev's mint (docs/design.md 39): makes principal dev-bootstrap (user, admin, createdBy null) in
    <Tenants>/<Tenant>/principals.yaml when it is not there, removes every token of dev-bootstrap from tokens.yaml, and
    adds one new token for it. Only the two gitignored identity files are written. Returns Name, Principal, Role, Token,
    Replaced (how many old dev-bootstrap tokens were removed) and Path; the token is shown this once. Throws unless the
    tenant's mode is dev, so it never mints for a demo or prod tenant.
    #>
    [CmdletBinding()]
    param(
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')] [string] $Tenant = (Get-PortalTenant),
        [string] $Tenants = $(if ($env:PORTAL_TENANTS) { $env:PORTAL_TENANTS } else { Join-Path $script:RepoRoot 'tenants' })
    )
    $Tenants = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Tenants))
    $mode = Get-PortalTenantMode -Tenant $Tenant -Tenants $Tenants
    if ($mode -ne 'dev') { throw "Tenant $Tenant is in mode $mode, not dev: dev-bootstrap is minted only for a tenant whose portal.yaml says mode: dev." }
    $dir = Join-Path $Tenants $Tenant
    $name = 'dev-bootstrap'
    $id = "ui:principal:$name"
    $now = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ', [Globalization.CultureInfo]::InvariantCulture)
    $q = { param([string] $s) "'" + $s.Replace("'", "''") + "'" }

    $principalsFile = Join-Path $dir 'principals.yaml'
    $principals = Read-PortalYamlList -Path $principalsFile -Key principals
    if (-not ($principals | Where-Object { $_.name -ceq $name })) {
        $entry = "  - id: '$id'`n    name: '$name'`n    type: user`n    role: admin`n    created: '$now'`n    createdBy: null`n"
        $text = if (Test-Path -LiteralPath $principalsFile) { Get-Content -LiteralPath $principalsFile -Raw } else { '' }
        if (-not $text.Trim() -or $text -notmatch '(?m)^principals:') {
            $text = "# Principals of tenant ${Tenant}: who may hold a token, and with what role (docs/design.md 38).`nprincipals:`n"
        }
        else {
            $text = $text -replace '(?m)^principals:\s*\[\]\s*$', 'principals:'
            if (-not $text.EndsWith("`n")) { $text += "`n" }
        }
        Set-Content -LiteralPath $principalsFile -Value ($text + $entry) -NoNewline -Encoding utf8NoBOM
    }

    # tokens.yaml is written whole in the binary's block shape: its leading comment, then every token but
    # dev-bootstrap's, then the new one.
    $tokensFile = Join-Path $dir 'tokens.yaml'
    $tokens = Read-PortalYamlList -Path $tokensFile -Key tokens
    $keep = @($tokens | Where-Object { $_['principal'] -cne $id })
    $header = if (Test-Path -LiteralPath $tokensFile) {
        (@(Get-Content -LiteralPath $tokensFile) | ForEach-Object -Begin { $in = $true } -Process { if ($in -and $_ -match '^\s*#') { $_ } else { $in = $false } }) -join "`n"
    }
    if (-not $header) { $header = "# Bearer tokens of tenant $Tenant, as the SHA-256 of each token (New-PortalToken)." }
    $bytes = [byte[]]::new(32)
    [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $token = [Convert]::ToHexString($bytes).ToLowerInvariant()
    $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($token))).ToLowerInvariant()
    $b = [Text.StringBuilder]::new()
    $null = $b.Append("$header`ntokens:`n")
    foreach ($k in $keep) {
        if ($k['principal']) {
            $null = $b.Append("  - principal: $(& $q $k['principal'])`n    sha256: $($k['sha256'])`n")
            if ($k['created']) { $null = $b.Append("    created: $(& $q $k['created'])`n") }
        }
        else { $null = $b.Append("  - name: $(& $q $k['name'])`n    sha256: $($k['sha256'])`n") }
    }
    $null = $b.Append("  - principal: '$id'`n    sha256: $hash`n    created: '$now'`n")
    Set-Content -LiteralPath $tokensFile -Value $b.ToString() -NoNewline -Encoding utf8NoBOM
    [pscustomobject]@{ PSTypeName = 'Portal.Token'; Name = $name; Principal = $id; Role = 'admin'; Token = $token; Replaced = $tokens.Count - $keep.Count; Path = $tokensFile }
}
