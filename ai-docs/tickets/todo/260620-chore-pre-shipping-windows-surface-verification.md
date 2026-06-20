---
title: Pre-shipping verification — Windows surface hardening
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260616-bug-exec-mcp-running-large-abort-flaky-under-full-suite: Phase 2 stabilizes this flaky abort before Windows abort/cancel results are trusted
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
  conformance with the existing cancel contract
  (`named-agent-runtime` `#260505-agent-cancel-recovery`), not introduce a new
  caller-visible contract. No spec change is expected; if one becomes necessary,
  route through `lead-write-spec`.
- Process-tree assertions must be deterministic (no sleep-races) — spawn a child
  that blocks on a sentinel, cancel, then assert the child is reaped.

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
backends; record the rejected alternatives.

Verification boundary: new test green on Linux; the Windows path is structurally
exercised (asserted on the Windows run in Phase 3). Also review
`processAlive` (Windows) — `OpenProcess`/`FindProcess` can report a zombie/exited
handle as alive; cover or document the recovery-path implication.

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
