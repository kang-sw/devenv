---
title: "Dashboard agent host: a browser terminal on holder-owned PTYs behind the trusted-origin boundary"
parent: 261110-epic-ws-dashboard-agent-host
blocked-by: 261110-feat-ws-dashboard-session-holder
related:
  261110-research-ws-dashboard-agent-host-foundations: "Topic 1 (archived ws-dashboard terminal)"
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope (no browser terminal) is reopened here"
---

# Dashboard terminal

## Background

Track B of the epic: restore a terminal in the dashboard. The archived
ws-dashboard (tag `archive/ws-dashboard`, `bc2e54e89`) had one: Rust,
`portable_pty`, a websocket transport with output backfill, shell selection,
and tab restore as new shells after a daemon restart. Tickets in `.done/`:
`260516-feat-ws-web-terminal-session-substrate`,
`260516-bug-ws-web-terminal-websocket-transport`,
`260516-bug-ws-web-terminal-cross-platform-portability`,
`260523-feat-ws-dashboard-terminal-tab-restore`,
`260620-bug-dashboard-terminal-inherits-dumb-term`.

## Decisions

1. **PTYs live in the session holder, so shells survive a daemon restart
   (user, (06) a; technical, (09)).** Each terminal is a holder-owned PTY
   (`261110-feat-ws-dashboard-session-holder`); after a restart the daemon
   reattaches instead of creating new shells.
2. **The archived Rust PTY code is not carried over (announced, (23)).** Only
   its frontend and websocket protocol design are references. The holder
   decision made the research's "port to Go or run Rust as a sidecar"
   question moot.
3. **Terminal access uses the approval trust boundary (technical, (05)).** A
   browser terminal is arbitrary shell execution, more than the tool
   approval that `261106-epic-ws-dashboard-serve-daemon` 241 b refused to
   put behind the LAN token. By default it is available only from loopback
   or a desktop-shell origin, the same boundary as approval in
   `261110-feat-ws-dashboard-headless-lead-sessions`. Rejected: a separate
   terminal boundary (the weaker of two boundaries becomes the real one).
4. **Each terminal is a card in the board column (user, 2026-11-10 whiteboard
   note; (37) a and note).** Terminals sit among the whiteboard's topics in
   the right column. Pinning a terminal card fixes it at the top. An
   unpinned card keeps its place in the topic flow: its own activity, such
   as a command finishing, never moves it up; it only moves down as updated
   topics rise above it. Rejected: unpinned terminals grouped at a fixed
   position under the pinned ones ((37) b).
5. **Gate: `localViewer` plus `sameOrigin` (technical, (101) ok).** The
   terminal routes and the websocket upgrade answer only a browser on this
   machine whose Origin is the daemon's own; a LAN viewer gets 404, as on
   the system-open route. Recognizing a desktop-shell origin waits until a
   shell exists ((19)). Rejected: an extra token; the archive's token was its
   general dashboard auth (owner cookie or bearer, plus loopback Host and
   Origin), which today's loopback check replaces.
6. **xterm.js, vendored like mermaid (technical, (102) ok).** `@xterm/xterm`
   5.x with `addon-fit`, added the way `internal/wswhiteboard/assets/mermaid/`
   is (npm pack, single-file dist plus LICENSE, hash-named served file).
   Rejected: another emulator; a bundler.
7. **The daemon picks the shell (technical, (103) ok).** The archive rule:
   `$SHELL`, then `/bin/sh` on POSIX; `pwsh.exe`, `powershell.exe`,
   `%COMSPEC%`, then `cmd.exe` on Windows; an empty or `dumb` TERM becomes
   `xterm-256color`. The daemon passes the result to the holder as argv and
   env, and there is no shell picker in Phase 1. Rejected: a holder-side
   rule (the holder stays a generic process owner).
8. **Scrollback (technical, (104) ok).** The client keeps 5000 lines; the
   backfill after a reattach comes from the holder's output ring
   (`261110-feat-ws-dashboard-session-holder` Decision 7). Rejected: a second
   daemon-side buffer.

## Prior Art

- The trusted-origin gate already exists for one host-executing route: the
  system-open route (`internal/wsdashboard/sysopen_route.go`) answers only a
  browser on this machine (`localViewer`, `access.go`) whose Origin equals
  the daemon's own (`sameOrigin`, `inbox_routes.go`); a LAN viewer gets 404.

## Constraints

- The terminal is independent of daemon lead sessions; it depends only on
  the holder.
- The working directory comes from the workspace model
  (`261110-feat-ws-dashboard-workspaces`) once it exists; before then, the
  terminal opens in a folder the user picks.
- Before promotion, an HTML mockup settles the card's size and expansion,
  how a fit resize reaches the PTY, and where a terminal is opened from
  before workspaces exist (user, (90) a).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/windows-dogfood.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "One trusted-origin boundary. Tool approval and the terminal are both execution authority and share one boundary: loopback or a desktop-shell origin by default, never the LAN token alone." — bearing: supports
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "The terminal runs on holder PTYs behind the trusted-origin boundary; the archived Rust PTY code is not carried over ((05), (23))" — bearing: supports
- 261110-feat-ws-dashboard-session-holder (2026-11-10, Decisions): "PTY: `pty` on POSIX, ConPTY through `ctypes` on Windows. The standard library `pty` module is POSIX-only; on Windows the holder calls `CreatePseudoConsole` ... through `ctypes`." — bearing: constrains
- 0336011f (2026-11-07, commit): "Non-local viewers get 404 from the open route (not 403) so the route looks absent to a LAN token holder and TestLANModeGuardsEveryRoute stays uniform" — bearing: constrains
- 261110-bug-ws-dashboard-sse-connection-budget (2026-11-10, Decisions): "No new module dependency. The WebSocket server uses `golang.org/x/net/websocket` ... adding another module needs its reason in the commit's `## AI Context`." — bearing: constrains
- 2abc8bc8 (2026-06-20, commit): "The fix keeps explicit capable parent TERM values but normalizes missing, empty, or dumb values to xterm-256color at PTY spawn time" — bearing: constrains
- 260523-feat-ws-dashboard-terminal-tab-restore (2026-05-23, commit facfac97): "The implementation intentionally recreates new daemon terminal sessions instead of pretending PTY processes survive daemon death." — bearing: constrains
- 260514-epic-ws-web-dashboard-mvp (2026-05-14, Cross-Child Decisions): "Keep terminal nvim available as an optional PTY workflow, but do not make it the primary editor path because Windows PTY behavior is a known stability concern." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | new terminal routes beside agents-plugin-tool/internal/wsdashboard/sysopen_route.go and access.go, plus assets/dashboard.js, dashboard.css, pages.go embed list, and the holder attach client |
| scope.surface | cross-module | daemon HTTP and websocket routes in wsdashboard, the dashboard frontend, and the session holder attach protocol from 261110-feat-ws-dashboard-session-holder |
| scope.new_public_symbol | yes | new browser-facing terminal routes and a terminal websocket; exact names unsettled until promotion |
| scope.new_type_contract | yes | a terminal frame protocol for output backfill, input, resize, and status, modeled on the archived terminal.rs websocket design |
| scope.test_surface | new-files | no terminal or PTY code in agents-plugin-tool/internal; neighbors sysopen_route_test.go and stream_ws_test.go show the test pattern |
| complexity.reuse_points | confirmed | localViewer access.go#L136-L138, sameOrigin inbox_routes.go#L122-L132, x/net/websocket stream_ws.go, go:embed list pages.go#L34 |
| complexity.side_effect_risk | high | spawns and attaches interactive shells, which is arbitrary command execution on the host |
| risk.correctness | high | reattach across daemon restart, output backfill, and resize on Linux, macOS, and native Windows ConPTY |
| risk.fit | moderate | terminal cards must join the board column topic flow and pin rules of Decision 4, and a JS emulator must be vendored into a no-npm embed tree |
| risk.test | high | verification needs a daemon restart with an open shell on three platforms and browser-driven emulator checks |
| risk.security_or_contract | high | the gate must match the approval trust boundary; a LAN token holder must never reach a shell |

## Phases

### Phase 1: terminal on holder PTYs

Scope and verification are settled at promotion, after the Constraints'
open items. Verification must include a daemon restart with an open shell
on Linux, macOS, and native Windows.
