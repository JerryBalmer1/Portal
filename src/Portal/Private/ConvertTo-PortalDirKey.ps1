function ConvertTo-PortalDirKey {
    <#
    .SYNOPSIS
    A directory as one comparable string: full path, no trailing separator. Windows paths compare
    case-insensitively, as PowerShell's -eq does.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param([Parameter(Mandatory)] [string] $Path)
    $full = [IO.Path]::GetFullPath($Path)
    $root = [IO.Path]::GetPathRoot($full)
    if ($full.Length -gt $root.Length) { $full = $full.TrimEnd('\', '/') }
    $full
}
