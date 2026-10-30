---
title: Pi adapter emits subagent lifecycle events for Orca's status bridge
related:
  260730-research-orca-plugin-ws-workflow-surface: earlier Orca integration survey (context only)
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
  are handled identically (the row is forgotten; children whose `parent`
  equals the id are forgotten with it). Details are recorded only on the first
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
  (`agents-plugin-pi/src/subtree-lifecycle.ts`), capped at
  `MAX_SUBTREE_DESCENDANTS`.

## Decisions

- **Lead-side, emit-only adapter.** The emitter lives in the lead process's
  spawner side and only calls `pi.events.emit`; it never runs in a child
  process, opens no socket, handles no Orca token, and never depends on the
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
  true, and the self-wake `agent_start` branch of `applyRpcEvent`.
- **Completion.** The run closes on the admitted settlement
  (`admitSettlement` / `createTerminalDelivery`), not on the raw
  `agent_settled` event; a held settlement (waiting on the child's own
  children) keeps the run open. The silent auto-park `stopAgent` that follows
  every settlement is a no-op for an already-closed run and is never mapped
  to an abort.
- **Guaranteed closure.** Every path that ends a live child closes its open
  run: `clearLiveState` closes any run still open (status by the caller's
  reason: exit and launch failure as `failed`, explicit stop and capacity
  eviction as `aborted`), and the `session_shutdown` path (`stopAll`) closes
  every open run, since it does not go through `stopAgent`. Revived dormant
  records emit nothing until their next relaunch. Exact terminal status is
  secondary because Orca treats all three alike; closure is the requirement.
- **Forks are included.** Fork children are shown like other children.
- **Background delegates may hold the lead pane busy.** A child still running
  after the lead's turn ends keeps Orca's lead pane busy until its run closes;
  this is accepted behavior, not a defect.
- **Descendants are included via the subtree snapshot.** The root emitter
  diffs its descendant view: an id turning `live` opens a run (`started`), an
  id turning not-live or leaving the view closes it, with
  `parentWorkflowRunId` set to the parent's open run id. No new relay is
  added. Descendants beyond `MAX_SUBTREE_DESCENDANTS` stay invisible.

## Constraints

- Out of scope: native-chat child cards, transcript streaming, and
  bidirectional controls; they need an Orca-side child-session API. The status
  bridge is Orca-internal and unversioned.
- Manuals: none of the AGENTS.md Implementation Conventions rows cover
  `agents-plugin-pi/src/`; follow the package's own tests.

## Phases

### Phase 1: Lifecycle emitter

Before coding, verify two unchecked facts and stop back to the lead if either
changes the contract: whether `fork.ts` or `execute-gateway.ts` create child
records outside `spawnAgent`, and whether `stopAll` touches record state.

Then implement the decisions above in `agents-plugin-pi/src/`.

Verification: unit tests with a fake event bus covering both start sites and
idempotence, held versus admitted settlement, the auto-park no-op, every
`clearLiveState` caller and shutdown closing open runs, revived records
emitting nothing, and descendant diffing with `parentWorkflowRunId`. Then a
live check on a dev build inside Orca, run by the lead: one delegate shows in
the dashboard card count, the lead pane stays busy past the lead's turn end,
and both settle when the child finishes.
