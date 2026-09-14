---
title: "Recursive subagent tracking: cross-process subtree propagation + nested gutter rendering"
related:
  260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets: split-from; that ticket owns the flat-panel polish (bullets/activity/ctx/heading), this owns the tree
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only
---

# Recursive subagent tracking: cross-process subtree propagation + nested gutter

## Background

The owner wants the live agent panel to show subagents as a nested, recursive
tree — an agent's own spawned children indented under it with a per-depth
gutter — instead of a flat list. Split out of
`260914-feat-ws-pi-agent-widget-recursive-gutter-and-state-bullets` because,
unlike that ticket's flat-panel polish, the tree has **no data source today**.

## The blocker (why this is its own ticket)

`RpcAgentRegistry` is a per-process `Map<string, RpcAgentRecord>`
(`agents-plugin-pi/src/spawner.ts:713`), instantiated fresh per extension
activation (`spawner.ts:3471`, from `index.ts:603`). Each process (lead, fork,
worker) sees only its own direct children; a worker's own spawned grandchildren
live only in that worker's process-local registry, invisible to the lead. The
only descendant signal that crosses a process boundary today is the aggregate
boolean `record.waitingOnChildren` (`spawner.ts:463`), computed from
`subtreeChannel`/`publishSubtree`'s count-only `SubtreeSnapshot`
(`subtree-lifecycle.ts:7-8`: `outstanding`/`active`/`deliveries`/`delegated`/
`revision`) — counts, not itemized descendant records a tree could be built
from. So a recursive gutter needs a new mechanism to propagate structured
descendant identity/state up the process tree before any rendering is possible.

## Open Questions (research needed before this is actionable)

- What structured per-descendant payload crosses the process boundary (id,
  parent id, state, name, depth) and how it extends the existing
  `SubtreeSnapshot`/`publishSubtree` path without regressing the wait/settle
  correctness that `waitingOnChildren` consumers depend on.
- Propagation cost/cadence: pushed on every descendant state change vs. sampled;
  bound on tree size/depth surfaced.
- Rendering: per-depth gutter/indent design, and whether the same tree
  propagates to the `/audit` picker (deferred out of the flat-panel ticket).
- Whether this stays a pi-adapter-internal concern or touches a shared contract.

## Notes

Underspecified on purpose — this needs a design pass on the cross-process
propagation before it becomes an actionable phased ticket. Not part of the
current ready batch.
