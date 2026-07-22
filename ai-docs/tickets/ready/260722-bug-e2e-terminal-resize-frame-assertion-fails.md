---
title: ws-dashboard e2e - "create terminal and run a command" resize-frame assertion fails, blocks acceptance suite green
related:
  260722-bug-e2e-open-work-root-locator-ambiguity: unmasked this failure once the earlier locator ambiguity was fixed
related-mental-model:
  - ws-web-dashboard
---

# ws-dashboard e2e - "create terminal and run a command" resize-frame assertion fails, blocks acceptance suite green

## Background

While fixing the openWorkRoot locator ambiguity
(`260722-bug-e2e-open-work-root-locator-ambiguity`), the acceptance suite began
progressing past the previously-blocking first step. This unmasked a separate,
pre-existing failure further down the same serial test.

## Symptom

`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts:2714`, in the
`"create terminal and run a command"` step, asserts that a WebSocket `resize`-type
frame was sent. That assertion fails. Because `dashboard-acceptance.spec.ts` runs
as a single serial test, this red-lines the remainder of the suite (the later
`"linked server root picker ..."` test does not run).

Confirmed via baseline rerun (locator fix stashed) that this same resize-frame
assertion reproduces identically once the earlier rootPicker failure is bypassed —
i.e. it is pre-existing, not caused by the locator fix.

## Impact

Blocks `npm run test:browser` / the Playwright acceptance gate from reaching full
green. That gate is the mandatory verification for UI-facing dashboard work (per
the ws-web-dashboard domain rule), so tickets that defer to it (which-key overlay
Phase 2, the App.tsx decomposition refactor, 260525) stay verification-blocked
until the suite runs green end to end.

## Triage Needed

Determine whether this is a genuine product bug (daemon/frontend does not emit the
expected resize frame on terminal creation/resize) or a test-harness / environment
artifact (timing, headless terminal sizing, daemon not fully wired in the test
env). Capture the reproduction and the expected-vs-actual frame before deciding a
fix direction.

## Reporter Context

Surfaced by the implementer during
`260722-bug-e2e-open-work-root-locator-ambiguity` Phase 1 (commit 2bc160d4), which
narrowed the openWorkRoot locator and let the suite advance to this step.
