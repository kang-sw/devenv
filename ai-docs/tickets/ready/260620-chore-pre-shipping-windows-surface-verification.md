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

### Phase 4: Windows worktree path-layout coverage (P3, optional/stretch)

`TestLinkedWorktreeSharesProjectIdentityAndSeparatesWorktreeState`
(`internal/wsstate/paths_test.go`) skips on Windows. Either remove the skip with
a Windows-aware variant, or add a new test covering linked-worktree project
identity + worktree-state separation under drive-letter/UNC roots. Stretch
because it is lower frequency than the cancel/abort path; promote if Windows
worktree usage is on the near-term roadmap.
