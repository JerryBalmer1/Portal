function Mount-PortalLayer {
    <#
    .SYNOPSIS
        POST /api/tenants/{tenant}/layers: mounts a data layer read-only, from a path (a .graph.json, or a directory whose newest
        *.graph.json is taken) or from an envelope. Returns the layer summary; throws with the Graph.Node problem
        codes when the envelope fails graphnode validate (the error's TargetObject holds them).
    .EXAMPLE
        Mount-PortalLayer -Name filesystem -Path C:\__Code\Graph.Emitter.FileSystem\artifacts\layers
    .EXAMPLE
        Mount-PortalLayer -Name runs -Envelope (Get-Content .\run-report.json -Raw)
    #>
    [CmdletBinding(DefaultParameterSetName = 'Path')]
    param(
        [Parameter(Mandatory, Position = 0)]
        [ValidatePattern('^[a-z0-9][a-z0-9._-]*$')]
        [string] $Name,

        [Parameter(Mandatory, ParameterSetName = 'Path', Position = 1)]
        [string] $Path,

        [Parameter(Mandatory, ParameterSetName = 'Envelope')]
        $Envelope,

        [string] $Url
    )
    if ($PSCmdlet.ParameterSetName -eq 'Path') {
        $full = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path))
        $body = @{ name = $Name; path = $full } | ConvertTo-Json -Compress
    }
    else {
        $envJson = if ($Envelope -is [string]) { $Envelope } else { $Envelope | ConvertTo-Json -Depth 64 -Compress }
        $body = '{"name":' + ($Name | ConvertTo-Json) + ',"envelope":' + $envJson + '}'
    }
    Invoke-PortalApi -Url (Resolve-PortalUrl $Url) -Path '/layers' -Method POST -Body $body
}
