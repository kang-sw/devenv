---
title: "Attention alert (terminal bell / desktop notification) when a child or the lead is waiting on the owner"
related:
  260908-epic-ws-pi-subagent-conversation-view: owns the in-TUI rendering of `idle-awaiting-owner`; this ticket adds an out-of-band alert on the same transitions, not another widget state
  260908-feat-ws-pi-subagent-audit-window-and-owner-steering: introduces `lastWriter`/owner-held liveness; the alert keys off the liveness it defines
  260905-feat-ws-pi-live-agent-widget: the `awaiting owner`/`awaiting approval` rows that today are the only signal
  260906-workset-ws-pi-dogfood-ux: polishing board
---

# Attention alert (terminal bell / desktop notification) when a child or the lead is waiting on the owner

## Background

Owner request (2026-09-08): agents waiting on the owner — a `ws-ask`
question, a `ws-approve` approval, an owner-held child that went
`idle-awaiting-owner` — are announced only inside the TUI: a widget row
(`agent-widget.ts`, `awaiting owner` / `awaiting approval`) and, per the
conversation-view epic, a prominent row style plus a `ctx.ui.notify` toast
on owner-held settles. All of that is invisible when the terminal is not
in front of the owner. The owner wants these waits to be "louder".

Checked against existing tickets: the conversation-view epic and the
audit-window child render the wait prominently but say nothing about an
audible or OS-level signal, so this is not covered. Pi itself
(0.85.1) has no bell or notification setting: the only terminal sequence
it emits is the OSC 9;4 progress indicator (`pi-tui` `terminal.js`,
behind `showTerminalProgress`).

## Direction

- Fire an alert on the transitions where an agent starts waiting on the
  owner: a `ws-ask` question registered, a `ws-approve` request pending,
  a child entering `idle-awaiting-owner` (once the audit-window ticket
  lands), and optionally the lead's own turn end (Pi has no such alert
  either). One alert per transition, never repeated while the wait
  persists.
- Channels, by preference and platform availability: terminal BEL
  (`\x07`, written by the extension process to the TTY; most terminals
  map it to a sound or a dock bounce), then an OSC 9 / OSC 777 desktop
  notification for terminals that support it (iTerm2, WezTerm, kitty,
  ghostty), then a macOS user notification via `osascript` as the last
  resort. Configurable per channel and off by default for children of
  the lead (forks, workers) so only the owner's own Pi rings.
- Configuration lives in the adapter's harness config layer
  (`260905-feat-ws-pi-harness-config-layer`) rather than a new file.

## Open questions

- Whether Pi's TUI swallows a BEL written outside its render path, and
  whether it should go through `ctx.ui` instead (no such API today).
- Rate limiting when several children wait at once.
- Whether the owner wants the lead's own turn end to ring by default.
