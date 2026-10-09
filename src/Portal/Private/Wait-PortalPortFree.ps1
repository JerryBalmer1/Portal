function Wait-PortalPortFree {
    <#
    .SYNOPSIS
    Returns once nothing listens on -Port (Assert-PortalPortFree passes), polling every 200 ms; throws when the port
    is still held after -TimeoutSec, with Assert-PortalPortFree's message naming the owner.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [ValidateRange(1, 65535)] [int] $Port,
        [ValidateRange(1, 300)] [int] $TimeoutSec = 10
    )
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    while ($true) {
        try { Assert-PortalPortFree -Port $Port; return }
        catch {
            if ([DateTime]::UtcNow -ge $deadline) { throw "port $Port is still held $TimeoutSec s after Stop-Portal: $_" }
            Start-Sleep -Milliseconds 200
        }
    }
}
