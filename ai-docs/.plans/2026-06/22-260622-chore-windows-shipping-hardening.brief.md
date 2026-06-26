# Brief: 260622-chore-windows-shipping-hardening (Phase A)

## Intent

Make the ws named-agent ("mercenary") runtime Windows-correct at the code level so
the upcoming Windows shipping is safe. This is the static-hardening slice: seven
Windows-only conformance fixes plus their Windows unit tests, bringing the Windows
runtime into behavioral parity with the already-correct Unix paths. No
caller-visible contract changes — these are behavior-preserving fixes.

All work is in `agents-plugin-tool/` (Go module) plus one shell script under
`agents-plugin-tool/scripts/`.

## Scope Boundary

In scope: the seven items below (Phase A only).

Explicitly deferred (do NOT touch):
- Launcher cold-load robustness (rsrc materialization race, AV-scan retry,
  os.replace-over-running-exe) — that is Phase B.
- Any real Windows install / `go test` execution on a Windows host — that is Phase C.
- Removing the legacy shell launcher `bin/ws-mcp-launcher` — tracked separately;
  here you only make the Windows launcher-probe *skip* it, not delete it.

## Caller-Visible Contract

None changes. Every item is internal Windows conformance. No MCP tool, CLI command,
schema, or output format changes. The Unix behavior must remain byte-for-byte
unchanged (guard this: most fixes are behind `//go:build windows` or a
`runtime.GOOS == "windows"` branch, leaving the Unix code path untouched).

## Hard Constraint (live-host safety — non-negotiable)

Every Windows process termination MUST stay strictly **PID-scoped** — open a process
by PID (Toolhelp32 PPID walk + `OpenProcess`/`TerminateProcess`), never by image
name. Image-name termination (e.g. `taskkill /IM`) or any broad sweep is FORBIDDEN:
the dogfooding host runs a live `claude.exe` and a non-PID-scoped kill could reach
it. Item 6 must reuse the existing PID-scoped walk, not invent a new kill. If you
cannot do a fix PID-scoped, escalate instead of broadening the kill.

## Contract Instructions

All paths are under `agents-plugin-tool/`. Confirm exact line numbers yourself
(they may have drifted); the function names and patterns below are authoritative.

### Item 1 — Windows-correct interrupt-hook command quoting
- File: `internal/wsagent/agent.go`, `interruptHookCommand(root, name string)` and
  `shellQuote(value string)` (~line 2215-2229).
- Problem: `shellQuote` emits POSIX single-quote quoting (`'...'`). The result is
  embedded into a codex hook command string (see `internal/wsagent/codex.go`
  `buildCodexInvocation`, the `hooks.PostToolUse` TOML, ~line 157-164) that codex
  later executes via the OS shell. POSIX single-quote quoting is meaningless to
  Windows `cmd.exe`, so a `root`/`name`/exe path containing spaces breaks the hook.
- Fix: make the quoting platform-aware. Keep the existing POSIX single-quote logic
  for non-Windows. Add Windows-correct quoting (double-quote wrapping with embedded
  double-quotes escaped per Windows command-line rules). Prefer a build-tagged
  helper (e.g. `quoteHookArg` in `hook_quote_windows.go` / `hook_quote_unix.go`)
  so the Unix path is untouched and the Windows quoter is unit-testable as a pure
  function. `interruptHookCommand` should call the platform helper instead of
  `shellQuote` directly. Leave `shellQuote` in place if other callers exist; if it
  is the only caller, you may rename/relocate.
- Note for the report: the exact shell codex uses to run the hook on Windows is
  confirmed empirically in Phase C; target `cmd.exe`-correct double-quoting here.

### Item 2 — codex `model_instructions_file` path is forward-slashed
- File: `internal/wsagent/codex.go` ~line 152:
  `args = append(args, "-c", fmt.Sprintf("model_instructions_file=%q", req.SystemPromptPath))`.
- Problem: `%q` of a Windows path emits `"C:\\Users\\..."`; passing
  backslash-escaped paths through codex's `-c` config value is ambiguous/parser-
  fragile and risks the system prompt being dropped.
- Fix: apply `filepath.ToSlash(req.SystemPromptPath)` before formatting with `%q`,
  so the value is `"C:/Users/.../system.md"` (forward slashes work on Windows and
  carry no escape ambiguity). On Unix `ToSlash` is a no-op, so behavior is unchanged.
- A sibling occurrence of the same format string exists in
  `internal/wsagent/agent_test.go` (~line 685); update the test expectation to match
  the new emission so it stays green (on a Linux test host the path has no
  backslashes, so ToSlash is a no-op there — keep the test meaningful with a
  backslash-bearing input where practical, or add a dedicated Windows-path test).

### Item 3 — Windows async-worker launcher probe must not return the POSIX shim
- File: `internal/wsagent/agent.go`, `cacheLauncherCommand(exe string)` (~line 249)
  and `asyncWorkerCommandFor` (~line 225).
- Problem: `cacheLauncherCommand` probes `bin/ws-mcp-launcher` (the extensionless
  POSIX **shell** launcher, which ships in the plugin dir) BEFORE the
  `ws-mcp-launcher.py` + `python3` branch. On Windows the shell shim is not
  executable, so returning it as the worker command breaks async/mercenary spawn —
  and the working `.py`+python branch is never reached.
- Fix (Windows-correct probe order): on Windows, do NOT select the extensionless
  shell shim. Probe order on Windows: a native `ws-mcp-launcher.exe` if present →
  then `ws-mcp-launcher.py` via `python3`/`python`. On non-Windows keep the current
  order (shell shim first, then `.py`). Use a `runtime.GOOS == "windows"` branch (or
  a small build-tagged helper) localized to the shim-selection step. Do not change
  the `WS_MCP_RUNTIME_BINARY` / current-exe candidate logic or the `LookPath`
  fallback (Windows `LookPath` already resolves `.exe`).
- Add a Windows test exercising the cache layout: given a plugin cache dir
  containing both `ws-mcp-launcher` (shell) and `ws-mcp-launcher.py`, the Windows
  probe must NOT return the extensionless shell path.

### Item 4 — atomic file replacement on Windows
- Files: `internal/wsagent/agent.go` `replaceFile(tmp, path)` (~line 2328) and
  `internal/wsstate/paths.go` `replaceFile(tmp, path)` (~line 348). Both are
  identical remove-then-rename implementations.
- Problem: `os.Rename` over an existing file is non-atomic on Windows and the
  remove+rename fallback fails with `ERROR_SHARING_VIOLATION` when a concurrent
  reader (dashboard / AV scanner) holds the destination.
- Fix: introduce a build-tagged atomic-replace primitive in EACH package
  (`replace_file_windows.go` / `replace_file_unix.go`, matching the existing
  per-package duplication style), e.g. `atomicReplaceFile(tmp, path string) error`:
  - Unix build tag: `return os.Rename(tmp, path)` (already atomic over an existing
    file within the same directory).
  - Windows build tag: `windows.MoveFileEx(utf16(tmp), utf16(path),
    windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)`. On
    `ERROR_SHARING_VIOLATION`, do a small bounded retry (a few attempts with short
    backoff) before returning the error — the transient AV/dashboard hold case.
  - `golang.org/x/sys/windows` is already a module dependency (v0.22.0); use
    `windows.UTF16PtrFromString` for the path args.
  - Rewrite both `replaceFile` bodies to delegate to `atomicReplaceFile`. On Windows
    `MoveFileEx(REPLACE_EXISTING)` handles dest-exists atomically, so the old
    remove+rename dance is no longer needed; keep the Unix path equivalent to today.
- Add a Windows test: replacing an existing destination succeeds and yields the new
  contents.

### Item 5 — `processAlive` ACCESS_DENIED → alive symmetry
- Files (three packages, Windows variants):
  `internal/wsagent/process_windows.go` `processAlive(pid int) (bool, error)`,
  `internal/execjob/process_windows.go` `processAlive(pid int) bool`,
  `internal/wsstate/process_alive_windows.go` `processAlive(pid int) bool`.
- Problem: each treats ALL `OpenProcess` failures as "not alive". The Unix
  counterparts treat `EPERM` (access denied) as alive
  (`err == nil || err == syscall.EPERM`). The Windows side lacks this symmetry: an
  `ERROR_ACCESS_DENIED` from `OpenProcess` means the process exists but is not
  openable at the requested rights — it is alive, not dead.
- Fix: in each of the three, when `OpenProcess` returns an error, check for
  `windows.ERROR_ACCESS_DENIED` and treat that as alive (preserving each function's
  existing signature: the `(bool, error)` one returns `(true, nil)`; the `bool` ones
  return `true`). All other open errors (e.g. `ERROR_INVALID_PARAMETER` = no such
  pid) remain "not alive". Do NOT change the existing post-open
  `WaitForSingleObject(handle, 0)` zombie/liveness probe — that stays exactly as is.
- Add a Windows test asserting the ACCESS_DENIED→alive mapping where feasible (a
  pure helper that classifies an error code is the easiest unit-testable seam; you
  may extract a tiny `openErrorMeansAlive(err) bool` helper to test directly).

### Item 6 — Windows sync-runner context-timeout tree-kill
- File: `internal/wsagent/runner_command_windows.go`, `configureRunnerCommand(cmd)`.
- Problem: the Windows version sets only `CreationFlags: CREATE_NEW_PROCESS_GROUP`
  and no `cmd.Cancel`. When the synchronous runner's context times out
  (`exec.CommandContext` in `codex.go`/`claude.go`), Go kills only the root process,
  leaving children alive. The Unix version
  (`runner_command_unix.go`) sets `cmd.Cancel` to `syscall.Kill(-pgid, SIGKILL)`.
- Fix: set `cmd.Cancel` on Windows to a PID-scoped subtree kill that REUSES the
  existing helper `cancelAsyncProcessTree(pid int)` in
  `internal/wsagent/cancel_process_windows.go` (the same Toolhelp32 PPID-walk the
  async cancel path uses via `Manager.cancelProcessTree`). Mirror the Unix shape:
  guard `cmd.Process == nil` (return nil), else
  `return cancelAsyncProcessTree(cmd.Process.Pid)`. Do NOT write a new kill routine
  and do NOT use any image-name termination (see Hard Constraint).
- A Windows unit test here is hard (needs a real child tree); a compile + vet pass
  plus the existing async-cancel coverage is acceptable. If a lightweight test of
  the wiring is feasible without spawning real subprocess trees, add it; otherwise
  note the deferral to Phase C in the report.

### Item 7 — smoke script uses a removed tool name
- File: `agents-plugin-tool/scripts/smoke-ws-mcp.sh` ~line 53:
  `"params": {"name": "ws.lead.login", ...}`.
- Problem: `ws.lead.login` was renamed; the session-auth tool is now `ws.ferrule`.
  The only manual sanity script must not ship calling a non-existent tool.
- Fix: change the tool name to the current registered name. Verify the exact name
  against the MCP dispatch table in `internal/mcp/server.go` (search for the
  ferrule/session-open tool registration) and use that literal name. It is
  `ws.ferrule` unless the dispatch table says otherwise — confirm, do not assume.

## Integration Test Instructions

- New Windows-only tests go in `*_windows_test.go` files (build tag `windows`) next
  to the code they cover, so they compile and run only on Windows.
- Platform-neutral assertions (item 2 ToSlash behavior, item 1 Unix quoting,
  pure-helper classifiers) go in ordinary `_test.go` so they run on the Linux host.
- Follow existing table-test style in `internal/wsagent/agent_test.go`.

## Implementation Strategy Decisions (do not reopen)

- Keep Unix behavior unchanged; isolate every Windows fix behind a build tag or a
  `runtime.GOOS == "windows"` branch.
- Reuse existing PID-scoped Windows machinery (`cancelAsyncProcessTree`,
  `snapshotWindowsProcesses`, the `WaitForSingleObject` probe). Do not duplicate or
  replace it.
- Use `golang.org/x/sys/windows` (already a dependency) for MoveFileEx and error
  constants; do not add new dependencies.

## Rejected Alternatives

- Image-name / broad process termination for item 6 — forbidden (live-host safety).
- Deleting the legacy shell launcher to "fix" item 3 — out of scope; only skip it on
  Windows.
- Leaving `model_instructions_file` backslash-escaped and "hoping the parser copes"
  (item 2) — `ToSlash` removes the ambiguity cheaply.

## Constraints

- No caller-visible contract change; no new MCP/CLI surface.
- Surgical edits following existing per-package style (the duplicated `replaceFile`,
  the build-tagged `*_windows.go`/`*_unix.go` pairs).
- Do not modify the spec or ticket; the lead owns docs.

## Out of scope

- Phase B (launcher cold-load) and Phase C (real Windows install + run).
- Legacy shell-launcher removal.

## Verification Contract

This host is Linux/WSL2; Windows-tagged code cannot be *run* here, only
cross-compiled. Required before reporting completion:

1. Native (Linux) — must pass:
   - `cd agents-plugin-tool && go build ./...`
   - `cd agents-plugin-tool && go test ./...` (covers platform-neutral changes:
     item 2, item 1 Unix branch, pure helpers)
   - `cd agents-plugin-tool && go vet ./...`
2. Windows cross-compile — must pass (catches all `//go:build windows` code,
   including new Windows tests):
   - `cd agents-plugin-tool && GOOS=windows GOARCH=amd64 go build ./...`
   - `cd agents-plugin-tool && GOOS=windows GOARCH=amd64 go vet ./...`
   - `cd agents-plugin-tool && GOOS=windows GOARCH=amd64 go test -c ./internal/wsagent ./internal/execjob ./internal/wsstate -o /dev/null`
     (compiles the Windows test binaries — including the new `*_windows_test.go` —
     without executing them)
3. Actual execution of the Windows tests runs on a Windows host and is part of
   Phase C acceptance — not required on this host. Report which assertions are
   Windows-execution-deferred.

Read full output of every command; never claim pass without reading it. Diagnose
test-vs-impl blame before any fix.

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/mental-model/named-agent-runtime.md` - [Must] Common Mistakes section
  documents the exact Windows cancel (Toolhelp32 PPID walk, PID-scoped only) and
  `processAlive` `WaitForSingleObject` invariants you are conforming to.
- `ai-docs/mental-model/plugin-runtime.md` - [Maybe] launcher/async-worker context
  for item 3.
- `agents-plugin-tool/internal/wsagent/cancel_process_windows.go` - [Must] the
  existing `cancelAsyncProcessTree` to reuse for item 6.
- `agents-plugin-tool/internal/wsagent/runner_command_unix.go` - [Must] the Unix
  `cmd.Cancel` shape to mirror for item 6.
- `agents-plugin-tool/internal/execjob/process_windows.go` - [Maybe] reference
  implementation of a PID-scoped tree kill in the sibling package.
- `agents-plugin-tool/internal/wsstate/process_alive_unix.go` - [Must] the Unix
  `EPERM`→alive logic item 5 mirrors.
