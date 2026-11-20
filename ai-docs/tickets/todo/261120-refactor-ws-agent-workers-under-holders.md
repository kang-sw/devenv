---
title: "agent.spawn 워커를 세션 홀더 아래로 이동"
related:
  261118-feat-ws-agy-tool-names-and-worker-role: "reverses Decision (151), which made agy workers daemon-owned"
  261119-feat-ws-codex-hosted-worker: "reverses Decision (172), which copied the daemon-owned agy worker shape"
  261119-bug-ws-agent-v2-dogfood-surprises: "Phase 4 Items 8 and 10 are symptoms of daemon-owned agy workers"
---

# Move agent.spawn workers under session holders

## Background

`agent.spawn` currently has two process-ownership models at source commit
`06e61595650295706781bd5d1d2ba72577c1cb0d`:

- Hosted leads on every registered backend and production Pi workers are
  holder-backed. Pi workers receive a `holderChild`, persist holder identity,
  detach instead of dying on daemon shutdown, and rejoin/replay on the
  successor (`internal/wsagent/runner_process.go:61-190`,
  `internal/wsagent/pi.go:429-472`, `internal/wsagent/runner_events.go:804-818`,
  `internal/wsagent/runner_process.go:483-625`).
- Codex, Claude, and agy workers run as daemon-owned subprocesses. `startChild`
  uses a holder only when `StartSpec.Holder` is present; otherwise the daemon
  owns the pipes and process (`internal/wsagent/stream.go:108-166`). The common
  worker path supplies a holder only for Pi (`runner_process.go:178-190`),
  Codex and agy explicitly reject a worker holder (`codex.go:20-23,74-83`,
  `agy.go:21-24,142-164`), and Claude starts whatever transport the common path
  supplies (`claude.go:443-472`). On daemon shutdown `letGo` detaches a holder
  process but stops every other process; a successor settles a persisted open
  direct-worker turn as interrupted (`runner_events.go:804-818`,
  `spawn.go:368-381`).

The owner identified that split as a missed gap and requires one ownership
model for every spawned worker. Owner, 2026-11-20 17:31 KST: "그리고
agent.spawn으로 띄운 에이전트가 홀더에 안 붙고 데몬에 붙는 건 제가 놓친
갭입니다. 전부 holder로 옮겨가야 합니다." (An agent started by
`agent.spawn` attaching to the daemon instead of a holder is a missed gap; all
of them must move to holders.) The goal is that a Codex, Claude, agy, or Pi
worker and its in-flight turn survive a normal daemon restart or replacement,
then complete and report exactly once.

## Decisions

- **All `agent.spawn` workers are holder-backed (owner, 2026-11-20 17:31
  KST).** A daemon replacement must detach from, not terminate, a spawned
  worker. The successor rejoins the same holder and process, restores the
  worker's daemon services, reconciles the protocol turn, and preserves the
  ordinary completion/report path.
  Rejected: the daemon-owned worker decisions in
  `261118-feat-ws-agy-tool-names-and-worker-role` (151) and
  `261119-feat-ws-codex-hosted-worker` (172). Decision (151) avoided a new
  holder kind and reattach path by following the then-current Claude worker;
  its implementation kept worker HTTP-MCP tokens in memory and revoked them
  at process/daemon exit. Decision (172) copied that agy shape to avoid a
  holder/lead-spec branch and retained token revocation on stop/shutdown. Those
  simplifications now lose because daemon replacement kills Codex, Claude, and
  agy worker turns, while the owner requires continuity across replacement.

## Constraints

- Manuals for the touched paths: `ai-docs/manuals/ws-mcp.md`,
  `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/performance.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md`.
- A configured holder failure refuses the worker launch; it must not silently
  fall back to daemon ownership. The current Pi contract already applies this
  rule (`ai-docs/manuals/ws-mcp.md`, holder-backed worker recovery section).
- Preserve the performance contract: no per-message or per-timer disk reads,
  no per-turn whole-file rewrite added beyond the existing record persistence,
  and no new process when an in-process route exists. Recovery state is read
  at daemon startup/rejoin and steady-state turn handling remains memory-first
  (`ai-docs/manuals/performance.md`).
- Windows survival is conditional on the holder being restart-safe. A holder
  marked not restart-safe may die with its starter's job object; expose that
  state and settle the turn honestly instead of claiming continuity
  (`ai-docs/manuals/ws-mcp.md`, Session holders).
- The holder ring remains bounded and filtered. In particular, agy thought
  chunks continue to match `agyRingSkip` and stay out of the ring
  (`internal/wsagent/agy.go:91-94,162-164`,
  `internal/wsholder/holder.py:1195-1214`). The in-flight branch
  `impl/develop/agy-thought-pulse` adds an activity pulse for held agy while
  retaining that filter; rebase or reconcile that behavior rather than
  restoring thought-chunk storage.
- The in-flight branch `impl/develop/manual-replace-ownership` is adding the
  `ai-docs/manuals/ws-mcp.md` section "Daemon replacement and process
  ownership". The implementation updates that section after reconciling the
  branch; it does not create a competing process-ownership section.
- `agent.interrupt` ends only the current turn. Idle timeout, explicit process
  stop, conversation close, expiry, and descendant close keep their existing
  distinct meanings. A stopped-but-open conversation remains resumable on its
  backend session; a closed conversation is not.

## Current recovery seams

The implementation should reuse these established seams instead of creating a
second worker supervisor:

- `Record` already persists `HolderID`, harness `HostPID`, `HolderSeq`,
  answered control requests, capability-token material, turn/report identity,
  and the held-lead HTTP-MCP token tuple (`record.go:27-38,83-96,112-164`). Its
  JSON codec preserves unknown top-level members across read-modify-write
  (`record.go:179-238`). No worker-only holder field is required by the current
  evidence.
- `holderChild` already launches, attaches, replays a bounded ring, records
  the applied sequence, and exposes the harness PID. The holder descriptor's
  session payload is consumer JSON and currently distinguishes `lead` from
  `pi-worker` (`holder.go:70-150`).
- `checkReattach` already validates a Pi worker's holder kind, reply id,
  capability hash, and child PID before restoring its delegation token; a
  mismatch detaches without stopping an unowned holder
  (`runner_process.go:515-564`). General worker ownership validation must be at
  least as strict.
- `WorkerHTTPMCP` identifies Claude, Codex, and agy worker transports. Current
  starts mint a memory-only token because workers are not held
  (`registration.go:18-27`, `runner_process.go:137-182`,
  `mcp_session.go:14-32,88-137`). Held workers need the same-token successor
  restore that held leads use, after holder ownership is validated, so the
  already-running harness keeps its ws connection.
- `idle_timeout_seconds` is applied after every worker start. A finished child
  sleeps by stopping its process, mail wakes it on the same conversation and
  backend session, and descendants/report holds prevent an idle stop. Moving
  ownership must change who owns the running process, not these sleep/wake
  semantics (`runner_process.go:201-226`, `runner_events.go:840-872`,
  `ai-docs/manuals/ws-mcp.md`, spawned worker Close/Holds sections).

## Per-backend analysis

The mechanism details below are candidates pending the corresponding Open
Decision Queue items; the protocol obligations themselves are required by the
owner decision.

### Codex app-server JSON-RPC

- Remove the worker-holder refusal and start/rejoin Codex workers through the
  same `holderChild` transport as held Codex leads. A rejoin must not run
  `initialize`, `thread/start`, `thread/resume`, or `model/list` again merely
  to attach; holder replay rebuilds mapper state first (`codex.go:71-140`).
- Codex control requests use kind-bearing JSON-RPC ids and synchronous
  request/response waits (`codex.go:151-176`). The predecessor's waiters die
  with it. Replay must treat their responses as historical, use fresh epochs
  for successor requests, and reconcile an ambiguous open turn through the
  existing `thread/read` recovery before releasing held input. Persisted reply
  source identities continue to suppress duplicate final replies
  (`codex.go:98-110`; `ai-docs/manuals/ws-mcp.md`, Codex app-server).
- Restore the worker's existing `WorkerHTTPMCP` token/hello against the
  validated holder and harness PID before replay releases work. A token bound
  to another holder is forgotten, never adopted.

### Claude stream-json

- Supply a holder for Claude workers and make the attach path skip launch-time
  argument/profile reconstruction, as a held lead already does. The current
  worker attach condition still builds `claudeArgs` even when a holder id is
  present (`claude.go:449-471`), so it can fail rejoin on launch-only state that
  the running process no longer needs.
- Replay stream-json output silently: rebuild mapper and turn state, do not
  re-send historical control answers, and use persisted `Answered` ids so a
  permission/control request answered before replacement is not answered a
  second time. A live unanswered request remains answerable by the successor.
- Restore Claude's worker HTTP-MCP token only after holder ownership and child
  PID validation, then preserve the existing interrupt, idle sleep, resume,
  close, and exact-once report behavior.

### agy ACP

- Remove the worker-holder refusal and use `holderChild` for worker ACP just as
  for hosted agy leads, retaining `agyRingSkip`. ACP request ids identify
  initialize/session/prompt/control kinds; replay must distinguish historical
  responses from requests the successor may issue and must not auto-allow the
  same permission request twice (`agy.go:21-43,138-191`).
- Restore the worker HTTP-MCP session token before agy's held ACP process makes
  another tool call. Keep the token out of argv and environment as today; agy
  receives it in its session request, and the record binds it to the holder.
- The agy model-wait watchdog is daemon memory: it loads configuration once at
  process start, arms for an ordinary prompt, pauses for pending tools and
  approvals, interrupts/retries with `continue`, and emits one exhaustion
  diagnostic (`agy_watchdog.go:13-20,59-128,255-405`). Rejoin must reconstruct
  a safe watchdog state from replay/current turn state without turning a
  daemon swap into an extra retry, a premature timeout, or a second final
  report. Coordinate with `impl/develop/agy-thought-pulse`, whose pulse supplies
  held-agy activity without storing thought chunks.

## Lifecycle and process-accounting acceptance

- A running worker record names one holder and one harness child PID. Holder
  PID comes from the holder descriptor/manager and harness PID remains
  `Record.HostPID`; status pages and stop paths must not confuse them or count
  either process twice.
- Successor rejoin validates holder kind, worker backend, reply id, working
  directory where relevant, capability/MCP binding, and the live child PID.
  A mismatch detaches and reports recovery failure without killing a process
  it cannot prove it owns.
- `agent.interrupt` routes to the rejoined process, settles the turn once, and
  leaves the holder/process available for follow-up. Idle timeout stops the
  holder and child but leaves the conversation resumable. Conversation close
  stops the holder and child, revokes delegation and HTTP-MCP tokens, closes
  descendants according to existing rules, and prevents later wake.
- A replacement during a turn keeps the worker `running` while rejoin/replay is
  unresolved. Mail accepted during that window remains queued, then steers the
  recovered turn or starts the next turn according to the backend's existing
  steer capability. It is not submitted into an ambiguous turn.
- On Windows, tests cover restart-safe and explicitly not-restart-safe holder
  status. The latter may end interrupted/failed, but must never be reported as
  a successful survival.

## Compatibility analysis

The source basis below is `develop` at `06e615956`; implementation must repeat
the directional read/write trace against its final base and record the final
tier in `## Decisions` before ready promotion.

- **Agent record shape: candidate T0 if existing fields are reused.** The new
  writer can populate existing `holder_id`, `host_pid`, `holder_seq`,
  `capability_secret`, and the existing MCP tuple for additional backends
  (`record.go:83-96,144-164`). The old reader knows those members and preserves
  unknown top-level members (`record.go:184-238`); the new reader already reads
  legacy direct-worker records whose holder fields are absent. A new field is
  not covered by this verdict and must be traced independently.
- **Holder protocol/descriptor: candidate T0 without a protocol bump.** A new
  worker session kind/backend binding can stay inside the existing opaque
  session-description JSON while launch, input, replay, status, detach, and
  stop use the existing holder protocol (`holder.go:70-150`,
  `internal/wsholder/holder.py:1176-1214`). Older holders keep serving their
  existing protocol; a protocol-version refusal would instead be T3.
- **HTTP-MCP worker-token restore: candidate T0 for new-daemon to new-daemon
  replacement.** Reuse of the existing record tuple and `attachMCP` restore
  path adds no hello field, MCP tool, or tool argument. The successor reads the
  retained token and reconstructs the in-memory hello from the recorded env,
  cwd, and child PID (`record.go:153-164`, `mcp_session.go:88-137`). Token
  rotation is not compatible with an already-running harness that still knows
  only the old token.
- **Legacy direct workers opened by the new daemon: candidate T0.** Their
  records have no holder. A turn cut off by the predecessor is already settled
  interrupted once before queued input runs (`runner_conv.go:511-512`,
  `spawn.go:368-381`); the next actual launch can create the first holder
  without rewriting the backend session id or conversation identity.
- **New held non-Pi worker opened by the current old daemon: provisional T3,
  pending (1849).** The old runner rejoins a non-lead holder only for
  `BackendPi` (`runner.go:727-735`) and the old common path creates a holder
  only for Pi (`runner_process.go:178-190`). It therefore cannot restore the
  held Claude/Codex/agy worker or its HTTP-MCP token, and a later wake can start
  a second daemon-owned process while the holder child still lives. Reopening
  with the new daemon can recover retained record/ring state, but that does not
  make the old daemon's concurrent process behavior T2. Unless implementation
  finds and proves a T0-T2 guard, this is an old-version boundary requiring a
  minor bump and explicit owner approval under
  `ai-docs/manuals/release-compatibility.md`.
- **MCP/tool API: candidate T0.** No `agent.*` tool name/argument, `/mcp` hello,
  profile manifest field, or tracker event is required to change. Existing
  direct-worker tests that assert empty holder/token fields must change, but
  caller syntax and authority remain the same.

## Open Decision Queue

Last ID: (1851)

(1845) [open] [technical] Use one backend-neutral `agent-worker` holder session descriptor carrying backend, reply id, cwd, and token bindings over separate holder kinds per backend - one validated lifecycle keeps common launch/rejoin/stop behavior from drifting; rejected alternative multiplies holder-kind branches and tests.

(1846) [open] [technical] Persist and restore `WorkerHTTPMCP` through the existing `Record.MCPToken/MCPURL/MCPEnv/MCPHolder` tuple after holder ownership validation over rotating the token on successor start - a running Claude/Codex/agy process cannot learn a replacement token, while the existing tuple already binds a token to a holder.

(1847) [open] [technical] Reconcile replay per protocol (Codex fresh request epochs plus `thread/read`, Claude silent historical control answers plus `Answered`, agy ACP kind ids plus permission-answer dedupe) over treating ring replay alone as proof of turn completion - predecessor waiters and control side effects are not reconstructible from raw output alone.

(1848) [open] [technical] Settle an open legacy direct-worker turn interrupted exactly once, then create a holder on its next process launch over attempting to revive a process the predecessor daemon already killed - the record retains the backend session but has no surviving process or holder to prove ownership of.

(1849) [open] [policy] Treat an old daemon opening a live newly-held non-Pi worker as a T3 boundary with a minor bump unless implementation proves a T0-T2 guard, rather than shipping the current old runner's inability to rejoin or restore the worker token as a patch-level downgrade exception.

Response format:

- `approve T3 + minor` - accept that an old daemon must be prevented from serving these records; the owner explicitly approves the T3 boundary and minor bump.
- `require T0-T2` - do not settle the ticket until a source-backed compatible guard or recovery path is designed and verified.

Recommendation: `require T0-T2` while the ticket is in `todo/`; if no such design exists, return with the exact old/new trace and request explicit approval for `T3 + minor` before promotion.

(1850) [open] [technical] Re-arm the agy model-wait watchdog from successor replay/current-turn state with retry count reconstructed or conservatively bounded over resetting a fresh full retry budget on every daemon replacement - replacement must not create duplicate `continue` attempts or defer exhaustion indefinitely, and persisting timer ticks would violate the memory-first performance contract.

(1851) [open] [technical] Keep `agent.interrupt` turn-scoped while idle stop/explicit process stop/close end the holder and revoke its tokens over making interrupt kill the held process - follow-up and resume semantics require the conversation process to remain available after a normal interrupt.

## Phases

### Phase 1: Generalize worker holder ownership and compatibility guards

After settling the Open Decision Queue:

1. Generalize the Pi-only holder allocation/rejoin gate to every spawnable
   backend without adding backend-name branches to common lifecycle code.
   Persist and validate the holder association, harness PID, applied sequence,
   delegation capability, and (where selected by `WorkerHTTPMCP`) HTTP-MCP
   session tuple. Refuse launch/rejoin on an unvalidated association.
2. Preserve transition semantics for records from a direct-worker build:
   settle any unrecoverable open turn exactly once, retain queued input and
   backend session identity, and start the next process under a holder.
3. Make interrupt, idle timeout/sleep-wake, explicit stop, conversation close,
   descendant close, token revocation, and process accounting operate on the
   holder-owned worker without changing caller-visible authority or mailbox
   behavior.
4. Resolve (1849) with directional compatibility fixtures against the exact
   old source. Record which version writes each record/descriptor member,
   which version reads or rewrites it, whether an old daemon can start a
   duplicate process or lose HTTP-MCP service, and what a new reopen recovers.
   A T3 result stops promotion until the owner approves the minor boundary.
5. Verify no new hot-path file operation. Report startup/rejoin and per-turn
   file-operation deltas; measure on the Windows antivirus baseline when the
   implementation changes counts, or state why counts are structurally
   unchanged with test evidence.

### Phase 2: Port Codex, Claude, and agy worker protocols and prove replacement

1. Codex: accept a worker holder, restore HTTP MCP, replay app-server output,
   issue fresh successor request ids, and reconcile an ambiguous turn through
   `thread/read` before releasing input.
2. Claude: accept a worker holder, skip launch-only reconstruction on attach,
   restore HTTP MCP, replay stream-json without duplicate control answers, and
   retain interrupt/follow-up behavior.
3. agy: accept a worker holder, preserve the thought-chunk ring filter and the
   related activity pulse, restore HTTP MCP, deduplicate ACP control answers,
   and make model-wait watchdog retry/exhaustion replacement-safe.
4. Add one provider-free simulated-daemon-replacement test per backend (Codex,
   Claude, agy, and Pi as the regression reference). Each test starts an
   `agent.spawn` worker turn, records its holder/harness PID and session id,
   lets a successor runner rejoin the same holder, completes the turn after
   replacement, and asserts:
   - the harness PID and holder id did not change during replacement;
   - the turn never transiently settles interrupted merely because of the
     replacement;
   - HTTP MCP/delegation remains usable after rejoin where applicable;
   - the parent receives one terminal status and one final report, exactly
     once, including when completion happened while no daemon was attached;
   - queued mail/steer is delivered only after reconciliation; and
   - interrupt, sleep/wake, stop, and close retain their distinct outcomes.
5. Update existing direct-worker tests (`agy_worker_test.go`,
   `codex_worker_test.go`, Claude worker coverage, common contract tests) that
   currently require no holder or persisted token. Keep an explicit legacy
   record fixture proving the direct-worker transition instead of deleting all
   old-shape coverage. Add mismatch tests for holder kind/backend/reply id,
   token binding, and PID reuse, plus Windows restart-safe/not-restart-safe
   cases and agy watchdog replacement during silence, pending tool/approval,
   retry, and exhaustion.

### Phase 3: Update the operations contract and close superseded guidance

1. Reconcile `impl/develop/manual-replace-ownership`, then update
   `ai-docs/manuals/ws-mcp.md` under "Daemon replacement and process ownership"
   and its holder/worker sections: every `agent.spawn` backend is holder-backed;
   successor rejoin/replay and protocol reconciliation; HTTP-MCP token restore;
   Windows restart-safety; worker sleep/wake, interrupt, stop, close, PID
   accounting; and legacy direct-record transition.
2. Update stale comments and tests that say workers are daemon-owned or that a
   daemon replacement necessarily interrupts them. Preserve the historical
   ticket decisions as superseded history; do not rewrite their frozen Result
   sections.
3. Record final T0-T3 verdicts with source-line read/write/reopen evidence and
   the owner approval/minor bump if (1849) settles to T3. Run focused holder,
   runner, backend, MCP, Windows process-control, and Python holder tests, then
   the repository's configured Go verification on the permitted machine.
