# Plan: 260724-bug-windows-mcp-mid-session-disconnect — Phase 3: Windows process-lifecycle hardening

## Relevant Ticket Contract

- Assign the Go child to a Windows Job Object with kill-on-close so terminating
  the launcher deterministically reaps the server (eliminates orphans/stale
  locks — blocks hypothesis A), and/or add server-side parent-death detection
  so an orphaned serve process self-terminates (ticket Phase 3, `:216-220`).
- Full removal of the intermediate Windows process is likely infeasible (no
  true `exec` on Windows), so Job Object is framed as "the pragmatic path"
  (ticket `:219-220`) — this survey re-tests that framing against source
  rather than accepting it outright (see Design Fork below).
- Repro-dependent; validate locally via `powershell.exe` interop from WSL (real
  Windows process spawning), with a Windows CI runner for automated regression
  (ticket `:221-222`, Constraints `:89-93`).
- Launcher edits must be applied to `agents-plugin/bin/ws-mcp-launcher.py` and
  kept byte-identical with the `agents-plugin-wsflow` mirror (Constraints
  `:94-95`).
- Hypothesis A (orphaning) is already reclassified in the ticket Decisions
  (`:80-85`) as an **amplifier** (stale locks break the *next* connect), not
  the disconnect trigger itself (that is Phase 1's now-fixed panic). Phase 3
  is robustness/cleanup work, not a confirmed-cause fix — same posture as
  Phase 4.
- Spec Impact target: `plugin-runtime.md` (launcher/serve diagnostics) for the
  Windows lifecycle behavior; `mcp-tools.md`'s existing
  `{#260724-serve-request-panic-resilience}` anchor is Phase 1/2 territory, not
  this phase's primary target (ticket `:97-105`).

This document is a **survey and implementation plan only**. No source files
were modified while producing it.

## Out of Scope

- Phase 4 (SQLite point-read retry + WAL re-assert, `store.go:632,789,860,884`)
  — separate phase; this survey only determines whether
  `orchestrator_lock.go`'s existing `processAlive` pattern is reusable
  plumbing for Phase 3, not whether the lock itself gets wired up (that
  remains Phase 4's "evaluate wiring the already-present but unused
  `orchestrator_lock.go`" item, ticket `:226-230`).
- Any change to the POSIX `os.execvpe` branch — the Go binary IS the
  Claude-Code-supervised PID on POSIX (Verified Findings `:29-32`), so there is
  no orphan class to fix there; must stay byte-for-byte unchanged.
- Rewriting `ServeStdio`'s read loop to be preemptible via `context.Context`
  (e.g. wrapping `os.Stdin` in a cancellable reader). A real gap was found in
  this survey (see Codebase Findings) but reworking the read loop is a bigger,
  separate change than "process-lifecycle hardening" and is called out as a
  known limitation, not undertaken here.
- Consolidating the duplicated `processAlive`/`openErrorMeansAlive` functions
  that already exist near-verbatim in `internal/wsstate/process_alive_windows.go`
  and `internal/execjob/process_windows.go` (and presumably `internal/wsagent`
  too, per `process_alive_windows_test.go` in all three packages). Noted as
  pre-existing debt; out of scope to refactor here.
- Actually implementing the Job Object / parent-death-detection code. This
  ticket phase requires the design fork resolved and (per the ticket's own
  "Repro-dependent" framing) empirical `powershell.exe` validation before
  committing to a mechanism; this document plans that work, it does not do it.

## Codebase Findings

### Launcher constraints (Python, `agents-plugin/bin/ws-mcp-launcher.py`)

- `ws-mcp-launcher.py:2-13` — imports are `hashlib, json, os, platform, shutil,
  subprocess, sys, tempfile, time, uuid, urllib.request, pathlib`. **No
  `ctypes`, no `win32api`/`pywin32` anywhere in the file** (confirmed by a full
  grep for `ctypes|win32|pywin32` — zero hits). Any Windows Job Object call
  from the launcher must go through `ctypes.windll.kernel32.*` — this is a
  **new capability**, not an extension of an existing pattern in this file.
- `ws-mcp-launcher.py:884-891` — current handoff block (line numbers shifted
  from the ticket's `:866-869` by Phase 2's `write_exit_breadcrumb` insertion):
  ```python
  args = [str(binary), *sys.argv[1:]]
  if os_name == "windows":
      exit_code = subprocess.call(args)
      if exit_code != 0:
          write_exit_breadcrumb(exit_code)
      return exit_code
  os.execvpe(str(binary), args, os.environ)
  return 1
  ```
  `subprocess.call` **spawns and blocks until completion in one call** — it
  never exposes a PID/handle mid-flight. Attaching a Job Object requires
  switching to `subprocess.Popen(args)`, capturing `proc.pid` (or `proc._handle`,
  an undocumented-but-stable CPython-on-Windows attribute) immediately after
  spawn, calling `AssignProcessToJobObject`, then `proc.wait()` in place of
  `.call()`'s combined behavior — a **structural rewrite of the Windows
  handoff branch**, not an additive change.
- `CREATE_SUSPENDED` is **not required**: `AssignProcessToJobObject` can attach
  an already-running (non-suspended) process to a job as long as the process
  isn't already in another job. The race window between `Popen()` returning
  and the assignment call executing is real but narrow, and for this specific
  child (`ws-mcp serve`, a single Go binary with no OS-level forking at
  startup) the practical risk is low. `CREATE_SUSPENDED` + manual `ResumeThread`
  would close the race fully but needs the primary thread handle, which
  `subprocess.Popen` does not expose (only `_winapi.CreateProcess` at the
  private-module level does) — meaningfully more invasive for marginal benefit
  given the low-risk profile of this specific child.
- `diff agents-plugin/bin/ws-mcp-launcher.py agents-plugin-wsflow/bin/ws-mcp-launcher.py`
  — **zero differences**, both 894 lines. Confirmed byte-identical as of this
  survey; any Job Object edit must be mirrored into both files identically,
  same as Phase 2.

### Server-side option (Go, `agents-plugin-tool/`)

- `agents-plugin-tool/go.mod:7` — `golang.org/x/sys v0.22.0` is **already a
  direct (non-indirect) module dependency**. The ticket's own framing doesn't
  flag this either way; it is worth stating explicitly that adopting
  `golang.org/x/sys/windows` for Job Object or parent-death APIs is **not** a
  new cross-module dependency — it is already imported in 9 files across
  `internal/wsagent`, `internal/wsstate`, `internal/execjob` (confirmed via
  `grep -rn "golang.org/x/sys"`).
- `internal/wsstate/process_alive_windows.go:1-38` and
  `internal/execjob/process_windows.go:18-51` — **near-identical** existing
  `processAlive(pid int) bool` / `openErrorMeansAlive(err error) bool`
  implementations already using exactly the primitives a parent-death watcher
  needs: `windows.OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION|SYNCHRONIZE, ...)`
  + `windows.WaitForSingleObject(handle, 0)` (poll) or blocking wait. This is
  directly reusable/adaptable plumbing, not a new pattern for this codebase.
- `golang.org/x/sys@v0.22.0/windows` (module cache, confirmed present):
  `CreateJobObject` (`zsyscall_windows.go:1764`), `AssignProcessToJobObject`
  (`:1642`), `SetInformationJobObject` (`:3173`), `JOBOBJECT_EXTENDED_LIMIT_INFORMATION`
  and `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` (`types_windows.go:2210/2230`), and
  `JOB_OBJECT_LIMIT_BREAKAWAY_OK` / `JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK`
  (`:2206/2216`) are all present in the vendored version. A **Go-side** Job
  Object implementation is equally feasible to a Python/ctypes one, and would
  avoid the launcher's ctypes-struct-layout risk entirely if the job were
  instead created and owned by the Go process itself — see Design Fork.
- `internal/mcp/server.go:142-208` (`ServeStdio`) — the read loop is
  `for scanner.Scan() { select { case <-ctx.Done(): ...; default: } ... }`.
  **The `ctx.Done()` check only runs between successfully-scanned lines**;
  `bufio.Scanner.Scan()` itself blocks on a synchronous `Read` against `in`
  (`os.Stdin` in production) with no cancellation hook into `ctx`. **Canceling
  `ctx` from a parent-death-watcher goroutine will NOT unblock a `Scan()` call
  that is currently blocked waiting for the next request** — which is the
  normal idle state of an MCP server between tool calls. This directly
  contradicts an implicit assumption in the ticket phrasing ("self-terminate")
  that a graceful, context-based shutdown is available; in the current code,
  a parent-death detector's realistic action is `os.Exit(...)`, not a graceful
  return from `ServeStdio`. This is functionally as abrupt as an OS-level
  Job-Object kill (skips all Go defers) — the "cleaner shutdown" framing for
  option (b) below is weaker than it first appears.
- `cmd/ws-mcp/main.go:88-104` (`serve()`) and `main.go:106-147` (`runSmoke`) —
  `ServeStdio` is called from three call sites: production `serve()`, the
  `smoke` self-test, and every test in `internal/mcp/*_test.go` (e.g. the
  Phase 1 `TestServeStdioRecoversPanicAndPersistsCrashTrace`). **A parent-death
  watcher must NOT be started unconditionally inside `Server.ServeStdio`** —
  doing so would arm a live `os.Getppid()`-tracking goroutine inside every
  test and the `smoke` run too. On a Windows CI runner, if the test process's
  own ancestor (shell/test-runner) happens to exit or get reparented mid-run
  (a normal CI process-tree event, not a bug), a naively-placed watcher could
  spuriously fire `os.Exit()` mid-test — a real flakiness trap this survey
  found and future implementation must avoid. The safe placement is an opt-in
  parameter (e.g. an `ServeStdioOptions.EnableParentDeathWatch` or a watcher
  started explicitly by `cmd/ws-mcp/main.go`'s `serve()` only, not inside the
  shared `ServeStdio` method).
- `internal/mcp/server.go:270-306` region — `appendDebugEvent` and Phase 1's
  `crashLogPath()`/`recordPanic()` helpers (added under
  `{#260724-serve-request-panic-resilience}`) are direct, already-built
  infrastructure a parent-death breadcrumb can reuse (e.g. a
  `"process.parent_exited"` event mirrored to the same always-on
  `<cache-root>/crash/` directory or the debug-event ring buffer) rather than
  inventing a new sink.
- `internal/wsstate/orchestrator_lock.go:28-90` (`AcquireOrchestratorLock`) —
  confirmed **zero non-test callers** (`grep -rn "AcquireOrchestratorLock"`
  only matches its own definition and `paths_test.go`). It locks a per-worktree
  `orchestrator.lock` file (`WorktreeLocksDir`) keyed by PID + `processAlive`
  staleness recovery (`:56-61`) — this is a **different lock** than the
  `state.sqlite` WAL/`-shm` files the ticket's hypothesis-A stale-lock concern
  is about, and is unrelated to MCP-server-vs-launcher process lifecycle. It
  is Phase 4 territory (as the ticket already scopes it, `:226-230`), not
  Phase 3; the only thing Phase 3 borrows from it is the **pattern** (PID
  staleness via `processAlive`), already independently available from
  `process_alive_windows.go`.

### The mercenary-survives-disconnect regression risk (not mentioned in the ticket)

This is the most significant finding of this survey and is not stated in the
ticket's Phase 3 text at all:

- `internal/wsagent/async_command_windows.go:1-13` and
  `internal/wsagent/runner_command_windows.go:1-22` — async mercenary worker
  processes (`SelfWorkerStarter.StartAsyncCall`, `agent.go:186-215`) and
  synchronous runner subprocesses are spawned via `exec.Command` with
  `cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}`
  only. **No `CREATE_BREAKAWAY_FROM_JOB` flag is set anywhere.**
  `CREATE_NEW_PROCESS_GROUP` governs console Ctrl+C/Break signal scope; it has
  **no effect on Windows Job Object membership**.
- `ai-docs/mental-model/mcp-runtime.md:12` and the `ws.mercenary.*` surface
  (`main.go:1023`: `follow_up: ws.mercenary.result | ws.mercenary.wait | ...`)
  establish that mercenary agents are **designed to keep running independently
  across MCP disconnects/reconnects** — a user disconnects, later reconnects,
  and checks on the async agent via `mercenary.wait`/`mercenary.result`/
  `mercenary.check-inbox`. This is a load-bearing product behavior, not an
  incidental detail.
- On Windows, child processes automatically inherit their parent's Job Object
  membership unless the job was created with `JOB_OBJECT_LIMIT_BREAKAWAY_OK`
  (or `_SILENT_BREAKAWAY_OK`) **and** the child is spawned with
  `CREATE_BREAKAWAY_FROM_JOB`. **If the `ws-mcp serve` process is assigned to a
  kill-on-close job (option (a) below) without this companion change, every
  mercenary async worker/runner subprocess it spawns after that point would
  automatically become a member of the same job — and would be forcibly
  killed the moment the launcher dies and kill-on-close fires.** This would
  silently break the "agent survives disconnect" contract for any mercenary
  call in flight at the moment of a Windows MCP disconnect — a real regression,
  not a hypothetical edge case, since disconnects are exactly the scenario
  this ticket is about.
- `CREATE_BREAKAWAY_FROM_JOB` (`0x01000000`) is **not exposed as a named
  constant in Go's stdlib `syscall` package on Windows** (confirmed by grep
  against the local Go toolchain's `syscall` sources) — it would need to be
  defined locally (a raw untyped int constant), the same way
  `syscall.CREATE_NEW_PROCESS_GROUP` is already used from stdlib elsewhere in
  this codebase. This is a small, well-precedented addition, not a blocker.
- **Any implementation of option (a)/(c) below MUST, as a mandatory companion
  change, set `JOB_OBJECT_LIMIT_BREAKAWAY_OK` on the job's
  `JOBOBJECT_EXTENDED_LIMIT_INFORMATION.BasicLimitInformation.LimitFlags` and
  add `CREATE_BREAKAWAY_FROM_JOB` to the `CreationFlags` in
  `internal/wsagent/async_command_windows.go`, `runner_command_windows.go`,
  and `internal/execjob/process_windows.go:16`** (the `exec.*` MCP tool family
  also spawns arbitrary long-lived Windows subprocesses that should not be
  tied to the MCP server's own lifetime either). Option (b) (server-side
  parent-death detection, no Job Object) does **not** have this risk at all:
  `os.Exit()` in the parent Go process has zero effect on independently
  `Start()`-ed OS child processes.

### Existing Windows CI coverage

- `.github/workflows/ws-mcp-release.yml:89-115` — a real `windows-smoke` job
  (`runs-on: windows-latest`) already exists, triggered on `pull_request` for
  paths touching `agents-plugin-tool/**` (and others), tag pushes, and
  `workflow_dispatch`. **It currently only builds the binary and runs
  `ws-mcp smoke --root`; it does not run `go test ./...` on Windows at all.**
  The ticket's "Windows CI runner for automated regression" is therefore
  partially existing infrastructure (a Windows runner is already wired up) but
  needs a new step, not a new job, to actually execute Go tests on Windows.

## Design Fork

**(a) Launcher-side Job Object + kill-on-close only**

- Feasibility: real but requires new `ctypes` Win32 plumbing in a file that
  has never used `ctypes`/pywin32 (struct definitions for
  `JOBOBJECT_EXTENDED_LIMIT_INFORMATION`, `CreateJobObjectW`,
  `SetInformationJobObject`, `AssignProcessToJobObject` calling conventions,
  x64 struct alignment) and a structural rewrite of the handoff block
  (`subprocess.call` → `Popen` + assign + `wait`).
- Complexity: medium-high, and it is the **riskiest of the three to get right
  the first time** because it cannot be exercised on this dev box at all (see
  Validation Plan) — `ctypes.windll` does not exist under CPython on
  Linux/WSL, so no amount of local iteration proves the struct layout or call
  sequence correct; correctness is only provable on a real Windows Python
  interpreter (CI or a Windows box), not via `powershell.exe` interop from
  WSL (that spawns Windows-native *processes*, but cannot host the Linux
  Python interpreter running the launcher script itself).
- Cross-platform safety: fully `if os_name == "windows":` gated; POSIX path
  untouched.
- Blocks hypothesis A: **yes, most deterministically** — kill-on-close is a
  kernel-level guarantee, independent of stdio/pipe semantics, and (with the
  breakaway companion fix) also reaps any Go-spawned subprocess that should
  NOT survive (e.g. stray `exec.*` MCP tool children), not just the Go server
  itself. Mandatory companion fix: `CREATE_BREAKAWAY_FROM_JOB` for mercenary
  workers (see above) — without it, this option is a functional regression,
  not just a robustness gain.

**(b) Server-side parent-death detection only**

- Feasibility: high — reuses the already-vendored `golang.org/x/sys/windows`
  dependency and an existing, nearly-copy-pasteable pattern
  (`process_alive_windows.go`'s `OpenProcess`/`WaitForSingleObject`). Capture
  `os.Getppid()` (confirmed real, non-stub on Windows in the Go toolchain's
  `syscall` package) once at `serve()` startup, open a `SYNCHRONIZE` handle,
  block on `windows.WaitForSingleObject(handle, windows.INFINITE)` in a
  goroutine, and `os.Exit(...)` (with a breadcrumb write via Phase 1/2's
  existing crash-log infra) when it returns.
- Complexity: low, and — per the finding above — **must not be wired
  unconditionally inside `Server.ServeStdio`** (test/smoke flakiness risk);
  must be started only from `cmd/ws-mcp/main.go`'s `serve()`.
- Cross-platform safety: `//go:build windows` file + a `//go:build !windows`
  no-op stub, mirroring the existing `process_alive_windows.go`/
  `process_alive_unix.go` split. Zero risk to POSIX.
- Blocks hypothesis A: **yes, for the specific complaint** (an orphaned
  `ws-mcp serve` process holding a stale `state.sqlite` lock) — reaping the
  Go process itself is exactly what prevents the *next* connection's lock
  contention. It does **not** provide a Job Object's transitive-subtree
  guarantee, but per the finding above, that is actually the **correct**
  behavior for mercenary workers (they should NOT be reaped), so this is not
  really a gap for this specific ticket's hypothesis A, only for any
  as-yet-unidentified non-mercenary grandchild. `os.Exit()`'s abruptness
  (skips defers) is no worse than a Job Object kill in practice, since the
  per-operation SQLite `Manager.Open`/`Close` model (Phase 1 finding,
  `store.go:181`) already means there is normally no open `*sql.DB` handle at
  the moment of an idle-loop parent-death detection to begin with.

**(c) Both, sequenced B then A**

- Rationale: (b) is low-risk, fully testable end-to-end today (unlike (a)),
  reuses proven code, and independently closes the ticket's confirmed
  amplifier (orphaned server → stale lock) without the mercenary-breakaway
  risk. (a) adds a kernel-guaranteed backstop for the case (b) cannot cover —
  the launcher being force-killed (crash, AV, Task Manager "End task") in a
  way that either races ahead of, or entirely bypasses scheduling for, the
  Go-side watcher goroutine. Whichever mechanism's kill reaches the process
  first "wins" the race; both outcomes are acceptable (Job Object's hard kill
  is equivalent to today's any-other forced kill, which WAL already tolerates
  modulo Phase 4's stale-`-wal` cleanup).
- Given (a)'s cost/verifiability profile, it should not be built to the same
  timeline as (b) without the empirical `powershell.exe` experiments below
  actually running first — see Escalations.

**Recommendation: (c), split into 3a (server-side detection, ship first) and
3b (launcher Job Object, ship only if 3a's field/experiment evidence still
shows real orphans).** Rationale: 3a is cheap, safe, fully verifiable locally
today, and directly answers the ticket's confirmed hypothesis-A complaint; 3b
carries a nontrivial, currently-unverifiable-on-this-box implementation risk
(ctypes struct correctness) and a real regression risk (mercenary breakaway)
that both need to be justified by evidence that 3a alone is insufficient,
not assumed from the ticket's "pragmatic path" framing alone.

## Validation Plan

### Local `powershell.exe`-from-WSL experiments (run before finalizing 3b's necessity)

1. **Job Object mechanism sanity check** (validates the raw OS guarantee,
   independent of our Python/Go code): from WSL, run
   `powershell.exe -NoProfile -Command <script>` where the script uses
   `Add-Type` to inline a minimal C# helper calling `CreateJobObject`,
   `SetInformationJobObject` (with `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE`), and
   `AssignProcessToJobObject` against a spawned long-lived child (e.g.
   `Start-Process powershell -ArgumentList '-Command','Start-Sleep -Seconds
   300'`); then terminate the parent PowerShell process itself (or have a
   second control script `Stop-Process -Force` it) and poll
   `Get-Process -Id <childPid> -ErrorAction SilentlyContinue` to assert the
   child is gone shortly after. This proves the kernel mechanism works on the
   actual Windows runtime without needing the real launcher/ctypes code yet.
2. **The stdin-EOF-masking crux experiment** (directly answers "is
   server-side detection even needed"): from WSL, construct a 3-tier process
   chain that mirrors the real topology — a top-level script holding the
   write end of a piped stdin (playing "Claude Code"), a middle process with
   **inherited, non-redirected** stdio spawned via a mechanism equivalent to
   `subprocess.call(args)`'s default handle inheritance (playing the
   launcher), and a grandchild that blocks reading stdin in a loop (playing
   `ws-mcp serve`). Force-kill **only the middle process**
   (`taskkill /F /PID <middlePid>`, mirroring "Claude Code kills the immediate
   child" without closing its own top-level write handle) and observe whether
   the grandchild sees stdin EOF and exits on its own, or keeps blocking
   (still alive). This experiment's result is the deciding evidence for
   whether 3b (or even 3a) is empirically necessary versus already-organic
   stdin-EOF propagation being sufficient once Phase 1/2 are in place.

### Automated regression coverage (given Windows-only behavior)

- **Launcher (Python) orchestration test — CI-safe on Linux**: refactor the
  Job Object call sequence behind an injectable seam (e.g. module-level
  `create_job_object()`, `assign_process_to_job(job, pid)`,
  `set_kill_on_close(job)` functions) so a test can monkeypatch them and
  assert: `subprocess.Popen` (not `.call`) is used, assignment happens before
  `.wait()`, and the limit-flags value passed to `SetInformationJobObject`
  includes both `JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE` and
  `JOB_OBJECT_LIMIT_BREAKAWAY_OK`. This tests the **orchestration**, not the
  real OS call (which cannot execute under Linux CPython — `ctypes.windll`
  does not exist there), and should follow the existing `load_launcher()` /
  full-`main()`-invocation idiom from `test_ws_mcp_launcher_capabilities.py`.
- **Go server-side watcher test — Windows-only, `//go:build windows`**:
  `internal/mcp/parent_watch_windows_test.go` spawns a short-lived real helper
  process (e.g. `cmd /c exit 0`), points the watcher at that PID, and asserts
  the exit callback fires within a bounded timeout. This test **cannot run on
  Linux CI** (build-tag excluded), matching the existing precedent of
  `process_alive_windows_test.go` already coexisting with that constraint.
- **CI wiring**: add a `go test ./...` (or a narrower Windows-tagged subset)
  step to the existing `windows-smoke` job in
  `.github/workflows/ws-mcp-release.yml:89-115` — today that job only runs
  `ws-mcp smoke`, so any new `_windows_test.go` file added by this phase would
  otherwise never execute anywhere automated.
- **What CANNOT be covered in Linux CI, stated plainly**: the real Job Object
  kernel behavior, the real `ctypes` struct/calling-convention correctness,
  and the real stdin-handle-inheritance/EOF-propagation behavior across the
  Python launcher → Go child boundary are all genuinely Windows-only and
  require either the `powershell.exe`-from-WSL experiments (mechanism-level,
  can run today) or an actual Windows run of the modified launcher/binary
  (behavior-level, needs the `windows-smoke` CI job extended or a manual
  Windows session) — this dev environment cannot prove launcher-side ctypes
  correctness by itself under any amount of local iteration.

## Spec/Doc Impact

- `ai-docs/spec/plugin-runtime.md` — the existing
  `{#260505-windows-plugin-managed-startup}` and
  `{#260724-launcher-abnormal-exit-breadcrumb}` anchors (`:198`, `:295`) are
  the natural home for a new anchored paragraph documenting the Windows
  process-lifecycle guarantee once 3a/3b land (e.g.
  `{#260724-windows-process-lifecycle-hardening}`): what happens to the Go
  server when the launcher dies, and — critically, since it is a caller-visible
  guarantee — that mercenary async workers are *not* reaped by this mechanism
  (must be stated explicitly given the regression risk found above).
- `ai-docs/spec/mcp-tools.md` — no new anchor needed for this phase; the
  existing `{#260724-serve-request-panic-resilience}` anchor is Phase 1/2's,
  and Phase 3 does not change the JSON-RPC-visible protocol surface.
- `ai-docs/mental-model/plugin-runtime.md` — the `## Common Mistakes` section
  (`:62-75`) should gain an entry once implemented: assuming a Job Object
  assigned to `ws-mcp serve` doesn't need `JOB_OBJECT_LIMIT_BREAKAWAY_OK` +
  `CREATE_BREAKAWAY_FROM_JOB` on mercenary worker spawns — this is exactly the
  kind of non-obvious, source-verified gotcha that section exists to record.
- `ai-docs/mental-model/mcp-runtime.md` — no update needed for the server-side
  option beyond noting (if 3a ships) that `ServeStdio`'s shutdown path is
  stdin-EOF **or** (Windows-only) parent-process-exit, and that the `ctx.Done()`
  check inside the read loop does not preempt a blocked `Scan()` (a fact worth
  recording so a future change doesn't assume graceful ctx-cancellation works
  there today).

## Escalations

- `[escalate-to-binding-decision]` **Whether to build 3b (launcher Job Object)
  at all, or ship 3a only and revisit.** This hinges on two things this survey
  cannot resolve from source alone: (1) the actual Windows runtime behavior of
  stdin-handle inheritance across the Python-launcher → Go-child boundary when
  only the immediate launcher process is force-killed (the crux experiment
  above must actually run on real Windows to answer this — I have reasoned
  through the Windows pipe/handle semantics but not observed them); and (2)
  whether the project is willing to accept 3b's implementation risk (new,
  locally-unverifiable `ctypes` Win32 struct/calling-convention code in a
  previously pure-stdlib launcher) for a backstop that may turn out to be
  redundant with 3a. **My recommended default: ship 3a (server-side detection)
  first as its own change, run both `powershell.exe` experiments from the
  Validation Plan against the real 3a behavior, and only proceed to 3b if that
  evidence shows orphans/stale locks still occur** (e.g., the launcher gets
  force-killed by something that also kills or races ahead of the Go
  watcher). Do not build 3b speculatively alongside 3a in the same change.
