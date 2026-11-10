---
title: "Tracker support agent drives the dashboard UI: the operations a user can do (open search, focus a thread, open a notification), sent as conversation-stream markers"
related:
  261108-feat-ws-tracker-support-agent-chat: "the support conversation and its card channel this ticket extends"
---

# Tracker support agent drives the dashboard UI

## Background

User, 2026-11-09/10 whiteboard ((539), (540)): the store's support agent
should be able to operate the dashboard on the user's behalf, such as
opening the search box, focusing a thread, or opening what a notification
points at. The direction is to expose the operations a user can perform in
the UI, rather than to give the agent features of its own.

The support chat already has one agent-to-UI channel: a `tracker.show` tool
result whose first line is `card <store> <alias>` becomes a card event
(`agents-plugin-tool/internal/wsagent/stream.go:410-436`, `CardTool`,
`cardEvent`). The event is emitted by both backends
(`wsagent/claude.go:573`, `wsagent/pi.go:512`), carried through the
conversation ring (`wslive`), and drawn by the dashboard.

## Decisions

1. **UI commands travel as markers in the conversation stream, the same path
   as cards, not as a new SSE event (user ack, (539)).** A command is bound
   to its conversation and so to the tab showing that support chat; other
   devices and tabs do not move. Each command fires once: replaying the
   conversation (reload, reconnect, history) must not run it again.
2. **The command set mirrors operations the user can do in the UI (user,
   (540) note).** Rejected: a notification filter the agent applies
   ((540) a, and the persistent mute rules of (540) b). For "look at
   these", the agent shows cards in the chat instead.
3. **The first command set is navigation only (user, (563) a):** open the
   search box with a query, focus an item and its thread, open a
   notification's target, and switch store, scope, or view. Every write
   (accept, status, comment) already has a tracker tool the agent calls
   directly. Rejected: also pressing UI buttons that write, such as accept
   or close ((563) b), which duplicates the tracker tools and puts a write
   behind a UI step.
4. **A new support tool (working name `tracker.ui`) carries the commands
   ((564)):** its result's first line `ui <op> <args>` is parsed by the
   stream the way `cardEvent` parses `card`. Rejected: overloading
   `tracker.show`; keeping "show a card" and "move the view" separate lets
   the allowlist name exactly what the agent may do.

## Constraints

- Tool descriptions and the support profile's prompt are shipped text: read
  `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
  before editing `internal/mcp/` or `internal/wsagent/`.
- The support profile runs with a tool allowlist (`wsagent/claude.go`); a new
  tool must be added there and to the Pi backend.

## Phases

### Phase 1: UI command channel and the first command set

The marker, the fire-once guard, the dashboard handlers, and Decision 3's
navigation commands through Decision 4's tool. Verification: `go test ./...` under
`agents-plugin-tool` (stream parsing for both backends, replay not
re-firing), plus a headless-browser check that a command moves only the
tab hosting the conversation.
