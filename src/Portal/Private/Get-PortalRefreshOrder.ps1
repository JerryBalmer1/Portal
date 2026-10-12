function Get-PortalRefreshOrder {
    <#
    .SYNOPSIS
    The Refresh order from docs/refresh/order.psd1 (docs/design.md 56): Steps, one object per step in order, and
    CardOnly, the repos with no build script. Missing booleans read false.
    #>
    [CmdletBinding()]
    param([string] $Path = (Join-Path $script:RepoRoot 'docs' 'refresh' 'order.psd1'))
    $data = Import-PowerShellDataFile -LiteralPath $Path
    $steps = @(foreach ($s in @($data.Steps)) {
            [pscustomobject]@{
                Id                = [string]$s['Id']
                Repo              = [string]$s['Repo']
                Action            = [string]$s['Action']
                File              = [string]$s['File']
                Task              = [string]$s['Task']
                Phase             = [string]$s['Phase']
                NeedsPortal       = [bool]$s['NeedsPortal']
                RestartsPortal    = [bool]$s['RestartsPortal']
                PushesLive        = [bool]$s['PushesLive']
                SkipWithoutDocker = [bool]$s['SkipWithoutDocker']
            }
        })
    [pscustomobject]@{ Steps = $steps; CardOnly = @($data['CardOnly'] | Where-Object { $_ }) }
}
