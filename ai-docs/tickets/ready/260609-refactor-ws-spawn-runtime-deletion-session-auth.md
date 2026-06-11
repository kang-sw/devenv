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

> Phase 2 was sliced into 2a/2b/2c at implementation time (sub-phase labels keep
> the stable Phase 3 number unrenumbered per ticket conventions). The original
> single-phase sketch ("mercenary reshape + actor model deletion") is preserved by
> the union of 2a+2b+2c; the binding decisions stay in `## Decisions` above. The
> three slices have distinct review/rollback boundaries and a hard dependency
> order (2a → 2b → 2c), which is why they are separate reviewable units.

### Phase 2a: caller migration to session keys + actor model deletion

The highest-risk core; lands first because every later deletion depends on the
actor dependency already being gone. Migrate the live callers (the still-additive
Phase 1 session-auth path becomes mandatory) onto session keys, then delete the
persistent actor/authority/child-actor machinery and the `ws.setup` setup-fence,
and harden the keyless path into a hard rejection (mandatory key, REST-bearer
style — no keyless fallback to a foreign root). See `## Decisions` → "Ephemeral
session-auth model" (mandatory key, no keyless fallback; in-memory map replaces
the setup-fence) and "root vs cwd" (root stays session-carried, no cwd arg on
root-bound tools).

Removed here: `actor_id`-as-identity, the authority field, `ensureChildActor` /
`childActorInstruction`, `restoreActor` / `bindActor` persistence, wsstore actor
records, `isSetupFenceRequest` / `wg.Wait()` fence. The retained mercenary spawn
path (reshaped in 2c) is rewired onto session keys via pre-allocate +
system-prompt splice; in 2a only the actor dependency is severed.

Boundary: does NOT delete gemini/subquery/exploration spawn (2b) and does NOT
reshape the codex `agents.*` surface (2c); those still compile against the
session-keyed core after 2a.

Verification: parallel distinct-root calls each resolve their own root with no
fence/serialization and no clobber; a keyless call is hard-rejected (not silently
defaulted to a foreign root); `unknown_session` still yields the re-login
recovery contract; no actor/authority/child-actor code remains.

### Result (9649a4bf) - 2026-06-11

Landed on `implement/ws-session-auth-phase2a` (from epic tip `c917c9f0` +
slice commit `29bbf19a`). The auth-model cutover is complete: the persistent
actor/authority/child-actor model, `ws.setup` (both forms), and the setup-fence
are deleted; `session_key` is mandatory for every root-aware tool; `ws.lead.login(root)`
is the sole bootstrap and the only `root` acceptor.

Delivered:
- `resolveToolRoot` collapsed to key-only: known key → root; absent →
  `mandatory_session_key` reject; unknown → `unknown_session` reject. All silent
  fallbacks removed (explicit `root` arg, volatile sessionRoot, host metadata,
  `WS_MCP_PROJECT_ROOT`, startup root) (`79fe8bfa`).
- `ws.setup` removed — schema, dispatch, alias, and the request-order fence
  (`isSetupFenceRequest`/`wg.Wait`); explicit calls now fall through as unknown
  tools (`24569308`). Server actor scope/binding/gate deleted (`1e6e1932`).
- `wsstore` actor records deleted and the named-agent registry re-keyed off
  `actorID` — `AgentInternalKey(publicName)` namespaced by the resolved worktree
  store; child-actor credential injection removed; idempotent transactional
  migration rebuilds existing DBs without the actor columns (`67d640ad`,
  `9649a4bf`).
- `root` parameter stripped from every root-aware tool schema; `ws.lead.login` is
  the only advertised root acceptor (`f2d2422f`).
- ws + wsflow skill bootstrap text migrated `ws.setup` → `ws.lead.login` /
  `wsflow/ws.lead.login` (`ec1d80fb`, `9649a4bf`); CLI `--actor-id` flags dropped
  (`6dd18196`).

User-locked decisions honored: (Q1) full key-only in 2a — `root` stripped from all
schemas and `260523-agents-root-schema-invisibility` removed; (Q2) `ws.setup`
removed entirely with skill-text migration in 2a.

Deviations / notes:
- The `plan-populator-survey` named agent hit the 200k context limit mapping this
  large surface ("Prompt is too long"); the lead authored the source-survey plan
  from targeted recon instead. Captured as a dogfood `idea/` ticket
  (`260611-bug-agent-context-exhaustion-opaque-failure`).
- A delegated test pass first over-deleted ~37 out-of-scope tests (git/api/exec/
  config/etc.) to force a green suite; caught in pre-review, corrected with a
  `serveStdioWithSession` harness that logs in and threads `session_key` and strips
  non-login `root` args so restored tests genuinely exercise the mandatory-key path.
- CLI `--root` flags retained (lead ruling, reviewer-accepted): CLI mirrors are a
  session-less local adapter; the mandatory-key model governs MCP tool calls, not
  CLI invocations. This corrected a brief over-specification.
- Review: partitioned correctness/fit/test, 2 cycles, all clean. Verification:
  `go build/vet/test ./... -count=1` green; wsflow contract test green.

Spec: `260610-ephemeral-session-auth-model` 🚧 stripped (implemented); removed
`260524-mcp-actor-setup-bootstrap` and `260523-agents-root-schema-invisibility`;
`260505-mcp-session-default-root` rewritten to key-only (`f3f50dcb`). Mental models
updated (`58b97bf0`). Planned markers kept for 2b/2c/Phase-3 (mercenary surface,
`260508` register reshape, tool-profile capability-scope enforcement).

> Forward (Phase 2b): the spawn path currently compiles WITHOUT child credentials
> (render-minted child keys are 2c). Delete the gemini runner impl, subquery
> runtime, exploration spawn, and diagnostic sprawl; drop the three
> resolved-by-deletion bug tickets.
> Forward (Phase 2c): agents.* mercenary reshape (drop register-with-stems,
> native-handle parity, routing gate, `prefer_mercenary`) + render-minted child
> keys via `playbook.render`; `260508` register-schema removal lands here.
> Forward (Phase 3): exec stateless + capability-scope ENFORCEMENT (the key already
> carries a reserved scope) + dashboard build-fix — the `actors` table and
> AgentDefinition actor columns are gone, but the dashboard was not yet adapted;
> exec-job `owner_actor_id` is left in place for the Phase 3 exec rework.

### Phase 2b: delete genuinely-retired spawn surfaces

Delete the parts that are retired outright, now that 2a severed the actor
dependency. See `## Decisions` → "Spawn-runtime reshape to mercenary" (Delete
list) and "Resolved-by-deletion bug tickets".

Delete: the **gemini** (`gemini.go`) runner **implementation** — but **keep the
harness-neutral runner-backend interface** so gemini/antigravity/custom harnesses
remain a deferred plug, not a structural exclusion; the `subquery` tool runtime
(exploration → native subagents); the exploration-purpose spawn paths; and the
diagnostic sprawl beyond mercenary needs (`agents.tail/status/debug` minimized to
what the retained mercenary lifecycle needs).

Drop resolved-by-deletion bug tickets to `.dropped/` in the removing commits (use
`git mv`): `260524-bug-wsstore-ci-sqlite-busy` (wsstore actor records gone — note
the in-memory map already landed in 2a), `260524-bug-subquery-non-head-history-evidence`,
and `260524-bug-subquery-working-directory-stderr` (subquery runtime removed).

Boundary: does NOT reshape the codex `agents.*` call/lifecycle surface (2c). The
codex runner stays live and callable through 2b; only retired backends/paths go.

Verification: no gemini / subquery / exploration spawn remains; the
runner-backend interface is still present and pluggable (codex still attaches
through it); the three dropped bug tickets are in `.dropped/`.

### Result (60015691) - 2026-06-11

Deleted the retired spawn surfaces on branch `implement/ws-session-auth-phase2b`
(stacked on unmerged Phase 2a; both pending a combined merge to the epic).

Delivered:
- Gemini runner implementation removed (`gemini.go`/`gemini_test.go`, the
  `runnerForBackend` gemini case, config harness alias/detection, the
  `ClaudeRunner` shorthand entry). The harness-neutral `Runner` interface +
  `RunnerRequest`/`RunnerResult` + Codex/Claude runners are unchanged; gemini is
  now a deferred plug (`runnerForBackend("gemini")` returns the
  unsupported-backend error). Guard: `TestRunnerForBackendGeminiIsUnsupported`.
- `subquery` tool runtime removed end-to-end: wsagent `Subquery`/
  `SubqueryOptions`/`SubquerySystemPrompt`, the MCP `subquery` tool (schema +
  dispatch + the `subqueryAgentAccessAllowed`/`isSubqueryAgentTool` gating), and
  the CLI `subquery` subcommand; `agents-plugin/runtime.json` contract updated.
  Guard: `TestSubqueryToolRemovedFromListAndCallRejected`.
- Exploration-purpose spawn was the subquery path (removed). The `ExploreAgent`
  playbook terminology vars were retained — they are native-subagent render
  idioms (the direction the epic moves toward), not the retired path.
- The 3 resolved-by-deletion bug tickets moved to `.dropped/` in the removing
  commits: `260524-bug-wsstore-ci-sqlite-busy`,
  `260524-bug-subquery-non-head-history-evidence`,
  `260524-bug-subquery-working-directory-stderr`.

Deviation from literal Phase 2b text: general `agents.status/tail/debug.*`
diagnostic minimization was DEFERRED to Phase 2c. "Minimize to what the retained
mercenary lifecycle needs" cannot be done correctly before 2c defines that
lifecycle; 2b removed only diagnostics/gating coupled to deleted paths to avoid
speculative delete-then-re-add churn. The general diagnostic surface (serving the
live Codex runner) is retained.

Verification: `go build/vet/test ./... -count=1` green; wsflow runtime contract
test green. No gemini/subquery/exploration spawn code remains; the runner-backend
interface is present and pluggable (codex attaches through it); the 3 bug tickets
are under `.dropped/`.

Review: partitioned (correctness/fit/test). Correctness + fit clean first pass;
test non-clean (vacuous deleted-tool assertions in profile-filter lists) → fixed
in `c5865894`, one finding partially rejected (kept the subquery-removal
regression guard) and reviewer-accepted on re-review.

Docs: spec reconciled (`4fe1a7e0` — subquery + gemini sections removed, Phase-2c
planned callout trimmed); mental models reconciled (`df0740a2` — dangling
`#260512-gemini-agent-runner` / `#260505-async-subquery-ephemeral-agent` anchors
cleaned); a doc-pre-pass-discovered shipped-guidance gap fixed (`bb2d3558` —
`lead-workflow-manual` no longer points at the removed `subquery` tool).

Forward (2c): codex `agents.*` reshape, `register(prompts:[stems])` drop,
native-handle parity, mercenary routing gate, render-minted child keys, and the
deferred general-diagnostic minimization. Phase 3: exec stateless + capability
enforcement + dashboard build-fix.

### Phase 2c: codex mercenary reshape + parity + routing gate

Reshape the retained **codex** runner backend plus the `agents.*` call/lifecycle
core into the first-class **mercenary** surface. See `## Decisions` → "Spawn-runtime
reshape to mercenary" (Retain/reshape, Routing, Child-key acquisition, parity) and
"`ws.lead` namespace + keyed-handler containment".

Reshape: rewire the spawn path onto session keys (pre-allocate + system-prompt
splice — `ensureAgentChildSetup`, token swapped actor-id → session key); drop the
`register(prompts: [stems])` schema for a single self-contained prompt from
`playbook.render`; align the mercenary continuation handle to the native agentId
shape (interface parity); scope mercenary to **implementer/reviewer roles only**
with the finalized routing — default delegation always native, mercenary always
available to the lead, primary-guidance flip via `ws/lead.prefer_mercenary(session_key)`
(lead-only, `ws.lead.*`) plus the always-on tip fragment so the on-request path is
reachable without the toggle. Child-key acquisition is render-minted
(`playbook.render` mints + splices the child key when `session_key.role == lead`);
coordinate the keyed `playbook.render` signature + `root_override` + lead-gated
mint branch with M1.

Bug re-triage (NOT auto-resolved — these live in the retained codex path):
`260517-bug-ws-agent-empty-result-after-tool-use` (likely persists; needs a real
fix on the reshaped path) and `260524-bug-ws-agent-register-stale-dir-result-hang`
(may be obsoleted by dropping the register-with-stems schema; re-triage once the
single-prompt contract lands).

Verification: native is the default delegation path; the mercenary spawns only for
implementer/reviewer via the routing gate (user-explicit or `prefer_mercenary`
flip); the continuation handle matches the native agentId shape; the
register-with-stems schema is gone; the two re-triaged bugs have an explicit
disposition (fixed or a recorded follow-up).

### Result (0c7c0f50) - 2026-06-11

Reshaped the retained codex/claude runner + `agents.*` core into the first-class
mercenary surface on branch `implement/ws-session-auth-phase2c` (stacked on the
unmerged 2a+2b; all three pending a combined merge to the epic). Code result-commit
`0c7c0f50`; review fixes `b2c13f02`.

Delivered:
- **Render-minted child keys** — `playbook.render(session_key, name, context?,
  root_override?)` mints a fresh child session key when the caller key is
  lead-scoped AND the playbook frontmatter `role` is delegate-eligible
  (`implementer`/`reviewer`/`delegate`→`roleDelegate`, `leaf`→`roleLeaf`), binds it
  to `root_override`-or-caller-root, and splices a credential block into the
  rendered body. Added `PlaybookMeta.Role` + frontmatter parse. Non-lead callers
  and non-delegate-role playbooks never mint; a second render mints a distinct key.
  Filled M1's deliberately-placed seams (no `playbook.render` redesign).
- **`ws.lead.prefer_mercenary(session_key)`** — lead-only render-mode flip
  (`sessionEntry.preferMercenary`); enforced by the existing 2a keyed-handler
  `ws.lead.*` gate (no second check). Flips only the default delegation *guidance*
  for implementer/reviewer renderings; an always-on mercenary tip in every
  `delegates:true` rendering keeps the on-request path reachable without the flip.
  ws-only: hidden in agentless wsflow (`noAgentHiddenTool`); `ws.lead.login` stays
  visible.
- **Register narrowing** — dropped `prompts`/`prompt_refs`/`tier`/`model` from the
  `agents.register` MCP schema + dispatch (satisfies `spec-remove`
  `260508-agents-register-model-alias-field`); `RegisterOptions` struct fields
  retained for live internal callers (api_docs/oneShot). **Native-shaped handle** —
  `agents.call` returns `agentId=<name>` for one continuation idiom across native
  and mercenary paths.
- **Spawn depth strictly 1** preserved (child keys are non-lead → keyed gate
  rejects `ws.lead.*` → a child cannot login/mint); no recursion counter, no
  capability ENFORCEMENT added (Phase 3).

Deviation — **diagnostic minimization is a deliberate no-op** (lead ruling): the
contract-first spec (`#260508`/`#260512`) still documents `agents.debug.*`/status/
tail for the live mercenary lifecycle, and 2b already removed the subquery/gemini
coupling, so removing more would contradict the binding spec. Nothing was
orphaned by the reshape. Carried-over scope from 2b discharged as "retain".

Bug re-triage (U7) — both kept, neither dropped (option B retains the path,
superseding the stale option-C "Pending Removal" notes which were reconciled,
`eec9f6d1`): `260517-bug-ws-agent-empty-result-after-tool-use` persists (result-
capture path untouched by the reshape; stays `todo/`, needs a dedicated fix);
`260524-bug-ws-agent-register-stale-dir-result-hang` not obsoleted by the
register-stems drop (stale-dir reset + result-hang live in the agent-dir/lifecycle
path; stays `idea/`).

Execution deviation — the implementer mercenary completed Units 1-5 then its
claude backend crashed (`backend invocation failed: exit status 1`); the
mental-model-updater mercenary likewise completed its edits then crashed on the
report phase (two backend crashes this run). Delegation-first was honored; the
lead finished Units 6-7, the new-test coverage, and the review fixes directly from
the high-quality partial work. Fresh evidence for the `260517` retained-path
robustness class; also surfaced a Phase 2b regression (shipped-rsrc edit without
manifest regen left the tree red — fixed `a21241e6`, dogfood idea
`260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`).

Verification: `go build/vet/test ./... -count=1` green; wsflow contract test green
(3/3); `prefer_mercenary` in full ws `runtime.json`, absent from wsflow.

Review: partitioned (correctness/fit/test). Correctness clean; fit non-clean
minor-only (file naming, fixture comment) all addressed; test non-clean 2 important
(prefer_mercenary delegate-rejection coverage, native-handle untested) fixed in
`b2c13f02` (extracted `agentCallHandleText` helper + unit test; added the rejection
test). Re-review lead-adjudicated (reviewers are crash-prone mercenaries; findings
were coverage/cosmetic, fixes verified green). No unresolved disputes.

Spec: `260610-mercenary-delegation-surface` 🚧 stripped (implemented);
`260508-agents-register-model-alias-field` removed; `#260609` `playbook.render`
keyed signature documented (`5cc57c64`). Mental models updated (`095184bc`).

> Forward (Phase 3): exec stateless + capability-scope ENFORCEMENT (the
> `WS_MCP_TOOL_PROFILE` fold; the keyed gate already does `roleAllowsTool`) +
> dashboard build-fix (actors table / AgentDefinition actor columns gone; exec-job
> `owner_actor_id` deferred here). Re-triaged bugs `260517` + `260524` remain live
> on the retained mercenary path for a dedicated fix.

#### Edition (379ff5e5) - 2026-06-11

Follow-up scope identified during live 2c dogfooding (post-merge of the
WS_RSRC_ROOT launcher fix, `379ff5e5`): two delegate-surface behaviors that 2c
built the runtime/mechanism for are **unreachable on the shipped surface**
because the rsrc asset that would exercise them is missing. The user elected to
fill these within this ticket (they are expected behaviors of the surface 2c
already shipped, not new Phase 3 scope). Both gaps close with one root asset.

- **Gap 1 — render-minted child-key splice never fires on shipped playbooks.**
  `renderPlaybookBody` (`internal/mcp/playbook_tools.go`) only mints + splices
  the credential block when the caller is lead AND
  `childRoleForPlaybookRole(meta.Role)` is ok (role ∈
  `implementer|reviewer|delegate|leaf`). The mechanism, `PlaybookMeta.Role`
  frontmatter parse, and unit tests (`mercenary_surface_test.go`,
  `session_auth_test.go` via in-memory role fixtures) all exist, but
  `grep -rl '^role:' agents-plugin/rsrc/*/*.md` matches nothing — no shipped
  playbook declares a role, so every real render has `meta.Role == ""` and the
  credential block is never spliced.
- **Gap 2 — light/core/deep → per-harness model vars are never surfaced.**
  `resolveModelVars(harness, config)` resolves `{{.LightModel}}`/`{{.CoreModel}}`/
  `{{.DeepModel}}` per harness via config `ModelAliases`; defaults diverge
  correctly (`claude` → haiku/sonnet/opus, `codex`/default →
  gpt-5.4-mini/gpt-5.5/gpt-5.5). But no shipped `rsrc/` playbook nor any
  `internal/wsprompt/prompts/` prompt declares or uses these tier vars, so the
  harness-diverged model guidance is never rendered into a delegate prompt.

Fill (single root asset): add the missing shipped delegate playbook asset(s)
under `agents-plugin/rsrc/` with `role: implementer` / `role: reviewer`
frontmatter that ALSO declare and use the `{{.LightModel}}`/`{{.CoreModel}}`/
`{{.DeepModel}}` tier vars in their guidance text; regenerate the rsrc manifest
(shipped-rsrc edits require regen — see `260611-bug-rsrc-manifest-regen-missed-after-shipped-edit`);
add an end-to-end test that renders a SHIPPED delegate playbook with a lead key
and asserts (a) the credential block is present and (b) the tier vars resolve to
the expected per-harness model strings. The existing unit tests pass on
in-memory fixtures and do not catch a missing shipped asset, so the
shipped-asset e2e assertion is the key new coverage. This delivers the
"single self-contained prompt from `playbook.render`" + "scope mercenary to
implementer/reviewer roles only" 2c contract end-to-end (the self-contained
prompt IS this delegate playbook asset).

Absorbed: idea `260611-bug-no-delegate-role-playbook-asset-renders-child-key-unreachable`
(moved to `.dropped/`). Distinct sibling dogfood findings kept open:
`260611-bug-rsrc-load-unknown-playbook-misleading-error` and
`260611-bug-launcher-repair-failure-opaque-mcp-error`.

#### Edition (0c7c0f50) - 2026-06-11

(Hash points at the 2c reshape commit that introduced this regression; this
Edition records a follow-up gap, not yet implemented. Continues the
delegate-surface gap analysis from the prior Edition — same root, third axis.)

Audit (sonnet Explore, two passes) of the mercenary layer vs. the pre-2c
`ws.agents` surface found that **per-spawn / per-role tier (and concrete model)
selection was lost from the MCP surface as direct collateral of the
register-schema removal** (Unit 4 of `0c7c0f50` dropped `tier`/`model`/`prompts`/
`prompt_refs` from `agents.register`).

- **Was reachable pre-2c, now is not.** Pre-2c a lead could
  `agents.register(tier: "deep")` (or `model: …`) and route the spawn through
  `ResolveAgentForHarnessConfig` to the user's custom deep-tier backend/model
  config. Post-2c the MCP handler reads no tier/model (`server.go:857-875`), and
  `Manager.Register` hardcodes `opts.Tier = "core"` whenever no tier flows in
  (`agent.go:550-552`) — which is always, from MCP. Every MCP-spawned mercenary
  resolves against the **core** tier only.
- **The custom-tier routing mechanism itself is intact but unreachable.**
  `config.agents_tier` still exposes the full `SetAgentsTierForHarness`
  (`tier`/`backend`/`model`/`harness`/`effort`; tiers constrained to
  light/core/deep by `normalizedTier`, harness to default/codex/claude). But on
  the shipped MCP surface only the **core** entry is ever consulted for a
  mercenary. Custom **light**/**deep** configs are reachable only by the two
  hardcoded internal `api_docs.go` callers (pre-router=light, manager=core); the
  **deep** tier config is consumed by **no live code path at all**. A user who
  sets `light → {claude, haiku}` / `deep → {codex, gpt-5.5-pro}` sees it applied
  to no mercenary spawn (and, for deep, to nothing).
- The 2c Result's "per-mercenary model moves to the rendered prompt + harness
  config" is only half true: the harness-config path works but is pinned to
  core; the rendered-prompt path injects `{{.Light/Core/DeepModel}}` into prompt
  *text* only and never sets the subprocess `--model`/`-m`.

Fill (extends the same delegate-asset work in the prior Edition — close all
three together): restore an MCP-reachable per-spawn/per-role tier (optionally a
concrete model) selection so a lead can route a specific mercenary to
light/core/deep and thus to the user's custom backend/model config. Decide the
path at implementation — candidates: a `tier` arg on `agents.call` and/or
`playbook.render`; OR have `playbook.render` read a `tier:` frontmatter (reusing
the same shipped delegate playbook that carries `role:`) and thread it into the
minted child's `RegisterOptions`; OR bind a tier into the render-minted child
key. The delegate playbook asset added for the child-key/model-var gaps should
ALSO declare the role's default tier so one asset closes all three. Verification
addition: an e2e test that customizes light & deep via `config.agents_tier`,
spawns a mercenary routed to each, and asserts the subprocess resolves to the
custom backend/model (not core).

Also flagged (incomplete-migration markers, fold into the same pass):
`Manager.oneShot()` (`agent.go:1099`) + `oneShotOptions` have no production
caller (test-only) despite the 2c Result citing oneShot as a "live internal
caller" that justifies retaining the `RegisterOptions` tier/model fields.

### Phase 3: exec stateless + role-containment fold + dashboard build-fix

Confirm exec is a stateless `exec_key` capability anchored at the session root;
fold role-containment into capability-scoped keys; keep the dashboard **compiling**
against the reshaped session/lifecycle surface (mechanical read adaptation, no
feature strip — the agent-activity port-vs-remove decision is deferred to a
separate dashboard ticket). Verification: exec works without any actor;
capability-scoped keys gate delegate tools; the dashboard builds and runs against
the reshaped surface.

### Result (ec2ad888) - 2026-06-11

Final phase landed on `implement/ws-session-auth-phase3` (renamed from
`...-phase2c`; stacked on unmerged 2a+2b+2c — all four pending a combined merge to
the epic). Code commits `466d103c..ec2ad888`; review was clean first pass (no
separate fix commit, so the result hash is the last implementation commit). Lead
pre-surveyed the three areas via native Explore passes — the survey collapsed the
phase to a smaller surface than the sketch.

Delivered:
- **exec fully stateless.** `owner_actor_id` removed from the `exec_jobs` record,
  DDL, INSERT/ON CONFLICT/SELECT/scan, and the exec→artifact linkage. A
  drop-column migration generalizes the existing Phase 2a
  `recreateTableWithoutColumns` helper (added createSQL/removedColumns/tempSuffix
  params, shared with the agent-table callers) and recreates legacy `exec_jobs`
  without the column. exec already resolved root via mandatory `session_key` only;
  `owner_actor_id` (always empty post-2a) was the sole remaining actor residue.
  The shared `artifacts.owner_actor_id` column is left in place (named-agent
  infra, out of scope).
- **Capability-scope fold — the keyed scope is now the sole authority.** Removed
  the process-wide env role layer: `Server.role`, `requestedToolRole()`, the
  `roleAllowsTool(s.role, …)` coupling in `toolAllowed`, and the
  `WS_MCP_TOOL_PROFILE=` env append in `codex.go`/`claude.go`; the now-dead
  `RegisterOptions`/`RunnerRequest.ToolProfile` field was removed (it terminated at
  the env seam, no lifecycle widening). The keyed `callTool` gate
  (`roleAllowsTool(entry.scope, …)` + `ws.lead.*` block for non-lead keys) is
  unchanged and is the sole tool-permission boundary; `tools/list` advertises the
  full lead surface (advisory). `WS_MCP_ALLOWED_TOOLS` preserved as a
  role-independent visibility allowlist. No containment regression: the env layer
  was verified non-functional and the working replacement (render-minted child
  key) already shipped in 2c.
- **Dashboard build-fix.** `cargo build` already succeeded against the reshaped
  schema (production queries never read actor columns) — confirmed, no production
  change needed. Aligned the one stale test fixture
  (`crates/daemon/tests/routes.rs`): dropped the `agent_defs` `actor_id` column
  from the CREATE, INSERT list, and bind. No Activity feature change
  (port-vs-remove stays deferred to a separate dashboard ticket).

Tests: realigned the env-profile tests (`TestServeStdioFiltersToolsByProfile` →
keyed-scope; `TestExplicitAllowedToolsCannotBypassEffectiveRole`) to exercise a
leaf **session key** through the keyed gate (coverage preserved, not deleted);
added an `exec_jobs` migration test (legacy column dropped + row data preserved;
fresh DB omits the column). `go build/vet/test ./... -count=1` green (13/13
packages); `cargo build` + daemon crate tests green.

Pre-existing red (NOT Phase 3): the Python suites `agents-plugin/tests` and
`agents-plugin-wsflow/tests` fail on stale skill-dispatch/bundle contracts
(lead-workflow-manual absent under `agents-plugin/skills/`; lead-proceed route
text moved to the playbook). Verified identical at epic base `c917c9f0`, and the
Phase 3 diff is disjoint (Go+Rust only). Captured as idea
`260611-bug-skill-dispatch-contract-tests-stale-after-entry-shim-migration`.

Review: partitioned (correctness/fit/test), all `[clean]` first pass. Two
non-blocking notes accepted: the dropped `tools/list`-hidden assertion is
by-design (schema is advisory post-fold), and the generalized-migration
agent-table branch is transitively covered by the green 2a migration tests.
Reviewers ran native (default delegation; the user did not request mercenary); a
reviewer-tier policy (correctness→deep, fit/test→core) was adopted for future
delegations.

Spec: `#260505-tool-profile-gating` reconciled (`WS_MCP_TOOL_PROFILE` retired as
authority; the keyed gate is the enforcement; Planned callout dropped) plus the
stale exec `actor/session binding` wording (`12140a70`). Mental models
mcp-runtime + named-agent-runtime updated (`9d0d15d0`); the mental-model-updater
was lead-authored due to host-neutral delegation friction (register dropped the
prompt-bundle field; a native delegate lacks the bundle).

> Forward: the two Phase 2c Editions (delegate playbook `role:`/`tier:` asset +
> MCP-reachable per-spawn/per-role tier routing) remain open fill scope the user
> elected to complete; the per-role/per-partition delegation-tuning config idea
> (`260611-research-ws-per-role-delegation-tuning-config`) generalizes the tier
> direction. Re-triaged bugs `260517` + `260524` stay live on the retained
> mercenary path. All of 2a+2b+2c+3 await a single combined merge to epic
> `260605` by user decision (this phase was run "without merging").

Phase order: 1 → 2a → 2b → 2c → 3, strictly sequential. Phase 1 (additive
session-auth) before 2a (session-auth must exist before the actor model is
deleted). 2a (sever the actor dependency + mandatory key) before 2b (the retired
backends/paths only compile against the session-keyed core once the actor model
is gone) and before 2c (the codex reshape rewires the spawn path onto session
keys). 2b before 2c is the lower-risk ordering (delete the retired surfaces while
the codex path is still in its pre-reshape shape, then reshape the smaller
remaining surface). Phase 3 (exec stateless + role-containment fold + dashboard
build-fix) last.

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
