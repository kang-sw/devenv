---
title: ws web dashboard instance event stream
parent: 260515-epic-ws-web-dashboard-first-visible-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260515-epic-ws-web-dashboard-first-visible-substrate: coordinating first visible substrate epic
  260516-feat-ws-web-resource-view-model-contract: resource ids and instance model this stream references
  260513-feat-async-exec-output-reader: adjacent persisted output and reader-agent pattern
  260514-research-ws-web-dashboard-direction: source research for later PTY, agent, and diagnostic streams
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
---

# ws web dashboard instance event stream

## Background

The first visible substrate should not implement full PTY, named-agent,
document-viewer, or diagnostic streaming, but it needs a common event envelope
before those features create incompatible stream shapes. This ticket defines
the minimal stream substrate attached to dashboard instances.

## Decisions

- Stream events should reference opaque server, workspace, workRoot, and
  instance ids from the resource view-model contract.
- The stream is an authenticated dashboard surface and must remain behind the
  owner-auth boundary.
- The first substrate should define envelope, ordering, reconnect/backfill, and
  transcript fixture behavior before implementing feature-specific event
  payload depth.
- PTY bytes, named-agent calls, exec output, diagnostics, document viewing, and
  translation payloads remain later consumers of the shared envelope.

## Phases

### Phase 1: Event envelope and transcript fixtures

Define the event envelope, stream identity, sequence or cursor behavior,
timestamps, event categories, instance references, and error/end markers needed
by later live streams. Add deterministic transcript fixtures that cover
ordinary output, status transitions, errors, reconnect/backfill, and empty
streams.

Success criteria:

- Later PTY, agent, exec, diagnostic, viewer, and translation features can
  reuse the same envelope without redefining stream identity.
- Fixtures are stable enough for frontend rendering and reconnect tests.

### Phase 2: Authenticated stream route scaffold

Expose a minimal authenticated stream route that can serve fixture-backed
events and exercise reconnect/backfill semantics without binding to live PTY or
named-agent sources. Preserve the daemon foundation's WebSocket pre-upgrade
auth requirement if WebSockets are used.

Success criteria:

- Unauthenticated stream callers are rejected before upgrade or stream
  acceptance.
- Authenticated fixture-backed streams can be consumed by the frontend shell.
- The scaffold does not make the dashboard daemon the ws MCP or named-agent
  session authority.
