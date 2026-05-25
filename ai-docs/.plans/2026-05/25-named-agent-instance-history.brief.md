# Brief: 260525-feat-named-agent-instance-history

## Intent

Preserve named-agent execution history by splitting stable public role names
from immutable agent instances while keeping the existing name-based `agents.*`
API. Re-registration should advance a SQLite role pointer to a new instance
instead of deleting the previous payload directory or overwriting the same
registry row.

## Scope Boundary

Selected scope: `260525-feat-named-agent-instance-history` Phase 1, "Implement
role pointers, instance history, and retention cleanup".

Out of scope: dashboard Activity UI consumption, new public history browsing
tools, moving prompt/output/event bytes into SQLite, and changing backend
runner behavior except where runners must resolve the current instance.

## Caller-Visible Contract

Public `agents.register`, `agents.call`, `agents.status`, `agents.wait`,
`agents.result`, `agents.tail`, `agents.cancel`, `agents.print`,
`agents.interrupt`, and `agents.erase` remain name-based. Same actor scope plus
same public name resolves to a role pointer. Registering an inactive role creates
a fresh instance and points the role at it; the previous instance remains
retained until cleanup. Registering while the current instance has an active call
is still rejected. `agents.erase(name)` removes or hides the role pointer and
does not synchronously delete historical payloads.

## Contract Instructions

Use `wsstore` as the write authority for role, instance, path, retention,
cleanup, and tombstone metadata. Keep `current/state.json`, prompts, inbox
files, events, backend streams, runtime logs, and final outputs file-backed.
Preserve actor-scoped identity and explicit-root global compatibility identity.
Legacy `agent.json` import should create a global compatibility role plus its
first instance, then keep `agent.json` retired as read-only import input only.

Do not add a second registry path, do not recreate `agent.json` write authority,
and do not make ordinary `agents.*` calls discover cleanup candidates by walking
agent directories.

## Integration Test Instructions

Extend `wsstore`, `wsagent`, and `mcp` tests around existing registry,
actor-scope, and lifecycle fixtures. Required coverage:

- same actor plus same public name re-registration preserves the old instance
  row and payload directory while moving the role pointer;
- actor-scoped and explicit-root global roles with the same public name do not
  collide;
- failed registration does not advance the role pointer;
- call/status/wait/result/tail/cancel/interrupt/worker/inbox hooks operate on
  the resolved current instance;
- erase hides the role without immediate historical payload deletion;
- successful ephemeral result consumption hides the ephemeral role and leaves
  instance cleanup to retention;
- seven-day retention skips current, active, pinned, and recovery instances,
  cleans only due retired instances, records retry fences on cleanup failure,
  and does not scan unrelated directories on ordinary calls;
- legacy `agent.json` import creates the first global role instance without
  restoring file-backed write authority.

Run targeted `go test -count=1 ./internal/wsstore ./internal/wsagent ./internal/mcp`,
the full `go test -count=1 ./...`, and native Windows coverage for path cleanup
and retention timing behavior.

## Implementation Strategy Decisions

- Model identity as role plus instance, not mutable single-row agent records.
- Make role pointer updates transactional and final-step: partial registration
  must not leave a role pointing at an incomplete instance.
- Use SQLite retention fields or equivalents for `retention_eligible_at`,
  `retention_checked_at`, `retention_next_check_at`, `cleanup_state`,
  `cleanup_attempted_at`, and `cleanup_error`.
- Eligibility defaults to seven days after final call time, or seven days after
  instance creation when no call exists.
- Cleanup is bounded and candidate-driven from SQLite. It stats/removes only
  recorded instance paths.

## Rejected Alternatives

- Keeping reset-on-register: it destroys useful diagnostics and cannot support
  Activity/history readers.
- Moving per-call payload bytes into SQLite: it increases database pressure and
  conflicts with existing tail/debug/raw file-backed behavior.
- Directory-walk cleanup during ordinary calls: it creates hidden latency and
  cross-ownership surprises in a file-permission-sensitive runtime.
- Treating `current/state.json` as role-level metadata: it is per-instance
  current or last call state and should remain beside that instance's payloads.

## Approach

- Survey current registry schema, manager lifecycle, MCP dispatch, worker hooks,
  and cleanup helpers.
- Introduce role and instance metadata with migrations and compatibility import.
- Route manager operations through role resolution to the current instance.
- Change registration and erase semantics to update role pointers without
  deleting retired instance payloads.
- Add bounded retention scheduling and cleanup using SQLite candidate indexes.
- Update tests at storage, manager, MCP, and Windows path boundaries.

## Constraints

- Preserve public tool inputs and readable behavior except for the intended
  history-preserving re-registration and erase semantics.
- Preserve root-omitted actor scope and explicit-root global compatibility.
- Do not hold SQLite transactions across backend execution or filesystem
  cleanup beyond short metadata writes.
- Do not regress missing file-backed payload diagnostics.
- Keep branch changes scoped to named-agent registry, lifecycle resolution,
  retention cleanup, tests, and required closeout docs.

## Out of scope

- Public API to enumerate historical instances.
- Dashboard Activity read-model integration.
- Prompt bundle changes.
- wsflow no-agent behavior changes unless tests reveal runtime metadata drift.

## Details

The implementation may choose exact table and field names, but the persisted
model must make current role resolution and historical instance retention
explicit. Existing `AgentInternalKey` semantics remain relevant for role keys;
instances need stable internal ids or keys that can own distinct payload
directories under the same role.

`current/state.json` remains the active-call authority for an instance. Only
`queued` and `running` are active states. Any cleanup or registration check that
needs active-call safety must reconcile current state before deleting payloads
or advancing a pointer.

## Verification Contract

Acceptance requires committed implementation, review-clean result, spec and
mental-model closeout, ticket Phase 1 result, local targeted Go tests, local full
Go suite, and native Windows coverage for cleanup/path timing.

## References

- [Must] `ai-docs/tickets/ready/260525-feat-named-agent-instance-history.md` -
  selected ticket and Phase 1 contract.
- [Must] `ai-docs/mental-model/named-agent-runtime.md` - wsagent/wsstore
  lifecycle and file-backed payload boundaries.
- [Must] `ai-docs/mental-model/mcp-runtime.md` - MCP actor scope and SQLite
  metadata migration rules.
- [Must] `ai-docs/spec/named-agent-runtime.md` - caller-visible named-agent
  behavior to update after implementation.
