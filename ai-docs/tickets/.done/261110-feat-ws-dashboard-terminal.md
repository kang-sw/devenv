---
title: "Dashboard agent host: a browser terminal on holder-owned PTYs behind the trusted-origin boundary"
parent: 261110-epic-ws-dashboard-agent-host
blocked-by: 261110-feat-ws-dashboard-session-holder
related:
  261110-research-ws-dashboard-agent-host-foundations: "Topic 1 (archived ws-dashboard terminal)"
  261106-epic-ws-dashboard-serve-daemon: "its Non-Scope (no browser terminal) is reopened here"
  261110-feat-ws-dashboard-file-preview: "shares the board card framework (its Decision 10)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 17a95b70b45181ea
sage-review-completeness-reviewed: 17a95b70b45181ea
completed: 2026-11-11
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
   position under the pinned ones ((37) b). Where a new card enters is
   Decision 14.
5. **Gate: `localViewer` plus `sameOrigin` (technical, (101) ok).** The
   terminal routes and the websocket upgrade answer only a browser on this
   machine whose Origin is the daemon's own; a LAN viewer gets 404, as on
   the system-open route, and a loopback request with a foreign Origin gets
   403, as there (`sysopen_route.go#L132-L134`). Recognizing a desktop-shell origin waits until a
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
   (`261110-feat-ws-dashboard-session-holder` Decision 7). With several
   viewers, the daemon serves a late joiner by replaying from the lowest
   sequence any viewer still needs and filtering per viewer by sequence.
   Rejected: a second daemon-side buffer.
9. **Card size and expansion (user, (118) a).** A terminal card is folded
   (its header and the last output line), base (14 rows, the default), or a
   height the user drags with a handle under the card, remembered in the
   browser. "Maximize" opens the terminal in a sheet over the conversation
   and board columns; Esc returns it to its card. Rejected: in-card height
   only, without the sheet ((118) b); expanding to the full board column
   height and pushing topics down ((118) c).
10. **Fit resize reaches the PTY debounced (technical, (119) ok).** A card
    size change runs `addon-fit`, and after a 150 ms debounce the daemon
    sends the new columns and rows to the holder. A folded card sends none
    and keeps the last size, since 0 rows breaks shell output. When two
    browsers view one terminal, the last one that typed into or focused it
    sets the size, as tmux's latest-client rule. Rejected: a resize per
    pixel step (a SIGWINCH storm); the minimum size over all viewers (more
    state).
11. **Opened from the board header (user, (120) a and note).** The board
    column gains a one-row header, mockup-sized, holding "+ Terminal". Its
    popover picks the working folder: the session's worktree (default), the
    repository root, or another folder by dialog or typed absolute path. A
    typed path that does not exist or is not a folder is refused with an
    inline error, and the popover stays open.
    After `261110-feat-ws-dashboard-workspaces`, the rail's worktree rows
    open terminals too. Rejected: a button in the session top bar
    ((120) b).
12. **A pinned card shows only its pin icon pressed (user, 2026-11-10
    whiteboard note on the mockup).** No accent edge or outline on the card.
13. **A terminal shows on the session page it was opened from (user,
    (128) a).** The holder descriptor's session description
    (`Descriptor.Session`, holder Decision 5) records the opening session's
    reply ID, a terminal marker, and the working folder; the spawn spec has
    no metadata field, and the holder itself has no reply ID (holder
    Decision 8). Once that session's record
    is pruned (7-day `RegistryRetention`), its live terminals are reached
    from the dashboard drawer's holder list, where they can be attached or
    stopped; attaching opens the terminal as a card on the session page the
    user is viewing. Rejected: every session page rooted at the same worktree
    ((128) b; revisit when workspaces Phase 2 adds rail worktree entry
    points); every session page ((128) c).
14. **One placement rule for every card kind (policy, (137) a, assumed
    under the user's 2026-11-10 delegation of remaining details).** A card
    enters at the head of the unpinned flow, just below the pinned cards,
    when it is opened; after that it follows Decision 4. The framework owns
    this rule for every kind, and each kind supplies its folded content and
    base height (a terminal folds to its header and last output line and
    has a 14-row base, Decision 9). Card nodes are fixed anchors in the
    board morph (`morphBoard`, `dashboard.js#L452`) or live outside its
    swapped children, so a board update never removes, moves, or reattaches
    a card. Rejected: a per-kind placement rule ((129)).

## Prior Art

- The trusted-origin gate already exists for one host-executing route: the
  system-open route (`internal/wsdashboard/sysopen_route.go`) answers only a
  browser on this machine (`localViewer`, `access.go`) whose Origin equals
  the daemon's own (`sameOrigin`, `inbox_routes.go`); a LAN viewer gets 404.

## Constraints

- The terminal is independent of daemon lead sessions; it depends only on
  the holder.
- The working folder choices are Decision 11's until
  `261110-feat-ws-dashboard-workspaces` adds rail entry points.
- Terminal cards are one kind on the shared board card framework
  (`261110-feat-ws-dashboard-file-preview` Decision 10): the framework owns
  fold, base, drag handle, the maximize sheet, pin, close, persisted view
  state, and the Decision 4 placement; the terminal kind supplies its
  header, body, and holder-backed lifecycle (technical, (129) ok; rejected:
  kind-owned placement, which a later right-edge dock would have to change
  in every kind). Whichever of the two tickets
  lands first builds the framework as one shared component; the other adds
  its kind to it.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/windows-dogfood.md`.

## Prior Decisions

- 261110-epic-ws-dashboard-agent-host (2026-11-10, Cross-Child Decisions): "One trusted-origin boundary. Tool approval and the terminal are both execution authority and share one boundary: loopback or a desktop-shell origin by default, never the LAN token alone." — bearing: supports
- 261110-research-ws-dashboard-agent-host-foundations (2026-11-10, Confirmed Decisions): "The terminal runs on holder PTYs behind the trusted-origin boundary; the archived Rust PTY code is not carried over ((05), (23))" — bearing: supports
- 261110-feat-ws-dashboard-session-holder (2026-11-10, Decisions): "PTY: `pty` on POSIX, ConPTY through `ctypes` on Windows. The standard library `pty` module is POSIX-only; on Windows the holder calls `CreatePseudoConsole` ... through `ctypes`." — bearing: constrains
- 0336011f (2026-11-07, commit): "Non-local viewers get 404 from the open route (not 403) so the route looks absent to a LAN token holder and TestLANModeGuardsEveryRoute stays uniform" — bearing: constrains
- 261110-bug-ws-dashboard-sse-connection-budget (2026-11-10, Decisions): "No new module dependency. The WebSocket server uses `golang.org/x/net/websocket` ... adding another module needs its reason in the commit's `## AI Context`." — bearing: constrains
- 2abc8bc8 (2026-06-20, commit): "The fix keeps explicit capable parent TERM values but normalizes missing, empty, or dumb values to xterm-256color at PTY spawn time" — bearing: constrains
- 260523-feat-ws-dashboard-terminal-tab-restore (2026-05-23, commit facfac97): "The implementation intentionally recreates new daemon terminal sessions instead of pretending PTY processes survive daemon death." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, Decisions): "Preview cards are a kind on a shared board card framework. The framework owns what every card kind shares: fold, base height, drag handle, the maximize sheet with Esc, pin, close, ... and placement in the board flow" — bearing: supports

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
| risk.fit | moderate | terminal cards must join the board column topic flow and pin rules of Decision 4 on the card framework shared with 261110-feat-ws-dashboard-file-preview Decision 10 (ownership split (129) ok), and a JS emulator must be vendored into a no-npm embed tree |
| risk.test | high | verification needs a daemon restart with an open shell on three platforms and browser-driven emulator checks |
| risk.security_or_contract | high | the gate must match the approval trust boundary; a LAN token holder must never reach a shell |

## Phases

### Phase 1: terminal on holder PTYs

Scope:

- Daemon: a terminal registry over holder PTYs. Create spawns a holder with
  the daemon-picked shell and TERM (Decision 7), the chosen working folder
  (Decision 11), and the opening session's reply ID (Decision 13). Routes
  list a session's terminals, create one, and close one (stop through the
  holder protocol after a browser confirm); one websocket per terminal
  carries output with sequence numbers, input, resize, and exit status.
  Every terminal route and the websocket upgrade sit behind `localViewer`
  plus `sameOrigin` (Decision 5).
- Reattach: on daemon start, terminal holders found by the holder scan are
  reattached and their output is backfilled from the holder ring
  (Decision 8); a reported gap shows in the card.
- Frontend: `@xterm/xterm` and `addon-fit` vendored (Decision 6); the board
  header row with "+ Terminal" and its folder popover (Decision 11);
  terminal cards in the board column with fold, base, drag height, the
  maximize sheet, and pin (Decisions 4, 9, 12); the debounced fit resize
  (Decision 10); an exited shell shows its exit status and a close button.
- The drawer lists live terminals whose session record is gone
  (Decision 13), with attach and stop.

Verification:

- Go tests: a loopback same-origin request reaches every terminal route and
  the websocket; a LAN token holder gets 404 and a cross-origin request
  gets 403; the routes are added to `TestLANModeGuardsEveryRoute`; the frame
  protocol round-trips input, output, resize, and exit status against a
  test holder; reattach backfills from the last sequence and reports a gap;
  with two viewers at different sequences, a late joiner receives the
  replay from its own sequence and the other viewer sees no duplicate.
- Headless Chromium: open a terminal, type, and see output; fold, base,
  drag, maximize, and Esc; pin; a card size change reaches the PTY (`stty
  size`) once per debounce; a folded card sends no resize; with two viewers
  the last-focused one sets the size; the "+ Terminal" popover opens in the
  worktree, the repository root, and a typed folder, and shows an inline
  error for a missing path; close asks a browser confirm and then stops
  the shell; an exited shell shows its exit status and a close button; a
  backfill gap shows in the card; the drawer lists a terminal whose session
  record is gone and attaches and stops it; a terminal survives a board
  update without reattaching.
- Board card framework, when this ticket builds it (Constraints): the
  framework checks of `261110-feat-ws-dashboard-file-preview` Phase 1
  apply here, the Decision 14 placement included, and a reload restores an
  unpinned card at its earlier place from its persisted open time (the
  holder's start time serves for a terminal).
- When the framework already exists (built by
  `261110-feat-ws-dashboard-file-preview`): a terminal card folds to its
  header plus last line, opens at 14 rows, and enters by the Decision 14
  placement.
- A daemon restart with an open shell on Linux, macOS, and native Windows
  (`ai-docs/manuals/windows-dogfood.md`): the shell survives and its earlier
  output reappears after reattach.

### Result (818879863) - 2026-11-11

Landed, base `2950c3e9c`:

- `internal/wsholder`: `Manager.AddSink` (a second receiver of holder events
  beside the one `New` was given), `Options.RingBytes` (a new holder's ring
  size), and loss counted once: a gap a consumer's own replay reports again
  no longer inflates `Info.Lost`.
- `internal/wsdashboard/terminals.go`: the terminal registry over holder
  PTYs. A terminal is its holder; the holder's session description carries
  the terminal marker, the opening session's reply ID, and the folder. No
  daemon buffer: one holder stream is fanned out by sequence, each viewer
  takes exactly the record it needs next, and a late joiner or a reconnecting
  page has the holder replay from the lowest sequence any viewer needs.
- `terminal_routes.go`: `GET`/`POST /s/<reply-id>/terminals`,
  `POST .../terminals/<id>/close`, and the WebSocket
  `.../terminals/<id>/ws?from=<sequence>` (JSON text frames, base64 output;
  the protocol is the file's header comment). Every route and the upgrade
  sit behind `localViewer` (else 404) and the daemon's own Origin (else 403).
- `terminal_shell.go`: the Decision 7 shell rule as a pure function of the
  OS, the environment, and a lookup.
- `assets/xterm/`: `@xterm/xterm` 5.5.0 and `@xterm/addon-fit` 0.10.0,
  unchanged, with both licenses; served as one hash-named script beside the
  page, the stylesheet inlined, and only to a browser on the daemon's machine.
- `assets/terminal.js`: the `terminal` card kind on the existing board card
  framework, the board header row with "+ Terminal" and its folder popover,
  the reconcile with the list route, and the drawer's orphan rows (attach
  and stop). `assets/cards.js` gained three generic kind hooks (a `closing`
  veto, `base` as a function of the card, an open time on `open`).
  `assets/files.js` gained "Open terminal here" on folder rows.
- i18n: 29 keys in each of the 11 tables.

Decisions taken (the ticket left them open, or the implementation narrowed
them):

- "Another folder" is a typed absolute path only. A browser has no folder
  dialog that yields an absolute path (the same reason the file explorer's
  change root is typed), so the "by dialog" half of Decision 11 is not built.
- The list is a `GET`, and a browser sends no Origin with a same-origin
  `GET`; it refuses a foreign Origin or a `Sec-Fetch-Site` other than
  `same-origin`/`none` instead of requiring the header. `POST` and the
  WebSocket handshake require the daemon's Origin.
- The list answers this session's terminals plus the orphans (terminals
  whose session record is gone); an orphan attaches on the page being viewed.
- The PTY takes the size of the viewer that last typed or focused; until one
  has, the last size any viewer sent. A size equal to the PTY's is not sent.
- A viewer whose backlog passes 16 MiB is dropped and connects again from
  the sequence it reached.
- The routes wait up to 5 s for the daemon's first holder scan and answer
  503 after; a terminal whose holder the daemon is not attached to answers
  503 on the upgrade, and the page shows it as reconnecting.
- Windows shells are started by their resolved path.
- Esc in the maximize sheet returns to the card and does not reach the
  shell (Ctrl+[ still sends ESC); in the card, Esc reaches the shell.
- Closing a running shell asks a browser confirm; an exited shell closes
  without the question.
- A gap shows as a marker in the output and as a dismissible strip above it,
  since the replay that follows scrolls the marker out of the scrollback.
- The folded line is the last non-empty screen line, prompt included.
- "Open terminal here" is offered on a folder row of the file explorer.

Verification:

- Remote builder, at `818879863`: `go test ./internal/wsdashboard/
  ./internal/wsholder/ ./internal/wsi18n/ ./cmd/... -count=1`: `ok` for all
  four; the terminal tests also `ok` under `-race -count=3`. `go build ./...`
  and `go vet ./internal/wsdashboard/` clean. `go test ./... -count=1` at
  `7eda9ad11` (before the review fixes): `ok` for all 31 packages.
- The Go tests cover every item of the Go list: the gate on every route and
  the real handshake (`TestTerminalRoutesAnswerOnlyTheDaemonsOwnLocalPage`),
  the routes in `TestLANModeGuardsEveryRoute`, the round trip of input,
  output, resize, and exit status against a real holder, the reattach under a
  second daemon with a reported gap, the late joiner and the reconnecting
  viewer without a duplicate, the size rule with two viewers, the lost
  holder, the first-scan wait, the shell rule, and the folder refusals. The
  holder tests are skipped where no python 3.10 or newer is on PATH.
- Headless Chromium against a fixture daemon with real holders: 57 checks,
  all passing, covering every item of the headless list (popover folders and
  inline errors, type and output, fold with no resize, 14-row base, drag with
  one size frame, maximize and Esc, pin, close confirm, exit status, two
  viewers, the gap, reload restore, a board update with the same socket, the
  drawer orphan attach and stop, a daemon restart with the same shell PID and
  its earlier output, "Open terminal here", dark theme, narrow width). The
  Files tab entry was exercised on a mounted explorer, since the fixture
  sessions have no live adapter and so no Files tab.
- Daemon restart on Linux: beside the in-process restart above, the fixture
  daemon's process was killed with SIGKILL; the holder stayed alive and a new
  process attached to it and listed the terminal as running. Not run on
  macOS or native Windows.
- Lite review, one pass: two Important findings (a folded card sent a size
  on connect; a viewer was not told of a lost holder), both fixed in
  `818879863` with two of the Minor ones (the first-scan race, the paste
  limit).
