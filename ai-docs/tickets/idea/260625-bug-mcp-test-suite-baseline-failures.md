---
title: internal/mcp test-suite baseline failures — prompt-seed drift and wsflow rsrc mirror drift
related-mental-model:
  - prompt-bundle
  - plugin-runtime
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

## Possible Follow-Ups

- For the wsflow mirror failure: regenerate with
  `WS_REGEN_WSFLOW_RSRC=1 go test ./internal/wsrsrc -run TestRegenerateWsflowRsrcMirror`
  and confirm the drift was purely a missed regen, not a semantic divergence.
  This overlaps `260625-bug-wsflow-rsrc-mirror-regen-missed-after-shipped-edit`
  (the process gap that lets a shipped rsrc edit skip the mirror).
- For the three prompt-seed assertions: determine whether they need a regolden
  against the current shipped seed text (drift is intended, test stale) or
  whether they caught a real seed regression (text changed unintentionally).
  Bisect to the commit that moved the seed text.
- Restore a green baseline so future reviews can make an unqualified green-bar
  claim instead of carrying a "4 known-unrelated failures" caveat every run.
