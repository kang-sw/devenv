---
title: "Pi adapter: push every child report into the lead session and retire `ws-agent-wait`"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor — the spawn/continue/wait MVP vocabulary this ticket revises for Pi
  260903-feat-ws-pi-subagent-rpc-ux: introduced the persistent RPC children, `pendingReports` and the pull-style `ws-agent-wait` this ticket replaces
  260904-feat-ws-pi-side-thread-fork-question-surface: the fork-raised question notice currently tells the lead to keep polling `ws-agent-wait`; first surface to move to push
  260905-bug-ws-pi-approval-relay-deadlocks-under-agent-wait: the deadlock class that disappears once the lead no longer blocks on wait
  260905-feat-ws-pi-live-agent-widget: sibling — the always-visible running-agent list the lead and owner read instead of polling
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 32284652afca6610
sage-review-completeness-reviewed: 32284652afca6610
completed: 2026-09-05
---

# Pi adapter: push every child report into the lead session and retire `ws-agent-wait`

## Background

The Pi adapter's delegation surface (`260903`) is pull-based: a child's
`ws-report-to-lead` reports land in the lead-process `pendingReports` buffer
and the lead harvests them by blocking in `ws-agent-wait`, which races
`agent_settled` events and a timeout. That mirrors the ws doctrine written for
hosts without a push path (Claude, Codex). Pi has one: the adapter runs inside
the lead process, already observes every child event in `applyRpcEvent`, and
`pi.sendMessage(..., { deliverAs: "followUp", triggerTurn: true })` delivers a
custom message to the lead and starts a turn when the lead is idle — the exact
mechanism the owner-question surface already uses for its `ws-thread-summary`
injection and the execute gateway uses for approval relay.

Owner dogfood (2026-09-05) showed the cost of the mismatch: a lead parked on a
fork that had raised an owner question (`260904`'s notice says "keep waiting on
this agent (`ws-agent-wait`)") looped `ws-agent-wait` + timeout for as long as
the owner was away, burning turns to learn nothing. The `260905` approval-relay
deadlock was the same tension from the other side: a lead blocked in `wait`
cannot receive a pushed approval request, so a wake path had to be bolted on.

Structurally, a blocking wait is not needed on Pi. The lead spawns and ends its
turn; each child report arrives as a message and wakes it. This ticket makes
that the only model.

## Decisions

- **Every child signal is pushed; nothing is harvested.** Six message
  families, each a `pi.sendMessage` custom message with `display: true` and
  `details` carrying `agent_id`, the family-specific payload and the status
  line below. The push is issued by whichever process owns the child's
  registry — the lead for its children, a fork process for the fork's own
  workers and execute workers (`isLeadOrFork` gate, matching today's
  per-process approval relay) — into that process's own session:
  - `ws-agent-report` — `ws-report-to-lead` with `kind:"final"` or untagged
    progress (`details.kind` distinguishes). Pushed from the
    `tool_execution_start` branch, i.e. while the sender is still mid-turn;
    a `final` first sets `record.terminalThisTurn` so the status line
    computed for that very message no longer counts its sender (see fan-in
    below). Delivered `followUp`, `triggerTurn: true`.
  - `ws-agent-settled` — a persistent child (`ws-agent-spawn` worker,
    `ws-execute` worker, task fork) leaves the running state **without**
    having sent a `kind:"final"` report during that turn. `details.reason`
    is `idle` (RPC settle; carries `last_message`, the former `ws-agent-wait`
    `reason:"idle"` payload via `harvestLastMessage`, so a worker whose
    shared playbook never says `ws-report-to-lead` still signals
    completion), `stopped` (the `ws-agent-stop` **tool** only — adapter
    internal stops such as `injectDiscussionSummary`'s thread close and
    `session_shutdown`'s `stopAll()` call `stopAgent(record, { silent:
    true })` and push nothing), `exited` (child process gone, see liveness
    below) or `spawn-failed`. A turn that did send `final` **or
    `question`** (both set `terminalThisTurn`; a question-parked fork ends
    its turn there per `classifyForkTurnOutcome`) pushes only the report;
    the settle is silent. Suppressed while the record is **thread-bound**
    (below). Delivered `followUp`, `triggerTurn: true`.
  - `ws-agent-question` — `kind:"question"` in the headless relay case
    (`260904` §8). Delivered `steer`, `triggerTurn: true`: the child is
    blocked on the answer, so it may interrupt a mid-turn lead. In TUI the
    question is consumed by the `onQuestionReport` hook (owner surface) and
    not pushed; in both modes `handleForkRaisedQuestion` sets `threadBound`
    at question registration, so the parked fork leaves N and M and its
    settle is silent regardless of when (or whether) the owner opens the
    thread.
  - `ws-agent-approval` — execute-gateway approval requests. Delivered
    `steer`, `triggerTurn: true`, keeping today's `sendUserMessage(...,
    steer)` interrupt latency (the worker's `ws-worker-exec` is blocked while
    it waits); the gateway's `waiterWoken` suppression branch is deleted with
    the wake.
  - `ws-agent-advisory` — the three fork anti-bleed advisories
    (idle-without-final, fail-loud transcript tail, `expects_commit`
    non-completion; `details.advisory` names which). Suppressed while
    thread-bound (today's `overlayAttached` check widens to the new flag).
    The idle-without-final predicate loses its `pendingReports` input and
    reads `record.reportLog` instead — a bounded per-record log of
    `{kind, at}` for every report the child ever sent, which also feeds
    `ws-agent-list`'s last-report time — considering only entries since the
    lead's last `ws-agent-send` to that record (`record.lastLeadPromptAt`;
    a nudge prompt does not reset it), so a fork that reported `final` and
    was then re-tasked is judged on the new task, never re-flagged for the
    old one. Delivered `followUp`, `triggerTurn: true`.
  - `ws-agent-orphaned` — pushed once on `session_start` when the
    `.ws-agents.json` sidecar lists children that were alive at the previous
    shutdown (see durability below). Delivered `followUp`,
    `triggerTurn: true`.

  The report branch keeps consulting the `onQuestionReport` / `onFinalReport`
  record hooks first; a hook that consumes the report (a `lead-ask` thread's
  final, which becomes the `ws-thread-summary` injection) suppresses the
  push, while a `fork-raised` final — the task fork's real completion — is
  not consumed: the hook closes the thread (clearing thread-bound) and the
  push then proceeds with a status line that already excludes it.
  **Thread-bound** is a record flag `record.threadBound`, distinct from the
  per-view `overlayAttached` (which `ask.ts` clears on every overlay exit,
  including `Esc`): `ask.ts` sets it on every path that binds a respondent
  to a non-closed thread — `ensureRespondent`/`openThread` (first open and
  every reopen of a dormant thread, spawn or rehydrate alike) and
  `handleForkRaisedQuestion` (question registration) — and clears it only
  when the thread closes (`/done`, fork final, `ws-resolve`). A `lead-ask`
  respondent is therefore bound for its whole life. While set, the record
  pushes no settle or advisory and is
  outside N and M, so owner↔fork exchanges — overlay open or not — never
  reach the lead except through the `260904` summary / fork-final paths.
  Pushes are issued in arrival order; Pi queues those
  that land mid-turn and drains them one per loop iteration (`followUpMode`
  "one-at-a-time"), so a fan-out burst produces one lead run that sees the
  messages in order, not one turn per message. The `pendingReports` buffer
  and its drop-oldest cap go away with the harvester; the adapter never drops
  a report on its own — a report is lost only together with the child that
  produced it (see durability below).
- **Fan-in stays with the model, not the adapter.** Owner decision
  (2026-09-05): each push carries a status line
  `N of M spawned agents still running: <ids>` where M = this process's
  registry members that are neither dormant nor stopped/exited and not
  thread-bound (workers, execute workers, task forks; explore leaves are
  excluded — they return through their own tool result and are not registry
  members; a fork parked on an owner question is thread-bound and therefore
  outside both, its pending question being shown by the `260904` pending
  line instead) and N = the subset that is **running and has not yet sent
  `kind:"final"` or `kind:"question"` in the current turn**
  (`terminalThisTurn`). `running` is set the moment the
  adapter issues a prompt to the child — every `client.prompt` call site
  (`spawnAgent`, both `sendToAgent` branches, the `fork.ts` nudge, the
  overlay `ForkChannel`) goes through one `promptAgent(record, ...)` helper
  that sets it, so there is no spawn→`agent_start` window in which a
  just-launched child reads as not running — confirmed by `agent_start`,
  and cleared on `agent_settled`, stop, exit or spawn failure.
  `terminalThisTurn` is set by a `final` or `question` report and cleared
  on the next prompt. A child blocked on an approval (mid-tool-call) is
  running; a child parked on a question has settled and is thread-bound. A
  child that settled idle, or that has reported `final` and is winding
  down, is **not** in N; it re-enters N only when it is prompted again.
  Hence the
  last `final` of a fan-out is the message that reads `0 of M`, and a
  worker that never reports `final` reaches `0 of M` through its
  `ws-agent-settled`. So a lead waiting on several children can tell
  "not yet — end the turn again" (N > 0) from "all in — synthesize" (N = 0).
  The same predicate drives the Phase 2 yield. Rejected: the adapter
  coalescing a fan-out's reports until the last one settles — it would also
  hold back questions and approval requests, which must not wait.
- **Children stop on shutdown; the resumed session is told who was alive
  and can revive them.** Verified against Pi 0.84.4: a followUp queued
  mid-turn lives in the in-memory `PendingMessageQueue`; `/reload`,
  `switchSession`, `newSession` and `fork` tear the extension runtime down
  and discard it — and the adapter's `session_shutdown` handler already
  `stopAll()`s every RPC child, while `registerAgentTools` builds a fresh
  registry on each `session_start`. Owner decision (2026-09-05): keep the
  stop (no child survives a lead teardown; a pushed message is durable for
  as long as its child and the lead session live) but do not lose the
  children's identity. `session_shutdown` writes, before `stopAll()`, a
  sidecar `<leadSessionFile>.ws-agents.json` (the `ask.ts`
  `.ws-threads.json` precedent) holding every non-dormant record's
  `agent_id`, role (`worker` / `execute-worker` / `fork`), the exact resume
  set `sendToAgent`'s dormant branch consumes — `sessionPath`, `modelBase`,
  `modelEffort`, `systemPromptPath`, `toolGroup`, `explicitTools`,
  `wsToolNames` (the `ask.ts` `PersistedForkResume`/`captureForkResume`
  precedent) — plus state at shutdown (`running` / `idle`) and last-report
  time. On `session_start` for the same lead session file (resume, reload)
  the sidecar is consumed: each entry is re-registered as a **dormant**
  record with its role-specific wiring re-armed (`fork` → the `registerFork`
  hooks: `onQuestionReport`, `wireAntiBleedLoop`, `onFinalReport`, after
  the `ask.ts` `rehydrateForkRecord` + `armFinalReportHook` precedent;
  `execute-worker` → the approval relay's `onApprovalPending`; `worker` →
  none), and one `ws-agent-orphaned` custom
  message is pushed (`followUp`, `triggerTurn: true`) listing them with
  their state and a revival hint — `ws-agent-send <id>` auto-resumes a
  dormant record from its cached session file, so the hint is literally
  executable. What each child was doing is not restated; the resumed lead's
  own transcript already has it. A different session (`/new`) leaves the
  sidecar beside the old session file to fire on that session's later
  resume. Caveat recorded for the guide: a child stopped mid-turn resumes
  from its last flushed turn, so the orphan line marks `running` entries as
  "was mid-turn; re-issue the instruction". Rejected: keeping children alive
  across teardown (needs a re-attach protocol outside the extension
  runtime); replaying buffered reports from the sidecar (they belong to a
  process that no longer exists).
- **Liveness backstop: a child that stops without settling still pushes.**
  `ws-agent-stop` and spawn failure are synchronous and push
  `ws-agent-settled` (`stopped` / `spawn-failed`) directly. Process death is
  not observable through `RpcClient`'s event stream (no exit callback, and
  `process`/`exitError` are private), so the spawner keeps a coarse liveness
  probe that issues `client.getState()` — which rejects through
  `createProcessExitError` once the child is gone — on every registry
  transition and on a timer while N > 0 (order of 30 s), and additionally
  treats the rejection of any in-flight request the same way; a dead
  running child is transitioned to `exited` and pushed. This
  replaces the timeout that `ws-agent-wait` used to provide and is what
  makes the Phase 2 yield unable to hang.
- **`ws-agent-wait` is removed, not deprecated.** A tool that blocks the lead
  is the hazard; leaving it available keeps the timeout loop reachable. The
  approval-pending wake, the `idlePending` edge-consume flag and the waiter
  bookkeeping in `spawner.ts` are deleted with it. `ws-agent-list` remains the
  status query (running / idle / dormant, last report time) and
  `ws-agent-send` / `ws-agent-stop` / `ws-agent-transcript` are unchanged.
- **Goal loop yields to running children.** Owner decision (2026-09-05):
  while N > 0 under the fan-in predicate above (some persistent child is
  mid-turn), an `agent_settled` on the lead does **not** re-inject the goal
  reminder and does not count toward the runaway streak; the pushed
  `ws-agent-report`/`ws-agent-settled` message that wakes the lead is what
  continues the goal. Because every running child eventually settles, stops
  or is found dead by the liveness probe, and each of those pushes, the
  yield cannot hang. Rejected: putting a
  "waiting on N agents" line into the reminder and still re-firing — it still
  spends a turn to say "still waiting".
- **Transcript rendering.** Pushed custom messages are `CustomMessageEntry`
  (role `custom`) and are rendered through `pi.registerMessageRenderer`, not
  `registerEntryRenderer` (which only fires for `appendEntry` entries that
  never enter LLM context). Implementation first checks whether Pi's default
  custom-message rendering is readable (the existing `ws-thread-summary`
  push registers none); a renderer is added only if the default is not.
- **Guide text maps the doctrine, Go stays untouched.** Shared playbooks keep
  saying "wait for the reviewer"; `pi-lead-guide.md` maps that verb to "end
  your turn — the report arrives as a message", and the `260904` fork-raised
  notice drops its `ws-agent-wait` instruction. ws-mcp Go source and
  `agents-plugin/skills/` are not modified (golden rule; the spawner is
  adapter-owned).

- **Edition decisions (owner, 2026-09-05, after the first live run).**
  Supersede the bullets above where they differ:
  - *Status line is built at delivery, not at push.* Pi has no hook at
    followUp delivery (`before_agent_start` fires only for owner-typed
    prompts), and a message built mid-turn read `0 of 1` for the first of
    three workers. `followUp` pushes are sent at once only when the owning
    process's agent is idle (`ctx.isIdle` captured at `session_start`);
    otherwise they are held and released in arrival order from that
    process's `agent_settled`, each built at release time. `steer` families
    are never held; held pushes die with the process. The line gains
    `: <running ids>` and is omitted when M = 0 (no `0 of 0` on the orphan
    message).
  - *A `final` is released when the child's turn ends.* Stashed on the
    record at the tool call (last wins) and pushed as one `ws-agent-report`
    with `details.settled_reason` (`idle` / `stopped` / `exited`) at the
    leave-running transition, with no `ws-agent-settled`; a silent stop
    drops it; hook-consumed finals still push nothing. Chosen over "final at
    once, settle silent" so the lead never acts on a completion report
    while the child is still finishing (a commit after the report, say).
  - *`ws-agent-orphaned` only for children cut off mid-turn.* Every sidecar
    entry is still re-registered dormant with role wiring, but the message
    is pushed only when some entry was `running`; idle entries (finished or
    waiting for a relay) are summarized in one line there and otherwise
    only visible in `ws-agent-list`. The first live run announced three
    finished workers as orphans, which was noise.
  - *Per-family message renderer.* Pi's default custom-message rendering
    repeated the family (its `[customType]` label plus the content head), so
    the six families get a compact renderer in TUI; content is unchanged.
  - *Status line counts only running children (owner, after the second live
    run).* `N delegated agents still running`, with no `of M` and no id
    suffix; still omitted when nothing is delegated. M only grew across a
    session (idle children keep their process until stopped or exited), and
    the ids duplicated `ws-agent-list`. `0 delegated agents still running`
    remains the synthesis cue. Naming children readably (spawn-time title,
    context-bearing list) is `260905-feat-ws-pi-agent-alias-park-and-registry-cap`.

## Constraints

- Headless lead (`--mode rpc`): `pi.sendMessage` is host-level, so push works
  there too; the `260904` §8 relay baseline for fork-raised questions becomes
  a pushed `kind:"question"` message rather than a `ws-agent-wait` return.
- Within a live lead session no push is dropped or duplicated; the adapter
  holds `followUp` pushes only while its own agent is mid-turn and releases
  them at `agent_settled` (Edition, 2026-09-05); it keeps no buffer a lead
  could drain. On
  `/reload` or a session switch the existing `session_shutdown` → `stopAll()`
  behavior stands; the `.ws-agents.json` sidecar carries identities, never
  reports, and is consumed exactly once so a resumed session gets one
  `ws-agent-orphaned` message, not one per resume.
- Push is gated on `isLeadOrFork`: the lead pushes for its children, a fork
  process pushes into its own session for the fork's workers and execute
  workers (its tool surface keeps `ws-agent-spawn`/`ws-execute`). A
  `worker` process never pushes — its only delegate is the `explore` leaf,
  whose `recon` tool group has no `ws-report-to-lead` and whose result
  returns as the tool's own return value.
- A child's turn never reaches the lead twice: a report consumed by an
  owner-thread hook, a settle or advisory of a thread-bound record, and an
  adapter-internal (`silent`) stop are not pushed.

## Spec Impact

`pi-adapter-runtime`: the delegation-spawner anchor's tool list and the
"Child→lead report channel" anchor change from harvest to push (the five
message families, `deliverAs` per family, ordering under Pi's one-at-a-time
followUp drain, the status line and its `running` predicate, the
`isLeadOrFork` push gate, the thread-bound suppression, the liveness
probe, and the shutdown sidecar → `ws-agent-orphaned` → dormant re-register
contract); "Turn completion is gated on RPC idle" keeps
its RPC semantics, gains the settle-pushes-`ws-agent-settled` rule (with its
`reason` values), and loses the `ws-agent-wait` wording together with the
"spawn failure settles its waiters" sentence; the lead-execute approval
gateway anchor's `pending_approval` return path is replaced by the pushed
`steer` approval message; the goal-loop anchor gains the running-children
yield rule and its "Lead-session-only" bullet drops "`ws-agent-send` /
`ws-agent-wait`" for "`ws-agent-send`, reports pushed back"; the
side-thread anchors drop the "keep waiting" notice text and
gain the thread-bound rule. Three more anchors still carry `ws-agent-wait`
prose and are rewritten in the same pass: `{#260903-pi-explore-recon-leaf}`
("not harvestable through `ws-agent-wait`" → not a registry member, no
push), the `{#260905-pi-side-thread-fork-task-thread}` live-verification
note ("harvests via `ws-agent-wait`" → arrives as `ws-agent-report`), and
the fork-raised `/done` rationale in
`{#260905-pi-side-thread-owner-question-surface}` ("strand the lead's
`ws-agent-wait`" → the fork's own final is the lead's signal).

## Phases

### Phase 1: Push channel + `ws-agent-wait` removal

In `agents-plugin-pi/src/spawner.ts`, add a single `pushToLead(record,
family, payload, deliverAs)` helper, gated on `isLeadOrFork`, that builds
the custom message (family `customType`, `display: true`, `details` with
`agent_id`, payload, and the `N of M ... still running` status line computed
from the registry with the fan-in predicate) and calls
`pi.sendMessage(..., { deliverAs, triggerTurn: true })`. Route through it:
`applyRpcEvent`'s report branch after the `onQuestionReport`/`onFinalReport`
hooks (`ws-agent-report` — set `terminalThisTurn` on `final` before
computing the status line — and `ws-agent-question`, which also sets
`terminalThisTurn`), `applyRpcEvent`'s settle branch (`ws-agent-settled`
`idle` with `last_message` when `terminalThisTurn` is unset and the record
is not `threadBound`), the `ws-agent-stop` tool and spawn
failure (`stopped` / `spawn-failed`; `stopAgent` gains a `silent` option
used by `ask.ts` and `stopAll()`), the liveness probe (`exited`), the fork
loop's advisories in `fork.ts` (`ws-agent-advisory`), and the execute
gateway's approval requests in `execute-gateway.ts` (`ws-agent-approval`,
`steer`; delete the `waiterWoken` branch). Add `promptAgent(record, ...)`
wrapping every `client.prompt` call site (`spawnAgent`, `sendToAgent`,
the `fork.ts` nudge, the overlay `ForkChannel`) to set `running`, clear
`terminalThisTurn`, and stamp `lastLeadPromptAt` on lead-originated sends;
clear `running` on settle, stop, exit and spawn failure; expose
`running`/`idle`/`dormant` plus last-report time in `ws-agent-list`. Add
`record.threadBound`, set by `ask.ts` in `ensureRespondent`/`openThread`
(first open and reopen) and `handleForkRaisedQuestion`, cleared in the
thread-close paths (`/done`, fork final, `ws-resolve`), and
`record.reportLog` (bounded) replacing `pendingReports` for `fork.ts`'s
`isIdleWithoutFinal` (entries since `lastLeadPromptAt`) and for
`ws-agent-list`. Add the liveness probe (`client.getState()` ping on
registry transitions + timer while N > 0; in-flight request rejection
handled the same way).
Add `agents-plugin-pi/src/agent-sidecar.ts` with pure
`serializeOrphans(records, now)` / `parseOrphans(json)` and the IO glue:
`session_shutdown` writes `<leadSessionFile>.ws-agents.json` before
`stopAll()`; `session_start` (lead-or-fork, same session file) reads and
deletes it, re-registers each entry as a dormant record carrying the full
resume set (`sessionPath`, `modelBase`, `modelEffort`, `systemPromptPath`,
`toolGroup`, `explicitTools`, `wsToolNames`), re-arms role wiring by
`role` (fork hooks via the `registerFork` path, execute-worker approval
relay), and pushes one `ws-agent-orphaned` message. Expose the registry
to factory-scope consumers through a ref filled at `session_start` (the
`wsBlockRef` pattern in `index.ts`) for Phase 2. Delete `ws-agent-wait`,
`pendingReports`, `idlePending`, `settleWaiters`, the approval-pending wake
and their tests; keep `ws-agent-list`, extending it with last-report time.
Check default rendering of the pushed messages in the TUI; add
`pi.registerMessageRenderer` for the six families only if the default is
unreadable. Rewrite `pi-lead-guide.md` (wait row → "end your turn; reports
arrive as messages", approval row, fork row, settle-message row incl. the
`stopped`/`exited` reasons, orphan row with the mid-turn caveat), the
`260904` fork-raised notice in `ask.ts`,
and the tool descriptions that say "harvest with ws-agent-wait".
Verify offline with `npm test`: push call shape and `deliverAs` per family;
no push from a `worker`-role process, push from a `fork`-role process;
settle pushes `ws-agent-settled` only when no `final` or `question` landed
in that turn and never while `threadBound`; a question-parked fork is
`threadBound` from registration, outside N and M, with a silent settle;
`threadBound` is set on a thread reopen as well as first open; an advisory
is withheld for a `threadBound` record; a `lead-ask` final is hook-consumed
and not
pushed, a `fork-raised` final is pushed with the record already outside N;
status-line arithmetic (a `final` sender excluded from N on its own
message so the last of three finals reads `0 of 3`; idle child not counted;
approval-blocked child counted; `threadBound` record absent from M even
after an overlay `Esc`; stopped child absent; explore leaf absent; a child
counted as running from `promptAgent` before `agent_start` arrives; a
nudge prompt counted); the `ws-agent-stop` tool pushes `stopped`, a
`silent` stop does not; spawn failure pushes `spawn-failed`; a dead child
is pushed as `exited` by the probe; `isIdleWithoutFinal` over `reportLog`
(final before the last lead send → not flagged on a new task; final in the
old task and none since a lead re-send → flagged; nudge does not reset);
sidecar round-trip (serialize non-dormant records only, parse
back to dormant records with the full resume set intact and the role
wiring re-armed — a revived fork's `question` routes to the owner surface,
a revived execute-worker's approval relays — consumed once,
absent sidecar → no push, empty sidecar → no push); no waiter code paths
left. Live: spawn three workers and end the turn — all three signals
arrive (any turn grouping) each carrying a correct status line, and the lead
synthesizes only after the one that says `0 of 3`; a worker approval request
reaches a mid-turn lead without waiting for the turn to end; a fork-raised
question in TUI still routes to the owner overlay, owner↔fork exchanges
push nothing to the lead, and the fork's final report wakes the lead without
any wait call; kill a running worker's process from outside and confirm an
`exited` message wakes the lead; `/reload` while a worker is running yields
one `ws-agent-orphaned` message naming it as `running`, after which
`ws-agent-send <id>` resumes it from its session file and a second
`/reload` with nothing live pushes nothing; `/new` (session switch) while a
worker is running pushes nothing into the new session, and a later
`/resume` of the old session gets the orphan message.

### Result (68150a2c) - 2026-09-05

Landed as `654f2fe4` (feature), `01dd2824` (tests), `ab9832a4` (review
relay #1), `c37f920e` (spec), `68150a2c` (sidecar state, relay #2), on the
implementation branch under the goal branch.

Behavioral delta:

- `ws-agent-wait`, `pendingReports`, `idlePending`, the settle waiters and
  the approval-pending wake are gone. Every child signal is a
  `pi.sendMessage` custom message from one `pushToLead` helper: the six
  families with the `deliverAs` per family as decided, gated on
  `isLeadOrFork`. The status line reads `N of M delegated agents still
  running` (no id list; ids are in each message's `details.agent_id`). M is
  every registry record with a live client that is not thread-bound; N is
  the `running && !terminalThisTurn` subset, so a fan-out of three finals
  reads `2 of 3`, `1 of 3`, `0 of 3`.
- `promptAgent` wraps every `client.prompt` site; `record.threadBound` is
  set on every thread open/reopen and on fork-raised question registration
  and cleared on `/done`, fork final, `ws-resolve` and (addition beyond the
  plan) on a lead `ws-agent-send` to the record and on `stopAgent`;
  `record.reportLog` + `lastLeadPromptAt` drive `isIdleWithoutFinal` and
  `ws-agent-list`'s `last_report_at`; the `getState()` liveness probe runs
  on registry transitions and on a ~30 s timer while N > 0.
- `agent-sidecar.ts`: `session_shutdown` writes
  `<leadSessionFile>.ws-agents.json` before `stopAll()` with the resume set,
  `spawnRole`, `state` (`running`/`idle`) and `lastReportAt`; `session_start`
  consumes it once, re-registers dormant records with role wiring re-armed
  (`fork` → `armForkRoleWiring`, `execute-worker` → the per-record
  `onApprovalPending` relay) and pushes one `ws-agent-orphaned` roll-call,
  one line per agent, with the mid-turn re-issue caveat on `running`
  entries. Thread-bound records are not captured (the `.ws-threads.json`
  registry already persists them).
- `pi-lead-guide.md` maps "wait for the reviewer" to "end your turn; the
  report arrives as a message" and documents the settle reasons, the
  approval message and the orphan row. No custom message renderer was
  registered: Pi's default custom-message rendering shows the plain-text
  body `buildPushContent` builds (`[family] agent <id>` head, payload,
  status line last); readability is confirmed at the live gate below.

Deviations from the plan: `sendToAgent` and `stopAgent` also clear
`threadBound` (review #2 minor: a lead send into an owner-open thread
briefly unbinds it until the next `/answer` re-binds; left as is). A revived
fork loses `expects_commit` (not persisted in the sidecar; the revival
defaults it to `false`). A task fork whose `final` lands while its
fork-raised question is still `pending` detaches that thread to dormant, so
the pending count drops without the owner having answered. The suggested
`tsc --noEmit` gate was not added (new dev dependency; owner decision).

Verification: `cd agents-plugin-pi && npm test` → 593/593. Review #1
(correctness 1C/3I, fit 1I, test 4C/5I) → relay #1 fixed all Critical and
Important items; review #2 (Critical-scoped, fresh reviewers): correctness
`clean with 4 minor remaining`, test `clean`. Findings under
`~/.cache/ws@kang-sw-devenv/proj/17da6bdc@657b050c/review-paths/f34fec99-*`
and `511e2b2d-01-*` / `16155c3e-01-*`.

Not yet exercised live (owner-run, needs the adapter user-scope installed;
record results as an Edition): the three-worker fan-out status lines and the
`0 of 3` synthesis; a worker approval reaching a mid-turn lead; a TUI
fork-raised question routing to the overlay with nothing pushed for the
owner↔fork exchange and the fork's final waking the lead; an externally
killed worker surfacing as `exited`; `/reload` with a running worker →
one `ws-agent-orphaned` naming it `running`, `ws-agent-send <id>` resuming
it, and a second `/reload` pushing nothing; `/new` then `/resume` of the old
session receiving the orphan message; and default rendering readability of
the pushed messages.

#### Edition (9f740c46) - 2026-09-05

First live run (three workers, owner-driven) exercised the push channel and
surfaced four clutter defects: the fan-in status line was computed at push
time and read `0 of 1` / `0 of 2` / `0 of 3` on delivery; the TUI printed the
`[family]` header twice; the orphan message ended in `0 of 0`; and `/reload`
after all workers had finished pushed `ws-agent-orphaned` for idle entries.
Owner decisions are recorded under `## Decisions` ("Edition decisions").

Range `2627c6e6..9f740c46`:

- Hold-until-settle: `followUp` pushes go out at once only while the owning
  process's agent is idle (`ctx.isIdle` captured at `session_start`);
  otherwise they are held and flushed in arrival order from `agent_settled`,
  with the status line built at flush. Steer families are never held.
- Status line `N of M delegated agents still running: <running ids>`, omitted
  when M is 0.
- `kind:"final"` is stashed on the record and pushed as one `ws-agent-report`
  carrying `details.settled_reason` (`idle` / `stopped` / `exited`) when the
  child leaves running; no `ws-agent-settled` follows it. Silent stops drop
  it; a new instruction (prompt, steer, or followUp) clears it.
- `ws-agent-orphaned` is pushed only when at least one sidecar entry was
  `running`; all entries are still re-registered dormant.
- Per-family TUI message renderer (`src/push-render.ts`) replaces the default
  `[customType]` label; message content is unchanged.

Verification: 634/634. Edition review (fresh reviewers): test `clean` with
1 minor; correctness `non-clean: 1 important` (stale `pendingFinal` survived
the live `sendToAgent` branch) → relay `9f740c46` fixed it and the unhandled
renderer-registration rejection. Findings under `653f5694-0{1,2}-*`.

Live re-check still owner-run: expect `2 of 3: <ids>` → `1 of 3: <id>` →
`0 of 3`, finals arriving at child turn end with `settled_reason`, a single
header per message, and no orphan push after `/reload` when all workers are
idle.

#### Edition (5b9fa21c) - 2026-09-05

Second live run (three workers) matched the first Edition's expectations:
finals arrived at child turn end with `settled_reason: idle`, one header per
message, status lines `2 of 3: <ids>` → `1 of 3: <id>` → `0 of 3`. Owner then
decided the `of M` total and the id suffix were noise (see the last Edition
decision under `## Decisions`). `5b9fa21c` changes the line to
`N delegated agents still running`, keeps the omission rule, and updates the
renderer classifier, the guide, and the tests (634/634). Follow-up for
readable child names: `260905-feat-ws-pi-agent-alias-park-and-registry-cap`.
Orphan gating (`/reload` all idle → nothing; `/reload` with one running →
one roll-call naming it) is still owner-run.

### Phase 2: Goal loop yields to live children

Depends on Phase 1. In `goal-loop.ts`, the armed `agent_settled` handler
consults the RPC registry (through the Phase 1 `session_start`-filled ref,
since `registerGoalLoop` runs at factory scope) with the Phase 1 fan-in
predicate: when N > 0
(some persistent child is mid-turn) it neither re-injects the reminder nor
advances the runaway streak (record a "yielding to running agents" status
via `ctx.ui.setStatus` under its own key, cleared on the next lead turn; the
footer's agent count belongs to `260905-feat-ws-pi-live-agent-widget`'s
segment, so this one carries the yield wording only). Cover with tests
for: running child → no re-fire, no streak change; children idle, dormant,
stopped or `final`-reported-this-turn → normal re-fire; a `threadBound`
respondent alone → normal re-fire; a pushed
`ws-agent-settled`/`ws-agent-report` arriving while
yielding starts the turn that continues the goal; a child found dead by the
probe while yielding ends the yield through its `exited` push. Live check:
`/goal` a task that spawns a worker and confirm the lead does not re-fire
until the worker's message lands.

### Result (4aef0afc) - 2026-09-05

Landed as `373ce297` (survey plan), `e9df8dd7` (feature), `4aef0afc`
(review relay #1), `e818d193` (spec), on the implementation branch under the
goal branch.

Behavioral delta:

- `spawner.ts` exports `hasRunningAgents(registry)`; it and
  `computeRunningStatusLine` share one private `computeFanIn` walk, so the
  yield gate and the pushed status line can never disagree on who counts as
  running (live client, mid-turn, not `terminalThisTurn`, not
  `threadBound`).
- `decideOnSettle(state, threshold, yielding)` gained a third parameter and
  a `{action: "yield"}` decision, checked right after the inactive-state
  check: state passes through unchanged (no reminder, no streak advance).
  `registerGoalLoop` takes an optional `rpcRegistryRef` (the factory-scope
  ref `index.ts` already built for the approval relay) and degrades to
  "never yield" when it is absent or empty.
- While yielding the footer shows `Goal loop: yielding to running agents`
  under the `ws-goal-loop-yield` status key; a factory-scope `agent_start`
  listener clears it on the next lead turn whatever started that turn. The
  listener no-ops while no goal is active so a headless `--mode rpc` session
  does not emit a no-op UI request per turn (review relay #1, minor).
- Tests (648/648): seven `hasRunningAgents` cases mirroring the status-line
  fixtures (running, idle, dormant, stopped, `final` this turn, thread-bound
  only, empty/absent registry), four `decideOnSettle` yield cases (no
  re-fire, no streak change, inactive-state precedence, yield-then-normal
  re-fire), the probe-death case (`probeAgentLiveness` finding the child gone
  flips the predicate false), and the registry-state half of the
  push-arrival case (`agent_settled` on the child flips it false).

Deferred, per the phase text's own live-check sentence: the "pushed message
starts the continuing turn" half depends on Pi scheduling a turn from
`pushToLead`'s `triggerTurn: true`, which the offline suite does not mock;
the `/goal`-with-a-worker live check remains owner-run.

Review: correctness clean (2 minor, both applied), fit clean, test one
Important [fixed]. Spec: `pi-adapter-runtime` goal-loop entry gains the
"Yield to live children" bullet and the lead-session-only bullet now names
the `WS_PI_SPAWN_ROLE` marker (the `WS_PI_AGENT_CHILD=1` wording was stale
since 260904 Phase 1).

## Non-goals

- Changing the shared ws doctrine or ws-mcp's own agent primitives for Claude
  or Codex.
- Coalescing or summarizing reports on the adapter side.
- The always-visible running-agent list (`260905-feat-ws-pi-live-agent-widget`).
- Keeping spawned children alive across a lead `/reload` or session switch
  (they stop; the orphan message plus dormant re-registration is the
  recovery path).

## Sage Review History (2026-09-05, resolved)

Not a live blocker: the final round stamped `completed` for both stages.
The round-3 tables below are kept as the record of the design iteration.

### Design Reviewer — block (round 3)

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | The terminal "0 of M" signal is unreachable when a child completes by reporting kind:"final" | critical | autonomous |
| 2 | overlayAttached is a per-view flag, so an Esc-detached owner thread resumes pushing into the lead | important | autonomous |
| 3 | The stopped push fires for the adapter's own internal stop of a discussion fork | important | autonomous |
| 4 | Deleting pendingReports removes the input of fork.ts's idle-without-final predicate with no replacement | important | autonomous |
| 5 | The liveness probe's two named signals are both private on RpcClient | minor | autonomous |
| 6 | "the send/prompt latch the existing streaming flag already covers" is not what the code does | minor | autonomous |
| 7 | Spec Impact misses three anchors that still name ws-agent-wait | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Live verification still names only /reload, not an explicit session-switch check | minor |

### Resolution (2026-09-05, rounds 1-3)

Round 1 (design block ×7, completeness concern ×2) and round 2 (design
block ×5 incl. one `missing`, completeness pass ×1) were folded into the
Decisions before this round; their tables were replaced by later stamps.
The round-2 `missing` item (reload durability) became an owner decision:
children still stop on `session_shutdown`, their identities are kept in a
`.ws-agents.json` sidecar, re-registered as dormant on the same session's
resume, and announced through one `ws-agent-orphaned` push with a
`ws-agent-send` revival hint.

Round 3 (all autonomous): a `final` sender sets `terminalThisTurn` and
leaves N on its own message, so the last final of a fan-out reads `0 of M`;
suppression and N/M exclusion re-keyed from the per-view `overlayAttached`
to a thread-lifetime `record.threadBound`; adapter-internal stops are
`silent`; `record.reportLog` + `lastLeadPromptAt` replace `pendingReports`
as the idle-without-final input; liveness probe pinned to
`client.getState()` rejection; `running` set at prompt issue via
`promptAgent`; the three additional spec anchors are listed under Spec
Impact; the session-switch live check is spelled out. Re-review requested
for a fresh verdict.

Round 4 (design concern ×5, completeness concern ×2, all autonomous, folded
in): `question` reports set `terminalThisTurn` like `final` and
`handleForkRaisedQuestion` binds the fork at registration; `threadBound` is
set on every open/reopen path, not at spawn; the sidecar carries the exact
dormant-resume set and re-registration re-arms role wiring; Phase 2 reaches
the registry through a `session_start`-filled ref; the goal-loop anchor's
`ws-agent-wait` mention and the stale "overlay-attached" phrasing in Spec
Impact are fixed; advisory suppression has its own test item. Posture:
completed/completed.


## Resolution (2026-09-05)

Both phases landed on the goal branch: Phase 1 (push channel, `ws-agent-wait` removal, sidecar, two Editions) and Phase 2 (goal loop yields to running children). Owner-run live checks still outstanding: the `N delegated agents still running` countdown, orphan gating across `/reload`, and `/goal` with a worker not re-firing until the worker's message lands.
