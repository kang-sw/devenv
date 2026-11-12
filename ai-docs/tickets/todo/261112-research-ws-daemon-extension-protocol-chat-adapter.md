---
title: "Daemon extension protocol: the dashboard daemon dials an adapter WebSocket server that bridges an external chat application"
related:
  261107-epic-ws-dashboard-messenger-round2: "dashboard-side messaging UI; not the external bridge"
---

# Daemon extension protocol and chat adapter

## Background

User, 2026-11-12 (whiteboard topic `chat-adapter`): define a protocol that
extends the dashboard daemon. The daemon connects as a WebSocket client to
an adapter server that implements the protocol's interface; the adapter
bridges an external chat application to the daemon. The first adapter
targets the user's company chat, whose client is poor and speaks plain
HTTP: the user intends to reimplement that client in an adapter, working
from the decompiled original.

## Topic 1: what crosses the protocol

- Outward: tracker events (the daemon event hook already delivers `v: 1`
  JSON events to local commands), session status, notifications.
- Inward: chat messages into a session (mailbox, dashboard intake) or into
  the tracker (comments, items).
- Identity and routing: which session or store a chat conversation maps to.

## Topic 2: protocol shape

- Connection direction (daemon dials the adapter), authentication, reconnect
  and backlog delivery, capability negotiation.
- A protocol version from the first release: this is a cross-version
  surface under `ai-docs/manuals/release-compatibility.md`.

## Topic 3: boundary

- This repository ships only the generic protocol and the daemon's client
  side (Architecture Rules 1 and 4); a company-specific adapter lives in a
  separate private project.
- Decompiling the company client may run into its license or company
  security policy; the user checks that before relying on the result.

## Outcome Ledger

### Verified Findings
- The tracker daemon event hook (`tracker-hooks.json`, events with `v: 1`)
  is the existing outward extension point.

### Confirmed Decisions
- Settle the interface's scope through this research before any epic
  (user, (577) c).

### Proposals
- None yet.

### Open Questions
- Topics 1-3 above; which inward targets the first adapter needs depends on
  what the company client actually exchanges.

### Rejected Alternatives
- Fixing scope now as outward-only ((577) a) or bidirectional ((577) b)
  before knowing what the company client exchanges.
