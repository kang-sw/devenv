---
title: "dashboard-acceptance.spec.ts e2e fails: transcript stays hidden after Codex tile click"
related:
  260713-fix-ws-dashboard-agent-chat-ui-usability-polish: related
---

# dashboard-acceptance.spec.ts e2e fails: transcript stays hidden after Codex tile click

## Background

Discovered during code review of `260713-fix-ws-dashboard-agent-chat-ui-usability-polish`
Phase 1 (commit `7171aa84`). `npm run test:browser` (build + daemon compile +
`playwright test`) run against this repo's current `dashboard-acceptance.spec.ts`
fails at line ~2578 — `expect(transcript).toBeVisible()` after clicking the
Codex tile (`data-agent-chat-tile="codex"`) — with the `agent-chat-transcript`
locator resolving but staying `hidden`, well before the test reaches later
blocks (e.g. the history-traversal coverage around line 2658).

Confirmed pre-existing and unrelated to the Phase 1 diff: reproduced the
identical failure (same locator, same line, same "hidden" outcome) against
the pre-diff baseline commit (`4e54fda7`) in an isolated git worktree with a
byte-identical `package-lock.json`. Both the diff and its baseline fail
identically, so this is not a regression introduced by that ticket's work —
it is a standing defect (or environment-sensitivity issue) in the e2e suite
or the code path it exercises.

Impact: this failure aborts the entire monolithic `dashboard-acceptance.spec.ts`
test at an early `test.step`, so no later coverage in the same file
(history-traversal, fork-from-here, resume popover, etc.) can currently be
exercised via this test run in this environment. A second, different flaky
failure (in a terminal-echo step) was also observed on a retry run,
suggesting broader environment instability beyond just this one locator.

## Phases

### Phase 1: Diagnose why the agent-chat-transcript stays hidden after Codex tile launch

Investigate whether this is a genuine product bug (transcript pane fails to
become visible after a stub/real Codex session launch under some condition)
or an environment/timing issue specific to how this sandbox runs Playwright
(e.g. daemon startup race, missing binary/auth state affecting stub-vs-real
harness selection, viewport/animation timing). Reproduce with tracing/video
enabled, inspect actual DOM state at the failure point, and determine root
cause before deciding whether the fix belongs in test code (harness/wait
strategy) or product code (visibility logic).

**Verification**: `dashboard-acceptance.spec.ts`'s Codex tile launch step
passes reliably (multiple runs, not just once) once root-caused and fixed.
