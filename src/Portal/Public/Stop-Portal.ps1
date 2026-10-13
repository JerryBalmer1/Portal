function Stop-Portal {
    <#
    .SYNOPSIS
        Stops every portal serving <Artifacts>, the one recorded in portal.json and any this repo's binary runs over
        the same directory unrecorded, waits for each to exit and removes the record.
    .DESCRIPTION
        A portal is this repo's build/win-x64/portal.exe (or portal.exe~, as go build renames a binary in use) whose
        -artifacts is <Artifacts> (docs/design.md 57). A recorded process id that is not such a portal, exited or
        reused by another program, is never stopped; its record is removed. When a portal is still running 10 s
        after Stop-Process, the record is kept and Stop-Portal throws naming it. Returns nothing; -Verbose says what
        was stopped, or that nothing was.
    .EXAMPLE
        Stop-Portal -Verbose
    #>
    [CmdletBinding()]
    param([string] $Artifacts = $script:ArtifactsRoot)
    $Artifacts = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Artifacts))
    $file = Join-Path $Artifacts 'portal.json'
    $record = Get-PortalRecord -Artifacts $Artifacts
    $running = @(Get-PortalProcess -Artifacts $Artifacts)
    $ids = @($running | ForEach-Object { $_.ProcessId })
    if (-not $record -and -not $running) {
        Write-Verbose "No portal is recorded in $Artifacts, and none is running over it."
        return
    }
    if ($record -and $record.ProcessId -notin $ids) {
        Write-Verbose "The record names process $($record.ProcessId), which is not a portal serving $Artifacts (exited, or the id was reused); it is not stopped."
    }
    foreach ($p in $running) {
        $how = if ($record -and $p.ProcessId -eq $record.ProcessId) { 'recorded' } else { 'unrecorded' }
        Write-Verbose "Stopping the $how portal $($p.ProcessId) ($($p.Path), port $($p.Port)) serving $Artifacts."
        Stop-Process -Id $p.ProcessId -Force -ErrorAction SilentlyContinue
    }
    foreach ($p in $running) { Wait-Process -Id $p.ProcessId -Timeout 10 -ErrorAction SilentlyContinue }
    $left = @($running | Where-Object { Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue })
    if ($left) {
        $named = ($left | ForEach-Object { "process $($_.ProcessId) ($($_.Path))" }) -join ', '
        throw "Stop-Portal: $named still serves $Artifacts 10 s after Stop-Process; $(if ($record) { "the record $file is kept" } else { 'it was unrecorded' }). Stop it by hand (Stop-Process -Id <id> -Force) and run Stop-Portal again."
    }
    Remove-Item -LiteralPath $file -ErrorAction SilentlyContinue
}
