function Get-PortalBinaryPath {
    <#
    .SYNOPSIS
    Path of the portal or graphnode binary, build/win-x64/<name>.exe at the repo root.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param(
        [ValidateSet('portal', 'graphnode')]
        [string] $Name = 'portal'
    )
    if (-not $IsWindows) { throw 'Portal builds win-x64 binaries only.' }
    $path = Join-Path $script:RepoRoot 'build' 'win-x64' "$Name.exe"
    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
        throw "Portal binary not found: $path. Build it from the repo root with: Invoke-Build Build"
    }
    $path
}
