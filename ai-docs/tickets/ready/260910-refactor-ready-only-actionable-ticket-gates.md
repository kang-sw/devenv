---
title: "Run actionable ticket facts and Sage review only at ready promotion"
sage-review-design: completed
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260910-chore-prune-dead-workflow-remnants: resolves the unused todo-landing Sage API decision
  260726-refactor-retire-workset-convention: removes the remaining nonimplementation category that no longer has a workflow role
sage-review-completeness: completed
sage-review-design-reviewed: 1ee2e902c5f7feb1
sage-review-completeness-reviewed: 1ee2e902c5f7feb1
---

# Run actionable ticket facts and Sage review only at ready promotion

## Background

The refoundation epic defines fact population and design review as the gate into
`ready/`, but the shipped `lead-ticket` procedure still runs fact population
while authoring or editing a checkable `todo/` ticket. The MCP Sage API also
accepts `landing: "todo"` for every actionable category even though shipped
prose calls it only during `ready/` promotion. Together these surfaces invite
repeated fact population and design-stamp refresh while a backlog ticket is
still being drafted.

The ticket categories already distinguish implementation targets from board
and investigation artifacts. Preserve that distinction instead of broadening
`ready/`: actionable tickets enter `ready/` only after their implementation
contract is grounded and reviewed; epics settle design at `idea/` to `todo/`;
research remains an ungated `idea/` or `todo/` artifact; workset retires under
the related ticket.

## Decisions

- **Actionable `todo/` authoring is ungated.** Creating or editing a `feat`,
  `bug`, `refactor`, or `chore` in `todo/` does not run fact population or Sage
  review. `todo/` remains editable accepted backlog.
- **Actionable promotion is the single expensive boundary.** Immediately
  before promotion to `ready/`, populate or refresh facts, then run the design
  and completeness stages against the same body. The ready stamp covers the
  facts the reviewers read.
- **Epic design settles at the accepted-backlog boundary.** An explicit epic
  promotion from `idea/` to `todo/` runs fact population followed by the
  design-only Sage stage. A later change to cross-child decisions requires an
  explicit re-settlement before a child relies on them; ordinary todo edits do
  not automatically spawn reviewers.
- **Restrict the todo Sage API to its surviving purpose.**
  `tickets.sage_gate(landing: "todo")` accepts epic design settlement and
  fails loudly for actionable tickets. Research remains exempt and workset is
  handled by its retirement ticket.
- **Ready remains the implementation queue.** Research and epic tickets remain
  in `idea/` or `todo/`; `lead-run` continues to select only actionable
  `ready/` tickets. No new execution-eligibility field or ready-category filter
  is introduced by this ticket.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Fact population and review must see the same ticket body at each settlement
  boundary; no review stamp may cover facts added afterward.
- Preserve direct historical ticket reads and existing ready-ticket execution.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin-wsflow/rsrc/lead-ticket/lead-ticket.md, agents-plugin-tool/internal/wsdoc/tickets_sage.go, agents-plugin-tool/internal/mcp/server.go, and existing tests |
| scope.surface | public-interface | tickets.sage_gate remains a runtime MCP tool whose landing=todo behavior changes (agents-plugin-tool/internal/mcp/server.go#L3485-L3495) |
| scope.new_public_symbol | no | no new tool or exported API is proposed; the existing tickets.sage_gate behavior changes |
| scope.new_type_contract | no | existing landing enum and SageGateOptions remain; the category rule changes (agents-plugin-tool/internal/wsdoc/tickets_sage.go#L127-L178) |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_sage_test.go and tickets_route_facts_test.go already cover todo and ready Sage-gate behavior |
| complexity.reuse_points | confirmed | SageGate and sageReviewStageRequirement already centralize landing and category behavior (agents-plugin-tool/internal/wsdoc/tickets_sage.go#L127-L245; agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L373-L393) |
| complexity.side_effect_risk | moderate | promotion, fact-population, and review order must stay synchronized across the two shipped playbooks and MCP gate |
| risk.correctness | high | category-specific todo and ready outcomes must preserve actionable, epic, research, and historical-ticket behavior |
| risk.fit | moderate | the same boundary must be expressed in lead-ticket, wsflow mirrors, ticket documentation, MCP schema/help, and tests |
| risk.test | high | the phase requires distinct no-spawn, once-only review, rejection, and preservation scenarios |
| risk.security_or_contract | high | direct tickets.sage_gate(landing: "todo") changes from accepting actionable tickets to rejecting them |

## Phases

### Phase 1: Make ticket grounding and review boundary-driven

Revise `lead-ticket` so ordinary actionable `todo/` writes perform neither
fact population nor Sage review. During actionable `ready/` promotion, run fact
population before the move/review sequence and run the applicable design and
completeness reviewers against the populated body.

Keep the todo-landing Sage path for epic design settlement only. Reject its use
for actionable tickets, and remove any actionable todo-move posture behavior
that implies a design review already belongs to that boundary. Add the epic
`idea/` to `todo/` settlement path to `lead-ticket`: populate checkable facts,
run design-only review, and stamp the reviewed body. A material edit to a
settled epic does not auto-spawn review; it leaves the prior digest stale until
the lead explicitly settles the revised design.

Update the ticket-system concepts, conventions, bootstrap migration text, MCP
schema/help text, and wsflow mirrors so they state the same category-aware
boundaries. Verify that actionable todo creation and repeated editing spawn no
delegate or reviewer; actionable ready promotion runs facts then design and
completeness exactly once; epic settlement runs facts then design only;
research remains ungated; direct actionable `landing: "todo"` fails loudly;
and existing ready execution and historical ticket discovery remain intact.
