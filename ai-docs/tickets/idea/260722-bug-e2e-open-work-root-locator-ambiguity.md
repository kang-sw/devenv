---
title: ws-dashboard e2e - openWorkRoot locator ambiguity (rootPicker.open vs empty-state CTA) red-lines acceptance suite
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
260525 (worktree removal / hide-unhide).
