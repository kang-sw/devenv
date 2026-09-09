---
title: "Visually loud widget rows when a child or the lead is waiting on the owner"
spec:
  - pi-adapter-runtime
related:
  260908-epic-ws-pi-subagent-conversation-view: specifies prominent rendering for `idle-awaiting-owner` (owner-held children) only, styling left open; this ticket covers the other waits and pins the styling
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: introduces `lastWriter`/owner-held liveness; its `idle-awaiting-owner` rows should get the same treatment
  260905-feat-ws-pi-live-agent-widget: the widget whose `awaiting owner` / `awaiting approval` rows are the surface being changed
  260906-workset-ws-pi-dogfood-ux: polishing board
  260909-feat-ws-pi-agent-count-panel-header: coordinate the single count destination; independent implementation
---

# Visually loud widget rows when a child or the lead is waiting on the owner

## Background

Owner request (2026-09-08): when an agent is waiting on the owner — a
`ws-ask` question, a `ws-approve` approval, an owner-held child that
went `idle-awaiting-owner` — the below-editor widget should look loud
enough that the owner cannot miss it: a visibly animated attention cue,
not a plain row. This is about visual salience inside the TUI; no sound
or OS notification is asked for.

What exists today (`agent-widget.ts`): `awaiting owner` and `awaiting
approval` rows sort first, are never trimmed by the display cap, carry
the `/answer <id>` hint, and repaint every 10 s for their elapsed clock.
They are styled like any other row.

Checked against existing tickets: the conversation-view epic says
`idle-awaiting-owner` "is rendered prominently — clearly more salient
than the other two ... exact styling is implementation detail", and only
for owner-held children; the ask question row and the approval row are
not mentioned. So this is uncovered for those two, and the styling is
unpinned for the third.

## Direction

- Treat all three waits the same: `awaiting-owner` (ask question),
  `awaiting-approval` (ws-approve), and `idle-awaiting-owner` (owner-held
  child, once the audit-window ticket lands) share one "needs the owner"
  style. The agent-count segment gets the same
  emphasis while any such row exists; `260909-feat-ws-pi-agent-count-panel-header`
  moves that segment above the list, so do not resurrect its old footer copy.
- **Owner-confirmed animation (2026-09-09).** Toggle the
  `/answer <title>` attention text between bold and ordinary weight every
  **0.33 seconds (330ms)**. The owner explicitly wants a noisy, conspicuous
  cue. This supersedes hue cycling and accent/warning color pulses; the text
  remains visible in both states. Preserve the elapsed clock. Stop the timer
  when no qualifying wait remains or the widget/session is torn down.
- Off switch: a single adapter config flag (harness config layer,
  `260905-feat-ws-pi-harness-config-layer`) that falls back to a static
  bold/colored row for owners who find animation distracting or run in a
  terminal that renders it badly.
- Children of the lead (forks, workers) do not animate; only the owner's
  own Pi does.

## Ready-preparation decisions still open

- The requested `/answer <title>` is a display phrase; the existing command
  resolves a question ID (`qN`). Confirm how the visible title and valid
  command hint coexist before committing the final interaction contract.
- Approval rows have no question target, and future owner-held idle rows may
  likewise have no `/answer` thread. Confirm the equivalent emphasis text for
  those waits, or explicitly limit this slice to question waits.
- The original timeout-to-static and ordinary lead-idle expansion questions
  are not settled by the 330ms animation choice. Keep ordinary lead-idle
  notifications outside implementation until their scope is confirmed.

## Spec Impact

Add the confirmed bold/plain attention effect and its final trigger/label
rules to the live-agent-widget section of `pi-adapter-runtime` when implemented.
Do not expose ws-ask/ws-resolve again; fork-raised owner questions remain an
existing source of real question rows. Ready promotion awaits the label/trigger
clarification above and the ordinary independent review.
