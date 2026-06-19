---
title: ws playbook-factory pivot — spawn removal and native-subagent convergence
related:
  260605-research-ws-native-subagent-pivot: full direction discussion, decisions, evidence, and open questions
  260429-research-host-neutral-ws-plugin: prior migration anchor, absorbed and superseded by this epic (archived .done)
  260514-epic-ws-web-dashboard-mvp: retained as a web-tmux surface; under option B the agent-activity feed is NOT stripped (mercenary lifecycle survives as its source) — M3 only keeps it compiling, port-vs-remove deferred to a dashboard idea ticket; MCP-integration children kept (TUI-replacement plan superseded)
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
- wsflow naming/convergence mechanics before M4; final convergence is a
  post-M4 follow-up owned by `260616-refactor-wsflow-product-mode-convergence`.
- Dashboard feature changes beyond keeping it compiling against the reshaped
  surface (the dashboard is retained as a web-tmux surface). Under option B the
  agent-activity feed is NOT stripped — its mercenary lifecycle source survives,
  so port-vs-remove is a deferred product decision in a separate dashboard idea
  ticket. The earlier TUI-replacement/deprecation plan is superseded.
- Gemini harness support (explicitly excluded).

## Child Tickets

- Planned (M0 — cleanup): board and direction hygiene — absorb/close 260429,
  retain dashboard as a web-tmux surface (under option B do NOT strip
  agent-activity; M3 keeps it compiling, port-vs-remove deferred to a dashboard
  idea ticket; destructive 260514 child drops move to idea-level backlog with an
  epic pointer, still user-gated), retire
  260521, re-triage spawn-bug tickets under option B (260517 empty-result and
  260524 stale-dir live in the retained mercenary path — NOT resolved-by-deletion;
  only subquery/wsstore-busy bugs drop), promote cache/launcher race tickets to
  prerequisites, start agentless-mode (`WS_MCP_NO_AGENT=1`) dogfooding as the
  breakage-discovery forcing function.
- `260609-feat-ws-playbook-surface-mvp` (ready, M1 — playbook surface MVP):
  `playbook.print`/`playbook.render`, rsrc/ tree + manifest/schema-version
  contract + `WS_RSRC_ROOT` dev override, harness terminology/model tables
  (config-backed), unknown-harness fallback, delegation tip injection.
  Contract-first spec authored (`260609-playbook-tools`,
  `260609-playbook-harness-rendering`, `260609-rsrc-playbook-distribution`).
  Follow-up bug: `260610-bug-wsflow-runtime-contract-playbook-tools-drift`
  (idea) — agentless capabilities expose `playbook.print`/`playbook.render` but
  the wsflow `runtime.json` contract omits them; pre-existing M1-rollout drift,
  not an M2 regression.
- `260609-refactor-ws-skill-text-playbook-conversion` (done `.done/`, M2 —
  skill-text conversion): retained-native-subagent delegation patterns replace
  agents.*/subquery references across skills; subquery → Explore absorption;
  entry-skill keep-list (11 entry / 9 playbook); internal skill bodies →
  playbooks. Depends on M1. **Complete (2026-06-10), merged to epic**: Phase 1
  (`e211f87b`, subquery→Explore), Phase 2 (`704d96fb`, 9 internal procedures →
  playbooks, surface narrowed to 11 entry skills), Phase 3 (`b6850dc3`, all 11
  entry skills → thin shims over `kind:print` playbooks). The entire lead-*
  procedure corpus now lives in `agents-plugin/rsrc/`; `ws/subquery`/`agents.*`
  runtime stays callable but unreferenced by shipped skill text (deletion/reshape
  is M3).
- `260609-refactor-ws-spawn-runtime-deletion-session-auth` (done `.done/`, M3 —
  spawn reshape + session-auth): **reshape** the spawn engine into a first-class scoped
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
  M2; coordinated with M4. **Complete (2026-06-11), merged to epic `be8c39e6`**
  (phases 1+2a+2b+2c+3, --no-ff). Remaining fill (delegate role/tier asset +
  per-spawn/per-role tier routing + reviewer-tier default) re-homed to
  `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`.
- `260611-refactor-ws-tier-taxonomy-delegate-tier-routing` (**ready** — tier
  taxonomy + delegate tier routing + delegate-prompt convergence): first-class
  `small/medium/large/xlarge` tier vocab, `light/core/deep` demoted to
  concrete-model aliases, shipped delegate `role:`/`tier:` playbook asset
  (child-key splice + model vars), mercenary per-spawn tier plumbing into
  `RegisterOptions`, reviewer-tier default re-authored in first-class vocab
  (Phases 1-3). **Absorbed 2026-06-12:** the full delegate-prompt convergence —
  port delegate prompts to rsrc (P4), migrate skills off `register(prompts)`
  (P5), retire the `wsprompt` loader incl. `api.ask`/wsflow `RenderSource` rewire
  (P6). Re-homes the 260609 Phase 2c Editions. Promoted to `ready/` 2026-06-11
  (`88e646c5`). Depends on M3; P6 coordinates `api.ask` prompt source with M4.
- `260616-refactor-remove-agent-backed-api-tools` (todo, M4 — api tool
  deletion): remove the agent-backed `api.ask` family and stale workflow
  guidance from the playbook pivot. Do not redesign dependency-documentation or
  hierarchical memory in this epic. The dropped predecessor
  `260609-refactor-ws-api-ask-corpus-routing` is replaced by this deletion-only
  scope; the future pure-tooling api namespace leaves through
  `260616-epic-api-namespace-documentation-memory-tooling`.
- `260616-refactor-wsflow-product-mode-convergence` (todo, post-M4 — wsflow
  convergence): after M4, remove the curated wsflow skill-body surface and
  collapse wsflow onto product-mode playbook rendering. Long-term ws/wsflow
  differences are namespace plus capability gates: mercenary/external-agent and
  exec surfaces remain full-ws only; wsflow gets no separately maintained
  workflow procedure corpus. Until this lands, wsflow is considered temporarily
  not usable for serious dogfood.
- `260619-feat-ws-session-lineage-children` (todo, ready-candidate — session-key
  parent lineage + enumeration): forward feature on the M3 ephemeral session-key
  model. Records an optional `parent` edge at both mint paths and adds a read-only
  `ws.session.children` enumeration tool so a lead can re-discover its keys from
  one anchor after context loss. Two worktree scenarios kept distinct:
  dispatch-into (delegate leaf via the anchor-settled `render` + `root_override`
  path, leaf, non-recursive) and work-in (lead drives a worktree directly via
  `ferrule` control key, coordination lineage). Grounds on — does not change —
  the existing lead-only `ferrule` guard (`isLeadOnlyTool`) + obscurity
  soft-guard, so the anchor's strict depth-1 containment is preserved. Worktree
  creation and merge-back stay out of scope (native git tooling + the
  merge/cleanup idea tickets). Captured from the 2026-06-19 lead-discuss dogfood
  that added the per-workroot `ferrule` manual clarification (`13eeccd9`).

## Cross-Child Decisions

- Spawn engine is **retained as a scoped first-class "mercenary" surface**
  (option B, supersedes the option-C freeze): codex + claude engines live (claude
  retained for harness-neutrality, 2026-06-09), scoped to implementer/reviewer.
  Routing: default always native; mercenary always available to the lead, invoked
  only by user-explicit request or a per-key `ws/lead.prefer_mercenary(session_key)`
  render-mode flip (changes default guidance, not availability), with a small
  always-on mercenary tip in every delegation-capable rendering.
  Exploration, survey, and mental-model-update prefer native subagents. The
  runner-backend interface stays **harness-neutral/pluggable**: the `gemini.go`
  implementation is unshipped (model-compat cost) but the plug point is preserved
  for gemini/antigravity/custom harnesses (deferred plug, not deletion). Mercenary
  ("special external agent") is a deliberately distinct term from native
  "subagent" to avoid the LLM semantic collision. ws-only capability: wsflow stays
  agentless, so the mercenary/spawn axis is a ws↔wsflow divergence (partial
  convergence; the shared playbook/text core still unifies).
- `login` + mercenary spawn/lifecycle live under a lead-centric **`ws.lead.*`
  namespace** (`ws/lead.login`); session term resolved as `lead.login`
  (`session.open`/`attach` dropped). Mercenary and native share one call shape: a
  single self-contained prompt from `playbook.render`, native-shaped continuation
  handle. **Child keys are render-minted**:
  `playbook.render(session_key, name, context?, root_override?)` mints+injects a
  fresh child key when `session_key.role == lead`; `root_override` rebinds both
  the auto-include root and the child-key root for worktree children (M1
  coordinates the keyed render signature).
- **Recursion containment + key-role enforcement live on the keyed `tools/call`
  handler, server-side** (resolved 2026-06-09): the handler rejects `ws.lead.*`
  calls from non-lead keys → children cannot spawn → spawn depth strictly 1, so
  the CLI-flag spawn-depth counter is unnecessary (defense-in-depth only). Schema
  / `tools/list` filtering is a harness-owned soft-guard (LLM-confusion reduction,
  not enforcement); both the `WS_MCP_TOOL_PROFILE` env profile and schema-hiding
  are non-enforcing — enforcement is the keyed-handler role check.
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
  containment (see `named-agent-runtime` mental model), and schema/`tools/list`
  filtering is likewise only a harness-owned soft-guard — containment must be
  enforced **server-side in the keyed `tools/call` handler** by session-key role,
  not via the env var or schema omission.

- **Tier vocabulary (resolved 2026-06-11):** the abstract delegation tier is
  first-class `small/medium/large/xlarge`; `light/core/deep` are demoted to
  concrete-model aliases (alongside `haiku`/`sonnet`/`opus`). Frontmatter
  declares the first-class tier; mercenary is opt-in; `config.agents_tier` stays
  the mercenary concretion layer (`tier × harness → backend/model/effort`). The
  first-class **axis is capability level** (not subscription/plan); locked alias
  mapping `light↦small`/`core↦medium`/`deep↦large` (`xlarge` = fable-class, no
  legacy alias); reviewer-allocation default in first-class vocab is
  correctness→large, fit/test→medium.
  Owned by research `260611-research-ws-per-role-delegation-tuning-config` and
  child `260611-refactor-ws-tier-taxonomy-delegate-tier-routing` (the 260609
  Phase 2c Editions re-home there).

- **Delegate-prompt convergence + wsprompt retirement (confirmed 2026-06-12):**
  the "shared playbook/text core unifies" intent (above) resolves concretely to
  *a single prompt source of truth at `agents-plugin/rsrc/`*. M3 Phase 2c already
  moved the mercenary delegate prompt onto `playbook.render` (removed
  `register(prompts:[stems])`); the remaining work converges every delegate
  (implementer, reviewer family, reference-discovery, mental-model-updater,
  plan-populator) onto rsrc playbooks, migrates skill call sites off the removed
  register field, and **retires the `wsprompt` go:embed loader entirely** —
  including its non-delegate consumers (`api.ask` hard-coded stems, the wsflow
  `prompt.render` `RenderSource`). This crosses into M4 (`api.ask` prompt source
  → rsrc). Mental-model `prompt-bundle.md` line 27 ("deliberately parallel,
  non-overlapping loaders") is 2c drift, rewritten at closeout, not a binding
  constraint. Owned by child `260611-refactor-ws-tier-taxonomy-delegate-tier-routing`
  (absorbed there per 2026-06-12; the user chose absorption over a new milestone);
  M4 coordinates the `api.ask` prompt-source move.

- **wsflow final convergence (confirmed 2026-06-16):** the previous capture
  stopped too early by preserving curated wsflow skill bodies as if they were a
  final contract. The intended endpoint is one shared playbook/resource corpus:
  wsflow differs from full ws by product-mode namespace and by hiding/rejecting
  mercenary/external-agent plus exec capabilities, not by maintaining separate
  workflow procedure text. The cleanup is intentionally delayed until after M4
  removes the agent-backed api tool family; until then, wsflow is not a reliable
  dogfood target.

- **api namespace boundary (confirmed 2026-06-16):** M4 removes the
  agent-backed `api.ask` family instead of redesigning it inside this epic.
  Future `api.*` work is outside the playbook-factory pivot and should treat the
  namespace as deterministic documentation, corpus, hierarchical memory, and
  agent manual tooling. It must not reintroduce MCP-owned agent delegation or
  model routing.

## Completion Criteria

- Done: native subagents are the default delegation path via playbooks; the spawn
  engine is reshaped into a scoped mercenary surface (codex retained,
  implementer/reviewer only) rather than deleted; the actor model is replaced by
  session-auth; the agent-backed api tool family is removed; M0–M4 children
  closed.
- Dropped: direction reversal recorded in the research ticket.
- Deferred: memory./mutation contracts and TUI implementation leave through
  their own follow-up tickets. Future pure-tooling api namespace work leaves
  through `260616-epic-api-namespace-documentation-memory-tooling`. wsflow
  convergence is no longer an open-ended non-scope item; it is deferred
  specifically to the post-M4 child
  `260616-refactor-wsflow-product-mode-convergence`.
