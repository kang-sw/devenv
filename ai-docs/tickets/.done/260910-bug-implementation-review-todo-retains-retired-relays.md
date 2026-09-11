---
title: Align implementation review todos with the current ticket-worker protocol
parent: 260909-epic-ws-worker-interpreter-refoundation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: efe5ffc9f7191f3b
sage-review-completeness-reviewed: efe5ffc9f7191f3b
completed: 2026-09-11
---

# Align implementation review todos with the current ticket-worker protocol

## Background

During execution of `260910-refactor-ready-only-actionable-ticket-gates`,
`route.resolve_implement` installed a partitioned review todo that still tells
the worker to use `implementer-relay`, permits three reviews, and escalates a
remaining Critical finding through `implementer-elevated`. The rendered
`ticket-worker` instead owns its fixes, allows two review rounds, and stops on
an unresolved Critical finding after round two.

The obsolete clauses are emitted by `implementReviewRelayClause` and
`implementReviewCriticalBranchClause` in
`agents-plugin-tool/internal/mcp/session_state.go`; tests in
`session_state_test.go` currently pin them. The current prompt contract is in
`agents-plugin/rsrc/ticket-worker/ticket-worker.md` and its worker-protocol
include. A caller following the installed todo can take a different review
path from the playbook that created the route.

## Decisions

- The runtime review todo is realigned to the shipped `ticket-worker`
  protocol, not the other way around: the worker owns its own fixes (no
  `implementer-relay` delegate), review runs at most two rounds, and an
  unresolved Critical finding after round two is a stop, never a silent
  elevation to `implementer-elevated`. The user authorized this review-
  semantics change; it removes a live contradiction between the installed
  todo and the playbook that spawned the worker.
- Independent review is retained. The change reconciles the round budget and
  fix ownership only; it does not remove the reviewer dispatch or the
  partitioned review allocation.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/session_state.go, agents-plugin-tool/internal/mcp/session_state_test.go |
| scope.surface | internal | implementReviewRelayClause, implementReviewCriticalBranchClause, implementReviewInstruction are unexported (session_state.go#L483-L514); no MCP tool schema change |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | session_state_test.go pins the current clause text (session_state_test.go#L204, L208-L209, L1928-L1929) |
| complexity.reuse_points | confirmed | reuses the existing reviewer-dispatch assembly in implementReviewInstruction (session_state.go#L503-L514); implementReviewDispositionClause is rewritten, not reused verbatim, since its relay / "Important is never re-reviewed" text (session_state.go#L481) must align to the two-round model |
| complexity.side_effect_risk | moderate | rewritten clauses feed all three review-allocation shapes (partitioned, single, default) via implementReviewInstruction, changing runtime todo text for every future implement route (session_state.go#L503-L514) |
| risk.correctness | moderate | new clause text must match the shipped two-round/stop-e protocol in ticket-worker.md and worker-stop-protocol.md exactly, or the emitted todo and the worker's actual behavior diverge again |
| risk.fit | low | Decisions section already authorizes realigning the runtime todo to the shipped ticket-worker protocol; no open architecture question |
| risk.test | low | change is confined to updating existing pinned string-literal expectations in session_state_test.go, not new test scaffolding |
| risk.security_or_contract | moderate | alters the worker's review-completion contract (fix ownership, round budget, stop condition) that every routed ticket implementation relies on |

## Phases

### Phase 1: Realign the runtime review todo to the worker protocol

Rewrite the review-instruction clauses emitted by `route.resolve_implement`
so the installed todo matches the shipped `ticket-worker` review protocol.
The obsolete clauses are `implementReviewRelayClause`,
`implementReviewCriticalBranchClause`, and the shared
`implementReviewDispositionClause` in
`agents-plugin-tool/internal/mcp/session_state.go` (and their assembly in the
review-dispatch clause); replace the `implementer-relay` relay language, the
three-review Critical branch, and the `implementer-elevated` escalation with
the shipped worker's contract: the worker dispositions findings itself, at
most one re-review (round two, which re-verifies round one's findings), and
stops on an unresolved Critical finding after round two. This includes
reconciling `implementReviewDispositionClause`'s own relay / "Important is
never re-reviewed" wording (session_state.go#L481) to the two-round model
rather than reusing it verbatim. Keep the reviewer dispatch and the
partitioned review allocation intact. Update the pinned expectations in `session_state_test.go`
to the new clause text. Source the exact target protocol from
`agents-plugin/rsrc/ticket-worker/ticket-worker.md` and its worker-protocol
include so the todo and the playbook agree. Verify by exercising
implementation routing for each review allocation and comparing the emitted
todo instructions against the shipped worker's review ownership, round budget,
and stop condition.

### Result (e62e8fc) - 2026-09-11

Realigned the review-instruction clauses `route.resolve_implement` installs to
the shipped `ticket-worker` two-round protocol.

- `agents-plugin-tool/internal/mcp/session_state.go`: replaced the three retired
  clauses (`implementReviewDispositionClause`, `implementReviewRelayClause`,
  `implementReviewCriticalBranchClause`) with two shared constants —
  `implementReviewFixClause` (worker owns its own fixes; no relay delegate;
  Minor recorded in the summary) and `implementReviewRoundsClause` (review is at
  most two rounds; round two verifies round one's fixes and raises nothing new;
  an unresolved Critical after round two is a stop reported to the lead, never an
  `implementer-elevated` escalation). `implementReviewInstruction` assembles the
  two into all three allocation shapes (single, `partitioned:`, bare partitioned)
  with reviewer dispatch and partitioned allocation intact.
- Removed the disposition-marker vocabulary entirely (the shipped protocol has
  none; the worker records outcomes via its Report block), and cleaned the two
  adjacent instructions that a round-1 review found still stranded the retired
  wording: `implementEditInstruction` ("...for review and relays" ->
  "...for review.") and `implementFinalActionInstruction` ("Verify review
  disposition" -> "Verify the review is resolved").
- `session_state_test.go`: retitled the shared pinning helpers to
  `implementReviewRoundWants` / `implementReviewRoundForbidden` with the
  two-round wants and a forbidden list that blocks `implementer-relay`,
  `implementer-elevated`, the bounded multi-round Critical branch, the
  disposition-marker vocabulary, and the older review-adjudicator terms;
  rewrote the Critical-branch test into `...CriticalStop` pinning the
  round-two stop; updated the final-gate positive pin to the new wording.

Decisions (recorded, not escalated): dropped the disposition-marker vocabulary
rather than reconciling it, matching the shipped protocol which carries no such
markers; renamed the two clause constants and the shared test helpers since the
old names encoded the retired relay/critical-branch model (cosmetic, no external
contract). Reviewer dispatch and partitioned allocation are unchanged, per the
ticket's retained-review decision.

Verification: `go build ./...` (agents-plugin-tool) OK;
`go test ./internal/mcp/` OK; `go test ./...` OK;
`gofmt -l` clean; `go vet ./internal/mcp/` clean. Independent review: two
rounds, single allocation. Round 1 raised two Important findings (stranded
`relays` / `disposition` wording), both fixed; round 2 verified the fixes clean
with no new blocking finding.
