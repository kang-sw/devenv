---
title: ws playbook-factory pivot — spawn removal and native-subagent convergence
related:
  260605-research-ws-native-subagent-pivot: full direction discussion, decisions, evidence, and open questions
  260429-research-host-neutral-ws-plugin: prior migration anchor; this epic absorbs and supersedes its direction
  260514-epic-ws-web-dashboard-mvp: to be deprecated under milestone 0; replacement is a lightweight TUI (follow-up)
  260521-research-libws-harness-agent-substrate: deprioritized by this pivot
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
- TUI replacement for the dashboard (follow-up; only the deprecation decision
  is in scope here).
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
- Planned (M1 — playbook surface MVP): `playbook.print`/`playbook.render`,
  rsrc/ tree + manifest/schema-version contract + `WS_RSRC_ROOT` dev
  override, harness terminology/model tables (config-backed), unknown-harness
  fallback, delegation tip injection.
- Planned (M2 — skill-text conversion): retained-native-subagent delegation
  patterns replace agents.*/subquery references across skills; subquery →
  Explore absorption; entry-skill keep-list decision; internal skill bodies →
  playbooks.
- Planned (M3 — runtime deletion): remove agents.*/spawn machinery, runner
  backends, SQLite/wsstate agent state; replace actor/wsstore/authority with an
  ephemeral mandatory per-call session-key auth model (login → word-chain key;
  in-memory session→root map).
- Planned (M4 — api.ask redesign): corpus-routed api-doc playbook, cache
  index/staleness conventions, async job surface removal.

## Cross-Child Decisions

- No spawn fallback path survives anywhere; dual-path designs are rejected.
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
  replaces the existing setup-fence (requests run in parallel goroutines);
  session lifecycle/eviction (logout/TTL/LRU) is an open child-ticket detail.

## Completion Criteria

- Done: full ws runs agentless by default; all delegation flows use native
  subagents via playbooks; spawn machinery deleted; M0–M4 children closed.
- Dropped: direction reversal recorded in the research ticket.
- Deferred: memory./mutation contracts, wsflow convergence mechanics, TUI
  implementation — each leaves through its own follow-up ticket.
