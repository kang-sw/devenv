---
title: exec MCP large running/abort test is flaky under full suite load
related:
  260524-epic-async-exec-job-surface: owns exec job MCP surface and bounded readers
  260524-chore-exec-surface-runtime-contract: owns exec runtime contract closure
related-mental-model:
  - mcp-runtime
completed: 2026-06-22
---

# exec MCP large running/abort test is flaky under full suite load

## Background

During wsflow product-mode rendering work, the targeted MCP and wsflow package
tests passed, but repeated `cd agents-plugin-tool && go test -count=1 ./...`
runs exposed timing-sensitive failures in `TestExecMCPRunningLargeAndAbort`.

Observed failures on 2026-06-16:

- First full-suite run: abort status reported `status: succeeded` instead of an
  aborted/running transition; the same test passed when rerun alone.
- Second full-suite run: `non-blocking result calls took 1.162722287s`, tripping
  the test's timing expectation.

This looks unrelated to playbook rendering changes: the failure is in the exec
MCP running/abort timing path and only appears under broader package load. The
test may need a less scheduler-sensitive assertion, or the exec result/abort
path may need tighter non-blocking behavior under concurrent suite pressure.

## Evidence

- `cd agents-plugin-tool && go test -count=1 ./internal/mcp -run 'TestPlaybookPrintWsflowProductModeFiltersHiddenGuidance|TestDelegateTipOmitsMercenaryInNoAgentMode|TestServeStdioNoAgentModeHidesAgentBackedTools|TestRenderPromptSubstitutionAndAllowlist|TestWsflowModeAdvertisesAndServesPromptRender'` passed.
- `cd agents-plugin-tool && go test -count=1 ./internal/wsrsrc` passed.
- `python3 -m unittest discover agents-plugin-wsflow/tests` passed.
- `python3 -m unittest discover agents-plugin/tests` passed.
- `cd agents-plugin-tool && go test -count=1 ./internal/mcp -run TestExecMCPRunningLargeAndAbort -v` passed after the first full-suite failure.

## Resolution (f6c4e7d1) - 2026-06-22

Fixed under `260620-chore-pre-shipping-windows-surface-verification` Phase 2.
Both observed symptoms had distinct causes:

- The `status: succeeded`-instead-of-aborted failure was a real concurrency
  defect: `execjob.finalize()` deleted the `active` worker-map entry before
  acquiring `mu` to write the terminal status, so a concurrent `reconcile()`
  (from a result/status poll) could mis-mark a just-completed job
  `failed: exec job worker is no longer active`, and a too-tight abort window
  (shared `sleep 6` helper, ~1s left after the 5s `ForegroundWindow`) let the
  job complete before abort landed. The map deletion now happens inside the
  `mu` critical section after the status write, and the abort test uses a
  dedicated long-running (`sleep 30`) command.
- The `non-blocking result calls took 1.16s` failure was `serveStdioWithSession`
  setup jitter under suite load, not a blocking regression; the sub-second
  assertion was relaxed to a load-tolerant 5s ceiling (still far below the ~25s
  a genuine block would take against the long job).

Verified by negative mutation, `go test ./... ×6`, and `-race`. See the Phase 2
`### Result` in `260620` for full detail.
