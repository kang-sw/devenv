---
title: Shared question classes and conversation-unique question IDs in the workflow manual
related:
  261007-feat-odq-technical-decision-class: origin of the three item classes this ticket lifts
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 84cdcad4ad73f182
sage-review-completeness-reviewed: 84cdcad4ad73f182
---

# Shared question classes and conversation-unique question IDs in the workflow manual

## Background

How the lead asks the user is defined only inside the `lead-ticket`
playbook's Open Decision Queue (`### Item classes`, `### Queue state`,
`### Response format`): announced defaults, technical decisions, and policy
questions, each with an ID. `lead-discuss` instead says "Surface the single
highest-leverage ambiguity and stop until the user answers" (from
`3cec0ec03`, which named inference-filling as the failure mode). The user
finds the shift awkward: questions in discussion are loose prose, then
ticket authoring switches to a separate structured register, and the
conversation feels detached at that point. The user also wants every
question to carry an ID they can cite when answering earlier items.

The workflow manual is in every lead session's context, and the binding
anchor `260909-research-ws-refoundation-evidence-audit` counts it among the
mandated-read costs and judges procedural prose a poor producer of behavior
(A8), so only definitions move up; the queue mechanics stay in
`lead-ticket`.

## Decisions

- **Question classes live in the workflow manual.** A new `### Asking the
  user` subsection after `### User preferences` defines the three classes and the rules that hold wherever the lead asks the user:
  - announced default: one answer, with a citable reason its alternatives
    lose (a language or platform constraint; a prior commit or ticket
    decision; an established project convention; a direct consequence of an
    already-confirmed decision);
  - technical decision: no such citation, and the answer does not hinge on
    the user's values; the lead picks it and states the reasoning;
  - policy question: the answer depends on the user's preferences,
    priorities, or product direction;
  - an item that reverses a prior decision is always a policy question;
  - the lead's own uncertainty about a technical call does not make it a
    policy question;
  - the classes never lift an ask a playbook or the project reserves for the
    user (the `lead-review` next-step handoff, the `lead-ship` publish
    confirmation, a project's Approval Protocol "Ask first" items).
  Rejected: a short new definition in the manual with `lead-ticket` keeping
  its full copy (two definitions drift, and the drift brings back the
  register gap this ticket removes); placing them inside `### User
  preferences` (that is the user's own preference list, and a separate
  heading lets the playbooks point at it by name).
- **`lead-ticket` points at the manual.** Its `### Item classes` keeps only
  the ticket-specific rules: documentation-placement choices (parent or
  `related:` link, epic child or standalone, absorb or rewrite, initial
  status, stem naming, which commit carries the edit) as announced-default
  citations; a reviewer `missing` issue is always a policy question; a
  consequence of a policy question still open in the same round is held to
  the next round. Its queue state, acknowledgement rules, trace, and
  response format keep their behavior.
- **`lead-discuss` asks in the same classes, conversationally.** The lead
  says which items will be defaults, which are its technical calls, and
  which are policy questions the user must decide before the next topic, in
  prose: no block format, no queue, nothing persisted. The single-ambiguity
  rule becomes: ask the fewest policy questions, ask independent ones
  together, and hold a question that depends on an unanswered one to a
  later turn, as one line with no procedure or judge. The anti-inference
  intent of `3cec0ec03` stays: unanswered intent is never filled by
  inference. This does not reverse `61e45964`, which replaced the Interview
  Workflow (a procedure for question order: decision tree, one question per
  turn, judge-driven entry) as too rigid; that decision is about the order
  and procedure of asking, this one about which decisions the lead takes and
  which it hands to the user (user, 2026-10-23). `## Stops` changes from an
  unanswered surfaced ambiguity to an unanswered policy question. Rejected: laying out every open item
  in groups as the ticket queue does (in discussion an earlier answer often
  makes later questions vanish).
- **One conversation-unique ID space.** Every item the lead puts to the user,
  in discussion or in a ticket queue, carries an ID unique within the
  conversation, written `(nn)` zero-padded to two digits (`(03)`; three
  digits past 99). An item keeps its ID when it moves from discussion into a
  ticket queue. The ticket queue's last-ID line stays, so numbering
  continues after compaction. Rejected: `#nn` (GitHub autolinks it to an
  issue in commit messages); a per-ticket counter mapped to a separate
  conversation number (the queue section is deleted at the settle point, so
  a ticket-local number has no lasting use, and the mapping is one more
  place to confirm the wrong item).
- **Collisions on a resumed ticket.** A ticket queue left open from an
  earlier conversation keeps its IDs; only when one collides with an ID
  already used in the current conversation are that ticket's open items
  renumbered to fresh IDs, and the lead states the old-to-new mapping once
  in one line (`(04) -> (15)`).

## Constraints

- `lead-ticket` behavior is unchanged apart from the ID notation and the
  collision rule: only the `### Queue state` ID bullets and the
  `### Response format` skeleton and the bullets below it that carry the
  old `(n)` form change there, besides `### Item classes`.
- Every copy stays identical in substance: `agents-plugin/rsrc/`,
  `agents-plugin-wsflow/rsrc/`, and `agents-plugin-pi/rsrc/` for
  `lead-workflow-manual`, `lead-discuss`, and `lead-ticket`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin/hooks/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 261007-feat-odq-technical-decision-class (2026-10-07, Decisions): "Three item classes. This supersedes the two-class decision of 260925-feat-odq-announced-defaults-and-policy-questions, at the user's direction (2026-10-07); its other decisions stand." — bearing: supports
- 260925-feat-odq-announced-defaults-and-policy-questions (2026-09-25, Decisions): "The queue mechanism itself stays. Items still get stable IDs, live in the ticket's temporary ## Open Decision Queue section, and block persistence until settled." — bearing: constrains
- e05c7b23 (2026-10-07, commit): "The retired catch-all phrase "every item without such a citation" is added to the golden forbidden list so the old policy definition cannot return." — bearing: constrains
- d5682a35 (2026-09-26, commit): "Mirror process: WSRSRC_REGEN manifest regen, then WS_REGEN_WSFLOW_RSRC regen. The pi rsrc has no regen entrypoint ... copied and checked with diff -r. TestPiMirrorUpToDate and TestWsflowRsrcMirrorUpToDate pass." — bearing: constrains
- 95301971 (2026-09-24, commit): "The heading line must stay exactly ## Open Decision Queue and the last ID goes on the line under it, because the tooling gate matches the exact heading line" — bearing: constrains
- dd4b01b5 (2026-09-26, commit): "lead-discuss needs no change: its capture handoff says persistence writes only confirmed decisions, which still holds because announced defaults are confirmed by explicit acknowledgement." — bearing: supports
- 61e45964 (2026-07-01, commit, 260630-epic-skill-playbook-diet): "Interview Workflow removed as does-not-work-in-practice; replaced by single-question discipline rule." — bearing: contradiction-candidate
- 3cec0ec0 (2026-07-01, commit): "Rule now names the failure mode (inference-filling) and enforces hard stop, consistent with the "doubt the user" posture in workflow_manual." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin, agents-plugin-wsflow, agents-plugin-pi rsrc/lead-workflow-manual/lead-workflow-manual.md, rsrc/lead-discuss/lead-discuss.md, rsrc/lead-ticket/lead-ticket.md, plus each package rsrc/manifest.json hash and agents-plugin-tool/internal/mcp/playbook_tools_test.go pins |
| scope.surface | cross-module | shipped playbook text in three packages; no exported code symbol changes |
| scope.new_public_symbol | no | none; a new manual heading Asking the user only |
| scope.new_type_contract | no | ID notation changes from (1) to (01) in prose; the ODQ tooling gate matches only the heading line per agents-plugin-tool/internal/wsdoc/tickets_open_decision_queue_test.go |
| scope.test_surface | existing | TestPlaybookPrintGoldenLeadTicket in agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2236-L2350 pins Item classes sentences and the (1)/(2)/(3) skeleton verbatim and must be updated; prompt_override_test.go#L602-L634 bounds the User preferences section at the next ### heading; TestPiMirrorUpToDate and TestWsflowRsrcMirrorUpToDate |
| complexity.reuse_points | confirmed | existing class definitions in agents-plugin/rsrc/lead-ticket/lead-ticket.md#L60-L106 are lifted, not rewritten |
| complexity.side_effect_risk | moderate | the workflow manual is rendered into every lead session, so the new subsection changes context for all lead skills |
| risk.correctness | moderate | dozens of verbatim golden pins and three hand-synced copies plus manifests must move together; lead-ticket line 132 says IDs never change, which the collision rule must amend |
| risk.fit | moderate | relaxes the single-question discuss rule from 61e45964 and 3cec0ec03 and adds mandated-read text the anchor counts as cost |
| risk.test | moderate | question register is model behavior with no deterministic test; only text pins guard it |
| risk.security_or_contract | low | no tool schema or parser reads item IDs; only the heading gate is mechanical |

## Phases

### Phase 1: Lift the classes and IDs, align discuss and ticket

Edit `lead-workflow-manual`, `lead-discuss`, and `lead-ticket` per
Decisions in every package copy: regenerate each `rsrc/manifest.json`
(WSRSRC_REGEN, then WS_REGEN_WSFLOW_RSRC) and hand-copy the pi tree, checked
by `TestPiMirrorUpToDate` and `TestWsflowRsrcMirrorUpToDate` (`d5682a35`).
Update `TestPlaybookPrintGoldenLeadTicket`
(`agents-plugin-tool/internal/mcp/playbook_tools_test.go`) to the new
`lead-ticket` text, and add golden pins for the manual's `### Asking the
user` definitions, the `(nn)` notation, the collision rule, and the
`lead-discuss` rule; keep `e05c7b23`'s forbidden-phrase list. Rejected:
dropping the moved pins (the regression guard would leave the text that now
carries the definitions).

Verification: the existing playbook render, mirror-drift, and package test
suites pass in every copy, and one fresh-read audit of the three changed
documents per `skill-authoring.md` reports no unresolved finding. A scripted
conversation test is not attempted: the question register is model behavior
with no deterministic test surface.

Implementation notes from review:
- The ID-space and collision rules live in the manual's `### Asking the
  user` subsection with the classes; `lead-ticket` `### Queue state` points at
  them and keeps only its last-ID line.
- A ticket queue's first ID continues from the highest ID already used in
  the conversation (`(01)` when none).
- In discussion-only work there is no last-ID line; after compaction the
  lead continues from the highest ID in the summary or restarts at the next
  free number, accepting a loose collision, since nothing persists.
- Items acknowledged in discussion are acknowledged again when they enter a
  ticket queue; `lead-ticket` acknowledgement behavior is unchanged.
- Parallel with `261023-bug-claude-mod-sentinel-key-mailbox-push`: on a
  manifest or mirror conflict at merge, regenerate, never hand-merge.
