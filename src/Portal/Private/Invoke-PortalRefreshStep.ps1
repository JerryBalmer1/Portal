function Invoke-PortalRefreshStep {
    <#
    .SYNOPSIS
    One invoke step of Refresh (docs/design.md 56): `Invoke-Build <Task> -File <File>` in a child pwsh, its output and
    errors in -Log. Returns Ok, Line, and After (the log's last 20 lines on a failure).
    .DESCRIPTION
    Output goes to files, not a pipe, and the wait is on the child alone (WaitForExit, not Start-Process -Wait, which
    waits for descendants): a portal the step starts inherits the child's handles and outlives it.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string] $Task,
        [Parameter(Mandatory)] [string] $File,
        [Parameter(Mandatory)] [string] $Log
    )
    $quoted = $File -replace "'", "''"
    $argv = "-NoProfile -Command `"Invoke-Build $Task -File '$quoted'`""
    $p = Start-Process -FilePath 'pwsh' -ArgumentList $argv -NoNewWindow -PassThru -RedirectStandardOutput $Log -RedirectStandardError "$Log.err"
    $null = $p.Handle
    $p.WaitForExit()
    $err = Get-Content -LiteralPath "$Log.err" -Raw -ErrorAction SilentlyContinue
    if ($err) { Add-Content -LiteralPath $Log -Value $err }
    Remove-Item -LiteralPath "$Log.err" -ErrorAction SilentlyContinue
    if ($p.ExitCode -eq 0) { return [pscustomobject]@{ Ok = $true; Line = "ok; log $Log"; After = $null } }
    $tail = (Get-Content -LiteralPath $Log -Tail 20 -ErrorAction SilentlyContinue) -join [Environment]::NewLine
    [pscustomobject]@{ Ok = $false; Line = "FAILED (exit $($p.ExitCode)); log $Log"; After = $tail }
}
