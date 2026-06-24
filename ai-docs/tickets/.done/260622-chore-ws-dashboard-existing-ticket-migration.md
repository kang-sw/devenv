---
title: ws dashboard existing ticket migration
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260514-epic-ws-web-dashboard-mvp: predecessor board to audit and migrate
  260620-feat-ws-dashboard-agent-client-activity-sources: deferred Activity adapter ticket now routed under the realignment epic
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: linked-server ticket whose serverId assumptions should be preserved during migration
  260622-research-ws-dashboard-ferrule-session-binding: research capture that defines the intended session-binding model
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
completed: 2026-06-24
---

# ws dashboard existing ticket migration

## Background

The existing dashboard MVP epic and many dashboard child tickets were written
before the session-key/ferrule pivot fully settled. The implementation is not
yet committed for the new dashboard harness model, but the direction changed:
top-level harness sessions should own lead-scoped ws keys, the dashboard daemon
should keep only private ferrule-backed bindings, and browser-visible Activity
should remain source-neutral.

This ticket is the migration slice for the new dashboard session-key realignment
epic. It imports the realignment board into the active dashboard branch, routes
the current managed CLI and Activity adapter tickets through that board, and
leaves stale spec/mental-model cleanup as the next documentation pass before
implementation-ready promotion.

## Spec Impact

Target spec area: `ai-docs/spec/ws-web-dashboard/index.md` and
`ai-docs/mental-model/ws-web-dashboard.md`.

Expected caller-visible change: none. This phase is documentation cleanup that
reclassifies stale pre-session-key named-agent, SQLite, actor, and Activity
authority assumptions as compatibility behavior or deferred provider-adapter
scope. It should preserve implemented dashboard contracts while making the
accepted daemon-private ferrule/session-binding invariant recoverable for future
dashboard implementation tickets.

Contract-first spec: no.

## Phases

### Phase 1: Board import and active ticket routing

Import the session-key realignment epic and its research child into the active
dashboard branch. Route the active dashboard agent/harness backlog through the
realignment epic:

- `260622-epic-ws-dashboard-session-key-realignment` becomes the active
  coordination board for session-key-aware dashboard agent and harness work.
- `260514-epic-ws-web-dashboard-mvp` remains the predecessor product board for
  reusable workbench, PTY, document, WorkRoot, Activity, and linked-server
  surfaces.
- `260624-feat-ws-dashboard-managed-cli-terminal` becomes the first concrete todo
  child under the realignment epic.
- `260620-feat-ws-dashboard-agent-client-activity-sources` moves or stays at
  idea-level as deferred structured Activity adapter work after the managed CLI
  path is dogfoodable.
- `ai-docs/_index.md` lists the realignment board and current child tickets so a
  fresh dashboard session does not re-derive the branch split.

Deferred scope: do not implement provider adapters, daemon ferrule clients,
browser Activity redesign, linked-server forwarding changes, or spec contract
entries in this phase. This phase is ticket/index migration only.

Verification boundary: ticket status/reference checks should show the
realignment epic, the managed CLI child, the deferred Activity adapter ticket,
and the predecessor MVP board without stale parentage. No browser or daemon
runtime verification is required for this documentation-only phase.

### Result (05a0a9b4) - 2026-06-24

Imported the realignment epic, research child, and migration chore from
`review/dashboard-260622` into the active `dashboard` branch. Reparented
`260624-feat-ws-dashboard-managed-cli-terminal` and
`260620-feat-ws-dashboard-agent-client-activity-sources` under the realignment
epic, kept `260620` at idea level as deferred structured Activity adapter work,
and marked `260514-epic-ws-web-dashboard-mvp` as the predecessor board for
reusable dashboard product surfaces.

The managed CLI ticket now follows the settled daemon-private `ws.ferrule(root)`
binding model from `260622-research-ws-dashboard-ferrule-session-binding` rather
than treating daemon-side ferrule as an undecided future question. `_index.md`
lists the realignment board and adds this migration chore as non-ready dashboard
focus.

### Phase 2: Spec and mental-model drift cleanup

Audit `ai-docs/spec/ws-web-dashboard/index.md` and
`ai-docs/mental-model/ws-web-dashboard.md` for stale assumptions from the
pre-session-key / actor / named-agent era.

Target migration questions:

- Which spec entries still describe named-agent SQLite/wsstate authority as the
  main Activity source after the session-key pivot?
- Which Activity Console and linked-server sections need the
  `wsSessionKey` / `providerSessionId` / `activityId` separation made explicit?
- Which mental-model rules should be updated so future implementers preserve the
  daemon-private binding and do not expose ws session keys to browser routes or
  logs?
- Which current spec wording should remain as implemented compatibility behavior
  rather than planned provider-adapter behavior?

Deferred scope: do not create contract-first implementation spec entries until a
child ticket is promoted to `ready/`.

Verification boundary: a fresh session should be able to identify stale
compatibility sources, deferred provider-adapter work, and the accepted
daemon-private session-binding invariant without reading the old discussion
thread.

### Result (fb731f6) - 2026-06-24

Updated the dashboard spec and mental model so WorkRoot Activity and Activity
Console documentation distinguish the implemented SQLite/wsstate named-agent
compatibility source from future provider-native Activity authority. The spec now
keeps `ActivityFeed.items` as the forward source-neutral contract, treats
`ActivityFeed.agents` as compatibility projection data, and records the
daemon-private `wsSessionKey` / `providerSessionId` / browser-facing
`activityId` split for dashboard-launched harness and provider work.

No runtime behavior changed. Verification covered `ws/spec_index.verify`, a
targeted stale-authority grep over the edited docs, and partitioned correctness
and fit review.


## Resolution (2026-06-24)

Phase 1 imported and routed the realignment board. Phase 2 updated the
dashboard spec and mental model to separate current named-agent/SQLite
compatibility Activity sources from daemon-private ferrule/session-binding and
deferred provider-adapter scope.
