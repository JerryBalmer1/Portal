function Get-PortalProcess {
    <#
    .SYNOPSIS
    The live processes that are this repo's portal (docs/design.md 57): image build/win-x64/portal.exe at the repo
    root, or portal.exe~ after go build renamed it in place, matched on the full path. With -Artifacts, only those
    whose -artifacts argument is that directory. Each result has ProcessId, Port (its -port, else $null), Artifacts
    (its -artifacts, full path) and Path. Recorded or not makes no difference here; Stop-Portal and Get-PortalStatus
    compare with the record. -Process replaces the Win32_Process query (Pester): objects with ProcessId,
    ExecutablePath and CommandLine.
    #>
    [CmdletBinding()]
    param(
        [string] $Artifacts,
        [object[]] $Process
    )
    if (-not $PSBoundParameters.ContainsKey('Process')) {
        if (-not $IsWindows) { return }
        $Process = @(Get-CimInstance -ClassName Win32_Process -Filter "Name = 'portal.exe' OR Name = 'portal.exe~'" -ErrorAction SilentlyContinue -Verbose:$false)
    }
    $exe = [IO.Path]::GetFullPath((Join-Path $script:RepoRoot 'build' 'win-x64' 'portal.exe'))
    $want = if ($Artifacts) { ConvertTo-PortalDirKey $Artifacts }
    foreach ($p in $Process) {
        $path = [string]$p.ExecutablePath
        if (-not $path) { continue }
        $path = [IO.Path]::GetFullPath($path)
        if ($path -ne $exe -and $path -ne "$exe~") { continue }
        $flags = ConvertFrom-PortalCommandLine -CommandLine ([string]$p.CommandLine)
        if (-not $flags['artifacts']) { continue }
        $dir = ConvertTo-PortalDirKey $flags['artifacts']
        if ($want -and $dir -ne $want) { continue }
        $port = 0
        [pscustomobject]@{
            ProcessId = [int]$p.ProcessId
            Port      = if ([int]::TryParse([string]$flags['port'], [ref]$port)) { $port } else { $null }
            Artifacts = $dir
            Path      = $path
        }
    }
}

