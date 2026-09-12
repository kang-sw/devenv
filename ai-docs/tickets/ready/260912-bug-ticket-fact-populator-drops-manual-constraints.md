---
title: "Ticket fact population drops path-scoped manual constraints"
related:
  260912-feat-git-merge-generic-branch-promotion: dogfood source
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 8105785d2d93898f
sage-review-completeness-reviewed: 8105785d2d93898f
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

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md, agents-plugin-wsflow/rsrc/ticket-fact-populator/ticket-fact-populator.md, agents-plugin-tool/internal/mcp/ticket_review_design_test.go |
| scope.surface | public-interface | the rendered ticket-fact-populator instructions are a shipped delegate contract at agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md#L9-L42 |
| scope.new_public_symbol | no | no new MCP tool or exported symbol is requested |
| scope.new_type_contract | no | the existing ticket-fact-populator output format is extended in place |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/ticket_review_design_test.go#L99-L124 renders both package variants; no targeted constraint-preservation assertion exists |
| complexity.reuse_points | confirmed | the existing Constraints replacement flow at agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md#L35-L41 is the preservation point |
| complexity.side_effect_risk | moderate | the change rewrites ticket Constraints content during ready preparation |
| risk.correctness | moderate | every path-matched convention must be retained and reported consistently |
| risk.fit | moderate | both shipped package variants must retain identical fact-population behavior |
| risk.test | moderate | a new regression assertion must distinguish preserved constraints from a zero-mutation report |
| risk.security_or_contract | moderate | delegate output and its correction summary are a caller-visible workflow contract |

## Phases

### Phase 1: Preserve and report manual constraints during fact population

Make fact population preserve valid authored manual constraints and add any
missing path-matched constraints. Its report must count or otherwise surface a
constraint mutation instead of reporting zero corrections after removing one.

Verification reproduces the behavior with a ticket spanning MCP and shipped
playbook paths and asserts both the resulting Constraints section and the
reported correction summary.
