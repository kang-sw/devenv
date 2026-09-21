---
title: Explicit cwd override for pi ws-agent-spawn
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a9d9858672b6f634
sage-review-completeness-reviewed: a9d9858672b6f634
completed: 2026-09-21
---

# Explicit cwd override for pi ws-agent-spawn

## Background

pi's `ws-agent-spawn` sets a spawned subagent's working directory by blindly
inheriting the lead session's cwd: the lead's `ctx.cwd` is threaded
`registerAgentTools({cwd})` → `sessionCtx.cwd` → `RpcSpawnCtx.cwd` →
`buildRpcClientOptions(ctx.cwd, ...)` → the `spawn(..., {cwd})` in the RPC
client (search `agents-plugin-pi/src/spawner.ts` for `buildRpcClientOptions`
and `RpcSpawnCtx`). There is no per-spawn cwd argument, no env, and no
worktree-derived cwd.

Consequently a **worktree-spawn workflow** — acquire a worktree with
`worktree.acquire` (which returns the new directory `Path`), then spawn a
subagent to run inside it — has no way to place the child in that worktree
unless the lead's own cwd already is that worktree. This blocks explicit,
per-spawn working-directory control. Goal: add an optional explicit cwd
argument so worktree-spawn (and any directed-cwd) workflow can place a child
deterministically.

## Decisions

- **D1 — Add a pi-local optional `cwd_override` argument taking a raw absolute
  path.** It consumes `worktree.acquire`'s returned `Path` directly; pi does
  **not** resolve a worktree key. Rejected: a worktree-key argument — it would
  couple pi to the Go worktree registry for no gain, since `acquire` already
  returns the path.
- **D2 — The override survives stop/resume.** `ws-agent-send` auto-resume
  re-derives cwd from `sessionCtx.cwd` (search `spawner.ts` for `RpcResumeCtx`
  and the resume branch), so a spawn-time-only override would silently revert
  the child to the lead's cwd on resume — a wrong-directory bug. The override is
  therefore persisted on the agent record (`RpcAgentRecord`) at spawn and
  re-applied in the resume branch. Rejected: spawn-time-only application (reverts
  on resume; the user flagged this as the larger confusion risk).
- **D3 — Expose `cwd_override` only on `ws-agent-spawn`.** Because `cwdOverride`
  becomes a `SpawnAgentParams` field, the `ws-execute` spawn site (search
  `agents-plugin-pi/src/execute-gateway.ts` for its `spawnAgent` call) would
  technically honor it, but this ticket does not expose the argument there.

## Constraints

- **Validation (author fresh).** `cwd_override` must be an absolute path that
  exists and is a directory; reject otherwise with a clear error. No spawn-arg
  path-validation hook exists today: `write_scopes` is the schema→handler→params
  optional-arg precedent to mirror for wiring, but it is consumed in admission
  and is not a filesystem path, so its validation logic is not reusable. No
  repo/worktree-pool root constraint in this ticket — cwd is the process's
  location; writes stay gated by `write_scopes`. (Capability note: a future
  tightening could constrain `cwd_override` to an allowed root; out of scope
  here.)
- **pi-local surface.** `ws-agent-spawn` is defined only in `agents-plugin-pi`
  (`registerWsTool`); ws-mcp (Go) has no equivalent spawn tool, only a
  `SpawnIdiom` documentation label in `agents-plugin-tool/internal/mcp`. This
  change is confined to `agents-plugin-pi/src`; a host adapter, so no
  shipped-surface boundary obligation.
- **Tests are the behavioral contract.** Cover: the spawned child's cwd is the
  override when present and the inherited cwd when absent (regression); an
  invalid/nonexistent override is rejected; the override survives a
  stop→resume cycle.

## Prior Art / slot-in

Mirror the `write_scopes` / `alias` / `title` optional-arg flow (schema →
handler parse → `SpawnAgentParams` literal + interface) for the plumbing, with
one difference: `write_scopes` is consumed in admission, whereas `cwd_override`
must reach the `buildRpcClientOptions(...)` call in `spawnAgent` (change its
first argument to `params.cwdOverride ?? ctx.cwd`; the builder needs no
signature change). Persist on `RpcAgentRecord` and re-apply in the resume
branch for D2. All anchors in `agents-plugin-pi/src/spawner.ts`.

## Prior Decisions

- 260913-feat-ws-pi-delegated-write-scopes (2026-09-13, commit d00e66d3): "Scoped children reuse Pi's same-name native edit/write definitions after per-operation canonical authorization; no parallel tool vocabulary or broad fallback is introduced." — bearing: supports
- 260905-feat-ws-pi-agent-alias-park-and-registry-cap (2026-09-05, commit 4060bc59): "Alias reuse overwrites a dormant/idle holder's alias (title kept) rather than silently stacking two records under one name." — bearing: supports
- 260905-feat-ws-pi-push-only-child-reports (2026-09-05, commit ab9832a4): "I1 adds RpcAgentRecord.onResume because wireAntiBleedLoop needs a live client a revived dormant record has not got yet." — bearing: supports
- 260908-feat-ws-pi-agent-session-disk-retention (2026-09-13, commit 32b01532): "Retention must never delete a home while activity, protection, or dormant-resume state is being established." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | single-file | agents-plugin-pi/src/spawner.ts (schema, SpawnAgentParams, RpcAgentRecord, buildRpcClientOptions, spawnAgent, sendToAgent all live there; execute-gateway.ts's own spawnAgent call, L674-698, is not touched per D3) |
| scope.surface | public-interface | new cwd_override argument on the ws-agent-spawn MCP tool schema, agents-plugin-pi/src/spawner.ts:3506-3554 |
| scope.new_public_symbol | no | no new exported function or class; only a new optional field on existing exported types |
| scope.new_type_contract | yes | SpawnAgentParams gains cwdOverride, agents-plugin-pi/src/spawner.ts:1899-1918; RpcAgentRecord gains a persisted cwd override field for D2 |
| scope.test_surface | existing | agents-plugin-pi/test/spawner.test.ts |
| complexity.reuse_points | confirmed | write_scopes/alias/title optional-arg schema-handler-params flow, agents-plugin-pi/src/spawner.ts:3529-3598 |
| complexity.side_effect_risk | moderate | changes the spawned child's process cwd and its persisted dormant-resume behavior, agents-plugin-pi/src/spawner.ts:2893-2899 and :3059-3074 |
| risk.correctness | moderate | D2 requires the override to re-apply correctly in the dormant-resume branch, agents-plugin-pi/src/spawner.ts:3044-3075, without reverting to ctx.cwd |
| risk.fit | low | mirrors the established write_scopes/alias/title optional-arg plumbing pattern exactly |
| risk.test | moderate | needs spawn, invalid-path, and spawn-stop-resume coverage beyond the existing regression baseline, per the ticket's own Verification boundary |
| risk.security_or_contract | moderate | accepts a caller-supplied absolute path as the child process cwd with only existence/directory validation and no root/allowed-directory containment, deferred explicitly in Constraints |

## Phases

### Phase 1: Add `cwd_override`, place the child, and persist it across resume

**Intended behavior.** `ws-agent-spawn` accepts an optional `cwd_override`
(absolute path), validated as absolute + existing + a directory. The spawned
child's process cwd is the override when present, else the inherited
`ctx.cwd`. The override is persisted on the agent record and re-applied on
auto-resume, so a stopped/resumed child re-lands in the same directory rather
than reverting to `sessionCtx.cwd`.

**Deferred scope.** No worktree-key resolution (raw path only); no exposure on
`ws-execute`; no root/allowed-directory constraint (writes stay gated by
`write_scopes`).

**Verification boundary.** A unit test on the params/argv builder shows
`cwd_override` reaches `RpcClientOptions.cwd`; a spawn→stop→resume test shows the
child re-lands in the override rather than `sessionCtx.cwd`; an invalid or
nonexistent path is rejected with a clear error; an absent argument preserves
the current inheritance behavior.

### Result (5611965a) - 2026-09-21

Added `cwd_override` to `ws-agent-spawn`, validating absolute existing
directories before admission or allocation. The selected directory is retained
on `RpcAgentRecord` and applied for both initial launch and dormant resume;
omitted overrides retain the inherited session cwd.

Verification:

- `env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE -u WS_PI_DELEGATION_POLICY -u WS_PI_SUBTREE_CHANNEL node --test --test-name-pattern='cwd_override' test/spawner.test.ts` passed.
- The full `test/spawner.test.ts` run passed 193/195 tests; its two pre-existing Explore tests could not load the unavailable `pi-web-access` extension in this isolated worktree (`web-search-extension-missing`).
- Correctness, test, and round-2 review reports were clean.

Decision: validation precedes spawn admission so invalid overrides fail without
child allocation; worktree keys remain out of scope because callers already
hold the raw path.
