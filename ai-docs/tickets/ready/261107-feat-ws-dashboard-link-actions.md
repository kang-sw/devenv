---
title: "Dashboard: link click actions, file view route, and mermaid in the conversation"
parent: 261107-epic-ws-dashboard-messenger-round2
related:
  261107-feat-ws-dashboard-conversation-history: prerequisite - finished messages draw mermaid once through its per-entry render cache and keyed merge
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: cce0c1bf7289ab86
sage-review-completeness-reviewed: cce0c1bf7289ab86
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
- **File view reach on LAN** (user, 2026-11-07, "(79) a"): the file view
  serves every file inside the session root (`.env`, credentials,
  `.git/config` included) to loopback viewers and LAN token holders alike.
  Rationale: the LAN token is the user's own and already grants message and
  steer to the session, whose lead can read any file anyway. Rejected: LAN
  viewers limited to files git does not ignore; file view loopback-only.
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
  rules (`access.go`: LAN token and loopback Host guard; the Origin check on
  writes is `sameOrigin` in `inbox_routes.go`, agents-plugin-tool/internal/wsdashboard/inbox_routes.go#L105-L118).
- System open is enforced on the server, not only by hiding the menu item:
  the handler refuses unless `localViewer(r)` and `sameOrigin` hold, parses
  the target strictly (http(s), or the daemon's own file-view path), and runs
  the open command through exec with no shell. On Windows do not use
  `cmd /c start` (a `&` in a URL query injects a command); use
  `rundll32 url.dll,FileProtocolHandler` or an equivalent.
- File view of other kinds: a binary non-image file or a file over a size cap
  gets a "not viewable" page with the copy actions only. Text is served with
  `text/plain`, `nosniff`, and the image route's CSP, so a `.html` or `.svg`
  file never runs in the daemon origin; symlinks are refused as the
  attachment route does (Lstat). A rendered `.md` file view stays in escaped
  mode (file content is not a lead reply, see
  261107-feat-ws-whiteboard-now-markdown); the implementer picks one base for
  its relative links and tests it.
- Cmd/Ctrl+click on a path outside the session root opens the menu (copy
  items only); the relative-copy item is hidden there.
- The click handler keys on a marker the renderer emits on links it resolved,
  not on every `a[href]`, so raw anchors the renderer did not resolve (the Now
  slot's inline HTML) are not intercepted; reply HTML links do go through
  `resolveTarget` (261107-feat-ws-whiteboard-now-markdown, "(81) a") and get
  the menu.
- Mermaid in entries: `dashboard.js` draws mermaid only after a board swap
  today; the draw call must also run after the entry merge and after the
  scroll-up history insert (it already skips `[data-processed]`).
- The new routes are added to `TestLANModeGuardsEveryRoute`.
- Landing order with 261107-feat-ws-whiteboard-now-markdown is free, but that
  ticket lets lead-reply HTML into the same page origin: whichever lands
  second keeps its reply sanitizer and these system-open checks together, and
  the sanitizer tests include a vector aimed at system open. Both tickets add
  `RenderText`/`TextTargets` options and split the reply render into
  streaming and finished variants; the second resolves that as a merge.

## Prior Decisions

- 261107-feat-ws-dashboard-conversation-history (2026-11-07, ticket close 1cb3638d): "the worker's forward merge ... conflict resolutions keep both history and attachment routes, entries cache plus buildTime fields, thumbnails plus data-entry keys" - bearing: supports
- 7676c013 (2026-11-07, commit): "Mermaid fences in a bubble render as a plain code block (new unexported pageData.noMermaid). Rejected: leaving pre.mermaid, because the page script draws every pre.mermaid in the document." - bearing: constrains
- 261107-feat-ws-dashboard-composer-image-attach (2026-11-07, 0801c68d): "Attachment route uses Lstat (symlinks refused) plus the name grammar; access rules come from the global hostGuard/lanGuard like the image route. Added to TestLANModeGuardsEveryRoute." - bearing: constrains
- 261106-feat-ws-dashboard-browser-interaction (2026-11-07, 090f4d00): "Paths the route would refuse by name (outside the root, no image extension) return "" from ImageSrc and stay path text, so the page never shows a broken image for a path it already knows is refused." - bearing: constrains
- 261106-feat-whiteboard-rich-items (2026-11-06, Decisions): "Image link rule. Image paths follow the panel ticket-row link rule (8a926fd83, `whiteboardFileLink` in `internal/mcp/whiteboard_panels.go`), which keys on where the page is written" - bearing: constrains
- 261106-feat-whiteboard-rich-items (2026-11-06, Decisions): "Mermaid asset lifecycle. The shared mermaid file name carries the embedded asset's version hash and is written only when missing; a page loads it only when it contains a mermaid block." - bearing: constrains
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Result 2f1ea3cc3): "`View.FileText` and `PanelRow.Path` render file targets as path text." - bearing: supports
- 3d92b646 (2026-11-07, commit): "User answered board items (48)-(58) on 2026-11-07: link menu + file view route (53a) + system open (54a) + conversation mermaid for finished messages as one epic child" - bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard (routes, assets, pages.go, pageview.go), agents-plugin-tool/internal/wswhiteboard (markdown.go, markdown_text.go, render.go) |
| scope.surface | cross-module | wsdashboard routes and assets plus wswhiteboard RenderText/TextTargets/pageData link targets; two new HTTP routes |
| scope.new_public_symbol | yes | new file view and system-open route handlers and TextTargets/link-target fields for served pages, exact names not fixed by the ticket |
| scope.new_type_contract | yes | two new route contracts (file view, system open with target allow-list); TextTargets gains link and mermaid inputs |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/image_route_test.go, access_test.go, conversation_markdown_test.go, inbox_routes_test.go; wswhiteboard/markdown_test.go |
| complexity.reuse_points | confirmed | image_route.go relativeInside and containment rule, inbox_routes.go sameOrigin, access.go localViewer, wswhiteboard.RenderText and resolveTarget/fileTarget |
| complexity.side_effect_risk | high | system-open endpoint runs a host OS command; file view reads host files; mermaid toggle changes shared pageData.noMermaid behavior |
| risk.correctness | moderate | root-containment and symlink checks, finished-versus-streaming mermaid split, clipboard fallback and menu JS with no automated browser check |
| risk.fit | moderate | touches shared renderer and the session-page script and per-entry cache; collides with the Now-markdown ticket on the same renderer |
| risk.test | moderate | route and render tests are specified, but the menu and clipboard behavior are manual only |
| risk.security_or_contract | high | host-command execution from a web route, file-read route, LAN exposure, Origin and loopback gating, SVG/HTML served in the daemon origin |

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
