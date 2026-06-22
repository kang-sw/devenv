---
title: Windows local-devenv auto-build (lift launcher gate + one-shot installer)
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260622-chore-windows-shipping-hardening: its Phase C branch-pin strategy assumes "cold install -> real Go build", which is currently false on Windows; this ticket makes that true
  260622-bug-wsflow-launcher-coldload-divergence: wsflow launcher shares the same gate and must be mirrored
related-mental-model:
  - plugin-runtime
---

# Windows local-devenv auto-build (lift launcher gate + one-shot installer)

## Background

The plugin launcher supports a local-devenv dogfood loop: a gitignored
`.local-devenv-runtime` marker (absolute `source_root` / `tool_dir` / `go`) makes
the launcher build the Go runtime from the local checkout on each cold-load
(fingerprint-cached so an unchanged tree reuses the binary). On Linux/macOS
`install.sh` snapshots `agents-plugin/` into `~/.claude/plugins/ws-plugin/ws/`,
registers a directory-source marketplace, and the hand-authored marker drives
auto-build.

On Windows this loop is dead: `read_local_devenv_contract` short-circuits on
`os_name == "windows"` (canonical launcher line ~442, wsflow line ~398), so the
marker is inert and the runtime must come from a published release or
`WS_MCP_BOOTSTRAP_BINARY`. There is also no Windows equivalent of `install.sh`, so
even the marketplace/settings registration is manual.

Net effect: Windows dogfood requires a manual `go build` + env injection on every
source change, and `260622-chore`'s Phase C strategy ("cold install -> real Go
build") cannot actually happen on Windows. The Windows runtime bugs that made the
gate conservative (atomic replace, path handling, processAlive) were fixed in
`260622-chore` Phase A, so lifting the gate is now safe to validate.

## Decisions

- **Lift the Windows gate**, not add a committed dev label. The marker must carry
  machine-absolute paths (`source_root`/`tool_dir`/`go`), which a committed file
  cannot; the existing gitignored marker is the correct opt-in and only needs to
  fire on Windows.
- **Full one-shot scope** (user decision): ship a PowerShell installer that ports
  `install.sh`'s plugin registration (snapshot + marketplace.json +
  settings.json + known_marketplaces.json) and writes the marker, so Windows
  dogfood is one-shot like Linux.
- **Spec change is in scope.** This changes documented launcher behavior
  (`plugin-runtime` spec + `ws-mcp.md` runbook both state local repair is inactive
  on Windows). Existing tests assert the Windows-disabled behavior and must be
  inverted.
- **wsflow mirror required.** The launcher is shared packaging surface; the
  gate-lift and build-env recovery mirror into the wsflow launcher.

## Constraints

- **Live-host safety (hard, inherited):** no process termination added; never
  image-name kill. (This slice adds none.)
- **Happy-path / non-Windows behavior unchanged.** Marker-absent or marker-invalid
  installs (the normal end-user case on every OS) still fall back to the release
  path. Activation requires a valid marker + present `go` + the `.claude`/`.codex`
  plugins cache layout, exactly as on Linux today.
- **Empirical Windows confirmation deferred** to the chore's Phase C real-Windows
  run; this slice's Linux verification is unit tests (mocking `os_name`) + script
  lint.

## Phases

### Phase 1: Launcher gate-lift + Windows build correctness + spec/tests/wsflow

- Remove `os_name == "windows"` from `read_local_devenv_contract` (canonical +
  wsflow mirror); keep all marker validation.
- Make the `go` executable check Windows-aware (accept `go.exe`; `os.access(X_OK)`
  is loose on Windows — require an existing file).
- Extend `local_devenv_build_env` to recover `USERPROFILE` / `LOCALAPPDATA` on
  Windows (mirrors the existing `HOME` recovery) so a sanitized launch env still
  resolves GOCACHE/GOMODCACHE for `go build`.
- Invert the existing Windows-disabled assertions and add positive Windows
  activation + build-env tests (mock `os_name == "windows"`).
- Update `ai-docs/spec/plugin-runtime.md`, `ai-docs/ref/ws-mcp.md`, and
  `ai-docs/mental-model/plugin-runtime.md` to reflect Windows local-devenv build.

### Result (ba67f61e) - 2026-06-22

Done. Gate `os_name == "windows"` removed from `read_local_devenv_contract` in
both launchers; `go` check made Windows-aware (require existing file, keep X_OK on
POSIX); `local_devenv_build_env` now takes `os_name` and recovers
`USERPROFILE`/`LOCALAPPDATA` on Windows (`os_name` threaded through
`build_local_devenv_runtime`). Tests: two Windows-disabled assertions inverted to
`assertTrue`, new Windows build-env recovery test + non-Windows negative
assertions, `fake_build` stubs widened. 40 canonical + 8 wsflow tests green,
both launchers `py_compile` clean.

- **No spec change:** `plugin-runtime.md` local-devenv text is already OS-neutral
  (no Windows exclusion clause); the change makes it more accurate, not divergent.
  `ws-mcp.md` runbook + `plugin-runtime` mental model updated.
- **End-user-safe:** marker-absent installs return `None` (release path) on every
  OS; only Windows + valid marker + present `go` newly activates.
- Closes the gate-divergence portion of
  `260622-bug-wsflow-launcher-coldload-divergence` (wsflow mirrored).

### Phase 2: PowerShell one-shot installer + marker writer + dogfood doc

- Add `scripts/install-claude-plugin.ps1`: snapshot `agents-plugin/` into
  `%USERPROFILE%\.claude\plugins\ws-plugin\ws`, generate marketplace.json, patch
  `settings.json` (extraKnownMarketplaces + enabledPlugins) and
  `known_marketplaces.json`, and write the `.local-devenv-runtime` marker pointing
  at the clone (`source_root` / `tool_dir` / `go.exe`).
- Update `ai-docs/ref/windows-dogfood.md`: replace the manual
  `WS_MCP_BOOTSTRAP_BINARY` route with the one-shot installer + auto-build loop;
  keep the bootstrap route documented as the no-clone fallback.

### Result (cbb7f983) - 2026-06-22

Done. `scripts/install-claude-plugin.ps1` ports install.sh's registration
(snapshot + marketplace.json + settings.json + known_marketplaces.json) and writes
the marker; JSON merges delegated to `python3` (PS-version agnostic), marker via
`ConvertTo-Json`, `-DryRun` supported, `claude plugin install` invoked when
present. `windows-dogfood.md` rewritten to lead with the one-shot + auto-build
loop, `WS_MCP_BOOTSTRAP_BINARY` kept as no-clone fallback.

- **PowerShell syntax deferred to the Windows host** (no `pwsh` on the Linux dev
  host). The embedded python JSON-merge logic was functionally verified on Linux:
  settings merge preserves unrelated keys + drops obsolete `ws`/`ws@ws`,
  marketplace.json generated against the real `plugin.json` (v0.30.1, `./ws`),
  known_marketplaces entry correct.
- Empirical Windows cold-load + auto-build remains deferred to `260622-chore`
  Phase C.

## Verification

- `python3 -m unittest discover agents-plugin/tests` green (inverted + new tests).
- `python3 -m py_compile agents-plugin/bin/ws-mcp-launcher.py` and the wsflow copy
  clean.
- `python3 -m unittest discover agents-plugin-wsflow/tests` green (mirror intact).
- PowerShell installer: `pwsh -NoProfile -Command "& { . ./scripts/install-claude-plugin.ps1 -WhatIf }"`
  or syntax check if pwsh available on the Linux host; otherwise lint-read and
  defer execution to the Windows host.
- Empirical Windows auto-build cold-load deferred to chore Phase C.
