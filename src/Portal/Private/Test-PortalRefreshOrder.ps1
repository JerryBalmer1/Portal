function Test-PortalRefreshOrder {
    <#
    .SYNOPSIS
    The rules R1 to R6 of docs/refresh/dependency-order.md over a Refresh order's steps: one line per problem, each
    starting with its rule ("R3: ..."), or nothing when the order keeps them all.
    #>
    [CmdletBinding()]
    [OutputType([string])]
    param([Parameter(Mandatory)] [AllowEmptyCollection()] [object[]] $Steps)
    $rank = @{ down = 0; up = 1; check = 2 }
    $n = $Steps.Count

    # R5: known repos, actions and phases; an invoke names its file and task; ids unique.
    $seen = @{}
    foreach ($s in $Steps) {
        if ($s.Repo -notmatch '^Portal(\.[A-Za-z][A-Za-z0-9]*)*$') { "R5: step $($s.Id) names repo '$($s.Repo)', not Portal or Portal.<Name>" }
        if ($s.Action -notin 'invoke', 'approve', 'catalogue') { "R5: step $($s.Id) has action '$($s.Action)', not invoke, approve or catalogue" }
        if (-not $rank.ContainsKey([string]$s.Phase)) { "R5: step $($s.Id) has phase '$($s.Phase)', not down, up or check" }
        if ($s.Action -eq 'invoke' -and (-not $s.File -or -not $s.Task)) { "R5: step $($s.Id) invokes without a File and a Task" }
        if ($seen.ContainsKey([string]$s.Id)) { "R5: step id $($s.Id) is used twice" }
        $seen[[string]$s.Id] = $true
    }

    # R1: down, then up, then check.
    for ($i = 1; $i -lt $n; $i++) {
        $a, $b = $Steps[$i - 1], $Steps[$i]
        if ($rank.ContainsKey([string]$a.Phase) -and $rank.ContainsKey([string]$b.Phase) -and $rank[[string]$b.Phase] -lt $rank[[string]$a.Phase]) {
            "R1: step $($b.Id) ($($b.Phase)) comes after $($a.Id) ($($a.Phase))"
        }
    }

    # R2: portal-down is the last down, portal-dev the first up, and a step that needs a portal runs while one is up.
    $index = @{}
    for ($i = 0; $i -lt $n; $i++) { $index[[string]$Steps[$i].Id] = $i }
    $downs = @(for ($i = 0; $i -lt $n; $i++) { if ($Steps[$i].Phase -eq 'down') { $i } })
    $ups = @(for ($i = 0; $i -lt $n; $i++) { if ($Steps[$i].Phase -eq 'up') { $i } })
    if (-not $index.ContainsKey('portal-down')) { 'R2: there is no portal-down step' }
    elseif ($downs -and $Steps[$downs[-1]].Id -ne 'portal-down') { "R2: the last down step is $($Steps[$downs[-1]].Id), not portal-down" }
    if (-not $index.ContainsKey('portal-dev')) { 'R2: there is no portal-dev step' }
    elseif ($ups -and $Steps[$ups[0]].Id -ne 'portal-dev') { "R2: the first up step is $($Steps[$ups[0]].Id), not portal-dev" }
    for ($i = 0; $i -lt $n; $i++) {
        $s = $Steps[$i]
        if (-not $s.NeedsPortal) { continue }
        if ($s.Phase -eq 'down' -and $index.ContainsKey('portal-down') -and $i -gt $index['portal-down']) {
            "R2: step $($s.Id) needs a portal but runs after portal-down"
        }
        if ($s.Phase -in 'up', 'check' -and $index.ContainsKey('portal-dev') -and $i -lt $index['portal-dev']) {
            "R2: step $($s.Id) needs a portal but runs before portal-dev"
        }
    }

    # R3: nothing restarts the portal once a live layer is pushed.
    $pushes = @(for ($i = 0; $i -lt $n; $i++) { if ($Steps[$i].PushesLive) { $i } })
    if ($pushes) {
        for ($i = $pushes[0] + 1; $i -lt $n; $i++) {
            if ($Steps[$i].RestartsPortal) { "R3: step $($Steps[$i].Id) restarts the portal after $($Steps[$pushes[0]].Id) pushed live layers, which it would drop" }
        }
    }

    # R4: an approval after the last live push, before the catalogue.
    $catalogues = @(for ($i = 0; $i -lt $n; $i++) { if ($Steps[$i].Action -eq 'catalogue') { $i } })
    if ($pushes) {
        $end = if ($catalogues) { $catalogues[0] } else { $n }
        $approved = @(for ($i = $pushes[-1] + 1; $i -lt $end; $i++) { if ($Steps[$i].Action -eq 'approve') { $i } })
        if (-not $approved) { "R4: no approve step between $($Steps[$pushes[-1]].Id), the last live push, and the catalogue" }
    }

    # R6: one catalogue, last.
    if ($catalogues.Count -ne 1) { "R6: $($catalogues.Count) catalogue steps, not one" }
    elseif ($catalogues[0] -ne $n - 1) { "R6: the catalogue step $($Steps[$catalogues[0]].Id) is not the last step" }
}
