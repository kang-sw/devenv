---
title: "Research Outcome Ledger and research-to-actionable derivation contract"
related:
  260911-research-ticket-decision-state-convention-gap: source; postmortem and user-confirmed contract
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 6d9d90ff01fbdf78
sage-review-completeness-reviewed: 6d9d90ff01fbdf78
---

# Research Outcome Ledger and derivation contract

## Background

Research tickets use freeform topic sections, but that structural freedom does
not distinguish verified evidence, confirmed normative decisions, unconfirmed
proposals, open questions, and rejected alternatives. The category-invariant
ticket checklist also excludes all unconfirmed content, while research needs to
preserve explicitly non-authoritative investigation output. `lead-ticket` has no
research-to-actionable derivation step, so a child author can silently promote
freeform narrative into implementation contract.

## Decisions

- The standard research template includes this exact section:

```markdown
## Outcome Ledger

### Verified Findings
<!-- Evidence-backed observations. These may support later tickets but do not choose behavior. -->

### Confirmed Decisions
<!-- Normative choices explicitly confirmed by the user. -->

### Proposals
<!-- Unconfirmed candidates. Never treat these as actionable authority. -->

### Open Questions
<!-- Unresolved choices that require further investigation or user input. -->

### Rejected Alternatives
<!-- Alternatives explicitly rejected, with the reason when useful. -->
```

- Install this sentence verbatim in `lead-ticket`'s general write guidance:

> Persist only decisions the user confirmed. Research tickets may preserve explicitly non-authoritative proposals and open questions in their Outcome Ledger.

- Install these sentences verbatim in `lead-ticket`'s research-to-actionable
  derivation guidance:

> When deriving actionable work from research, treat the Outcome Ledger as the sole authority: use `Verified Findings` as evidence and `Confirmed Decisions` as contract; read the narrative only as supporting context, and never promote `Proposals`, `Open Questions`, or unlisted narrative into the child.

> If the research has no Outcome Ledger, stop and ask whether to add one or settle the child’s decisions directly through the Open Decision Queue.

- Keep research topic sections freeform and keep research ungated. The Outcome
  Ledger is a standard section in the research ticket, not a separate artifact.
- Give the ticket content and intent checklists a research-specific branch that
  permits explicitly non-authoritative `Proposals` and `Open Questions` in the
  Outcome Ledger while preserving the existing confirmed-only discipline for
  actionable tickets.
- Do not change fact population, Sage completeness review, or Sage design review
  in this ticket.

## Constraints

- Preserve the approved sentences and the Outcome Ledger template verbatim.
- The shipped convention, template, checklist, and `lead-ticket` surfaces must
  stay aligned across the full ws package and the wsflow derivative.
- Read `ai-docs/manuals/skill-authoring.md`,
  `ai-docs/manuals/shipped-surface-boundary.md`, and
  `ai-docs/manuals/wsflow-mirroring.md` before editing shipped text.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md, agents-plugin-tool/internal/wsdoc/tickets_template.go, agents-plugin-tool/internal/wsdoc/tickets_checklist.go, agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin-wsflow/rsrc/ |
| scope.surface | public-interface | tickets.template and tickets.checklist are MCP tool outputs; lead-ticket is shipped playbook text (agents-plugin-tool/internal/mcp/server.go#L1254-L1259, agents-plugin/rsrc/lead-ticket/lead-ticket.md#L8-L40) |
| scope.new_public_symbol | no | existing TicketTemplate and TicketChecklist outputs change; no new exported symbol is named (agents-plugin-tool/internal/wsdoc/tickets_template.go#L103-L116, agents-plugin-tool/internal/wsdoc/tickets_checklist.go#L29-L45) |
| scope.new_type_contract | no | no new Go type or signature is named; the change is to existing template, checklist, and playbook text |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tickets_template_test.go#L10-L80, agents-plugin-tool/internal/mcp/tickets_checklist_test.go#L10-L110, agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L80 |
| complexity.reuse_points | confirmed | existing byte-identical wsflow rsrc mirror guard covers canonical rsrc changes (agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L47-L80) |
| complexity.side_effect_risk | moderate | changes the template and checklist behavior used for every research ticket |
| risk.correctness | moderate | research-only exemptions must preserve confirmed-only behavior for actionable tickets |
| risk.fit | moderate | existing checklists deliberately return category-invariant text (agents-plugin-tool/internal/wsdoc/tickets_checklist.go#L13-L27) |
| risk.test | moderate | approved sentences and five headings require exact-text and mirror coverage |
| risk.security_or_contract | high | changes the shipped authority boundary for deriving actionable work from research |

## Phases

### Phase 1: Install the research handoff contract

Update the shipped ticket conventions and research template with the standard
Outcome Ledger and its exact HTML comments. Add the approved general-write and
research-derivation sentences to `lead-ticket`. Make the ticket checklists
category-aware so research can retain explicitly labeled non-authoritative
material without weakening actionable ticket discipline. Mirror all affected
playbook resources into wsflow and regenerate or update the existing exact-text
fixtures.

Verify the research template renders the five ledger headings and comments
verbatim; research content and intent checklists allow labeled proposals and open
questions; actionable checklist output remains unchanged; `lead-ticket` and its
wsflow mirror contain the approved sentences verbatim; convention/resource drift
and the full affected Go and package test suites pass.
