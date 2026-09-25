---
title: Open Decision Queue splits announced defaults from policy questions
related:
  260726-feat-doc-organization-autonomy-odq-admission-filter: absorbed and dropped; its placement-autonomy concern is a subset of the announced-default class here
  260730-feat-odq-batch-interview: prerequisite, done; the batch interview this ticket reshapes
  260924-feat-open-decision-queue-response-format: done; the current Response format this ticket amends
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 18e4dc73d935af48
sage-review-completeness-reviewed: 18e4dc73d935af48
---

# Open Decision Queue splits announced defaults from policy questions

## Background

The `lead-ticket` Open Decision Queue admits every unconfirmed item that could
change ticket text and presents each one the same way: context, alternatives,
a recommendation, then an explicit answer. In practice most items have one
answer that follows from something citable, and the owner ends up answering
"as recommended" to all of them to reach the one or two that actually need a
judgment.

Owner observation (2026-09-25 session): two ticket settlements in a row ran
this way. For `260924-chore-review-sweep-test-and-naming-minors`, seven queued
items had six whose answer was forced by a Go language limit (`_test.go`
helpers cannot be shared across packages), a prior commit decision
(ca734c1d), or a plain consolidation; only one reversed a prior decision
(7c16252a) and needed the owner. The owner asked to leave only the policy
questions, then asked to make that the procedure. Both settlements also hit
the Final confirmation step, which re-asked for approval of items already
answered.

`260726-feat-doc-organization-autonomy-odq-admission-filter` raised the same
problem for documentation-placement decisions and deferred its boundary until
the batch interview landed (done, 766281e65). Its counter-evidence still
applies: in a downstream field report, seven items an agent had internalized
as settled were queued, and two were materially revised once actually asked.
Whatever the agent treats as determined must still pass in front of the owner.

## Decisions

- **The queue has two classes, presented in two groups.**
  - **Announced defaults** (upper group): one line each, "will do X", with
    the citation that makes the alternatives lose. No per-item answer is
    requested.
  - **Policy questions** (lower group, visibly marked as such): the current
    block format - context, alternatives, recommendation.
- **Admission to the announced-default class requires a citable reason**, one
  of: a language or platform constraint; a prior commit or ticket decision; an
  established project convention; a direct consequence of an
  already-confirmed decision; or a documentation-placement choice
  (parent/related, epic-child vs standalone, absorb vs rewrite, initial
  status, stem naming, which commit carries the edit), whose one line names
  the placement and cites the neighbouring ticket or convention it follows.
  Anything without such a citation is a policy question.
  - A consequence of a policy question still open in the same round is not
    yet "already-confirmed": it waits and joins the next round's announced
    group once that answer lands. A policy answer that undercuts an
    already-acknowledged announced item reopens that item as a policy
    question under its existing ID.
  - Fact-populator decision gaps and reviewer issues may be announced
    defaults when they meet the citation rule; a reviewer `missing` issue
    stays a policy question, since it is by definition a choice the reviewer
    could not derive.
- **An item that reverses a prior decision is always a policy question**, even
  when the lead can cite a reason for the reversal.
- **Announced defaults are confirmed by one explicit owner acknowledgement**
  covering the whole group, which may arrive in the same turn as the policy
  answers. The owner may object to any announced item; that item becomes a
  policy question under its existing ID. If the owner answers the policy
  questions but does not acknowledge the announced group, the announced
  items stay pending and the lead re-asks for the acknowledgement in one
  line. Announced items raised later in the same settlement (a trace
  knock-on, a new populator gap) need their own acknowledgement.
  - Rejected: silence as consent (settling announced items when the owner
    answers only the policy questions). It breaks the rule that only
    explicitly confirmed decisions are persisted.
- **The Final confirmation step is removed** (supersedes the blocking final
  confirmation of 96d2da42 and the refusal and promotion wording 0dfbd854 and
  95301971 added around it). Once announced defaults are
  acknowledged and every policy question is settled, the lead persists
  without re-showing the confirmed set for another approval. The
  trace-before-write step and the Sage reviewers remain the guard against
  interacting decisions.
  - Rejected: keeping Final confirmation. Under the two-class queue, the
    acknowledgement already shows every item at once, so the step re-asks the
    same content.
  - The section's two surviving rules move into Queue state: a correction
    returns its item to `[open]` under its existing ID (back from
    `## Decisions` into the queue section if it had moved), and an invocation
    that queued no item has no settlement step at all.
  - Every other reference to the final confirmation is reworded to the new
    settle point (announced defaults acknowledged, every policy question
    settled): the playbook's Queue state deletion rule and epic review step,
    and the two MCP refusal strings that tell a lead how to clear a pending
    queue (`openDecisionQueueRefusal` in
    `agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L367` and the
    `sage_gate` open-queue `next_instruction` in
    `agents-plugin-tool/internal/mcp/server.go#L2920`). Leaving them would
    send a lead to a step that no longer exists.
- **The queue mechanism itself stays.** Items still get stable IDs, live in the
  ticket's temporary `## Open Decision Queue` section, and block persistence
  until settled. Each queued item records its class next to its status tag,
  and an announced item records its citation in place of alternatives and a
  recommendation, so the class survives compaction. An unacknowledged
  announced item is re-shown as one line; an objected announced item is
  shown in the full policy block the first time it appears as a policy
  question. The exact markers are the implementer's choice, pinned by the
  golden test.
- **"Announced default" stays distinct from the existing per-round
  settlement announcement.** The playbook text keeps the two terms apart so
  a reader does not confuse the class with the round's settlement report.

## Constraints

- `lead-ticket` is a shipped playbook: the change follows
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/skill-authoring.md`, and its wsflow mirror follows
  `ai-docs/manuals/wsflow-mirroring.md`.
- No text may cite this repository's own tickets or commits as the rule's
  authority (Architecture Rule 4); the examples above are motivation, not
  shipped content.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/skill-authoring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/rsrc/`, `agents-plugin-wsflow/skills/`, `agents-plugin-tool/internal/wsdoc/conventions/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin-wsflow/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 0dfbd854 (2026-09-24, commit): "Correctness (minor): a lead acting only on the tool text could delete the section without the blocking final confirmation; both refusals now say \"through the final confirmation\"." — bearing: superseded (by the Final confirmation removal)
- 95301971 (2026-09-24, commit): "Promotion step 2 and the epic review step gain \"settle the queue through the final confirmation and delete the section\" before the single sage_gate mention, keeping TestLeadTicketSageGatePrecedesCommit / TestLeadTicketSettlementBoundaries intact." — bearing: superseded (by the Final confirmation removal)
- 96d2da42 (2026-09-24, commit): "User-confirmed format: full block per item only at first presentation, (n) parenthesized-number IDs, one-line [open] re-asks, only this round's settlements announced, blocking final confirmation before persisting." — bearing: superseded (by the Final confirmation removal)
- 260726-feat-doc-organization-autonomy-odq-admission-filter (2026-09-25, commit dc7f6ea1): "Absorbed ... its downstream counter-evidence is kept as the reason announced items must still be shown and acknowledged." — bearing: supports
- b833d898 (2026-09-11, commit): "Scope the Open Decision Queue exception to non-authoritative research entries so general settlement instructions cannot negate the approved research capture contract." — bearing: constrains
- 260730-feat-odq-batch-interview (2026-07-30, commit 766281e6): "Serial asking was the defect, not the gate. Queue items co-vary ... the downstream field report already recorded two of seven items being materially revised on contact with the question." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-ticket/lead-ticket.md, agents-plugin-wsflow/rsrc/lead-ticket/lead-ticket.md, agents-plugin-pi/rsrc/lead-ticket/lead-ticket.md byte mirror, agents-plugin-tool/internal/mcp/playbook_tools_test.go; possibly agents-plugin-tool/internal/wsdoc/tickets_mutate.go#L367 and agents-plugin-tool/internal/mcp/server.go#L2920 refusal text naming the final confirmation |
| scope.surface | public-interface | shipped lead-ticket playbook text in the ws, wsflow, and pi packages; MCP refusal wording if updated |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none; prose and tool-message wording only |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go TestPlaybookPrintGoldenLeadTicket pins Final confirmation phrases; agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go; agents-plugin-wsflow/tests |
| complexity.reuse_points | confirmed | existing ODQ admission, Queue state, Response format block, and trace-before-write rules in agents-plugin/rsrc/lead-ticket/lead-ticket.md#L44-L143 |
| complexity.side_effect_risk | moderate | changes the owner-consent flow of every downstream ticket settlement; final-confirmation references also sit at lead-ticket.md#L84 and #L188 and in two MCP refusal strings |
| risk.correctness | moderate | a stale final-confirmation reference outside the removed section would leave contradictory settle-then-persist instructions |
| risk.fit | moderate | shipped text must state host-neutral admission criteria without citing this repo, mirrored across three packages |
| risk.test | moderate | golden test phrases for Final confirmation must be replaced by pins for the two groups, citation rule, reversal rule, and single acknowledgement |
| risk.security_or_contract | moderate | alters the shipped explicit-confirmation-before-persistence contract by removing a blocking confirmation step |

## Phases

### Phase 1: Two-class queue and Final confirmation removal

Amend the `lead-ticket` playbook's Open Decision Queue admission, Queue state,
Response format, and Final confirmation sections to the Decisions above,
reword the remaining final-confirmation references (playbook
`lead-ticket.md#L84` and `#L188`, and the two MCP refusal strings named in
Decisions, with any test that pins their wording), and mirror the playbook
change to wsflow and to the pi resource copy (`agents-plugin-pi/rsrc/`, kept
byte-identical by `TestPiMirrorUpToDate` in
`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go`; resync it in this
phase rather than waiting for the release bump). Check whether `lead-discuss`'s
capture handoff and any contract test that pins the ODQ text need the matching
change, and edit them in this phase if so
(`agents-plugin/tests/test_skill_dispatch_contracts.py#L147-L155` pins only the
surviving `## Open Decision Queue` heading; the Queue state, Response format,
and Final confirmation phrases of `agents-plugin/rsrc/lead-ticket/lead-ticket.md#L44-L143`
are pinned by `TestPlaybookPrintGoldenLeadTicket` in
`agents-plugin-tool/internal/mcp/playbook_tools_test.go#L2220-L2281`).

Verification:

- The rendered `lead-ticket` playbook presents the two groups with the policy
  group visibly marked, states the citation requirement and the
  reversal-is-policy rule, requires one explicit acknowledgement for announced
  defaults, and no longer contains a Final confirmation step.
- `TestPlaybookPrintGoldenLeadTicket` gains pins for the two groups with the
  marked policy group, the citation rule, the reversal-is-policy rule, and
  the single explicit acknowledgement, replacing the Final confirmation pins
  rather than only deleting them.
- `grep -rni "final confirmation"` over `agents-plugin/rsrc/`,
  `agents-plugin-wsflow/rsrc/`, `agents-plugin-pi/rsrc/`, and non-test Go
  sources under `agents-plugin-tool/internal/` returns nothing (at capture
  time every match was ODQ-related; any unrelated match found later is out
  of scope and left alone).
- The full and wsflow packages' tests pass, including skill-shim and mirror
  drift tests, and `cd agents-plugin-tool && go test ./...` passes (including
  `internal/wsrsrc` pi mirror and `internal/mcp` golden tests); if
  `260924-chore-review-sweep-test-and-naming-minors` has not landed yet, run
  it with an isolated `HOME`, since host `~/.ws/config.json` tier overrides
  fail unrelated config tests.
