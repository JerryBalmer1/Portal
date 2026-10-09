function Invoke-PortalApi {
    <#
    .SYNOPSIS
    One call to the portal API. Returns the JSON body as objects with stamps left as strings; throws on a non-2xx
    status with the spec Error (code, message, problems) as the error's TargetObject.
    #>
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string] $Url,
        [Parameter(Mandatory)] [string] $Path,
        [ValidateSet('GET', 'POST', 'PATCH', 'DELETE')] [string] $Method = 'GET',
        $Body
    )
    $request = @{ Uri = "$Url$Path"; Method = $Method; SkipHttpErrorCheck = $true; TimeoutSec = 30 }
    if ($PSBoundParameters.ContainsKey('Body')) {
        $request.ContentType = 'application/json'
        $request.Body = if ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json -Depth 64 -Compress }
    }
    $response = Invoke-WebRequest @request
    # Stamps stay strings: ConvertFrom-Json would turn them into [datetime] (PowerShell 7.5+ can say not to).
    $convert = @{ Depth = 64 }
    if ((Get-Command ConvertFrom-Json).Parameters.ContainsKey('DateKind')) { $convert.DateKind = 'String' }
    $text = [string]$response.Content
    $data = if ($text) { $text | ConvertFrom-Json @convert } else { $null }
    if ([int]$response.StatusCode -ge 400) {
        $message = if ($data -and $data.PSObject.Properties['message']) { $data.message } else { $text }
        $lines = @(if ($data -and $data.PSObject.Properties['problems']) { $data.problems | ForEach-Object { "$($_.code) $($_.path): $($_.message)" } })
        $err = [System.Management.Automation.ErrorRecord]::new(
            [System.InvalidOperationException]::new((@("$Method $Path -> $([int]$response.StatusCode): $message") + $lines) -join [Environment]::NewLine),
            "Portal.$([int]$response.StatusCode)", [System.Management.Automation.ErrorCategory]::InvalidResult, $data)
        $PSCmdlet.ThrowTerminatingError($err)
    }
    $data
}
