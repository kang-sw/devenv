# Brief: 260622-dashboard-ticket-migration-phase2

## Intent

Clean up dashboard spec and mental-model drift after the session-key/ferrule
realignment. A fresh implementation session should understand that current
SQLite/wsstate named-agent Activity behavior is compatibility behavior, while
future dashboard-launched harness/CLI/provider work must preserve daemon-private
`ws.ferrule(root)` bindings and source-neutral browser Activity identity.

## Scope Boundary

Selected scope: `260622-chore-ws-dashboard-existing-ticket-migration` Phase 2.

In scope:
- Update `ai-docs/spec/ws-web-dashboard/index.md` to make current Activity
  behavior, compatibility projections, deferred provider adapters, and
  daemon-private session-binding boundaries explicit.
- Update `ai-docs/mental-model/ws-web-dashboard.md` with modification-relevant
  rules so future dashboard work does not expose ws session keys or rebuild
  named-agent/SQLite assumptions as the future Activity authority.
- Record the Phase 2 result in the ticket and refresh `_index.md` only if the
  focus/status summary changes materially.

Out of scope:
- No code changes.
- No provider adapter implementation.
- No managed CLI terminal contract-first spec text.
- No browser route or payload schema changes beyond documenting existing and
  accepted boundaries.

## Caller-Visible Contract

Documentation-only change. There is no runtime caller-visible behavior change.
The documented contract should say:
- Browser routes and payloads use dashboard-owned opaque identifiers such as
  `serverId`, `workRootId`, `activityId`, and transcript cursors.
- `wsSessionKey` and `providerSessionId` are daemon-private binding concepts,
  not browser route identity, command payload identity, Activity ids, or
  diagnostic payload fields.
- `ActivityFeed.items` is the forward source-neutral Activity contract.
  `ActivityFeed.agents` and named-agent summary rows are compatibility
  projections for existing UI consumers.
- Current SQLite/wsstate named-agent reading remains implemented compatibility
  behavior. It is not the future authority for Codex app-server, OpenCode ACP,
  or managed vendor CLI sessions.

## Contract Instructions

Edit only documentation files needed for the selected scope.

Preserve implemented spec entries without adding `🚧` planned contract entries.
Use body prose or implementation-gap wording to distinguish current behavior
from deferred provider-adapter scope. Do not imply `260624` managed CLI or
`260620` Activity adapters are already ready to implement.

Do not remove existing anchors or rename spec stems. If text under
`{#260525-ws-dashboard-sqlite-agent-activity-source}` changes, preserve the
anchor and reframe the entry as compatibility source behavior.

## Integration Test Instructions

No runtime tests are required for this documentation-only phase.

Verification required:
- `ws/spec_index.verify`
- `rg` checks over the edited spec and mental model for stale authority wording
  around `SQLite`, `wsstate`, `session_key`, `ferrule`, `ActivityFeed.agents`,
  `providerSessionId`, and `wsSessionKey`
- `git diff` review of edited docs

## Implementation Strategy Decisions

- Keep current implemented SQLite/wsstate projection documented, but name it as
  a compatibility source.
- Add the three-identity split in the dashboard docs:
  `wsSessionKey`, `providerSessionId`, and browser-facing `activityId`.
- Make linked-server behavior local-per-daemon: a selected server daemon owns
  its own ferrule/session behavior; the local gateway does not mint keys for a
  remote root.
- Keep provider adapters deferred to `260620` idea-level work and managed CLI
  terminal work deferred to `260624` todo-level work.

## Rejected Alternatives

- Do not delete existing named-agent/SQLite spec text; that would erase current
  implemented compatibility behavior.
- Do not add a new contract-first `🚧` managed CLI or provider-adapter spec
  entry in this ticket.
- Do not make the dashboard daemon the ws MCP root authority or provider
  session authority in documentation.

## Approach

- Reword WorkRoot Activity and Activity Console spec sections so they distinguish
  public source-neutral Activity shape from legacy named-agent compatibility
  projection.
- Update the SQLite-backed source section to describe the implemented source as
  compatibility metadata, not the long-term Activity authority.
- Add or refine spec prose for daemon-private `wsSessionKey` /
  `providerSessionId` / `activityId` separation and linked-server locality.
- Tighten mental-model contracts and common mistakes around the same boundary.
- Record the ticket result after docs are updated and verified.

## Constraints

- All AI-authored docs stay in English.
- No browser-visible payload should be documented as carrying session keys,
  provider session ids, raw event ids, cache paths, transcript paths, process ids,
  or raw provider records.
- Future provider activity should enter through source-neutral Activity items,
  not by expanding the legacy `agents` projection.

## Out of scope

- Code, tests, fixtures, UI changes, route changes, and provider process
  dogfood.
- Promotion of `260624-feat-ws-dashboard-managed-cli-terminal`.
- Promotion of `260620-feat-ws-dashboard-agent-client-activity-sources`.

## Details

Primary stale areas already identified:
- `WorkRoot Activity Projection`
- `Activity Console Read Model`
- `SQLite-Backed Agent Activity Source`
- `Activity Console Transcript Expansion`
- `ws-web-dashboard` mental-model module contracts, change recipes, and common
  mistakes touching Activity/session authority

## Verification Contract

Pass means:
- `ws/spec_index.verify` reports healthy enough for the edited spec corpus.
- No edited prose presents SQLite/wsstate named-agent metadata as the future
  Activity authority.
- A fresh reader can identify current compatibility sources, deferred provider
  adapters, and daemon-private ferrule/session-binding invariants without the
  original discussion thread.

## References

- [Must] `ai-docs/tickets/ready/260622-chore-ws-dashboard-existing-ticket-migration.md` - selected scope and verification boundary.
- [Must] `ai-docs/tickets/todo/260622-epic-ws-dashboard-session-key-realignment.md` - cross-child identity and authority decisions.
- [Must] `ai-docs/tickets/todo/260622-research-ws-dashboard-ferrule-session-binding.md` - daemon-private ferrule/session-binding model.
- [Must] `ai-docs/tickets/todo/260624-feat-ws-dashboard-managed-cli-terminal.md` - deferred managed CLI scope and bootstrap policy boundaries.
- [Must] `ai-docs/tickets/idea/260620-feat-ws-dashboard-agent-client-activity-sources.md` - deferred structured Activity adapter scope.
- [Must] `ai-docs/spec/ws-web-dashboard/index.md` - target spec.
- [Must] `ai-docs/mental-model/ws-web-dashboard.md` - target mental model.
- [Must] `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md` - migration anchor for spawn-removal, dashboard retention tension, and adapter boundaries.
- [Maybe] `ai-docs/mental-model/mcp-runtime.md` - session-key and `ws.ferrule` runtime authority details.
- [Maybe] `ai-docs/mental-model/named-agent-runtime.md` - current mercenary/named-agent compatibility behavior.
