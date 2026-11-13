---
title: "Tickets link approved visual references; board checks narrow to verifying finished work"
---

# Tickets link approved visual references; board checks narrow to verifying finished work

## Background

On 2026-11-13 the whiteboard topic panels + log feature (`.done/261113-feat-ws-whiteboard-topic-panels-and-log`) shipped with a UI unlike the owner-approved mockup G. The audit found the lead never linked the mockup (it lived in a temp directory and was later deleted) in the ticket or the worker brief. The ticket described the layout only abstractly, and verification had only behavioural checks, so nothing compared the result to the approved look.

Separately, the lead routed several policy decisions to the board as `check` items ("start this ticket now?", "OK to proceed with A/B?") because the `check` definition includes "approve something".

## Context

- Lead-ticket playbook source: `agents-plugin/rsrc/lead-ticket/lead-ticket.md`; verification expectations are captured per `:32-33`, the promotion procedure is `## Promote to ready/` at `:266-290` (gate resolution via `tickets.sage_gate` at `:276-284`). Mirrors with the same relative path: `agents-plugin-wsflow/rsrc/lead-ticket/lead-ticket.md` and `agents-plugin-pi/rsrc/lead-ticket/lead-ticket.md`.
- Capture and intent checklists: `agents-plugin-tool/internal/wsdoc/tickets_checklist.go` - content checklist `:15-16` (item 1 already lists "verification expectation"), intent checklist `:25-27` (exactly three items, asserted by `agents-plugin-tool/internal/mcp/tickets_checklist_test.go:50-66`, so a new item changes that count assertion); research variants `:29-30`.
- Sage gate code (relevant if the ready refusal is code, not text): `agents-plugin-tool/internal/mcp/server.go:2044` (`tickets.sage_gate` dispatch), `:3709` (`formatSageGate`), `:3764` (`sageGateNextInstruction`).
- Worker playbook that runs a ticket: `agents-plugin/rsrc/ticket-worker/ticket-worker.md`, step "Edit and verify" at `:73-75`; no LOOK-capture comparison is described there. Mirrors: `agents-plugin-wsflow/rsrc/ticket-worker/ticket-worker.md`, `agents-plugin-pi/rsrc/ticket-worker/ticket-worker.md`; the elevated variant `ticket-worker-elevated/ticket-worker-elevated.md` exists in each package.
- Workflow manual `### Asking the user`: `agents-plugin/rsrc/lead-workflow-manual/lead-workflow-manual.md:48`; the "Check" bullet is `:69-75` ("look at, verify, or approve something"). Mirrors: `agents-plugin-wsflow/rsrc/lead-workflow-manual/lead-workflow-manual.md`, `agents-plugin-pi/rsrc/lead-workflow-manual/lead-workflow-manual.md`.
- Whiteboard playbook `class` bullet describing `check`: `agents-plugin/rsrc/whiteboard/whiteboard.md:85-91` (mirrors in wsflow and pi at the same relative path). Other referrers of the definition: `agents-plugin/rsrc/lead-ticket/lead-ticket.md:64,112` and `agents-plugin/rsrc/lead-discuss/lead-discuss.md:38` (check that they need no edit).

## Decisions

- Decision 1 (owner, 2026-11-13; was 413=b): the root cause is the lead dropping the mockup path from the ticket, not the worker, so the rule binds the lead first. In lead-ticket: when the owner approved a visual reference (mockup, screenshot, design file) for the work, the lead stores it at a durable location, links it by path in the ticket, and writes a verification expectation requiring a LOOK capture compared side by side against it. Promotion to `ready/` is refused while an approved visual reference exists but is not linked, or while that comparison is not in the verification. Secondary, in the worker playbook: when a ticket links a visual reference, the worker captures the result and compares it against the reference before closing the phase, and reports both paths.
- Decision 2 (owner, 2026-11-13; was 414=b): `check` narrows to looking at, verifying, or confirming a finished artifact, or an action whose choice is already settled. Any "should we do X?" or "which one?" ask is a policy question with options, including a yes/no one. Closing a policy item as `assumed` remains legitimate; the `assumed` rule is unchanged. Edits land in the workflow manual `### Asking the user` and the whiteboard playbook `class` bullet.

## Constraints

- Compatibility: T0 (playbook, manual, and checklist content only). No version bump.
- Shipped text is downstream-first: no reference to this repository's own paths (no temp-directory or `ai-docs/` specifics) in shipped text; the durable-location wording is generic.
- Manuals read before editing: `ai-docs/manuals/skill-authoring.md` (apply its invariant checklist to every changed Invariants/Constraints line), `ai-docs/manuals/wsflow-mirroring.md` (apply the ws and wsflow mirrors; carry the pi copies if the mirroring manual requires them), `ai-docs/manuals/shipped-surface-boundary.md`.
- The ready refusal should be text and checklist first (lead-ticket promote step plus a checklist item). Code in `tickets.sage_gate` is added only if text cannot carry it; if so, say so in the Phase 1 result.

## Open Decision Queue

Last ID: (416)

- (416) [open] [technical] Durable home for an approved visual reference: a tracked folder beside the ticket board (e.g. `ai-docs/tickets/assets/<stem>/`), over an untracked durable dir - it survives temp cleanup, travels with the ticket into worktrees, and reviewers can open it; large binaries are the cost, so the rule caps it at the reference files actually approved.

## Phases

### Phase 1: Lead-side visual reference rule

Add the rule to lead-ticket: store an approved visual reference durably, link it by path in the ticket, and write a side-by-side LOOK-capture verification expectation. Add the matching item to the `tickets.checklist` output for actionable tickets (content and/or intent checklist; update the count assertion in `tickets_checklist_test.go`), and make the `ready/` promotion step refuse while an approved reference is unlinked or its comparison is absent from the verification. Apply the ws, wsflow, and pi mirrors.

Verification: the playbook/manual drift and mirror tests pass. A rendered lead-ticket playbook shows the new text. The checklist output for an actionable ticket includes the visual-reference item.

### Phase 2: Worker-side comparison and check narrowing

Worker playbook: when a ticket links a visual reference, capture the result and compare it against the reference before closing the phase, and report both paths. Narrow the `check` definition in the workflow manual `### Asking the user` and the whiteboard playbook `class` bullet per Decision 2, leaving the `assumed` rule unchanged. Apply the mirrors.

Verification: the playbook/manual drift and mirror tests pass. The rendered workflow manual and whiteboard playbook show the narrowed `check` text, and the rendered worker playbook shows the comparison step.
