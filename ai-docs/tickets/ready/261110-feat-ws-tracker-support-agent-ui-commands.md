---
title: "Tracker support agent drives the dashboard UI: the operations a user can do (open search, focus a thread, open a notification), sent as conversation-stream markers"
related:
  261108-feat-ws-tracker-support-agent-chat: "the support conversation and its card channel this ticket extends"
  261110-feat-ws-tracker-search-query-grammar: "the open-search command's query is that ticket's grammar once it lands; the later of the two updates the tracker.ui description"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 8ee60d3e759c24e9
sage-review-design-reviewed: 8ee60d3e759c24e9
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
   notification's target, and switch scope or view, all within the
   conversation's store. Every write
   (accept, status, comment) already has a tracker tool the agent calls
   directly. Rejected: also pressing UI buttons that write, such as accept
   or close ((563) b), which duplicates the tracker tools and puts a write
   behind a UI step.
   **No store switch (user, (581) a):** for an item in another store the
   agent answers in text naming the store, as `tracker.show` already does
   (261108). Each store's drawer hosts its own Chat tab
   (`dashboard.js:2967-2972`), so switching store would close the chat that
   sent the command while the agent's tools stay pinned to its store.
   Rejected: a terminal "switch store and leave the chat" command ((581) b,
   strands the conversation); keeping the chat visible across the switch
   ((581) c, detaches the chat from its drawer and reopens the store
   pinning).
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
   mode outside those lists, an alias not in the conversation's store, or a
   comment id that is not a comment on that item's thread is a tool error
   naming the problem, and no marker is produced. The scope keys and mode
   keys are a Go copy of `SCOPES` / `SCOPE_MODES` (mode keys `list`,
   `kanban`, `gantt`, `calendar`; the `all` scope's tree is `list`), with a
   comment at both sites and a Go test that reads `tracker.js` and fails
   when the two lists differ. A view mode
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
6. **"Open a notification's target" is the focus command with the target's
   item alias and, for a thread event, its comment.** Notifications are
   per-viewer feed entries (`focusTarget(root, evId)`, `tracker.js#L2004`)
   whose IDs no tracker tool exposes, while the agent already reads item
   aliases and thread comments through the tracker tools. Rejected: a
   notification-ID argument, which would need a new tool surface to list
   the viewer's notifications.
7. **A command whose target is hidden by the chat moves the view and
   accepts that the chat's stream closes.** In the narrow layout
   (`tracker.js:1883-1890`) Items and the Chat tab share one pane, so
   search, scope, and view switch the pane to Items, which closes the
   chat's stream (`supShown`, `dashboard.js:3056-3060`); markers after that
   point are met on reopen as replay and stay inert under Decision 5. Focus
   is not one of them: it opens the item window (`openItemWindow`, then
   `focusTarget` for a comment), an overlay that leaves the Chat tab and
   its stream as they are, in both layouts. The
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
- The new tool goes through `hiddenUnlessAllowlisted` like `tracker.show`
  (261108 Result): absent from `tools/list`, refused by `tools/call`, and
  left out of `LeadToolNames` for any session not allowlisted for it, so a
  lead never sees it.
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
at narrow width a search command switches the pane to Items while a
focus command leaves the chat open under the item window; and a search
command shows its results without waiting out the typing debounce.

### Result (4ab4c3fe6) - 2026-11-10

Landed in b181144ee, a860ccf83, d0a7073c6 (develop merge with the search
grammar), 306f7dad1, and 4ab4c3fe6.

- **Tool.** `tracker.ui {op: search|focus|scope|view, query | item
  [+comment] | scope | mode}` (mcp/tracker_ui.go). It exists only for a
  store's support conversation: it is hidden unless allowlisted, sits in
  `SupportWSTools`, and a lead gets a refusal. Checks: an item of the
  conversation's own store, a comment on that item's thread, a scope or mode
  from `wslive.UIScopes` / `UIViewModes`, and a one-line query of at most 200
  runes that `wstracker.ParseQuery` accepts. The answer's first line is
  `ui <op> <arg> [<comment>]`.
- **Channel.** Both backends read result lines through the shared
  `resultTool` / `resultEvents` (wsagent/stream.go). A `ui` line becomes an
  `Event.UI`, which the runner writes as a `kind: "ui"` ring entry
  (wslive/ui.go, shared `lineAfterReply` with cards). The template draws it
  as a muted `.sysline.uiline`; the i18n keys are in all 11 tables.
- **Fire-once (Decision 5).** In the browser, each EventSource treats its
  first `entries` region as history. A ui line acts only when a later region
  adds it (dashboard.js supActUI, then tracker.js `uiCommand`). A command
  that closes the stream stops the rest of its batch.
- **Search.** It sets the Items box and skips the grammar ticket's typing
  debounce through a one-shot `searchState().now` flag. It keeps the open
  item window and keyboard focus.
- **Decision 8.** The tracker.ui description and the support prompt name
  the search grammar and tell the agent to find items with `tracker.query`
  `q` first.

Verification:
- **Go tests** (remote ws-rgo, the lead's routing during the machine-load
  incident): `go test ./internal/wslive/ ./internal/wsagent/ ./internal/mcp/
  ./internal/wsdashboard/ ./internal/wsi18n/ ./internal/wstracker/
  ./cmd/ws-mcp/ -count=1` all pass at 306f7dad1. mcp and wsagent pass again
  at 4ab4c3fe6. `go vet` on the touched packages is clean, and so is
  `node --check` on dashboard.js and tracker.js. This covers the packages the
  change touches, not a full `./...`, per the lead's instruction.
- **Headless check** on an isolated dev daemon (own WS_CACHE_HOME and
  WS_CONFIG_HOME, port 39611, binary at 306f7dad1). Commands came from one
  real claude run plus ring-file injection followed by /nudge. Results:
  - Real run: a claude support turn's `tracker.ui search` filtered the rows
    from 3 to 1.
  - Live commands acted in both chat tabs and not in a Notify tab.
  - Reload, tab switch, and drawer reopen kept the lines inert with stable
    keys, and the next live line acted.
  - Wide focus opened #tkwin on the comment (fx-flash) with the chat still
    live.
  - At 900 px wide, search switched the pane to Items and closed the
    EventSource, while focus left narrow=chat with the stream open.
  - The search request went out 3.7 ms after the line appeared, against
    252 ms for typed input.
  - No pageerror lines.

Decisions:
- The `ui` line carries no store. The tool checks the item against the
  conversation's own store, and the dashboard acts only when that chat's
  store is shown.
- The Go copy of the scope and mode lists lives in wslive. A test reads the
  embedded tracker.js to keep the two in sync.
- Lite review (one pass) raised one Important finding. In the narrow layout,
  the prompt's scope-then-search sequence loses the second command, because
  the first one closes the stream. Fixed on the prompt side: at most one
  search, scope, or view per reply, said in the prompt, the description, and
  the tool's answer. A scope argument on `search` was rejected: the query is
  the rest of the line, so a second argument has no clean delimiter.
- From the same review's Minor findings, the text fixes were taken: a team
  scope that the panel does not offer is a no-op, and a subagent's call does
  not move the view. Two Minors were left as they are: the 404-retry
  first-turn race and the drift-test text span. Both are rare and consistent
  with Decision 5.

## Sage Review Round 1 (2026-11-10)

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
