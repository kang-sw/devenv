---
title: Add cycled named-agent instance history
related:
  260523-feat-ws-dashboard-main-session-activity-source: Activity freshness gap exposed during dashboard dogfood
  260523-feat-ws-dashboard-activity-console-tail-ribbon-polish: Activity Console transcript inspection makes lost history visible
spec:
  - 260505-async-subquery-ephemeral-agent
related-mental-model:
  - named-agent-runtime
  - ws-web-dashboard
---

# Add cycled named-agent instance history

## Disposition (2026-06-09): dropped — feature target removed by the pivot

This feature extends the named-agent (`agents.*`) runtime, which
`260605-epic-ws-playbook-factory-pivot` deletes wholesale (delegation moves to
harness-native subagents). With the machinery gone there is nothing to add
instance-cycling to, so this is dropped rather than annotated for later. The
`spec:` link to `260505-async-subquery-ephemeral-agent` is left intact; that
spec anchor lives in `named-agent-runtime.md` and is removed by the epic's own
`spec-remove` flow at M2/M3, not by this drop. Body retained for context.

## Background

Dogfood investigation of stale-looking WorkRoot Activity raised whether named
agents had been deleted during implementation. The runtime currently has two
destructive paths:

- `agents.erase` removes the named agent directory for the current worktree.
- `agents.register` removes the existing agent directory before creating the new
  registry record, as long as the current call is not active.

This reset-on-register behavior is intentional today: re-registering a stable
role name means the caller wants a fresh agent definition and fresh runtime
state. However, the Activity Console and future execution history views need a
way to explain recent work without making a stable role row depend on a single
mutable directory.

A better future layout is likely instance-based. Each registered agent instance
should get a unique storage path, while the stable role name points at the
current instance. When a new instance is registered for the same role, old
inactive instances can be cycled out by count, age, or storage budget instead of
being immediately overwritten. Active instances must remain protected from
cycling.

## Follow-Up Questions

- What should the instance id be: monotonic per role, timestamped, UUID-like, or
  derived from execution id?
- Where should the stable role pointer live, and how should interrupted
  registrations avoid pointing at a partial instance?
- What cycling policy should apply to old inactive instances by default?
- How should WorkRoot Activity surface current role rows versus older inactive
  instances without exposing private backend paths?
- How does explicit `agents.erase` interact with current instance cleanup and
  retained historical instances?
