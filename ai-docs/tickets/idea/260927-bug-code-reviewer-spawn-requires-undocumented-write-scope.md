---
title: "Document or automate the required review artifact scope for code reviewers"
---

# Document or automate the required review artifact scope for code reviewers

## Background

During a `lead-review` release-range sweep, the rendered `code-review-correctness`, `code-review-fit`, and `code-review-test` procedures instructed the lead to spawn reviewers but did not supply or request an artifact path. `ws-agent-spawn` rejected all three dispatches with `code reviewer requires exactly one file write scope`. The lead had to infer the missing `ws/path.generate(kind: "review")` step, add an output-path instruction, and bind that exact file as the sole write scope.

The rendered review procedure and the Pi spawn enforcement should agree so the documented happy path does not fail at dispatch.

## Phases

### Phase 1: Align code-reviewer dispatch guidance with spawn enforcement

Determine whether review playbooks should declare and receive a generated artifact path automatically or whether `lead-review` must explicitly generate and bind it. Update the authoritative procedure and tests so all partition reviewer dispatches satisfy the exactly-one-file requirement on their first attempt.
