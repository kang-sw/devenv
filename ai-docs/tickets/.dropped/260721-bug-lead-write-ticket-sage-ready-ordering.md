---
title: Lead-write-ticket attempts ready move before the sage gate
related:
  260622-feat-sage-review-ticket-gate: introduced the ready-landing sage gate and tickets.move posture checks
dropped: 2026-07-26
---

# bug: lead-write-ticket attempts ready move before the sage gate

## Observed Behavior

While refreshing `260622-feat-playbook-render-tier-label`, the rendered
`lead-write-ticket` procedure ordered the ready move before its Sage Review Gate.
`tickets.move(stem, to: "ready")` rejected the move because design and
completeness reviews were required, but first wrote
`sage-review-design: required` and `sage-review-completeness: required` into the
todo ticket. The tool returned:

```text
sage-review-design: required; run sage review before promoting to ready
partial-mutation: frontmatter was written before this call blocked
```

## Expected Behavior

The authoring playbook and mutation tools should expose one coherent ordering:
resolve and record the required sage reviews before attempting the ready move,
or make `tickets.move` perform a side-effect-free preflight failure. A blocked
move should not partially modify the ticket without the caller explicitly
requesting that posture write.

## Follow-up Questions

- Should `lead-write-ticket` move the Sage Review Gate before `Move`, then retry
  `tickets.move` after `tickets.sage_record` commits the posture?
- Independently, should `tickets.move` make its unresolved-review rejection
  atomic so failed promotion cannot leave frontmatter mutations?
- Add an integration test that executes the rendered playbook order against a
  todo feature requiring both design and completeness review.


## Resolution (2026-07-26)

Absorbed by `260726-bug-sage-ready-enforcement-single-chokepoint`, which carries
this ticket's evidence (the move-before-gate ordering and the partial-mutation
observation) forward.

Dropped rather than rewritten because the concept changed fundamentally: this
ticket framed the defect as a *sequencing* bug inside `lead-write-ticket`, while
the absorbing ticket treats it as *duplicated enforcement* — sage posture checked
both at the mutation primitives and at the `tickets.verify` / `ws/git.commit`
guardrail — and resolves it by collapsing onto the commit gate. Per ticket
conventions, a fundamentally changed concept gets a new stem.

Partially addressed in the interim: `260713-bug-tickets-move-error-mutates-frontmatter`
(done) added the loud partial-mutation notice this ticket asked for. The absorbing
ticket notes that de-blocking may make that notice dead code.
