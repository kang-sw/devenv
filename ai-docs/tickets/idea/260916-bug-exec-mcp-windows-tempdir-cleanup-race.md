---
title: TestExecMCPRunningLargeAndAbort flakes on Windows via TempDir cleanup race
---

# TestExecMCPRunningLargeAndAbort flakes on Windows via TempDir cleanup race

## Status of this ticket

Bug capture from the ws 0.46.8 release run. Test-only defect (no shipped-binary
impact). Ready to triage/fix once the fix approach is confirmed.

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

## Notes

- Does not require unshipping 0.46.8; the release is live and complete.
- Consider re-running the failed Windows smoke job to confirm flakiness on the
  0.46.8 tag before/independent of the fix.
- Related process gap: the pi TS suite CI-scope ticket
  (260916-research-pi-ts-suite-outside-release-ci-contract-rot) is a different
  suite; this one is the Go Windows smoke.
