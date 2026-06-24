---
title: ws dashboard ferrule-backed session binding
parent: 260622-epic-ws-dashboard-session-key-realignment
related:
  260514-epic-ws-web-dashboard-mvp: predecessor dashboard board with pre-session-key direction
  260605-epic-ws-playbook-factory-pivot: pivot epic that introduced session-key root authority and mercenary reshape
  260605-research-ws-native-subagent-pivot: research anchor for spawn removal, actor removal, and dashboard retention tension
  260617-refactor-mcp-stateless-subagent-context: made session keys filesystem-backed and restart-resolvable
  260620-feat-ws-dashboard-agent-client-activity-sources: deferred Activity adapter ticket that must build on this binding model
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: serverId routing boundary for linked-server session behavior
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
---

# ws dashboard ferrule-backed session binding

## Background

The dashboard direction needs re-grounding after the ws playbook-factory pivot.
The pivot removed actor-scoped setup/root defaults, renamed the retained ws-owned
agent lifecycle to `ws.mercenary.*`, made `session_key` mandatory for
root-aware MCP tools, and moved session keys into restart-resolvable filesystem
records. The dashboard retained its browser UI/UX direction, but much of its
agent/activity planning still came from the older actor/named-agent model.

This research ticket captures the intended dashboard session-binding model from
the 2026-06-22 discussion so future sessions do not have to reconstruct it.

## History Evidence

- `260609-refactor-ws-spawn-runtime-deletion-session-auth` changed ws from
  actor/root defaults to mandatory per-call `session_key`, removed actor and
  child-actor authority, and kept the dashboard compiling.
- `9649a4bf` removed actor-model coupling from the runtime path; dashboard
  production reads did not depend on `actor_id`, and `ec2ad888` only removed a
  stale dashboard route-test fixture field.
- `260617-refactor-mcp-stateless-subagent-context` moved session keys from an
  in-memory map to flat filesystem records under `<cache-root>/keys/<key>.json`,
  so a fresh MCP process or restarted lead can resolve a known key.
- `260617-refactor-ws-session-bootstrap-obscurity` renamed the bootstrap tool to
  `ws.ferrule` and intentionally kept the name obscure in ordinary runtime
  guidance.
- `260619-feat-ws-session-lineage-children` added `parent_session_key` and
  `session.children`, making parent-to-child key lineage available for workflow
  coordination.
- `260611-refactor-ws-tier-taxonomy-delegate-tier-routing` Phase 7 hard-renamed
  `agents.*` to `ws.mercenary.*` without a compatibility alias.

## Settled Model

Top-level dashboard-launched harness sessions are the lead key owners. The
dashboard does not create an extra harness-less root/control key above them.

When the owner opens a top-level harness session from the dashboard, the target
daemon calls `ws.ferrule(root)` in its local ws MCP environment, receives a
lead-scoped `session_key`, injects it into the harness launch context, and stores
it in daemon-private binding state. The daemon may use the same binding to render
dashboard Activity/status, but it must treat the key as an opaque credential
issued and resolved by ws MCP.

Delegate or leaf keys are not dashboard-level roots. They are derived inside the
top-level harness workflow through the existing render/delegation path, such as
`playbook.render(root_override)` when a delegate playbook needs a scoped child
key. `parent_session_key` is therefore useful for harness-owned lineage, not for
a separate dashboard parent key that sits above every top-level session.

## Identity Terms

- `wsSessionKey`: daemon-private ws MCP session key returned by `ws.ferrule`.
  It is authority for root-aware ws MCP calls and must not be exposed in browser
  payloads, command logs, route ids, or diagnostic text.
- `providerSessionId`: provider-native Codex/OpenCode/harness session, thread,
  or rollout id. It may be needed for adapter lookup or transcript backfill, but
  it is not ws MCP authority and should stay daemon-private unless a future
  browser contract explicitly defines a bounded display form.
- `activityId`: browser-facing dashboard opaque id for Activity rows and
  transcript fetches. It is the stable public id the UI should use.
- `serverId` and `workRootId`: dashboard-owned routing and resource identity.
  They scope activity/session identity before any provider-native or ws-native
  id is considered.

The existing dashboard Codex transcript backfill uses a provider-native
`session_id` from wsagent/mercenary metadata to find Codex rollout JSONL files.
That is not the same concept as a ws MCP `session_key`.

## Linked-Server Boundary

The dashboard does not need a separate remote session model. Linked-server
operation is transparent at the API layer: the browser talks to the local
gateway, the local gateway forwards to the selected server daemon, and the target
daemon performs its own local root/session behavior.

The constraint is execution locality. A local gateway must not mint a ws key for
a remote root path. For a remote workRoot, the selected remote daemon calls its
local ws MCP `ws.ferrule(root)` and owns the resulting daemon-private binding.
The browser continues to address the result by `serverId`, `workRootId`, and
`activityId`.

Frontend and persisted dashboard state must include or derive `serverId` when a
bare `workRootId`, `activityId`, terminal id, or provider id could collide across
servers.

## Current Dashboard Impact

The existing Activity Console public shape is reusable: `ActivityFeed.items`,
transcript fetches, SSE events, selection, dirty acknowledgement, and transcript
rendering are already source-neutral enough to carry top-level harness sessions.

The stale source layer is the daemon projection. Today
`work_root_activity.rs` and `work_root_activity_registry.rs` derive the wsstate
layout, read SQLite `agent_defs` and `agent_instances`, use `agent_key` and
`state_path`, inspect `current/state.json` and `output.md`, and optionally use a
Codex provider `session_id` for native transcript backfill. The dashboard code
has no direct `ferrule`, `session_key`, or `mercenary` integration yet.

The frontend also still has named-agent-specific compatibility assumptions:
`NamedAgentActivityView`, `ActivityFeed.agents`, merge-by-`agentId`, `agent:*`
ids in fixtures, badge copy such as `agents` and `no agents`, and pane metadata
that renders counts as agents. These can be migrated without replacing the whole
UI if the daemon continues returning opaque `activityId` rows through
`ActivityFeed.items`.

## Design Consequences

- The migration should add a dashboard-private session binding layer before
  provider-specific adapters.
- Browser-visible Activity should become source-neutral first; legacy
  named-agent/mercenary state can remain as a compatibility source.
- Dashboard tests need fixture coverage for the three-id separation:
  `wsSessionKey`, provider session id, and browser `activityId`.
- Error paths must handle `unknown_session` by re-ferruling or marking the
  daemon-private binding stale, without surfacing the key to the browser.
- Any future control action must be explicit owner action. Read-only Activity
  rendering should come first.

## Open Follow-Up

The 2026-06-24 ticket-routing pass imported the realignment epic into the active
dashboard branch, routed the managed CLI terminal ticket under it, and deferred
`260620-feat-ws-dashboard-agent-client-activity-sources` to idea-level Activity
adapter work. The next migration pass should audit dashboard spec and mental
model text for stale named-agent, SQLite, actor, and browser-visible session-key
assumptions before provider implementation begins.
