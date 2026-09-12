---
title: "Let Sage design review autonomously explore code contracts"
parent: 260909-epic-ws-worker-interpreter-refoundation
related:
  260909-refactor-retire-spec-mental-model-layers: the retired document layers no longer supply Sage with a precomputed product-contract view
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 78612a145d41ff79
sage-review-design-reviewed: 78612a145d41ff79
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

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/skill-authoring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/rsrc/, agents-plugin-wsflow/skills/, agents-plugin-tool/internal/wsdoc/conventions/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/rsrc/, agents-plugin/skills/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Preserve the populator's exact-one-ticket write boundary and the design
  reviewer's read-only boundary.
- The direct-exploration restriction is a playbook contract; it does not attempt
  to remove host filesystem or terminal capabilities from the reviewer.
- Preserve pointer-over-summary discipline: explorer reports carry exact
  citations, gaps, and omissions, and the reviewer may verify the cited source
  rather than treating a subagent summary as sole authority.
- Keep model mappings config-resolved for the detected harness. Do not hard-code
  provider model names or introduce a second tier configuration source.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/rsrc/ticket-fact-populator/ticket-fact-populator.md, agents-plugin/rsrc/ticket-reviewer-design/ticket-reviewer-design.md, agents-plugin-wsflow/rsrc/ticket-reviewer-design/ticket-reviewer-design.md, and agents-plugin-tool/internal/mcp/playbook_tools.go |
| scope.surface | public-interface | The rendered ticket reviewer and fact-populator playbooks are shipped workflow interfaces; playbook.render is an MCP surface (agents-plugin-tool/internal/mcp/server.go#L3711-L3714). |
| scope.new_public_symbol | no | No new skill, MCP tool, or named exported symbol is specified; the ticket changes existing rendered playbooks and bindings. |
| scope.new_type_contract | no | The phase names no new Go type, method, or MCP request/response signature. |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/playbook_render_surface_test.go#L419-L500 and agents-plugin-tool/internal/mcp/playbook_tools_test.go cover rendered binding and playbook behavior. |
| complexity.reuse_points | confirmed | Existing fixed-tier model rendering is implemented by resolveTierModelVars (agents-plugin-tool/internal/mcp/playbook_tools.go#L127-L157), and native spawn consumes recommended bindings (agents-plugin/rsrc/lead-workflow-manual/native-spawn-binding.codex.md#L1-L9). |
| complexity.side_effect_risk | high | The reviewer's promotion-time behavior gains autonomous host-native delegation and cross-harness model and effort selection. |
| risk.correctness | high | A wrong direct-exploration or citation-verification boundary can make Sage's design judgment depend on unsupported or unchecked product-contract evidence. |
| risk.fit | high | Full ws and wsflow reviewer resources must remain mirrored while the renderer gains per-tier effort bindings. |
| risk.test | high | The phase requires Claude, Codex, fallback, custom configuration, unset effort, mirror, session-role, and plugin-contract coverage. |
| risk.security_or_contract | moderate | The change expands a read-only reviewer into a host-native Explore dispatcher while retaining the stated no-MCP-spawn boundary. |

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
