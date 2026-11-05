---
title: sage_stamp and tickets.move next_instruction text points the lead backwards
---

# sage_stamp and tickets.move next_instruction text points the lead backwards

## Background

Dogfood observation while promoting
`261105-feat-whiteboard-explicit-item-ref-syntax` (2026-11-05):

- `tickets.sage_stamp(stage: combined)` with pass verdicts carrying
  autonomous issues returned "Fix the 7 autonomous issue(s) ... call
  ws/tickets.sage_stamp again with the same verdicts". The lead had already
  applied the fixes before the first stamp. The re-stamp returned the same
  instruction verbatim, so the caller cannot tell a covered body from an
  unfixed one and the instruction reads as a loop.
- `tickets.move(to: ready)` then returned "Call tickets.sage_gate(stem,
  landing) next to resolve the sage-review gate for this promotion", although
  `lead-ticket` resolves the gate before the move and the gate had already
  run. A follow-up `sage_gate` returned `skip`, so no harm resulted, but the
  text contradicts the playbook's order.

## Open questions

- Should `sage_stamp` report whether the recorded digest already covers the
  current body (for example, a re-stamp over an unchanged body says so and
  drops the fix-then-restamp instruction)?
- Should `tickets.move` omit the gate instruction when the ticket's sage
  posture is already completed for the ready landing?
