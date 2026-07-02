---
kind: print
delegates: false
includes:
  - sample-conventions
variables:
  - WorktreeID
---
# Sample Playbook

This is the base variant of the sample playbook.
It exercises the rsrc loader substrate: auto-include, variable substitution,
and frontmatter parsing.

Current worktree: {{.WorktreeID}}
