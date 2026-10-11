function Publish-PortalCard {
    <#
    .SYNOPSIS
        POST /api/tenants/{tenant}/feeds/{feed}/push: publishes one Graph.Node manifest card (a folder holding
        manifest.yaml) to one of the tenant's feeds. Sends manifest.yaml as the manifest part and every other file
        under the folder as a file part named by its path relative to the folder, forward slashes (dot files and
        directories skipped). Needs PORTAL_TOKEN of a contributor or admin. Returns the ledger entry the push wrote
        (docs/design.md 40); throws with the portal's status and message otherwise: 409 when the feed already holds
        that name and version, 422 with Graph.Node's problems for a card that fails the manifest schema.
    .EXAMPLE
        Publish-PortalCard -Feed cards-oci -Path C:\__Code\Portal.Tools\tools\Git
    #>
    [CmdletBinding(SupportsShouldProcess)]
    param(
        [Parameter(Mandatory, Position = 0)]
        [ValidatePattern('^[a-z0-9][a-z0-9-]*$')]
        [string] $Feed,

        [Parameter(Mandatory, Position = 1)]
        [string] $Path,

        [string] $Url
    )
    $dir = [IO.Path]::GetFullPath($ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($Path))
    $manifest = Join-Path $dir 'manifest.yaml'
    if (-not (Test-Path -LiteralPath $manifest -PathType Leaf)) { throw "No manifest.yaml in ${dir}: a card is a folder holding its manifest.yaml." }
    $files = @(Get-ChildItem -LiteralPath $dir -File -Recurse | Where-Object {
            $rel = [IO.Path]::GetRelativePath($dir, $_.FullName)
            $rel -ne 'manifest.yaml' -and -not ($rel -split '[\\/]' | Where-Object { $_.StartsWith('.') })
        } | Sort-Object FullName)
    if (-not $PSCmdlet.ShouldProcess("$Feed", "push the card in $dir ($($files.Count + 1) files)")) { return }
    $form = [System.Net.Http.MultipartFormDataContent]::new()
    try {
        $add = {
            param([string] $Name, [string] $File, [string] $As)
            $part = [System.Net.Http.ByteArrayContent]::new([IO.File]::ReadAllBytes($File))
            $part.Headers.ContentType = [System.Net.Http.Headers.MediaTypeHeaderValue]::new('application/octet-stream')
            $cd = [System.Net.Http.Headers.ContentDispositionHeaderValue]::new('form-data')
            $cd.Name = '"' + $Name + '"'
            $cd.FileName = '"' + $As + '"'
            $part.Headers.ContentDisposition = $cd
            $form.Add($part)
        }
        & $add 'manifest' $manifest 'manifest.yaml'
        foreach ($f in $files) {
            & $add 'file' $f.FullName ([IO.Path]::GetRelativePath($dir, $f.FullName) -replace '\\', '/')
        }
        Invoke-PortalApi -Url (Resolve-PortalUrl $Url) -Path "/feeds/$Feed/push" -Method POST -Form $form
    }
    finally { $form.Dispose() }
}
