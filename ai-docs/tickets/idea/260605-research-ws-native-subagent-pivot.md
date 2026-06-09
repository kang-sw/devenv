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

Re-confirmed 2026-06-09 (independent run, Claude Code 2.1.168): spawned a probe
holding `crimson-otter-lantern-seventeen-velvet` (first reply withheld it,
confirming no echo), then `SendMessage(to: <agentId>)` to the completed agent
("had no active task; resumed from transcript"); the resumed turn returned the
passphrase verbatim. The load-bearing durability premise holds across versions.

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

## Continuation Decisions (2026-06-09)

Third lead-discuss session. The actor/setup disposition is settled with
empirical grounding; several earlier decisions are superseded or refined. The
2026-06-08 section above is kept as the prior trail.

### Empirical basis: native subagents SHARE the lead's MCP process

In-session worktree probe (2026-06-09): a native Claude subagent spawned with
worktree isolation was inspected. Its no-arg `ws_setup` returned the LEAD's
actor (`actor_id: lead-kkp0lze6`, `has_actor: true`); its root-omitted
`git_status` reflected the MAIN repo (`main`, `22c16db`), NOT its worktree
branch; `server_root`/`env_project_root` were the main repo. Conclusion:
**native subagents share the lead's single ws-mcp process and state over one
multiplexed stdio connection.** This verifies the earlier "share the lead's MCP
server instance" premise, which had been asserted from tool-visibility alone.

Consequences:

- The single process-global `sessionRoot` field (server.go:38) clobbers across
  callers; the server cannot distinguish callers ambiently (Claude cwd
  auto-derivation is verified-failing).
- A worktree delegate doing root-omitted ws calls **silently operates on the
  lead's main repo (no error)** — a wrong-tree footgun the server cannot detect.
- The current `childActorInstruction` recovery (`setup(id:)`) is itself broken
  under a shared process: it would clobber the lead's `sessionRoot`. This
  reinforces removal, not redesign.

### Decision: actor/setup → ephemeral session auth model

Replace the persistent actor / wsstore-actor / authority / child-actor
machinery with an **ephemeral in-memory session (auth model)**:

- A login-style call (term open: `login` | `session.open` | `attach`) takes a
  root and returns an **LLM-friendly word-chain session key** (not a UUID —
  word chains copy more reliably and cost fewer tokens).
- The single `sessionRoot` field becomes a `{session-key → root context}`
  **map**, so concurrent distinct worktree roots are supported and there is no
  clobber — each caller logs in and gets its own key.
- **Mandatory session key on every ws call** (chosen over a keyless-lead-default
  alternative), analogous to REST bearer auth. There is no keyless fallback to a
  foreign root, which is exactly what kills the silent wrong-root footgun.
  Playbooks generate the call patterns with the session slot, so the marginal
  burden is ~0.
- **In-memory, not SQLite/wsstore** → sidesteps `260524-bug-wsstore-ci-sqlite-busy`;
  recovery is simply re-login (no persistent actor records to restore).
- Removed: actor_id-as-identity, the authority field, `ensureChildActor` /
  `childActorInstruction`, `restoreActor` / `bindActor` persistence, wsstore
  actor records.

Rejected: keyless-lead-default + keyed-delegates — leaves the footgun for any
delegate that drops its key; the server cannot enforce keying without caller
identity, so only mandatory keys close the hole.

**Concurrency & lifecycle (verified 2026-06-09, `ws:lead-verify-discussion`):**

- The session map MUST be concurrency-safe (`sync.RWMutex` / `sync.Map`).
  Non-setup requests are handled in parallel goroutines (`ServeStdio` spawns
  `go func()` per request, server.go:156); the current single `sessionRoot`
  field (server.go:38) is mutated under a synchronous **setup-fence**
  (`isSetupFenceRequest` drains in-flight work via `wg.Wait()` before running
  setup, server.go:142-153).
- Adopting per-call keys lets the **setup-fence be removed**: with no shared
  mutable root field, concurrent distinct-worktree calls each resolve their own
  root from the map with no serialization. This is a net hygiene reduction
  (wsstore actor persistence + fence + single field → one guarded map).
- **Resolved — session lifecycle (2026-06-09): in-memory, no logout, no
  eviction.** `login` is a bootstrap verb only; there is no `logout`. Session
  rows are tiny ((word-chain key, root path)) and bounded in practice by the
  number of distinct roots a fleet touches, so unbounded growth is a
  non-problem — no eviction needed. The map stays `sync.Map`-backed in-memory.
- **Forward-compat guard — `unknown_session` → re-login contract.** Every keyed
  ws call specifies: on an `unknown_session` rejection the caller re-logins
  (with its known root) and retries. This is a general key-rejection recovery
  path, not a restart-only path. Because the caller-visible contract
  (`login(root) → key`; `<tool>(key, …)`; re-login-on-reject) hides the backend,
  switching the map from in-memory to a persistent backend (SQLite or a
  key-file folder) later is a pure implementation swap with **zero contract
  migration**; it only changes how often the re-login branch fires (every
  restart → ~never), never whether it exists. Persistence is therefore deferred
  until session-wise state grows heavy enough to justify it.
- Re-login always has its root available, consistent with the no-auto-derive
  rule: the lead knows its own root; a subagent's delegation brief carries it.
- Rejected for now: SQLite-backed sessions (reopens
  `260524-bug-wsstore-ci-sqlite-busy`'s surface for a marginal "survive restart"
  gain that the re-login branch already covers cheaply) and a key-file folder
  (persistence-by-another-name: forces a global location via a chicken-and-egg —
  the folder cannot live under the session root it is meant to resolve — and
  tends to grow an in-memory read cache anyway). Both stay available as the
  later backend-swap target under the same contract.

### Decision: root role unchanged; cwd separated

- **root** stays the project/repo anchor + ws bookkeeping locus (git target,
  ai-docs discovery, exec cache/output anchor); it is carried **by the session**.
  ws ignores caller cwd entirely for root resolution (probe-proven).
- **cwd** is a separate per-call execution directory. **Confirmed current
  behavior** (server.go:3251): `execLaunchSchema` already exposes `working_dir`
  ("defaults to the resolved ws worktree root; relative paths resolve beneath
  that root"). So exec already separates working_dir from root; this is not new
  design, only formalized.

**Root-vs-cwd classification table (resolved 2026-06-09).** The earlier
"worktree git-vs-cache contradiction" dissolves: under per-session roots a
worktree is its own session (re-login), so git and exec cache both anchor at
that session's root — there is no split to reconcile.

- **root-bound, no cwd input:** git.* (status/diff/log/merge_base/commit),
  discovery (tickets.*/specs.*/mental_models.*/references.trace),
  convention.read, project_tree, path.generate, config.*. These must NOT take a
  cwd argument (a cwd input here would reintroduce a wrong-tree footgun).
- **global / session-agnostic:** runtime.info, infra.read.
- **cwd-consuming — exec launch only:** `exec.spawn` / `exec.shell` read
  `working_dir` at launch; cwd is captured into the job. Default = session root;
  explicit `working_dir` is free (chosen option: outside-root allowed, no
  containment — reads are key-scoped, so there is no path footgun to contain).
- **exec_key-scoped, dir-agnostic:** `exec.raw_read` / `raw_grep` / `raw_tail` /
  `result` / `status` / `abort` take only `exec_key` (`execKeySchema`,
  server.go:3267) — they read the captured streams of one job, NOT files by
  path. The feared "raw_read reads outside-root files via cwd" footgun does not
  exist.

Net: **the only cwd consumer is exec launch; everything else is root-bound; exec
output is tracked by `exec_key`**, so an outside-root working_dir does not break
output/cache traceability. Worktrees remain first-class distinct roots
(`canonicalGitRoot` resolves each worktree to its own toplevel) and re-login per
worktree.

### Decision: exec → stateless capability

`exec.spawn` returns an `exec_id` capability token; output/cache anchored at the
session root; cwd per spawn. No actor needed (confirms earlier reasoning that
exec keys by job token, not caller).

### Decision: playbook schema is fully custom

The playbook schema is **not bound to any agent / MCP-prompt standard** — ws is
the sole reader and renderer. Frontmatter fields (including the auto-include
`includes:`), directory layout, and manifest format are an autonomous design
detail.

### Supersede: dashboard retained, not deprecated to TUI

The 2026-06-05 decision "Dashboard is deprecated, downgraded to a lightweight
TUI" is **superseded**. The dashboard is **retained as a usable
web-tmux-style surface**; only the subagent-audit / agent-activity logic
(sourced from `agents.tail/status/debug`, removed with spawn) is ripped out. ws
MCP integration surfaces (ticket board, index, file/terminal) are **kept and
intended to grow**.

### Decision: role-containment retained — capability-scoped session keys

`WS_MCP_TOOL_PROFILE` role gating (lead/delegate/leaf tool restrictions) was
spawn-containment. It is **retained**, not deprecated. Rationale: it composes
with the session-key model into a useful long-term capability — the lead can
`login` and mint a **capability-scoped key** (e.g. commit-disabled) to hand a
delegate, so a session key carries `{root + capability/role scope}`, not just a
root. This is defense-in-depth layered on top of the harness's own subagent tool
restriction.

Honest scope: this is a **soft guard**, not a hard security boundary — a
delegate can always issue a fresh `login` to re-escalate. That is acceptable and
consistent with the existing layered prompt-based soft guards (the `setup`/root
discipline class). The value is friction and intent-signalling, not enforcement.

Forward note: the session-key issuance API should therefore reserve an optional
capability/role-scope parameter from the first cut, even if the first
implementation only honours a single default profile.

### Refine: spawn core frozen-preserved, not source-deleted (option C)

> **Superseded for the retained core by option B below (2026-06-09).** The
> dominant codex mercenary path is retained live, not frozen. The freeze framing
> survives only for genuinely-retired parts (gemini runner, subquery, exploration
> spawn, diagnostic sprawl). Kept as the decision trail.

Fourth lead-discuss session (2026-06-09). The "total spawn removal" decision is
refined on its disposition of the spawn **source**, not its disposition of the
**live path**.

Reconsidered question: is it cheaper to keep ws.agent but hide it from the
public schema rather than delete it? Findings:

- "Hide from public schema" already exists and is dogfooded: `WS_MCP_NO_AGENT=1`
  → `noAgentHiddenTool()` (`mcp/server.go:3118`) already filters `agents.*`,
  `exec.*`, `subquery`, `api.ask*`, `config.agents_tier` from the schema. The
  hiding cost is ≈ 0 — a default-flip, not new work.
- The expensive work (skill-text conversion to native delegation, M2) is paid
  either way; hiding the runtime does not save it.
- The real tax of keeping the spawn code **compiled-but-dormant** is
  entanglement: `agent.go`'s `SelfWorkerStarter`/`childActor` paths are welded
  to the actor/setup machinery this pivot deletes, and `childActorInstruction`
  recovery is already broken under shared-process native subagents. Keeping it
  compilable means either reviving the broken actor model or rewiring dormant
  code onto session-auth — integration labor with no deletion payoff.
- The token motivation does not favor ws-spawn: a spawned subprocess reboots a
  full model session (system prompt + full tool schema reload → more tokens),
  while the context-isolation win is delivered equally by native subagents
  (which inherit the full ws toolset and return only results). The sole genuine
  residual value is harness-independence (a harness with no subagent feature at
  all), which the tip-only / fresh-spawn + resume-brief recovery path already
  covers as a soft dependency.

Outcome — **option C, chosen over both total source-deletion and
compiled-but-dormant retention**: remove the spawn surface from the live server
and delete the actor entanglement, but **freeze-preserve the runner backends
(claude.go/codex.go/gemini.go) and the spawn core out of the compiled server**
(build-tag isolation or `ai-docs/ref/`), carrying zero compile/compat tax. The
capability is resurrectable if a no-native-subagent harness ever becomes a real
target. Consistent with repo Architecture rule #5 (preserve historical material
under `ai-docs/ref/`).

Unchanged by this refine: the live `agents.*`/`subquery` schema removal, the
actor → session-auth replacement, and the resolved-by-deletion outcome for the
spawn bug backlog (the buggy code no longer executes on any live path; the
frozen copy carries the known issues only if resurrected, not before).

### Supersede: option B — mercenary retained first-class, scoped to implementer/reviewer

Fifth lead-discuss exchange (2026-06-09). Use-case data plus a verified
limitation reshape option C into option B. Trail kept (total deletion → freeze →
retain-reshape); option B is the current decision for the retained core.

Drivers:

- ws-spawn **codex** calls are the dominant real use case and the code is
  battle-tested; deep-freezing the dominant path is wasteful — option C's
  "archival insurance" framing under-values it.
- The assumed recursion barrier is gone: the `WS_MCP_TOOL_PROFILE` env profile
  is **verified non-functional** for capability containment (recorded in the
  `named-agent-runtime` mental model, 2026-06). Capability-scoped session keys
  are soft (re-loginable). Neither layer hard-prevents recursive spawning.

Decision — **option B: retain the spawn engine as a first-class "mercenary"
surface** (not frozen-archival), but scoped:

- **mercenary = ws-spawned external subprocess agent**, a deliberately distinct
  term from harness-native "subagent" (resolves the LLM semantic collision). The
  engine = the runner backends + the reshaped lifecycle.
- **Scope restriction: mercenary is for implementer and reviewer roles ONLY.**
  Exploration/survey (reference-discovery, plan-populator), mental-model-update,
  and `subquery`'s successors route to **native subagents** — the pivot
  direction is retained for those (subquery→Explore absorption stands).
- **Routing**: a mercenary is selected when (a) the user explicitly requests a
  mercenary call, or (b) config enables it and `playbook.render` advises the
  mercenary spawn idiom inside the rendered implementer/reviewer prompt. Default
  without that signal is native.
- **codex runner stays live** (primary mercenary engine); claude/gemini
  disposition is refined in the ws.lead/child-key section below (claude
  **retained**, gemini → **deferred plug**, not a deletion).
- `subquery.go` spawn path is removed (exploration → native).

What option B changes vs option C / total-deletion:

- The codex (and possibly claude) runner backends + the reshaped `agents.*`
  call/lifecycle core stay **compiled and live** behind the mercenary surface —
  NOT frozen out of the server. Freeze/removal now applies only to genuinely
  retired parts (gemini runner, subquery spawn, exploration spawn paths, and the
  diagnostic sprawl beyond what mercenary needs).
- The actor → ephemeral session-auth replacement is **unchanged** (M3 holds);
  the mercenary spawn path is rewired onto session keys (see wiring below).

Call/interface parity (mercenary aligned to native, not divergent):

- **Drop the `agents.register(prompts: [stems])` schema.** Both native and
  mercenary are invoked with a single self-contained prompt produced by
  `playbook.render(name, context)`; the dispatch target is orthogonal to prompt
  production. Stem assembly at registration is removed.
- Output parity: a mercenary returns a continuation handle of the same shape as
  a native agentId; the tip-only continuity fragment applies to both.
- Net: the retained mercenary interface is SMALLER than today's
  register-with-stems surface.

Wiring onto session-auth (both feasible; moderate-rewire, sonnet-Explore
verified — actorID is a lookup coordinate, not an auth token; runner backends
are fully decoupled from actor identity):

- **native subagent ↔ parent-login**: the lead (same MCP process) mints a key
  (optionally capability-scoped) and passes it in the delegation brief; worktree
  native subagents may self-login with their own root.
- **mercenary ↔ pre-allocate + splice**: the spawn path pre-allocates a session
  key and splices the login instruction into the spawned system prompt — the
  existing `ensureAgentChildSetup` mechanism (`agent.go:1243-1265`) with the
  token swapped from actor-id to session key.

Recursion containment (open, but largely defused by scope):

> **Resolved below (ws.lead/child-key section):** the keyed-call-handler role
> check rejects non-lead `ws.lead.*` calls, so children cannot spawn → depth
> strictly 1; the CLI-flag depth-token backstop becomes unnecessary
> (defense-in-depth only).

- Because mercenaries are spawned **only by the (native) lead** and are scoped to
  leaf implementer/reviewer roles, the workflow naturally bounds spawning to
  depth 1 (lead → mercenary; mercenaries do not spawn). Recursion becomes a
  workflow-design property, not a hard requirement.
- A server-side **enforced spawn-depth (or capability) backstop** is still wanted
  because no hard barrier prevents a mercenary from calling the spawn tools (env
  profile dead, key scope soft). Mechanism detail (depth token propagated via CLI
  flag — CLI propagation works where env does not — tracked server-side per
  key-chain to resist forging) is deferred to mercenary-revival design.

ws↔wsflow convergence is PARTIAL under option B:

- mercenary is a **ws-only capability** wsflow (agentless, company-distributed)
  does not carry. The shared playbook/text core still converges to a single
  source via namespace rendering, but the mercenary/spawn surface is a ws-only
  divergence. The earlier "mirroring burden mostly disappears" claim is
  qualified: the agent/spawn axis stays a real ws-vs-wsflow difference (a ws-only
  section excluded from wsflow rendering, or separately maintained on that axis).

M3 scope implication: `260609-refactor-ws-spawn-runtime-deletion-session-auth`
is no longer pure deletion — it becomes spawn-runtime **reshape to mercenary** +
session-auth for the retained codex/claude path, with deletion confined to
gemini / subquery / exploration-spawn / diagnostic-sprawl. Re-scope at ready
promotion.

### Refine: ws.lead namespace, render-minted child keys, containment at the keyed call handler

Sixth lead-discuss exchange (2026-06-09). Refines option B's open details
(claude retention, child-key acquisition, recursion backstop) and corrects the
containment mechanism.

**Harness-neutral mercenary backend; claude retained, gemini deferred-plug.**

- harness-neutrality is a kept goal: gemini (now antigravity) and likely a
  future custom harness will attach via the mercenary **runner-backend
  interface**, so that abstraction stays harness-neutral/pluggable.
- **claude mercenary is RETAINED** (closes the option-B OPEN) — harness-neutrality
  argues for keeping it alongside codex.
- **gemini is a deferred plug, not a deletion.** Keep the neutral backend
  interface; remove only the `gemini.go` implementation (its real cost is
  model-compat tracking, not the abstraction). gemini/antigravity re-attach later
  through the same interface. Distinguish the two costs: the harness-neutral
  abstraction (one layer, ~free) is kept; only the unmaintained implementation is
  unshipped. "gemini dropped" is re-read as "gemini implementation unshipped now;
  plug point preserved."

**Child-key acquisition = render-minted, with a `root_override` param (option (c)).**

- Rejected: (a) lead key passed verbatim (no role distinction) and (b) an
  explicit `lead.issue` step (lead handles raw keys, extra round-trip).
- `playbook.render` is the mint+inject point: it already injects the delegation
  brief and knows the playbook is a delegate. Final signature:
  `playbook.render(session_key, name, context?, root_override?)`.
  - `session_key`: required (render is a ws call → keyed like all others); gives
    the default root for root-scoped **auto-includes** (e.g. local ai-docs) and
    the **role** for the mint gate.
  - `root_override`: when the child will run in a different worktree, overrides
    BOTH the auto-include resolution root AND the minted child-key binding root.
    render does **not** infer "lead-in-worktree vs spawn-into-worktree" — the
    caller that decided the worktree passes its path. Timing holds: the mercenary
    path pre-allocates the worktree before prompt splice, so the path is known at
    render time (native worktree likewise decided by the lead).
  - `role` is derived from the playbook frontmatter (playbooks are role-specific),
    so `root_override` is the only genuinely new param.
  - **Mint branch** (mint + inject a fresh child key) fires only when
    `session_key.role == lead` (server-side check). For non-lead keys render still
    returns text (auto-includes for the caller's own root) but mints nothing.
- Native and mercenary both receive "a rendered prompt with the child key already
  spliced in" → call parity is automatic.

**ws.lead namespace.** `login` and the mercenary spawn/lifecycle move under a
lead-centric `ws.lead.*` namespace (`ws/lead.login(root)`,
`ws/lead.<mercenary spawn>`). This resolves the session-term question:
**`lead.login`** (drop `session.open`/`attach`). `playbook.print`/`playbook.render`
stay under `playbook.*` (M1 surface); only render's mint branch is lead-gated by a
capability check, not by namespace.

**Containment correction — schema filtering is soft; enforcement is the keyed
call handler.** Hiding `ws.lead.*` from `tools/list` is a **harness-owned
soft-guard** (LLM-confusion reduction, the same role `noAgentHiddenTool` plays) —
a caller that knows the tool name can still issue `tools/call`. The hard guard is
a **server-side role check in the keyed `tools/call` handler** that rejects
`ws.lead.*` calls from non-lead keys. This converges all containment onto the
keyed-call handler (consistent with "containment lives on the session key,
server-side"). Same class of correction as the `WS_MCP_TOOL_PROFILE` env-barrier
finding.

**Recursion backstop resolved.** Because the keyed handler rejects non-lead
`ws.lead.*` calls, a child (native or mercenary) cannot login or spawn → spawn
depth is **strictly 1** (lead → mercenary leaf). The earlier deferred spawn-depth
counter (depth token via CLI flag) is therefore **unnecessary**; demote to
optional defense-in-depth. Raising depth > 1 later = an explicit lead-key grant to
a mercenary, never accidental.

**Durable lesson (capture to `named-agent-runtime`):** MCP `tools/list` (schema)
filtering is harness-owned and advisory; capability/role enforcement must be a
server-side check in the keyed `tools/call` handler, not schema omission.

**Still OPEN:** child `unknown_session` recovery message — role-split
("lead → re-login / child → ask issuer to re-render") vs a generic
"re-acquire via your issuer" line.

## Open Questions (continuation agenda)

- ~~Entry-skill keep-list~~ — resolved (11 entry / 9 playbook).
- ~~actor/setup model~~ — resolved as the ephemeral mandatory-session-key auth
  model (2026-06-09 section).
- ~~Per-tool root-vs-cwd classification table~~ — resolved (2026-06-09 section):
  exec launch is the only cwd consumer; all else root-bound; raw_* are
  exec_key-scoped.
- ~~Session lifecycle/eviction rule~~ — resolved (2026-06-09): in-memory, no
  logout, no eviction (rows tiny + bounded); `unknown_session → re-login`
  guardrail keeps a later persistent backend a contract-invariant swap.
- ~~Session term choice~~ — resolved (2026-06-09): **`ws/lead.login`** under a
  lead-centric `ws.lead.*` namespace; `session.open`/`attach` dropped.
- ~~Recursion spawn-depth backstop mechanism~~ — resolved (2026-06-09): the keyed
  `tools/call` handler rejects non-lead `ws.lead.*` calls → depth strictly 1; the
  CLI-flag depth-token counter is unnecessary (defense-in-depth only).
- ~~claude mercenary retention~~ — resolved (2026-06-09): **retained**
  (harness-neutrality); gemini → deferred plug (neutral backend interface kept,
  `gemini.go` implementation unshipped).
- **child `unknown_session` recovery message**: role-split (lead → re-login /
  child → ask issuer to re-render) vs a generic "re-acquire via your issuer" line.
- ~~Role-containment (`WS_MCP_TOOL_PROFILE`) deprecation~~ — resolved
  (2026-06-09): retained and folded into the session key as an optional
  capability/role scope (soft guard). Session-key issuance reserves a
  capability-scope parameter from the first cut.
- memory./mutation tool first slice: which operations, what layering, where
  delegation notes (if any) live.
- Playbook schema is fully custom (2026-06-09); remaining detail: concrete
  frontmatter fields (`kind: print|render`, `delegates: bool`,
  `includes: [<text-dep>]`, params, overlays), directory layout, manifest format.
- Codex: reconnect UX after binary swap (rsrc materialization and parallel
  fan-out now verified).
- Migration sequencing detail within the epic: agentless-default dogfood →
  skill-text agents.* reference removal → runtime code deletion (order agreed
  in principle).
- Disposition mechanics: 260429 absorption into the epic; dashboard retained
  (strip agent-audit only, 2026-06-09); 260521 retirement.
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
