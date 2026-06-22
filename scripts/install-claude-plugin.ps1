#!/usr/bin/env pwsh
# Windows one-shot installer for the ws Claude plugin local-devenv dogfood loop.
#
# Mirrors the plugin-registration portion of install.sh (snapshot copy +
# marketplace.json + settings.json + known_marketplaces.json) and additionally
# writes the gitignored .local-devenv-runtime marker so the launcher builds the
# Go runtime from THIS clone on cold-load (260622-feat-windows-local-devenv-autobuild).
#
# Prerequisites: Go and python3 on PATH (python3 is already required by the
# launcher; this script reuses it for JSON merges to stay PowerShell-version
# agnostic). Run from the repo on the branch you want to dogfood.

[CmdletBinding()]
param(
    [switch]$DryRun
)
$ErrorActionPreference = 'Stop'

$RepoDir      = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$ClaudeHome   = Join-Path $env:USERPROFILE '.claude'
$PluginCache  = Join-Path $ClaudeHome 'plugins\ws-plugin'
$PluginWs     = Join-Path $PluginCache 'ws'
$AgentsPlugin = Join-Path $RepoDir 'agents-plugin'
$ToolDir      = Join-Path $RepoDir 'agents-plugin-tool'
$Settings     = Join-Path $ClaudeHome 'settings.json'
$KnownMkts    = Join-Path $ClaudeHome 'plugins\known_marketplaces.json'
$InstalledRec = Join-Path $ClaudeHome 'plugins\installed_plugins.json'

function Info($m) { Write-Host "  $m" }

# --- prerequisites ------------------------------------------------------------
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) { throw 'go not found on PATH; install Go to enable the local source build.' }
$GoPath = $go.Source
if (-not (Get-Command python3 -ErrorAction SilentlyContinue)) {
    throw 'python3 not found on PATH (required by the launcher and this installer).'
}
if (-not (Test-Path $AgentsPlugin)) { throw "agents-plugin not found under $RepoDir; run from a devenv clone." }
if (-not (Test-Path (Join-Path $ToolDir 'cmd\ws-mcp'))) { throw "agents-plugin-tool/cmd/ws-mcp not found under $RepoDir." }

Info "repo:        $RepoDir"
Info "plugin cache: $PluginWs"
Info "go:          $GoPath"
if ($DryRun) { Info '(dry run — no files written)' }

# Run a here-string python script with positional args; throws on non-zero exit.
function Invoke-Py([string]$Code, [string[]]$PyArgs) {
    if ($DryRun) { return }
    $Code | & python3 - @PyArgs
    if ($LASTEXITCODE -ne 0) { throw "python3 step failed (exit $LASTEXITCODE)" }
}

# --- 1. snapshot agents-plugin -> $PluginWs (mirror) --------------------------
Info 'Syncing ws plugin snapshot...'
if (-not $DryRun) {
    if (Test-Path $PluginWs) { Remove-Item -Recurse -Force $PluginWs }
    New-Item -ItemType Directory -Force -Path $PluginWs | Out-Null
    Copy-Item -Path (Join-Path $AgentsPlugin '*') -Destination $PluginWs -Recurse -Force
}

# --- 2. generate marketplace.json under the snapshot --------------------------
Info 'Generating marketplace.json...'
$mktCode = @'
import json, os, sys
cache, ws_dir = sys.argv[1], sys.argv[2]
plugin = json.load(open(os.path.join(ws_dir, ".claude-plugin", "plugin.json")))
marketplace = {
    "name": "kang-sw-devenv",
    "description": "kang-sw personal devenv plugin marketplace",
    "owner": {"name": "kang-sw"},
    "plugins": [{
        "name": plugin["name"],
        "version": plugin.get("version"),
        "description": plugin.get("description", ""),
        "author": plugin.get("author", {"name": "kang-sw"}),
        "source": "./ws",
    }],
}
out = os.path.join(cache, ".claude-plugin", "marketplace.json")
os.makedirs(os.path.dirname(out) or ".", exist_ok=True)
with open(out, "w") as f:
    json.dump(marketplace, f, indent=2)
    f.write("\n")
'@
Invoke-Py $mktCode @($PluginCache, $PluginWs)

# --- 3. patch settings.json (marketplace + enabled plugin) --------------------
Info 'Registering marketplace + enabling plugin in settings.json...'
$settingsCode = @'
import json, os, sys
settings_path, cache = sys.argv[1], sys.argv[2]
settings = json.load(open(settings_path)) if os.path.isfile(settings_path) else {}
mkts = settings.setdefault("extraKnownMarketplaces", {})
mkts.pop("ws", None)
mkts["kang-sw-devenv"] = {"source": {"source": "directory", "path": cache}}
plugins = settings.setdefault("enabledPlugins", {})
plugins.pop("ws@ws", None)
plugins.pop("wsflow@kang-sw-devenv", None)
plugins["ws@kang-sw-devenv"] = True
os.makedirs(os.path.dirname(settings_path) or ".", exist_ok=True)
with open(settings_path, "w") as f:
    json.dump(settings, f, indent=2)
    f.write("\n")
'@
Invoke-Py $settingsCode @($Settings, $PluginCache)

# --- 4. pre-register in known_marketplaces.json -------------------------------
Info 'Pre-registering marketplace in known_marketplaces.json...'
$knownCode = @'
import json, os, sys
from datetime import datetime, timezone
km_path, cache = sys.argv[1], sys.argv[2]
km = json.load(open(km_path)) if os.path.isfile(km_path) else {}
now = datetime.now(timezone.utc)
km["kang-sw-devenv"] = {
    "source": {"source": "directory", "path": cache},
    "installLocation": cache,
    "lastUpdated": now.strftime("%Y-%m-%dT%H:%M:%S.") + f"{now.microsecond // 1000:03d}Z",
}
os.makedirs(os.path.dirname(km_path) or ".", exist_ok=True)
with open(km_path, "w") as f:
    json.dump(km, f, indent=2)
    f.write("\n")
'@
Invoke-Py $knownCode @($KnownMkts, $PluginCache)

# --- 5. write the local-devenv marker (builds from THIS clone) ----------------
Info 'Writing .local-devenv-runtime marker...'
if (-not $DryRun) {
    $marker = [ordered]@{
        schema_version = 1
        source_root    = $RepoDir
        tool_dir       = $ToolDir
        go             = $GoPath
    } | ConvertTo-Json
    Set-Content -Path (Join-Path $PluginWs '.local-devenv-runtime') -Value $marker -Encoding utf8
}

# --- 6. install via claude CLI if present (drops a stale record first) --------
$claude = Get-Command claude -ErrorAction SilentlyContinue
if ($claude) {
    Info 'Installing plugin via claude CLI...'
    $dropCode = @'
import json, os, sys
path, ref = sys.argv[1], sys.argv[2]
if os.path.isfile(path):
    d = json.load(open(path))
    if ref in d.get("plugins", {}):
        del d["plugins"][ref]
        with open(path, "w") as f:
            json.dump(d, f, indent=2)
            f.write("\n")
'@
    Invoke-Py $dropCode @($InstalledRec, 'ws@kang-sw-devenv')
    if (-not $DryRun) {
        & claude plugin install ws@kang-sw-devenv
        if ($LASTEXITCODE -ne 0) {
            Write-Warning 'claude plugin install failed — run manually: claude plugin install ws@kang-sw-devenv'
        }
    }
} else {
    Write-Warning 'claude not found — run manually after install: claude plugin install ws@kang-sw-devenv'
}

Write-Host ''
Info 'Done. Restart Claude Code; first MCP load builds the runtime from this clone.'
Info 'Edit Go source -> restart rebuilds automatically. Edit launcher/skills -> re-run this script.'
