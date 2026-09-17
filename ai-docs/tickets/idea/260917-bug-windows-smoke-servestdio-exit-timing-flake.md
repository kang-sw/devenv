---
title: Windows smoke flake — ServeStdio-exit fixed 3s budget in TestServeStdioDoesNotBlockToolsListBehindLongCall
related:
  260917-bug-exec-mcp-windows-test-timing-margin-flake: sibling
---

# Windows smoke flake — ServeStdio-exit fixed 3s budget

## Observation (dogfood, 2026-09-17, v0.46.10 release)

The `v0.46.10` tag release run (35226870226) `windows-smoke` job failed on:

```
--- FAIL: TestServeStdioDoesNotBlockToolsListBehindLongCall (21.82s)
    server_test.go:1866: ServeStdio did not exit after input close
FAIL	github.com/kang-sw/devenv/internal/mcp	377.724s
```

The same commit's `windows-smoke` on PR #11 passed green, and the just-shipped
exec-flake fix (`260917-bug-exec-mcp-windows-test-timing-margin-flake`) kept the
exec tests green here (`internal/execjob` 49s ok, `cmd/ws-mcp` 88s ok). This is a
distinct fixed-margin timing assertion in the same test, not a regression of the
shipped fix. The release assets themselves published successfully (the `build`
job is the release gate; `windows-smoke` does not gate publish).

## Root cause (code read)

`agents-plugin-tool/internal/mcp/server_test.go:1860-1867` — after closing the
input pipe, the test waits a fixed 3s for `ServeStdio` to return:

```go
select {
case <-done:
case <-time.After(3 * time.Second):
    t.Fatal("ServeStdio did not exit after input close")
}
```

On a loaded Windows runner, io.Pipe close propagation plus scanner-loop teardown
can exceed 3s. The exec-flake fix that touched this test (the `mcpAbortShellArgs`
swap and the `t.Cleanup(reapExecKeys)` LIFO ordering) addressed the
`exec.result` first-output-line race but left this shutdown budget fixed. The
test also carries two other fixed budgets — the 500ms first-line wait and the
`exec.result timeout_seconds:2` — that are candidate flake sources under Windows
CI load.

## Why it matters

Same failure mode as the just-closed exec-flake ticket: a fixed wall-clock margin
on a Windows CI runner, which recurs intermittently and produces a red release
smoke that does not reflect a real defect. Every tag release re-runs this job, so
a fixed-budget flake here reintroduces the exact "is the release smoke
trustworthy?" question the sibling ticket set out to remove.

## Open questions / directions

- Widen or make adaptive the 3s `ServeStdio`-exit wait (and reconsider the 500ms
  first-line budget), mirroring the poll-until-terminal approach the sibling
  ticket applied to the exec budgets — without weakening what the test pins
  (concurrent dispatch: tools/list must not queue behind a blocking tools/call).
- Confirm whether the failure is pure runner load or a real Windows-specific
  ServeStdio shutdown-latency path worth fixing in production code (the test is
  in `internal/mcp`; production `ServeStdio` shutdown lives in the same package).

## Evidence

- Failed run: https://github.com/kang-sw/devenv/actions/runs/35226870226 (windows-smoke, attempt 1).
- Test: `agents-plugin-tool/internal/mcp/server_test.go` (`TestServeStdioDoesNotBlockToolsListBehindLongCall`, fatal at the ServeStdio-exit select).
- Sibling: `260917-bug-exec-mcp-windows-test-timing-margin-flake` (closed, poll-until-terminal exec-budget fix).
