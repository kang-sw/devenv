---
title: Lead-write-ticket attempts ready move before the sage gate
related:
  260622-feat-sage-review-ticket-gate: introduced the ready-landing sage gate and tickets.move posture checks
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
