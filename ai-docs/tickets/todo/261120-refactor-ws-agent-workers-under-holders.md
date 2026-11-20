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
against `develop` at `5d028ad10`.

## Open Decision Queue

Last ID: (1855)

(1855) [open] [policy] Compatibility tier of the downgrade surface: T2 with an owner-approved exception for aged-row prune, vs T3 with a minor bump.
- Context: T1 holds only for the fenced child row. The old daemon rewrites a
  conversation parent's record (release), and with the round-3 rule the new
  daemon reconciles it on re-upgrade with no duplicate report (T2-like
  convergence). The old prune deletes the fenced child's session dir after
  RegistryRetention (7 days); that loss is unrecoverable, so it is not T2
  under AGENTS.md without an exception.
- Alternatives: (a) T2 with an explicit owner-approved exception: a downgrade
  longer than RegistryRetention loses the fenced worker's session history;
  patch release, no minor bump. (b) T3 with a minor bump.
> Recommend (a): (1854)=a already accepted the 7-day prune as degradation;
> (b) locks out previous-minor plugin sessions.

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
  cannot learn a rotated token. This deliberately reverses Decisions (151)
  and (172), the `b3e2e5e65` Phase 2 Result's "Worker tokens stay in memory"
  contract, and the current daemon-owned comment
  (`internal/wsagent/runner_process.go:178-181`). On rejoin, restore the tuple
  next to `restoreToken` inside `checkReattach`, after the common holder
  ownership check, rather than copying held lead `startLead`, which restores
  it before that check (`internal/wsagent/runner_process.go:381-383,515-542`);
  Codex and agy do not require `MCPSession` when attaching an existing process
  (`internal/wsagent/codex.go:81-83`, `internal/wsagent/agy.go:154-156`).
  Rejected: token rotation during successor attach or pre-validation restore.
- **Reuse the existing daemon replay reconciliation, with common worker
  rejoin passes rather than a new backend recovery design.** Replay silently
  rebuilds state, uses existing turn/gap handling, deduplicates answered
  controls, and retains the backend request-id and Codex thread-read recovery
  paths
  (`internal/wsagent/stream.go:169-199,251-257`,
  `internal/wsagent/runner.go:727-741`,
  `internal/wsagent/runner_process.go:511-625`,
  `internal/wsagent/record.go:92-96`,
  `internal/wsagent/runner_events.go:377-400,562-580`,
  `internal/wsagent/codex.go:151-176`,
  `internal/wsagent/codex_mapper.go:120-129,398-425`,
  `internal/wsagent/agy.go:215-267,679-695`). Backend attach derives its mode
  only from the child's persisted explicit selection through `childLevel` and
  never blocks on a parent ceiling; after every record is loaded,
  `holdersScanned` recomputes `childAdmission` against the current parent as
  an advisory check. A refusal applies to the next launch and must not stop or
  orphan the held process, matching held-lead rejoin behavior. After replay,
  one common pass auto-allows pending approvals for every
  `WorkerAutoApprove` worker
  (`internal/wsagent/runner.go:337-386,483-537,710-745`,
  `internal/wsagent/runner_process.go:61-81,121-135,236-246,574-607`,
  `internal/wsagent/agent_tools.go:405-483`,
  `internal/wsagent/runner_events.go:379-421,534-540`,
  `internal/wsagent/agy_bypass.go:15-42`). Rejected: treating holder-ring
  replay alone as proof that an open turn completed, blocking attachment on a
  parent that may not be loaded yet, or limiting replayed approval recovery to
  agy leads.
- **Legacy direct workers settle an open turn interrupted once, then use a
  holder on their next launch.** Startup already performs this settlement
  before queued input can open a new turn
  (`internal/wsagent/runner_conv.go:521-528`,
  `internal/wsagent/spawn.go:368-381`). Rejected: attempting to revive a
  process that the predecessor daemon already killed.
- **(1854)=a, owner 2026-11-20: downgrade treats a holder-backed worker as
  ended, as a documented degradation.** This is the same outcome as today's
  daemon restart: the old daemon sees the fenced child as closed, its
  `holdsReport`/`openChildOf` no longer holds a done parent turn, and
  `releaseHeld` publishes and rewrites the parent's record
  (`internal/wsagent/children.go:754-807`,
  `internal/wsagent/runner.go:386-394`). After `RegistryRetention`, old
  `PruneRegistrations` deletes the fenced child's session directory
  (`internal/wsdashboard/registry.go:57,386-424,460-464`). This accepts the
  behavior, not its release tier; (1855) remains open between a T2 exception
  and T3. Rejected: fencing non-lead parents, which cannot cover lead parents
  because an old daemon sweeps an unowned lead holder
  (`internal/wsagent/runner.go:528-537`).
- **The fenced child row has T1 behavior while that row exists; (1855) owns
  the unresolved tier for the whole downgrade surface.** The new daemon keeps
  `HandedOffAt` set with `HolderID` for a live new-kind `agent-worker`. The old
  daemon projects that child as closed/ended, starts no loop, watcher, attach,
  or process, never rewrites the child's own record, and cannot stop its
  holder because its holder-kind filter does not recognize `agent-worker`
  (`internal/wsagent/runner.go:333-358,528-537,1657-1674`,
  `internal/wsagent/record.go:55-63`). That child-row display/read behavior is
  T1. Parent release/rewrite and aged-row deletion are explicitly accepted
  degradation outside that child-row fact; their release tier awaits (1855).
  A live legacy `pi-worker` holder keeps the legacy shape so an old daemon
  rejoins it rather than sweeping it. Only the new daemon interprets non-lead
  + `HolderID` + `HandedOffAt` as a held worker.
- **Re-upgrade silently retires only a downgrade-fenced open child of a
  conversation parent whose turn predates the parent's settlement.** Before
  `checkReattach` requests holder replay in `holdersScanned`, require all of:
  the child is non-lead, has `HolderID` and `HandedOffAt`, has a
  `ParentConversation`, and has `TurnOpen` with non-zero `TurnStartedAt`; the
  restored parent has `TurnOpen == false`, `HeldTurn == nil`, and a terminal
  `LastTurn`; and the child's `TurnStartedAt` strictly predates the parent's
  parsed `LastActiveAt`, which `publish` writes in the same update as
  `LastTurn` (`internal/wsagent/record.go:106-115,125-151,166-170`,
  `internal/wsagent/spawn.go:305-341`,
  `internal/wsagent/children.go:754-795`). A current daemon cannot create that
  ordering: `holdsReport` sees an open child and holds the parent's done turn.
  Missing or non-ordered evidence adopts the holder normally; an idle child
  and a child mail-woken after its parent settled therefore survive an
  ordinary replacement.

  For the matching downgrade case, detach the in-memory adapter and event
  route, retire the holder through the existing holder-stop path, and make one
  child-record update: call `finishActiveTurn`, set `TurnOpen = false`, clear
  `HandedOffAt`, and clear `HolderID`/`HostPID`/`HolderSeq`, without
  `processEnded`, `settleTurn`, or `publish`
  (`internal/wsagent/spawn.go:278-286`,
  `internal/wsagent/runner_process.go:462-480`,
  `internal/wsagent/runner_events.go:730-802`). Leave `ClosedAt` empty and
  `LastTurn` unchanged; `finishActiveTurn` clears `TurnStartedAt`. The child
  becomes an ordinary sleeping child that later mail may wake, while the
  already-settled parent gets no second terminal status or report. If old
  pruning removed the child record, the new `agent-worker` orphan sweep
  retires the unowned holder.
  Retirement remains simpler than adoption plus a backend-specific late-report
  filter.

  This reconciliation does not apply to `ParentLead`. A lead has no parent of
  its own, so its old-daemon `releaseHeld` changes only memory and leaves the
  persisted `HeldTurn`; re-upgrade restores that held turn, adopts the child,
  and accepts exactly one late child report before releasing the held lead turn
  in memory (`internal/wsagent/spawn.go:288-341`,
  `internal/wsagent/runner.go:694-708`).
- **One held-worker predicate overrides every handed-off reader in the new
  daemon.** Non-lead + non-empty `HolderID` + non-empty `HandedOffAt` means a
  held worker, not a closed or handed-off conversation. Use that predicate for
  startup/open projection, `AgentStatus`, ended-write errors, hiring cleanup,
  and registration retirement/pruning so the caller-visible `agent.*`
  contract is unchanged (`internal/wsagent/runner_conv.go:719-724`,
  `internal/wsagent/agent_tools.go:213-217`,
  `internal/wsagent/runner.go:789-794`, `internal/wsagent/hire.go:48-55`,
  `internal/wsdashboard/registry.go:386-424,460-465`). Rejected: scattered
  caller-specific exceptions that let one projection expose a live worker as
  closed.
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
- Rejoin restores the backend mode from the child's own persisted explicit
  selection through `childLevel`/`cachedPermissionSelection`; a missing or
  unavailable parent must never block attachment to the existing process.
  After all records load, recompute the current parent ceiling as an advisory
  check: a refusal governs the next launch and never stops or orphans the held
  process. This satisfies `261115-feat-ws-permission-profile`'s requirement to
  avoid both a duplicate process and trust in an old ceiling. A valid Codex
  read-only selection restores native `read-only` plus per-turn `never`;
  write/full restore their mapped sandbox and policy
  (`internal/wsagent/agent_tools.go:405-483`,
  `internal/wsagent/codex.go:85-97,290-307`).
- Preserve delegation capability semantics and binding. A rejoined worker may
  continue using its existing child capability only after the holder session's
  backend, reply id, cwd where relevant, capability hash, and child PID pass
  the common ownership check.
- `WorkerHTTPMCP` workers retain the same token tuple across replacement, and
  the tuple remains bound to the validated holder and harness PID. A tuple for
  another holder is never adopted.
- Pending approval requests replayed from the holder, including requests that
  arrived while no daemon was attached, run the ordinary traced at-most-once
  auto-allow after replay for every `WorkerAutoApprove` worker.
- Scope the compatibility claim to the fenced child's row while it survives.
  Downgrade may release and rewrite a held parent and may delete an aged child
  directory after `RegistryRetention`; both are documented degradation, not
  T1 behavior, and their whole-surface tier remains open in (1855). On
  re-upgrade, silently retire only a fenced, open child of a conversation
  parent when the child's non-zero `TurnStartedAt` strictly predates the
  parent's terminal `LastTurn` settlement stamp in `LastActiveAt`. Finish that
  active turn and clear `TurnOpen`, `HandedOffAt`, and the holder fields in one
  record update without publishing. Otherwise adopt normally. In particular,
  ordinary idle/mail-woken children and children of a lead must survive
  replacement; a lead parent restores its held turn and receives one late
  child report.
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
- Add `agent-worker` to the recognized `holdersScanned` kind filter while
  continuing to recognize live legacy `pi-worker` holders
  (`internal/wsagent/runner.go:483-537`). Generalize `checkReattach`'s
  hard-coded `holderKindPiWorker` ownership check without rejecting a matching
  legacy holder (`internal/wsagent/runner_process.go:515-542`).
- Use the child fence/open-turn stamp and the restored conversation parent's
  settlement as the pre-replay downgrade reconciliation seam. `startConv`
  restores `TurnOpen`, `TurnStartedAt`, `HeldTurn`, `LastTurn`, and
  `LastActiveAt` before the first holder scan, while `checkReattach` does not
  request replay until that scan. `publish` writes a conversation's terminal
  `LastTurn` and `LastActiveAt` together; a current daemon cannot do so while
  an older-open child exists because `holdsReport`/`openChildOf` sees the
  child's `TurnOpen`. Require the fenced child's non-zero `TurnStartedAt` to
  be strictly earlier than that parent settlement stamp; absent, equal, or
  later evidence rejoins normally
  (`internal/wsagent/record.go:106-115,125-151,166-170`,
  `internal/wsagent/spawn.go:305-341`,
  `internal/wsagent/children.go:754-795`,
  `internal/wsagent/runner.go:386-401,483-517,694-708`,
  `internal/wsagent/runner_process.go:511-565`). For a match, reuse holder-stop
  mechanics but replace the field-only `retireHolder` write with one update
  that calls `finishActiveTurn` and clears `TurnOpen`, `HandedOffAt`,
  `HolderID`, `HostPID`, and `HolderSeq`; emit no settlement or report
  (`internal/wsagent/spawn.go:278-286`,
  `internal/wsagent/runner_process.go:462-480`). A `ParentLead` never enters
  this rule: its old release has no record write, and its persisted `HeldTurn`
  is restored (`internal/wsagent/spawn.go:288-341`,
  `internal/wsagent/runner.go:694-708`).
- On attach, `startWorker` currently leaves authority empty because it calls
  `childAdmission` only for a fresh start; Codex immediately maps that empty
  level and fails (`internal/wsagent/runner_process.go:61-81,121-135`,
  `internal/wsagent/codex.go:81-97,290-307`). Rejoin occurs inside
  `startConv` while filename-ordered records are still loading, before the
  child itself enters `r.convs`; a conversation parent not loaded yet makes
  `agentCeiling` fail, while an absent lead parent falls back to
  workdir-write (`internal/wsagent/runner.go:337-386,710-745`,
  `internal/wsagent/agent_tools.go:405-428`). Give backend attach the child's
  persisted `childLevel` (`cachedPermissionSelection`) and never refuse the
  existing process there. Recompute `childAdmission` after all records load,
  in or after `holdersScanned`, as an advisory diagnostic whose refusal
  applies to the next launch, following the lead's attach rule
  (`internal/wsagent/agent_tools.go:439-483`,
  `internal/wsagent/runner_process.go:236-246`).
- Remove the Codex and agy worker-holder refusals
  (`internal/wsagent/codex.go:74-83`,
  `internal/wsagent/agy.go:142-156`). For Claude, change the launch-argument
  condition from `spec.Lead == nil || !attach` to `!attach`, because an attach
  rejoins an already-running process (`internal/wsagent/claude.go:449-461`).
- Save and restore the worker MCP tuple through the same validated attach path
  as a held lead, but move the restore after worker ownership validation next
  to `restoreToken`; do not add a second token-recovery mechanism.
- Replay marks approval events quiet, so live auto-allow does not run and
  `answerApproval` refuses while replaying. Generalize the post-replay
  `allowAgyBypassPending` pass to include every non-lead backend whose
  registration sets `WorkerAutoApprove`
  (`internal/wsagent/runner_events.go:17-24,379-421,534-540`,
  `internal/wsagent/runner_process.go:574-607`,
  `internal/wsagent/agy_bypass.go:15-42`).
- Generalize the Pi-only startup orphan classification
  (`internal/wsagent/runner.go:452-475`). A lost-ring open worker turn enters
  `probeWorkerRecovery`, but only Pi currently implements `workerRecoverer`
  (`internal/wsagent/runner_process.go:574-582,628-648`,
  `internal/wsagent/backend.go:213-219`, `internal/wsagent/pi.go:525-538`).
  Claude, Codex, and agy therefore fail and settle honestly when the bounded
  replay cannot reconcile their RPC state; replace the Pi-specific
  `"Pi worker recovery failed"` text with backend-neutral wording.

## Prior Decisions

- 261113-bug-ws-pi-subagent-turn-dies-on-daemon-restart (2026-11-13, Decisions and Result): "Accepted downgrade consequence: an older daemon interrupts the turn and leaves its held worker process orphaned" and the in-flight interruption "equals today's behaviour" — bearing: supports (1854)'s documented degradation and conversation-parent reconciliation, but does not settle the whole-surface tier now open in (1855)
- 261118-feat-ws-agy-tool-names-and-worker-role (2026-11-18, Decisions 151): "the agy worker is not holder-held, like the Claude worker; a daemon restart ends it" — bearing: constrains
- 261119-feat-ws-codex-hosted-worker (2026-11-19, Decisions 172): "The Codex worker start path follows the agy worker shape: daemon-owned with no holder and no lead spec" — bearing: constrains
- 261110-feat-ws-dashboard-headless-lead-sessions (2026-11-11, commit): "run hosted lead sessions under session holders" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsagent/holder.go, runner_process.go, runner.go, runner_conv.go, claude.go, codex.go, agy.go, record.go; internal/wsholder/holder.py; ai-docs/manuals/ws-mcp.md |
| scope.surface | cross-module | wsagent worker lifecycle, wsholder session descriptor and daemon compatibility behavior cross module boundaries |
| scope.new_public_symbol | no | no new exported API symbol is named; agent-worker is a holder kind and holderSession is internal |
| scope.new_type_contract | yes | holderSession gains backend ownership metadata; persisted Record.HandedOffAt supplies T1 evidence only for the surviving child row, while the whole downgrade tier remains open in (1855) because conversation-parent release converges on re-upgrade but RegistryRetention deletion does not (internal/wsagent/holder.go#L69-L89; internal/wsagent/record.go#L55-L63) |
| scope.test_surface | existing | internal/wsagent/pi_worker_recovery_test.go, codex_worker_test.go, codex_recovery_test.go and lead_recovery_test.go exist; Phase 1 adds a directional old-daemon fixture and Phase 2 adds timestamp-discriminated conversation-parent reconciliation, persistent sleeping-record, normal-replacement negative, and lead-parent adoption coverage |
| complexity.reuse_points | confirmed | existing Pi holder allocation and reattach, held-lead MCP tuple, childLevel persisted-selection projection, advisory held-lead permission rejoin, and replay reconciliation were read (internal/wsagent/runner_process.go#L184-L190; #L236-L246; #L359-L402; #L511-L625; internal/wsagent/agent_tools.go#L476-L483) |
| complexity.side_effect_risk | high | daemon replacement, holder lifetime, worker tokens and interrupted-turn settlement change together |
| risk.correctness | high | normal replacement replay must report exactly once across four backends; re-upgrade may retire only a fenced open child whose turn predates a conversation parent's persisted settlement, must leave a wakeable sleeping record, and must adopt ordinary or lead-parent children |
| risk.fit | high | one worker holder kind must preserve each backend's launch, cwd and ownership binding; attach mode comes from the child's persisted selection while current-parent ceiling admission waits until all records are loaded |
| risk.test | high | child-row T1 evidence, documented parent/prune degradation, timestamp discriminator and negative cases, one-update sleeping record plus another restart, lead-parent late-report behavior, four-backend replacement coverage and Windows restart behavior are required |
| risk.security_or_contract | high | the surviving child row is T1, but the whole downgrade tier is open in (1855) because a conversation-parent rewrite is reconcilable and aged-row deletion is not; token/capability restoration and advisory admission remain authority-sensitive |

## Phases

### Phase 1: Generalize holder ownership and install the child-row fence

1. Generalize the Pi-worker holder allocation, scan fence, startup reattach,
   holder session, ownership validation, applied sequence, and delegation
   capability restoration to Claude, Codex, agy, and Pi workers. Remove the
   Codex/agy refusals and make Claude attach skip launch-only argument/profile
   reconstruction. Recognize both the new `agent-worker` holder kind and live
   legacy `pi-worker` holders; generalize Pi-only orphan classification and
   use backend-neutral honest-failure text when a non-Pi lost-ring turn has no
   recovery implementation. On attach, derive backend mode from the child's
   persisted explicit `childLevel` and never gate the existing process on
   parent admission. After all records load, recompute `childAdmission` in or
   after `holdersScanned`; any refusal is advisory for the held process and
   applies to its next launch.
2. Persist and restore `WorkerHTTPMCP` through the existing held-lead tuple
   path after holder kind/backend, reply-id, cwd, capability, and PID
   validation. Restore beside `restoreToken` in `checkReattach`, not in the
   held-lead pre-validation location. Keep authority/profile/sandbox/worktree
   and delegation semantics unchanged.
3. Implement the `handed_off_at` compatibility fence: old daemons see a live
   held `agent-worker` as handed off and start nothing; a live legacy
   `pi-worker` holder is not fenced. New-daemon startup, open-conversation
   projection, `AgentStatus`, ended-write errors, hiring cleanup, and
   registration retirement/pruning all use one predicate—non-lead +
   `HolderID` + `HandedOffAt` means held, not handed off—and rejoin it; holder
   end clears the fence. Keep the existing once-only interrupted settlement
   for legacy direct-worker records.
4. Add the directional compatibility fixture against the old-daemon load
   path. For a fenced new-kind `agent-worker`, assert the old daemon projects
   the child closed/ended, starts no conversation loop, watcher, attach, or
   process, never rewrites the child's own record, and sends no holder `Stop`;
   the stop rule is unreachable because the old kind filter does not recognize
   `agent-worker`. Separately load a live legacy `pi-worker` holder without the
   fence and assert the old daemon rejoins the existing holder without a
   duplicate process or `Stop`. Pin the documented degradation too: a held
   parent releases, publishes, and rewrites its record when it sees the child
   closed, and old `PruneRegistrations` deletes the child directory only after
   `RegistryRetention` (`internal/wsagent/runner.go:386-397,483-537`,
   `internal/wsagent/children.go:754-807`,
   `internal/wsdashboard/registry.go:57,386-424,460-465`). The fixture
   establishes T1 only for the surviving fenced child row; the parent and
   retention assertions document accepted downgrade behavior, and (1855)
   remains the release-tier gate for the whole surface.

### Phase 2: Prove replacement continuity and update the operations contract

1. Add provider-free replacement coverage for Claude, Codex, agy, and Pi.
   During an in-flight turn, replace the daemon, rejoin the same holder and
   harness PID, restore replay and the `WorkerHTTPMCP` tuple where applicable,
   and complete without a replacement-induced interrupted settlement. For
   Codex, cover both read-only and write persisted selections through the real
   runner rejoin path and assert the restored sandbox/approval policy,
   including the read-only per-turn `never` policy. Load the child before both
   a conversation parent and a lead parent and prove attach still succeeds;
   after `holdersScanned`, prove current-ceiling refusal is advisory to that
   process, is retained for the next launch, and does not stop or orphan the
   holder.
2. Assert the parent receives one terminal status and one final worker report
   exactly once, including completion while no daemon is attached. Also prove
   queued mail waits for reconciliation and then wakes/steers normally. Cover
   a permission request already pending at daemon replacement and one raised
   while no daemon is attached; after replay, `WorkerAutoApprove` answers each
   once through the traced approval path. Add re-upgrade coverage for a
   conversation parent whose turn the old daemon already released. Before
   replay, require a fenced child with `TurnOpen`, non-zero `TurnStartedAt`,
   and `ParentConversation`; require the parent to have
   `TurnOpen == false`, `HeldTurn == nil`, and a terminal `LastTurn`; and prove
   the child's start strictly predates the parent's parsed `LastActiveAt`
   settlement stamp. Retire that holder without
   `processEnded`/`settleTurn`/`publish`, and in one record update call
   `finishActiveTurn`, clear `TurnOpen`, `TurnStartedAt`, `HandedOffAt`,
   `HolderID`, `HostPID`, and `HolderSeq`, leave `ClosedAt` empty and
   `LastTurn` unchanged, and assert no second terminal status or duplicate
   parent report. Prove the child is an ordinary sleeping record that mail can
   wake and one more daemon restart preserves that wakeable shape. Negative
   cases must prove an idle child and a child mail-woken after its parent
   settled both survive a normal replacement and retain their holder. For a
   `ParentLead`,
   prove the rule does not fire: re-upgrade adopts the child, restores the
   lead's persisted `HeldTurn`, accepts one late child report, and re-releases
   that held lead turn in memory exactly once. Also cover an old-pruned child
   directory: the unowned new-kind holder is retired by the new-daemon orphan
   sweep.
3. Cover holder-kind, backend, reply-id, cwd, capability, PID, and MCP-tuple
   mismatches; legacy direct-worker startup; authority/profile and delegation
   behavior; interrupt versus idle/stop/close; and restart-safe versus
   non-restart-safe Windows holders.
4. Update `ai-docs/manuals/ws-mcp.md` where it describes daemon replacement,
   session holders, and spawned-worker close/hold behavior. State that every
   `agent.spawn` backend is holder-backed, document the surviving child row's
   T1 behavior, the unresolved whole-surface tier until (1855) settles,
   accepted downgrade parent/prune degradation, the timestamp-gated
   conversation-parent retirement into a wakeable sleeping child, lead-parent
   adoption with one late report, and tuple binding. Keep watchdog re-arm
   explicitly out of scope.
5. Run focused holder, runner, backend, MCP, and process-control tests plus the
   repository's configured Go verification. Report startup/rejoin and
   steady-state file-operation deltas; measure on the Windows antivirus
   baseline only if counts change.
