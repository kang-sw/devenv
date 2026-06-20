---
title: exec MCP large running/abort test is flaky under full suite load
related:
  260524-epic-async-exec-job-surface: owns exec job MCP surface and bounded readers
  260524-chore-exec-surface-runtime-contract: owns exec runtime contract closure
related-mental-model:
  - mcp-runtime
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
