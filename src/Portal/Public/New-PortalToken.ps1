function New-PortalToken {
    <#
    .SYNOPSIS
        Issues a bearer token for an existing principal of a tenant: 32 random bytes as hex. Adds its SHA-256, the
        principal's id and when it was made to <Tenants>/<Tenant>/tokens.yaml (created when absent; gitignored) and
        returns Name, Principal, Role, Token and Path. The token is shown this once and never stored; a portal reads
        tokens.yaml at start (and again on an admin's Add or Revoke), so restart it to use the token.
    .DESCRIPTION
        -Name names a principal in principals.yaml (gitignored beside it); with none of that name it throws. A
        principal may hold any number of tokens: mint a new one, then revoke the old on Settings/Tokens.
        -Bootstrap is for a tenant with no principals yet: it creates principal -Name as the first admin, createdBy
        null, and issues its token. It works once; with any principal already in principals.yaml it throws. Other
        principals are added by an admin on Identity/Users (docs/design.md 38).
    .EXAMPLE
        $t = New-PortalToken -Name jerry -Bootstrap; $env:PORTAL_TOKEN = $t.Token
    .EXAMPLE
        New-PortalToken -Name ci
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory, Position = 0)]
        [ValidatePattern('^[a-z0-9][a-z0-9._-]{0,63}$')]
        [string] $Name,
        [switch] $Bootstrap,
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')] [string] $Tenant = (Get-PortalTenant),
        [string] $Tenants = $(if ($env:PORTAL_TENANTS) { $env:PORTAL_TENANTS } else { Join-Path $script:RepoRoot 'tenants' })
    )
    $Tenants = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Tenants))
    $dir = Join-Path $Tenants $Tenant
    if (-not (Test-Path -LiteralPath (Join-Path $dir 'portal.yaml'))) {
        throw "No tenant $Tenant under ${Tenants}: $(Join-Path $dir 'portal.yaml') is missing."
    }
    $principalsFile = Join-Path $dir 'principals.yaml'
    $principals = Read-PortalYamlList -Path $principalsFile -Key principals
    $now = [datetime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ss.fffZ', [Globalization.CultureInfo]::InvariantCulture)
    $id = "ui:principal:$Name"
    if ($Bootstrap) {
        if ($principals.Count -gt 0) {
            throw "-Bootstrap makes a tenant's first admin, once; $principalsFile already holds $($principals.Count) principal(s) ($(@($principals | ForEach-Object { $_.name }) -join ', ')). Issue a token for one with New-PortalToken -Name <principal>."
        }
        $text = "# Principals of tenant ${Tenant}: who may hold a token, and with what role (docs/design.md 38).`n" +
            "principals:`n  - id: '$id'`n    name: '$Name'`n    type: user`n    role: admin`n    created: '$now'`n    createdBy: null`n"
        Set-Content -LiteralPath $principalsFile -Value $text -NoNewline -Encoding utf8NoBOM
        $role = 'admin'
    }
    else {
        $p = $principals | Where-Object { $_.name -ceq $Name } | Select-Object -First 1
        if (-not $p) {
            $hint = if ($principals.Count -eq 0) { 'The tenant has no principals yet: make the first admin with -Bootstrap.' } else { 'An admin adds one on Identity/Users.' }
            throw "No principal named $Name in $principalsFile. $hint"
        }
        $role = $p.role
    }
    $file = Join-Path $dir 'tokens.yaml'
    $text = if (Test-Path -LiteralPath $file) { Get-Content -LiteralPath $file -Raw } else { '' }
    $bytes = [byte[]]::new(32)
    [Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    $token = [Convert]::ToHexString($bytes).ToLowerInvariant()
    $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData([Text.Encoding]::UTF8.GetBytes($token))).ToLowerInvariant()
    if (-not $text.Trim()) {
        $text = "# Bearer tokens of tenant $Tenant, as the SHA-256 of each token (New-PortalToken).`ntokens:`n"
    }
    else {
        # The portal writes an emptied list as "tokens: []"; an item goes under a block list.
        $text = $text -replace '(?m)^tokens:\s*\[\]\s*$', 'tokens:'
        if (-not $text.EndsWith("`n")) { $text += "`n" }
    }
    $text += "  - principal: '$id'`n    sha256: $hash`n    created: '$now'`n"
    Set-Content -LiteralPath $file -Value $text -NoNewline -Encoding utf8NoBOM
    [pscustomobject]@{ PSTypeName = 'Portal.Token'; Name = $Name; Principal = $id; Role = $role; Token = $token; Path = $file }
}
