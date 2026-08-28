---
title: sage_stamp leaves a stale Blocked section after a passing re-review
related:
  260828-refactor-per-slice-review-relay: dogfood case — design block was corrected and then passed, but its rendered Blocked section remained
---

# bug: sage_stamp leaves a stale Blocked section after a passing re-review

## Observed Behavior

`tickets.sage_stamp(..., verdict: block)` writes `sage-review-design: blocked`
and appends a `## Blocked` section. After the ticket is corrected and a fresh
design reviewer returns `pass`, a second `tickets.sage_stamp` updates the
frontmatter to `sage-review-design: completed` but leaves the earlier
`## Blocked` section in the ticket body.

## Expected Behavior

When a newly stamped passing verdict clears a stage's blocked posture, the
Go-owned blocked section for that stage is removed or rewritten so the ticket
does not simultaneously claim completed review and display an unresolved block.

## Follow-up Questions

- Does the same stale-section behavior occur for completeness and combined
  stamps?
- Should clearing a blocked section be covered by the same tests that assert
  block-section rendering?
