---
title: "agent.spawn 워커를 holder 아래로 이동"
related:
  261118-feat-ws-agy-tool-names-and-worker-role: "reverses Decision (151), which made agy workers daemon-owned"
  261119-feat-ws-codex-hosted-worker: "reverses Decision (172), which copied the daemon-owned agy worker shape"
  261119-bug-ws-agent-v2-dogfood-surprises: "Phase 4 Items 8 and 10 are symptoms of daemon-owned agy workers"
---

# Move agent.spawn workers under session holders

## Background

At `develop` commit `1820eb1fc`, hosted leads on every backend and Pi workers
are holder-backed, while Claude, Codex, and agy workers remain daemon-owned.
The owner identified the split as a missed gap and directed that all
`agent.spawn` workers move to holders (2026-11-20 17:31 KST).

This explicitly reverses the daemon-owned worker choices in
`261118-feat-ws-agy-tool-names-and-worker-role` Decision (151) and
`261119-feat-ws-codex-hosted-worker` Decision (172). Those choices reduced
holder branching, but a daemon replacement kills the worker and interrupts
its open turn. The required outcome is that every spawned worker survives a
normal daemon replacement, rejoins through its existing holder, and reports
its in-flight result to its parent exactly once.

All source paths below are relative to `agents-plugin-tool/` and were verified
against `develop` at `1820eb1fc`.

## Decisions

- **All `agent.spawn` workers are holder-backed (owner, 2026-11-20 17:31
  KST).** A daemon replacement detaches from, rather than terminates, the
  worker process. The successor rejoins the same holder and process and keeps
  the ordinary worker completion/report path. Rejected: Decision (151) in
  `261118-feat-ws-agy-tool-names-and-worker-role` and Decision (172) in
  `261119-feat-ws-codex-hosted-worker`, which made agy workers daemon-owned and
  copied that shape for Codex workers.
- **The holder remains a thin, protocol-blind stdio process holder.** It keeps
  launch arguments, one child, input/signal/stop operations, child status, and
  bounded output replay; backend protocol recovery remains in the daemon
  (`internal/wsholder/holder.py:13-20,48-66,1080-1191`). This answers the
  owner's 17:44 concern that a holder should not acquire protocol complexity;
  the owner acknowledged the simplified design at 17:59.
- **Use one backend-neutral `agent-worker` holder session and ownership
  check.** Generalize the existing Pi-worker `holderSession` to carry the
  worker backend, reply id, cwd, and capability binding, and apply the same
  holder/PID validation to every worker backend
  (`internal/wsagent/holder.go:69-89`,
  `internal/wsagent/runner_process.go:527-542`). Rejected: separate holder
  kinds and lifecycle branches for each backend.
- **Reuse the held-lead HTTP-MCP tuple save/restore path for
  `WorkerHTTPMCP`.** Persist and restore the same token, URL, environment, and
  holder binding after ownership validation; the already-running worker
  cannot learn a rotated token
  (`internal/wsagent/runner_process.go:359-402,451-456`,
  `internal/wsagent/mcp_session.go:88-137`). Rejected: token rotation during
  successor attach.
- **Reuse the existing daemon replay reconciliation with no worker-specific
  recovery design.** Replay silently rebuilds state, uses existing turn/gap
  handling, deduplicates answered controls, and retains the backend request-id
  and Codex thread-read recovery paths
  (`internal/wsagent/stream.go:169-199,251-257`,
  `internal/wsagent/runner.go:727-741`,
  `internal/wsagent/runner_process.go:511-625`,
  `internal/wsagent/record.go:92-96`,
  `internal/wsagent/runner_events.go:377-400,562-580`,
  `internal/wsagent/codex.go:151-176`,
  `internal/wsagent/codex_mapper.go:120-129,398-425`,
  `internal/wsagent/agy.go:215-267,679-695`). Rejected: treating holder-ring
  replay alone as proof that an open turn completed.
- **Legacy direct workers settle an open turn interrupted once, then use a
  holder on their next launch.** Startup already performs this settlement
  before queued input can open a new turn
  (`internal/wsagent/runner_conv.go:521-528`,
  `internal/wsagent/spawn.go:368-381`). Rejected: attempting to revive a
  process that the predecessor daemon already killed.
- **Compatibility target: T1 using `handed_off_at` as the old-daemon fence
  (owner, 2026-11-20 17:59 KST).** While a non-lead worker is live under a
  holder, the new daemon keeps `handed_off_at` set with `HolderID`. An old
  daemon loads any `ClosedAt || HandedOffAt` record through the closed path and
  starts no loop, mail watcher, or process; its UI shows handed-off state and
  it changes no data (`internal/wsagent/runner.go:327-358,1657-1674`,
  `internal/wsagent/record.go:55-63`). Only the new daemon interprets
  non-lead + `HolderID` + `HandedOffAt` as a held worker and opens/rejoins it.
  The fence is cleared when the holder ends. Verification must include a
  directional old-daemon fixture proving that the old load path starts
  nothing. If the fixture does not prove T1, implementation stops and returns
  the old/new trace to the lead; T3 and a minor bump require explicit owner
  approval. Rejected: settling this ticket as T3 with a minor bump.
- **Keep the existing lifecycle meanings.** `agent.interrupt` remains
  turn-scoped. Idle timeout, explicit stop, conversation close, and expiry end
  the holder/process as they do for held leads and Pi workers; close revokes
  delegation and HTTP-MCP tokens
  (`internal/wsagent/runner_events.go:754-802,875-894,977-1027`). Rejected:
  making interrupt kill the held process.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)
- Manuals for the touched paths: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/performance.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md`.
- Preserve worker authority, profile, sandbox/permission, worktree/cwd, tier,
  model/effort, and mailbox semantics. Holder ownership must not grant a
  worker lead authority or change its caller-visible `agent.*` contract.
- Preserve delegation capability semantics and binding. A rejoined worker may
  continue using its existing child capability only after the holder session's
  backend, reply id, cwd where relevant, capability hash, and child PID pass
  the common ownership check.
- `WorkerHTTPMCP` workers retain the same token tuple across replacement, and
  the tuple remains bound to the validated holder and harness PID. A tuple for
  another holder is never adopted.
- A configured holder failure refuses launch; it must not silently fall back
  to daemon ownership. Keep steady-state handling memory-first and add no
  per-message, per-timer, or per-turn whole-file rewrite.
- Preserve the bounded holder ring and agy's existing thought-chunk filter.
  Windows survival remains conditional on the holder being restart-safe; a
  non-restart-safe holder must settle honestly rather than claim survival.
- **Out of scope:** re-arming agy's model-wait watchdog across daemon
  replacement. Held agy leads do not restore that daemon-memory watchdog
  today; workers match current held-lead behavior.

## Verified implementation seams

- Generalize the Pi-worker holder allocation and pre-scan retirement fence
  (`internal/wsagent/runner_process.go:82-89,184-190`), startup reattach
  (`internal/wsagent/runner.go:727-741`), and the Pi-only `canKick` scan fence
  (`internal/wsagent/runner_conv.go:1108-1111`) to all workers.
- Remove the Codex and agy worker-holder refusals
  (`internal/wsagent/codex.go:74-83`,
  `internal/wsagent/agy.go:142-156`). For Claude, change the launch-argument
  condition from `spec.Lead == nil || !attach` to `!attach`, because an attach
  rejoins an already-running process (`internal/wsagent/claude.go:449-461`).
- Save and restore the worker MCP tuple through the same validated attach path
  as a held lead; do not add a second token-recovery mechanism.

## Prior Decisions

- 261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart (2026-11-13, commit): "Reuse the existing holder pipe transport and publish/settle path, not a new delivery mechanism. Lead privileges remain independent of holder selection" — bearing: supports
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, Decisions 151): "the agy worker is not holder-held, like the Claude worker; a daemon restart ends it" — bearing: constrains
- 261119-feat-ws-codex-hosted-worker (2026-11-19, Decisions 172): "The Codex worker start path follows the agy worker shape: daemon-owned with no holder and no lead spec" — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, commit): "run hosted lead sessions under session holders" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/holder.go, runner_process.go, runner.go, runner_conv.go, claude.go, codex.go, agy.go, record.go; internal/wsholder/holder.py; ai-docs/manuals/ws-mcp.md |
| scope.surface | cross-module | wsagent worker lifecycle, wsholder session descriptor and daemon compatibility behavior cross module boundaries |
| scope.new_public_symbol | no | no new exported API symbol is named; agent-worker is a holder kind and holderSession is internal |
| scope.new_type_contract | yes | holderSession gains backend ownership metadata; persisted Record.HandedOffAt becomes the T1 old-daemon fence (internal/wsagent/holder.go#L69-L89; internal/wsagent/record.go#L55-L63) |
| scope.test_surface | existing | internal/wsagent/pi_worker_recovery_test.go, codex_worker_test.go, codex_recovery_test.go and lead_recovery_test.go exist; Phase 1 adds an old-daemon fixture |
| complexity.reuse_points | confirmed | existing Pi holder allocation and reattach, held-lead MCP tuple, and replay reconciliation were read (internal/wsagent/runner_process.go#L184-L190; #L359-L402; #L511-L625) |
| complexity.side_effect_risk | high | daemon replacement, holder lifetime, worker tokens and interrupted-turn settlement change together |
| risk.correctness | high | replay must settle in-flight work and parent reporting exactly once across four backends |
| risk.fit | high | one worker holder kind must preserve each backend's launch, authority, cwd and ownership binding |
| risk.test | high | directional old-daemon fixture, four-backend replacement coverage and Windows restart behavior are required |
| risk.security_or_contract | high | T1 fence and token/capability restoration affect old-daemon writes and worker authority |

## Phases

### Phase 1: Generalize holder ownership and install the T1 fence

1. Generalize the Pi-worker holder allocation, scan fence, startup reattach,
   holder session, ownership validation, applied sequence, and delegation
   capability restoration to Claude, Codex, agy, and Pi workers. Remove the
   Codex/agy refusals and make Claude attach skip launch-only argument/profile
   reconstruction.
2. Persist and restore `WorkerHTTPMCP` through the existing held-lead tuple
   path after holder, reply-id, capability, and PID validation. Keep
   authority/profile/sandbox/worktree and delegation semantics unchanged.
3. Implement the `handed_off_at` compatibility fence: old daemons see a live
   held worker as handed off and start nothing; new daemons recognize the
   non-lead + `HolderID` + `HandedOffAt` combination and rejoin it; holder end
   clears the fence. Keep the existing once-only interrupted settlement for
   legacy direct-worker records.
4. Add the directional compatibility fixture against the old-daemon load
   path. Assert no conversation loop, mail watcher, holder attach, or process
   start occurs and no record data changes. If the trace is not T1, stop and
   return both old/new traces to the lead; do not select T3 or a minor bump.

### Phase 2: Prove replacement continuity and update the operations contract

1. Add provider-free replacement coverage for Claude, Codex, agy, and Pi.
   During an in-flight turn, replace the daemon, rejoin the same holder and
   harness PID, restore replay and the `WorkerHTTPMCP` tuple where applicable,
   and complete without a replacement-induced interrupted settlement.
2. Assert the parent receives one terminal status and one final worker report
   exactly once, including completion while no daemon is attached. Also prove
   queued mail waits for reconciliation and then wakes/steers normally.
3. Cover holder-kind, backend, reply-id, cwd, capability, PID, and MCP-tuple
   mismatches; legacy direct-worker startup; authority/profile and delegation
   behavior; interrupt versus idle/stop/close; and restart-safe versus
   non-restart-safe Windows holders.
4. Update `ai-docs/manuals/ws-mcp.md` where it describes daemon replacement,
   session holders, and spawned-worker close/hold behavior. State that every
   `agent.spawn` backend is holder-backed, document the T1 old-daemon fence and
   tuple binding, and keep watchdog re-arm explicitly out of scope.
5. Run focused holder, runner, backend, MCP, and process-control tests plus the
   repository's configured Go verification. Report startup/rejoin and
   steady-state file-operation deltas; measure on the Windows antivirus
   baseline only if counts change.
