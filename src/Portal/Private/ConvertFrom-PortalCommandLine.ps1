function ConvertFrom-PortalCommandLine {
    <#
    .SYNOPSIS
    The flags of a portal command line as a hashtable, name (no dashes) => value. Words split as Windows splits a
    command line for a Go program (CommandLineToArgvW: 2n backslashes before a quote are n and the quote delimits,
    2n+1 are n and a literal quote, other backslashes are literal), so a value reads as the portal read it.
    -name value, --name value and -name=value, as Go's flag package takes them. The first word is the image.
    #>
    [CmdletBinding()]
    param([AllowEmptyString()] [string] $CommandLine)
    $words = [System.Collections.Generic.List[string]]::new()
    $cur = [Text.StringBuilder]::new()
    $inWord = $false; $quoted = $false; $slashes = 0
    foreach ($c in $CommandLine.ToCharArray()) {
        if ($c -eq '\') { $slashes++; $inWord = $true; continue }
        if ($c -eq '"') {
            $null = $cur.Append('\', [math]::Floor($slashes / 2))
            if ($slashes % 2) { $null = $cur.Append('"') } else { $quoted = -not $quoted }
            $slashes = 0; $inWord = $true; continue
        }
        if ($slashes) { $null = $cur.Append('\', $slashes); $slashes = 0 }
        if (-not $quoted -and ($c -eq ' ' -or $c -eq "`t")) {
            if ($inWord) { $words.Add($cur.ToString()); $null = $cur.Clear(); $inWord = $false }
            continue
        }
        $null = $cur.Append($c); $inWord = $true
    }
    if ($slashes) { $null = $cur.Append('\', $slashes) }
    if ($inWord) { $words.Add($cur.ToString()) }
    $flags = @{}
    for ($i = 1; $i -lt $words.Count; $i++) {
        if ($words[$i] -notmatch '^--?([A-Za-z][\w-]*)(=(.*))?$') { continue }
        $name = $Matches[1]
        if ($Matches[2]) { $flags[$name] = $Matches[3]; continue }
        if ($i + 1 -lt $words.Count -and $words[$i + 1] -notmatch '^--?[A-Za-z]') { $flags[$name] = $words[$i + 1]; $i++ }
        else { $flags[$name] = '' }
    }
    $flags
}
