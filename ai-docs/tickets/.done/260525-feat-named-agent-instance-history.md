---
title: Named-agent role pointers and instance history
related:
  260524-feat-wsstore-runtime-metadata-migration-gate: introduced SQLite authority for named-agent registry metadata
  260518-epic-ws-dashboard-activity-console: future Activity views need historical agent instances
spec:
  - 260505-named-agent-registry-state-layout
  - 260525-named-agent-runtime-metadata-inventory
  - 260505-agent-async-single-call-lifecycle
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
completed: 2026-05-25
---

# Named-agent role pointers and instance history

## Background

Named-agent registry metadata is now SQLite-authoritative, but the current
runtime still treats a stable public name as one mutable agent record and one
mutable payload directory. Re-registering an inactive same-name agent removes
the prior payload directory and upserts the same SQLite registry row. That
preserves the current `agents.*(name)` API shape, but it loses outputs,
diagnostic streams, event logs, and current-call snapshots that Activity or
future history readers need to explain prior role executions.

The desired model is a stable role pointer over accumulated agent instances.
Public `agents.*` APIs continue to use the same public name, while SQLite
resolves that role to a current instance internally. Re-registering creates a
new instance and moves the role pointer; it does not overwrite or delete the
previous instance.

## Decisions

- Split named-agent identity into role and instance layers.
- A role is keyed by actor scope plus public name and stores the current
  instance pointer.
- An instance is created for each successful registration and owns its own
  payload directory.
- `current/state.json` remains per-instance file-backed current or last call
  state. It is not part of the role pointer and does not need to move into
  SQLite for this change.
- `agents.erase(name)` removes or hides the role pointer. It must not
  synchronously delete historical instances or payload directories.
- Retention defaults to seven days from the final call time. If an instance has
  never had a call, retention uses the instance creation time.
- SQLite metadata is the retention candidate index. Ordinary agent calls must
  not scan agent directories to discover cleanup work.

## Constraints

- Preserve public `agents.register`, `agents.call`, `agents.status`,
  `agents.wait`, `agents.result`, `agents.tail`, `agents.cancel`,
  `agents.print`, `agents.interrupt`, and `agents.erase` inputs for ordinary
  name-based use.
- Preserve actor-scoped identity. The same public name under different actor
  scopes, and the global explicit-root compatibility namespace, must maintain
  independent role pointers and instance histories.
- Registering while the current instance has an active call must remain
  rejected.
- Role pointer changes must be transactional: a failed or partial registration
  must not leave the role pointing at an incomplete instance.
- Historical instance payloads remain file-backed. SQLite stores metadata,
  path indexes, role pointers, retention state, cleanup state, and tombstones,
  not prompt, stream, event, runtime log, current-call, or output bytes.
- Retention cleanup must skip the current role instance, active calls, pinned
  or debug-retained instances, and instances in recoverable consistency states
  that still need diagnostics.
- Retention cleanup must be bounded per public operation. Do not perform a
  full filesystem walk during ordinary `agents.*` calls.
- Legacy `agent.json` import should create a global compatibility role and its
  first instance, then retire the legacy file as read-only input. It must not
  recreate `agent.json` as write authority.

## Spec Impact

Target spec areas: `ai-docs/spec/named-agent-runtime.md` and
`ai-docs/spec/mcp-tools.md`.

Expected caller-visible change: re-registering an inactive named agent keeps
the public name API stable but no longer destroys prior instance history. Erase
hides or removes the current role pointer while historical instances remain
retained until cleanup policy removes them.

Contract-first spec: no. Existing specs already cover the named-agent registry
and SQLite metadata boundary, but the implementation closeout must update the
current "re-register replaces payload directory" contract to the role-pointer
and instance-history behavior.

## Phases

### Phase 1: Implement role pointers, instance history, and retention cleanup

Introduce SQLite role and instance metadata for named agents. A role is keyed by
actor scope plus public name and stores the current instance pointer. Each
successful registration creates a new instance row and a new payload directory,
then updates the role pointer in the final transaction step. Existing public
`agents.*(name)` calls resolve the role pointer to the current instance and keep
their ordinary name-based API shape.

The implementation must preserve `current/state.json` as per-instance
file-backed current or last call state. Active-call checks, status, wait,
result, tail, cancel, debug streams, inbox delivery, and worker startup must
operate on the resolved current instance. Re-registering a role whose current
instance has an active call remains rejected. Re-registering an inactive role
creates a new instance instead of deleting or overwriting the old instance.

`agents.erase(name)` should remove or hide the role pointer and mark the
current instance retention-eligible as appropriate. It must not synchronously
delete historical instance payloads. Successful ephemeral result consumption
should likewise remove or hide the ephemeral role while allowing normal
retention cleanup to remove the instance payload later.

Add retention metadata and cleanup scheduling fences so ordinary operations do
not scan the filesystem for cleanup work. SQLite should track fields such as
`retention_eligible_at`, `retention_checked_at`,
`retention_next_check_at`, `cleanup_state`, `cleanup_attempted_at`, and
`cleanup_error`, or equivalent names. Retention eligibility defaults to seven
days after the final call time; if no call exists, use instance creation time.
Eligible cleanup should query SQLite for bounded candidates whose
`retention_next_check_at` is due, skip current, active, pinned, or diagnostic
recovery instances, and stat/remove only the instance paths already recorded in
SQLite. Failed cleanup should record error metadata and schedule a bounded
retry with backoff. Successful cleanup should leave tombstone or bounded
metadata sufficient for diagnostics without keeping payload bytes.

Verify same-actor same-name re-registration preserves the previous instance
payload directory and SQLite instance row while moving the role pointer to the
new instance. Verify explicit-root global compatibility and actor-scoped roles
do not collide. Verify failed registration does not advance the role pointer.
Verify `agents.call`, `agents.status`, `agents.wait`, `agents.result`,
`agents.tail`, `agents.cancel`, `agents.interrupt`, and worker/inbox hooks use
the resolved current instance. Verify `agents.erase` hides the role without
immediate historical payload deletion. Verify seven-day retention cleanup skips
current, active, pinned, and recovery instances; cleans only due retired
instances; records retry fences on cleanup failure; and does not walk unrelated
agent directories on ordinary calls. Verify legacy `agent.json` import creates
a global role and first instance without restoring file-backed write authority.
Run targeted `wsstore`, `wsagent`, and `mcp` tests, the full Go suite, and
native Windows coverage for path cleanup and retention timing behavior.

### Result (ade814a) - 2026-05-25

Implemented named-agent role pointers and retained instance history. SQLite now
stores role/current-instance metadata, retained instance rows, path indexes, and
retention/cleanup fences while prompt, output, event, stream, runtime-log, and
`current/state.json` bytes remain file-backed. Re-registering an inactive role
creates a new instance and moves the role pointer without deleting the previous
payload directory; failed registration leaves the existing pointer intact.

`agents.call`, `agents.status`, `agents.wait`, `agents.result`, `agents.tail`,
`agents.cancel`, `agents.interrupt`, worker/inbox hooks, and deprecated
`agents.print` resolve the current instance for root-omitted actor-scoped calls,
while hidden explicit-root compatibility remains global. `agents.erase` and
successful ephemeral result consumption hide/remove role pointers without
synchronous payload deletion.

Retention cleanup is bounded and SQLite-candidate-driven. It uses seven-day
eligibility from final call time or instance creation time, skips current,
pinned, recovery, backoff-fenced, and per-instance `current/state.json`
queued/running instances, removes only recorded instance paths, and records retry
metadata on `agent_instances` after cleanup failures. Legacy `agent.json` import
creates the first global role instance and keeps `agent.json` retired as
read-only compatibility input.

Verification:

- `go test -count=1 ./internal/wsstore ./internal/wsagent ./internal/mcp`
- `go test -count=1 ./...`
- Native Windows: `go test -count=1 ./internal/wsstore ./internal/wsagent ./internal/mcp`
