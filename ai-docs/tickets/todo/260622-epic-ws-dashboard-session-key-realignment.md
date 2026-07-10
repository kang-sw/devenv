---
title: ws dashboard session-key realignment
related:
  260514-epic-ws-web-dashboard-mvp: retired predecessor dashboard MVP board; this epic absorbs its agent-harness/session-key direction
  260710-epic-ws-dashboard-terminal-ux-polishing: sibling successor board that absorbs the dashboard-centric UX/terminal-polish backlog from the same split
  260605-epic-ws-playbook-factory-pivot: epic that changed ws delegation, root authority, actor removal, and mercenary/session-key behavior
  260605-research-ws-native-subagent-pivot: research anchor that briefly deprecated the dashboard before later retention decisions
  260620-feat-ws-dashboard-agent-client-activity-sources: deferred Activity adapter ticket that now routes through this realignment epic
  260525-feat-ws-dashboard-server-scoped-operation-forwarding: linked-server operation-forwarding ticket whose Server Route boundary constrains session binding
related-mental-model:
  - ws-web-dashboard
  - mcp-runtime
  - named-agent-runtime
---

# ws dashboard session-key realignment

## Scope

Realign the ws dashboard direction after the playbook-factory/session-key pivot.
The dashboard remains a browser control plane with reusable UI and terminal,
document, workRoot, Activity Console, and linked-server surfaces, but its agent
and harness integration must stop assuming the pre-pivot actor/named-agent MCP
model.

This epic owns the migration from the older dashboard MVP board into a
session-key-aware model:

- top-level harness sessions launched from the dashboard receive lead-scoped ws
  session keys minted with `ws.ferrule(root)`;
- the dashboard daemon keeps a private binding between dashboard activity/session
  ids, provider-native session ids, and ws session keys;
- browser-facing identity remains dashboard-owned (`serverRoute`,
  `workspaceId`, `workRootId`, `activityId`, transcript cursors);
- existing Activity Console, workbench, terminal, document, and server gateway UI
  should be reused where their contracts still fit;
- old dashboard tickets and specs are audited and migrated away from stale
  actor, named-agent, SQLite-agent-registry, and pre-session-key assumptions.

## Non-Scope

- Implementing Codex app-server or OpenCode ACP provider adapters directly in
  this epic ticket.
- Exposing ws session keys, provider session ids, cache paths, transcript paths,
  process ids, or raw provider event ids to browser payloads.
- Introducing a remote-only session model. Linked servers continue to work by
  routing browser requests through the local gateway to each target daemon, and
  each target daemon owns its local ws/ferrule behavior.
- Making the dashboard the ws MCP root authority, model backend authority, or
  hidden orchestrator above top-level harness sessions.
- Replacing the dashboard UI/UX wholesale. The migration should preserve usable
  surfaces and replace only stale source and authority layers.

## Child Tickets

- `260622-research-ws-dashboard-ferrule-session-binding` - captures the
  ferrule-backed dashboard session-binding model, terminology, history evidence,
  linked-server boundary, and current implementation impact.
- `260622-chore-ws-dashboard-existing-ticket-migration` - migration scaffold for
  importing this board into the active dashboard branch, routing existing
  dashboard tickets through the session-key-aware direction, and later cleaning
  stale spec/mental-model text.
- `260624-feat-ws-dashboard-managed-cli-terminal` - first concrete todo child
  after the 2026-06-24 direction adjustment: shared PTY text I/O, managed
  Codex/Claude/OpenCode-style CLI terminal surface, browser-side long-text
  composer, and explicit ferrule/bootstrap submit policy.
- Existing: `260620-feat-ws-dashboard-agent-client-activity-sources` is deferred
  back to idea-level Activity adapter work until the managed CLI path is
  dogfoodable; future provider adapters must build on the session-binding model
  instead of the pre-pivot named-agent/SQLite authority assumptions.
- `260525-feat-ws-dashboard-server-scoped-operation-forwarding` - now an
  explicit child of this epic (re-parented from the retired MVP board); this
  epic should treat Server Route scoping as an identity constraint rather than
  a separate remote session model.
- `260514-epic-ws-web-dashboard-mvp` is retired (`.done/`), split into this
  epic (agent-harness/session-key direction) and
  `260710-epic-ws-dashboard-terminal-ux-polishing` (dashboard-centric
  UX/terminal-polish direction). Reusable workbench, PTY, document, WorkRoot,
  Activity, and server gateway surfaces the old board delivered remain
  available to build on; this epic owns only the session-key-aware agent/
  harness direction over them.

## Cross-Child Decisions

- A top-level dashboard-launched harness session is the lead key owner. The
  dashboard should not create a separate harness-less root/control key for that
  session.
- The daemon obtains a top-level harness key by calling `ws.ferrule(root)` in the
  target daemon's local ws environment, injects that key into the harness launch
  context, and stores it only in daemon-private binding state.
- `wsSessionKey`, provider-native `providerSessionId`, and browser-facing
  `activityId` are distinct concepts and must stay distinct in code, tests,
  docs, and payloads.
- `parent_session_key` is not a dashboard-level parent/root registry for
  top-level harnesses. It remains the lineage mechanism for harness-created
  child/delegate/leaf keys, especially through `playbook.render` and workflow
  delegation.
- Linked-server behavior stays local-per-daemon: the browser talks to the local
  gateway, the local gateway proxies to the selected server daemon, and the
  selected daemon mints and resolves ws session keys for its own local roots.
- Browser-visible Activity should use `ActivityFeed.items` and source-neutral
  transcript blocks as the public shape. Legacy `agents` projections are
  compatibility data, not the future authority for new provider sessions.

## Completion Criteria

- Done: the dashboard ticket/spec/mental-model set has been migrated so future
  implementation sessions can proceed from the ferrule-backed top-level harness
  model without re-deriving the session-key, provider-session, and Server Route
  boundaries.
- Dropped: a later decision retires the browser dashboard or replaces the
  ferrule-backed harness-session model with a different accepted architecture.
- Deferred: provider-specific implementation, browser control actions beyond
  read/observe, credential persistence, deployment automation, and broader
  visual/UX refresh may remain in child tickets or future epics.
