---
title: Reviewer playbook render rejects the lead-implement review frame context
related-mental-model:
  - mcp-runtime
  - workflow-skills
---

# Reviewer playbook render rejects the lead-implement review frame context

## Background

During the ticketless Sage freshness implementation review on 2026-08-28,
`playbook.render` rejected the ordinary `lead-implement` reviewer-frame inputs
(`target_kind`, `inline_contract`, and `CommitRange`) as undeclared variables.
The reviewer playbooks therefore cannot be rendered with the procedure's stated
context contract, despite the review stage requiring those inputs.

## Phases

### Phase 1: Align reviewer render declarations and the review-frame contract

Make the declared variables accepted by the reviewer playbooks match the
`lead-implement` review frame, or change the frame to the actual supported
interface. Verify all correctness, fit, and test reviewer renders for both
ticket and inline targets. Preserve the declared-variable guard; do not weaken
render validation merely to bypass this mismatch.
