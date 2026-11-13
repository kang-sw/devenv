---
title: "pi.spawn subagents outlive a daemon restart under a holder"
---

# pi.spawn subagents outlive a daemon restart under a holder

## Background

Dogfood 2026-11-13: `python scripts/dev.py rerun daemon` interrupted four
mid-turn `pi.spawn` subagents. Each lead received "turn interrupted: the
dashboard daemon exited while the turn ran" and had to re-send the work.
Hosted leads survived the same restart.

Peer evidence (InspectTGV, wsflow 0.49.0, 2026-11-13): another lead's rerun
exited the machine-wide daemon at 02:11:28Z and 02:14:41Z, interrupting a Pi
worker and its nested leaf. Two ws-mcp builds and two dashboard processes
were observed, with repeated MCP reconnects. Afterwards `pi.list` said
"running", `pi.wait` said "interrupted", and `pi.send` queued indefinitely;
`pi.stop` and respawn were required. Preserve this as an acceptance regression,
not as proof that the multiple builds caused the stale state.

### Existing holder and hosted-lead recovery

- `agents-plugin-tool/internal/wsholder/holder.py:1-75` defines the existing
  transport: a detached Python holder owns one child, keeps stdin open and
  continuously drains stdout/stderr into a sequenced bounded ring. Its
  owner-only descriptor supplies the loopback port and attach token. The
  successor authenticates, takes the single attach slot, requests replay,
  then receives live output; `input` writes to the same child's stdin.
  Disconnecting is not EOF. A live child prevents the holder's idle exit.
- `agents-plugin-tool/cmd/ws-mcp/dashboard.go:79-92,256-284` constructs one
  holder manager and passes it to the agent runner. The hosted session code
  installs the endpoint and starts the runner before holder scanning;
  shutdown closes the runner before detaching the manager
  (`agents-plugin-tool/internal/wsdashboard/daemon.go:421-454,597-604`).
- A Claude lead's `startLead` builds a `holderChild` and records its identity
  and PID (`agents-plugin-tool/internal/wsagent/runner.go:1808-1896`).
  `ClaudeBackend.Start` routes stream-json through `startChild`, without
  re-sending initialize on attachment
  (`agents-plugin-tool/internal/wsagent/claude.go:316-345`). The holder spawns
  it in **pipe**, not PTY, mode; the adapter splits replayed stdout into the
  same JSON lines and sends input through the holder
  (`agents-plugin-tool/internal/wsagent/holder.go:157-197,226-263`).
- On shutdown, a lead detaches and persists its applied sequence rather than
  closing its process (`agents-plugin-tool/internal/wsagent/runner.go:2542-2556`);
  manager close only drops connections
  (`agents-plugin-tool/internal/wsholder/manager.go:224-240`). On startup,
  conversation adapters register before the first scan
  (`agents-plugin-tool/internal/wsagent/runner.go:208-264,397-402`). The manager
  discovers descriptors, authenticates and replays the ring; its own cursor
  is memory-only, so a new manager requests the whole retained ring
  (`agents-plugin-tool/internal/wsholder/manager.go:343-402`).
- Persisted `HolderID`, `HostPID`, `HolderSeq` and answered permission IDs
  distinguish replay from newly applied output
  (`agents-plugin-tool/internal/wsagent/record.go:66-83`). Replay rebuilds
  mapper and turn state with side effects suppressed for already-applied
  lines (`agents-plugin-tool/internal/wsagent/stream.go:164-195`). The runner
  waits for the scan and replay watermark, handles missing holders, and
  only then releases queued input. Lost ring boundaries conservatively hold
  input, with a Pi `get_state` probe when available
  (`agents-plugin-tool/internal/wsagent/runner.go:1920-2075`). This is process
  reattachment, not merely starting another process on a saved session.

### Current Pi workers and state ownership

- `Runner.Spawn` creates a non-lead conversation with parent/tier metadata
  (`agents-plugin-tool/internal/wsagent/spawn.go:58-111,116-165`). Its start
  composes the restricted profile, delegate ws key and MCP bridge
  (`agents-plugin-tool/internal/wsagent/runner.go:1733-1803`). Pi runs
  `--mode rpc` on its persisted `pi/session.jsonl`; prompts are JSONL RPC
  commands, not command-line prompts
  (`agents-plugin-tool/internal/wsagent/pi.go:121-124,246-285,323-362,395-407`).
- Holding is currently conditional on `StartSpec.Lead.Holder`
  (`agents-plugin-tool/internal/wsagent/backend.go:61-78`,
  `agents-plugin-tool/internal/wsagent/stream.go:102-161`). Workers therefore
  use daemon-owned `exec.Cmd` pipes. Shutdown's `letGo` preserves only leads
  and calls `stopProcess` for workers; closing stdin ends the RPC process,
  with termination fallback. Reload unconditionally settles a parent's open
  turn as interrupted (`agents-plugin-tool/internal/wsagent/runner.go:408-414,2528-2556`,
  `agents-plugin-tool/internal/wsagent/spawn.go:276-282`). Saving the Pi
  session does not preserve an in-flight turn.
- `pi.list` uses the runner's in-memory `c.turn` and `c.proc`, not the last
  persisted settle (`agents-plugin-tool/internal/wsagent/runner.go:1344-1374`).
  `pi.send` persists inbox input, but `kick` refuses it while `c.turn` or
  replay is set (`agents-plugin-tool/internal/wsagent/runner.go:678-712,1552-1555`).
  `pi.wait` uses memory-only send counters, settle channels and parent-wait
  registrations (`agents-plugin-tool/internal/wsagent/children.go:691-792`).
  These separate state paths explain why fixing only the wait result cannot
  establish the list/send invariant; reproduce the peer inconsistency in tests.
- This is **not** an entirely memory-only spawn store: `Record` persists
  parent/tier, `TurnOpen`, `LastTurn`, queued input and pending/received
  nested reports; startup seeds the last settle from it
  (`agents-plugin-tool/internal/wsagent/record.go:91-127`,
  `agents-plugin-tool/internal/wsagent/runner.go:383-389`). In-flight reply,
  send/settle marks, held parent result, steered reports and waiters are
  memory-only (`agents-plugin-tool/internal/wsagent/runner.go:1115-1147`).
- Nested children have a second restart hazard: their capability token is
  issued per process into an in-memory map and never persisted
  (`agents-plugin-tool/internal/wsagent/children.go:18-38,103-151`). The held
  MCP bridge would retain that old token and fixed endpoint environment;
  forwarding reads both directly on each call
  (`agents-plugin-tool/internal/mcp/pi_forward.go:31-79`). A surviving process
  alone cannot restore its `pi.*` authority.
- Completion is not yet a durable exactly-once handoff. `publish` persists a
  settle before invoking `Options.Report`, but only conversation parents
  get pending-report entries
  (`agents-plugin-tool/internal/wsagent/spawn.go:229-273`). Lead reports use
  an unconditional mailbox append, with failures merely logged
  (`agents-plugin-tool/internal/mcp/pi_tools.go:490-511`,
  `agents-plugin-tool/internal/wsmailbox/replyid.go:150-164`). Nested delivery
  removes the child's pending reports before handing them to the parent;
  steering/held-result state also needs recovery
  (`agents-plugin-tool/internal/wsagent/children.go:423-461,488-633`).

## Decisions

- **Owner direction, 2026-11-13:** isolate a `pi.spawn` subagent's process
  lifetime from the dashboard daemon, as for a holder-hosted Claude lead.
  A dev rerun or release replacement rejoins the same running turn.
  **Rejected:** merely warning about or refusing restarts while turns run;
  the daemon is machine-wide and workers in other sessions/projects count.
- **The holder fits Pi RPC.** It transports bytes over persistent pipes;
  hosted Pi leads already use it (`agents-plugin-tool/internal/wsagent/pi.go:364-393`).
  Reuse that transport and replay machinery. Separate holding from
  `LeadSpec`: a held worker must retain its restricted tools/instructions,
  delegate key and MCP bridge, never acquire lead permissions or ambient
  user configuration. Give worker holders a distinct `pi-worker` kind for
  ownership/orphan cleanup. Scope the change to spawned Pi conversations and
  existing lead behavior; do not change every non-lead backend's lifetime.
- Persist and restore holder identity, applied-output watermark and the
  active turn's identity/covered-send marks before accepting list/send/wait
  operations. Reconstruct worker turn starts too: Pi's mapper currently emits
  `agent_start` only for leads (`agents-plugin-tool/internal/wsagent/pi.go:690-695`).
  Rejoin before scanning; do not auto-interrupt a held `TurnOpen`, re-submit
  its prompt, or start a second process. Replay old lines quietly, apply new
  lines once, and settle a running turn at `agent_settled` (not `agent_end`
  or prompt acceptance); preserve immediate settlement for a refused or
  handled prompt that starts no run. Recover the final reply and parent-result hold,
  not only the running boolean.
- Retain the existing bounded-ring transport/protocol. Handle partial lines,
  ring gaps and a turn that finished while detached. After a gap, query Pi
  `get_state` and reconcile the active turn with the saved Pi session/RPC
  message history; never interpret a missing boundary as success or wait
  forever for an idle process. If a result truly cannot be reconstructed,
  expose a specific recovery failure once rather than inventing completion.
  A failed probe must not authorize a second process or a duplicate prompt.
- Preserve nested authority for the **same held process**: persist its
  capability secret in the owner-only agent record, restore token lookup
  before serving requests, and revoke on actual process end/close, not on
  daemon detach. Validate the holder/process association before restoring
  authority. Resolve the current daemon endpoint from its discovery file
  for forwarded calls rather than relying on the inherited port. Preserve
  parent ownership, tier caps, depth 2 and close-descendants-first semantics.
  A disconnected HTTP wait is reissued, not restored as a live goroutine;
  reconnect must not blindly retry a side-effecting spawn/send.
- Use a stable persisted turn/report ID and a durable report outbox for both
  parent kinds. Accept report IDs idempotently at the recipient, then
  acknowledge delivery. Mailbox deduplication receipts must survive claiming
  or draining mail, not just inspect the current queue. Persist nested
  acceptance/steering and held-parent-result state, reconcile against replay,
  and preserve the existing wait-vs-report ownership rule. Daemon failure
  before/after either side of a handoff must not lose or repeat completion.
  A lead's wait remains a fallback and does not suppress its completion mail.
- A genuinely gone process is **idle with an interrupted result/error**,
  never phantom-running. Clear stale process/turn/replay flags consistently;
  settle the old turn once before draining queued input. `pi.send` then
  starts a fresh turn on the same saved Pi session. A live but temporarily
  unattached holder is not "gone"; wait for scanning/reattachment evidence.
- **Compatibility: T2 overall.** Phase 1 is an internal stale-state repair;
  Phase 2 changes the on-disk recovery contract: `turn_open` no longer means
  "interrupt on load", and turn/report delivery, capability and parent-hold
  state become durable rather than disposable. Older writers discard fields
  they do not know (`agents-plugin-tool/internal/wsagent/record.go:195-226`),
  so optional fields alone do not justify T0. Raise the agent record's
  `schema_version` from 1 and add newer-schema write refusal; migrate legacy
  records without holders through interrupted/idle recovery. Apply the same
  version/write-guard discipline to any non-additively changed mailbox store.
  Land the breaking slice only in a minor bump under
  `ai-docs/manuals/release-compatibility.md`; unsupported downgrade writers
  are not a supported recovery path. Keep holder protocol 1 unchanged and
  its v/v-1 attach window intact; any necessary non-additive protocol change
  must explicitly raise its number and remain T2, not silently alter replay.

## Constraints

- Non-UI implementation runs through `ws/pi.*` workers. This ticket adds no
  UI and no restart guard; do not restart the shared dogfood daemon merely
  to exercise tests. Use isolated test daemons/state directories.
- Applicable `AGENTS.md` Implementation Conventions rows (read before editing
  matching paths):

  | paths | manual |
  |---|---|
  | `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/` | `ai-docs/manuals/shipped-surface-boundary.md` |
  | `agents-plugin-tool/internal/mcp/` | `ai-docs/manuals/ws-mcp.md` |
  | `agents-plugin-tool/internal/` | `ai-docs/manuals/release-compatibility.md` |

- Expected implementation scope: `agents-plugin-tool/internal/wsagent/`,
  `internal/mcp/`, `internal/wsmailbox/`, holder integration/tests under
  `internal/wsholder/`, and daemon wiring/tests in `internal/wsdashboard/`
  or `cmd/ws-mcp/` only as needed. Keep shipped strings downstream-neutral;
  no ticket/manual paths from this repository in tool output.
- Builds and Go tests run on the `sys_lock` remote **w5090**, with the remote
  resource lock held; record commands, revision and results. Windows-specific
  holder behavior also needs a **local Windows check**. Holder starts request
  job breakaway, retry without it and advertise `restart_safe: false` when
  refused (`agents-plugin-tool/internal/wsholder/spawn_windows.go:12-32`,
  `agents-plugin-tool/internal/wsholder/manager.go:583-588`). Do not conceal
  this limitation or claim a Linux run verifies Windows job isolation.
- Retain explicit `pi.stop`/close/interrupt behavior, transcript history,
  session IDs, delegate restrictions, nested wait delivery semantics and
  existing Claude/Pi hosted-lead recovery. No secret token in tool output,
  logs or completion mail. No unrelated harness or dashboard redesign.

## Phases

### Phase 1: Reconcile dead-process state before accepting another turn

Independent defensive fix; Phase 2 depends on it. This phase does not claim
that running Pi work survives a restart.

- Outcome: legacy/unheld records whose process is gone settle interrupted
  once, show idle with an actionable error, and have no stale running or
  replay flag blocking sends. Preserve a genuinely new turn started from
  queued input after that settlement; do not clear it as part of old-turn
  cleanup. No synthetic interruption for an already settled/closed record.
- Go tests: add `TestPiProcessGoneIsIdleAndSendStartsNewTurn` (seed the peer's
  contradictory list/wait state, assert wait interrupted, list idle and a
  follow-up actually reaches a newly started process); cover queued input,
  parent-before-child and child-before-parent load order, repeated reloads
  and one interruption report. Extend
  `TestRestartSettlesTheCutTurnBeforeTheNextStarts` in `children_test.go`;
  retain the legacy behavior covered by
  `TestDaemonExitReportsTheCutTurnAsInterrupted` in `spawn_test.go`.
- Verification: under the w5090 lock, run the focused recovery tests and
  `go test ./internal/wsagent ./internal/mcp` from `agents-plugin-tool/`.
  Record the observed list/wait/send transition; a successful queue append
  alone does not prove a new turn starts.

### Phase 2: Hold and rejoin spawned Pi turns, including their children

- Outcome: implement the Decisions' worker transport, schema migration,
  replay/turn recovery, durable report handoff and nested-token recovery.
  Shutdown detaches held workers; new-daemon startup rejoins all projects'
  workers before releasing input. A turn finishing during downtime is
  recovered and reported once, even across a second restart. Keep explicit
  stop destructive and sweep orphan **worker** holders without sweeping
  another holder consumer's processes.
- Go tests: add `TestPiDaemonRestartMidTurnCompletesWithOneCompletionMail`
  using the real holder and a deterministic fake Pi RPC executable. Start a
  turn, replace an isolated daemon, assert the same holder/child PID and
  session, consume the remaining RPC output, get the final answer through
  wait, and count exactly one completion mail. Assert no extra prompt or
  interrupted result. Repeat for normal shutdown and abrupt daemon loss,
  completion during downtime, a second restart, and separate parent roots.
- Go tests: add `TestPiNestedDaemonRestartRejoinsBothDepths`: lead -> worker
  -> leaf survives, the retained token can list/wait/send/spawn at permitted
  depth after rejoin (including an endpoint-port change), the leaf reports
  once, the worker's held report releases correctly, and a depth-2 caller
  still cannot spawn. Cover running/idle parent, steering pending/consumed,
  reissued waits, closed parent and stop-descendants-first.
- Go tests: extend `TestPiProcessGoneIsIdleAndSendStartsNewTurn` for a missing
  holder, holder retained with exited child, and process loss during replay;
  distinguish these from a delayed first scan or temporarily unattached
  live holder. Add partial-line, wrapped-ring/get_state, idle-probe race,
  final-reply recovery and non-lead tool-surface regression fixtures.
- Go tests: inject failures before/after settle persistence, outbox enqueue,
  mailbox append/claim and delivery acknowledgement, plus nested recipient
  acceptance/steering. Assert no missing/duplicate report and no stale wait
  target after queued sends. Add schema-1 migration, schema-2 reload,
  newer-schema write refusal and unchanged holder-v1 compatibility tests.
  Existing tests expecting interruption must remain for genuinely unheld or
  dead legacy processes, not be deleted wholesale.
- Verification: with the w5090 lock, run focused cases repeatedly, race-test
  `./internal/wsagent ./internal/wsholder ./internal/mcp ./internal/wsmailbox`,
  then `go test ./...` and `go build ./cmd/ws-mcp` from `agents-plugin-tool/`.
  Re-run hosted Claude/Pi lead rejoin tests. On local Windows use isolated
  daemon/holder state to check mid-turn replacement, actual process identity,
  one completion mail, nested reconnect and job breakaway/fallback reporting.
  Record platform, commands and evidence; remote-only verification leaves
  the Windows acceptance check outstanding. The lead performs the design
  gate and promotes this ticket from todo to ready before implementation.
