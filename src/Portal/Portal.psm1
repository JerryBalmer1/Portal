#Requires -Version 7.4
Set-StrictMode -Version Latest

$script:ModuleRoot    = $PSScriptRoot
$script:RepoRoot      = Split-Path (Split-Path $PSScriptRoot -Parent) -Parent
$script:ArtifactsRoot = Join-Path $script:RepoRoot 'artifacts'
$script:DefaultFilesystem = 'C:\__Code\Graph.Emitter.FileSystem\artifacts\layers'

foreach ($dir in 'Private', 'Public') {
    Get-ChildItem -Path (Join-Path $PSScriptRoot $dir) -Filter '*.ps1' -ErrorAction SilentlyContinue |
        ForEach-Object { . $_.FullName }
}

Export-ModuleMember -Function (
    Get-ChildItem -Path (Join-Path $PSScriptRoot 'Public') -Filter '*.ps1' |
        ForEach-Object { $_.BaseName }
)
