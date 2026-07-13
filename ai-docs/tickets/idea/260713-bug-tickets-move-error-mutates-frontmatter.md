---
title: "Prevent failed tickets.move promotion from mutating frontmatter"
---

# Prevent failed tickets.move promotion from mutating frontmatter

## Observation

On 2026-07-13, `tickets.move(..., to: "ready")` returned a blocking
`sage-review-completeness: required` error but first wrote that posture into the
todo ticket's working-tree frontmatter. A retry after an external completeness
review then produced a duplicate key when the caller reasonably added
`sage-review-completeness: completed` near the other Sage field.

## Expected

A mutation tool that reports failure should either leave the ticket unchanged or
return an explicit partial-mutation result that identifies the written posture.
Investigate whether validation can precede persistence, or whether the MCP result
and playbook must make the intentional self-healing write contract explicit and
duplicate-safe.
