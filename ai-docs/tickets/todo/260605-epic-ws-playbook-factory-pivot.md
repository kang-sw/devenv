---
title: ws playbook-factory pivot — spawn removal and native-subagent convergence
related:
  260605-research-ws-native-subagent-pivot: full direction discussion, decisions, evidence, and open questions
  260429-research-host-neutral-ws-plugin: prior migration anchor, absorbed and superseded by this epic (archived .done)
  260514-epic-ws-web-dashboard-mvp: retained as a web-tmux surface; M0 strips only agent-audit/agent-activity logic and keeps MCP-integration children (TUI-replacement plan superseded)
  260521-research-libws-harness-agent-substrate: deprioritized by this pivot (dropped .dropped)
  260523-bug-ws-mcp-launcher-runtime-repair-race: prerequisite — binary/text swap race for rsrc distribution
  260524-bug-codex-plugin-cache-refresh-mcp-startup-race: prerequisite — plugin cache refresh race for rsrc distribution
---

# ws playbook-factory pivot — spawn removal and native-subagent convergence

## Scope

Convert ws from "agent runtime + workflow text" into a **workflow knowledge
server**: harness-aware playbook factory (print/render), rsrc/ plain-text
prompt distribution, total removal of subprocess-spawn agent machinery, and
replacement of every delegation pattern with retained harness-native
subagents. Direction, decisions, and evidence live in
`260605-research-ws-native-subagent-pivot`.

## Non-Scope

- memory.* / ticket-mutation MCP tool contracts (deferred follow-up).
- wsflow naming/convergence mechanics (company compat; follow-up).
- Dashboard growth beyond the M0 agent-audit/agent-activity strip (the
  dashboard is retained as a web-tmux surface; broader MCP-integration features
  are follow-up). The earlier TUI-replacement/deprecation plan is superseded.
- Gemini harness support (explicitly excluded).

## Child Tickets

- Planned (M0 — cleanup): board and direction hygiene — absorb/close 260429,
  retain dashboard as a web-tmux surface (strip only agent-audit/agent-activity
  logic, keep MCP-integration children; destructive child drops stay
  user-gated), retire
  260521, mark spawn-bug tickets (260517, 260524 stale-dir) as
  resolved-by-deletion candidates, promote cache/launcher race tickets to
  prerequisites, start agentless-mode (`WS_MCP_NO_AGENT=1`) dogfooding as the
  breakage-discovery forcing function.
- `260609-feat-ws-playbook-surface-mvp` (ready, M1 — playbook surface MVP):
  `playbook.print`/`playbook.render`, rsrc/ tree + manifest/schema-version
  contract + `WS_RSRC_ROOT` dev override, harness terminology/model tables
  (config-backed), unknown-harness fallback, delegation tip injection.
  Contract-first spec authored (`260609-playbook-tools`,
  `260609-playbook-harness-rendering`, `260609-rsrc-playbook-distribution`).
- `260609-refactor-ws-skill-text-playbook-conversion` (todo, M2 — skill-text
  conversion): retained-native-subagent delegation patterns replace
  agents.*/subquery references across skills; subquery → Explore absorption;
  entry-skill keep-list (11 entry / 9 playbook); internal skill bodies →
  playbooks. Depends on M1.
- `260609-refactor-ws-spawn-runtime-deletion-session-auth` (todo, M3 — spawn
  reshape + session-auth): **reshape** the spawn engine into a first-class scoped
  "mercenary" surface (option B, supersedes the option-C freeze): retain the codex
  runner live (claude OPEN), drop gemini/subquery/exploration-spawn/diagnostic
  sprawl, scope mercenary to implementer/reviewer (exploration + mental-model
  update → native), route by user-explicit request or config-advised
  `playbook.render`, drop `register(prompts:[stems])` for native-parity single
  prompts; replace actor/wsstore/authority with an ephemeral mandatory per-call
  session-key auth model (login → word-chain key; in-memory session→root map);
  exec stateless; role-containment folded into capability-scoped keys; dashboard
  agent-audit strip. Bug-ticket disposition SPLITS (subquery/wsstore-busy dropped;
  agent-empty-result/register-stale re-triaged on the retained path). Depends on
  M2; coordinated with M4.
- `260609-refactor-ws-api-ask-corpus-routing` (todo, M4 — api.ask redesign):
  corpus-routed api-doc playbook, cache index/staleness conventions, async job
  surface removal. Depends on M1; coordinated with M3.

## Cross-Child Decisions

- Spawn engine is **retained as a scoped first-class "mercenary" surface**
  (option B, supersedes the option-C freeze): codex engine live, scoped to
  implementer/reviewer, routed by user-explicit request or config-advised
  `playbook.render`. Exploration, survey, and mental-model-update prefer native
  subagents. Mercenary ("special external agent") is a deliberately distinct term
  from native "subagent" to avoid the LLM semantic collision. ws-only capability:
  wsflow stays agentless, so the mercenary/spawn axis is a ws↔wsflow divergence
  (partial convergence; the shared playbook/text core still unifies).
- Mercenary and native share one call shape: a single self-contained prompt from
  `playbook.render`, native-shaped continuation handle; recursion is bounded by
  lead-only spawning (mercenaries are leaf) with a server-side spawn-depth
  backstop (the `WS_MCP_TOOL_PROFILE` env barrier is verified non-functional).
- Retained agent = fast path; fresh spawn + resume brief = recovery path.
  Reuse guarantees end at lead-context lifetime (tip-only continuity).
- Harness differences ship as data (terminology/model tables, overlays);
  model-name tables are user-updatable config, never baked into text or
  binary.
- Text-only playbook changes must be distributable without a binary version
  bump (schema-version compatibility, not hash equality).
- Playbook load failure is loud and partial; no embedded fallback text.
- ws-mcp uses mandatory ephemeral per-call session keys (auth model); no
  persistent actor/authority state; the session carries the project root;
  in-memory session→root map (no SQLite). The map is concurrency-safe and
  replaces the existing setup-fence (requests run in parallel goroutines). No
  logout, no eviction (rows tiny + bounded). An `unknown_session → re-login`
  contract guard is mandatory on every keyed call, making a later persistent
  backend a contract-invariant implementation swap.
- Role-containment moves to the **session key (server-side)**: a key carries
  `{root + optional capability/role scope}`, so the lead can mint
  capability-scoped keys for delegates. Soft guard (re-`login` can re-escalate);
  key issuance reserves a capability-scope parameter from the first cut. NOTE:
  the legacy `WS_MCP_TOOL_PROFILE` env profile is **verified non-functional** for
  containment (see `named-agent-runtime` mental model) — containment must be
  enforced server-side on the key, not via the env var.

## Completion Criteria

- Done: native subagents are the default delegation path via playbooks; the spawn
  engine is reshaped into a scoped mercenary surface (codex retained,
  implementer/reviewer only) rather than deleted; the actor model is replaced by
  session-auth; M0–M4 children closed.
- Dropped: direction reversal recorded in the research ticket.
- Deferred: memory./mutation contracts, wsflow convergence mechanics, TUI
  implementation — each leaves through its own follow-up ticket.
