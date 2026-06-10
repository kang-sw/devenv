---
title: ws spawn-runtime reshape to mercenary and ephemeral session-auth model
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: direction, option B mercenary reshape, session-auth model, root-vs-cwd, role-containment decisions
  260609-refactor-ws-skill-text-playbook-conversion: prerequisite — skill text must stop referencing the spawn surface first
  260517-bug-ws-agent-empty-result-after-tool-use: NOT auto-resolved under option B — lives in the retained mercenary path, must be fixed/re-triaged
  260524-bug-ws-agent-register-stale-dir-result-hang: NOT auto-resolved under option B — re-triage against the reshaped path (may be obsoleted by dropping register-with-stems)
  260524-bug-wsstore-ci-sqlite-busy: resolved-by-deletion (in-memory session map replaces wsstore actor records)
  260524-bug-subquery-non-head-history-evidence: resolved-by-deletion (subquery runtime removed)
  260524-bug-subquery-working-directory-stderr: resolved-by-deletion (subquery runtime removed)
  260525-bug-ws-setup-cwd-plugin-cache-root: design input — the new contract must not reproduce this footgun
spec:
  - 260610-ephemeral-session-auth-model
  - 260610-mercenary-delegation-surface
spec-remove:
  - 260508-agents-register-model-alias-field
  - 260523-agents-root-schema-invisibility
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
  - prompt-bundle
---

# ws spawn-runtime deletion and ephemeral session-auth model

## Background

Milestone M3 of the playbook-factory pivot (epic
`260605-epic-ws-playbook-factory-pivot`). With skill text no longer referencing
the spawn surface (M2), this slice **reshapes** the subprocess-spawn agent
machinery — retaining a scoped first-class "mercenary" surface (codex engine,
implementer/reviewer roles) per option B — and replaces the persistent
actor/authority model with an ephemeral in-memory session-auth model. It is
NOT a pure deletion (the earlier option-C freeze is superseded for the retained
core); deletion is confined to gemini / subquery / exploration-spawn /
diagnostic-sprawl.

This is the largest milestone and the one that realizes the bulk of the pivot's
simplification payoff (actor machinery removed; spawn surface scoped and
parity-aligned). Full direction, empirical grounding, and rejected alternatives
live in
`260605-research-ws-native-subagent-pivot` (see the 2026-06-09 "actor/setup →
ephemeral session auth model", "root role unchanged; cwd separated", "exec →
stateless capability", and "role-containment retained" sections, plus the
original "total spawn removal" inventory).

## Decisions

Binding decisions from the research ticket and the epic Cross-Child Decisions:

### Spawn-runtime reshape to mercenary (option B) — partial retain, not deletion

Disposition set by the research ticket's 2026-06-09 "option B — mercenary
retained first-class" decision, which supersedes the earlier option-C freeze for
the retained core. This milestone is no longer pure deletion: the dominant,
battle-tested codex spawn path is **reshaped into a first-class "mercenary"
surface** and kept live; deletion is confined to genuinely-retired parts.

- **mercenary = ws-spawned external subprocess agent**, a deliberately distinct
  term from harness-native "subagent" (resolves the LLM semantic collision).
- **Retain live (reshaped onto session-auth):** the **codex** and **claude**
  runner backends and the reshaped `agents.*` call/lifecycle core. Claude
  mercenary is **retained** (harness-neutrality, resolved 2026-06-09). The
  runner-backend interface stays **harness-neutral/pluggable** so gemini
  (antigravity) and a future custom harness can re-attach.
- **Scope restriction — mercenary is for implementer and reviewer roles ONLY.**
  Exploration/survey (reference-discovery, plan-populator), mental-model-update,
  and `subquery` successors route to native subagents.
- **Routing (finalized 2026-06-09):** default delegation is **always native**;
  the mercenary is **always available to the lead** (not a feature flag), invoked
  only when (a) the user explicitly requests it, or (b)
  `ws/lead.prefer_mercenary(session_key)` (lead-only, `ws.lead.*`) has flipped that
  key's render mode so `playbook.render` advises the mercenary spawn idiom as the
  primary delegation guidance for implementer/reviewer. The flip changes only the
  *default guidance*, never availability. Independently, a small **always-on tip
  fragment** (`tip: if the user requests a mercenary call, …`) is injected into
  every delegation-capable rendering so the on-request path is reachable without
  the toggle (token noise accepted).
- **Delete (genuinely retired):** the `gemini.go` runner **implementation**
  (unmaintained; model-compat tracking cost) — but **keep the harness-neutral
  runner-backend interface** so gemini/antigravity/custom harnesses are a deferred
  plug, not a structural exclusion; the `subquery` tool runtime (exploration →
  native); the exploration-purpose spawn paths; and the diagnostic sprawl beyond
  what mercenary needs (`agents.tail/status/debug` minimized to mercenary needs).
- **Delete the actor/authority/child-actor entanglement** (unchanged from the
  session-auth decision); the retained mercenary spawn path is rewired onto
  session keys via pre-allocate + system-prompt splice (existing
  `ensureAgentChildSetup`, `agent.go:1243-1265`, token swapped actor-id → session
  key).
- **Interface parity with native (drop divergence):** remove the
  `agents.register(prompts: [stems])` schema; mercenary and native are both
  invoked with a single self-contained prompt from `playbook.render`. Mercenary
  returns a continuation handle of the same shape as a native agentId. Net: the
  retained mercenary interface is smaller than today's register-with-stems
  surface.
- **Child-key acquisition = render-minted (resolved 2026-06-09).**
  `playbook.render(session_key, name, context?, root_override?)` is the mint+inject
  point for both native and mercenary delegates: when `session_key.role == lead` it
  mints a fresh child key (role from the playbook frontmatter) and splices it into
  the rendered prompt, so both paths receive a prompt with the child key already
  embedded (automatic call parity). `root_override` overrides BOTH the
  auto-include resolution root and the child-key binding root when the child runs
  in a different worktree; render does not infer worktree shape (caller passes the
  path; the pre-allocate-before-splice order makes it known at render time).
  render is keyed like every ws call (gives the root for root-scoped auto-includes
  such as local ai-docs). This crosses into the M1 `playbook.render` surface —
  coordinate the keyed signature + `root_override` + lead-gated mint branch with
  M1.
- **`ws.lead` namespace + keyed-handler containment (resolved 2026-06-09):**
  `login` and the mercenary spawn/lifecycle live under a lead-centric `ws.lead.*`
  namespace (`ws/lead.login`, `ws/lead.<spawn>`). Containment is a **server-side
  role check in the keyed `tools/call` handler** that rejects `ws.lead.*` calls
  from non-lead keys — NOT schema/`tools/list` filtering, which is a harness-owned
  soft-guard (LLM-confusion reduction only; a caller knowing the name can still
  invoke the tool). All containment converges on the keyed-call handler.
- **Recursion containment (resolved 2026-06-09):** because the handler rejects
  non-lead `ws.lead.*` calls, a child (native or mercenary) cannot login or spawn
  → spawn depth is **strictly 1** (lead → mercenary leaf). The earlier deferred
  CLI-flag spawn-depth counter is **unnecessary** (optional defense-in-depth
  only). The `WS_MCP_TOOL_PROFILE` env profile is verified non-functional (see
  `named-agent-runtime` mental model); the keyed-handler check replaces it.
- Remove `api.ask` spawn machinery and the async job surface from the live
  server — coordinated with M4 (`api.ask` redesign owns the replacement
  contract).
- Casualties accepted: `agents.tail/status/debug` observability and the
  dashboard's agent-activity sources disappear (harness-native visibility
  replaces them); uniform cross-harness wait/result/cancel semantics are
  delegated to per-harness UX.

### Ephemeral session-auth model (replaces actor/wsstore/authority)

- Replace persistent actor / wsstore-actor / authority / child-actor machinery
  with an ephemeral in-memory session. A login-style call takes a `root` and
  returns an **LLM-friendly word-chain session key** (not a UUID).
- The single process-global `sessionRoot` field becomes a `{session-key → root
  context}` map, so concurrent distinct worktree roots are supported with no
  clobber.
- **Mandatory session key on every ws call** (REST-bearer style). No keyless
  fallback to a foreign root — this is what kills the silent wrong-tree footgun
  (a worktree delegate doing root-omitted calls silently operating on the lead's
  main repo). Rejected: keyless-lead-default + keyed-delegates (leaves the
  footgun for any delegate that drops its key).
- **In-memory, concurrency-safe map** (`sync.Map`/`sync.RWMutex`). It replaces
  the current setup-fence (`isSetupFenceRequest` / `wg.Wait()`): with no shared
  mutable root field, parallel-goroutine requests each resolve their own root
  with no serialization. Net hygiene reduction (wsstore persistence + fence +
  single field → one guarded map). Sidesteps `260524-bug-wsstore-ci-sqlite-busy`.
- **No logout, no eviction.** `login` is a bootstrap verb only; session rows are
  tiny `(word-chain key, root path)` and bounded by the number of distinct roots
  a fleet touches.
- **Forward-compat guard — `unknown_session` → re-login contract.** Every keyed
  call specifies: on an `unknown_session` rejection the caller re-logins (with its
  known root) and retries. Because the caller-visible contract (`login(root) →
  key`; `<tool>(key, …)`; re-login-on-reject) hides the backend, switching the map
  to a persistent backend later is a pure implementation swap with zero contract
  migration. Persistence is deferred until session-wise state grows heavy.
- Re-login always has its root available (the lead knows its own root; a
  subagent's delegation brief carries it), consistent with no-auto-derive.
- Removed: actor_id-as-identity, the authority field, `ensureChildActor` /
  `childActorInstruction`, `restoreActor` / `bindActor` persistence, wsstore
  actor records.
- **Session term resolved (2026-06-09):** **`ws/lead.login(root)`** under a
  lead-centric `ws.lead.*` namespace (only the lead logs in; subagents never
  login — they receive a key, see mercenary child-key acquisition). `session.open`
  / `attach` dropped.
- **Child `unknown_session` — known gap, not a designed path (2026-06-09).** It
  fires only when the in-memory map is lost (lead ws-mcp process restart while a
  delegation is live). A child cannot self-recover (never logged in; `ws.lead.*`
  gating denies login), so interim behavior is a **generic `unknown_session`
  reject** → the child escalates to its issuer (the lead), which re-renders a fresh
  key and re-delegates (existing fresh-spawn + resume-brief recovery). A
  persistent session backend later shrinks this to ~never; revisit the message
  then. Do not design a role-specific child recovery message now.

### Session-key generation (resolved 2026-06-10)

The word-chain key generator is a **reusable, generic utility in its own small
package**, kept deliberately separate from auth/capability *policy*: it produces
the string; callers decide what a key authorizes.

- **Format:** 4 words + a 2-digit numeric suffix (e.g. `amber-tide-fox-river-42`).
  The 2-digit suffix is a readability/tiebreak nicety, not load-bearing entropy.
- **Correctness via mint-time uniqueness, not pool size:** `mint` checks the
  in-memory session map and regenerates on the (astronomically rare) collision,
  so pool size is an ergonomics choice, not a correctness guarantee.
- **Word pool = EFF large diceware list (7776 words), vendored + `go:embed`.** The
  list is fetched ONCE at development time into the repo as a data asset and
  embedded at build time. There is NO runtime network fetch: login is the
  per-session bootstrap verb, so a network dependency on key minting would be an
  offline/CI/airgap reliability regression and a supply-chain risk. Rejected:
  runtime REST fetch of a word corpus (the "save LLM context" motive is moot — an
  on-disk/embedded list costs zero conversation context, only build-time bytes).
  Rejected: hand-enumerating a word pool token-by-token in source.
- **M3 wiring scope:** only `ws.lead.login` session keys use the generator in this
  milestone. Re-minting the other id-issuing surfaces (`exec_key`, `api_job_key`,
  `path.generate` stems, mercenary continuation handles) onto the shared generator
  is reserved for the follow-up todo ticket
  `260610-refactor-ws-wordchain-id-generalization`, so M3 stays scoped. Build the
  generator generic now; wire only session keys.

### root vs cwd; exec; role-containment

- **root** stays the project/repo anchor + ws bookkeeping locus, carried by the
  session. ws ignores caller cwd for root resolution. root-bound tools (git.*,
  discovery, convention.read, project_tree, path.generate, config.*) must NOT
  take a cwd argument.
- **cwd** is consumed only by exec launch (`exec.spawn`/`exec.shell` `working_dir`,
  already current behavior, server.go:3251). Outside-root `working_dir` is
  allowed (no containment) because exec reads are key-scoped. `exec.raw_*` /
  result / status / abort are `exec_key`-scoped, dir-agnostic — they read captured
  streams, not files by path. Worktrees are first-class distinct roots (re-login
  per worktree).
- **exec → stateless capability.** `exec.spawn` returns an `exec_key` capability
  token; output/cache anchored at the session root; cwd per spawn. No actor
  needed.
- **Role-containment retained — capability-scoped session keys.**
  `WS_MCP_TOOL_PROFILE` role gating is retained (not deprecated): a session key
  carries `{root + optional capability/role scope}`, so the lead can mint a
  capability-scoped key (e.g. commit-disabled) for a delegate. Soft guard (a
  delegate can re-`login` to re-escalate), defense-in-depth on top of the
  harness's own subagent tool restriction. **The key-issuance API reserves an
  optional capability/role-scope parameter from the first cut**, even if the first
  implementation honours only a single default profile.

### Resolved-by-deletion bug tickets

Under option B the disposition SPLITS — only bugs whose code is genuinely removed
are resolved-by-deletion:

- **Resolved-by-deletion (drop to `.dropped/` in the removing commits):**
  `260524-bug-wsstore-ci-sqlite-busy` (wsstore actor records gone → in-memory
  session map), `260524-bug-subquery-non-head-history-evidence` and
  `260524-bug-subquery-working-directory-stderr` (subquery runtime removed).
- **NOT auto-resolved — live in the retained mercenary (codex) path, must be
  FIXED or re-triaged:** `260517-bug-ws-agent-empty-result-after-tool-use` and
  `260524-bug-ws-agent-register-stale-dir-result-hang`. These were "resolved by
  deletion" only under total removal; with the codex spawn engine retained they
  remain live defects. Re-triage against the reshaped path (the register bug may
  be obsoleted by dropping the register-with-stems schema; the empty-result bug
  likely persists and needs a real fix).
- `260525-bug-ws-setup-cwd-plugin-cache-root`: close when the session-auth
  contract lands (retained as design input — the new contract must not reproduce
  the plugin-cache-root binding footgun).

### Dashboard — keep compiling, defer the feature decision (revised under option B)

The earlier "strip the dashboard agent-audit / agent-activity logic" plan assumed
spawn deletion removed the source. **Under option B the codex mercenary lifecycle
survives, so the agent-activity source survives.** Therefore:

- M3 **does not strip** the feature. It only keeps the dashboard **compiling**
  against the reshaped session/lifecycle surface — the actor-model removal and the
  `register(prompts: [stems])` schema change force a mechanical read adaptation
  (the dashboard reads wsstore actor/instance records) regardless.
- "Port the agent-activity feed onto the mercenary lifecycle vs remove it" is a
  **deferred product decision** → a separate dashboard `idea/` ticket
  (`re-evaluate — see epic 260605`), not decided here.
- The dashboard is otherwise **retained** as a web-tmux surface.

## Constraints

- Depends on M2: shipped skill text must already be free of `agents.*`/`subquery`
  references before deletion, or workflows break.
- Coordinated with M4: `api.ask` spawn/async machinery is removed here only in
  concert with M4's replacement contract; do not strand `api.*` callers.
- Large `mcp-tools.md` contract change — ask-first / always-ask per repo Approval
  Protocol (protocol/API semantics change). Ready promotion requires a
  contract-first spec.

## Phases

> Phase sketch for backlog recovery; refine slice boundaries at ready promotion.

### Phase 1: session-auth model in (additive)

Introduce the login-style call, the word-chain key, the concurrency-safe
`{key → root}` map, mandatory per-call key acceptance, the `unknown_session` →
re-login contract, and the reserved capability/role-scope parameter. Remove the
setup-fence once the shared mutable root field is gone. Land alongside the
existing actor model so callers can migrate. Verification: concurrent
distinct-root calls do not clobber; missing/unknown key yields the re-login
recovery contract; capability-scoped key restricts the intended tools.

### Result (447946f4) - 2026-06-10

Landed additively on `implement/ws-session-auth-phase1`; the actor/`sessionRoot`
model and the `ws.setup` fence are fully intact this phase. Commits: `a5370cd1`
(wskey generator package), `50e7d7d0` (mcp session-auth wiring), `447946f4`
(review-cycle-1 fixes).

Delivered:
- `internal/wskey` — policy-free word-chain key generator: `//go:embed` EFF large
  diceware list vendored as 7772 pure-`[a-z]+` words (4 hyphenated EFF entries
  dropped so the `-` separator is unambiguous; pool size is ergonomic per the
  Session-key generation decision, not load-bearing). `Generate()` = 4 words + a
  2-digit suffix via `crypto/rand`; `GenerateUnique(exists)` for non-atomic callers.
  No import of mcp/auth.
- `internal/mcp/session_auth.go` — concurrency-safe in-memory `sessionRegistry`
  `{session_key → {root, scope toolRole}}`; `mint` does atomic check-and-insert
  under its own write lock (deliberately not `wskey.GenerateUnique`, which checks
  the predicate outside the lock → TOCTOU). No SQLite, no logout, no eviction.
- `ws.lead.login(root, capability?)` MCP tool (literal `ws.lead.*` namespace,
  paralleling `ws.setup`); returns the word-chain `session_key` + canonical root;
  NOT a setup-fence request; uses `canonicalSetupRoot` (rejects `"<cwd>"`, 260525
  footgun guard). `capability` reserved (maps to `roleLead`/`roleDelegate`/`roleLeaf`;
  omitted ⇒ lead/unrestricted).
- `resolveToolRoot` gains a highest-priority `session_key` branch: known ⇒ key's
  root; unknown ⇒ `unknown_session` re-login error (caller-visible isError, names
  `ws.lead.login`); absent ⇒ existing chain unchanged (additive).
- Keyed capability gate in `callTool`: a known non-lead key is gated by
  `roleAllowsTool`, and any `ws.lead.*` tool is rejected for non-lead keys
  (self-login escalation block — implements the spec Constraints "a delegate cannot
  self-login or escalate"). Keyless callers unaffected.
- `ws.lead.login` added to both `agents-plugin/runtime.json` and
  `agents-plugin-wsflow/runtime.json` (visible in wsflow no-agent mode; not
  agent-backed).

Verification: `go build/vet ./...` clean; `go test ./...` green (mcp + wskey incl.
the 5 integration cases + wskey units); Python launcher capability unittest (16
tests, incl. exact-surface rejection) green. Partitioned review (correctness/fit/test)
clean after one fix cycle; one test re-review objection was a refuted false positive
(it missed the `strings.HasPrefix("ws.lead.")` short-circuit preceding `roleAllowsTool`
— `TestCapabilityScopedKeyGatesTools` passes).

Spec: planned 🚧 stems `260610-ephemeral-session-auth-model` /
`260610-mercenary-delegation-surface` intentionally NOT stripped (span all three
phases; Phase 1 is partial). Mental models updated additively (`4594f70e`):
mcp-runtime + named-agent-runtime record the session-auth layer and the keyed
`ws.lead.*` prefix gate as the first concrete server-side containment, coexisting
with the still-live actor model.

> Forward (Phase 2): migrate callers to session keys, then delete the actor/authority/
> child-actor machinery + `subquery` runtime + gemini runner, reshape `agents.*` to the
> mercenary surface, remove the `ws.setup` fence, and harden keyless hard-rejection
> (mandatory key). `260524-bug-wsstore-ci-sqlite-busy` etc. drop when that code is removed.
> Forward (Phase 3): exec stateless + full role-containment fold (capability-scoped keys
> replace `WS_MCP_TOOL_PROFILE`) + dashboard build-fix.

### Phase 2: mercenary reshape + actor model deletion (option B)

Delete the actor/authority/child-actor machinery, the **gemini** runner backend,
the `subquery` runtime, exploration-purpose spawn paths, and diagnostic sprawl
beyond mercenary needs. **Retain and reshape** the **codex** runner backend
(claude OPEN) plus the `agents.*` call/lifecycle core into the mercenary surface:
rewire the spawn path onto session keys (pre-allocate + system-prompt splice),
drop the `register(prompts: [stems])` schema for a single self-contained prompt,
align the continuation handle to the native agentId shape, and scope mercenary to
implementer/reviewer with the user-explicit / config-advised routing. Drop the
subquery and wsstore-sqlite-busy bug tickets here (their code is removed); the
agent-empty-result and register-stale-dir bugs are NOT auto-resolved — they live
in the retained mercenary path and must be FIXED or re-triaged (see below).
Verification: native is the default delegation path; mercenary spawns only for
implementer/reviewer via the routing gate; the actor model is gone; no gemini /
subquery / exploration spawn remains.

### Phase 3: exec stateless + role-containment fold + dashboard build-fix

Confirm exec is a stateless `exec_key` capability anchored at the session root;
fold role-containment into capability-scoped keys; keep the dashboard **compiling**
against the reshaped session/lifecycle surface (mechanical read adaptation, no
feature strip — the agent-activity port-vs-remove decision is deferred to a
separate dashboard ticket). Verification: exec works without any actor;
capability-scoped keys gate delegate tools; the dashboard builds and runs against
the reshaped surface.

Phase order: Phase 1 before Phase 2 (session-auth must replace the actor
dependency before the actor model is deleted).

## Spec Impact

Contract-first spec authored at ready promotion (commit `77a9322a`); the planned
contract now lives in `ai-docs/spec/mcp-tools.md` and is tracked through `spec:` /
`spec-remove:` frontmatter:

- `spec:` — `260610-ephemeral-session-auth-model` (the `ws.lead.login` session-key
  auth model) and `260610-mercenary-delegation-surface` (the reshaped `agents.*`
  family). Existing reshaped-but-retained stems
  (`#260524-mcp-actor-setup-bootstrap`, `#260505-mcp-session-default-root`,
  `#260505-tool-profile-gating`, `#260512-agent-cancel-resume-guidance`,
  `#260512-agent-recall-hidden-surface`) carry Planned 🚧 callouts pointing to the
  two new stems; their `🚧` strips when this milestone implements.
- `spec-remove:` — `260508-agents-register-model-alias-field` and
  `260523-agents-root-schema-invisibility`, retired by the single-self-contained-prompt
  and mandatory-session-key contracts respectively.

Closeout notes not owned by the spec stems:

- The skill-facing `subquery` contract has no standalone spec stem; its removal is
  a text edit within `#260505-workflow-state-delegation-tools` (Planned 🚧 callout
  already added), not a `spec-remove:` stem.
- `api.*` spawn/async-job removals are owned by M4
  (`260609-refactor-ws-api-ask-corpus-routing`) and are intentionally absent from
  this ticket's spec surgery.
