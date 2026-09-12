---
title: "Ticket fact population drops path-scoped manual constraints"
related:
  260912-feat-git-merge-generic-branch-promotion: dogfood source
---

# Ticket fact population drops path-scoped manual constraints

## Background

During ready preparation for the related generic merge ticket, the authored
`## Constraints` section contained all four manuals required by the repository's
Implementation Conventions for the anticipated MCP and shipped-playbook paths.
The fact-populator edit added Route Facts but removed those four convention
lines, then reported `corrections: 0 applied`, `decision_gaps: 0`, and
`unverified: 0`.

This contradicts the repository rule that fact population copies every matching
Implementation Conventions row into the ticket's constraints and makes the
reported correction count incomplete.

## Phases

### Phase 1: Preserve and report manual constraints during fact population

Make fact population preserve valid authored manual constraints and add any
missing path-matched constraints. Its report must count or otherwise surface a
constraint mutation instead of reporting zero corrections after removing one.

Verification reproduces the behavior with a ticket spanning MCP and shipped
playbook paths and asserts both the resulting Constraints section and the
reported correction summary.
