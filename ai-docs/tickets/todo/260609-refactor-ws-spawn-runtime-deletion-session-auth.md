---
title: ws spawn-runtime deletion and ephemeral session-auth model
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260605-research-ws-native-subagent-pivot: direction, session-auth model, root-vs-cwd, role-containment decisions
  260609-refactor-ws-skill-text-playbook-conversion: prerequisite — skill text must stop referencing the spawn surface first
  260517-bug-ws-agent-empty-result-after-tool-use: resolved-by-deletion in this milestone
  260524-bug-ws-agent-register-stale-dir-result-hang: resolved-by-deletion in this milestone
  260524-bug-wsstore-ci-sqlite-busy: resolved-by-deletion (in-memory session map replaces wsstore actor records)
  260524-bug-subquery-non-head-history-evidence: resolved-by-deletion (subquery runtime removed)
  260524-bug-subquery-working-directory-stderr: resolved-by-deletion (subquery runtime removed)
  260525-bug-ws-setup-cwd-plugin-cache-root: design input — the new contract must not reproduce this footgun
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
  - prompt-bundle
---

# ws spawn-runtime deletion and ephemeral session-auth model

## Background

Milestone M3 of the playbook-factory pivot (epic
`260605-epic-ws-playbook-factory-pivot`). With skill text no longer referencing
the spawn surface (M2), this slice deletes the subprocess-spawn agent machinery
and replaces the persistent actor/authority model with an ephemeral in-memory
session-auth model.

This is the largest milestone and the one that realizes the pivot's
simplification payoff (no dual-path maintenance). Full direction, empirical
grounding, and rejected alternatives live in
`260605-research-ws-native-subagent-pivot` (see the 2026-06-09 "actor/setup →
ephemeral session auth model", "root role unchanged; cwd separated", "exec →
stateless capability", and "role-containment retained" sections, plus the
original "total spawn removal" inventory).

## Decisions

Binding decisions from the research ticket and the epic Cross-Child Decisions:

### Spawn removal (no fallback path survives)

- Remove `agents.*` lifecycle/diagnostic tools (register/call/wait/result/status/
  tail/print/cancel/erase/interrupt/debug.*), the `SelfWorkerStarter` async spawn
  path, the codex/claude/gemini Runner backends, SQLite role pointers and
  instance state, and wsstate file-backed agent state.
- Remove the `subquery` tool runtime (skill text already migrated to the Explore
  playbook in M2).
- Remove `api.ask` spawn machinery and the async job surface — coordinated with
  M4 (`api.ask` redesign owns the replacement contract).
- No spawn fallback path survives anywhere; dual-path designs are rejected.
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
- **Session term choice is OPEN**: `login` | `session.open` | `attach`. Decide at
  implementation; the contract shape above is term-independent.

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

Drop the following to `.dropped/` in the same commits that remove the code they
live on (audit linkage was deliberately preserved via their `## Pending Removal`
markers): `260517-bug-ws-agent-empty-result-after-tool-use`,
`260524-bug-ws-agent-register-stale-dir-result-hang`,
`260524-bug-wsstore-ci-sqlite-busy`, `260524-bug-subquery-non-head-history-evidence`,
`260524-bug-subquery-working-directory-stderr`. Close
`260525-bug-ws-setup-cwd-plugin-cache-root` when the session-auth contract lands
(it is retained as design input — the new contract must not reproduce the
plugin-cache-root binding footgun).

### Dashboard agent-audit strip

Strip the dashboard's agent-audit / agent-activity logic (sourced from
`agents.tail/status/debug`) here, since it shares the code removed by spawn
deletion. The dashboard is otherwise **retained** as a web-tmux surface; only the
agent-activity source is removed.

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

### Phase 2: spawn machinery + actor model deletion

Delete `agents.*`, `subquery` runtime, runner backends, `SelfWorkerStarter`,
SQLite role pointers/instance state, wsstate, and the actor/authority/child-actor
machinery. Drop the resolved-by-deletion bug tickets in the same commits.
Verification: full ws runs agentless by default; no spawn path remains; the
dropped bug tickets are removed with their code.

### Phase 3: exec stateless + role-containment fold + dashboard strip

Confirm exec is a stateless `exec_key` capability anchored at the session root;
fold role-containment into capability-scoped keys; strip the dashboard
agent-audit/agent-activity source. Verification: exec works without any actor;
capability-scoped keys gate delegate tools; the dashboard runs without the
agent-activity source and otherwise unchanged.

Phase order: Phase 1 before Phase 2 (session-auth must replace the actor
dependency before the actor model is deleted).

## Spec Impact

- Target spec area: `mcp-tools.md` — remove the named-agent contracts
  (`#260505-named-agent-mcp-tools`, `#260508-agents-register-model-alias-field`,
  `#260512-agent-cancel-resume-guidance`, `#260512-agent-recall-hidden-surface`,
  `#260523-agents-root-schema-invisibility`), rewrite
  `#260524-mcp-actor-setup-bootstrap` and adjust `#260505-mcp-session-default-root`
  to the session-auth model, remove the skill-facing `subquery` contract, and
  fold `#260505-tool-profile-gating` into capability-scoped keys.
  `api.*` removals are owned by M4.
- Expected caller-visible change: actor bootstrap replaced by mandatory
  session-key auth; the entire `agents.*`/`subquery` MCP surface removed.
- Contract-first spec: yes. Resolve at ready promotion via `lead-write-spec`
  (likely several `spec-remove:` stems plus a new session-auth stem).
