---
title: "sage_stamp tells the lead to fix autonomous issues after stamping, which stales the digest it just recorded"
---

# sage_stamp tells the lead to fix autonomous issues after stamping, which stales the digest it just recorded

## Background

`tickets.sage_stamp` computes `sage-review-<stage>-reviewed` as a digest of
the ticket body at stamp time. When the verdicts carry `resolution:
autonomous` issues, its `next_instruction` says "Fix the N autonomous
issue(s) in the ticket yourself first; none of them need a user decision.
Sage review posture recorded but not committed; commit this change via
ws/git.commit". Following that instruction edits the body after the digest
was taken, so the recorded digest is stale the moment the fixes land:
`tickets.verify` warns, and the next `sage_gate` on the ticket returns
`check_review_required` and offers to rerun the reviewers whose findings
were just applied.

Observed 2026-09-10 on the installed 0.45.2 build while promoting
`260909-bug-workflow-cost-measurement-manual-round-three-findings`: combined
stamp returned `autonomous_issues: 7` with the fix-then-commit instruction.
The workaround was to apply the fixes and call `sage_stamp` a second time
with the same verdicts, which refreshes the digest against the fixed body.

## Phases

### Phase 1: Make the instruction match the digest rule

Either (a) change the `autonomous_issues > 0` `next_instruction` to "apply
the fixes, then call `tickets.sage_stamp` again with the same verdicts so
the recorded digest covers the fixed body, then commit", or (b) have the
lead-ticket playbook state the same sequence. Prefer (a): the tool already
owns the digest and the instruction, so the fix is one string and its test
pin. Rejected: skipping the digest when autonomous issues exist — the digest
is what makes a post-stamp fact edit detectable, and the autonomous fixes
are exactly such edits.
