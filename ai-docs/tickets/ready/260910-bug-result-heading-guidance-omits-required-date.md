---
title: "Align Result heading guidance with the date required by ticket verification"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 35433d5f4d279db8
sage-review-completeness-reviewed: 35433d5f4d279db8
---

# Align Result heading guidance with the date required by ticket verification

## Background

While closing `260910-chore-session-children-scope-unnoted-filters`, following
both the rendered ticket-worker prompt and `convention.read`'s
`ticket-conventions` example produced `### Result (00f8f976)`.
`tickets.verify` rejected it with `phase-result-heading`, requiring
`### Result (<hash>) - YYYY-MM-DD`. `git.commit` enforces the same rejection.
The close was corrected to the dated form before its commit.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-worker/ticket-worker.md#L79, agents-plugin/rsrc/ticket-worker-escalated/ticket-worker-escalated.md#L79, agents-plugin/rsrc/ticket-worker-elevated/ticket-worker-elevated.md#L79, agents-plugin-wsflow/rsrc/ticket-worker/ticket-worker.md#L79, agents-plugin-wsflow/rsrc/ticket-worker-escalated/ticket-worker-escalated.md#L79, agents-plugin-wsflow/rsrc/ticket-worker-elevated/ticket-worker-elevated.md#L79, agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md#L56 all read `### Result (<short-hash>)` with no date placeholder |
| scope.surface | internal | prompt/convention markdown text only; no exported code symbol changes |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_verify_test.go already asserts the `phase-result-heading` guard (TestTicketVerifyMalformedPhaseAndResultHeadingsAreHardFindings, TestTicketVerifyWellFormedPhaseAndResultHeadingsPass); no new test files named |
| complexity.reuse_points | confirmed | reuses the existing `ticketResultHeadingRE` / `phase-result-heading` guard in agents-plugin-tool/internal/wsdoc/tickets_verify.go#L298-301 as the acceptance check, no new mechanism |
| complexity.side_effect_risk | low | mechanical text edits (append `- YYYY-MM-DD`) across mirrored playbook/convention files, each independently checkable against the existing regex |
| risk.correctness | low | fix is adding the missing date suffix to examples that otherwise already match the enforced pattern |
| risk.fit | low | aligns guidance with a validator that already enforces this exact format; no behavior or semantics change |
| risk.test | low | acceptance criterion is comparing edited text against an already-tested guard, not new test design |
| risk.security_or_contract | low | doc/prompt text only; no code, API, or MCP contract touched |

## Phases

### Phase 1: Align authoring guidance with the existing guard

Find the Result-heading instructions in worker playbooks and bundled ticket
conventions, and make their examples include the date the validator already
requires. Preserve validator semantics and regenerate affected manifests and
mirrors. Verify that a heading copied from the shipped guidance passes the
existing ticket guard.
