---
title: "Visually loud widget rows when a child or the lead is waiting on the owner"
related:
  260908-epic-ws-pi-subagent-conversation-view: specifies prominent rendering for `idle-awaiting-owner` (owner-held children) only, styling left open; this ticket covers the other waits and pins the styling
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: introduces `lastWriter`/owner-held liveness; its `idle-awaiting-owner` rows should get the same treatment
  260905-feat-ws-pi-live-agent-widget: the widget whose `awaiting owner` / `awaiting approval` rows are the surface being changed
  260906-workset-ws-pi-dogfood-ux: polishing board
---

# Visually loud widget rows when a child or the lead is waiting on the owner

## Background

Owner request (2026-09-08): when an agent is waiting on the owner — a
`ws-ask` question, a `ws-approve` approval, an owner-held child that
went `idle-awaiting-owner` — the below-editor widget should look loud
enough that the owner cannot miss it: a rainbow or pulsing highlight,
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
  style. The footer `ws: N agents` status segment gets the same
  emphasis while any such row exists.
- Style: an animated highlight on the row (cycling hue across the row
  text, or a two-phase pulse between the theme's accent and warning
  colors), with the elapsed clock kept. The widget already uses the
  `(tui, theme) => Component` factory overload, so the component owns a
  timer (a few hundred ms, only while a waiting row exists) and calls
  the TUI's re-render request; the 10 s elapsed tick stays as is.
- Off switch: a single adapter config flag (harness config layer,
  `260905-feat-ws-pi-harness-config-layer`) that falls back to a static
  bold/colored row for owners who find animation distracting or run in a
  terminal that renders it badly.
- Children of the lead (forks, workers) do not animate; only the owner's
  own Pi does.

## Open questions

- Color source: the factory's `theme` exposes `fg(ThemeColor, text)` and
  `bold` over the named palette (`accent`, `warning`, `error`, `success`,
  ...), and `render(width)` returns raw strings, so a hue cycle can use
  either the palette (theme-safe, few steps) or direct truecolor escapes
  (smooth, but ignores the theme). Decide which at implementation time.
- Whether the animation should stop after some minutes and settle into
  the static style, to avoid a permanently flashing row on a long wait.
- Whether the lead's own turn end (Pi idle, waiting for the owner's next
  message) deserves the same treatment; Pi itself gives no cue.
