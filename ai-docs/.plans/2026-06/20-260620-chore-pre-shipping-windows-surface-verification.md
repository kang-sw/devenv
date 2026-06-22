# Survey: 260620-chore-pre-shipping-windows-surface-verification

## Reusable Components

- `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L57-L77` — `unixProcessTree(rootPID int)`: enumerates process tree via `ps` command and recursively walks parent-child relationships; reusable for Windows counterpart to enumerate child processes for tree kill.

- `agents-plugin-tool/internal/execjob/process_unix.go#L60-L88` — `processTree(root int)`: identical pattern to wsagent's tree enumeration, returns `procInfo` with PID/PPID/PGID; both paths already have identical logic, no duplication needed if extracted to shared helper.

- `agents-plugin-tool/internal/execjob/process_unix.go#L58-L58` — `procInfo` struct: compact `{pid, ppid, pgid int}` type used in execjob path; wsagent uses `unixProcess` with identical fields — consider unifying if Windows helper is created.

- `agents-plugin-tool/internal/wsagent/async_command_windows.go#L10-L14` — `configureAsyncCommand(cmd *exec.Cmd)`: already sets `CREATE_NEW_PROCESS_GROUP` on Windows spawn; confirms process groups are set up at spawn time, ready for group-scoped kill.

- `agents-plugin-tool/internal/execjob/process_windows.go#L11-L13` — `configureCommand(cmd *exec.Cmd)`: identical `CREATE_NEW_PROCESS_GROUP` setup in execjob path; both async and execjob use the same group creation flag.

- `agents-plugin-tool/internal/wsagent/runner_command_windows.go#L10-L14` — `configureRunnerCommand(cmd *exec.Cmd)`: runner also uses `CREATE_NEW_PROCESS_GROUP` on Windows; all three spawn paths (async, runner, execjob) already create process groups.

## Existing Patterns

- **Process-group setup on spawn:** Unix sets `Setpgid: true` in `SysProcAttr` (async_command_unix.go#L11, runner_command_unix.go#L11, execjob process_unix.go#L13). Windows already sets `CREATE_NEW_PROCESS_GROUP` in `SysProcAttr.CreationFlags` (async_command_windows.go#L12, runner_command_windows.go#L12, execjob process_windows.go#L12). **All three spawn paths have group setup ready.**

- **Unix cancel pattern (tree kill):** cancel_process_unix.go#L19-L55 enumerates tree via `ps`, collects process IDs and group IDs, then kills both group (negative PID signal) and individual processes with `syscall.Kill(-pgid, SIGKILL)` + `syscall.Kill(pid, SIGKILL)`, skipping the current process group to avoid self-kill.

- **Unix cancel pattern (runner-specific):** runner_command_unix.go#L12-L17 sets up a direct `cmd.Cancel` hook that calls `syscall.Kill(-cmd.Process.Pid, SIGKILL)` on the process's own group ID — a simplified per-runner kill, complementary to the tree enumeration in `cancelAsyncProcessTree`.

- **Test pattern (real process spawn):** execjob_test.go#L232-L257 uses `TestHelperProcess` self-exec pattern: calls `exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", <action>)` with env var `GO_WANT_HELPER_PROCESS=1` to conditionally spawn a real child process within the test binary itself. Actions block on time.Sleep (flow, slow, large).

- **Test pattern (polling with deadline):** execjob_test.go#L91-L93 polls `Status()` in a loop until `ResultReady` or deadline `time.Now().Add(3 * time.Second)` expires, with 100ms sleep between polls. No fixed sleep-only patterns.

## Relevant Interfaces

- `agents-plugin-tool/internal/wsagent/agent.go#L2164-L2172` — `Manager.cancelProcessTree(pid int)`: dispatcher that calls `cancelAsyncProcessTree(pid)` (or `m.opts.ProcessCancel` if overridden); the public entry point for mercenary cancel flow, called with the worker's stored PID.

- `agents-plugin-tool/internal/execjob/execjob.go#L270, #L272` — `cancelProcess()` called from within the abort handler; line 270 uses active job's `cmd.Process.Pid`, line 272 uses the stored record PID as fallback.

- `agents-plugin-tool/internal/wsagent/process_windows.go#L7-L19` — `processAlive(pid int) (bool, error)`: Windows-specific liveness probe using `os.FindProcess(pid)` (imperfect, reports process found even if exited — matches the zombie/handle-reported-as-alive issue in brief); Unix version uses `syscall.Kill(pid, 0)` probe.

- `agents-plugin-tool/internal/wsstate/process_alive_windows.go#L9-L16` — `processAlive(pid int) bool`: alternative Windows liveness using `syscall.OpenProcess(0x1000, ...)`, also imperfect (same zombie issue); called from orchestrator_lock.go#L56 to check if existing orchestrator process is still alive before claiming lock.

- `agents-plugin-tool/internal/wsstate/process_alive_unix.go#L7-L10` — `processAlive(pid int) bool`: Unix version uses `syscall.Kill(pid, 0)` (signal 0 = liveness check, no-op kill, returns `EPERM` if alive but not owned, `ESRCH` if dead).

## Constraints

- **Hard constraint (live-host safety):** cancel paths must be PID/job-scoped only; image-name kills (`taskkill /IM`) strictly forbidden (live `claude.exe` risk noted in brief).

- **Existing process-group setup already in place:** All three spawn paths (wsagent async/runner, execjob) already create Windows process groups (`CREATE_NEW_PROCESS_GROUP`). The Windows cancel code must leverage this group setup for subtree kill (e.g., via `GenerateConsoleCtrlEvent` on the group, or direct child enumeration if process-group signal not available on Windows).

- **Process-group kill on Windows is not standard `syscall.Kill(-pgid, sig)`:** Windows lacks Unix's negative-PID group-signal semantics. Alternative mechanisms:
  1. `GenerateConsoleCtrlEvent(CTRL_C_EVENT, pid)` broadcasts to process group (requires PID, sends to group).
  2. Manual child enumeration + individual kill (mimic Unix tree logic).
  3. Windows Job Object (requires additional spawn-time setup, more complex).
  4. `taskkill /T /PID <root>` (simple, but need to verify it's PID-scoped and not image-name at risk).

- **No caller-visible contract change:** cancel already best-effort; `cleanup_needed` signal retained when tree reaping is incomplete.

- **Test determinism required:** no fixed `time.Sleep` waits; use polling with deadline.

- **Exec-surface audit rule:** do not migrate stream payloads to SQLite (mentioned in brief; metadata stays in `wsstore.ExecJob`, bytes in job-owned files).

## Relevant Symbol Locations

### wsagent mercenary cancel (Symbol 1)

- **Main Windows implementation (to fix):** `agents-plugin-tool/internal/wsagent/cancel_process_windows.go#L7-L16` — `cancelAsyncProcessTree(pid int)` currently calls `os.FindProcess(pid).Kill()` (root-only). Must replace with subtree kill.

- **Unix reference (for intent mirroring):** `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L19-L55` — `cancelAsyncProcessTree(pid int)` uses `unixProcessTree(pid)` to enumerate children, then kills by group ID via `syscall.Kill(-pgid, SIGKILL)` + individual PIDs.

- **Unix process-tree helper (reference):** `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L57-L77` — `unixProcessTree(rootPID int)` enumerates via `ps -axo pid=,ppid=,pgid=` and walks children recursively.

- **Unix process parser (reference):** `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L79-L95` — `parseUnixProcessTable(raw string)` and `isNoSuchProcess(err)` helper.

- **Caller context:** `agents-plugin-tool/internal/wsagent/agent.go#L2164-L2172` — `Manager.cancelProcessTree(pid int)` dispatches to `cancelAsyncProcessTree(pid)` with the worker's stored PID (or opts override).

### wsagent spawn-side group setup

- **Unix async spawn:** `agents-plugin-tool/internal/wsagent/async_command_unix.go#L10-L12` — `configureAsyncCommand(cmd)` sets `Setpgid: true`.

- **Windows async spawn:** `agents-plugin-tool/internal/wsagent/async_command_windows.go#L10-L14` — `configureAsyncCommand(cmd)` sets `CREATE_NEW_PROCESS_GROUP`.

- **Unix runner spawn:** `agents-plugin-tool/internal/wsagent/runner_command_unix.go#L10-L18` — `configureRunnerCommand(cmd)` sets `Setpgid: true` + inline `cmd.Cancel` hook with `syscall.Kill(-pid, SIGKILL)`.

- **Windows runner spawn:** `agents-plugin-tool/internal/wsagent/runner_command_windows.go#L10-L14` — `configureRunnerCommand(cmd)` sets `CREATE_NEW_PROCESS_GROUP` (no Cancel hook on Windows).

### execjob abort cancel (Symbol 2)

- **Main Windows implementation (to fix):** `agents-plugin-tool/internal/execjob/process_windows.go#L27-L36` — `cancelProcess(pid int)` currently calls `os.FindProcess(pid).Kill()` (root-only). Must replace with subtree kill.

- **Unix reference:** `agents-plugin-tool/internal/execjob/process_unix.go#L23-L56` — `cancelProcess(pid int)` mirrors wsagent logic: enumerates tree, collects PID/PGID, kills by group + individual.

- **Unix process-tree helper (reference):** `agents-plugin-tool/internal/execjob/process_unix.go#L60-L88` — `processTree(root int)` identical to wsagent pattern, returns `[]procInfo`.

- **execjob spawn-side group setup:**
  - Unix: `agents-plugin-tool/internal/execjob/process_unix.go#L13` — `configureCommand(cmd)` sets `Setpgid: true`.
  - Windows: `agents-plugin-tool/internal/execjob/process_windows.go#L11-L13` — `configureCommand(cmd)` sets `CREATE_NEW_PROCESS_GROUP`.

- **Caller context:** `agents-plugin-tool/internal/execjob/execjob.go#L268-L275` — abort handler calls `cancelProcess(v.(*activeJob).cmd.Process.Pid)` or `cancelProcess(rec.PID)`.

### processAlive implementations (Symbol 3)

- **wsagent Windows:** `agents-plugin-tool/internal/wsagent/process_windows.go#L7-L19` — `processAlive(pid int) (bool, error)` uses `os.FindProcess(pid)`, returns true if process found (imperfect: may report exited processes as alive on Windows if handle is cached).

- **wsagent Unix:** Unix processAlive is injected via Manager options or uses the default; not a separate file (see process_windows.go counterpart).

- **wsstate Windows:** `agents-plugin-tool/internal/wsstate/process_alive_windows.go#L9-L16` — `processAlive(pid int) bool` uses `syscall.OpenProcess(0x1000, false, uint32(pid))` and `CloseHandle()`, same zombie-handle issue as wsagent.

- **wsstate Unix:** `agents-plugin-tool/internal/wsstate/process_alive_unix.go#L7-L10` — `processAlive(pid int) bool` uses `syscall.Kill(pid, 0)`, returns true if `err == nil || err == EPERM` (alive; ESRCH = dead).

- **Caller:** `agents-plugin-tool/internal/wsstate/orchestrator_lock.go#L56` — orchestrator lock acquisition checks `processAlive(existing.PID)` before claiming a stale lock.

## Test Patterns & Idioms

- **Idiomatic helper-process pattern (THIS repo):** `agents-plugin-tool/internal/execjob/execjob_test.go#L232-L257` — `TestHelperProcess()` re-exec trick with `GO_WANT_HELPER_PROCESS=1` env var. Spawn via `exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--", <action>, ...)`. Actions: `"flow"` (short, exits), `"slow"` (6s sleep), `"large"` (big output). Child blocks on time.Sleep internally (no daemon/sentinel file in current tests).

- **Test main setup:** `agents-plugin-tool/internal/wsagent/agent_test.go#L18-L24` — sets `WS_RSRC_ROOT` env var so orientation load works in tests.

- **Polling pattern for async completion:** `agents-plugin-tool/internal/execjob/execjob_test.go#L91-L93` — polls `Status()` in a loop with deadline `time.Now().Add(3 * time.Second)` and 100ms sleep intervals. Do not use fixed sleep alone.

- **Test cleanup:** `execjob_test.go` uses `t.TempDir()` for ephemeral cache/repo. Tests set `WS_CACHE_HOME` via `t.Setenv()`.

- **Test run command:** Standard `go test ./internal/wsagent/...` and `go test ./internal/execjob/...` (or full `go test ./...`).

## Risk Signals

- `agents-plugin-tool/internal/wsstate/process_alive_windows.go#L9-L16` — Possible **liveness-check risk:** Windows `OpenProcess()` may report exited processes as "alive" if the handle hasn't been reaped by the OS yet (brief explicitly flags this as "zombie/exited-handle-reported-as-alive issue"). The fix may need to guard `processAlive` logic or document the limitation in orchestrator_lock.go. This is not a blocker for the cancel fix, but the survey should confirm whether it needs a guard in the Phase 1 verification boundary (Linux only; zombie issue is Windows-specific and deferred for Phase 3 full testing).

- `agents-plugin-tool/internal/wsagent/cancel_process_unix.go#L39` and `agents-plugin-tool/internal/execjob/process_unix.go#L40` — Both Unix paths use `syscall.Kill(-pgid, SIGKILL)` (negative PID for group kill). Windows has no direct equivalent; implementer must choose between `GenerateConsoleCtrlEvent`, manual child enumeration, or Job Object, and validate no image-name kills leak in.

- **Cross-module code duplication:** `unixProcessTree` (wsagent) and `processTree` (execjob) are nearly identical (same ps command, same parsing logic). The fix will add Windows counterparts to both paths. Consider extracting a shared helper to avoid divergence; however, if kept separate for per-module clarity, ensure both stay synchronized.

## Constraints Summary for Implementer

1. **PID/job-scoped kill only.** No `taskkill /IM` or process-name sweeps.
2. **Process groups already created at spawn.** Both async and execjob paths already set `CREATE_NEW_PROCESS_GROUP` on Windows.
3. **Choose Windows subtree-kill mechanism:** 
   - `GenerateConsoleCtrlEvent(CTRL_C_EVENT, pid)` — broadcasts to group, simple, but verify behavior under all scenarios.
   - Manual child enumeration — mimic Unix tree logic, more explicit control, requires PS or Win32 API to enumerate children.
   - Job Object — more complex spawn-side changes, but gives finer group control.
4. **Test determinism:** Use polling with deadline, not fixed sleeps.
5. **Test helper pattern:** Self-exec via `TestHelperProcess` re-exec trick (already idiomatic in this repo).
6. **No contract elevation:** Keep best-effort + `cleanup_needed` signal.
7. **Exec-surface audit:** Do not migrate stream bytes to SQLite.

## Opinion

The codebase is well-structured for this fix. All three spawn paths (wsagent async/runner, execjob) already create Windows process groups via `CREATE_NEW_PROCESS_GROUP`, so the infrastructure is ready. The Unix cancel logic is explicit and documented; Windows needs a group-scoped kill mechanism, and the implementer has three main options with clear trade-offs. The test pattern is idiomatic (self-exec `TestHelperProcess`), and polling-with-deadline is standard. The one duplication risk is between `unixProcessTree` and `processTree` — both implementations are nearly identical and will spawn Windows counterparts; a shared helper could prevent divergence, but keeping them separate for per-module clarity is defensible if both are kept in sync.

The zombie-handle liveness issue in `wsstate/process_alive_windows.go` is flagged in the brief but is deferred to Phase 3; Phase 1 verification is on Linux only, so no guard is needed now (but should be documented for Phase 3).
