---
title: "Tracker support agent drives the dashboard UI: the operations a user can do (open search, focus a thread, open a notification), sent as conversation-stream markers"
related:
  261108-feat-ws-tracker-support-agent-chat: "the support conversation and its card channel this ticket extends"
  261110-feat-ws-tracker-search-query-grammar: "the open-search command's query is that ticket's grammar once it lands; the later of the two updates the tracker.ui description"
sage-review-design: blocked
sage-review-completeness: blocked
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
(`agents-plugin-tool/internal/wsagent/stream.go#L411-L439`, `CardTool`,
`cardEvent`). The event is emitted by both backends
(`wsagent/claude.go#L572-L573`, `wsagent/pi.go#L505-L512`), carried through the
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
   The ops: `ui search <query>` (the rest of the line is the query),
   `ui focus <alias> [<comment-id>]`, `ui scope <mine|received|given|team|all>`
   (the `SCOPES` keys, `tracker.js:262-266`), and `ui view <mode>` (a mode
   the current scope offers, `SCOPE_MODES`). The tool validates op and
   arguments before producing the result line: an unknown op, a scope or
   mode outside those lists, or an alias not in the conversation's store is
   a tool error naming the problem, and no marker is produced. A view mode
   the tab's current scope does not offer is ignored by the tab.
5. **Fire-once is decided by the dashboard tab: a `ui` marker acts only
   when it arrives live, after the tab has loaded the conversation's
   history; a marker met while loading history, on reload, or on
   reconnect replay renders as an inert line and never acts.** Every tab
   that has that support chat open and receives the marker live acts on
   it. Rejected: a server-side consumed flag or seq watermark (new state in
   the conversation ring, and a second tab opened later would still need
   the live/replay distinction); electing one tab among several showing
   the same chat (no tab identity exists to elect by). Uncertain: whether
   two tabs on one chat moving together is wanted; it is the rare case.
   Every `ui` marker renders in the chat as one muted line summarizing the
   command (op and arguments); a live marker renders the same line after it
   acts, so live and replayed history look alike.
7. **A command whose target is hidden by the chat moves the view and
   accepts that the chat's stream closes.** In the narrow layout
   (`tracker.js:1883-1890`) Items and the Chat tab share one pane, so
   search, scope, and view switch the pane to Items, which closes the
   chat's stream (`supShown`, `dashboard.js:3056-3060`); markers after that
   point are met on reopen as replay and stay inert under Decision 5. The
   `tracker.ui` description tells the agent to send a UI command as the
   last step of its reply. Rejected: keeping a hidden chat's stream open
   (changes the stream lifecycle 7e99671b set: open only while the chat tab
   shows).
8. **The open-search query is the Items search box's text, set
   programmatically, and it runs the search at once rather than through the
   typing debounce.** Before `261110-feat-ws-tracker-search-query-grammar`
   lands the box is a substring match; after, it is that grammar. Whichever
   of the two tickets lands second updates the `tracker.ui` description and
   the support prompt (`internal/wsagent/instructions/tracker-support.md`)
   to name the grammar and point at `tracker.query` `q`.
6. **"Open a notification's target" is the focus command with the target's
   item alias and, for a thread event, its comment.** Notifications are
   per-viewer feed entries (`focusTarget(root, evId)`, `tracker.js#L2004`)
   whose IDs no tracker tool exposes, while the agent already reads item
   aliases and thread comments through the tracker tools. Rejected: a
   notification-ID argument, which would need a new tool surface to list
   the viewer's notifications.

## Constraints

- Tool descriptions and the support profile's prompt are shipped text: read
  `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/ws-mcp.md`
  before editing `internal/mcp/` or `internal/wsagent/`.
- The support profile runs with a tool allowlist: `SupportWSTools`
  (`agents-plugin-tool/internal/wsagent/builtin.go#L27-L30`), shared by both
  builtin support profiles and enforced by each backend
  (`wsagent/claude.go#L197-L212`, `wsagent/pi.go#L166-L190`); a new tool must be
  added to that list, admitted by the MCP support gate
  (`agents-plugin-tool/internal/mcp/server.go#L719-L725`), and parsed in both
  the Claude and Pi stream handlers.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Decisions): "Card references come from a designated tool call ... the renderer draws that call as the card-reference entry. Rejected: a text marker inside the reply, fragile to parse and to quote." — bearing: supports
- 7e99671b (2026-11-09, commit): "Cards come from the runner reading tool results, not a new live-intake event kind, so the adapter intake protocol stays unchanged; a card is only ever produced by the runner for its own conversations." — bearing: supports
- 7e99671b (2026-11-09, commit): "The chat reuses the session SSE with a region filter instead of a new chat-specific route; the stream is open only while the chat tab shows." — bearing: constrains
- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Result): "Hidden unless allowlisted (522). tracker.show and git.search go through hiddenUnlessAllowlisted, applied in toolAllowed (tools/list and tools/call) and in LeadToolNames." — bearing: constrains
- 68f4f629 (2026-11-10, commit): "the parsers stay pure event producers (TrackerWrite next to Card); the runner turns it into the Options callback." — bearing: constrains
- 261108-feat-ws-tracker-support-agent-chat (2026-11-08, Decisions): "tracker.show is pinned to the conversation's store: it takes an item ID and no store argument; an item in another store is answered as text, never as a card" — bearing: constrains
- ad198a0f (2026-11-08, commit): "Notification focus: ... a status/accept target is the assignee's row (data-hmember on the row) and focusTarget expands the fold ... when the row is among the hidden ones" — bearing: supports
- 260521-feat-ws-dashboard-command-dispatch-spine (2026-05-21, Decisions): "Activity Console controls added later must dispatch commands such as activity.selectItem ... and existing workbench open/focus commands rather than raw UI callbacks." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wsagent/stream.go, claude.go, pi.go, builtin.go, instructions/tracker-support.md, internal/mcp/ new tool file and server.go, internal/wslive ring, internal/wsdashboard/assets/dashboard.js and tracker.js |
| scope.surface | cross-module | new MCP tool, wslive ring entry kind, dashboard handlers across wsagent, mcp, wslive, wsdashboard |
| scope.new_public_symbol | yes | tracker.ui MCP tool working name, plus a UI-command Event field and ring kind |
| scope.new_type_contract | yes | ui op args result line grammar and a ring entry carrying the command |
| scope.test_surface | existing | internal/wsagent/card_test.go, stream_test.go, internal/wslive/state_test.go, internal/wsdashboard/support_chat_test.go |
| complexity.reuse_points | confirmed | cardEvent and CardTool path in stream.go, wslive RingCard, tracker.js focusTarget and openItemWindow |
| complexity.side_effect_risk | moderate | fire-once guard must survive reload and reconnect; tabs not showing the chat never move |
| risk.correctness | moderate | replay must not re-fire (Decision 5: live-only in the tab) and every tab showing that chat acts on a live marker |
| risk.fit | moderate | adds a second agent-to-UI channel beside cards across four packages |
| risk.test | moderate | needs both backend parsers, replay, and a headless-browser multi-tab check |
| risk.security_or_contract | moderate | new tool must stay hidden unless allowlisted and pinned to the conversation store |

## Phases

### Phase 1: UI command channel and the first command set

The marker, the fire-once guard (Decision 5), the dashboard handlers, and
Decision 3's navigation commands (with Decision 6's notification target)
through Decision 4's tool, plus Decisions 7 and 8. Verification: `go test
./...` under `agents-plugin-tool` (the `ui` result line parsed by both
backends, the tool's refusals, the ring carrying the marker, and a replay
delivering it unchanged), plus a headless-browser check on an isolated
daemon that: a live command acts in every tab showing that chat and in no
other tab; reloading or reconnecting renders the same markers inert; and
at narrow width a search command switches the pane to Items.

## Blocked (2026-11-10)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | 'Switch store' closes the chat that sent the command | important | missing |
| 2 | Narrow layout puts Items and Chat in one pane | important | autonomous |
| 3 | Query syntax of 'open the search box' | minor | autonomous |
| 4 | Cross-ticket [261110-feat-ws-tracker-search-query-grammar, 261110-feat-ws-tracker-support-agent-ui-commands]: open-search command and grammar compose but no related edge | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Store pinning contradicts 'switch store' command | important |
| 2 | Replay no-refire check belongs in headless browser, not go test | important |
| 3 | ui <op> <args> grammar and op set not enumerated; invalid op/alias reporting | minor |
| 4 | Stale route facts | minor |
| 5 | Inert replay rendering unspecified | minor |

## Open Decision Queue
Last ID: (581)

(581) [open] [policy] The first command set drops "switch store"; for an item in another store the agent answers in text naming the store.
- Context: each store's drawer hosts its own Chat tab over that store's conversation (`dashboard.js:2967-2972`), and the agent's tools are pinned to the conversation's store (261108 Prior Decision on `tracker.show`). Switching store closes the drawer showing the chat (`openStore`, `tracker.js:3773-3780`), so the chat that sent the command closes and the agent cannot act in the store it switched to.
- Alternative a: drop store switching from the first set; the agent says in text which store holds the item.
- Alternative b: keep it as a terminal command that switches store and leaves the chat (the new store's chat is a separate conversation).
- Alternative c: keep the hosting chat visible across the switch (the chat detaches from its store's drawer; a layout change and an unpinned conversation).
> Recommend a: the store-pinned conversation (261108) and the per-store drawer both stay as they are; b works but strands the conversation, and c reopens the pinning decision.
