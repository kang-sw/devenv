---
title: Pi adapter dogfood UX collection
---

# Pi Adapter Dogfood UX Collection

## Context

Collect the owner's Pi adapter UX requests in one ticket while dogfooding
inside Pi. More requests are forthcoming; record only supplied requests here.
The owner explicitly requested ticket capture only on 2026-09-06, superseding
the earlier instruction to implement report styling immediately.

## Tickets

- `260906-feat-ws-pi-tool-and-push-tui-polish` — owns the overlapping
  compact/muted push presentation and shared backgrounds; the owner requested
  ready promotion of its full remaining scope on 2026-09-08. This is inclusion,
  not a parent/child relationship. Other collected requests below are unchanged.

## Planned References

- **Report message visual distinction** — The common-background and subdued
  push treatment is now owned by `260906-feat-ws-pi-tool-and-push-tui-polish`.
  The older report-only background restriction is superseded by the later
  shared-push request reaffirmed on 2026-09-08. Preserve model-visible content
  and asynchronous delivery. Preserve the separate report-header-color request
  here as a historical capture; the polish ticket does not specify a new header
  redesign. Do not create duplicate compact/background styling work here.

- **Agent row model and usage visibility** — Extend agent rows such as
  `b9b83114 · execute · awaiting approval · 21s` with
  `<model-name> (<effort>)`. Show input tokens from the most recent model
  call and cumulative estimated cost for that agent in USD. Explicitly
  distinguish the estimate from actual billing; display `—` when data is
  unavailable. The owner confirmed these accounting and fallback semantics.
  Intended role: make the model, reasoning effort, and resource usage visible
  alongside agent status.
  Creation condition: the owner moves the collected request into planning;
  this entry does not assert that runtime usage or billing data is available.
- **Agent count above the agent list** — The owner observes the
  `ws: 1 agents` row appearing after the `openai-codex-usage` widget. Explore
  placing that row above the agent list instead of appending it at the tail.
  Intended role: keep the agent count visually grouped with the agent list.
  Creation condition: the owner moves the collected request into planning;
  feasibility and current widget ordering have not been independently verified.

- **Goal stop/reset control** — The owner could not find a goal reset button
  and attempted `goal clean` / `goal clear`, which armed new goals instead.
  Provide a discoverable way to stop/reset the active goal without accidentally
  replacing it with another goal. Intended role: make goal cancellation explicit.
  Creation condition: the owner moves this collected UX request into planning;
  exact control placement and stop/reset semantics are not yet specified.

- **Compact user-visible agent output** — Routed to
  `260906-feat-ws-pi-tool-and-push-tui-polish` by owner direction on 2026-09-08:
  ten logical lines, full expansion, muted foreground and common push background;
  no truncation of model-visible reports. This entry is no longer an unassigned
  creation request.

## Focus

Capture additional owner-supplied UX requests in this same collection. The
included polish ticket owns the approved ready-promotion request; this board
is not an implementation target. Unrelated widget, usage and goal-control
requests remain capture-only. Specific palette values are not fixed here.
Use `ws-ask` for questions requiring owner discussion while gathering these
requests. Usage display semantics are settled above; implementation remains
explicitly deferred.

## Exit Criteria

- Done: the owner ends collection and the recorded requests have explicit
  dispositions.
- Deferred: implementation planning and any separate actionable tickets await
  the owner's direction; unspecified future requests are not inferred.
