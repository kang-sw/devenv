---
title: "Pi Esc drops queued mailbox push batches"
---

# Pi Esc drops queued mailbox push batches

## Background

Found by the delta design review of `261107-feat-ws-dashboard-interrupt-badge`
(2026-11-08), outside that ticket's scope. The Pi adapter pushes mailbox mail
mid-run as custom messages through `sendMessage`, which queues them as steers
(`@earendil-works/pi-coding-agent` `agent-session.js` around line 1117). When
the user presses Esc in the Pi TUI, the abort handler
(`interactive-mode.js` `restoreQueuedMessagesToEditor({abort: true})`) calls
`session.clearQueue()` -> `agent.clearAllQueues()`: untaken text steers move to
the editor, but queued custom messages are dropped silently. The mail was
already drained by `mailbox.recv`, so it is lost.

Not reproduced live; source-read only against the installed Pi dist.

## Idea

Track push batches the run has not taken and re-deliver them after an abort
(for example as `deliverAs: "nextTurn"`), or return them to the mailbox. The
interrupt ticket's Pi decision (board (269)) may build the same tracking;
share it if so.
