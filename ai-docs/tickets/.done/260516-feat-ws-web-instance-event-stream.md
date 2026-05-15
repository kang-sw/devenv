---
title: ws web dashboard instance event stream
parent: 260515-epic-ws-web-dashboard-first-visible-substrate
related:
  260514-epic-ws-web-dashboard-mvp: parent dashboard MVP board
  260515-epic-ws-web-dashboard-first-visible-substrate: coordinating first visible substrate epic
  260516-feat-ws-web-resource-view-model-contract: resource ids and instance model this stream references
  260513-feat-async-exec-output-reader: adjacent persisted output and reader-agent pattern
  260514-research-ws-web-dashboard-direction: source research for later PTY, agent, and diagnostic streams
spec:
  - 260516-ws-web-dashboard-instance-event-envelope-fixtures
  - 260516-ws-web-dashboard-authenticated-instance-event-stream-scaffold
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
completed: 2026-05-16
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

### Result (5e3e530) - 2026-05-16

Implemented the shared instance event envelope and deterministic transcript
fixtures. The core crate now exposes event fixture, transcript, event,
category, and payload types that serialize with the dashboard camelCase
contract and carry stream plus resource identity on each individual event.

The daemon now has a fixture-backed transcript provider using
`daemon/tests/fixtures/instance_events.json`. The fixture covers ordinary
output, status transitions, error, terminal end markers, an empty stream, and
cursor backfill behavior. Unknown cursors return an empty backfill rather than
replaying the whole transcript, while unknown stream ids remain missing.

Verification: `cargo test --workspace` passed. Review found missing per-event
stream/resource identity and unknown-cursor full replay; the final commit fixes
both with serialization and backfill regression coverage.

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

### Result (f10e1b8) - 2026-05-16

Implemented the authenticated fixture-backed instance event route scaffold. The
daemon now exposes `GET /api/dashboard/instance-events/{stream_id}` inside the
existing owner-auth protected router. Authenticated callers can fetch all
fixture events or pass an `after` cursor for deterministic backfill; missing
streams return `404`, and unknown cursors return an empty event set rather than
replaying the transcript.

The route remains finite JSON and fixture-backed. It does not bind to live PTY,
named-agent, exec, diagnostic, viewer, or translation sources and does not make
the dashboard daemon ws MCP or named-agent session authority.

Verification: `cargo test -p ws-dashboard-daemon --test routes
instance_event_stream` and `cargo test --workspace` passed. Review found no
issues; residual transport depth such as SSE or WebSocket streaming remains
later work.
