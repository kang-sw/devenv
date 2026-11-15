---
title: "Session-less local browsing anchor for the workspace picker and the worktree page"
parent: 261114-epic-ws-dashboard-file-explorer
related:
  261114-feat-ws-dashboard-reusable-file-explorer-card: "its Phase 4 (workspace-picker consumer) is gated on this ticket's decision"
  261114-feat-ws-dashboard-worktree-view: "consumer: a worktree page with no agent session needs files and terminals rooted at a worktree"
  261115-feat-ws-dashboard-explorer-root-switcher: "neighbour: shares the worktree enumeration source"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: e961a4dbdbc60d5d
sage-review-completeness-reviewed: e961a4dbdbc60d5d
---

# Session-less local browsing anchor for the workspace picker and the worktree page

## Background

Two approved consumers need to browse local folders without an agent session:

- The directories-only workspace picker (epic; 755=a home and local-volume
  roots labeled with the daemon host, 756=a local-viewer only, 757=b hidden
  entries translucent). Its natural host is the rail add-workspace popover,
  which has no session.
- The worktree page (`261114-feat-ws-dashboard-worktree-view`; 784=b new page,
  785=ok explorer reuse and terminals with `cwd` at the worktree, no agent
  session).

Codes such as 894=a or 756=a are owner decision IDs from the coordination
board; each is restated in prose where it is cited. MAIN is the coordinating
lead session that merges to develop and gives implementation start signals.

Today (evidence gathered 2026-11-15):

- Every explorer route is session-scoped: `/s/<reply-id>/fs/{list,preview,raw,find,search}`
  and `POST /fs/act`. `browseRoot` (`fs_routes.go`) needs a live indexed
  session with a project; `root=<abs>` is honored only for a local viewer
  (loopback peer and loopback Host, `access.go` `localViewer`) that is not a
  cross-site fetch, and `browseRootParam` accepts any absolute plain-volume
  directory.
- Terminals are session-scoped too: `/s/<reply-id>/terminals` needs
  `offersTerminals` (terminals enabled and a local viewer), same-origin, and a
  live session; `terminalFolder` already accepts `folder:"path"` with any
  existing absolute directory, and terminals are keyed by holder id and carry
  the opening session's reply-id in the holder descriptor
  (`terminals.go#L49-L66`), while `listTerminals` lists a terminal whose
  session is not in the index as an orphan (`terminal_routes.go#L310-L324`).
- Session-less routes exist as precedent: `/rail/workspaces/{preview,add,forget}`
  and `/rail/worktrees/{options,add,delete}` take folder paths, gated by POST
  and `sameOrigin` (not `localViewer`, except worktree delete), normalizing
  with `normalizeWorkspacePath` (`workspace_routes.go`, `workspacepath.go`).
  The index page `/` serves the rail without a session. No session-less route
  lists a directory.
- The daemon knows its own home (`daemonPathHost`) but the browser does not
  receive the home path, a local-volume list, or a host label.

## Decisions

- **Daemon-owned host anchor (894=a).** One reserved, non-session reply-id is
  accepted by the existing `/s/<id>/fs/*` and `/s/<id>/terminals` handlers,
  only for a local viewer and only with an explicit root (explorer) or folder
  (terminal). All existing guards and the existing explorer and terminal
  clients are reused; anchor terminals are listed per worktree path.
  Compatibility: T0 (additive; no existing route's semantics change for real
  sessions). Rejected: a separate session-less route family (duplicates the
  handlers and tests for the same rules), and borrowing a live session of the
  same repository (unusable when no session of that repository is live, and
  terminals die or orphan with the borrowed session).
- **Local viewers only.** Session-less browsing and terminals refuse remote
  (LAN) viewers and cross-site fetches: 756=a makes the picker local-viewer
  only, terminals already require a local viewer, and `root=` browsing is
  local-viewer only today.
- **Server-side root discovery.** The picker's starting roots (daemon host
  label, home, local volumes) come from one additive local-viewer-only answer
  next to the anchor. Rejected: hard-coding paths in the browser, which cannot
  know the daemon user's home or mounted volumes.
- **Anchor scope.** Only the `/fs/*` and `/terminals` handlers accept the
  anchor id; every other `/s/<id>/` route (session page, conversation,
  streams, mailbox) answers 404 for it, because the anchor is not a session.
  `/fs/act` (system open/reveal, no filesystem mutation) is included through
  its own anchor branch with an explicit `root`, under its existing
  local-viewer and same-origin gate, because 894=a names `/s/<id>/fs/*`
  whole. The anchor check runs at dispatch before the session lookup, so the
  anchor's page path, with or without a trailing slash, answers a bare 404
  rather than the not-found page or a redirect. Rejected: a per-handler
  check, which leaves the session page and the trailing-slash redirect
  answering for a non-session.
- **Anchor terminals are kept apart from session terminals.** A real
  session's terminal list does not show anchor-held terminals as orphans;
  the holders card shows them as anchor-owned with their folder. The anchor
  `GET /terminals` takes an additive folder filter matching the holder's
  cleaned working directory exactly, and under the anchor only
  `folder:"path"` is accepted (`"worktree"` and `"root"` need a session).
  Terminal close and WebSocket attach keep the existing trust model (local
  viewer, same origin, holder id) with no new scoping. Rejected: a prefix
  match on the folder, which would show a nested worktree's terminals on its
  parent's page; adding ownership checks to close and attach, which changes
  existing route semantics.
- **Root discovery answer.** For a local viewer only: the host label
  (`os.Hostname`), the daemon user's home, and the local volumes - macOS
  `/Volumes/*`, Linux common mount roots (`/mnt/*`, `/media/<user>/*`,
  `/run/media/<user>/*`), Windows drive letters, WSL `/mnt/<letter>` -
  skipping unreadable ones. The route path is the implementer's choice
  within the anchor or the rail family. Rejected: parsing mount tables,
  which lists pseudo filesystems the picker must not offer.
- **No preview capability under the anchor.** HTML and SVG files answer as
  source text: `browseRoot` treats a `root=` read as non-session and mints
  capabilities only for a session's own root (Prior Decision 7f890500).
- **Anchor terminals persist** until the user closes them or their shell
  exits (holder-owned PTYs survive a daemon restart, `terminals.go#L170-L205`,
  `terminal_routes_test.go#L693-L699`); they are never reaped as "session
  gone" (the daemon has no terminal reaper: `terminal_routes.go#L310-L324`
  only lists a session-less terminal as an orphan). Rejected: reaping when no
  page shows them, which would kill a worktree terminal on a page reload.
- **Fixed reserved anchor id** containing a digit (for example `host0`),
  which `ValidReplyID` (`^[a-z0-9]{1,64}$`) accepts and the reply-id
  generator (`wsmailbox.ReplyID`, letters only, 12-36 characters) can never
  mint, proven by a test that every generator word is lowercase letters only
  and the anchor fails the generator's shape. Rejected: a random per-daemon
  id - pages need a stable id across daemon restarts with no discovery
  round-trip.
- **Compatibility: T0.** Every change is additive; an older daemon sharing
  the holders would only list anchor terminals as orphans.
- Settled 2026-11-15: 894=a by the owner; the other items acknowledged by
  MAIN, and the fact-population gaps (fs/act branch, terminal listing and
  folder filter, dispatch 404, discovery shape, id shape, T0) auto-confirmed
  by the lead.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Read-only; no filesystem mutation, no new symlink permissions; normal Files
  root guards unchanged. Any new route is additive (T0) and changes no
  existing route's semantics.
- Not authority to create an agent session, set a root flag, or control
  another session (worktree-view Constraints).
- Implementation waits for MAIN's start signal after
  `261114-refactor-ws-dashboard-whiteboard-asset-split`.

## Prior Decisions

- 7f890500 (2026-11-10, commit): "Change root is a `root` query parameter gated by localViewer and crossSiteFetch under the files route's abs-mode path rules; no capability is minted there, so HTML and SVG answer as source text (Decision 4)." — bearing: supports
- 261110-feat-ws-dashboard-terminal (2026-11-11, Result 818879863): "The list answers this session's terminals plus the orphans (terminals whose session record is gone); an orphan attaches on the page being viewed." — bearing: constrains
- 261110-feat-ws-dashboard-terminal (2026-11-10, Decisions): "PTYs live in the session holder, so shells survive a daemon restart (user, (06) a; technical, (09)). Each terminal is a holder-owned PTY; after a restart the daemon reattaches instead of creating new shells." — bearing: contradiction-candidate
- 295a841d (2026-11-11, commit): "The gate is the system-open route's: localViewer else 404 (a LAN token holder sees no route), then the daemon's own Origin else 403, the WebSocket handshake included." — bearing: constrains
- 261112-feat-ws-dashboard-outside-root-file-card (2026-11-12, Decisions): "One capability gates the reads for an outside link this ticket opens (review, auto-confirmed; the explorer's `root=` reads and the 261109 inline outside-image route stay as they are)" — bearing: supports
- 261114-epic-ws-dashboard-file-explorer (2026-11-14, Cross-Child Decisions): "Normal Files browsing remains read-only and keeps the existing guarded-root behavior. The separately approved workspace picker is local-viewer-only, directories-only, may browse home and local-volume roots" — bearing: constrains
- 261115-feat-ws-dashboard-explorer-root-switcher (2026-11-15, Decisions): "Local viewers only. The switcher is offered exactly where `root=` already works (a local viewer, not a cross-site fetch); remote (LAN) viewers keep seeing the session root alone." — bearing: supports
- 86042116 (2026-11-15, commit): "Anchor choice is open owner policy (894); announced/technical calls acked by MAIN: local-viewer-only root switch and session-less browsing, keep free-text root popover, additive session-scoped worktree JSON route (T0)" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/fs_routes.go, fs_act_route.go, terminal_routes.go, terminals.go, pages.go, registry.go, workspacepath.go plus a new root-discovery route and tests |
| scope.surface | cross-module | new reserved reply-id value accepted by ValidReplyID, new local-viewer-only root-discovery HTTP route, changed terminal list answer; ValidReplyID is exported and also used by cmd/ws-mcp/holder_exit.go and internal/mcp/playbook_tools.go |
| scope.new_public_symbol | yes | a reserved anchor id constant, likely exported because the page and ws-mcp packages both read ValidReplyID; exact name unsettled |
| scope.new_type_contract | yes | root-discovery answer type (host label, home, volumes) and a folder filter plus anchor shape on terminalList |
| scope.test_surface | existing | fs_routes_test.go (TestFSChangeRootIsLoopbackOnly), terminal_routes_test.go (TestTerminalFolder, TestTerminalRoutesAnswerOnlyTheDaemonsOwnLocalPage), access_test.go (TestLANModeGuardsEveryRoute), wsmailbox/replyid_test.go; new anchor and discovery tests also needed |
| complexity.reuse_points | confirmed | browseRoot and browseRootParam (fs_routes.go#L151-L205), terminalSession and terminalFolder (terminal_routes.go#L211-L355), daemonPathHost and pathHost.Home (workspacepath.go#L46-L55) read; no volume enumeration or host label exists to reuse (searches for os.Hostname, GetLogicalDrives, /proc/mounts, /Volumes under agents-plugin-tool returned nothing) |
| complexity.side_effect_risk | high | anchor terminals would be classed as orphans on every real session list and the holders card unless listTerminals is changed; fs/act has its own session lookup; terminal close and ws routes are not session-scoped |
| risk.correctness | high | the terminal list orphan branch, the folder filter, the fs/act branch and per-OS volume discovery are each unspecified in the draft |
| risk.fit | moderate | handler reuse fits the existing routes, but the anchor threads through several session lookups that each assume an indexed session with a project |
| risk.test | moderate | route-level fixtures exist (startPageFixture, fsViewers, the every-route LAN sweep), but real holder terminals need python 3.10+ and volume discovery is OS-specific |
| risk.security_or_contract | high | the anchor opens a session-less filesystem listing and a shell spawn at an arbitrary folder behind localViewer and same-origin alone, and adds a reserved id to the registry-directory-name guard |

## Phases

### Phase 1: Session-less anchor and root discovery

- Reserve the anchor id. `browseRoot` accepts it only for a local viewer that
  is not a cross-site fetch and only with an explicit `root=`; without `root=`,
  or for any other viewer, it answers 404 exactly like an unknown session. The
  read is non-session (no HTML/SVG capability). `/fs/list`, `/fs/preview`,
  `/fs/raw`, `/fs/find` and `/fs/search` work unchanged under it (all five
  call `browseRoot`, `fs_routes.go#L448`, `#L577`, `#L655`, `fs_search.go#L175`,
  `#L191`). `/fs/act` does not call `browseRoot` and looks up the session
  itself (`fs_act_route.go#L171-L206`); give it an anchor branch that
  requires a non-empty `root` and keeps its local-viewer and same-origin
  gate.
- `terminalSession` accepts the anchor under the same local-viewer,
  same-origin rules, and only with `folder:"path"` and an existing absolute
  directory (`"worktree"` and `"root"` refused under the anchor). The anchor
  list takes a folder filter matching the holder's cleaned working directory
  exactly, so a worktree page sees only its worktree's terminals; real
  session lists stop counting anchor-held terminals as orphans, and the
  holders card labels them anchor-owned with their folder, reading them from
  the anchor `GET /terminals` without a folder filter (which lists every
  anchor terminal). Under the anchor an empty `folder` is refused like
  `"worktree"`, and the list's `folders` answer carries only the filtered
  path (no worktree or root folder). Close and attach are unchanged. Anchor terminals persist until
  closed or their shell exits: terminals are holder-owned PTYs that survive a
  daemon restart (`terminals.go#L170-L205`,
  `TestTerminalSurvivesADaemonRestartAndReportsAGap`
  `terminal_routes_test.go#L693-L699`).
- Every other `/s/<id>/` route answers a bare 404 for the anchor id, checked
  at dispatch before the session lookup (page path with and without the
  trailing slash included).
- Add the picker's root discovery as described in `## Decisions`, refused to
  remote viewers and cross-site fetches with its own `localViewer` and
  cross-site check (rail routes check only `sameOrigin`), and add it to
  `TestLANModeGuardsEveryRoute`. Keep per-OS volume enumeration behind an
  injectable seam so each OS branch is testable on one host.

Verification: Go tests that the anchor is refused to remote viewers, to
cross-site fetches and without `root=`/`folder:"path"`; that listings under it
stay read-only and symlink-guarded and HTML answers as source; that a terminal
opened through it starts in the requested worktree path and appears only in
that folder's list (exact match, a nested folder's terminal excluded) and not
as an orphan in a real session's list; that `/fs/act` under the anchor needs
`root`; that other `/s/<anchor>/` routes, the page path with and without its
trailing slash included, answer a bare 404; that the generator cannot mint
the anchor id; that existing session routes and their tests are
unchanged (`TestFS|TestTerminal|TestFiles` neighbours); that root discovery
returns home and volumes for a local viewer and 404 otherwise (including the
LAN every-route sweep), with each OS volume branch exercised through the seam
on fixture directories and an unreadable volume skipped; a browser check that
the holders card lists an anchor terminal as anchor-owned with its folder. The picker
consumer (card ticket Phase 4) and the worktree page build on this phase.
