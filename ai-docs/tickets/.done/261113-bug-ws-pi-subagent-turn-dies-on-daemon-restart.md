---
title: "pi.spawn subagents outlive a daemon restart under a holder"
completed: 2026-11-13
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 4c674ce7b8197ff6
sage-review-completeness-reviewed: 4c674ce7b8197ff6
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

- Priority: top (owner 2026-11-13, blocks all Windows work)
- **Owner direction, 2026-11-13:** isolate Pi subagent process lifetime from
  the dashboard daemon as for a holder-hosted Claude lead. Dev reruns and
  release replacements rejoin the same running turn. **Rejected:** merely
  warning/refusing restarts while turns run, including other projects' turns.
- **Lead scope cuts, 2026-11-13:** simplest correct implementation for speed.
  Phase 1 is the small stale-state fix; Phase 2 reuses holder/rejoin and the
  existing publish/settle path. No new outbox, mailbox deduplication receipts,
  recipient-side idempotent report IDs or generic delivery redesign. Accept
  the documented downgrade consequence and one possible duplicate completion
  mail at the append/mark crash boundary; do not expand the worker test matrix.
- **Pi RPC fits the holder:** hosted Pi leads already use it
  (`agents-plugin-tool/internal/wsagent/pi.go:364-393`). Move holder selection
  out of `LeadSpec` so spawned Pi workers can use it without becoming leads.
  Preserve restricted tools/instructions, delegate keys and MCP bridges.
  Use a distinct `pi-worker` holder kind for ownership/orphan cleanup; leave
  unrelated non-lead backends alone.
- Rejoin recorded workers before scanning/releasing input. Reuse optional
  holder identity/applied-sequence fields and reconstruct turn/send coverage,
  reply and parent holds from existing records and replay, persisting only
  what rejoin strictly needs. Do not interrupt a live held `TurnOpen`, repeat
  its prompt or launch a second process. Map worker `agent_start` too
  (currently lead-only: `agents-plugin-tool/internal/wsagent/pi.go:690-695`).
  Rebuild old lines quietly, apply new lines once, and settle on
  `agent_settled`, not prompt acceptance/`agent_end`; retain immediate
  refused/handled-no-run cases.
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
- Reuse `publish`/settle and the existing completion-mail append. Persist the
  **last reported turn ID** on the agent record so rejoin replay does not
  normally report a completed turn twice. Mark it after reporting: a daemon
  crash exactly between mailbox append and that mark may duplicate **one
  completion mail**, explicitly accepted. This is not an exactly-once
  guarantee. Keep existing persisted nested pending-report entries and
  wait-vs-report ownership; add no steering persistence beyond what rejoin
  strictly needs. Lead waits still leave the normal completion mail.
- If the process is genuinely gone, settle interrupted once, show **idle with
  the interruption error**, clear stale turn/process/replay flags, then drain
  queued input. `pi.send` starts a new turn on the saved session. Do not clear
  a genuinely new queued turn, or mistake a delayed scan/unattached live holder
  for a dead process. Preserve explicit stop and close-descendants-first.
- **Additive implementation, no schema bump:** holder identity, applied
  sequence, last reported turn ID and capability secret are optional record
  fields; reuse existing ones where present. Keep `schema_version` unchanged.
  A legacy record, or one rewritten by an older daemon that dropped those
  fields, takes Phase 1's process-gone -> interrupted once -> idle fallback.
  **Accepted downgrade consequence:** an older daemon interrupts the turn and
  leaves its held worker process orphaned until the newer daemon's
  `pi-worker` orphan sweep. Do not claim downgrade preserves running work.
- **Compatibility: retain T2 under the current manual, despite the requested
  T0 classification.** Scenarios 1/2 introduce no removed tools/arguments;
  scenario 3 needs no tool-list change, so reconnect alone does not force T1.
  The actual conflict is scenario 4's older-writer field loss
  (`ai-docs/manuals/release-compatibility.md:51`) and the explicit T0 condition
  that read-modify-write preserves unknown fields (`:58-60`). Existing record
  round-trips drop them (`agents-plugin-tool/internal/wsagent/record.go:195-226`).
  Optional fields plus an accepted fallback do not meet that written condition.
  No schema/protocol bump or compatibility-guard work is assigned to this
  worker slice.
- **Owner release-policy exception, 2026-11-13 (whiteboard (37)=a): ship as
  T0, no `schema_version` bump, patch release allowed.** Reason: the shipped
  0.49 daemon has no newer-schema guard, so a bump would protect nothing on a
  downgrade, and the downgrade outcome (in-flight turn interrupted) equals
  today's behaviour. Condition: this change also makes the agent record's
  read-modify-write preserve fields the writer does not know (round-trip the
  unknown JSON members in `record.go`), so later additive fields meet the
  manual's T0 condition. Rejected: T2 with a schema bump and a 0.50 minor.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
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
  `internal/mcp/`, holder integration/tests in `internal/wsholder/`, and daemon
  wiring/tests only as needed. No mailbox-store changes. Keep shipped output
  downstream-neutral and secrets out of logs/mail.
- Builds/tests run on `sys_lock` remote **w5090**, holding its resource lock.
  The **local Windows live check belongs to the lead/owner, not the worker
  gate**. Check job breakaway/fallback: refusal retries inside the job and
  advertises `restart_safe: false`
  (`agents-plugin-tool/internal/wsholder/spawn_windows.go:12-32`,
  `agents-plugin-tool/internal/wsholder/manager.go:583-588`). Do not hide that
  limitation or treat remote Linux results as Windows isolation evidence.
- Preserve transcript/session identity, delegate restrictions, explicit
  stop/interrupt and existing Claude/Pi hosted-lead recovery. The lead runs
  the design gate and promotes todo -> ready before implementation.

## Prior Decisions

- unavailable: wsflow/rationale.query was not available to this session; prior decisions require a separate query.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | `agents-plugin-tool/internal/wsagent/`, `internal/mcp/`, `internal/wsholder/`, and daemon wiring and tests |
| scope.surface | cross-module | Worker recovery spans wsagent, holder transport, MCP forwarding, and dashboard wiring |
| scope.new_public_symbol | unknown | The phases do not specify whether implementation needs exported Go symbols |
| scope.new_type_contract | yes | Optional agent record fields and unknown JSON member preservation change the persisted record contract |
| scope.test_surface | existing | `agents-plugin-tool/internal/wsagent/spawn_test.go` and `children_test.go`; new named scenarios are planned |
| complexity.reuse_points | confirmed | `agents-plugin-tool/internal/wsagent/stream.go` uses `startChild` and holder replay; `spawn.go` implements `publish` |
| complexity.side_effect_risk | high | Restart recovery affects live processes, input, nested calls, and completion delivery |
| risk.correctness | high | Replay and settlement must not lose turns, restart live processes, or strand queued input |
| risk.fit | high | Recovery must fit existing holder, nested capability, and delivery lifecycles without a redesign |
| risk.test | high | Cross-restart, nested process, downgrade and Windows behavior need distinct evidence |
| risk.security_or_contract | high | Persisted capability secrets, endpoint changes and cross-version records affect security and compatibility |

## Phases

### Phase 1: Reconcile dead-process state before accepting another turn

Independent small defensive fix; Phase 2 depends on it. This phase does not
claim that running Pi work survives a restart.

- **Outcome:** legacy/unheld records whose process is gone settle interrupted
  once, show idle with the interruption error, and clear stale process/turn/
  replay flags. Settle before draining queued input; never clear the genuinely
  new turn that input starts. Already settled/closed records get no synthetic
  interruption. A subsequent `pi.send` actually starts a new turn.
- **Go tests:** `TestPiProcessGoneIsIdleAndSendStartsNewTurn` reproduces the
  contradictory list/wait state and verifies idle/error plus an actually
  started follow-up, not merely a queue append. Preserve queued-next-turn
  ordering in `TestRestartSettlesTheCutTurnBeforeTheNextStarts` and legacy
  interruption coverage in `TestDaemonExitReportsTheCutTurnAsInterrupted`.
- **Verification:** under the w5090 lock, run the focused tests and
  `go test ./internal/wsagent ./internal/mcp` from `agents-plugin-tool/`;
  record the observed list/wait/send transition.

### Result (744a5241f) - 2026-11-13

- Reconciliation clears stale turn/process/replay state and exposes the
  interruption error while idle, before queued input can start a new turn.
  Startup waits for reconciliation so an earlier LastTurn cannot answer a
  wait for the cut turn.
- Observed contract: completed A -> open B -> restart -> wait interrupted,
  list idle/error/no process -> send C actually submitted -> wait done.
  A settled record gets no further synthetic interruption.
- w5090 resource lock: focused process-gone, queued-next-turn ordering and
  legacy interruption tests passed; `go test ./internal/wsagent ./internal/mcp`
  passed, then passed again after R1's startup-barrier fix. R2 fixes-only
  review resolved the single substantive finding.
- No Forwards. Shared daemon and Windows live behavior not exercised.
- Workflow/route/convention read tools were unavailable in this session;
  read bundled ticket conventions and used the supplied branch and native git
  under the lead's wrong-root safety override. Review performed independently
  through Pi.

### Phase 2: Rejoin held Pi workers and their nested children

- **Outcome:** reuse the hosted Pi lead holder transport with a distinct
  `pi-worker` kind while retaining worker tools, delegate key and MCP bridge.
  Shutdown detaches; the successor rejoins across sessions/projects before
  releasing input. Restore nested tokens and discover the current endpoint.
  Replay the remaining turn and use existing publish/settle plus the last
  reported turn marker; retain the accepted append/mark duplicate-mail window.
  Sweep orphan worker holders without affecting other holder consumers.
- **Go tests, bounded to these scenarios:**
  - `TestPiDaemonRestartMidTurnCompletesWithOneCompletionMail`: real holder
    and deterministic fake Pi RPC child; normal shutdown mid-turn and turn
    finished during downtime. Assert the same holder/child PID and session,
    final reply via wait, one completion mail, no repeated prompt/interruption.
    Add **one** abrupt-daemon-loss case; no repeat/second-restart matrix.
  - `TestPiNestedDaemonRestartRejoinsBothDepths`: worker and leaf rejoin;
    nested `pi.*` works after an endpoint-port change, and depth-2 spawn
    remains refused. Preserve existing pending-report/parent-hold behavior.
  - Extend `TestPiProcessGoneIsIdleAndSendStartsNewTurn` for a missing holder
    and a holder whose child exited: interrupted once -> idle/error -> send
    starts a new turn. A not-yet-complete scan is not evidence of process loss.
  - Legacy record fallback: omit holder fields, including a fixture rewritten
    by the old record shape that drops optional recovery fields; verify Phase
    1 behavior with unchanged schema. Confirm the newer `pi-worker` sweep
    cleans the orphan left by the documented downgrade.
  - Record round-trip keeps unknown members: load a record carrying a field
    this code does not know, save it, and the field is still there.
- **Verification:** under the w5090 lock, run those focused cases,
  `go test ./internal/wsagent ./internal/wsholder ./internal/mcp` and
  `go build ./cmd/ws-mcp` from `agents-plugin-tool/`; record revision/commands/
  results. No new fault-injection matrix or race-test sweep. The lead/owner
  separately performs the local Windows live replacement check (process
  identity, completion mail, nested reconnect and breakaway/fallback); that
  check is not a worker landing gate.

### Result (184452a15) - 2026-11-13

- Restricted Pi workers now use distinct `pi-worker` holders without lead
  privileges. Shutdown detaches; rejoin preserves holder/child PID, backend
  session and delegate identity, rebuilds quiet output and send coverage, and
  settles at `agent_settled` without repeating the prompt. Orphan sweeping
  leaves unrelated holder kinds alone.
- Nested capabilities are persisted privately, bound to holder metadata by a
  digest, and restored before accepting nested calls. Forwarding discovers
  the current endpoint and reissues disconnected/detached waits within the
  original timeout, never blindly retrying spawn/send/stop. Depth/tier caps,
  pending reports and parent holds survive rejoin.
- Optional state includes last reported turn identity, active send coverage,
  held results and strictly needed steering recovery. Unknown top-level JSON
  members round-trip with unchanged schema 1. Bounded replay gaps use current
  RPC state and saved messages or a specific failure. The accepted append/mark
  boundary can still duplicate one completion mail. Old-writer field loss
  takes the interruption/idle fallback and subsequent worker-orphan sweep.
- Independent R1 identified six substantive recovery issues; all were fixed,
  and fixes-only R2 found none remaining. The full suite then exposed a new
  readiness-channel double close on repeated Start; the existing regression
  passed after applying the scan readiness's `sync.Once` pattern.
- Verification on w5090 under sys_lock, code revision `184452a15` (the tested
  precommit code snapshot): focused restart/nested/process-gone/record/forward
  cases were exercised during development. Final passing command:
  `go test ./internal/wsagent ./internal/wsholder ./internal/mcp -skip
  '^(TestUnattachedHolderIsListedWithItsCauseAndKilledUnderTheGuard|TestGuardedKillHoldsAfterAWallClockStep|TestHolderOutlivesTheProcessThatStartedIt)$'
  -count=1 -timeout=8m && go build ./cmd/ws-mcp`. Package results: wsagent
  58.734s, wsholder 13.942s, mcp 142.445s; build and lock job
  `6a5443b795554ade98ea913c27f93bf8` exited 0. No race sweep or additional
  repeated-restart matrix was run.
- **Forward (one known-base/environment item):** the three skipped holder
  tests all fail at base `47b7f4417` on w5090: the starter test times out
  stopping an orphan holder; guarded-kill tests time out observing child death.
  The abrupt-loss worker scenario itself passes; its cleanup tolerates only
  the exact Stop timeout with a verified Linux zombie and absent descriptor.
  Production holder behavior is unchanged. Restore unexcluded coverage after
  resolving this environment/cleanup issue.
- Not checked live: local Windows replacement, process isolation/job
  breakaway or `restart_safe: false` fallback, real-harness completion mail
  and nested reconnect. These remain the lead/owner's check. The shared
  dashboard daemon was never restarted; tests used isolated state and children.
