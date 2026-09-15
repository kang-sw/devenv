---
title: "Windows exec-abort TempDir cleanup race reds the release smoke job"
---

# Windows exec-abort TempDir cleanup race reds the release smoke job

## Background

The `ws-mcp release` workflow for `v0.46.5` (run `34922150213`) finished with
the `Build ws-mcp assets` job green (all seven binaries + `SHA256SUMS`
published) but the `Windows ws-mcp smoke` job red, failing the whole workflow.
The failure is unrelated to the shipped range (260915 lead-run
defer-phase-merge, 260914 pi Explore ladder — none touch exec/abort/server),
and the immediately prior release (`v0.46.4`) passed the same job, so this is
an intermittent Windows-only test-cleanup race, not a regression.

Observed failure (`internal/mcp`, `go test ./...` on Windows):

```
--- FAIL: TestExecMCPRunningLargeAndAbort (18.06s)
    server_test.go:2686: large response = ... status: running ...   # assertion OK
    testing.go:1232: TempDir RemoveAll cleanup: remove C:\Users\RUNNER~1\...\001:
        The process cannot access the file because it is being used by another process.
FAIL	github.com/kang-sw/devenv/internal/mcp	213.517s
```

The test's own assertions pass; the FAIL is Go's `t.TempDir()` `RemoveAll`
cleanup racing an aborted child process (pid 1800) that still holds a handle
open under the temp dir when cleanup runs. This is the classic Windows
file-lock-on-abort timing window: `os.RemoveAll` cannot unlink a file whose
process handle has not yet been released after the abort path returns.

Impact: a flaky non-release job can red an otherwise-successful release
workflow, making "release green" a misleading gate — the operator must open
the job logs to tell a real regression from this race.

## Phases

### Phase 1: Make the exec-abort teardown deterministic on Windows

Reproduce `TestExecMCPRunningLargeAndAbort` on Windows (or a Windows CI
matrix rerun) and confirm the abort path leaves a child process handle open
past test return. Fix so the aborted process's handles are released before the
test body returns — e.g. the abort path waits for the child to fully exit (or
the test explicitly awaits process teardown) before `t.TempDir()` cleanup
runs — without weakening the abort semantics the test asserts. Verification
must show the test cleanup no longer races on Windows (repeated runs green) and
that Linux behavior is unchanged. If the underlying handle-release is inherent
to the OS, the fallback is to make the smoke job tolerate the known cleanup
race narrowly (not blanket-ignore failures) so a real assertion failure still
reds the release.

#### Result

Root cause: `execjob.Abort` (execjob.go) called `cancelProcess` — whose
`TerminateProcess` (Windows) / `SIGKILL` (Unix) are asynchronous — then slept a
fixed `100ms` and returned. The sleep was probabilistic: on a slow/loaded
Windows runner the cancelled shell child (CWD = the exec worktree `root`) had
not exited when Go's `t.TempDir()` cleanup ran `os.RemoveAll`, and Windows
refuses to unlink a directory a live process holds as its CWD. This was the
prior "fix" (fixed sleep + subtree reap in `d89e6539`) that never made the wait
deterministic, which is why the flake recurred.

Fix: replaced the fixed sleep with `waitProcessExit(pid, abortExitTimeout=5s)`,
a bounded poll on the existing `processAlive` primitive that returns the moment
the tracked process actually exits (releasing its CWD/file handles) and is
bounded so a wedged process cannot hang the abort. Strictly faster than the old
sleep on the common path; abort semantics unchanged.

Verification: `go test ./internal/execjob/` and the `TestExecMCPRunningLargeAndAbort`
mcp test pass on Linux; `GOOS=windows go build ./...` + `go vet` clean
(`processAlive` is defined per-platform). True Windows-runner confirmation
landed on `workflow_dispatch` run `34923283670` (develop, carrying `4b94bb3e`):
the `Windows ws-mcp smoke` job — the exact job that reddened the v0.46.5 release
run on this test — passed green. The fix is deterministic by construction rather
than timing-dependent, so it holds regardless of runner load.
