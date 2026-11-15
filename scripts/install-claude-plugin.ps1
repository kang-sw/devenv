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

# --- 1-2. validate, stage snapshot + marketplace, then publish -----------------
Info 'Syncing ws plugin snapshot and marketplace...'
$snapshotCode = @'
import json, shutil, subprocess, sys, tempfile
from pathlib import Path

repo, cache = map(Path, sys.argv[1:3])
go = sys.argv[3]
source = repo / "agents-plugin"
helper = source / "bin/ws-pi-payload.py"
subprocess.run([sys.executable, str(helper), "--plugin", str(source), "--check-source", str(repo), "--allow-stale-source"], check=True)
cache.parent.mkdir(parents=True, exist_ok=True)
stage = Path(tempfile.mkdtemp(prefix=".ws-snapshot-", dir=cache.parent))
backup = stage / "previous"
try:
    shutil.copytree(source, stage / "ws")
    subprocess.run([sys.executable, str(stage / "ws/bin/ws-pi-payload.py"), "--plugin", str(stage / "ws")], check=True)
    # Deliberate ws-only dogfood marker, never part of the native archive.
    (stage / "ws/.local-devenv-runtime").write_bytes((json.dumps({
        "schema_version": 1, "source_root": str(repo),
        "tool_dir": str(repo / "agents-plugin-tool"), "go": go,
    }) + "\n").encode("utf-8"))
    plugin = json.loads((stage / "ws/.claude-plugin/plugin.json").read_text())
    marketplace = {
        "name": "kang-sw-devenv",
        "description": "kang-sw personal devenv plugin marketplace",
        "owner": {"name": "kang-sw"},
        "plugins": [{"name": plugin["name"], "version": plugin.get("version"),
                     "description": plugin.get("description", ""),
                     "author": plugin.get("author", {"name": "kang-sw"}), "source": "./ws"}],
    }
    (stage / ".claude-plugin").mkdir()
    (stage / ".claude-plugin/marketplace.json").write_text(json.dumps(marketplace, indent=2) + "\n")
    ready = stage / "ready"
    ready.mkdir()
    for name in ("ws", ".claude-plugin"):
        (stage / name).rename(ready / name)
    if cache.exists():
        cache.rename(backup)
    try:
        ready.rename(cache)
    except BaseException:
        if backup.exists():
            backup.rename(cache)
        raise
finally:
    if backup.exists() and not cache.exists():
        print(f"Snapshot rollback needs recovery from {backup}", file=sys.stderr)
    else:
        shutil.rmtree(stage)
'@
Invoke-Py $snapshotCode @($RepoDir, $PluginCache, $GoPath)

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

# --- 5. BOM-less UTF-8 marker was published with the validated snapshot -------
Info '.local-devenv-runtime marker included in staged ws snapshot.'

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
