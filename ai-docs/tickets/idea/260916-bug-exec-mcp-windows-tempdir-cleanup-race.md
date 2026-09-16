---
title: TestExecMCPRunningLargeAndAbort flakes on Windows via TempDir cleanup race
---

# TestExecMCPRunningLargeAndAbort flakes on Windows via TempDir cleanup race

## Status of this ticket

**CI failure RESOLVED (2026-09-16)** by a test-only fix — see `## Resolution`.
Kept open for one remaining, lower-priority thread: a latent production
descendant-wait note (see `## Remaining`), which is best-effort by design and did
not cause the CI failure.

## Observation

The `Windows ws-mcp smoke` job of the `ws-mcp release` workflow failed on the
v0.46.8 tag run (35054542337). The `Build ws-mcp assets` job succeeded and the
GitHub release v0.46.8 published with all seven assets, so this is a Windows
test-cleanup failure, not a release-artifact or shipped-binary defect.

Single failing test, in code untouched by the 0.46.8 batch (the range touched no
exec files and did not modify `server_test.go`):

```
--- FAIL: TestExecMCPRunningLargeAndAbort (17.63s)
    server_test.go:2686: large response = ... status: running ...
    testing.go:1232: TempDir RemoveAll cleanup: remove C:\...\TestExecMCPRunningLargeAndAbort...\001:
        The process cannot access the file because it is being used by another process.
FAIL	github.com/kang-sw/devenv/internal/mcp	289.977s
```

The failure is in Go's `t.TempDir()` post-test `RemoveAll`, not the test body:
the test starts a large/running exec subprocess and aborts it, but the child
process (or a handle it owns) is still alive when cleanup runs, and Windows
refuses to remove an in-use file. This is timing-dependent: the v0.46.7 Windows
smoke passed the same test one release earlier.

## Likely cause

The aborted exec subprocess is not fully terminated-and-waited before the test
returns, so `t.TempDir()` cleanup races the OS releasing the child's handles.
On POSIX an open handle does not block `unlink`, which is why this only surfaces
on Windows.

## Fix directions (confirm before implementing)

- Ensure the test explicitly kills AND waits for the aborted exec child (and any
  reader goroutines / file handles) before returning, so no handle outlives the
  test body.
- If a handle can legitimately linger, replace `t.TempDir()` for this test with a
  manually-managed dir plus a Windows-tolerant `RemoveAll` retry loop (short
  backoff), rather than relying on Go's single-shot cleanup.
- Verify the exec abort path itself reaps the child on Windows (if the leak is in
  production abort handling, not just the test, that is a real exec.* bug and this
  ticket should be re-scoped up from test-only).

## Resolution

Test-only fix landed on develop via PR #10 (merge 8973a8b7, fix commit 87187663),
validated on real Windows CI by the release workflow's PR run 35057067620
(Build + Windows smoke both green). Two changes in
`TestExecMCPRunningLargeAndAbort`:

- Drive `exec.result` with `timeout_seconds: 30` (polls to a terminal status) and
  assert `combined_bytes: 5000` on that response, instead of trusting the 5s
  synchronous `exec.shell` launch (which returned `running` under Windows load).
- Add `reapExecKeys` + a `t.Cleanup` (registered after server setup, so it runs
  before the `WS_CACHE_HOME`/`t.TempDir` cleanups) that aborts and polls-to-terminal
  every spawned `exec_key` before TempDir removal.

Investigation confirmed **no production exec bug** for this failure: `Abort`
already waits for the shell process exit and `ResultWithTimeout` already polls to
terminal, so the fix stayed entirely test-only.

## Remaining (latent, low priority)

`Abort`/`waitProcessExit` (execjob.go) wait only on the tracked shell pid, while
`cancelProcess` issues async `TerminateProcess` to descendants without waiting.
On Windows a grandchild (e.g. `ping`) that inherited the worktree root as CWD
could briefly outlive the shell after `Abort` returns — best-effort by design,
but the comment at execjob.go:277-285 overstates that the deterministic wait
covers the CWD-holding case (true only for the shell, not descendants). No
observed failure; consider making `Abort` wait the whole process tree.

## Notes

- Does not require unshipping 0.46.8; the release is live and complete.
- Consider re-running the failed Windows smoke job to confirm flakiness on the
  0.46.8 tag before/independent of the fix.
- Related process gap: the pi TS suite CI-scope ticket
  (260916-research-pi-ts-suite-outside-release-ci-contract-rot) is a different
  suite; this one is the Go Windows smoke.
