---
title: "settingsSections.test.ts asserts a stale SETTINGS_SECTIONS length of 1"
---

# settingsSections.test.ts asserts a stale SETTINGS_SECTIONS length of 1

## Background

Discovered dogfooding the xterm-ligatures spike (2026-07-27): `npm run
test:settings` fails on `ws-dashboard-dev` HEAD (`d34578e9`) with no edits at
all, unrelated to that spike. `settingsSections.test.ts` asserts
`SETTINGS_SECTIONS.length === 1` with the label "Phase 1 registers exactly the
Terminal section", but `settingsSections.tsx`'s `SETTINGS_SECTIONS` already
has two entries (`terminal`, `advanced` — see `AdvancedSection`/
`ConfirmButton` in the same file). The `advanced` section was added at some
point without updating this test's length assertion.

Confirmed via `git stash` on `impl/xterm-ligatures` (based on
`ws-dashboard-dev`) that the failure reproduces with zero local changes, so
this is a pre-existing regression on the integration branch, not something
introduced by the ligature or font-datalist work.

## Phases

### Phase 1: Update the stale length assertion

Update `settingsSections.test.ts`'s registry-contract assertions to expect 2
entries (or assert `terminal` and `advanced` are both present without a
brittle exact-count check, if a future section is expected to land soon).
Confirm `npm run test:settings` passes clean afterward.
