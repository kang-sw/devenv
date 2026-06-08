---
title: ws native-subagent pivot — spawn removal and playbook-factory direction
related:
  260429-research-host-neutral-ws-plugin: migration anchor this discussion supersedes; epic absorbs its direction
  260605-epic-ws-playbook-factory-pivot: skeleton epic carrying this direction into milestones
  260514-epic-ws-web-dashboard-mvp: dashboard epic to be deprecated and downgraded to a lightweight TUI
  260521-research-libws-harness-agent-substrate: opposite-direction substrate research, deprioritized by this pivot
  260517-bug-ws-agent-empty-result-after-tool-use: spawn-machinery bug resolved by deletion under this direction
  260524-bug-ws-agent-register-stale-dir-result-hang: spawn-machinery bug resolved by deletion under this direction
  260523-bug-ws-mcp-launcher-runtime-repair-race: binary/text swap race promoted to prerequisite for rsrc distribution
  260524-bug-codex-plugin-cache-refresh-mcp-startup-race: plugin cache race promoted to prerequisite for rsrc distribution
  260525-bug-codex-local-marketplace-worktree-cache-regression: local marketplace cache fidelity affects rsrc dev loop
related-mental-model:
  - named-agent-runtime
  - prompt-bundle
  - workflow-skills
  - mcp-runtime
  - api-documentation-cache
---

# ws native-subagent pivot — spawn removal and playbook-factory direction

## Background

Full-session lead-discuss capture (2026-06-05). The user proposed a sweeping
redirection of the ws workflow system. Original agenda (translated/condensed):

1. Remove ws's own subprocess-spawn-based agent management (`agents.*`);
   depend exclusively on harness-native subagents.
2. ws stays harness-neutral: ws-mcp is reframed as a **prompt factory** that
   feeds harness-native subagents (playbook generation for native subagents).
3. Assume **durable/retained subagents** are a baseline harness capability.
4. Combine harness detection with a playbook API (`mcp.playbook`-style) that
   loads harness-specific workflow manuals — reducing dependence on each
   harness's native skills feature — and templates `light/core/deep` aliases
   into concrete per-provider model names.
5. wsflow loses its distinguishing trait (it is already agentless); wsflow is
   company-distributed so name/compat must be preserved — follow-up topic.
6. Integrate ticket/index/memory management more aggressively into ws-mcp;
   introduce layered memory.
7. Drastically shrink the skills surface — keep only directly-executed entry
   skills; migrate agent-internal skills to MCP playbook commands.

This ticket records the settled decisions, their rationale, rejected
alternatives, empirical evidence, and the open questions for the continuation
session. The companion epic `260605-epic-ws-playbook-factory-pivot` holds the
milestone board.

## Settled Direction (summary)

- **Total spawn removal.** All subprocess-spawn-based ws subagent machinery is
  removed. Every existing delegation pattern is replaced with retained
  harness-native subagent patterns. No fallback spawn path is kept.
- **`subquery` is absorbed** into per-harness exploration subagents (Claude and
  Codex both expose an `Explore`-style agent type); only terminology is
  rendered per harness.
- **`api.ask` is redesigned** on native subagents with routing simplified from
  model judgment to corpus design (see dedicated topic below).
- **Playbook API splits into two commands**: `playbook.print` (lead-facing
  procedure, returned inline) and `playbook.render` (subagent injection
  prompt, written to a tmp file, path returned). Single-command-with-metadata
  was rejected as needless flexibility loss.
- **Harness-aware routing inside MCP** becomes a first-class content-selection
  input. Harness set: claude + codex only. **Gemini is explicitly excluded**
  (not used). Unknown harness falls back to host-neutral text.
- **Prompts/playbooks ship as plain text under a plugin-path `rsrc/` tree**,
  loaded at call time — not go:embed, not Go raw literals.
- **agentId continuity is tip-only**: render/print tool results carry a short
  reminder to reuse the harness-returned agent id for continuation. No MCP
  registry, no mandated memory-file recording.
- **Skill surface shrinks to entry-only skills**; internal skill bodies move to
  `playbook.print` content. Goal includes reducing dependence on harness-native
  skills features.
- **Dashboard is deprecated**, downgraded to a lightweight TUI that browses
  AI-generated content (tickets, index, etc.) — details are a follow-up topic.
- **Migration forcing function**: run full ws in the existing agentless mode
  (`WS_MCP_NO_AGENT=1`) and treat everything that breaks as the work list.

## Key Reframe: ws converges onto wsflow's architecture

The session's load-bearing discovery: roughly half of the proposal already
exists in production. wsflow runs agentless (`WS_MCP_NO_AGENT=1` filters
`agents.*`), and `wsflow/prompt.render` already renders bundled prompt stems
(allowlist: reference-discovery, plan-populator-survey, plan-populator-research,
code-reviewer, mental-model-updater) to tmp files for native subagents, with
`\bws[/:]` namespace substitution and a Render Context injection block.

Consequences:

- Agenda items 1+2 restate as "promote wsflow's agentless model to ws default,
  extend with harness templating and a playbook surface" — a path already
  shipped and dogfooded at the company, which materially de-risks the pivot.
- Agenda 5's "distinction disappears" is the natural endpoint. Once converged,
  the current manual curated-mirror process (`ai-docs/ref/wsflow-mirroring.md`,
  forbidden-reference tests over `agents-plugin-wsflow/`) can be replaced by
  namespace rendering from a single source — the mirroring maintenance burden
  mostly disappears. (Naming/compat handling remains a follow-up topic.)

## Empirical Evidence: Claude Code durable subagents (verified 2026-06-05)

Direct in-session test on Claude Code 2.1.165:

1. Spawned a named subagent holding a secret codeword ("TANGERINE-47"), told
   not to write it to any file. Completed; harness returned
   `agentId: a271a531113d175f0`.
2. Sent `SendMessage(to: <agentId>)` to the **completed** agent. Harness
   responded: "had no active task; resumed from transcript in the background".
3. Final output of the resumed turn: `TANGERINE-47` — full context retained.
   Transcript inspection confirmed the original conversation continued with
   the recall question appended as a new turn.

Documentation cross-check (claude-code-guide agent):

- Completed-subagent resume with full transcript is **officially documented
  and stable** at the Agent SDK level (`resume: sessionId` + agent id).
- Transcripts persist ~30 days (`cleanupPeriodDays` default), survive main
  conversation compaction, and are resumable across session restarts if
  session id + agent id are persisted externally.
- `SendMessage` is formally an agent-teams (experimental) feature, but
  SendMessage-by-**agentId** worked without teams enabled (empirically
  verified above). Known GitHub issue: SendMessage-by-**name** can silently
  fail — **agentId is the stable addressing key**.
- Caveat: the SendMessage-by-agentId path sits in a fuzzily documented zone,
  so playbook rendering should be harness+version aware.

Additional transcript byproduct: **native Claude subagents inherit the full ws
MCP toolset** (`mcp__plugin_ws_ws__*` visible in the probe's context). Existing
delegation patterns (implementer calling `ws/git.commit`, `tickets.find`, ...)
survive the pivot unchanged.

Structural observation: Claude Code's native resume does exactly what ws's
`claude.go` runner implemented manually (session-id persistence → resume
spawn). The harness absorbed what ws built, strengthening the removal case.

Codex: the user separately verified retained-subagent capability. Parallel
fan-out (multiple concurrent explore agents) on Codex remains unverified.

## Decision: total spawn removal (scope and casualties)

Options considered:

- A. Full conversion — MCP never spawns model processes (chosen; api.ask
  joins via redesign, see below).
- B. Delegation-only removal, keep subquery/api.ask internal model calls
  (rejected: inconsistent model, keeps spawn machinery alive).
- C. Native-default with spawn fallback (rejected: dual-path maintenance
  preserves the entire bug surface — 260517 empty result, 260524 stale-dir
  and register races — and forfeits the simplification payoff).

Removal inventory (from session survey):

- `agents.*` lifecycle/diagnostic tools (register/call/wait/result/status/
  tail/print/cancel/erase/interrupt/debug.*), the `SelfWorkerStarter` async
  spawn path, codex/claude/gemini Runner backends, SQLite role pointers and
  instance state, wsstate file-backed agent state.
- `subquery` (ephemeral spawned agents) — absorbed by native Explore agents.
- `api.ask` spawn machinery including the async job surface
  (`api.ask_async/status/result/cancel`) — replaced per the api redesign.

Casualties accepted:

- **Observability**: `agents.tail/status/debug` and the dashboard's
  agent-activity sources disappear; in exchange, harness-native visibility of
  subagents is generally better in-harness.
- **Uniform cross-harness semantics** (wait/result/cancel identical
  everywhere) are delegated to per-harness UX.
- **260521 libws-harness** substrate research is deprioritized/retired.
- **Actor model rework (hidden dependency)**: the current `ws.setup` actor
  bootstrap assumes per-process separation; native subagents share the lead's
  MCP server instance, so actor boundaries shift from process-level to logical
  sessions within one server. Needs design before removal lands.
- Wins by deletion: the spawn bug backlog (260517, 260524 stale-dir/register
  race) is resolved by removing the code it lives in.

## Decision: subquery → harness Explore absorption

The async fire-and-forget + deferred-result pattern (lead launches several
surveys in parallel, collects later) maps to native background subagents on
Claude; Codex parallel fan-out needs one verification pass. The subquery
prompt-stem text (evidence discipline, scoping) becomes a render-kind playbook;
terminology (agent type name, spawn idiom) renders per harness.

## Decision: api.ask redesign — routing moves from model judgment to corpus design

Current architecture exists because ws-spawned agents were expensive and
domain sessions were durable: a pre-router agent picks the domain, then a
per-domain manager session answers, with runtime-owned cache access and stale
checks. Under retained native subagents that premise is gone. Agreed shape:

- One autonomous native subagent explores the cache corpus directly; the
  sophisticated two-stage routing is deleted.
- **Routing → index file**: `api.list`'s role degrades into a cache-root
  `index.md` (domain list + one-line descriptions). The playbook instructs
  "read the index first, then descend into the domain doc". Routing accuracy
  becomes a property of index quality — debuggable, human-fixable surface.
- **Durable domain knowledge lives in cache files only.** Domain-manager
  session continuity is dropped; its main value (avoiding cache re-reads) is
  cheap for an Explore-style agent.
- **Staleness → corpus metadata**: `fetched_at` / `source_url` frontmatter on
  cached docs; the playbook states the staleness rule. Runtime logic goes to
  zero.
- **Async job machinery dies** with the rest of spawn removal; native
  background subagents replace it.
- Cache write discipline: concurrency is low and doc caches tolerate
  last-write-wins, but the playbook must mandate "one file per domain,
  whole-file replacement" to preclude partial-update collisions.

Net: the whole `api.*` surface reduces to one api-doc playbook plus a cache
directory convention — small enough to include in the first epic scope.

## Decision: playbook API — print/render command split

- `playbook.print(name, context?)` — lead-facing procedure text returned
  inline in the tool result; successor of internal-skill bodies.
- `playbook.render(name, context?)` — subagent injection prompt written to a
  worktree-scoped tmp file, path returned; direct promotion of wsflow's
  existing `prompt.render` (main work: lift the allowlist, expose under ws).
- Rejected: single tool with output-kind metadata/file unification — reduces
  flexibility without compensating benefit. The split also prevents the
  regression where full delegate prompts land in the lead's context.

## Decision: harness-aware routing — structure as data

- Detection primitives already exist (`detectHarnessFromRaw` → claude/codex,
  per-session `observeHarness`); the change is making the detected harness a
  first-class input to playbook content selection.
- **Shared body, harness differences as data**: a per-harness terminology
  table — e.g. `{explore_agent, spawn_idiom, continue_idiom:
  "SendMessage(to: <agentId>)", models: {light, core, deep}}`. Structural
  divergence only via per-harness overlay sections (e.g. `subquery.md` +
  `subquery.codex.md`).
- **Model-name tables live in config** (extension of `config.agents_tier`),
  not in the binary/text bundle — concrete model names churn per provider
  release; users must be able to update without redistribution.
- **Unknown-harness fallback contract**: render host-neutral text in the
  current skill prose style. This preserves the harness-neutral doctrine even
  with a 2-way harness set.
- Gemini: explicitly out of consideration (not used).

## Decision: prompts as plugin-path `rsrc/` plain text

Evolution across the session (record kept because each step was argued):

1. go:embed .md bundle (status quo) — binary-atomic but text edits require
   rebuild; nested-file discovery limitation.
2. Go raw literals, one file per playbook, typed params, golden-file tests —
   proposed when rebuild was judged cheap; gives compile-time completeness.
3. **Chosen: `rsrc/` plain-text tree in the plugin distribution path**, loaded
   at call time. MCP and plugin ship together; the MCP execution path is the
   plugin path.

Why 3 beats 2: workflow-text churn ≫ Go-code churn in this project, so the
text loop dominates; call-time file loading makes edits live without rebuild
or reconnect; PR review reads source text directly (golden files become
unnecessary synthesis); consistent with how the system already distributes
text (skills as files, wsflow forbidden-reference tests over distributed
text, `convention.read`/`infra.read` bundled text can unify on the same
loader); prompts become human/TUI-readable.

Rebuild re-evaluation that enabled this chain: incremental `go build` is
seconds; the real cost was never compilation but (a) MCP server reconnect to
swap binaries and (b) the binary-swap plumbing races (260523 launcher repair,
260524 cache refresh, 260525 local marketplace cache). Those races bite at
swap time regardless of approach — they are **promoted from idle backlog to
prerequisites** of this direction.

Accompanying contract (conditions of the choice):

- `rsrc/manifest.json` with file hashes + **playbook schema version**; binary
  checks schema-version compatibility, not exact hash — text-only changes
  must ship without a binary version bump (a core benefit of the approach).
- Mismatch ⇒ loud failure of the playbook surface; **no embedded fallback
  copy** (split-brain drift risk; an agentless MCP without playbooks still
  serves discovery/git tools, partial death is acceptable and visible).
- **Dev override**: `WS_RSRC_ROOT=<repo>/.../rsrc` env var, because dogfood
  MCP reads the plugin **cache copy** — without the override, repo edits wait
  on cache refresh and the iteration win evaporates. Include in MVP.
- "One code per prompt" survives as a layout convention: one directory per
  playbook, frontmatter schema, harness overlay naming; CI validates the tree
  (required variants present, substitution variables declared). Go side
  shrinks to loader + validator + substitution engine.
- Open verification item: confirm Codex plugin distribution materializes
  non-skill `rsrc/` directories into its cache (skills are known to work).

## Decision: agentId continuity — tip-only

Evolution (each layer of state ownership peeled off in turn):

1. MCP-owned role→agentId registry (SQLite role-pointer successor) —
   rejected: harness-supplied agentIds cannot be reliably observed by MCP;
   the lead would have to report them manually, omission is the default.
2. Lead-discipline recording into "session memory" — rejected: the term
   risks leads reaching for harness-native memory features, producing
   nondeterministic storage locations/formats.
3. **Chosen: tip-only.** `playbook.render`/`print` tool results append a
   short reminder, e.g. `tip: after spawning, the harness returns an agent
   id — reuse it for continuation (claude: SendMessage(to: <agentId>))
   instead of respawning.` Injection point is the moment of action (extreme
   application of the needle-in-haystack argument the user raised against
   load-once manuals).

Supporting facts: the agentId already lives in the lead transcript at spawn
time (the Agent tool result prints it), so within a live context window no
extra storage is needed. Compaction is itself LLM-performed; a live delegate
is the kind of fact summaries tend to preserve ("must not forget").

**Accepted semantic boundary (record as design, not accident):** retained
agent reuse is guaranteed only within the lead context lifetime. If compaction
drops the id, the fast path ends and the recovery path takes over — fresh
spawn + **resume brief** (a regenerated self-contained prompt carrying prior
results and next steps, the prompt-rendering successor of what ws state files
did). Retained agent = fast path / opportunistic optimization; fresh spawn +
resume brief = recovery path. A future "implementer continuity broke after
compaction" report is normal behavior, not a bug.

The same recovery path doubles as the degradation story for any harness
without durable subagents, keeping agenda premise 3 a soft dependency.

Delegation-fragment detail: the retain/spawn idiom text is injected only into
playbooks with delegation (`delegates: true` metadata), as a compact standard
fragment — not the full manual, not into every playbook.

## Decision: skill surface reduction

- Only directly-executed entry skills remain as thin shims (with good
  trigger descriptions); internal/orchestration skill bodies move to
  `playbook.print` content.
- Motivation includes reducing dependence on harness-native skills features
  (agenda 4.1): harnesses without a skills system can still drive workflows
  through MCP playbook calls.
- The exact entry-skill keep-list is an **open decision** (observable
  workflow change ⇒ ask-first per repo protocol).
- Side effect: `lead-skill-authoring`'s invariant-audit surface moves with
  the text — its target becomes rsrc playbook sources; the audit procedure
  must follow.

## Deferred Topics (explicitly excluded from first epic scope)

1. **memory.* / mutation tool contracts** (agenda 6). Requirements noted so
   far: layered scheme should map to existing layers (`_index.md` /
   `_index.local.md` / `_continue.local.md` / `.plans`); ticket mutations
   (create/status-move/focus) as MCP tools was already an open question in
   260429; avoid "memory" terminology collisions with harness-native memory
   features when instructing leads.
2. **wsflow naming/convergence** (agenda 5). Company-distributed; name and
   compat must be preserved. Convergence endpoint suggests namespace-rendered
   single source replacing the curated mirror.
3. **TUI shape** — dashboard deprecation target: lightweight process for
   browsing tickets/index/AI-generated content. Disposition of the 260514
   epic tree (~15 children, drop/salvage) is a destructive decision requiring
   explicit user approval.
4. **api.ask cache-write rules detail** — direction settled above; concrete
   conventions (file layout, frontmatter schema) to be specified in a child
   ticket.
5. **Actor/setup model redesign** for shared-server logical sessions.

## Continuation Decisions (2026-06-08)

Second lead-discuss session. Resolves/refines three items from the original
record; the original decision sections above are kept as the first-session
trail.

### Entry-skill keep-list (resolves the open question)

Actual inventory is 20 `SKILL.md` under `agents-plugin/skills` (the prior "21"
was off-by-one / counted the `reference-discovery` prompt-stem). Settled split:
**11 entry shims, 9 internal → `playbook.print` bodies.**

- Entry shims (user-exposed, thin shims with good trigger descriptions):
  `lead-discuss`, `lead-sprint`, `lead-proceed`, `lead-review`, `lead-ship`,
  `lead-salvage`, `lead-bootstrap`, `lead-skill-authoring`, `lead-add-rule`,
  `lead-forge-mental-model`, `lead-forge-spec`.
- Internal → `playbook.print` bodies: `lead-implement`, `lead-write-ticket`,
  `lead-write-spec`, `lead-workflow-manual`, `lead-check-blockers`,
  `lead-verify-design`, `lead-verify-discussion`, `lead-write-skeleton`,
  `lead-update-spec`.
- `lead-write-ticket` and `lead-write-spec` are dual-use today but are NOT
  user-invoked directly going forward (orchestration-only); their bodies move
  to playbook content invoked by caller skills. `forge-*` stay as entry
  (rare user-driven reconstruction). `lead-skill-authoring` stays entry, but
  its invariant-audit target moves to the rsrc playbook sources, so the audit
  procedure follows the text.
- Classification axis is "is the user meant to type `/ws:<name>` directly",
  not cross-skill invocation count (which mixes pure-internal references like
  `lead-workflow-manual` with dual-use writers).

### Implement entry routing (new decision)

`lead-implement` is NOT exposed as a user entry. `lead-proceed` stays the
single implementation entry point and gains a conservative ticket-skip gate
with two paths:

- **Explicit skip (deterministic)**: user requests a direct edit or explicitly
  says go without a ticket — judged from user words, no model guess.
- **Implicit skip (conservative, default to full routing)**: proceed auto-skips
  ticket routing only when an all-of threshold holds — relevant code already
  read into the session context, localized 1-2 file scope, and no
  cross-module/behavioral/spec impact. When in doubt, fall through to full
  routing.

The skip threshold **reuses the repo Approval Protocol categories** as the
decision key (Auto-proceed set = implicit-skip-eligible; Ask-first / Always-ask
= force full routing) — no new abstraction. Guard: if an implicit-skip edit
grows mid-implementation, retroactively suggest ticket creation (escape hatch,
not a forced step). Cold/compacted context naturally degrades the gate to full
routing, which is the correct behavior.

Rejected: exposing `lead-implement` as a second user entry — it reopens the
"code touched before spec/ticket/plan routing" hole that `lead-proceed` exists
to close, and forces a when-to-use-which judgment onto the user.

### Convention loading via playbook (refines api.ask / rsrc-loader direction)

Flag-based loading (e.g. `playbook.read(name, ["conventions"])`) is **rejected**
— it pushes the include decision back to the caller and is no better than
today's `convention.read` skip surface (the caller must still remember the
flag). Chosen instead:

- **Playbook frontmatter declares its own text dependencies** (e.g.
  `includes: [ticket-conventions]`); the rsrc loader **auto-includes** them at
  print time. The caller makes the one `playbook.print(name)` call it was going
  to make anyway.
- The real benefit is **atomicity** (the procedure cannot be obtained without
  its conventions), not round-trip reduction. The include decision is fixed at
  authoring time, not runtime, so context-gating is per-playbook and CI
  validates declared includes (extends the planned tree validator).
- `convention.read` / `infra.read` **survive as standalone discovery tools**
  for raw access (e.g. `lead-skill-authoring` audits, ad-hoc lead inspection).
  They serve a different entry point and do not compete with execution-path
  auto-include.

This settles the design for the open question "whether `infra.read`/
`convention.read` unify onto the rsrc loader"; the unify mechanism is
auto-include with the read tools retained for discovery. First-pass-vs-later
timing remains an implementation-sequencing call.

### Verification status update (from this session)

- Codex `rsrc/` materialization and parallel fan-out were confirmed verified by
  the user; these drop from the open verification list. Reconnect UX after
  binary swap is the remaining Codex item.
- Observability loss (`agents.tail/status/debug`, dashboard agent-activity
  sources) is accepted; harness-native subagent visibility is the replacement.

## Open Questions (continuation agenda)

- ~~Entry-skill keep-list~~ — resolved above (11 entry / 9 playbook).
- memory./mutation tool first slice: which operations, what layering, where
  delegation notes (if any) live.
- Playbook schema: frontmatter fields (`kind: print|render`,
  `delegates: bool`, `includes: [<text-dep>]`, params, overlays), directory
  layout, manifest format.
- Codex: reconnect UX after binary swap (rsrc materialization and parallel
  fan-out now verified).
- Migration sequencing detail within the epic: agentless-default dogfood →
  skill-text agents.* reference removal → runtime code deletion (order agreed
  in principle).
- Disposition mechanics: 260429 absorption into the epic; dashboard tree
  drop/salvage pass; 260521 retirement.
- `infra.read`/`convention.read` rsrc-loader unify: design settled
  (auto-include + read tools retained); first-pass-vs-later timing open.

## Survey Provenance (session evidence trail)

Four parallel subagent surveys grounded this discussion: (1) named-agent
runtime internals (spawn path, SQLite/wsstate, runner backends, bug history),
(2) skills inventory + prompt bundle + prompt.render + model alias/harness
detection, (3) MCP vs file-convention split for tickets/specs/mental-models
and memory layers, (4) 260429 direction, wsflow mirroring rules, per-harness
named-agent contracts. Claude durability was verified empirically in-session;
Codex retained-subagent capability was verified by the user out-of-band.
