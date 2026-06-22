---
title: Pre-shipping verification — Windows surface hardening
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260616-bug-exec-mcp-running-large-abort-flaky-under-full-suite: Phase 2 stabilizes this flaky abort before Windows abort/cancel results are trusted
spec:
  - 260505-agent-cancel-recovery
related-mental-model:
  - named-agent-runtime
---

# Pre-shipping verification — Windows surface hardening

## Background

The playbook-factory pivot epic has landed all core-concept milestones (M0–M4 +
post-M4 children closed). Before shipping, the runtime needs a deliberate
pre-shipping verification pass focused on the **Windows surface**, because the
Unix/Linux surface is continuously dogfooded but Windows is only validated by
post-deploy manual runs.

Baseline (Linux/WSL, 2026-06-20):

- `go test ./...` — all 12 packages pass; `go vet ./...` clean.
- SQLite driver is `modernc.org/sqlite` (pure Go, no CGO) → Windows builds
  need no C toolchain. The largest Windows-build gating risk is absent.

Residual Windows debug surface (the reason for this ticket): six `*_windows.go`
files are build-tagged out of Linux CI and therefore have **0% coverage on
Linux**, and one worktree test explicitly skips on Windows. The untested surface
is concentrated on the **abort / cancel / process-tree / path-layout** axis:

- `internal/wsagent/cancel_process_windows.go`
- `internal/wsagent/process_windows.go`
- `internal/wsagent/async_command_windows.go`
- `internal/wsagent/runner_command_windows.go`
- `internal/execjob/process_windows.go`
- `internal/wsstate/process_alive_windows.go`
- `internal/wsstate/paths_test.go` — `TestLinkedWorktreeShares...` skips on Windows.

The single highest-risk divergence: `cancelAsyncProcessTree`
(`cancel_process_windows.go`) calls `os.FindProcess(pid).Kill()`, which on
Windows terminates only the **root PID**, while the Unix path kills the whole
process group (`syscall.Kill(-pid, SIGKILL)`). A backend runner (Codex/Claude)
that spawns children would leave orphans alive after cancellation on Windows.
This path has no test on any platform.

Goal: shrink the Windows debug surface to the point where a green
`go test ./...` on a Windows host is a meaningful pre-ship signal, and fix the
known cancel-tree divergence.

## Constraints

- The abort/cancel behavior change in Phase 1 must bring Windows into
  conformance with the existing cancel contract (`named-agent-runtime`
  `#260505-agent-cancel-recovery`: best-effort local process cancellation for the
  stored worker pid plus a `cleanup_needed` signal). The fix strengthens Windows
  best-effort coverage (reap the spawned child tree, not only the root pid); it
  does NOT elevate the promise beyond best-effort and introduces no new
  caller-visible contract. The existing anchor is listed in `spec:`; no spec text
  change is expected. If one becomes necessary, route through `lead-write-spec`.
- Process-tree assertions must be deterministic (no sleep-races) — spawn a child
  that blocks on a sentinel, cancel, then assert the child is reaped.
- Windows builds and tests are invoked through WSL2 → Windows interop
  (`cmd.exe` / `powershell.exe` from the WSL2 shell); invoking the Go toolchain
  (`go build` / `go test`) over that interop boundary is the expected execution
  path for the Windows phases.
- **Live-host safety (hard constraint).** The dogfooding WSL2 host runs an active
  `claude.exe` (the harness driving this work). Process-tree termination — in both
  the new test helper and the `cancelAsyncProcessTree` fix — MUST be scoped to the
  test's own spawned subtree by PID or job object. Never terminate by image name
  (`taskkill /IM`, process-name sweeps) or any broad mechanism that could reach
  the live `claude.exe`.

## Phases

Recommended execution order is the phase order below. Note the P-priority labels
map to the pre-ship discussion: Phase 1 = P0, Phase 2 = P2, Phase 3 = P1,
Phase 4 = P3.

### Phase 1: Process-tree cancellation — cross-platform test + Windows fix (P0)

Add a behavioral test that spawns a parent process which itself spawns a child,
drives the cancel path (mercenary cancel and/or `execjob` abort), and asserts the
**entire tree** is terminated. The test must run on both Linux and Windows
(table-driven on `runtime.GOOS` where the spawn helper differs), so it is the
mechanism that finally executes `cancel_process_windows.go` /
`process_windows.go` under a Windows run.

Then fix `cancelAsyncProcessTree` (and `execjob` `cancelProcess`) on Windows to
terminate the child tree, not only the root PID. Options to evaluate:
`CREATE_NEW_PROCESS_GROUP` + `GenerateConsoleCtrlEvent`, `taskkill /T`, or a job
object. Pick the simplest that reliably reaps children created by the runner
backends; record the rejected alternatives. Whichever mechanism is chosen, the
kill MUST be scoped to the spawned root's subtree (PID- or job-scoped, e.g.
`taskkill /T /PID <root>`); image-name termination (`taskkill /IM`) is forbidden
because the dogfooding host runs a live `claude.exe` (see Constraints).

Verification boundary: new test green on Linux; the Windows path is structurally
exercised (asserted on the Windows run in Phase 3). Also review
`processAlive` (Windows) — `OpenProcess`/`FindProcess` can report a zombie/exited
handle as alive; cover or document the recovery-path implication.

### Result (d89e6539) - 2026-06-20

Both Windows cancel paths now reap the whole spawned subtree instead of only the
root PID, mirroring the Unix process-group/tree intent. Contract unchanged
(best-effort + `cleanup_needed`); spec pass found no caller-visible change.

- **Mechanism (chosen):** Toolhelp32 snapshot (`CreateToolhelp32Snapshot` +
  `Process32First/Next`) reads the live PID/PPID table; walk parent→child links
  rooted at the cancel target; `OpenProcess(PROCESS_TERMINATE)` +
  `TerminateProcess` per PID. Strictly PID-scoped, in-process, no spawn-side
  change (the existing `CREATE_NEW_PROCESS_GROUP` suffices). New file
  `wsagent/process_snapshot_windows.go`; `wsagent/cancel_process_windows.go` and
  `execjob/process_windows.go` rewritten.
- **Rejected:** `GenerateConsoleCtrlEvent` (console-bound, catchable, unreliable
  with detached `CREATE_NEW_PROCESS_GROUP` children); `taskkill /T` (shells out,
  depends on `taskkill` presence, harder to unit-test); Windows Job Object
  (cleanest boundary but needs spawn-side assignment across all three spawn
  paths — brief said avoid spawn-side changes).
- **Test:** deterministic cross-platform behavioral tests in both packages
  (`cancel_tree_test.go`) — parent self-execs a child that blocks on a sentinel,
  cancel, then poll-until-dead (no fixed sleeps), following the repo's
  `TestHelperProcess` + `GO_WANT_HELPER_PROCESS=1` idiom. Verified meaningful by a
  throwaway negative mutation (root-only `Kill` leaves the child alive → test
  FAILS; the tree kill → PASSES).
- **Verification:** `go test ./internal/wsagent/... ./internal/execjob/...` green
  on Linux; `go vet` clean on Linux and `GOOS=windows`; `GOOS=windows go build
  ./...` clean. `go.mod` promoted `golang.org/x/sys` to a direct dependency.
- **Review:** 3-partition (correctness/fit/test) all clean, zero
  Critical/Important. Two comment-clarity minors fixed by the lead (`5fe37ea9`);
  remaining minors recorded won't-fix: `terminateProcess` swallows only
  `ERROR_INVALID_PARAMETER` (Unix `ESRCH`-only parity); `procInfo` struct
  divergence + test-helper duplication (per-module structure accepted by brief);
  import-block style (gofmt-neutral).
- **processAlive:** zombie/exited-handle issue left DEFERRED to Phase 3 per scope
  (one-line code comment); `processAlive` switched to `x/sys`.

> Forward (Phase 3): the Windows subtree-reap assertion COMPILES under build tags
> but has NOT run on a Windows host — Phase 3 must actually execute it there.
> Also validate on Windows: the `processAlive` zombie handling, and the
> reparented/orphaned-child limitation (descendants that no longer chain to the
> root PID are not reaped — same as the Unix ps-table walk, no Windows group
> analogue).

### Phase 2: Stabilize flaky abort under the full suite (P1-priority, prereq)

Abort/cancel is the OS-divergent axis, so its Linux flakiness must be removed
before a Windows abort/cancel result can be trusted. Stabilize
`260616-bug-exec-mcp-running-large-abort-flaky-under-full-suite` (large-payload
abort flaky only under the full `internal/mcp` suite). Land the fix or, if it is
environmental, document the trigger and a deterministic guard. This phase gates
confidence in Phase 3's abort assertions.

### Result (f6c4e7d1) - 2026-06-22

Landed a real runtime fix — the flake was a concurrency defect, not just a
scheduler-sensitive assertion. `260616` is resolved and moved to `.done/`.

- **Root cause:** `execjob.finalize()` deleted the `active` sync.Map worker entry
  BEFORE taking `mu` to write the terminal status. After `cmd.Wait()` the OS
  process is already gone, so a concurrent `mu`-guarded `reconcile()` (driven by
  an `exec.result`/`exec.status` poll) could observe the window
  {active absent, process dead, status still running} and mis-mark a
  just-succeeded job `failed: exec job worker is no longer active` (exit_code 0,
  stdout already had the completion marker). The job self-healed when
  `finalize()` later wrote `succeeded`, but the poll had already returned the
  transient bad state — the "flaky under full-suite load" failure, since
  back-to-back tests shift scheduling into the window.
- **Fix (runtime):** delete the active entry via a `defer` registered after the
  `defer mu.Unlock()` so it runs (LIFO) still under `mu`, after
  `writeRecordLocked`. `finalize()` and `reconcile()` are now serialized;
  reconcile only ever sees {active present, status maybe running} or
  {active absent, status terminal}, never the bad window. The defer also covers
  the `loadErr` early return so an unreadable record cannot leak a stale entry.
- **Fix (test):** the abort test (`TestExecMCPRunningLargeAndAbort`) shared the
  `sleep 6` helper, which after the 5s `ForegroundWindow` left only ~1s before
  natural completion — under load the job finished before abort landed
  (`status: succeeded`). Gave it a dedicated `sleep 30` helper
  (`mcpAbortShellArgs`) so abort deterministically lands while running (abort
  reaps it promptly, so no 30s wait). Relaxed the non-blocking budget
  `1s → 5s` (the 1.16s spike was `serveStdioWithSession` setup jitter, not a
  block; a real block against the long job would be ~25s). The shared `sleep 6`
  `mcpLongShellArgs` is preserved for its other consumer
  (`TestExecMCPResultReadableJSONStdoutAndTimeout`).
- **Verification:** negative mutation proved causality — old ordering + a
  throwaway pre-lock sleep failed the timeout test deterministically; fixed
  ordering + a widened locked window PASSED. Full `go test ./... ×6` green,
  `internal/mcp ×5` green, affected tests + full `execjob` package green under
  `-race`, `go vet` clean on Linux and `GOOS=windows`, `GOOS=windows go build`
  clean.
- **Review:** single general reviewer, **clean** (zero Critical/Important/minor);
  independently re-verified the defer-LIFO ordering, no deadlock/cleanup
  regression, the lost-worker invariant for genuine crashes, and the test
  changes under `-race`.
- **Spec:** no changes (concurrency bug fix restoring documented best-effort
  behavior; `exec.result/status/abort` contracts unchanged). Mental model
  `mcp-runtime` gained one Common Mistakes entry for the active-delete ordering
  invariant.

> Forward (Phase 3): Linux abort/cancel assertions are now trustworthy, so the
> Windows full-suite run can rely on them. The Windows abort path
> (`execjob/process_windows.go` subtree reap from Phase 1) still has not run on a
> Windows host — Phase 3 must execute it and confirm the same finalize/reconcile
> ordering holds there (the race fix is OS-agnostic; only `processAlive` /
> `cancelProcess` differ by platform).

### Phase 3: Windows full-suite run + Windows-branch validation (P1)

On the WSL-attached Windows host, run `go test ./... -count=1` and capture the
result. This is the first time the six `*_windows.go` files compile and the
Windows execution branches run:

- exec-tool integration branches in `internal/mcp/server_test.go`
  (`mcpShellShapeArgs` / `mcpLongShellArgs` / `mcpLargeShellArgs` /
  `execToolJSONPath` PowerShell substitutions);
- Claude `.cmd` fake-executable branch in `internal/wsagent/agent_test.go`;
- the Phase 1 process-tree test on the real Windows kill path.

Record any build-tag breakage, separator/quoting failures, or behavioral
divergence. Verification boundary: green `go test ./...` on Windows (or an
enumerated, triaged failure list).

### Result (326fa74f) - 2026-06-22

First real Windows full-suite run (go1.26.3 windows/amd64) on the WSL2->Windows
interop host. Go cannot build over the WSL 9p mount (`RLock go.mod: Incorrect
function` — 9p has no file locking), so the tree was copied to a Windows-native
path (sibling layout preserved so wsrsrc shipped/mirror tests resolve
`../../../agents-plugin/rsrc`). Run #1: 8/12 packages passed, 4 failed; all four
fixed, re-run green.

- **Verification boundary met:** `go test ./... -count=1` green, 12/12 packages,
  `go vet`/`go build` clean on Windows. The six `*_windows.go` files compiled and
  ran for the first time; the Phase 1 cancel subtree-reap executed on the real
  Windows kill path; abort/cancel tests re-run x3 each, stable (cancel-tree
  ~0.09s, execjob abort ~18.5s, mcp abort ~11.4s).
- **#1 runtime defect (why Phase 3 mattered):** the Phase 1 Windows subtree kill
  *worked*, but the liveness probe lied — Windows `processAlive` treated
  `OpenProcess` success as alive, yet the kernel process object survives
  termination until all handles close, so an exited-but-unreaped child was
  reported alive (`TestCancelAsyncProcessTreeReapsChildTree` failed). This is
  exactly the zombie/cached-handle issue Phase 1 deferred here. Fixed in all
  three packages (wsagent, execjob, wsstate) via a zero-timeout
  `WaitForSingleObject` signaled-state check (WAIT_TIMEOUT=running,
  WAIT_OBJECT_0=exited). The probe feeds reconcile/reconcileActiveCall, so this
  also closes a Windows recovery-path defect (a dead worker stuck `running`).
  Per-package duplication kept (no new shared package), matching Phase 1's
  accepted per-module structure.
- **#2–#4 Windows test-side divergences (anticipated):** execjob abort timing —
  shared `slow`(6s) left ~1s after the 5s ForegroundWindow, so a dedicated
  `slowabort`(30s) helper makes abort land while running (reaped promptly, no
  full wait); mcp `execToolJSONPath` doubled backslashes but assertions match
  json-decoded `toolText` output, so it returns the native path; wsconfig
  expectation hard-coded `/` -> `filepath.Join`.
- **Review:** single general reviewer, clean (0 Critical/Important/minor);
  statically confirmed the `WaitForSingleObject` semantics (error only on
  WAIT_FAILED, so the 259/STILL_ACTIVE ambiguity is avoided), handle
  rights/no-leak, recovery-path consumers, and that the Unix variants are
  untouched.
- **Spec:** no changes (portability bug fix restoring documented best-effort
  cancel/liveness; no new caller-visible interface). Mental model
  `named-agent-runtime` updated (`3587c2c1`): the deferred Windows-liveness
  Technical Debt is resolved and a Common Mistakes entry records the
  OpenProcess-success != alive invariant.
- **processAlive zombie handling (Phase 1 forward item): RESOLVED** here.
- **Live-host safety:** all kills stayed strictly PID-scoped (Toolhelp32 PPID
  walk from the test's own spawned root); no image-name termination. `tasklist`
  shows no Windows `claude.exe` image — the live harness is the WSL2 Linux
  process — so the PID-scoped kills had no Windows process to reach regardless.

> Forward (Phase 4 / merge): the Windows surface is now green on a real host
> (Windows 11, go1.26.3) — single host / single toolchain, not multi-version.
> The reparented/orphaned-child limitation noted in Phase 1 (descendants that no
> longer chain to the root PID are not reaped — no Windows group analogue) still
> holds and was not exercised. Phase 4 (worktree path-layout, P3 stretch) remains
> optional.

### Phase 4: Windows worktree path-layout coverage (P3, optional/stretch)

`TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState`
(`internal/wsstate/paths_test.go`) skips on Windows. Either remove the skip with
a Windows-aware variant, or add a new test covering linked-worktree project
identity + worktree-state separation under drive-letter/UNC roots. Stretch
because it is lower frequency than the cancel/abort path; promote if Windows
worktree usage is on the near-term roadmap.
