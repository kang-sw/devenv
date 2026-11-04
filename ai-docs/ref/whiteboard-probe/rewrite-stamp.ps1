# One-shot stamp rewriter for the whiteboard file:// probe (native Windows).
# Rewrites the page version in probe.html, then probe.stamp.js, each through a
# temp file and an atomic replace: the order the whiteboard render uses.
# Run: powershell -ExecutionPolicy Bypass -File rewrite-stamp.ps1
$ErrorActionPreference = 'Stop'
$dir = $PSScriptRoot
$v = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
$enc = New-Object System.Text.UTF8Encoding($false)

$page = Join-Path $dir 'probe.html'
$html = [IO.File]::ReadAllText($page, $enc) -replace '__PV_[0-9]*__', "__PV_${v}__"
[IO.File]::WriteAllText("$page.tmp", $html, $enc)
[IO.File]::Replace("$page.tmp", $page, [NullString]::Value)

$stamp = Join-Path $dir 'probe.stamp.js'
[IO.File]::WriteAllText("$stamp.tmp", "wbStamp(`"$v`");`n", $enc)
[IO.File]::Replace("$stamp.tmp", $stamp, [NullString]::Value)
"stamp $v"
