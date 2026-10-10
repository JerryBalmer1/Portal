@{
    RootModule        = 'Portal.psm1'
    ModuleVersion     = '0.3.6'
    GUID              = '3f6c2a9e-7d41-4b8e-9c55-1a2b3c4d5e6f'
    Author            = 'Jerry Balmer'
    CompanyName       = 'JerryBalmer1'
    Copyright         = '(c) 2026 Jerry Balmer. All rights reserved.'
    Description       = 'Thin wrappers over the Portal API: start and stop the portal, read the screen graph, approve it as a snap and diff two, mount a data layer.'
    PowerShellVersion = '7.4'
    FunctionsToExport = @(
        'Start-Portal'
        'Stop-Portal'
        'Get-PortalStatus'
        'Get-PortalScreen'
        'New-PortalSnap'
        'Compare-PortalSnap'
        'Mount-PortalLayer'
    )
    CmdletsToExport   = @()
    VariablesToExport = @()
    AliasesToExport   = @()
    PrivateData       = @{
        PSData = @{
            Tags       = @('graph', 'portal', 'openapi')
            ProjectUri = 'https://github.com/JerryBalmer1/Portal'
        }
    }
}
