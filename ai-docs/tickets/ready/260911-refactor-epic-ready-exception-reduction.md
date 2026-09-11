---
title: "Reduce the epic-specific workflow exception surface: treat epics as soft living boards, hard-bar non-implementation categories from ready/, and make epic design review lead-judgment-invoked"
related:
  260910-refactor-ready-only-actionable-ticket-gates: supersedes part of; that ticket moved epic design settlement to the idea->todo boundary and barred epics from ready/ in prose only — this ticket replaces that boundary-gated model with a lighter judgment-invoked one and hard-enforces the ready/ bar in code
  260910-chore-design-review-ready-inventory-contradiction-anchor: context; the ready-inventory contradiction cross-check stays the design reviewer's job
  260911-research-epic-close-on-last-child-prompt: related; the robust epic-close procedure is researched there and hardens the minimal terminal nudge this ticket lands
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1badd25f9f957efd
sage-review-completeness-reviewed: 1badd25f9f957efd
---

# Reduce the epic-specific workflow exception surface

## Background

The refoundation left a large epic-specific exception surface in the ticket
system: a category x stage matrix special-case (`epic` -> design-only), a
dedicated `sage_gate(landing: "todo")` epic-design-settlement path fired at the
`idea/` -> `todo/` promotion, "epics settle design at idea->todo / epics stay in
idea/todo" prose across four shipped surfaces, and an epic-at-`ready/`
design-only branch in `SageGate`. The prose bars epics from `ready/` but no code
enforces it: `TicketsMove(to: "ready")` has no category bar, so a stray
`tickets.move(epic, to: "ready")` succeeds against the rule.

The intended model is lighter. An epic is a soft living board document that
accretes follow-up and research tickets over time; it is never itself an
execution target. Its design review should not be pinned to a status-transition
boundary by rule — the lead runs it on judgment, when an epic's design has
drifted materially. Completeness never applies to an epic (it is not an
execution target). Research tickets likewise stay ungated and out of `ready/`.
Collapsing the epic ceremony this way is the point: fewer boundary rules, more
lead common-sense.

## Decisions

- **Epics and research never enter `ready/`, enforced in code (D1).** Add a hard
  bar at the ready landing that rejects any `nonImplementationCategories` member
  (`epic`, `research`, `workset`) — the set already exists in
  `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`. This is the single
  chokepoint that makes the prose rule real. Because `lead-run` selects only
  from `ready/` (`lead-run.md`), barring epics from `ready/` is exactly what
  keeps `lead-run` from ever targeting an epic — no separate `lead-run`
  selection change is required. Rejected: prose-only enforcement plus a
  `lead-run` self-filter — a mistaken promotion would still hand a worker an
  epic, and the codebase already prefers one hard chokepoint over scattered
  soft guards.
- **Epic design review is lead-judgment-invoked, not boundary-gated (D2).** The
  Go entry point stays: `sage_gate` is already decoupled from `tickets.move`,
  and the existing `landing: "todo"` path runs epic design-only review over the
  ticket's current frontmatter regardless of where it sits. Keep that path;
  remove only the prose that ties it to the `idea/` -> `todo/` promotion, and
  re-narrate it as "run when the epic's design has drifted materially — your
  judgment." No new Go affordance. Rejected: renaming `landing: "todo"` to
  `landing: "epic"` — a policy-surface churn that adds an exception name rather
  than removing one.
- **The matrix stays; epics remain design-capable, completeness-exempt.**
  `sageReviewStageRequirement` keeps `epic` -> `(design, not completeness)`; only
  the trigger prose changes. The epic-at-`ready/` design-only handling in
  `SageGate` is retained, not removed: `sage_gate` is decoupled from
  `tickets.move`, so a direct `sage_gate(epic, landing: "ready")` must still stay
  design-only and never fall into completeness review (see Phase 1). No new
  scoring, no new category logic.
- **A minimal safety net lands now so epics do not float forever.** Because
  nothing auto-closes an epic, add one prose line at the `lead-run` terminal:
  when a drained cycle completed an epic's last open child, surface that epic to
  the user for a close decision. The robust, reliable-trigger version is
  deferred to `260911-research-epic-close-on-last-child-prompt`; this line is
  the interim guard, not the final mechanism.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- No shipped playbook may enforce a rule that lives only in this repository; the epic bar rides the generic `nonImplementationCategories` set and Route Facts, not any devenv-only fact.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/tickets_mutate.go, agents-plugin-tool/internal/wsdoc/tickets_sage.go, agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go, agents-plugin-tool/internal/wsdoc/tickets_sage_test.go, agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md, agents-plugin/rsrc/lead-run/lead-run.md, agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md, agents-plugin-tool/internal/mcp/server.go, plus the agents-plugin-wsflow/ mirrors |
| scope.surface | public-interface | tickets.move and tickets.sage_gate are runtime MCP tools whose behavior/description changes (agents-plugin-tool/internal/mcp/server.go#L3439-L3495) |
| scope.new_public_symbol | no | no new tool or exported API; existing tickets.move and tickets.sage_gate behavior changes |
| scope.new_type_contract | no | existing TicketMoveOptions/SageGateOptions and the idea/todo/ready landing enum are unchanged; the change is the ready-landing rejection plus retaining the epic-at-ready design-only guard |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdoc/tickets_mutate_test.go#L657 (TestTicketsMoveUpwardToReadyEpicOnlyChecksDesign) and tickets_sage_test.go#L257-L277 already cover epic-at-ready behavior and are the tests the phase updates |
| complexity.reuse_points | confirmed | nonImplementationCategories (tickets_mutate.go#L296-L304) and sageReviewStageRequirement (tickets_mutate.go#L375-L388) already centralize the category set both TicketsMove and SageGate read |
| complexity.side_effect_risk | moderate | one behavior flip (ready-landing rejection) plus preserving the epic-at-ready design-only guard against the direct sage_gate path plus a four-surface prose sync and wsflow-mirror regeneration, each independently localized |
| risk.correctness | moderate | must preserve existing actionable-ready and todo-epic-design flows while flipping the ready outcome only for epic/research/workset |
| risk.fit | moderate | the same soft-to-hard boundary and prose reframe must land consistently across lead-ticket.md, ticket-conventions.md, lead-workflow-manual.md, the sage_gate tool description, lead-run.md, and the wsflow mirror |
| risk.test | moderate | requires updating the existing epic-at-ready assertions in tickets_sage_test.go and tickets_mutate_test.go and adding a rejection-path case, within the existing suite |
| risk.security_or_contract | high | tickets.move(to: "ready") flips from succeeding to erroring for epic/research/workset stems, a breaking change to a runtime MCP tool's contract (agents-plugin-tool/internal/mcp/server.go#L3439-L3450); the sibling ticket 260910-refactor-ready-only-actionable-ticket-gates rated an analogous sage_gate contract flip high |

## Phases

### Phase 1: Hard-bar non-implementation categories from ready/ and drop the vestigial epic-at-ready gate

In `agents-plugin-tool/internal/wsdoc/tickets_mutate.go`, add a hard rejection at
the `to == "ready"` landing for any stem whose category is in
`nonImplementationCategories` (`epic`, `research`, `workset`), replacing today's
soft-tip-only ready landing as the enforcement point; keep the existing
route-facts and sage-posture tips for the actionable categories that still
promote. In `agents-plugin-tool/internal/wsdoc/tickets_sage.go`, preserve the epic
completeness-exemption at the ready landing rather than deleting the
epic-at-`ready/` design-only branch outright. The `tickets.move` bar closes the
promotion path, but `sage_gate` is decoupled from `tickets.move` (D2 relies on
this), so a direct `sage_gate(epic, landing: "ready")` stays reachable; a
design-only category must never fall through into the "Both stages required"
path, which would run completeness on an epic and contradict D3. Keep the
`designRequired && !completenessRequired` block (or replace it with an explicit
design-only / skip guard) so an epic at the ready landing stays design-only.
Leave the `todo`-landing epic design path and the research/workset skips intact,
and keep `sageReviewStageRequirement` unchanged apart from a comment refresh.
Update `tickets_mutate_test.go` and `tickets_sage_test.go` to pin the new bar
and the retained epic-at-ready design-only behavior. Verify a
`tickets.move(epic|research|workset, to: "ready")` is rejected with a clear
error, an actionable move to `ready/` is unaffected, epic design review still
runs via the `todo` path, and a direct `sage_gate(epic, landing: "ready")` runs
design only and never completeness.

### Phase 2: Reframe the prose to the soft-epic model, add the terminal nudge, and mirror

Rewrite the epic sections of the four shipped surfaces to the soft-living-board
model: `agents-plugin/rsrc/lead-ticket/lead-ticket.md` (the "Settle epic design"
and "Promote to `ready/`" sections and the "Epics and research stay in
idea/todo" line), `agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`,
`agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md`, and the
`sage_gate` tool description in `agents-plugin-tool/internal/mcp/server.go`. The
new prose: an epic is a living board that is never an execution target and never
enters `ready/`; the lead runs epic design review (design only; completeness
never applies) on judgment when the design has drifted materially, not as a
promotion boundary; research stays ungated and out of `ready/`. Add one line to
the `lead-run` terminal (`agents-plugin/rsrc/lead-run/lead-run.md`): when a
drained cycle completed an epic's last open child, surface that epic to the user
for a close decision (interim guard; see the research ticket). Mirror every
`rsrc/` edit byte-identically to `agents-plugin-wsflow/` and regenerate the
affected manifests. Verify the ws/wsflow rsrc mirror is byte-identical and the
skill/mirror drift tests pass.
