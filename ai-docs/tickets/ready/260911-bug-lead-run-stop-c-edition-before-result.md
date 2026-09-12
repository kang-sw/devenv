---
title: "lead-run stop (c) requires an Edition even when the executed phase has no Result"
related:
  260911-feat-impl-derivation-hardening-branch-aware-select: dogfood incident; worker stopped before edits because Phase 3's pre-Select contract was structurally impossible
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 5970418bf9bb0980
sage-review-completeness-reviewed: 5970418bf9bb0980
---

# lead-run stop (c) requires an Edition even when the executed phase has no Result

## Background

During `ws:lead-run` dogfooding, a ticket worker raised stop (c) before making
any source edit or writing a phase Result. The lead-run playbook nevertheless
requires the proposed resolution to be recorded as an `#### Edition` on the
executed phase. Ticket conventions allow an Edition only for a later pass under
an already completed phase's Result; an unimplemented phase remains editable
directly. Following the stop instruction literally would create an invalid
ticket shape, while following the convention would violate the lead-run
recovery procedure.

## Decisions

- When the executed phase has no `### Result`, revise that unimplemented phase
  directly under the raised design-review gate.
- When the executed phase already has a `### Result`, append an `#### Edition`
  under the existing Result area.
- Preserve the raised design-review gate and worker-resume behavior in both
  cases.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-run/lead-run.md and its coverage in agents-plugin-tool/internal/mcp/playbook_tools_test.go |
| scope.surface | public-interface | lead-run is a shipped playbook whose stop-(c) recovery is caller-visible |
| scope.new_public_symbol | no | no new named symbol or playbook is proposed |
| scope.new_type_contract | no | the change selects ticket lifecycle handling; it adds no type or signature |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go renders lead-run for existing coverage |
| complexity.reuse_points | confirmed | reuse the existing Result/Edition lifecycle in agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md#L111-L117 and the existing lead-ticket route |
| complexity.side_effect_risk | moderate | recovery determines whether a stopped worker resumes after a raised design review |
| risk.correctness | moderate | the no-Result and existing-Result branches must preserve valid ticket lifecycle semantics |
| risk.fit | low | the settled branches directly reconcile the current lead-run instruction with ticket conventions |
| risk.test | moderate | coverage must distinguish stops before a Result from follow-up stops on a completed phase |
| risk.security_or_contract | moderate | this changes the shipped stop-(c) recovery contract between lead-run and lead-ticket |

## Phases

### Phase 1: Align stop-(c) recovery with phase lifecycle

Implement the settled Result-sensitive recovery branches and add coverage for
stops raised both before the first Result and after a Result exists.
