---
title: Lead-side explore becomes an async RPC child; the blocking leaf stays worker-only
spec:
  - pi-adapter-runtime
related:
  - 260906-bug-ws-pi-goal-reminder-races-child-push-at-settle
  - 260906-feat-ws-pi-tool-result-yaml-tui-rendering
  - 260906-feat-ws-pi-spawn-warns-when-tier-resolution-degrades-to-inherit
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
  playbook, spawns an RPC child with `--tools=recon`, the `small` alias (or
  the inherited model when the alias is unset, as today), an auto alias
  such as `explore-N`, and a title derived from the query, and returns
  `{agent_id, alias}` immediately. The child's answer arrives as the usual
  `ws-agent-report kind:"final"` push followed by its settle; the lead
  reads it in the push (or through `ws-agent-transcript`) like any other
  child. The record is marked one-shot: on settle it is stopped and removed
  from the registry instead of parked, so explore never accumulates
  dormant records or accepts `ws-agent-send`.
- **Worker and explore leaf: the blocking leaf, unchanged.** The
  `full-worker` and `execute-worker` tool groups keep the current
  `exploreLeaf` under the same `explore` name, minus the `async` param
  (its only consumer was the lead). An explore leaf itself still gets no
  `explore` (recon group), so the depth cap holds.
- **Registration by role.** `registerSpawnTools` registers the lead preset
  when the process is the lead or a fork (`isLeadOrFork`, the seam
  `lead-skills.ts`/`lead-bootstrap.ts` already use) and the blocking leaf
  otherwise; the two never coexist in one process. The tool's description
  text says which shape the caller holds.
- **Fallout absorbed here.** The goal-loop yield gate and fan-in line count
  lead explores automatically (same registry), which removes the
  "async explore is not a waker" carve-out in the settle-race ticket. The
  dispatch-tool row work in the YAML rendering ticket (Phase 2) gets its
  resolved model/effort line for explore through `spawnAgent`'s
  `onModelResolved` for free, and the explore-effort fix in the
  tier-degrade ticket (Phase 2) applies to the lead path through
  `spawnAgent`'s own thinking handling; the worker leaf keeps needing the
  `--thinking` launch flag from that ticket.
- Rejected: one tool with a `mode` param. The two shapes have different
  return contracts and different lifecycles; a role-keyed registration is
  what the owner asked for and keeps each description honest. Rejected:
  making the worker leaf async too; a worker has nothing to be woken by.

## Spec Impact

`pi-adapter-runtime` `{#260903-pi-spawner-model-tier-inherit}` (or the
spawner passage that describes explore): split the explore description into
the lead preset (RPC child, immediate id, final push, one-shot stop) and
the worker leaf (blocking, self-reaping), keyed on role. The goal-loop
passage's async-explore exclusion is removed once this lands.

## Constraints

- No ws-mcp change; playbook and alias resolution reused.
- Depth cap unchanged: a lead explore child gets the `recon` group only.
- Ticket `260906-bug-ws-pi-goal-reminder-races-child-push-at-settle`
  Phase 1 can land before this; its async-explore carve-out is then
  deleted here, not there.

## Phases

### Phase 1: Role-keyed explore registration

Add the lead preset over `spawnAgent` with one-shot stop-on-settle, drop
the `async` param from the worker leaf, and register by role. Tests: the
lead process registers the preset and not the leaf, a worker process the
reverse; the preset returns an id and alias and its child is counted by
the fan-in line and the goal-loop gate; the child's final arrives as a
report push and the record is gone after settle; `ws-agent-send` to an
explore id is rejected; the worker leaf still blocks and self-reaps.
Amend the spec passages. Owner-run live check: lead calls `explore`, sees
the id at once, and receives the answer as a push while free to act.
