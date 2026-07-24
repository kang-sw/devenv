---
title: Ctrl+Space hotkey swallowed when a terminal pane has focus
---

# Ctrl+Space hotkey swallowed when a terminal pane has focus

## Background

Dogfood observation (owner, 2026-07-24): when a terminal pane has focus, the
`Ctrl+Space` hotkey does not work — it does not reach the dashboard's global
command/hotkey dispatcher.

Hypothesis (not yet code-confirmed; do not deep-investigate at idea stage):
xterm.js keydown handling in the terminal pane captures the chord before it
can be forwarded/bubbled to the global hotkey dispatcher.

Pointers for the ready-ing pass:

- Frontend terminal input/keydown wiring:
  `ws-dashboard/frontend/src/terminalPaneBody.tsx` (xterm `onData`/keydown
  handling).
- Command/hotkey dispatch spine: introduced by
  `260521-feat-ws-dashboard-command-dispatch-spine`.
- Related prior work the fix must fit within (all `.done/`):
  `260722-feat-dashboard-hotkey-config-framework`,
  `260721-feat-dashboard-suppress-browser-shortcuts`,
  `260517-bug-ws-dashboard-terminal-focus-browser-gate-regression`.

## Phases

### Phase 1: Confirm cause and restore Ctrl+Space while a terminal is focused

Confirm whether xterm's keydown handler in `terminalPaneBody.tsx` consumes
`Ctrl+Space` before the global hotkey dispatcher sees it, then fit a fix
into the existing hotkey-dispatch + terminal-focus interaction model
established by the related prior tickets above (no new interaction model).
