---
title: ws dashboard SQLite-backed agent activity source
parent: 260514-epic-ws-web-dashboard-mvp
related:
  260524-feat-wsstore-runtime-metadata-migration-gate: establishes SQLite as the named-agent metadata authority and keeps payload bytes file-backed
  260525-feat-named-agent-instance-history: introduces named-agent role pointers, current instances, retained instance history, and retention cleanup fences
  260518-epic-ws-dashboard-activity-console: provides the existing Activity feed, transcript, SSE, and compatibility projection surface this ticket must preserve
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
---

# ws dashboard SQLite-backed agent activity source

## Background

The dashboard WorkRoot Activity projection still assumes named-agent metadata is
discovered by scanning `<cache>/proj/<worktreeKey>/agents/*/agent.json`. The
current ws-mcp runtime moved named-agent registry metadata to the worktree-local
SQLite store at `<cache>/proj/<worktreeKey>/state.sqlite`. Agent payload bytes
such as `current/state.json`, stdout/stderr, runtime logs, event JSONL, Codex
native transcripts, and final `output.md` remain file-backed under
`agents/<state_path>/...`.

`ws-mcp` and `ws-dashboard` should be treated as one product unit for this
integration. The dashboard should align with the current ws-mcp runtime model
instead of preserving legacy `agent.json` discovery assumptions. Backward
compatibility for pre-SQLite agent metadata belongs inside ws-mcp/wsagent, not
inside dashboard projection code.

## Decisions

- Use `state.sqlite` as the dashboard named-agent registry source of truth.
- Add a backend-only, read-only SQLite adapter for dashboard Activity
  projection. The dashboard must not write, migrate, import, or repair wsstore
  state.
- Remove dashboard-side `agents/*/agent.json` scan and fallback logic.
- Keep frontend code and API response shapes stable. This is a backend internal
  reorganization, not a React/UI change.
- Keep `ActivityFeed.agents` and `ActivityFeed.summary.total` based on current
  role rows from `agent_defs`.
- Allow `ActivityFeed.items` to include retained useful `agent_instances`
  history in addition to current role/current-instance items.
- Keep large and append-heavy payloads file-backed. SQLite supplies identity,
  lifecycle metadata, current role pointers, instance metadata, timestamps,
  path indexes, and retention state; payload readers continue to read files.
- Treat historical instance display as Activity feed context, not as an
  increase in the current agent count.

## Constraints

- Do not edit frontend source as part of this ticket unless a test fixture must
  be adjusted for unchanged API semantics.
- The public dashboard routes remain stable:
  `/activity`, `/activity/events`, and `/activity/items/{activityId}/transcript`
  keep their existing response shapes.
- Missing SQLite database, missing expected tables, unknown schema, locked
  reads, or unavailable registry rows should fail soft as an empty or degraded
  Activity projection. They must not crash the route.
- SQLite connections should be read-only and short-lived or otherwise bounded.
  Use an explicit busy timeout and avoid holding a connection across payload
  file reads when that would increase lock coupling.
- Current role identity and instance identity must be separate. Current role
  compatibility rows come from `agent_defs`; historical Activity items come
  from `agent_instances`.
- Transcript lookup must resolve both current role activity ids and historical
  instance activity ids back to the correct `state_path`.
- Exclude cleanup-deleted or payload-useless rows from visible historical
  Activity items. Retention tombstones are registry internals unless they carry
  useful diagnostics.
- Dashboard code may depend on the wsstore schema as an internal product
  contract, but that dependency must be isolated in a small adapter module
  rather than spread across `work_root_activity.rs`.

## Phases

### Phase 1: Replace agent.json scan with a read-only SQLite registry adapter

Introduce a daemon-side adapter that resolves the wsstate worktree key for an
opened WorkRoot, opens `<cache>/proj/<worktreeKey>/state.sqlite` read-only, and
projects current named-agent role rows from `agent_defs` into the existing
`NamedAgentActivityView` compatibility shape.

The adapter should normalize only the fields the dashboard needs: opaque role
id, public name, backend, harness, tier, model, effort, status, session
presence, last call/seen/update timestamps, current `state_path`, and any
diagnostics needed for degraded rows. It should not import legacy `agent.json`
records or scan the `agents/` directory to discover current agents.

Keep the existing payload parsers for current call state, final output, and
Codex native transcript resolution where possible, but feed them the payload
directory resolved from registry `state_path` instead of a directory scan.

Verification should cover a WorkRoot whose SQLite registry contains current
agents and whose payload directories have no `agent.json` files. The route
should report the correct current agent count, status counts, privacy-preserving
session presence, transcript availability, and degraded behavior for missing or
locked registry state.

### Phase 2: Add retained instance history to Activity items without changing current agent counts

Extend the backend projection so `agent_instances` rows can contribute
historical Activity items when they are useful and retained. Current
role/current-instance items remain visible, and retained instance history may
appear in `ActivityFeed.items`, but `ActivityFeed.agents` and
`ActivityFeed.summary.total` remain based only on current `agent_defs` roles.

Historical item ids must be opaque and instance-stable. Transcript reads for
historical items must resolve the instance row to its `state_path` and then use
the same file-backed transcript/output readers. Current role ids and historical
instance ids must not collide.

Filter out rows whose `cleanup_state` indicates deleted payloads, rows whose
payload path is missing and carries no useful metadata, and internal retention
tombstones. Keep failed, cancelled, completed, or retired instances when they
have output, current-call state, transcript availability, or meaningful
diagnostics.

Verification should prove retained historical instances appear in
`ActivityFeed.items` without increasing `agents.length` or `summary.total`, and
that transcript routes resolve both current role items and retained instance
items.

### Phase 3: Make Activity refresh/versioning SQLite-aware

Update snapshot, SSE polling, and recent refresh version calculation so SQLite
metadata changes can be observed even when no legacy `agent.json` file or
payload file mtime changes. Versioning should consider registry timestamps such
as `updated_at`, `last_seen_at`, `last_call_at`, and relevant instance cleanup
or retention fields, plus payload mtimes for current call, output, runtime log,
stdout, stderr, and Codex native transcripts.

Keep frontend behavior unchanged. Existing Activity event streams should still
emit item upserts, removals, transcript updates, and snapshot invalidations
through the current API. Recent refresh limits should remain meaningful with
registry-backed ordering.

Verification should cover registry-only status changes, payload-only transcript
changes, retained instance appearance/removal, and recent-limit behavior after
the source switches from directory mtimes to registry-aware versions.
