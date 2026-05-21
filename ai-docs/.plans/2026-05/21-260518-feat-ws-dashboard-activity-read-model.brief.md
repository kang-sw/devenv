# Brief: 260518-feat-ws-dashboard-activity-read-model

## Intent

Implement the backend Activity Console read model that turns the existing
WorkRoot Activity named-agent projection into a workRoot-scoped Activity Feed
snapshot plus selected transcript backfill contract. This unblocks the static
Activity Console UI shell without adding live streaming, agent controls, or
non-agent transcript resolvers.

## Scope Boundary

Implement only Phase 1 from
`ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-read-model.md`.

In scope:
- Backend/core public types for source-neutral Activity Feed snapshots,
  Activity Items, Activity Transcript responses, and Transcript Blocks.
- Daemon route handling for workRoot-scoped feed snapshots and selected
  named-agent transcript backfill.
- Frontend TypeScript types and route helpers that consume the new read-model
  contract.
- Named agents as the first activity source.
- Route tests, serde tests, frontend helper tests, and redaction/degraded-state
  coverage.

Out of scope:
- Activity Ribbon or Transcript Block visible UI rendering.
- SSE/watch stream, local acknowledgement dirty-state logic, or live UX merge.
- Native Codex/Claude/Gemini transcript resolver support beyond reserving
  source-neutral shape.
- Async exec implementation or exec transcript source beyond reserved kinds.
- Agent start, interrupt, cancel, erase, retry, or any other control action.

## Caller-Visible Contract

Authenticated browser/API callers can request a workRoot-scoped Activity Feed
snapshot for an opened workRoot and receive source-neutral selectable activity
items ordered for live/latest ribbon use. Callers can request a selected
activity transcript and receive bounded normalized transcript blocks. Responses
use opaque `workRootId` plus activity ids and never expose daemon-private host
paths, cache paths, session ids, process ids, stdout/stderr paths, stream paths,
or backend-native transcript paths.

Unknown workRoots and unknown activity ids return explicit route errors or
unavailable/degraded response states. Malformed or unavailable named-agent
records degrade individual items or transcripts without failing the whole feed.

## Contract Instructions

Public contract:
- Implement spec `{#260521-ws-dashboard-activity-console-read-model}`.
- Keep the model workRoot-scoped and owner-authenticated under the existing
  dashboard protected route boundary.
- Preserve compatibility for the existing WorkRoot Activity route or make any
  response migration explicit in route tests, frontend helper tests, and docs.
- Use camelCase JSON and public core types consistent with
  `ws-dashboard/crates/core/src/activity.rs`.
- Keep activity ids logical and stable enough for transcript route addressing;
  do not expose directory paths, session ids, pids, stream paths, or cache keys.
- The named-agent source should be represented as an Activity Item and
  transcript source kind, not as a named-agent-only API surface.
- Activity Feed item state must cover at least: id, kind, label, status,
  live/attention flags, timing fields, source display metadata, transcript
  availability/status/cursor, diagnostics, and metadata suitable for later UI.
- Feed ordering must prioritize active/live/attention/blocked/failed/recent
  items before alphabetical tie-breaking.
- Transcript backfill must return bounded normalized blocks with cursor,
  timestamp when available, render kind, title/text/data, degraded marker,
  `nextCursor`, `hasMore`, and live/source status.

Integration boundaries:
- Backend route helpers belong in existing daemon routing patterns. Do not add
  an unauthenticated route or a route outside the protected router.
- WorkRoot id resolution must reuse daemon-owned opened-workRoot state. Browser
  route params must not infer host paths.
- Existing named-agent wsstate layout derivation in `work_root_activity.rs` must
  remain compatible with primary and linked Git workRoots.
- Synchronous fs/git/cache scanning must stay off Axum async workers.
- Frontend helpers should follow `workRootActivity.ts`/`workRootFiles.ts` style:
  typed route builders, fetch wrappers, opaque workRoot ids, and no browser
  route authority.

Forbidden temporary wiring:
- Do not return raw `agent.json`, `current/state.json`, stream files, markdown
  logs, stdout/stderr files, or backend-native session records to the browser.
- Do not use mock-only data as the canonical route source.
- Do not add visible UI controls or raw callback wiring for Activity Console.
- Do not broaden the daemon into ws MCP/named-agent session authority.

## Integration Test Instructions

Required backend coverage:
- Core serde test for camelCase Activity Feed/Transcript shapes and private
  field redaction.
- Daemon route tests for unauthenticated rejection and paired-cookie success.
- Unknown workRoot and unknown activity id behavior.
- Empty/no-agent workRoot behavior.
- Named-agent feed projection for primary and linked Git workRoots.
- Malformed named-agent records degrade without whole-feed failure.
- Private-field redaction for feed and transcript responses.
- Ordering case proving live/attention/blocked/failed/recent beats A-Z.
- Transcript backfill bounds: limit/cursor behavior, empty transcript, and
  unavailable/degraded transcript source.

Required frontend coverage:
- Route helper tests for feed and transcript endpoints with encoded opaque ids.
- Fetch helper error handling.
- Type/helper tests proving response shape can be consumed without host paths.

Verification commands:
- `cd ws-dashboard && cargo test -p ws-dashboard-core activity`
- `cd ws-dashboard && cargo test -p ws-dashboard-daemon work_root_activity`
- `cd ws-dashboard && cargo test -p ws-dashboard-daemon routes::work_root_activity`
- `cd ws-dashboard/frontend && npm run test:work-root-activity`
- `cd ws-dashboard/frontend && npm run build`

Run broader `cd ws-dashboard && cargo test` if route/module changes touch shared
daemon state, auth, resources, or core exports beyond the activity surface.
Run browser verification only if visible UI behavior changes; this slice should
avoid visible UI changes.

## Implementation Strategy Decisions

- Extend the existing WorkRoot Activity projection surface rather than adding a
  parallel mock-only Activity Console route.
- Treat the existing named-agent projection as the first source adapter feeding
  source-neutral Activity Items.
- Keep the current route family workRoot-scoped. If the existing
  `/api/dashboard/work-roots/{workRootId}/activity` response evolves into the
  feed snapshot, maintain old consumers or update them atomically.
- Represent transcript blocks as normalized read-model output, not source file
  passthrough.
- Use explicit degraded/unavailable states for malformed records and missing
  transcript sources.

## Rejected Alternatives

- Building the Activity Ribbon UI first: rejected because the UI shell depends
  on the read-model contract.
- Keeping a named-agent-only shape: rejected because later main-agent, exec,
  diagnostic, and native transcript sources must fit the same contract.
- Returning backend-native transcript/cache data: rejected because it leaks
  private implementation details and makes UI rendering source-specific.
- Adding agent controls or exec job execution: explicitly outside the read-only
  Activity Console milestone.

## Approach

- Start from `ws-dashboard/crates/core/src/activity.rs` and add/reshape public
  Activity Feed and Transcript types.
- Extend `ws-dashboard/crates/daemon/src/work_root_activity.rs` to produce feed
  items from existing named-agent rows, preserve recent-limit behavior where
  useful, and add transcript lookup/backfill for named-agent activity ids.
- Wire any new transcript route through the existing protected router and
  opened-workRoot resolution.
- Update frontend `workRootActivity.ts` types/helpers/tests to consume the new
  feed and transcript API.
- Keep old Activity pane code compiling; if its current named-agent view shape
  changes, provide a compatibility projection or update only helper-level data
  mapping without redesigning the visible UI.

## Constraints

- No host/cache/session/process/path leakage in public payloads or diagnostics.
- No visible UI redesign in this phase.
- No stream/watch behavior in this phase.
- No agent or exec controls.
- Preserve dashboard command-path invariant for any visible control accidentally
  touched; the expected implementation should not add new visible controls.
- All AI-authored docs/comments must be English.

## Out of scope

- Activity Console ribbon/transcript React layout.
- Live update stream and frontend dirty/acknowledgement behavior.
- Native transcript resolver expansion for Codex, Claude, Gemini, or exec.
- Browser screenshots unless UI behavior changes.

## Details

The implementation should keep the public vocabulary aligned with the ticket:
`ActivityFeed`, `ActivityItem`, `ActivityTranscript`, and `TranscriptBlock`.
Exact Rust/TypeScript names may follow local style, but the serialized JSON
must stay camelCase and source-neutral.

The first feed source is ws named-agent state under daemon-owned wsstate/wsagent
cache layout. Existing cache-path and Git worktree-key derivation rules remain
private implementation details. The browser receives only bounded source
display metadata and opaque activity ids.

Transcript backfill can be minimal for named agents as long as it is normalized,
bounded, cursor-addressable, and honest about unavailable or unsupported source
states. It must not expose source file paths or raw cache record paths.

## Verification Contract

The implementation is acceptable only when the listed Rust and frontend tests
pass after the final code changes. If any command is skipped, the implementer
must report the exact blocker and residual risk. Build warnings that predate
this work may be reported but should not mask new failures.

## References

- `ai-docs/spec/ws-web-dashboard/index.md` — [Must]
  `{#260521-ws-dashboard-activity-console-read-model}` and existing WorkRoot
  Activity projection specs.
- `ai-docs/tickets/ready/260518-feat-ws-dashboard-activity-read-model.md` —
  [Must] target ticket and Phase 1 verification expectations.
- `ai-docs/tickets/todo/260518-epic-ws-dashboard-activity-console.md` —
  [Must] Activity Console vocabulary, read-only scope, and cross-child
  constraints.
- `ai-docs/mental-model/ws-web-dashboard.md` — [Must] route/auth, redaction,
  WorkRoot Activity coupling, and dashboard command-path rules.
- `ai-docs/mental-model/named-agent-runtime.md` — [Must] wsstate/wsagent layout
  and named-agent state semantics.
- `ws-dashboard/crates/daemon/src/work_root_activity.rs` — [Must] existing
  named-agent projection and wsstate layout derivation.
- `ws-dashboard/crates/daemon/src/router.rs` — [Must] protected route wiring.
- `ws-dashboard/crates/daemon/tests/routes.rs` — [Must] route auth and
  WorkRoot Activity test patterns.
- `ws-dashboard/crates/core/src/activity.rs` — [Must] current public activity
  serde types.
- `ws-dashboard/frontend/src/workRootActivity.ts` — [Must] current frontend
  helper/types.
- `ws-dashboard/frontend/src/workRootActivity.test.ts` — [Must] frontend helper
  tests to extend.
- `ws-dashboard/crates/core/src/events.rs` and
  `ws-dashboard/crates/daemon/src/events.rs` — [Maybe] cursor/backfill fixture
  precedent if useful.
