function Stop-Portal {
    <#
    .SYNOPSIS
        Stops the portal recorded in <Artifacts>/portal.json, waits for it to exit and removes the record.
        Returns nothing; says so with -Verbose when no portal is recorded.
    .EXAMPLE
        Stop-Portal
    #>
    [CmdletBinding()]
    param([string] $Artifacts = $script:ArtifactsRoot)
    $Artifacts = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Artifacts))
    $record = Get-PortalRecord -Artifacts $Artifacts
    if (-not $record) {
        Write-Verbose "No portal is recorded in $Artifacts."
        return
    }
    Stop-Process -Id $record.ProcessId -Force -ErrorAction SilentlyContinue
    Wait-Process -Id $record.ProcessId -Timeout 10 -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath (Join-Path $Artifacts 'portal.json') -ErrorAction SilentlyContinue
}
