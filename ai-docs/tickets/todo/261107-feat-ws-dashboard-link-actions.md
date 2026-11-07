---
title: "Dashboard: link click actions, file view route, and mermaid in the conversation"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-feat-ws-dashboard-conversation-history: prerequisite - finished messages draw mermaid once through its per-entry render cache and keyed merge
---

# Dashboard: link click actions, file view route, and mermaid in the conversation

## Background

The user wants links in the session page to behave like a desktop app
(2026-11-07): Cmd/Ctrl+click opens a link at once; a single click opens a
context menu with open in a new tab, open in the system browser, copy the
relative path, and copy the absolute path. This should hold in the
conversation and in the whiteboard markdown alike, and the user asked whether
the two could share one markdown renderer, mermaid included.

The renderer is already shared: conversation bubbles render through
`wswhiteboard.RenderText` with the board's goldmark settings (ddb721de1,
`agents-plugin-tool/internal/wswhiteboard/markdown_text.go`). Two differences
remain: the conversation turns mermaid off (`noMermaid: true`, a delegate
plan's choice, not a user decision), and on daemon-served pages file paths
render as plain text with no link (`resolveTarget`/`fileTarget` in
`markdown.go`), because the daemon has no route that serves a file. It has
`/s/<rid>/image` only.

## Decisions

- **One renderer, mermaid on for finished messages** (user, 2026-11-07,
  "(48), (49) ok"): the conversation keeps `RenderText`; mermaid is enabled
  for finished messages only, while a streaming reply keeps a mermaid fence
  as a code block. Rejected: drawing while streaming - the streaming reply
  re-renders every 250 ms and the diagram would redraw each tick.
- **Scope is daemon-served pages** (user, "(50) ok"): the click menu applies
  to the live session page conversation and the served whiteboard. Board
  files written to disk (embeds, Orca) keep today's `file:` links. Rejected:
  the menu on disk files - new-tab and system-browser actions need daemon
  routes a disk file cannot reach.
- **Click model** (user, "(51) ok"): Cmd/Ctrl+click opens immediately in a new
  tab; a single click opens the menu: open in new tab / open in system
  browser / copy relative path / copy absolute path. It applies to http(s)
  links and file paths; for an http(s) link the two path copies are replaced
  by copy URL.
- **Path copy** (user, "(52) ok"): the relative path is relative to the
  session root. Copy uses the Clipboard API and falls back to a textarea copy,
  because a LAN page over plain http is not a secure context and
  `navigator.clipboard` is unavailable there.
- **File view route** (user, "(53) a"): a new daemon route shows a file inside
  the session root only, reusing the image route's session-root rule: text as
  plain text, `.md` through the same markdown renderer, images inline. A path
  outside the session root offers the copy items only. Rejected: serving the
  raw file and letting the browser decide (binary files download, `.md` shows
  unrendered).
- **System browser open** (user, "(54) a"): a daemon endpoint runs the host
  OS open command (`open` on macOS, the platform equivalent elsewhere). The
  menu item shows only when the page itself was opened from the host
  (loopback); its targets are limited to http(s) URLs and the daemon's file
  view URLs; it uses the same Origin check as the existing write routes.
  Rejected: dropping the item - a web page cannot open the system browser by
  itself, and from a phone over LAN the open would land on the host, hence
  the loopback-only display.
- **A ticket, not a delegate** (user, "(57) ok"): two new routes are new
  contracts.

## Constraints

- The file view and system-open routes follow the dashboard's existing access
  rules (`access.go`: LAN token, Origin check on writes).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/ws-mcp.md` (route docs) apply.

## Phases

### Phase 1: Link actions, file view, system open, conversation mermaid

Implement the decisions above in `agents-plugin-tool/internal/wsdashboard`
(routes, page assets) and `internal/wswhiteboard` (link targets on served
pages, mermaid for finished conversation entries).

Verification: route tests for the file view (inside root, outside root,
`.md` rendered, image inline, traversal refused) and for system open
(loopback only, target allow-list, Origin check, the open command stubbed);
render tests that a served page's file paths become actionable links while a
disk board keeps `file:` links; a finished conversation entry draws mermaid
and a streaming one keeps the code block. Manual: Cmd/Ctrl+click and the menu
in a desktop browser, path copy over LAN.
