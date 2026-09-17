---
title: Ticket selector can choose a ready ticket with a current Blocked note
related:
  260908-feat-ws-pi-claude-delegate-tool: first blocked-ticket example selected during dogfood
  260911-bug-ws-pi-question-queue-dogfood: first advanceable ticket omitted by the incorrect selection
  260914-chore-ws-pi-root-manifest-runtime-deps: second blocked-ticket example selected during dogfood
  260916-feat-pi-agent-gutter-active-time-placement: second advanceable ticket omitted by the incorrect selection
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: d640aebcd0a0db61
sage-review-design-reviewed: d640aebcd0a0db61
---

# Ticket selector can choose a ready ticket with a current Blocked note

## Background

During a Pi goal-run drain on 2026-09-13, `ticket-selector` first reported that every remaining ready ticket was blocked and correctly reproduced the owner-acceptance blocker on `260908-feat-ws-pi-claude-delegate-tool`. After an unrelated stale blocker was cleared from `260911-bug-ws-pi-question-queue-dogfood`, a fresh selector invocation chose the still-blocked Claude delegate ticket instead of the newly advanceable question-queue ticket. A correction prompt that explicitly required skipping current `## Blocked` sections then selected the question-queue ticket.

The same failure reproduced on 2026-09-16 outside a goal run. The selector chose `260914-chore-ws-pi-root-manifest-runtime-deps`, whose body carries `## Blocked (2026-09-16)` and explicitly says no worker or lead-run can advance the remaining owner-only smoke, ahead of the advanceable `260916-feat-pi-agent-gutter-active-time-placement` ticket.

The selected tickets had no `blocked-by:` frontmatter edge. `tickets.query` therefore correctly emitted no `dispatch_blocked`: that projection is the typed hard gate for unlanded prerequisites only. The body-level Blocked marker was absent from both the JSON inventory and the default compact-text output. The small-model selector was expected to open every candidate body to discover it, omitted that read, and then selected the older ticket. `lead-run` trusted the returned `selection:` without a queue-selection backstop.

This is an evidence-visibility failure, not a request to infer hard execution state from arbitrary body prose. The selector needs the literal body marker in its ordinary inventory and must inspect the referenced section before deciding whether the blocker is current.

## Decisions

- Keep `blocked-by:` and the derived `dispatch_blocked` value as the only typed hard prerequisite gate. A body-level Blocked section does not become `dispatch_blocked` and does not by itself create a hard MCP refusal.
- Extend the existing ticket-body parser to collect literal Markdown heading lines that begin with `## Blocked`. Do not interpret the heading suffix or section prose; values such as `— RESOLVED ...` are returned verbatim rather than classified by code.
- Expose the raw Blocked heading lines consistently in both `tickets.query` JSON and its default compact-text output. Discovery listings must carry the marker because that is the inventory used by `ticket-selector`; point resolution must preserve the same evidence.
- Keep the raw body marker visibly distinct from `dispatch_blocked` so callers cannot confuse advisory evidence with the typed dependency gate.
- When a queue candidate carries a raw Blocked heading marker, `ticket-selector` reads the corresponding ticket section and judges whether the blocker is current before selecting or skipping it.
- For queue-selected work, `lead-run` rechecks the selected ticket body before worker dispatch so a selector omission cannot silently launch work into a recorded blocker. When the recheck finds a current blocker, `lead-run` treats the ticket as the selector should have — drop it from this cycle and re-select the next candidate, falling through to the existing all-blocked result when no advanceable candidate remains — and never dispatches into the recorded blocker. Preserve direct named-ticket invocation behavior; an explicit user-selected ticket is not replaced by queue ordering.
- When every remaining ready ticket is judged blocked, retain the existing all-blocked result and include the recorded blocker inventory. Do not make blocked tickets disappear from diagnostics.

## Constraints

- Preserve the existing `blocked-by:` promotion and dispatch behavior.
- Do not add natural-language classification of owner actions, dependency state, or resolution wording to `tickets.query`.
- Do not require worker dispatch merely to rediscover a body-level blocker already present in the ticket.
- Keep JSON and compact-text projections behaviorally aligned.
- Mirror the `ticket-selector.md` and `lead-run.md` edits byte-identically into `agents-plugin-pi/rsrc/` as well as `agents-plugin-wsflow/rsrc/`; the pi package carries no declared Convention manual row but is an in-scope mirror (Route Facts `scope.span`), so keep all three copies identical.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/rsrc/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdoc/tickets.go, agents-plugin-tool/internal/mcp/server.go, agents-plugin/rsrc/ticket-selector/ticket-selector.md, agents-plugin/rsrc/lead-run/lead-run.md, plus their byte-identical agents-plugin-wsflow/rsrc/ and agents-plugin-pi/rsrc/ mirrors (confirmed identical via diff) |
| scope.surface | public-interface | tickets.query is an MCP tool whose JSON and compact-text output contract gains a new field (server.go:3764 tool description; TicketInfo struct at tickets.go:46-84 has no such field today) |
| scope.new_public_symbol | yes | a new TicketInfo field carrying the raw literal "## Blocked" heading line, absent today |
| scope.new_type_contract | yes | tickets.query's JSON schema and its default compact-text formatter (formatTickets, server.go:2795-2829) both gain the new field; no such line exists today |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/ticket_dispatch_gate_test.go, agents-plugin-tool/internal/mcp/playbook_tools_test.go, agents-plugin/tests/test_skill_dispatch_contracts.py |
| complexity.reuse_points | confirmed | existing Blocked-heading writer (appendOrReplaceBlockedSection, tickets_sage.go:626) and existing body/frontmatter extraction pattern (ticketRouteFacts, tickets.go:581) are direct precedent; ticket-selector.md and lead-run.md are already byte-identical across the agents-plugin/, agents-plugin-wsflow/, and agents-plugin-pi/ rsrc mirrors |
| complexity.side_effect_risk | moderate | changes a shipped MCP tool's output contract consumed by the ticket-selector and lead-run playbooks across three mirrored packages |
| risk.correctness | moderate | must return the heading line verbatim without inferring resolved/current semantics, and lead-run's new pre-dispatch recheck must not replace direct named-ticket invocation |
| risk.fit | low | follows the established dispatch_blocked-gate-plus-advisory-marker shape and the existing byte-identical rsrc mirror pattern |
| risk.test | moderate | new heading-extraction and selection-recheck behavior needs new test coverage; existing test files give a pattern to extend but none cover this today |
| risk.security_or_contract | moderate | a shipped-surface contract change to a caller-facing MCP tool; the ticket's own constraint requires JSON/compact-text stay aligned and the advisory marker must not become a new hard gate |

## Phases

### Phase 1: Surface raw Blocked headings and make queue selection consume them

Add the raw body-heading projection to the shared ticket parser and both `tickets.query` output modes. Update serial ready selection and its lead-side pre-dispatch check to use the marker as a cue to inspect the ticket section, while retaining typed prerequisite behavior and direct named-ticket invocation.

Verification:

- Reproduce each mixed ready queue: one earlier ticket with a body `## Blocked` marker and one later advanceable ticket. Queue selection must return the advanceable ticket.
- Cover an all-blocked queue and assert that each blocked ticket and its recorded reason remains reportable.
- Assert matching raw marker evidence in JSON and default compact-text discovery and point-resolve output.
- Include a nonstandard heading such as `## Blocked (2026-07-27) — RESOLVED 2026-08-11`; assert that the parser returns the complete literal line without assigning resolved/current semantics.
- Preserve selection and `dispatch_blocked` behavior for typed prerequisite blocks, direct named tickets, and ready tickets without a body marker.
