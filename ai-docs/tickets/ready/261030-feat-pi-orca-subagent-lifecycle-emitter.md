---
title: Pi adapter emits subagent lifecycle events for Orca's status bridge
related:
  260730-research-orca-plugin-ws-workflow-surface: earlier Orca integration survey (context only)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: b7cb0d5a9c1c7861
sage-review-completeness-reviewed: b7cb0d5a9c1c7861
---

# Pi adapter emits subagent lifecycle events for Orca's status bridge

## Background

Inside Orca (observed: Orca 1.4.245, Pi 1.0.4, ws/Pi 0.48.7, Pi launched with
`--mode rpc`), ws-owned Pi subagents run but Orca does not see them. Orca
installs a Pi extension, `~/.pi/agent/extensions/orca-agent-status.ts`, that
listens on the lead process's Pi event bus and posts an active roster to
Orca's authenticated `/hook/pi`. ws emits nothing on that bus today
(`agents-plugin-pi/src` has no `pi.events` use).

Verified facts this ticket builds on:

- Pi `ExtensionAPI.events: EventBus { emit(channel, data): void; on(channel,
  handler): () => void }` (pi-coding-agent `dist/core/event-bus.d.ts`,
  `dist/core/extensions/types.d.ts`).
- The Orca extension subscribes to `task:subagent:lifecycle`,
  `subagent:async-started`, `subagent:async-complete`, and
  `subagent:process-terminal` (`orca-agent-status.ts`, the `piEventBus.on`
  block). Accepted lifecycle payload: `{ id (or runId), status: 'started' |
  'completed' | 'failed' | 'aborted', agent?, description?, mode?,
  workflowKey?, parentWorkflowRunId? }`. `completed`, `failed`, and `aborted`
  are handled identically (the named row is forgotten; linked children are
  cascaded only when their registration differs from the current extension
  registration, so same-registration children must be closed explicitly). Details are recorded only on the first
  `started` for an id. A `started` arriving with no lead turn in flight sets
  `waiting`, which keeps the pane busy until the roster drains. `mode:
  'workflow'` rows are hidden but still count as active.
  `subagent:process-terminal {runId}` hides the row and posts `agent_end`
  after a 2 s grace.
- Roster rows are `{ id, state: 'working', startedAt, agentType<=40,
  description<=160 }`. Orca shows them as the dashboard popout card's
  expandable "N subagents" list (`dashboardPopout.card.subagents`) and uses
  them for live busy status, counting only children with `startedAt >=` the
  parent's `stateStartedAt` (`out/shared/native-chat/native-chat-live-status.js`).
  They do not create native-chat child cards or transcript rows.
- ws children are separate Pi subprocesses launched with `--no-extensions
  --extension <ws entry>` (`agents-plugin-pi/src/spawner.ts`,
  `buildRpcClientOptions`), so the Orca extension runs only in the lead
  process and a child's own bus never reaches it.
- Descendant lifecycle already reaches the root: the subtree snapshot carries
  `SubtreeDescendant { id, parentId, depth, role: "worker" | "execute" |
  "fork" | "explore", live, display? }` hop by hop over the ParentChannel
  (`agents-plugin-pi/src/subtree-lifecycle.ts`), capped at 128 rows and depth 8
  (`MAX_SUBTREE_DESCENDANTS` and `MAX_SUBTREE_DEPTH`). Existing `live` means
  `record.client !== undefined`, not execution; display state, timestamps,
  snapshot revisions, and work generations cannot identify admitted spans.
- Fork and execute children both use `spawnAgent` and the shared event
  listener, including the self-wake path; no separate child creation path
  needs a second emitter (`fork.ts:353`, `execute-gateway.ts:738`,
  `spawner.ts:3585-3607,3816-3820,4037-4041,2916-2928`).
- `stopAll` already calls silent `stopAgent` for registry records, and
  `stopAgent` clears live state before awaiting teardown. Shutdown captures
  the recoverable snapshot before those stops (`spawner.ts:4768-4778,
  4290-4303`; `index.ts:357-363`). Capacity eviction only removes eligible
  dormant records and does not terminate live children
  (`spawner.ts:3414-3460`).

## Decisions

- **Lead-side, emit-only adapter.** The emitter lives in the lead process's
  spawner side and only calls `pi.events.emit("task:subagent:lifecycle", payload)`;
  it never emits Orca events in a child process, opens no socket, handles no
  Orca token, and never depends on the
  Orca extension being present (emitting with no listener is harmless).
- **Per-running-span run id.** Each agent has at most one open run at a time;
  a run spans one running period. The event `id` is `${agentId}:${n}`, `n` a
  counter owned by the emitter. Rejected: the persistent `agentId` as the event
  id (details are recorded once, so startedAt/description go stale and one
  missed terminal event pins the lead busy); deriving `n` from
  `record.workGeneration` (it also advances on queued-work boundaries and stops
  inside one span). `agent` is the alias or agent kind, `description` the
  title; `mode` is never set.
- **Start sites.** `started` is emitted idempotently (no-op while a run is
  open) from both places a run begins: `promptAgent` when `startsNewRun` is
  true, and the self-wake `agent_start` branch of `applyRpcEvent`. Preserve
  `applyRpcEvent` as pure state transition logic; emit through its IO wrapper.
- **Completion.** The run closes on the admitted settlement
  (`admitSettlement` / `createTerminalDelivery`), not on the raw
  `agent_settled` event; a held settlement (waiting on the child's own
  children) keeps the run open. Automatic parking is conditional: it excludes
  thread-bound, owner-held, running, streaming, or descendant-waiting records;
  forks use their coordinator's finish path. When silent auto-parking occurs,
  its `stopAgent` is a no-op for an already-closed run, never a second abort.
- **Guaranteed closure.** Every path that ends a live child closes its open
  run: `clearLiveState` closes any run still open (exit and launch failure as
  `failed`, explicit stop as `aborted`). The `session_shutdown` path
  (`stopAll`) already reaches `stopAgent` and must leave no open runs,
  including descendant runs tracked at the root. Dormant capacity eviction
  and revived dormant records emit nothing; a revived record starts a run
  only when execution resumes. Exact terminal status is secondary because
  Orca treats all three alike; closure is the requirement.
- **Forks are included.** Fork children are shown like other children.
- **Background delegates may hold the lead pane busy.** A child still running
  after the lead's turn ends keeps Orca's lead pane busy until its run closes;
  this is accepted behavior, not a defect.
- **Descendants use execution spans over the existing subtree snapshot.**
  Extend descendant metadata with explicit span-active state and a source-span
  identity, maintained by the owning process at the same start, admission,
  and teardown boundaries. Preserve and forward it through the existing
  ParentChannel snapshot; only the root emits Orca events. The source identity
  must distinguish replacement spans across relaunch/recovery; it is not the
  root-owned `${agentId}:${n}` event ID. The root diffs active state and source
  identity, closes a replaced span before opening its replacement, and closes
  a span when it becomes inactive, disconnects, or leaves the bounded view.
  Dormant or live-but-idle descendants do not open runs. No new relay is added;
  descendants beyond either snapshot bound stay invisible. Rejected:
  process-live edge diffing as a descendant exception, because it would show
  settled owner-held/thread-bound processes as ongoing work and miss successive
  spans; display-state heuristics do not repair this distinction.
- **Parent linkage is opportunistic at initial start.** Use the parent's open
  event ID as `parentWorkflowRunId` when observed at the child's initial start.
  Otherwise omit that field and emit the child immediately. Do not reparent
  that same event later: Orca records metadata only on its first start. Snapshot
  and RPC-start arrival ordering differ, so parent-before-child selection is
  not proof that a parent event is open. Close descendants explicitly rather
  than relying on Orca cascade. Rejected: delaying child visibility until a
  parent start arrives (may never show activity), and referencing a last-known
  closed parent (does not satisfy open-parent linkage).

## Constraints

- Out of scope: native-chat child cards, transcript streaming, and
  bidirectional controls; they need an Orca-side child-session API. The status
  bridge is Orca-internal and unversioned.
- Manuals: none of the AGENTS.md Implementation Conventions rows cover
  `agents-plugin-pi/src/`; follow the package's own tests.

## Prior Decisions

- `654f2fe4` (2026-09-05): keep `applyRpcEvent` pure; IO belongs in
  `attachEventListener`.
- `6bafc76f` (2026-09-13): execution, descendant waits, and pending terminal
  admission are distinct states.
- `5886d3b7` (2026-09-20): descendant identity is advisory, parent-before-child,
  and bounded independently of settlement accounting.
- `c834d5b4` (2026-10-04): disconnects invalidate cached descendant live flags;
  lifecycle edges publish promptly.
- `56b69863` (2026-10-28): the subtree snapshot remains acknowledged lifecycle
  state; the batch-display relay is separate.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | Spawner, subtree integration, and lifecycle tests. |
| scope.surface | cross-module | Registry lifecycle reaches Pi's extension event bus. |
| scope.new_public_symbol | unknown | Exported helper names are not prescribed. |
| scope.new_type_contract | yes | Lifecycle payload and descendant span metadata. |
| scope.test_surface | existing | Spawner, recursive-worker, subtree, fork, and execute tests. |
| complexity.reuse_points | confirmed | Shared spawn, listener, terminal-delivery, clear-state, and subtree seams. |
| complexity.side_effect_risk | moderate | Events affect Orca busy state, not execution authority. |
| risk.correctness | high | Admission, teardown, replacement spans, reconnect, and parent ordering interact. |
| risk.fit | moderate | Pure state bookkeeping; IO wrapper emits events. |
| risk.test | high | Race coverage and live dashboard acceptance are required. |
| risk.security_or_contract | moderate | External Orca bridge is private and unversioned. |

## Blocked (2026-10-30) — no supported structured RPC delivery path

The lead stopped implementation after new evidence contradicted the integration
premise for the owner's actual Orca Pi RPC session. The owner had authorized
overnight implementation and merge, but did not authorize a terminal-only
scope change or Orca-side modifications. This ticket remains open without a
Phase 1 Result; do not dispatch it until the integration contract is settled.

Verified in the actual root session using presence booleans only:
`ORCA_PANE_KEY`, `ORCA_AGENT_HOOK_PORT`, `ORCA_AGENT_HOOK_TOKEN`, and
`ORCA_CHAT_SCRATCH_DIR` are absent; `ORCA_PI_STATUS_OWNED` is present. No
values or credentials were printed. The ownership marker proves extension
initialization, not hook delivery.

The peer co-lead independently corroborated the installed Orca 1.4.245 code:
`~/.pi/agent/extensions/orca-agent-status.ts`, `postOnce` around lines 300-311,
returns without pane key, port, and token. The structured Pi launcher in
`out/main/chunks/abort-aware-delay-D4NfEMlz.js:48` deliberately deletes the
pane/hook variables through `inheritedEnvToDelete: EM`. Its Pi JSONL-RPC
translator in `structured-agent-session-host-assembly-5BQCq7U4.js:115` has no
subagent/child cases; the ACP/Codex child machinery is not wired to Pi. No
supported alternative destination was found. Thus the emitter alone cannot
reach the target session's dashboard/busy UI; this is a contract failure,
not merely an unobserved live acceptance check.

Incomplete prototype: `5c51c7e77` on `impl/develop/exes-defy-cache`, retained
unmerged (branch tip `53888ac25` includes blocked-state documentation).
Changed prototype paths: `agents-plugin-pi/src/subagent-lifecycle.ts`,
`src/spawner.ts`, `src/subtree-lifecycle.ts`, and
`test/subagent-lifecycle.test.ts`. The worker reports targeted tests passed
using `npm test -- --test-reporter=dot test/subagent-lifecycle.test.ts
 test/subtree-lifecycle.test.ts test/recursive-worker.test.ts test/spawner.test.ts`.
The full suite, remaining integration coverage, independent implementation
review, and live UI acceptance were not completed; the prototype is not
merge-ready. The clean pooled worktree was released, preserving its branch.

Only this blocked-state documentation lands on develop. No source merge,
production install/reload, synthetic hook call, forged environment, or Orca
patch was performed. Resume requires the owner's scope decision: establish
an Orca-side structured-Pi integration prerequisite, or explicitly choose a
terminal-mode target and validate that bridge there. Neither option is yet
approved; morning UI inspection alone does not remove this blocker.

## Phases

### Phase 1: Lifecycle emitter

Implement the decisions above in `agents-plugin-pi/src/`, preserving the
existing pure-state/IO split and snapshot transport. Update descendant row
selection, parsing, forwarding, and lifecycle publication to carry explicit
span state and source identity; do not infer spans from `live` or display
classification. Fork and execute already share creation and listener paths;
shutdown already reaches silent stop. Preserve pre-stop recovery snapshots.

Verification: unit tests with a fake event bus covering both start sites and
idempotence, admitted versus held settlement, conditional auto-parking, fork
and execute creation/self-wake, every `clearLiveState` caller and shutdown
closing all open direct and descendant runs, dormant eviction/recovery emitting
nothing, and operation with no Orca listener. Cover descendant snapshot parsing
and forwarding, repeated spans within one live process, identity replacement
across relaunch/recovery, live-but-idle owner-held/thread-bound records,
disconnect/reconnect and bounded-view removal/reintroduction, parent-before-child
start selection, a child observed before its parent start, missing parent
linkage with no reparenting, and explicit descendant closure independent of
Orca's cascade.

Then perform a live check on a dev build inside Orca, run by the lead: one
delegate shows in the dashboard card count, the lead pane stays busy past the
lead's turn end, and both settle when the child finishes. Live delivery remains
unverified at promotion; static/unit evidence is not a substitute for this
implementation acceptance check.
