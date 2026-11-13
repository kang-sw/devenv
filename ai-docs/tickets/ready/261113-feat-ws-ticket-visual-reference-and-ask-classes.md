---
title: "Tickets link approved visual references; board checks narrow to verifying finished work"
related:
  261113-feat-ws-whiteboard-topic-panels-and-log: "incident source (mockup dropped from the ticket)"
  261112-feat-ws-whiteboard-check-items: "origin of the check item class"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 206a381a30c4f335
sage-review-completeness-reviewed: 206a381a30c4f335
---

# Tickets link approved visual references; board checks narrow to verifying finished work

## Background

On 2026-11-13 the whiteboard topic panels + log feature (`.done/261113-feat-ws-whiteboard-topic-panels-and-log`) shipped with a UI unlike the owner-approved mockup G. The audit found the lead never linked the mockup (it lived in a temp directory and was later deleted) in the ticket or the worker brief. The ticket described the layout only abstractly, and verification had only behavioural checks, so nothing compared the result to the approved look.

Separately, the lead routed several policy decisions to the board as `check` items ("start this ticket now?", "OK to proceed with A/B?") because the `check` definition includes "approve something".

## Context

- Lead-ticket playbook source: `agents-plugin/rsrc/lead-ticket/lead-ticket.md`; verification expectations are captured per `:31-34` (the Phase 1 visual-reference bullet is `:35`, `agents-plugin/rsrc/lead-ticket/lead-ticket.md#L35-L35`), the promotion procedure is `## Promote to ready/` at `:267-348` (gate resolution via `tickets.sage_gate` at `:279-285`; the Phase 1 refusal is only the "do not promote to ready/" clause inside the `:35` bullet, with no step in the promotion procedure yet). Mirrors with the same relative path: `agents-plugin-wsflow/rsrc/lead-ticket/lead-ticket.md` and `agents-plugin-pi/rsrc/lead-ticket/lead-ticket.md`.
- Capture and intent checklists: `agents-plugin-tool/internal/wsdoc/tickets_checklist.go` - content checklist `:15-16` (item 1 already lists "verification expectation"), intent checklist `:25-27` (exactly three items, asserted by `agents-plugin-tool/internal/mcp/tickets_checklist_test.go:50-66`, so a new item changes that count assertion); research variants `:29-30`.
- Sage gate code (relevant if the ready refusal is code, not text): `agents-plugin-tool/internal/mcp/server.go:2044` (`tickets.sage_gate` dispatch), `:3709` (`formatSageGate`), `:3764` (`sageGateNextInstruction`).
- Worker playbook that runs a ticket: `agents-plugin/rsrc/ticket-worker/ticket-worker.md`, step "Edit and verify" at `:73-75`; no LOOK-capture comparison is described there. Mirrors: `agents-plugin-wsflow/rsrc/ticket-worker/ticket-worker.md`, `agents-plugin-pi/rsrc/ticket-worker/ticket-worker.md`; the elevated variant `ticket-worker-elevated/ticket-worker-elevated.md` exists in each package.
- Workflow manual `### Asking the user`: `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:48`; the "Check" bullet is `:69-75` ("look at, verify, or approve something"). Mirrors: `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md`, `agents-plugin-pi/rsrc/lead-workflow-manual/lead-workflow-manual.md`.
- Whiteboard playbook `class` bullet describing `check`: `agents-plugin/rsrc/whiteboard/whiteboard.md:85-91` (mirrors in wsflow and pi at the same relative path). Other referrers of the definition: `agents-plugin/rsrc/lead-ticket/lead-ticket.md:64,112` and `agents-plugin/rsrc/lead-discuss/lead-discuss.md:38` (check that they need no edit).

## Decisions

- Decision 1 (owner, 2026-11-13; was 413=b): the root cause is the lead dropping the mockup path from the ticket, not the worker, so the rule binds the lead first. In lead-ticket: when the owner approved a visual reference (mockup, screenshot, design file) for the work, the lead stores it at a durable location, links it by path in the ticket, and writes a verification expectation requiring a LOOK capture compared side by side against it. Promotion to `ready/` is refused while an approved visual reference exists but is not linked, or while that comparison is not in the verification. Secondary, in the worker playbook: when a ticket links a visual reference, the worker captures the result and compares it against the reference before closing the phase, and reports both paths.
- Decision 2 (owner, 2026-11-13; was 414=b): `check` narrows to looking at, verifying, or confirming a finished artifact, or an action whose choice is already settled. Any "should we do X?" or "which one?" ask is a policy question with options, including a yes/no one. Closing a policy item as `assumed` remains legitimate; the `assumed` rule is unchanged. Edits land in the workflow manual `### Asking the user` and the whiteboard playbook `class` bullet.

- Decision 3 (owner, 2026-11-13; was 416): an approved visual reference lives in a tracked folder beside the ticket board (path rule like `ai-docs/tickets/assets/<stem>/`, to be stated in the shipped convention). There is no hard format constraint; text assets (HTML, SVG, Markdown) are preferred over raster images. Rejected: an untracked durable directory, because it does not survive cleanup or travel with the ticket.
- Decision 4 (owner, 2026-11-13; from board item 391): check items are granular, one verifiable thing per item, so the owner answers each separately. Trigger: one check item bundled five things (search palette, memo pad, tracker draft restore, token usage, mailbox label copy). The guidance lands in the Phase 2 workflow-manual / whiteboard-playbook edit.
- Decision 5 (technical, EXEC 2026-11-13, from fact population): the Phase 1 checklist item and promote-step refusal that did not land become Phase 3, because Phase 1's plan and Result are frozen. The item goes into the `content` checklist; the `intent` checklist stays at its deliberate three items (`tickets_checklist.go` doc comment). Rejected: an Edition under Phase 1 (an Edition records a later pass over completed work, not unlanded scope); adding a fourth intent item.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/rsrc/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- File overlap: Phase 2 edits the `class` bullet of the whiteboard playbook (`agents-plugin/rsrc/whiteboard/whiteboard.md` and mirrors), which `261112-feat-ws-whiteboard-topic-tickets-sticky-head` (in flight) and `261113-feat-ws-dashboard-browser-pane` also edit in other sections; expect a textual merge and a manifest-hash rebase, no behavior conflict.
- Compatibility: T0 (playbook, manual, and checklist content only). No version bump.
- Shipped text is downstream-first: no reference to this repository's own paths (no temp-directory or `ai-docs/` specifics) in shipped text; the durable-location wording is generic.
- Manuals read before editing: `ai-docs/manuals/skill-authoring.md` (apply its invariant checklist to every changed Invariants/Constraints line), `ai-docs/manuals/wsflow-mirroring.md` (apply the ws and wsflow mirrors; carry the pi copies if the mirroring manual requires them), `ai-docs/manuals/shipped-surface-boundary.md`.
- The ready refusal should be text and checklist first (lead-ticket promote step plus a checklist item). Code in `tickets.sage_gate` is added only if text cannot carry it; if so, say so in the Phase 3 result.

## Prior Decisions

- 057a81e3 (2026-11-13, commit): "Owner 2026-11-13: the visual-reference rule should say text assets such as HTML or SVG are preferred (decision 416: preference, no hard constraint)." — bearing: supports
- 14178812 (2026-11-13, commit): "Granular checks: one verifiable thing per check item so each is answered separately." — bearing: supports
- 261112-feat-ws-whiteboard-check-items (2026-11-12, Decisions): "The workflow manual's `### Asking the user` gains `check` as a fourth kind of ask, described as a gate rather than a decision class, scoped to whiteboard and discussion asks (user, (572) a option text)." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, Decisions): "All decisions are owner-settled on 2026-11-13 via the whiteboard." (the feature whose UI missed the approved mockup, the trigger for this ticket) — bearing: supports
- 084a4c43 (2026-11-13, commit): "T0: additive workflow and skill guidance; generated resource mirrors and manifests stay aligned." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | open work: agents-plugin/rsrc/ticket-worker, lead-workflow-manual, whiteboard playbooks with wsflow and pi mirrors; agents-plugin-tool/internal/wsdoc/tickets_checklist.go and internal/mcp/tickets_checklist_test.go |
| scope.surface | cross-module | playbook text across three packages plus the wsdoc checklist constant and its mcp test |
| scope.new_public_symbol | no | none; TicketChecklist keeps its signature, only constant text changes |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/tickets_checklist_test.go (intent count assertion), rsrc mirror and manifest drift tests |
| complexity.reuse_points | confirmed | Phase 1 bullet at agents-plugin/rsrc/lead-ticket/lead-ticket.md#L35-L35 and its mirrors; existing checklist constants read |
| complexity.side_effect_risk | low | text and checklist constants only, but the checklist count assertion and manifest hashes must stay in sync |
| risk.correctness | low | text edits plus one checklist item with a literal-text test |
| risk.fit | moderate | the ready refusal must land in the Promote step and checklist rather than only a clause in a capture bullet; Decision 2 and 4 wording must fit the existing check text |
| risk.test | moderate | no test covers playbook wording beyond mirror drift; the checklist item needs its own fragment assertion |
| risk.security_or_contract | low | T0 content only, no tool or store shape change |

## Phases

### Phase 1: Lead-side visual reference rule

Add the rule to lead-ticket: store an approved visual reference durably, link it by path in the ticket, and write a side-by-side LOOK-capture verification expectation. Add the matching item to the `tickets.checklist` output for actionable tickets (content and/or intent checklist; update the count assertion in `tickets_checklist_test.go`), and make the `ready/` promotion step refuse while an approved reference is unlinked or its comparison is absent from the verification. Apply the ws, wsflow, and pi mirrors.

Verification: the playbook/manual drift and mirror tests pass. A rendered lead-ticket playbook shows the new text. The checklist output for an actionable ticket includes the visual-reference item.

### Result (9d5cdfc) - 2026-11-13

Based on `9d5cdfc`. Added the lead-side visual-reference bullet to `agents-plugin/rsrc/lead-ticket/lead-ticket.md` and its wsflow and pi mirrors, with manifests synchronized.
Go `./internal/wsrsrc/... ./internal/mcp/...` and wsflow package tests passed (one wsflow test skipped). The separate checklist change remains open.

### Phase 2: Worker-side comparison and check narrowing

Worker playbook: when a ticket links a visual reference, capture the result and compare it against the reference before closing the phase, and report both paths. Narrow the `check` definition in the workflow manual `### Asking the user` and the whiteboard playbook `class` bullet per Decision 2, leaving the `assumed` rule unchanged. Also add the granularity guidance per Decision 4: check items are written granular, one verifiable thing per item, never bundled. Apply the mirrors.

Verification: the playbook/manual drift and mirror tests pass. The rendered workflow manual and whiteboard playbook show the narrowed `check` text, and the rendered worker playbook shows the comparison step.

### Phase 3: Phase 1 remainder - checklist item and promote-step refusal

Phase 1's rule text landed in `ad90052e9` and `057a81e39` (merged in
`c5c5fd682`); its Result's "Based on `9d5cdfc`" names an unrelated commit.
Phase 1's checklist item and an explicit promote-step refusal did not land.
Independent of Phase 2; either order works.

- Add a visual-reference item to the actionable `content` checklist
  (`ticketChecklistContent` in
  `agents-plugin-tool/internal/wsdoc/tickets_checklist.go`): when the user
  approved a visual reference, it is stored in the tracked ticket assets
  folder, linked by path, and the verification requires a side-by-side
  comparison of a result capture against it. Leave `ticketChecklistIntent`
  at its deliberate three items. Add a fragment assertion for the new item
  in `agents-plugin-tool/internal/mcp/tickets_checklist_test.go` (the content
  checklist has no count assertion; the shared content text reaches every
  non-research category, which its "when the user approved" condition makes
  harmless).
- State the folder rule in the bundled ticket conventions
  (`agents-plugin-tool/internal/wsdoc/conventions/ticket-conventions.md`,
  Decision 3) in the same generic form lead-ticket already uses:
  `<ticket root>/assets/<stem>/`, text forms preferred. No repository-specific
  path in shipped text.
- Add a step to lead-ticket's `## Promote to ready/` that refuses the move
  while an approved visual reference is unlinked or its comparison is absent
  from the verification, so the refusal is not only a clause inside the
  capture bullet. Text only; no `tickets.sage_gate` code (Constraints: text
  and checklist first). Apply the wsflow and pi mirrors and manifest hashes.

Verification: `go test ./internal/wsdoc/... ./internal/mcp/...` and the
mirror/manifest drift tests pass; `tickets.checklist(type: feat, phase:
content)` output includes the new item; the rendered lead-ticket playbook
shows the promote-step refusal.
