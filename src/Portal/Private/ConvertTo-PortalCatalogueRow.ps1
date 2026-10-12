function ConvertTo-PortalCatalogueRow {
    <#
    .SYNOPSIS
    The Catalogue's rows from a GET .../emitters envelope: one per Card or Emitter node (those carry status), as
    Name, Kind, Status, Version, Layers and LayerVersions. Layer nodes are not rows.
    #>
    [CmdletBinding()]
    param([Parameter(Mandatory)] $Envelope)
    foreach ($node in @($Envelope.nodes)) {
        $p = $node.PSObject.Properties['properties']
        $props = if ($p) { $p.Value }
        if (-not $props) { continue }
        $get = { param($k) $q = $props.PSObject.Properties[$k]; if ($q) { $q.Value } else { $null } }
        $status = & $get 'status'
        if (-not $status) { continue }
        [pscustomobject]@{
            Name          = [string]$node.name
            Kind          = [string]$node.kind
            Status        = [string]$status
            Version       = [string](& $get 'version')
            Layers        = [int](& $get 'layers')
            LayerVersions = (@(& $get 'layerVersions') | Where-Object { $_ }) -join ', '
        }
    }
}
