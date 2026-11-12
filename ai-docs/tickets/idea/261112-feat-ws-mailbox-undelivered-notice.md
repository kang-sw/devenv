---
title: "Mailbox: notify the sender when a message is not read within 3 minutes"
related:
  261111-feat-ws-mailbox-wait-without-process-relaunch: the mailbox wait path a wake notice would ride
---

# Mailbox: notify the sender when a message is not read within 3 minutes

## Background

User feedback (whiteboard, 2026-11-12), recorded as feedback only; nothing
here is a settled decision yet.

A session sent mail to a peer whose session had already exited, and waited
all night without knowing the message never arrived. The user asked that
when a sent message is not delivered within 3 minutes, the sender receive a
"message not delivered" notice that wakes it for a new turn. Delivered means
the recipient agent successfully read the message, not that it replied.

## Open Questions

- What counts as a successful read (a `mailbox.recv` claim of that message,
  a push delivery acknowledged by the adapter).
- Who runs the 3-minute timer when the sender itself is idle (the sender's
  mailbox wait, the dashboard daemon) and how the notice wakes the sender.
- Whether the timeout is configurable, and whether a later read after the
  notice sends a follow-up "delivered" notice.
- Tracker or store shape changes and their compatibility tier per
  `ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Undelivered-mail notice

To be settled at capture through the Open Decision Queue.
