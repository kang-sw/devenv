---
title: "Review a ready-promotion batch with one cross-ticket design reviewer"
related:
  260911-research-batch-promotion-cross-ticket-coherence-gap: source research; batch-wide design review was a listed direction (ai-docs/tickets/idea/260911-research-batch-promotion-cross-ticket-coherence-gap.md#L33-L49)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 9ad31bd58d5a7d10
sage-review-completeness-reviewed: 9ad31bd58d5a7d10
completed: 2026-09-12
---

# Review a ready-promotion batch with one cross-ticket design reviewer

## Background

`lead-ticket` promotes related tickets as a batch but currently runs Sage review
per ticket. A design reviewer therefore cannot detect contradictions, duplicate
scope, or collisions between sibling tickets that enter `ready/` together. The
triggering research recorded that an ad-hoc batch-wide design review was needed
when three sibling tickets all changed `lead-run`.

## Decisions

- A promotion batch containing multiple actionable tickets gets one design
  review over the complete batch instead of one isolated design review per
  ticket.
- The reviewer returns a design verdict for every review-eligible ticket plus a
  cross-ticket coherence verdict covering contradictions, duplicated scope,
  dependency mistakes, and overlapping implementation surfaces.
- A ticket whose effective design posture is `skipped` participates in a batch
  review as context only. It receives no design verdict or design stamp, and
  the batch review does not override its configured skip posture.
- Completeness review remains per ticket because it evaluates whether each
  individual ticket is executable and structurally sufficient.
- A single-ticket promotion preserves the existing single-ticket Sage path.
- The first batch design review evaluates the complete batch. A follow-up after
  fixes is a pass-preserving delta review over changed tickets and their
  dependency or collision edges; previously passed tickets are accepted
  baseline context rather than fresh review targets.
- A follow-up may reverse a previously passed ticket only by naming the changed
  ticket or relation, citing the concrete premise in the passed ticket, and
  explaining how the change invalidates that premise. Other newly noticed
  concerns become separate follow-up findings and do not change the current
  batch verdict.
- Every cross-ticket finding names its `affected_stems`. A blocking
  cross-ticket finding pauses the whole promotion batch so no partial landing
  occurs, but only affected tickets receive blocked design verdicts. After a
  fix, the delta-review rule above governs the retry.

## Constraints

- Every ticket is fact-populated before the batch design reviewer reads it, and
  every resulting stamp covers the same ticket body the reviewer evaluated.
- Preserve the existing public MCP tool inventory and the per-ticket
  `tickets.sage_gate` and `tickets.sage_stamp` authority boundaries. This ticket
  does not authorize a new MCP tool.
- Preserve configured Sage postures, blocked-ticket handling, dependency-order
  checks, and the lead-only stamp writer.
- A context-only skipped ticket may inform coherence findings for review-eligible
  tickets, but it is never an affected verdict or stamp target.
- Keep completeness findings and cross-ticket design findings distinct in
  reviewer output and blocked-ticket diagnostics.
- Pass the reviewer the changed stems and the previously passed stems on a
  follow-up so the delta boundary is explicit rather than inferred from prose.
- Apply the shipped-surface, skill-authoring, and wsflow-mirroring manuals to
  every matching changed path.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md, agents-plugin-tool/internal/wsdoc/tickets_sage.go |
| scope.surface | cross-module | ready-promotion playbook, reviewer contract, and per-ticket stamp handling change together |
| scope.new_public_symbol | no | preserves the MCP tool inventory and existing tickets.sage_gate and tickets.sage_stamp tools |
| scope.new_type_contract | yes | batch design-reviewer output gains per-review-eligible-ticket verdicts and a cross-ticket coherence verdict |
| scope.test_surface | existing | agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go, agents-plugin-tool/internal/wsdoc/tickets_sage_test.go, agents-plugin-tool/internal/mcp/tickets_sage_test.go |
| complexity.reuse_points | confirmed | existing per-ticket promotion flow and Sage record handling in agents-plugin/rsrc/lead-ticket/lead-ticket.md#L105-L129 and agents-plugin-tool/internal/wsdoc/tickets_sage.go#L442-L540 |
| complexity.side_effect_risk | moderate | changes the review and promotion order for multi-ticket ready batches |
| risk.correctness | high | an incorrect per-ticket mapping of a cross-ticket conflict can promote conflicting tickets or block unaffected ones |
| risk.fit | moderate | the source research leaves the cross-ticket verdict-to-stamp policy open |
| risk.test | high | coverage must distinguish single-ticket, clean-batch, per-ticket-failure, cross-conflict, and mixed-posture outcomes |
| risk.security_or_contract | moderate | reviewer output and stamping behavior change while the public MCP inventory and authority boundaries must remain stable |

## Phases

### Phase 1: Add batch-wide design review to ready promotion

Extend the ready-promotion orchestration and design-reviewer contract so one
reviewer evaluates every ticket in a multi-ticket batch and returns both
per-review-eligible-ticket and cross-ticket outcomes. Continue to run
completeness review per ticket, map reviewer outcomes into the existing
per-ticket Sage stamps, and add
contract coverage for single-ticket preservation, clean batches, one-ticket
design failures, cross-ticket conflicts, mixed configured postures, affected
stem mapping, skipped members as context-only without stamp mutation, pass
preservation, premise-invalidating reversals, and delta re-review convergence.

### Result (17d7b8cb) - 2026-09-12

Implemented one design-review dispatch for multi-ticket promotion, explicit
eligible/context-only membership, per-ticket verdict mapping, cross-ticket
affected-stem diagnostics, and pass-preserving delta review. Completeness stays
per ticket. Promotion moves occur only after the entire batch settles, with
rollback to original statuses if a later move fails.

The existing Sage gate and stamp API needed no runtime changes. The lead keeps
individual review evidence independently of combined blocked postures and may
reuse an unchanged completeness pass with a fresh design verdict to clear both
postures. Skipped design stages receive no verdict or stamp. Canonical resources
and the generated wsflow mirror carry the same contract.

Verification: `go test ./...`, `go build ./cmd/ws-mcp`,
`scripts/smoke-ws-mcp.sh ..`, and
`python3 -m unittest discover agents-plugin-wsflow/tests` passed. Rendered
contract tests cover both products; mapped Sage fixtures cover clean batches,
ticket-local failures, cross-ticket conflicts, skipped context, unaffected
stamps, and combined delta recovery. These tests validate prompt delivery and
the existing stamp boundary, not a live model's design judgment.

Independent correctness, fit, and test review completed. Two Important findings
were fixed in `55f7c72e` and verified in round 2: retained completeness evidence
during delta recovery, and distinct local versus cross-ticket failure fixtures.
The fresh-reader audit's prior-report ambiguity was also clarified. No findings
remain unresolved; no scope was deferred.
