---
title: whiteboard - a lead-confirmed status for items settled without a user answer
related:
  - 261105-feat-lead-ticket-review-auto-ack
  - 261030-feat-session-whiteboard-mode
---

# whiteboard - a lead-confirmed status for items settled without a user answer

## Background

`261105-feat-lead-ticket-review-auto-ack` lets the lead confirm a
review-raised announced or technical item without a user acknowledgement
round, reporting it as `(nn) auto-confirmed`. With
`workflow.whiteboard_enabled` on, such an item stays `open` on the session
whiteboard: `whiteboard.update`'s `answer` requires `answer_quote`, the
user's verbatim words, and an auto-confirmed item has none. The auto-ack
ticket put recording it out of scope ((141), user decision 2026-11-05)
because it changes the whiteboard store contract.

## Open questions

- A new status (for example `confirmed-by-lead`) settled like `answered`,
  or an `answer` variant that allows a missing `answer_quote` when flagged?
- How the page shows it, so the user can still object and reopen the item
  under its ID ((127) in the auto-ack ticket).
