function Read-PortalYamlList {
    <#
    .SYNOPSIS
        Reads the list under -Key in one of a tenant's identity files (principals.yaml, tokens.yaml) as one ordered
        hashtable per item. Only the shape those files hold: a top-level key whose items are block mappings
        ("- id: ...", one key per line) or one-line flow mappings ("- { id: ..., role: ... }") of scalars. A scalar may
        be single- or double-quoted; null and ~ are $null. No file, or no items, is an empty list.
    #>
    [CmdletBinding()]
    [OutputType([System.Collections.Specialized.OrderedDictionary[]])]
    param(
        [Parameter(Mandatory)] [string] $Path,
        [Parameter(Mandatory)] [string] $Key
    )
    $scalar = {
        param([string] $v)
        $v = $v.Trim()
        if ($v -match "^'(.*)'$") { return $Matches[1].Replace("''", "'") }
        if ($v -match '^"(.*)"$') { return $Matches[1] }
        if ($v -in 'null', '~', '') { return $null }
        $v
    }
    $items = [System.Collections.Generic.List[System.Collections.Specialized.OrderedDictionary]]::new()
    if (-not (Test-Path -LiteralPath $Path)) { return , $items.ToArray() }
    $inList = $false
    $item = $null
    foreach ($line in (Get-Content -LiteralPath $Path)) {
        if ($line -match '^\s*(#|$)') { continue }
        if ($line -match '^(\S[^:]*):') { $inList = $Matches[1].Trim() -eq $Key; continue }
        if (-not $inList) { continue }
        if ($line -match '^\s*-\s*\{(.*)\}\s*$') {
            $item = [ordered]@{}
            foreach ($pair in ($Matches[1] -split ',')) {
                $k, $v = $pair -split ':', 2
                if ($null -ne $v) { $item[$k.Trim()] = & $scalar $v }
            }
            $items.Add($item)
            $item = $null
            continue
        }
        if ($line -match '^\s*-\s*([^:]+):(.*)$') {
            $item = [ordered]@{}
            $items.Add($item)
            $item[$Matches[1].Trim()] = & $scalar $Matches[2]
            continue
        }
        if ($item -and $line -match '^\s+([^:]+):(.*)$') { $item[$Matches[1].Trim()] = & $scalar $Matches[2] }
    }
    , $items.ToArray()
}
