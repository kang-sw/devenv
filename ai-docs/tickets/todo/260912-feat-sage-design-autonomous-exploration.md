---
title: "Let Sage design review autonomously explore code contracts"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-refactor-retire-spec-mental-model-layers: the retired document layers no longer supply Sage with a precomputed product-contract view
---

# Let Sage design review autonomously explore code contracts

## Background

After the spec and mental-model layers retired, ready promotion grounds a ticket
with `ticket-fact-populator` and then runs Sage design review. The current
populator verifies claims the ticket already makes and writes Route Facts, while
the design reviewer reads the ticket, the ready inventory, and its parent but
cannot search beyond source paths the ticket already cites. The two roles partly
overlap on ticket facts without giving design review an autonomous way to learn
the existing product contract from interfaces, callers, and behavioral tests.

The intended separation is a bounded, write-enabled factual grounder followed
by a read-only, evidence-backed planner. It deliberately bets on the reviewer's
model judgment instead of encoding another fixed routing matrix.

## Decisions

- Keep `ticket-fact-populator` as the direct codebase grounder. Within the
  ticket's scope it may search for the codebase's actual terminology, paths,
  symbols, current logic, and tests, and may correct unambiguous factual prose
  in the one ticket it owns.
- The populator must not rewrite a confirmed decision or phase goal to match the
  current implementation. An ambiguous terminology or behavior mapping is
  `unverified` or a `decision_gap`, not a choice the populator makes.
- Expand the Sage design reviewer into the evidence-backed planner. It remains
  read-only and does not directly search or navigate the codebase. It may
  autonomously spawn host-native Explore subagents to investigate existing
  interfaces, callers, behavioral tests, reuse points, compatibility effects,
  and other code facts needed to judge the ticket.
- The reviewer decides whether exploration is needed, how many explorers to
  use, and which of the `small`, `medium`, or `large` tiers fits each question.
  The playbook supplies examples of useful exploration, not mandatory triggers,
  thresholds, counts, or a routing matrix.
- Render the resolved model name and reasoning effort for each available
  exploration tier into the reviewer playbook. The reviewer passes the selected
  binding explicitly to the host-native spawn mechanism rather than relying on
  a harness default.
- Direct exploration is prohibited at the search/navigation boundary, not at
  evidence verification: after an explorer returns exact file, test, or symbol
  citations, the reviewer may open those cited artifacts to verify load-bearing
  claims before judging them.
- Treat code interfaces and behavioral tests as evidence of the existing
  product contract. They do not override a confirmed ticket decision that
  intentionally changes that contract; an unexplained conflict becomes a
  missing decision rather than an inferred policy choice.
- Add no MCP-owned spawning surface. Delegation stays host-native and
  harness-aware. The completeness reviewer remains ticket-only and unchanged.

## Constraints

- Preserve the populator's exact-one-ticket write boundary and the design
  reviewer's read-only boundary.
- The direct-exploration restriction is a playbook contract; it does not attempt
  to remove host filesystem or terminal capabilities from the reviewer.
- Preserve pointer-over-summary discipline: explorer reports carry exact
  citations, gaps, and omissions, and the reviewer may verify the cited source
  rather than treating a subagent summary as sole authority.
- Keep model mappings config-resolved for the detected harness. Do not hard-code
  provider model names or introduce a second tier configuration source.

## Phases

### Phase 1: Separate factual grounding from autonomous design exploration

Update the fact-populator and Sage design-review playbooks to establish the
grounder/planner split above. Extend playbook rendering with the missing
config-resolved reasoning-effort bindings needed to show the `small`, `medium`,
and `large` exploration choices alongside their model names, and carry the
shared resource changes through the wsflow mirror and manifests.

Verify rendered reviewer text for Claude, Codex, and the host-neutral fallback;
custom per-tier model and effort configuration; graceful unset-effort behavior;
autonomous tier and fan-out choice without hard routing criteria; prohibition
of direct search with permission to verify explorer citations; and preservation
of the populator's factual-only write boundary. Run the focused renderer,
session-role, resource-mirror, and plugin contract suites.
