---
title: Lead-side explore becomes an async RPC child; the blocking leaf stays worker-only
spec:
  - pi-adapter-runtime
related:
  - 260906-bug-ws-pi-goal-reminder-races-child-push-at-settle
  - 260906-feat-ws-pi-tool-result-yaml-tui-rendering
  - 260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ba2a307381422fcf
sage-review-completeness-reviewed: ba2a307381422fcf
---

# Lead-side explore becomes an async RPC child; the blocking leaf stays worker-only

## Background

Owner direction, 2026-09-06, while reading the goal-loop settle race: the
`explore` tool is the one dispatch tool that does not behave like the rest.
`ws-agent-spawn`, `ws-fork`, and `ws-execute` all go through the RPC-backed
engine (`spawner.ts` `spawnAgent`): the caller gets an agent id back at
once, the child shows up in the shared registry and the fan-in status line,
and its final report and settle reach the lead as pushes. `explore` is the
one-shot `pi --mode json -p --no-session --tools=recon` leaf left over from
the pre-RPC phases (`exploreLeaf`, `spawnPiProcess`, `waitForDone`): it
blocks the caller's tool call until the child exits, keeps its own
`AgentRegistry` that the goal loop's yield gate does not see, and with
`async: true` returns a running entry that nothing ever pushes about, so
the caller has to come back and poll.

The reason it stayed that way is recursion. A worker (full-worker or
execute-worker tool group) gets `explore` too, and a worker is a single-task
RPC child that wants the answer as a tool result inside its own turn: it
has no push inbox of its own, must not spawn persistent children (depth cap
lead → worker → explore-leaf), and a blocking leaf that self-reaps is the
right shape for it. That shape was then also handed to the lead, where it
is the wrong one: the lead is built to dispatch and be woken by pushes
(`spawner.ts` header), a blocking explore holds the lead's turn so nothing
can reach it meanwhile, and the async variant is invisible to the goal
loop.

Owner's call: expose different tools to the two roles rather than one tool
with two modes.

## Proposed direction

Adapter-only (`agents-plugin-pi/`); no ws-mcp change. The `explore`
playbook, the `small` alias resolution, and the `recon` tool group are
reused as they are.

- **Lead and fork: `explore` is a preset over `spawnAgent`.** Same tool
  name and `query` param, no `async` param. It renders the `explore`
  playbook, spawns an RPC child with `toolGroup: "recon"` passed
  explicitly (`resolveSpawnToolGroup` defaults to `full-worker`), the
  `small` alias (or the inherited model when the alias is unset, as
  today), an auto alias such as `explore-N`, and a title derived from the
  query, and returns `{agent_id, alias}` immediately. A fork gets the same
  preset: it runs its own `session_start` and registry, and
  `computeForkToolSurface` passes the `explore` name through unchanged.
- **Answer delivery: the settle push.** The child has the `recon` group
  only, so it holds no `ws-report-to-lead` and can file no `final`
  report; `applyRpcEvent` only ever raises a report from that tool. The
  answer is the child's last assistant message, and it reaches the lead on
  the `ws-agent-settled` push's `last_message` (`harvestLastMessage`), the
  same path every other child's closing message takes. The explore
  playbook already makes the child end on the answer. `ws-agent-transcript`
  remains available for the full text while the record exists.
- **One-shot record.** `RpcAgentRecord` gains `oneShot: true` for the
  preset. `sendToAgent` (`ws-agent-send`) refuses a one-shot record with
  a message naming it as an explore; `ws-agent-stop` (cancel) and
  `ws-agent-transcript` (read) stay allowed. On settle, the settle IIFE in
  `attachEventListener` runs the existing silent `stopAgent` and then
  deletes the registry entry itself, after the push; the deletion lives in
  the IIFE, not in `stopAgent`, whose D-C invariant ("the registry entry
  is NEVER deleted here") is unchanged. An owner-cancelled explore
  (`ws-agent-stop` on a one-shot) is deleted by the stop tool's own body
  right after `stopAgent` returns, for the same reason and at the same
  layer, so a cancelled explore never lingers as an un-resumable dormant
  row. One-shot records are excluded from
  the shutdown sidecar snapshot (taken before `stopAll()` while records
  are still live), so an explore in flight at lead exit does not come back
  as a dormant record.
- **Live-agent widget.** A lead explore is a live registry member with a
  client, so it renders a row while running and disappears at settle. The
  row must read `explore`, which takes three mechanical changes:
  `SpawnAgentRole` (`spawner.ts`, today `worker | execute-worker | fork`)
  and the widget's `AgentRowRole` / `roleFromSpawnRole` (`agent-widget.ts`,
  which maps every unrecognized value to `worker`) both gain `explore`;
  `buildRpcClientOptions` (`spawner.ts`, which hardcodes
  `WS_PI_SPAWN_ROLE` to `fork`/`worker`) takes the role from a new
  `spawnRole` override on `spawnAgent` so the child process also carries
  `explore`; and the preset sets `record.spawnRole = "explore"`. The leaf's
  `buildChildProcessEnv` is the process-spawn path and is not reused. The
  widget spec's "no explore row" sentence is amended (see Spec Impact).
- **Worker and explore leaf: the blocking leaf, unchanged.** The
  `full-worker` and `execute-worker` tool groups keep the current
  `exploreLeaf` under the same `explore` name, minus the `async` param
  (its only consumer was the lead). An explore child itself still gets no
  `explore` (recon group), so the depth cap holds in all three shapes
  (lead → explore child, worker → blocking leaf, lead → fork → explore
  child).
- **Registration by role.** `registerAgentTools` (`spawner.ts`, called
  from `index.ts`) registers the lead preset when `readSpawnRole` says
  lead or fork and the blocking leaf otherwise; the role is in the
  process environment from launch, so it is known at factory time, and
  the two never coexist in one process. The tool's description text says
  which shape the caller holds. `pi-lead-guide.md`'s dispatch row for
  `explore` changes from "answer" to "id now, answer on the settle push".
- **Fallout absorbed here.** The goal-loop yield gate and fan-in line count
  lead explores automatically (same registry), which removes the
  "async explore is not a waker" carve-out in the settle-race ticket. The
  explore-effort fix in the tier-degrade ticket (Phase 2) applies to the
  lead path through `spawnAgent`'s own `effectiveModelEffort` /
  `setThinkingLevel`; the worker leaf keeps needing the `--thinking`
  launch flag from that ticket.
- **Ordering against the YAML rendering ticket.** That ticket's Phase 2
  states "only `explore` resolves inside its own `execute`" and introduces
  `spawnAgent`'s `onModelResolved`. If this ticket lands first, the lead
  explore path resolves through `spawnAgent` and that phase wires the
  resolved line once, in `spawnAgent`; if that phase lands first, its
  explore-specific wiring is replaced here. Either order works; the
  Constraints line records it.
- Rejected: one tool with a `mode` param. The two shapes have different
  return contracts and different lifecycles; a role-keyed registration is
  what the owner asked for and keeps each description honest. Rejected:
  making the worker leaf async too; a worker has nothing to be woken by.
  Rejected: giving the explore child `ws-report-to-lead` for a `final`
  push; it widens the recon surface for no gain over the settle push.

## Spec Impact

`pi-adapter-runtime`:

- `{#260903-pi-explore-recon-leaf}` (primary): split into the lead preset
  (RPC child, immediate id, answer on the settle push, one-shot removal)
  and the worker leaf (blocking, self-reaping), keyed on role.
- `{#260904-pi-spawner-bounded-depth-explore-leaf}`: restate the depth cap
  for the three shapes.
- `{#260903-pi-spawner-tool-groups}` and `{#260903-pi-delegation-spawner-tools}`:
  the `explore` name in the worker groups is the leaf; the lead's `explore`
  is the preset; `ws-agent-send` refuses one-shot records.
- `{#260905-pi-live-agent-widget}`: replace "a one-shot explore never
  enters the registry" with the live `explore` row that disappears at
  settle.
- `{#260903-pi-spawner-model-tier-inherit}`: only its sentence on explore's
  effort surface, now split by shape.
- The goal-loop passage's async-explore exclusion is removed once this
  lands.

## Constraints

- No ws-mcp change; playbook and alias resolution reused.
- Depth cap unchanged: a lead explore child gets the `recon` group only.
- The explore child gets no report tool; the answer travels on the settle
  push's `last_message`.
- Ticket `260906-bug-ws-pi-goal-reminder-races-child-push-at-settle`
  Phase 1 can land before this; its async-explore carve-out is then
  deleted here, not there.
- Ticket `260906-feat-ws-pi-tool-result-yaml-tui-rendering` Phase 2 and
  this ticket may land in either order; whichever lands second reconciles
  explore's resolved-model wiring so it lives in `spawnAgent` once.

## Phases

### Phase 1: Role-keyed explore registration

Add the lead preset over `spawnAgent` (explicit `recon` group, `spawnRole`
override, `oneShot` flag), the `sendToAgent` refusal, the settle-IIFE
removal after the silent stop, the sidecar exclusion, drop the `async`
param from the worker leaf, register by role in `registerAgentTools`, and
update the lead guide row. Tests: the lead process registers the preset
and not the leaf, a worker process the reverse, a fork the preset; the
child env and the record both carry `explore` and `roleFromSpawnRole`
maps it to the `explore` row; the
preset returns an id and alias and its child is counted by the fan-in
line and the goal-loop gate; the child's answer arrives as the settle
push's `last_message` and the record is gone after settle; the widget row
reads `explore` while live and is absent after settle; `ws-agent-send` to
an explore id is refused while stop and transcript work, and a stopped
one-shot is gone from the registry; a one-shot record
is not in the sidecar snapshot; the worker leaf still blocks and
self-reaps. Amend the spec passages under Spec Impact. Owner-run live
check: lead calls `explore`, sees the id at once, and receives the answer
on the settle push while free to act.

### Result (a7e42cf5) - 2026-09-06

Landed on `impl/track/pi-agent/kestrel-fern-lantern` (plan `8d12af2f`,
implementation `21488404`, guide and spec `15dec54a`, review fixes
`1fed78ba` and `a7e42cf5`). `npm test`: 877 pass, 0 fail (baseline 859; 18
new tests).

What landed:

- Lead and fork processes register `explore` as a preset over `spawnAgent`:
  explicit `recon` tool group, `small` alias with inherited-model fallback,
  auto alias `explore-N`, title derived from the query, `spawnRole:
  "explore"`, `oneShot: true`; returns `{agent_id, alias}` at once. The
  answer arrives on the `ws-agent-settled` push's `last_message`.
- Worker and execute-worker processes keep the blocking `exploreLeaf`
  under the same name, minus the `async` param.
- One-shot lifecycle: `ws-agent-send` refuses one-shot records; the settle
  IIFE deletes the record after its push and the silent `stopAgent`; the
  `ws-agent-stop` tool body deletes a cancelled one-shot after `stopAgent`
  returns; `pushSpawnFailed` deletes a one-shot whose launch failed after
  its spawn-failed push (review fix); `captureOrphans` skips one-shot
  records. `stopAgent` itself is unchanged.
- `buildRpcClientOptions` takes a `spawnRoleOverride` so the explore child
  carries `WS_PI_SPAWN_ROLE=explore`; execute-worker children still carry
  `worker`. `SpawnAgentRole`, `AgentRowRole`, and `roleFromSpawnRole` gain
  `explore`, so the widget shows an `explore` row while the child runs.
- The goal-loop yield gate and the fan-in line count a lead explore through
  the shared registry with no goal-loop code change. The spec's goal-loop
  passage never carried an async-explore exclusion; the settle-race
  ticket's carve-out lives only in its own Result-bearing Phase 1 text.
- Spec: the six `pi-adapter-runtime` anchors under Spec Impact amended;
  `pi-lead-guide.md` explore row now reads "id now, answer on the settle
  push".

Review (partitioned, one relay): correctness Important (failed spawn left
a one-shot record parked forever) fixed in `pushSpawnFailed`; fit Important
(stale module header, `registerAgentTools` doc comment, and a false "no
session persisted" clause in the preset description) fixed; a follow-up
fit finding (spec did not name the spawn-failure deletion trigger) fixed
in `a7e42cf5`. Recorded
minors, no action: `ws-agent-stop`/`ws-agent-transcript` throw `unknown
agentId` when the settle deletion wins the race (previously a no-op);
`explore-N` has no collision handling against a user-chosen alias;
`evicted` is not surfaced by the preset (plan-authorized);
`deriveExploreTitle` slices UTF-16 units; the `explore` playbook still
tells the child's caller to continue via `ContinueIdiom`, which the
one-shot refusal now blocks (playbook out of this ticket's scope); the
worker leaf's blocking behavior is verified structurally only (invoking it
under `node --test` would re-exec the test file as the Pi child); new
`registerAgentTools` tests do not call `stopLivenessProbe()`.

Owner-run live check still pending: lead calls `explore`, sees the id at
once, and receives the answer on the settle push while free to act.
