---
title: "Windows parent-watch PID-reuse race flakes TestWatchProcessExit_NeverOpenablePID"
---

## Background

The `v0.45.0` `ws-mcp release` workflow (run 33853160687) failed its post-publish
`Windows ws-mcp smoke` job on a single non-deterministic test while the `Build
ws-mcp assets` job succeeded (so the release + cross-platform binaries published
regardless). A `--failed` re-run was triggered to confirm the flake.

Failing test:

```
--- FAIL: TestWatchProcessExit_NeverOpenablePID (0.01s)
    parent_watch_windows_test.go:69: watchProcessExit fired onExit for an already-dead pid
```

`cmd/ws-mcp/parent_watch_windows_test.go` kills+reaps a helper, then asserts
`watchProcessExit(pid, …)` never fires because `OpenProcess` should fail on the
dead PID. The test's own comment (lines 46-50) already documents the flaw: there
is an "accepted PID-reuse race" — the OS may reassign the reaped PID before
`OpenProcess` runs, in which case `WaitForSingleObject` returns `WAIT_OBJECT_0`
against the reused process and `onExit` fires. This run realized that race. It is
orthogonal to epic 260903 (MCP tool-surface reduction) — every `internal/*`
package passed; only this `cmd/ws-mcp` test failed.

## Open Questions

- Make the test deterministic rather than probabilistic. Options:
  - Assert on process *identity*, not just handle-openability — capture the
    helper's creation time (`GetProcessTimes`) before kill and, in the impl or
    test, reject a handle whose creation time differs (true PID-reuse detection).
  - Or drop the "never fires" assertion entirely and only keep the positive
    `FiresOnRealExit` test, since a reused PID is genuinely indistinguishable by
    handle alone — the "never openable" guarantee is not one Windows actually
    offers.
- Does the production `watchProcessExit` need the same PID-reuse hardening? The
  parent (resident launcher) PID could in principle be reused after the launcher
  dies, causing a spurious self-terminate. Low probability (short window between
  `os.Getppid()` and `OpenProcess`), but the creation-time check would close it
  in both the test and the real watch.

## Notes

- Blast radius: CI-only flake on the Windows post-publish smoke gate; does not
  affect published release assets and does not reproduce on Linux.
- If the re-run passes, this stays `idea/`; if it fails deterministically,
  promote to a real fix ticket and prefer the creation-time identity check.
