---
title: Add a technical-decision class to the Open Decision Queue
related:
  260925-feat-odq-announced-defaults-and-policy-questions: supersedes its two-class decision
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 50404b9851b12949
sage-review-completeness-reviewed: 50404b9851b12949
---

# Add a technical-decision class to the Open Decision Queue

## Background

The lead-ticket Open Decision Queue (ODQ) classes each item as an announced
default or a policy question (`dd4b01b52`). An announced default needs a
citation from a closed list (language or platform constraint, prior decision,
project convention, direct consequence of a confirmed decision,
documentation placement); a policy question is defined as "every item without
such a citation".

In use, the policy group fills with items whose answer is technically clear
but argued from engineering judgment (simpler, fewer moving parts, clearer
failure mode) rather than a citation. Because the policy class is the
catch-all rather than a positive definition, such items land there, and the
user cannot tell the few choices that need their values from the many that
only need a technical sanity check. A technically fluent user wants to review
the lead's technical calls; a user without the technical picture wants to
answer only the genuine policy choices and acknowledge the rest.

## Decisions

- **Three item classes.** This supersedes the two-class decision of
  `260925-feat-odq-announced-defaults-and-policy-questions`, at the user's
  direction (2026-10-07); its other decisions stand.
  - *Announced default*: unchanged; the answer is forced by a citation from
    the existing closed list.
  - *Technical decision* (new): the lead's opinionated technical
    recommendation. It has no such citation, and its answer does not hinge on
    the user's values; the lead picks it on technical reasoning and states
    that reasoning.
  - *Policy question*: redefined positively. The answer depends on the
    user's preferences, priorities, or product direction, for example the
    audience or scale a default assumes, scope in or out, cost or risk
    tolerance, or a trade-off in downstream-observable behavior. It still
    carries the lead's recommendation in its `>` line.
- **Lead uncertainty alone is not a policy reason.** A technical call the
  lead is unsure of stays a technical decision with its reasoning stated;
  otherwise the policy group regrows.
- **A technical decision is one line**: `Will <X> over <Y> - <technical
  reason>`, so a reviewer sees the losing alternative and why it lost.
- **One acknowledgement covers the announced and technical groups together.**
  The user's work is unchanged; only the display splits. The existing
  announced-default rules apply to technical decisions as well: silence is not
  consent (unacknowledged items stay `[open]` and are re-shown one line each
  under a single re-ask), an objection turns the item into a policy question
  under its existing ID, a later policy answer that undercuts an acknowledged
  item does the same, and items raised later in the same settlement need their
  own acknowledgement.
- **The technical group label carries a one-line reader hint** that the group
  is acknowledged with the defaults and is worth reviewing for a reader who
  holds the technical picture. The hint lives in the label because that is
  where it is read at the moment of answering.
- **Display order is announced, technical, policy.** Items that need an
  answer come last, closest to where the user replies.
- **Unchanged pins.** An item that reverses a prior decision is always a
  policy question; a reviewer `missing` issue is always a policy question.
- **Surface details.**
  - Class tag `[technical]` beside `[announced]` and `[policy]`; chat label
    `**Technical decisions**`, following the existing tag and English
    bold-label convention (`dd4b01b52`).
  - A fact-populator decision gap or a reviewer issue may be classed a
    technical decision by the same rule as any other item; a reviewer
    `missing` issue stays pinned to policy.
  - The rule that holds a consequence of a still-open policy question keeps
    targeting the next round's announced defaults: once the answer lands, the
    consequence is citable, which is the announced-default citation.
  - Every place that names the settle point or the unacknowledged-group stop
    says the announced defaults and technical decisions are acknowledged: the
    playbook's settle-point sentence and `## Stops` line, the `tickets.move`
    refusal in `internal/wsdoc/tickets_mutate.go`, and the `sage_gate`
    next_instruction in `internal/mcp/server.go`.
  - The wsflow and pi rsrc mirrors of `lead-ticket.md` are resynced in the
    same change: wsflow by the regeneration order in
    `ai-docs/manuals/wsflow-mirroring.md` (manifest regen, then wsflow regen),
    pi by byte copy of `lead-ticket.md` and `manifest.json` (`d5682a35`).
  - In the ticket section, a `[technical]` item records its one-line
    `Will <X> over <Y> - <technical reason>`, the reason standing where an
    announced default's citation stands; on settlement, `<Y>` goes into the
    decision's `Rejected:` text.
  - The label and language-rule wording follows the shared acknowledgement:
    "the two group labels" becomes three, and neither the announced nor the
    technical label suggests a separate acknowledgement per group. The rule
    lives once, in the Item classes text.
  - Pinned phrases in `internal/mcp/playbook_tools_test.go` and
    `internal/mcp/tickets_sage_test.go` are updated, and pins are added for
    the technical class definition, its one-line format, the shared
    acknowledgement, the label hint, and the group order.
- Rejected: widening the announced-default citation list to admit technical
  reasoning. It dilutes the announced class's "one forced answer" promise and
  removes the review-depth split this change exists for.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 260925-feat-odq-announced-defaults-and-policy-questions (2026-09-25, Decisions): "Admission to the announced-default class requires a citable reason, one of: a language or platform constraint; a prior commit or ticket decision; an established project convention; ..." — bearing: constrains
- 260925-feat-odq-announced-defaults-and-policy-questions (2026-09-25, Decisions): "The queue has two classes, presented in two groups. Announced defaults (upper group): one line each ... Policy questions (lower group, visibly marked as such): the current block format" — bearing: contradiction-candidate
- 260925-feat-odq-announced-defaults-and-policy-questions (2026-09-25, Decisions): "Announced defaults are confirmed by one explicit owner acknowledgement covering the whole group, which may arrive in the same turn as the policy answers." — bearing: supports
- 260925-feat-odq-announced-defaults-and-policy-questions (2026-09-25, Decisions): "An item that reverses a prior decision is always a policy question, even when the lead can cite a reason for the reversal." — bearing: constrains
- dd4b01b52 (2026-09-26, commit): "Class markers chosen as `[announced]`/`[policy]` class tags after the status tag ... bold rather than `##` sub-headings so a lead copying the response into the ticket cannot end the section early." — bearing: supports
- dd4b01b52 (2026-09-26, commit): "A 'settle point' term is defined once in the section intro and reused by the Queue state deletion rule, the epic review and promotion steps, and both MCP refusal strings" — bearing: constrains
- d5682a35 (2026-09-26, commit): "Mirror process: WSRSRC_REGEN manifest regen, then WS_REGEN_WSFLOW_RSRC regen. The pi rsrc has no regen entrypoint, so lead-ticket.md and manifest.json were copied and checked with diff -r." — bearing: constrains
- d5682a35 (2026-09-26, commit): "items are re-shown unchanged as one-line forms, under a single group acknowledgement re-ask. The rule lives only in the Item classes bullet" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/lead-ticket/lead-ticket.md plus wsflow and pi mirrors and three rsrc/manifest.json, agents-plugin-tool/internal/wsdoc/tickets_mutate.go, agents-plugin-tool/internal/mcp/server.go, playbook_tools_test.go, tickets_sage_test.go |
| scope.surface | public-interface | shipped lead-ticket playbook text and MCP refusal strings returned to callers by tickets.move and tickets.sage_gate |
| scope.new_public_symbol | no | none; only string constants openDecisionQueueRefusal and the sage_gate next_instruction change |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin-tool/internal/mcp/tickets_sage_test.go, agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go, wsflow_mirror_test.go |
| complexity.reuse_points | confirmed | existing two-class Item classes, Response format, and settle-point text in lead-ticket.md L44-L190 read |
| complexity.side_effect_risk | moderate | refusal strings are pinned in tests and three mirror trees plus manifest digests must stay byte-identical |
| risk.correctness | moderate | the policy class is redefined positively and every settle-point and stop reference must change consistently |
| risk.fit | moderate | observable workflow behavior change in a shipped playbook that must keep the existing English tag and bold-label conventions |
| risk.test | moderate | existing golden pins must be updated and new pins added for the class definition, format, shared acknowledgement, label hint, and order |
| risk.security_or_contract | moderate | caller-visible refusal text of tickets.move and tickets.sage_gate changes |

## Phases

### Phase 1: Technical-decision class in the lead-ticket queue

Edit `agents-plugin/rsrc/lead-ticket/lead-ticket.md` `## Open Decision Queue`
(item classes, queue state tags, response format, settle point) and its
`## Stops` line per the decisions above; resync the wsflow and pi mirrors;
update the two MCP refusal strings; update and add test pins.

Verification: `cd agents-plugin-tool && go test ./...`; the wsflow package
tests (`python3 -m unittest discover -s agents-plugin-wsflow/tests`); the pi and wsflow rsrc byte pins are
Go tests covered by `go test ./...`, not the pi TypeScript suite
(`agents-plugin-tool/internal/wsrsrc/pi_mirror_test.go#L30`,
`agents-plugin-tool/internal/wsrsrc/wsflow_mirror_test.go#L54`; no
`agents-plugin-pi/test/` file pins `lead-ticket.md` bytes); those mirror
guards compare whole rsrc trees, so the `lead-ticket/lead-ticket.md` digest in
each `rsrc/manifest.json` (`agents-plugin/rsrc/manifest.json#L28`) must be
regenerated too, and `TestValidateRealTree` fails on a stale canonical digest
(`agents-plugin-tool/internal/wsrsrc/wsrsrc_test.go#L887-L892`; regen via
`TestGenerateRealManifest` with `WSRSRC_REGEN=1`, `wsrsrc_test.go#L948-L955`).
A rendered `ws/playbook.read(name: "lead-ticket")` shows three classes
in the order announced, technical, policy, with no remaining "every item
without such a citation" definition of the policy class.
