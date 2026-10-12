#Requires -Version 7.4
<#
.SYNOPSIS
    Invoke-Build Refresh in this Portal checkout, from any directory: every Portal repo down and back up in the order
    docs/refresh/order.psd1 declares, an approved snap, then the Catalogue checked against the last Refresh.
.DESCRIPTION
    The same task as `Invoke-Build Refresh` run in the repo root (docs/design.md 56); the build file is found from this
    script's own path, so the current directory does not matter. Exits non-zero when a step fails or a card regressed.
.EXAMPLE
    pwsh -NoProfile -File C:\__Code\Portal\scripts\Refresh-Portal.ps1
.EXAMPLE
    pwsh -NoProfile -File C:\__Code\Portal\scripts\Refresh-Portal.ps1 -DryRun
#>
param(
    # Print what each step would do and run nothing.
    [switch] $DryRun
)
$ErrorActionPreference = 'Stop'
if (-not (Get-Command Invoke-Build -ErrorAction SilentlyContinue)) {
    throw 'Invoke-Build is not installed: Install-Module InvokeBuild -MinimumVersion 5.11.0 -Scope CurrentUser'
}
$build = Join-Path (Split-Path $PSScriptRoot -Parent) '.build.ps1'
Invoke-Build Refresh -File $build -DryRun:$DryRun
