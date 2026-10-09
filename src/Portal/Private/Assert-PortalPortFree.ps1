function Assert-PortalPortFree {
    <#
    .SYNOPSIS
    Throws when something already listens on -Port, naming the port and the owning process where Windows can
    read it (Get-NetTCPConnection); otherwise tries to bind 127.0.0.1:<Port> and throws if that fails. Never
    picks another port. Lifted from Graph's Assert-GraphPortFree.
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)] [ValidateRange(1, 65535)] [int] $Port)
    $hint = 'free it, or set PORTAL_PORT in .env or pass -Port'
    if ($IsWindows -and (Get-Command Get-NetTCPConnection -ErrorAction SilentlyContinue)) {
        $owner = Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue |
            Select-Object -First 1 -ExpandProperty OwningProcess
        if ($owner) {
            $process = Get-Process -Id $owner -ErrorAction SilentlyContinue
            $name = if ($process) { $process.ProcessName } else { 'unknown' }
            throw "port $Port is in use by $name (process $owner); $hint"
        }
    }
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, $Port)
    try { $listener.Start() }
    catch { throw "port $Port is in use (the owning process could not be read); $hint" }
    finally { $listener.Stop() }
}
