---
title: "Reconcile lead-review fix handoff with ticket-only lead-run"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c736ff9759cea328
sage-review-completeness-reviewed: c736ff9759cea328
---

# Reconcile lead-review fix handoff with ticket-only lead-run

## Background

The `lead-review` NEEDS FIX procedure tells the lead to generate a review
artifact and invoke `lead-run` with that artifact as the implementation
contract. The current `lead-run` procedure is ticket-only: it point-resolves a
ticket stem, requires ticket Route Facts, and rejects `completion: ad_hoc`.
During the 2026-09-12 release sweep this made the documented direct handoff
unexecutable and required an additional ticket-authoring pass.

## Decisions

- Keep `lead-run` ticket-only; do not restore an ad-hoc review-artifact route.
- Route a local NEEDS FIX through `lead-delegate` with the generated review
  artifact when the repair is bounded and eligible for delegation.
- Apply `lead-delegate`'s existing routing gate. A repair that affects public
  behavior, an API, protocol, schema, template, canonical flow, architecture,
  or an unresolved decision is captured through `lead-ticket` and then
  executed through `lead-run`.
- For this handoff, the required follow-up `lead-review` supplies independent
  review. That criterion alone does not reroute an otherwise bounded local
  repair from `lead-delegate` to `lead-run`.
- Run `lead-review` again after the local repair so the original findings and
  reviewed range receive independent verification.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-review/lead-review.md, agents-plugin/rsrc/lead-delegate/lead-delegate.md, and agents-plugin/tests/fixtures/lead_delegate_contract.json |
| scope.surface | public-interface | lead-review, lead-delegate, and lead-run are shipped skill procedures |
| scope.new_public_symbol | no | no new symbol is proposed |
| scope.new_type_contract | no | no type or signature change is proposed |
| scope.test_surface | existing | agents-plugin/tests/fixtures/lead_delegate_contract.json covers the lead-delegate routing contract |
| complexity.reuse_points | confirmed | reuse the existing lead-delegate routing gate in agents-plugin/rsrc/lead-delegate/lead-delegate.md |
| complexity.side_effect_risk | moderate | changes the review remediation handoff |
| risk.correctness | moderate | the repair must preserve routing and re-review behavior |
| risk.fit | moderate | the change must align lead-review with the settled lead-delegate gate |
| risk.test | moderate | contract coverage must distinguish bounded repair from escalation |
| risk.security_or_contract | high | this changes the shipped workflow contract between lead-review, lead-delegate, and lead-run |

## Phases

### Phase 1: Reconcile the review-to-fix handoff contract

Replace the invalid local artifact-to-`lead-run` handoff with the settled
`lead-delegate` route and its escalation to `lead-ticket`. Preserve contributor
handoff behavior and add contract coverage for bounded local repair, material
repair escalation, and re-review after the repair.
