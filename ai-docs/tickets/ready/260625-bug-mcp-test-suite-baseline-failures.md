---
title: internal/mcp test-suite baseline failures — prompt-seed drift and wsflow rsrc mirror drift
related-mental-model:
  - prompt-bundle
  - plugin-runtime
sage-review: skipped
---

# internal/mcp test-suite baseline failures — prompt-seed drift and wsflow rsrc mirror drift

## Problem

Found while running the test suite during `260625-feat-ws-session-state-machine`
Phase 1 review. Four tests fail on the epic branch tip (`72c0c418`) independent
of the Phase 1 work — they fail identically on the parent commit `b0a3278d~1`,
and the Phase 1 commit touches none of their files. So they are a pre-existing
baseline-red condition, not a regression introduced by current work.

Failing tests:

- `TestShippedDelegationSectionSeedAndOverride` (`prompt_override_test.go`)
- `TestShippedUserPreferenceSectionEmptySlotAndOverride` (`prompt_override_test.go`)
- `TestConfigPromptSetEndToEnd` (`prompt_override_test.go`)
- `TestWsflowRsrcMirrorUpToDate` (package `internal/wsrsrc`)

The three `prompt_override_test.go` failures all assert shipped prompt-seed /
delegation-section text, which points at prompt-seed drift: the shipped seed
blocks the tests pin no longer match what the rsrc tree renders.

`TestWsflowRsrcMirrorUpToDate` reports byte-drift in the generated wsflow rsrc
mirror across several files (`delegate-orientation.md`, `lead-implement`,
`lead-prefer-subagent`, `lead-workflow-manual`, `lead-write-ticket`,
`manifest.json`), meaning canonical `agents-plugin/rsrc/` was edited without
regenerating `agents-plugin-wsflow/rsrc/`.

## Phases

### Phase 1: Restore green baseline

Verify the four previously failing tests now pass on the current branch tip.
Run `go test ./internal/mcp/... ./internal/wsrsrc/...` and confirm all tests
pass. If still failing: regolden prompt-seed fixtures with `-update` flag; for
the wsflow mirror, regenerate with
`WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -count=1 -run TestRegenerateWsflowRsrcMirror`.

Completion: all four previously failing tests pass; no new failures introduced.

## Spec Impact

Internal test hygiene only — no caller-visible contract change.
Contract-first spec: no.
