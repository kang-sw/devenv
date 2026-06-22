# Windows source-build dogfood

How to dogfood a branch's `ws` plugin on a native Windows host by building the
runtime from a local clone and injecting it. This is the path used for
`260622-chore-windows-shipping-hardening` Phase C (branch-pinned real-Windows
acceptance) and any later Windows cold-load verification.

## Why this is needed

On Windows the launcher's local-devenv source-build path is inactive
(`read_local_devenv_contract` short-circuits on `os_name == "windows"`), and the
runtime binary is otherwise resolved from a published GitHub release. A branch
under acceptance has no release yet, so the runtime is provided by building it in
the clone and injecting it through `WS_MCP_BOOTSTRAP_BINARY`. The injected binary
still flows through `install_tmp_runtime`, so the Phase B cold-load hardening
(rsrc-tree wait, runtime-contract read retry, `os.replace` retry) is exercised.

A clone on the Windows host is required regardless: the Go module
(`agents-plugin-tool/`) is not part of the `agents-plugin/` plugin package, so the
source must be physically present to build at all.

## Prerequisites

- Native Windows host with the target branch checked out from a clone.
- Go toolchain on `PATH`.
- `python3` on `PATH` (the launcher is Python; a Windows Store alias without a
  real install fails — install Python 3 and reinstall the plugin).

## Procedure

```powershell
# In the clone, on the target branch
cd agents-plugin-tool
go build -o C:\ws\ws-mcp-windows-amd64.exe ./cmd/ws-mcp
go test ./...                                   # optional: native full-suite

# Inject the built runtime (user environment variable)
setx WS_MCP_BOOTSTRAP_BINARY C:\ws\ws-mcp-windows-amd64.exe
# Optional cold-load diagnostics to stderr:
#   setx WS_MCP_LAUNCHER_DEBUG 1
```

Then install the branch's plugin tree so the launcher, `runtime.json`, and skills
come from the branch (not `main`):

- Local directory-source marketplace pointing at the clone (no push needed; always
  reflects the checked-out branch), or
- Push the branch and branch-pin install the usual way.

The plugin version is bumped per dev-merge, so the plugin cache invalidates and
cold-extracts on reinstall — this is the first-load path under test. Restart the
host; on first MCP load the launcher copies the injected binary into
`.runtime/windows-amd64/`, validates the runtime contract, and execs it.

## Acceptance checklist (Phase C)

- Cold MCP load succeeds (no failure from rsrc-tree race, unreadable contract, or
  replace contention).
- `ws.ferrule` login resolves a session key.
- Mercenary round-trip: `register` -> `call` -> `wait` -> `result`, plus `cancel`.
- Workflow bootstrapping: skill invoke -> `playbook.print` / `playbook.render`.

## Hard constraint

Live-host safety: kill only PID/job-scoped subtrees. Never image-name kill
(`taskkill /IM`) or sweep — a dogfooding host may run a live `claude.exe`.

## Optional future simplification

Lifting the `os_name == "windows"` gate in `read_local_devenv_contract` (with a
Windows-aware `go` check) plus a marker-writer helper would let the existing
gitignored `.local-devenv-runtime` marker drive an automatic build-on-launch loop
on Windows, removing the manual build + `WS_MCP_BOOTSTRAP_BINARY` step. Not
required for Phase C; capture as a ticket if the manual loop becomes a burden.
