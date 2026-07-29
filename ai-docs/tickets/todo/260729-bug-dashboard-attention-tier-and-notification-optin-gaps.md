---
title: Attention tiers disagree about `working`, and a dismissed permission leaves the opt-in on
sage-review-design: required
---

# Attention tiers disagree about `working`, and a dismissed permission leaves the opt-in on

## Background

Found by code review of PR #4's agent-attention frontend
(`goal/ws-dashboard-dev/velvet-arbor-quill`, merged as `1b41a37b`). The pipeline
reviewed clean on reconnect, teardown, per-server isolation and secure-context
handling; these are the remaining rough edges.

### 1. Tier 1 treats `working` as "Attention needed"; Tier 2 deliberately does not

`App.tsx` fires the title flash on any non-null `globalAttentionTone`, and
`browserAttentionCue.ts` renders `"● Attention needed - ws dashboard"`. But
`globalAttentionTone` is `"working"` whenever any unacknowledged agent is
mid-turn. Tier 2 explicitly excludes `working` as "normal background progress,
not an actionable interruption".

Scenario: the user spawns an agent, it starts a 20-minute turn, the user switches
browser tabs — the dashboard tab title flashes "Attention needed" at 1 Hz for the
full 20 minutes and stops only when the user returns and clicks the tab.

`e2e/agent-attention-indicator.spec.ts` pins this as intended (it drives the flash
with a `working` POST), so it is a choice rather than a slip. The two tiers now
disagree about what `working` means; either record the divergence as deliberate
with its reason, or align them.

### 2. A dismissed permission prompt leaves the opt-in persisted for a tier that can never fire

`settingsSections.tsx`'s `NotificationSection` `onChange` reconciles only on
`permission === "denied"`. Chrome resolves `"default"` when the user dismisses the
prompt (Esc or click-away) instead of choosing.

Scenario: the user checks the box and dismisses the prompt. The box stays
checked, `{"version":1,"value":{"enabled":true}}` stays in localStorage, and no
notification ever fires. The only clue is "Current permission: default" in the
note. Gating on `permission !== "granted"` closes it.

This has a test twin: `e2e/agent-attention-notification.spec.ts` hedges the
permission read ("not brittle if the measured value is 'default'") but then
asserts `PREFS_DISABLED`, which only holds on `"denied"`. If Chrome for Testing
ever dismisses rather than denies, that spec fails while the app behaves exactly
as written.

### 3. Minor items in the same area

- The title flash ignores `prefers-reduced-motion`. Every new CSS animation is
  correctly listed in the trailing reduce block, but the flash is a
  `window.setInterval` in `App.tsx` and is unaffected — a reduced-motion user
  still gets a flashing tab title.
- `App.tsx`'s EventSource error path deletes by route key rather than by source
  identity. If an error event for source A arrived after the eligibility loop had
  already closed A and installed B for the same route, the delete would drop B
  from the map while B stayed open. `close()` suppresses further events so this is
  very hard to reach; `if (sources.get(serverRoute) === source)` would make it
  structurally impossible.
- `package.json`'s `test:browser` now builds the frontend twice — once in the
  script, once in Playwright's new unconditional `globalSetup`. Dropping the
  npm-script half leaves one build surface.

## Phases

### Phase 1: Settle the `working` tone contract across both tiers

Decide whether `working` is an interruption. Whichever way it goes, both tiers and
the e2e expectation must agree, and the reason belongs in the spec — this is the
kind of divergence that gets "fixed" back and forth otherwise.

### Phase 2: Close the permission-dismiss gap and the minor items

Gate on `permission !== "granted"`, and fix the e2e spec's matching assumption at
the same time so the two cannot drift apart again.
