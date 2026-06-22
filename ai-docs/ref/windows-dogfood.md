# Windows source-build dogfood

How to dogfood a branch's `ws` plugin on a native Windows host with an automatic
source-build loop, mirroring the Linux `install.sh` flow. Used for
`260622-chore-windows-shipping-hardening` Phase C and any later Windows cold-load
verification.

## How it works

The launcher's local-devenv path (gitignored `.local-devenv-runtime` marker ->
build the Go runtime from the checkout on cold-load, fingerprint-cached) is
honored on Windows as of `260622-feat-windows-local-devenv-autobuild`. A clone on
the Windows host is required regardless: the Go module (`agents-plugin-tool/`) is
not part of the `agents-plugin/` plugin package, so the source must be physically
present to build.

`scripts/install-claude-plugin.ps1` is the Windows equivalent of `install.sh`'s
plugin registration: it snapshots `agents-plugin/` into
`%USERPROFILE%\.claude\plugins\ws-plugin\ws`, generates `marketplace.json`, patches
`settings.json` + `known_marketplaces.json`, and writes the marker pointing at the
clone. After that, every Claude Code restart rebuilds the runtime from the clone
when the Go source changed.

## Prerequisites

- Native Windows host with the target branch checked out from a clone.
- Go toolchain on `PATH`.
- `python3` on `PATH` (the launcher is Python; the installer reuses it for JSON
  merges. A Windows Store alias without a real install fails — install Python 3).

## Procedure (one-shot)

```powershell
# In the clone, on the target branch
pwsh -NoProfile -File scripts\install-claude-plugin.ps1
# Preview without writing:
#   pwsh -NoProfile -File scripts\install-claude-plugin.ps1 -DryRun
```

Restart Claude Code. On first MCP load the launcher reads the marker, builds
`agents-plugin-tool/cmd/ws-mcp` with the clone's `go.exe`, validates the runtime
contract, and execs it. The plugin version is bumped per dev-merge, so the cache
invalidates and cold-extracts on reinstall — this is the first-load path under
test.

## Iteration loop

- **Edit Go source** -> just restart Claude Code; the source fingerprint changes
  and the runtime rebuilds automatically.
- **Edit `launcher.py` / skills / `runtime.json`** -> re-run the installer to
  re-snapshot, then restart.

## Fallback: no clone / no Go toolchain

If building on the host is not possible, inject a prebuilt binary instead (skips
the marker/auto-build path; still exercises `install_tmp_runtime`):

```powershell
cd agents-plugin-tool
go build -o C:\ws\ws-mcp-windows-amd64.exe ./cmd/ws-mcp   # or cross-compile elsewhere
setx WS_MCP_BOOTSTRAP_BINARY C:\ws\ws-mcp-windows-amd64.exe
```

Then register the plugin (installer above, or your usual branch-pin install) and
restart. `WS_MCP_LAUNCHER_DEBUG=1` prints cold-load diagnostics to stderr.

## Acceptance checklist (Phase C)

- Cold MCP load succeeds (no failure from rsrc-tree race, unreadable contract, or
  replace contention; the source build completes within the startup window).
- `ws.ferrule` login resolves a session key.
- Mercenary round-trip: `register` -> `call` -> `wait` -> `result`, plus `cancel`.
- Workflow bootstrapping: skill invoke -> `playbook.print` / `playbook.render`.

## Hard constraint

Live-host safety: kill only PID/job-scoped subtrees. Never image-name kill
(`taskkill /IM`) or sweep — a dogfooding host may run a live `claude.exe`.
