---
title: ws-dashboard e2e - openWorkRoot locator ambiguity (rootPicker.open vs empty-state CTA) red-lines acceptance suite
sage-review-design: completed
sage-review-completeness: completed
related: 260525-feat-ws-dashboard-workroot-polishing-backlog
---

# ws-dashboard e2e - openWorkRoot locator ambiguity (rootPicker.open vs empty-state CTA) red-lines acceptance suite

## Background

`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` and its shared
`openWorkRootInBrowser` helper (Playwright) drive the frontend acceptance
suite. The suite is a single serial test, so its first step must succeed
before any later step runs.

## Symptom

`openWorkRootInBrowser`'s Playwright strict-mode locator
`[data-command-id="rootPicker.open"]` now matches **two** elements instead of
one, so the strict locator throws immediately. The second match is the
empty-state CTA button carrying class `.open-work-root-empty-cta`, which was
introduced by an earlier commit `21116b54`.

Because the acceptance spec is a single serial test, this one locator
ambiguity red-lines the entire e2e suite at its first step - no later test in
the file gets a chance to run.

## Impact

No e2e/browser coverage can currently be verified to green. In particular,
newly authored acceptance coverage (e.g. the worktree-removal modal and
hide/unhide flow added under ticket 260525) type-checks but cannot be run to
green until this locator ambiguity is resolved.

## Suspected Fix Direction (suggestion, not mandate)

Disambiguate the locator - e.g.:

- scope it to the primary root-picker control specifically, or
- give the two buttons distinct `data-command-id`/`data-testid` values, or
- narrow the helper's selector so it does not also match the
  `.open-work-root-empty-cta` empty-state CTA.

## Reporter Context

Surfaced 2026-07-22 while adding ticket-mandated browser coverage for ticket
260525-feat-ws-dashboard-workroot-polishing-backlog (worktree removal /
hide-unhide).

## Phases

### Phase 1: Disambiguate the openWorkRoot locator

Scope: `openWorkRootInBrowser`'s Playwright strict-mode locator
`[data-command-id="rootPicker.open"]` double-matches the primary root-picker
opener (App.tsx:2513) and the empty-state CTA button
`.open-work-root-empty-cta` (App.tsx:2553). Disambiguate so the helper
resolves exactly one element.

Implementer has latitude across the three directions listed under Suspected
Fix Direction above. Design-reviewer guidance: prefer narrowing the test
helper's selector (e.g. scope to the primary control, or exclude
`.open-work-root-empty-cta`) or adding a distinct `data-testid` to one of the
two buttons. Avoid changing either button's `data-command-id` value - both
`commands.ts` and `hotkeys.ts` key off `rootPicker.open` for command
dispatch/hotkey binding, so retargeting that attribute risks breaking
non-test behavior.

Completion boundary: the locator no longer double-matches, and the full
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` acceptance suite runs
green end to end.

Verification: run the acceptance suite (e.g. `npx playwright test
dashboard-acceptance.spec.ts` from `ws-dashboard/frontend/`) and confirm all
tests pass, including the first `openWorkRootInBrowser` step.

### Result (2bc160d4) - 2026-07-22

Narrowed the `openWorkRootInBrowser` helper locator in
`ws-dashboard/frontend/e2e/dashboard-acceptance.spec.ts` (line 591) from
`[data-command-id="rootPicker.open"]` to
`[data-command-id="rootPicker.open"]:not(.open-work-root-empty-cta)`. Test-only
change; `App.tsx` and all `data-command-id` values left untouched per the
design-reviewer guidance (those are dispatch/hotkey load-bearing).

Verified: reproduced the original strict-mode double-match on the unmodified file
(stash baseline) at spec.ts:592/951, then confirmed the fix resolves it to exactly
one element — the suite now proceeds past every `openWorkRootInBrowser` call site
including the `.toBeFocused()` assertion. `npm run build` green. Single full-scope
review: clean.

Deviation from the completion boundary: the acceptance suite is NOT fully green
end to end. Removing the locator ambiguity unmasked a separate, pre-existing
failure at `dashboard-acceptance.spec.ts:2714` ("create terminal and run a
command", asserting a WebSocket resize-type frame was sent), confirmed via baseline
rerun to reproduce identically and NOT caused by this change. That distinct failure
is out of this ticket's scope and is tracked by
`260722-bug-e2e-terminal-resize-frame-assertion-fails`. This ticket's own scope —
locator disambiguation — is complete.
