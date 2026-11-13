---
title: "pi.spawn subagents outlive a daemon restart under a holder"
---

# pi.spawn subagents outlive a daemon restart under a holder

## Background

Dogfood 2026-11-13: `python scripts/dev.py rerun daemon` interrupted four
mid-turn `pi.spawn` subagents. Each lead received "turn interrupted: the
dashboard daemon exited while the turn ran" and had to re-send work;
holder-hosted leads survived.

Peer evidence (InspectTGV, wsflow 0.49.0): another lead's rerun exited the
machine-wide daemon at 02:11:28Z and 02:14:41Z, interrupting a worker and its
nested leaf. Two builds/dashboard processes and repeated MCP reconnects were
observed. Afterwards `pi.list` said "running", `pi.wait` said "interrupted",
and `pi.send` queued forever; stop/respawn was required. Preserve that
regression without assuming the multiple builds caused it.

### Hosted leads already rejoin held processes

- The detached Python holder owns persistent child pipes, continuously drains
  stdout/stderr into a sequenced bounded ring, and advertises an owner-only
  descriptor containing its loopback endpoint/token. The next daemon
  authenticates, takes the attach slot, replays output and continues the same
  stdin/RPC stream. Detach is not EOF; a live child prevents idle expiry
  (`agents-plugin-tool/internal/wsholder/holder.py:1-75`).
- Dashboard wiring passes the manager to the runner; hosted-session startup
  registers adapters before scanning, and shutdown closes the runner before
  detaching the manager (`agents-plugin-tool/cmd/ws-mcp/dashboard.go:79-92,256-284`,
  `agents-plugin-tool/internal/wsdashboard/daemon.go:421-454,597-604`).
  Claude's backend uses `startChild` without repeating initialize on rejoin;
  the adapter transports JSON lines in holder **pipe** mode
  (`agents-plugin-tool/internal/wsagent/claude.go:316-345`,
  `agents-plugin-tool/internal/wsagent/holder.go:157-197,226-263`).
- The runner persists holder identity/PID and the last wholly applied output
  sequence, then detaches on shutdown. The manager discovers descriptors and
  replays the retained ring (`agents-plugin-tool/internal/wsagent/record.go:66-83`,
  `agents-plugin-tool/internal/wsagent/runner.go:2542-2556`,
  `agents-plugin-tool/internal/wsholder/manager.go:224-240,343-402`). Replayed
  lines rebuild mapper/turn state quietly; new lines apply normally. Input
  waits for scan/replay completion, with missing-holder and wrapped-ring
  handling (`agents-plugin-tool/internal/wsagent/stream.go:164-195`,
  `agents-plugin-tool/internal/wsagent/runner.go:1920-2075`).

### Pi workers bypass that recovery

- Spawn records parent/tier and starts a restricted non-lead process with a
  delegate key/MCP bridge. Pi uses `--mode rpc`, a saved `pi/session.jsonl`,
  and JSONL prompt commands (`agents-plugin-tool/internal/wsagent/spawn.go:58-165`,
  `agents-plugin-tool/internal/wsagent/runner.go:1733-1803`,
  `agents-plugin-tool/internal/wsagent/pi.go:246-285,323-407`). Holding is gated
  on `StartSpec.Lead.Holder`, so workers use daemon-owned `exec.Cmd` pipes
  (`agents-plugin-tool/internal/wsagent/backend.go:61-78`,
  `agents-plugin-tool/internal/wsagent/stream.go:102-161`). Shutdown stops
  workers; reload unconditionally interrupts their open turns
  (`agents-plugin-tool/internal/wsagent/runner.go:408-414,2528-2556`,
  `agents-plugin-tool/internal/wsagent/spawn.go:276-282`). A session file alone
  cannot preserve an in-flight turn.
- List status comes from memory-only `c.turn`/`c.proc`; send persists inbox
  input but `kick` refuses it while running/replaying; wait uses in-memory
  send counters, settle channels and parent-wait registrations
  (`agents-plugin-tool/internal/wsagent/runner.go:678-712,1344-1374,1552-1555`,
  `agents-plugin-tool/internal/wsagent/children.go:691-792`). Persisted state
  includes parent/tier, `TurnOpen`, `LastTurn`, queued input and nested
  pending/received reports; startup seeds the last settle. Active reply,
  covered-send marks, held parent result and steered reports are memory-only
  (`agents-plugin-tool/internal/wsagent/record.go:91-127`,
  `agents-plugin-tool/internal/wsagent/runner.go:383-389,1115-1147`). Repair
  list/send state as well as the wait result.
- Nested capability tokens exist only in memory and are issued per process;
  the surviving MCP bridge would retain its old token and endpoint environment
  (`agents-plugin-tool/internal/wsagent/children.go:18-38,103-151`,
  `agents-plugin-tool/internal/mcp/pi_forward.go:31-79`). Depth is capped at 2
  (`agents-plugin-tool/internal/wsagent/children.go:57-59`).
- Completion needs replay deduplication: settlement is persisted before the
  report callback, but lead delivery is an unconditional mailbox append;
  only conversation parents have pending-report entries
  (`agents-plugin-tool/internal/wsagent/spawn.go:229-273`,
  `agents-plugin-tool/internal/mcp/pi_tools.go:490-511`,
  `agents-plugin-tool/internal/wsmailbox/replyid.go:150-164`). Nested handoff
  removes pending reports before acceptance, while steering and parent-result
  holds also require recovery
  (`agents-plugin-tool/internal/wsagent/children.go:423-461,488-633`).

## Decisions

- priority: top (owner 2026-11-13). This blocks all other Windows work.
- **Owner direction, 2026-11-13:** isolate Pi subagent process lifetime from
  the dashboard daemon as for a holder-hosted Claude lead. Dev reruns and
  release replacements rejoin the same running turn. **Rejected:** merely
  warning/refusing restarts while turns run, including other projects' turns.
- **One bounded Phase 1, one worker:** land holder/rejoin behavior and the
  small dead-process reconciliation together. A stale-state-only patch does
  not remove the Windows blocker. Reuse existing holder/replay infrastructure;
  no generic process-supervision, mailbox or dashboard redesign, exhaustive
  fault-matrix project, or separate preliminary phase.
- **Pi RPC fits the holder:** hosted Pi leads already use it
  (`agents-plugin-tool/internal/wsagent/pi.go:364-393`). Move holder selection
  out of `LeadSpec` so spawned Pi workers can use it without becoming leads.
  Preserve restricted tools/instructions, delegate keys and MCP bridges.
  Use a distinct `pi-worker` holder kind for ownership/orphan cleanup; leave
  unrelated non-lead backends alone.
- Rejoin recorded workers before scanning/releasing input. Persist holder
  identity, applied sequence, active turn/send coverage, reply and held-parent
  result needed to resume. Do not interrupt a live held `TurnOpen`, repeat its
  prompt or launch a second process. Map worker `agent_start` too (currently
  lead-only: `agents-plugin-tool/internal/wsagent/pi.go:690-695`). Rebuild old
  lines quietly, apply new lines once, and settle on `agent_settled`, not
  prompt acceptance/`agent_end`; retain immediate refused/handled-no-run cases.
- Keep holder protocol 1 and its existing bounded ring. Reuse partial-line,
  replay-watermark and `get_state` probing. After a gap reconcile with Pi's
  saved session/RPC messages; if the final result cannot be reconstructed,
  report a specific recovery failure once, never fabricated success or an
  indefinite idle-process hold. A failed probe cannot justify another process.
- Preserve depth-2 children and their parent/tier restrictions. Persist the
  capability secret in the owner-only agent record; validate its held-process
  association and restore token lookup before accepting nested calls. Revoke
  on actual process end/close, not daemon detach. Forward calls discover the
  current endpoint instead of retaining an obsolete port. Reissue disconnected
  waits; never blindly retry side-effecting spawn/send calls.
- Scope delivery changes to **Pi completion reports**: stable persisted
  turn/report IDs, a retryable pending report for either parent kind, and
  recipient deduplication before acknowledging handoff. Mail deduplication
  must survive mail claim/drain. Recover nested acceptance/steering and parent
  holds; retain wait-vs-report ownership, with lead waits still leaving one
  completion mail. This is not a general mailbox exactly-once API project.
- If the process is genuinely gone, settle interrupted once, show **idle with
  the interruption error**, clear stale turn/process/replay flags, then drain
  queued input. `pi.send` starts a new turn on the saved session. Do not clear
  a genuinely new queued turn, or mistake a delayed scan/unattached live holder
  for a dead process. Preserve explicit stop and close-descendants-first.
- **Compatibility: T2.** The recovery contract changes non-additively:
  `turn_open` no longer means interrupt-on-load, and durable turn/report/token
  state must survive writers. Existing struct round-trips discard unknown
  fields (`agents-plugin-tool/internal/wsagent/record.go:195-226`), so optional
  fields alone are not T0. Raise agent `schema_version` from 1 to 2 with
  newer-schema write refusal; migrate unheld legacy records through
  interrupted/idle recovery. Version/guard any non-additively changed mailbox
  state likewise. Land only in a minor bump; unsupported old downgrade
  writers are not a supported recovery path. No holder protocol change.

## Constraints

- Non-UI implementation runs through `ws/pi.*` workers. No UI/restart guard;
  use isolated daemon state for tests, not a restart of the shared daemon.
- Applicable `AGENTS.md` Implementation Conventions rows; read each manual
  before editing matching paths:

  | paths | manual |
  |---|---|
  | `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/` | `ai-docs/manuals/shipped-surface-boundary.md` |
  | `agents-plugin-tool/internal/mcp/` | `ai-docs/manuals/ws-mcp.md` |
  | `agents-plugin-tool/internal/` | `ai-docs/manuals/release-compatibility.md` |

- Scope: `agents-plugin-tool/internal/wsagent/`, Pi report/forwarding code in
  `internal/mcp/`, narrowly needed `internal/wsmailbox/` deduplication, holder
  integration/tests in `internal/wsholder/`, and daemon wiring/tests only as
  needed. Keep shipped output downstream-neutral and secrets out of logs/mail.
- Builds/tests run on `sys_lock` remote **w5090**, holding its resource lock.
  Windows holder behavior also requires a **local Windows check**: job
  breakaway is requested, refusal retries inside the job and advertises
  `restart_safe: false` (`agents-plugin-tool/internal/wsholder/spawn_windows.go:12-32`,
  `agents-plugin-tool/internal/wsholder/manager.go:583-588`). Do not hide that
  limitation or treat remote Linux results as Windows isolation evidence.
- Preserve transcript/session identity, delegate restrictions, explicit
  stop/interrupt and existing Claude/Pi hosted-lead recovery. The lead runs
  the design gate and promotes todo -> ready before implementation.

## Phases

### Phase 1: Pi workers and nested children rejoin after daemon replacement

- **Outcome:** one worker lands the existing-holder integration, minimal
  durable turn/report/token recovery, and stale-state cleanup described in
  Decisions as one reviewable slice. Normal shutdown detaches workers; the
  successor rejoins the same processes across sessions/projects, consumes
  remaining RPC output, and reports each completed turn once. A turn ending
  during downtime is recovered. No stale-state-only milestone or prerequisite
  refactor; retain the current holder protocol and profile boundaries.
- **Go acceptance tests:**
  - `TestPiDaemonRestartMidTurnCompletesWithOneCompletionMail`: real holder
    plus deterministic fake Pi RPC child; replace an isolated daemon mid-turn,
    assert identical holder/child PID and session, final reply via wait and
    exactly one mail, no repeated prompt/interruption. Include abrupt daemon
    loss, completion while detached and another restart after delivery.
  - `TestPiNestedDaemonRestartRejoinsBothDepths`: worker and leaf survive;
    retained credentials/endpoint discovery permit list/wait/send and legal
    spawns after rejoin, depth-2 spawn remains refused, leaf completion reaches
    its parent once, held parent result releases, and explicit stop closes
    descendants. Cover an endpoint-port change and both parent load orders.
  - `TestPiProcessGoneIsIdleAndSendStartsNewTurn`: missing holder/exited child
    yields wait interrupted, list idle/error and an actually started follow-up
    turn, not merely queued text. Cover delayed scans, queued next input and
    repeated reload without duplicate interruption. Retain legacy/unheld
    coverage from `TestRestartSettlesTheCutTurnBeforeTheNextStarts` and
    `TestDaemonExitReportsTheCutTurnAsInterrupted`.
- **Focused regressions:** partial/gapped replay plus idle `get_state` probe;
  final reply and held-parent recovery; report restart immediately before/
  after handoff, including claimed mail and nested steering; schema-1
  migration/schema-2 reload/newer-schema write refusal; worker tool restrictions
  and unchanged hosted-lead recovery. Extend existing fixtures rather than
  creating a generic new test framework.
- **Verification:** under the w5090 lock, run the named tests repeatedly,
  `go test -race ./internal/wsagent ./internal/wsholder ./internal/mcp ./internal/wsmailbox`,
  `go test ./...` and `go build ./cmd/ws-mcp` from `agents-plugin-tool/`.
  Locally on Windows, use isolated daemon/holder state to check a mid-turn
  replacement, same process identities, one completion mail, nested reconnect,
  and breakaway/fallback reporting. Record revision, platform, commands and
  results; missing Windows evidence remains an explicit acceptance gap.
