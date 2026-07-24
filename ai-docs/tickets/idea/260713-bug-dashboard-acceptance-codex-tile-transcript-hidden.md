---
title: "dashboard-acceptance.spec.ts e2e fails: transcript stays hidden after Codex tile click"
related:
  260713-fix-ws-dashboard-agent-chat-ui-usability-polish: related
  260714-refactor-dashboard-active-root-atomic-select-pure-derivation: ruled-out-cause
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

### Update 2026-07-20: re-confirmed byte-identical across two more commits, ruling out the atomic-select refactor

Re-reproduced the exact same failure (same locator, same line, same
"hidden" outcome) on both the `goal/ws-dashboard-tickets` tip
(`f0ab0fb1cb52583129da5a00274bce99043b8073`) and the pre-session baseline
(`612bc0fae3e3f8ab22682d73003275716de6bf06`, i.e. before
`260714-refactor-dashboard-active-root-atomic-select-pure-derivation`
Phase 2/3 landed). This rules out that refactor (the `selectRoot`
atomic-action / pure-derivation change) as the cause — the bug predates it
and is unaffected by it.

**Exact repro steps**: run `npm run test:browser` from `ws-dashboard/frontend`.
The suite is `e2e/dashboard-acceptance.spec.ts`, top-level test
`"dashboard workRoot UI browser acceptance"` (line 800; the whole file runs
`test.describe.configure({ mode: "serial" })` at line 54). The failing step
is `test.step("open new agent tab and launch a stub harness session", ...)`
at line 2534: after
`pane.locator('[data-agent-chat-tile="codex"]').click()` (line 2576), the
assertion at line 2578 (`await expect(transcript).toBeVisible();`) times out
after 20000ms:

```
Error: expect(locator).toBeVisible() failed
Locator:  locator('.workbench-pane[data-surface-kind="agentChat"] .workbench-pane-body').locator('[data-testid="agent-chat-transcript"]')
Expected: visible
Received: hidden
```

The tab header does update (e.g. to "Codex — Codex conversation") and the
message input appears; only the transcript body area beneath it never
becomes visible.

**Source-level lead (open investigation, not yet root-caused)**: the
element is `ws-dashboard/frontend/src/App.tsx:7816`
(`<div className="agent-chat-pane-transcript" data-testid="agent-chat-transcript">`,
inside the `if (pane.session) {...}` render branch). Its own CSS rule at
`ws-dashboard/frontend/src/styles.css:3807`
(`.agent-chat-pane-transcript { display: grid; gap: var(--ws-space-06); overflow: auto; }`)
does not itself hide the element, so the computed `hidden` state must come
from an ancestor — likely Dockview's inactive-tab/pane visibility toggling,
or some other conditional wrapper — not yet traced.

**Test-coverage side effect**: because the suite runs in serial mode, this
failure also blocks the second top-level test,
`"linked server root picker uses server-scoped local gateway routes"`
(line 3249), from running at all. That test's status is therefore unknown
(not merely "blocked") until this bug is fixed and the suite can proceed
past this step.

**Artifacts referenced (not moved/copied, cite only)**:
- Failure screenshot:
  `ws-dashboard/frontend/test-results/dashboard-acceptance-dashb-734ce-kRoot-UI-browser-acceptance/test-failed-1.png`
  — shows the header already switched to "Codex — Codex conversation" with
  the input box visible, transcript body area blank.
- Full run log was written to a session-scoped scratchpad path during
  investigation; that path is ephemeral and may not survive — do not rely
  on it existing when reading this ticket later. Re-run
  `npm run test:browser` to regenerate.

## Phases

### Phase 1: Root-cause and fix

Trace which ancestor sets the `agent-chat-transcript` element to
computed-hidden after a new agent tab (e.g. the "codex" tile) launches a
session — likely a Dockview inactive-tab/pane visibility mechanism or a
conditional wrapper between the Dockview pane body and
`App.tsx:7816`, but not yet confirmed. Fix the underlying visibility logic
(product code, not a test-side wait-strategy workaround, unless
investigation shows the failure is genuinely test/environment-only).

**Verification**: both top-level tests in `dashboard-acceptance.spec.ts`
pass — `"dashboard workRoot UI browser acceptance"` (including the
previously-failing Codex tile launch step) and `"linked server root picker
uses server-scoped local gateway routes"` (run for the first time once the
first test no longer aborts the serial suite early; its outcome is
currently unverified, not just blocked).

## Deferral (2026-07-20)

Research escalated this to discussion (plan committed on the parked
`impl/transcript-hidden-fix` branch). Blocked pending a user decision on
e2e hermeticity: (A) keep the real-adapter wiring and rewrite the
acceptance spec's stub-shaped assertions; (B) add a deterministic fixture
and accept losing real-adapter browser coverage; (C) if a fixture is added,
which layer (daemon Rust vs frontend) it lives at. The separable CSS
min-height/collapse fix can land independently of that decision.

## Deferred to todo (2026-07-21)

Deferred out of the ready queue this round per user curation (agent-chat work
not this round); the existing Deferral/blocker note above remains valid.

## Disposition

2026-07-22: Demoted to idea/. The dashboard agent-dogfooding track (agent
activity source, agent-chat real-adapter wiring, and related acceptance) is
deprioritized in favor of completing the dashboard/terminal usability track
first. Rationale: once terminal usability reaches 100%, swapping the
underlying CLI harness is a viable alternative to native agent surfacing, so
finishing the dashboard is the higher-value path now. Prior shipped work
stands (see phase Results above); only unfinished work is parked. Re-promote
when the dashboard/terminal track is complete and agent dogfooding resumes.

## Now blocks a ready ticket (2026-07-24)

Since the 2714 resize-frame fix landed, this failure is no longer masked: it now aborts the serial `dashboard-acceptance.spec.ts` suite at line ~2938 and blocks `260722-refactor-dashboard-app-tsx-leaf-extraction` from reaching its full `test:browser`-green closure bar (and prevents the post-2938 terminal-restore / dockview-split acceptance steps from running at all). Root cause is still untraced (an ancestor — likely Dockview inactive-pane visibility toggling — hides the transcript), so this stays in `idea/` pending investigation; flagging that it now gates a completed ready ticket's closure and is worth prioritizing.

## Suspended (2026-07-25)

Agent-GUI feature suspended per user directive (2026-07-25). The dashboard
agent-chat / Codex-tile UI is hidden and un-spawnable (spawn entry points
disabled behind `AGENT_GUI_SUSPENDED`); its acceptance steps are quarantined.
This ticket is excluded from drain selection until the feature is resumed.
Physical FE+BE module extraction is tracked separately in
`260725-refactor-dashboard-agent-gui-physical-module-isolation`.
