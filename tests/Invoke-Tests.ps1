#Requires -Version 7.4
# Run only as: pwsh -NoProfile -File .\tests\Invoke-Tests.ps1
# Exits with the number of failed tests (0 = all passed).
param([string] $Tag, [string] $ExcludeTag)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module Pester -MinimumVersion 5.5 -ErrorAction Stop

$cfg = New-PesterConfiguration
$cfg.Run.Path = $PSScriptRoot
$cfg.Run.Exit = $true
$cfg.Output.Verbosity = 'Detailed'
$cfg.TestResult.Enabled = $true
$cfg.TestResult.OutputPath = Join-Path $PSScriptRoot 'testResults.xml'
if ($Tag)        { $cfg.Filter.Tag = $Tag }
if ($ExcludeTag) { $cfg.Filter.ExcludeTag = $ExcludeTag }
Invoke-Pester -Configuration $cfg
