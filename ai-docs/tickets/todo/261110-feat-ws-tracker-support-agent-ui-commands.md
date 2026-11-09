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

## Constraints

- Tool descriptions and the support profile's prompt are shipped text: read
  `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
  before editing `internal/mcp/` or `internal/wsagent/`.
- The support profile runs with a tool allowlist (`wsagent/claude.go`); a new
  tool must be added there and to the Pi backend.

## Phases

### Phase 1: UI command channel and the first command set

The marker, the fire-once guard, the dashboard handlers, and the commands
settled in the queue below. Verification: `go test ./...` under
`agents-plugin-tool` (stream parsing for both backends, replay not
re-firing), plus a headless-browser check that a command moves only the
tab hosting the conversation.

## Open Decision Queue
Last ID: (567)

(563) [open] [policy] The first command set is navigation only: open the search box with a query, focus an item and its thread, open a notification's target, and switch store, scope, or view.
- Context: (540) asked for "operations the user can do in the UI"; writes (accept, status, comment) are already tracker tools the agent calls directly.
- Alternative a: navigation only, as above.
- Alternative b: a plus pressing UI buttons that write (accept, close), which duplicates the tracker tools and puts a write behind a UI step.
> a: every write already has a tool; the UI channel adds what the agent cannot do today, moving the user's view.
(564) [open] [technical] A new support tool (working name `tracker.ui`) whose result's first line `ui <op> <args>` the stream parses as `cardEvent` parses `card`, over overloading `tracker.show` - keeps "show a card" and "move the view" separate, and the allowlist names exactly what the agent may do.
(567) [open] [announced] The 2026-11-10 tickets (this one, `261110-feat-ws-tracker-idea-status-and-assignment-invariant`, `261110-feat-ws-tracker-search-query-grammar`, `261110-research-ws-dashboard-background-task-visibility`) land in `todo/`, standalone with `related:` links - user, 2026-11-10: "다 todo로요"; ready promotion needs fact population and Sage review, which the user did not ask for tonight.
