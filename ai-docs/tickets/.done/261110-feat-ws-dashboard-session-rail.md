---
title: "Dashboard agent host: a session rail card on the session page, always shown when wide and sliding out from the left otherwise, with per-session draft restore"
parent: 261110-epic-ws-dashboard-agent-host
related:
  261110-feat-ws-dashboard-workspaces: "adds user-added workspaces and the rail's + buttons on top of this ticket's workspace and worktree grouping"
  261110-feat-ws-dashboard-agent-panel-telemetry: "sibling multi-session UX child; independent"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 26a645c5c36fb444
sage-review-completeness-reviewed: 26a645c5c36fb444
completed: 2026-11-10
---

# Dashboard session rail

## Background

Track A of the epic, for the sessions that exist today: the user runs many
sessions at once and wants switching between them to be smooth, independent
of the terminal and the session holder (2026-11-10 whiteboard, topic
"멀티 세션 관리 UX", (28) b assumed from the user's request).

Today the arrow at the session page's top left (`assets/dashboard.tmpl:9`,
`<a class="back" href="/">`) is a plain link to the sessions index page,
not a dropdown or a drawer. The index (`handleIndex`, `pages.go:804`;
`buildIndexView`, `pageview.go:766`; templates `index` and `r-sessions`,
`assets/dashboard.tmpl:175`, `:194`) already groups sessions by registration
project path (`ProjectGroup`, `pageview.go:732`) with live sessions first,
an ended subsection, and rows (`IndexRow`, `pageview.go:742`) showing the
reply ID, harness, phase pill, current tool, running-agent count, policy
item count, and last seen. It is server-rendered and refreshed over the
`/events` WebSocket.

The session grid is one column with tabs by default; the "split" layout
gives two columns (`5fr | 7fr`, conversation and board) from 760px and the
"auto" layout from 1100px, while "tabs" stays one column
(`assets/dashboard.css#L468-L496`, media queries at `:482` and `:489`); the layout menu (auto, split, tabs) is saved in
localStorage `ws.dashboard.layout` (`assets/dashboard.js:152`). A
three-column mode exists only in the tracker drawer, from 1600px
(`layoutOf`, `assets/tracker.js:2037`).

What the daemon holds for the rail's row today (Explore, 2026-11-10; paths
under `agents-plugin-tool/internal/`):

- Harness: `IndexRow.Harness` (`wsdashboard/pageview.go:780`).
- Lead working: `Snapshot.Lead.Phase` with a live adapter; the daemon
  already computes the index's `view.Busy` from a live adapter and a
  non-idle phase, permission waits included (`pageview.go#L810-L812`).
  Phases: idle, thinking, replying, tool, permission, compacting.
- Running subagents: `IndexRow.RunningAgents` counts every running agent,
  nested ones included (`pageview.go#L800-L804`); `wslive.Agent` carries
  `ParentID`.
- Last tool call time: no field. `IndexRow.LastSeen` is the registration
  heartbeat (`wsdashboard/registry.go:101-109`). The snapshot keeps the
  newest five lead batches with start and end times and the running call's
  start (`wslive/snapshot.go`).
- Last message: the conversation ring (`<session dir>/ring/`, `RingUser`
  and `RingReply` entries, `wslive/ring.go`) outlives the session; the
  index does not read it.
- Grouping: `Registration.Project` is the session root path; nothing
  records which repository a linked worktree belongs to. The daemon reads
  git for a root only while a page of a session under it holds its stream
  open (`wsdashboard/gitbadge.go` header). Pooled worktrees are told apart
  by the `worktree_pool` roots (`mcp/dashboard_worktrees.go`, `underAnyPool`).

Mockup the user confirmed (v13, adjusted to v14 after promotion; 2026-11-10 whiteboard, topic "세션 레일
목업"): `ai-docs/ref/session-rail-mockup/index.html` (three widths, the
rail sliding out, sample data; the + buttons and their popovers in it
belong to `261110-feat-ws-dashboard-workspaces`). An earlier artifact
mockup (https://claude.ai/artifact/TboGt3LPM37sSVSknTMZbE) is superseded.

## Decisions

1. **The session list becomes a card, the session rail, on the session
   page (user's request; technical, (29) ok).** A 340px card at the
   page's left. From a 1600px viewport the page has three columns, rail |
   conversation | board, and the rail is always shown. Below 1600px the
   top-left arrow toggles the rail as an overlay that slides in from the
   left over the page, with a scrim; Esc, a scrim click, or picking a
   session closes it. The index page `/` stays, linked from the rail's
   foot (it also lists the ended sessions in full). 1600px is the tracker
   drawer's three-column threshold, so the page has one notion of "wide".
   A board-first page (no live layer) places the rail the same way: a
   left column from 1600px, the overlay below (technical, (86)).
   Rejected: inserting the rail as a grid column below 1600px, which
   narrows the conversation column.
2. **Picking a session is a full page navigation to `/s/<id>` (technical,
   (30) ok).** Rejected: an in-page (SPA) swap, which would have to swap the
   region stream, the composer draft, scroll, and board state inside one
   page. Every page renders the rail as the same region, so a navigation
   costs only a short repaint.
3. **Per-session draft restore in localStorage (user, (30) note; technical,
   (39) ok).** Keyed by session ID: the composer text with its mentions,
   whiteboard answer and note inputs in progress, and open, collapsed, tab,
   and selection states (the worker lists the elements covered in the
   Result). Sending a message clears that session's draft; beyond 50
   sessions' drafts the oldest is dropped first (technical, (87)). Pasted images are not
   stored; a restored draft that had images says that the images were not
   restored. This reuses the page's existing `save`/`load` localStorage
   helpers (`assets/dashboard.js:16-21`; `load` at `:18`, `save` at `:19-21`); the composer draft is held in
   memory only today. Rejected: storing images in IndexedDB (a new storage
   layer for one case); relying on localStorage for images (its quota,
   around 5MB, fills quickly).
4. **Leaving with unrestorable input asks first (user, (39) note).** While
   the current page holds input that the draft store cannot restore (pasted
   images), navigating away, including picking another session in the rail,
   raises the browser's leave-page confirmation (`beforeunload`), the way an
   editor guards unsaved edits.
5. **The rail's visibility depends only on the viewport width (technical,
   (40) ok).** From 1600px it is always shown whatever the layout menu
   (auto, split, tabs) says; the menu keeps governing the conversation and
   board columns only. Rejected: hiding the rail under the "tabs" layout,
   which would make it vanish when the user only wanted tabs for
   conversation and board.
6. **A session row is two lines (user, mockup v13).** Line 1: star,
   harness badge, robot icon with the running level-1 subagent count,
   reply ID, then at the right end the policy item count chip (when
   nonzero) and the time of the last tool call (relative, "now", "12s",
   "1m", ticked on the client by the page's existing relative-time
   formatting, `assets/dashboard.js`, so an idle rail stays current;
   technical, (88)). Line 2: the last message, one line, aligned to the badge's left
   edge, prefixed with the user marker when the user spoke last. The row
   has no phase pill, no current-tool line, no subagent list, and no fold
   carets. Rejected: richer cards with a reply preview and a context bar
   ((45), (46) withdrawn); a phase and tool line (user, 2026-11-10).
7. **Status lives on the badge and the robot (user, mockup v12-v13;
   technical, (61), (67) ok).**
   - While the lead works, a light orbits the harness badge's border.
     Working means a live adapter and a phase other than idle and
     permission: a permission wait is the lead waiting on the user, and a
     session without a live adapter (or with a stale snapshot) shows as
     waiting. The index page's working total (`view.Busy`, which counts
     permission waits) stays as it is (technical, (80), with (74)).
     Rejected: changing the index's count to match the rail in this
     ticket.
   - While at least one level-1 subagent runs, the robot blinks and its
     count shows; with none, the robot and count take no space, so the
     reply ID sits right after the badge (user, 2026-11-10 whiteboard,
     after v13; mockup v14).
   - The badge has a fixed width so the reply ID starts at the same place
     in rows without subagents (CLAUDE and PI differ in length).
   - Under `prefers-reduced-motion: reduce` the ring is drawn whole and
     still, and the robot shows in the accent color without blinking.
   Rejected: a separate dual status glyph beside the badge (outer arc for
   the lead, inner dot for subagents; mockups v6-v11), replaced at the
   user's request; badge width following its text; keeping the robot's
   space when no subagent runs (v13), which left the reply ID floating.
8. **Starred sessions (user, (51) c; announced, (52) ok; user, v13).** A
   star at the row's left marks a preferred session. Starred sessions are
   pinned to the top of their own worktree group, in their original order;
   unstarring returns a session to its place. An unstarred star shows only
   while the row is hovered or holds focus (always, faintly, on touch
   screens); a starred one always shows. The star set is browser-local, in
   localStorage under `ws.dashboard.`, with no server state. Rejected: a
   separate favourites group at the top of the rail ((51) b); server-side
   star state.
9. **The rail groups by workspace, then worktree (user, (62) a).**
   - A workspace is a repository (its root worktree) or a plain non-git
     folder, per the epic. Its bar shows an icon (a repository icon for a
     repository, a folder icon for a plain folder; user, 2026-11-10
     whiteboard, mockup v14), the name, the path, and the live session
     count, and collapses on click.
   - Under a repository, every worktree that `git worktree list` reports
     is a row with a branch icon, the branch, the folder name ("root" for
     the root worktree), and the change count; a worktree with no session
     shows dimmed. The change count comes from the newest snapshot of a
     session in that worktree (`wslive.Repo` modified, staged, and
     untracked), and a worktree without one shows no count, so the rail
     never reads a working tree (user, (81) a). ws pooled worktrees (from `ws/worktree.acquire`, under
     the `worktree_pool` roots) are not shown ((54) ok), except that a
     pooled worktree holding a live or ended-but-listed session shows as a
     worktree row for as long as it holds one ((71) a), so no live session
     disappears while idle pool slots stay hidden.
   - The daemon learns a repository's pool roots through the session
     source (`mcp/dashboard_worktrees.go`), extended to answer per
     repository; when no session record resolves them, it applies the
     existing pool rule (`ownedPoolRoots`, `mcp/worktree_tools.go`) to the
     default `worktree_pool` value, which covers both
     `$(GitRoot)/../.ws-worktrees/$(GitRootDirName)` and the legacy
     in-tree `$(GitRoot)/.ws-worktrees` (technical, (84)).
   - Sessions sit under their worktree; ended sessions within the index's
     24h window fold behind an "N ended" line per worktree. A plain folder
     lists its sessions directly.
   - The daemon gets the worktree list by running `git worktree list
     --porcelain` for the repositories of the sessions the rail shows, an
     exception to its rule of reading git only for roots with an open
     session stream (`gitbadge.go`); that command reads only the
     repository's worktree records, not the working tree, and never
     fetches. The `gitbadge.go` header names the exception.
   - The daemon rereads a repository's worktree list at most every 3
     minutes and only while some page showing the rail is open, the git
     badge's cadence and viewer rule (technical, (72) ok). One refresh
     button in the rail header rereads the lists at once (user, (72)
     note); it is not in the mockup.
   Rejected: only worktrees that hold a session, with the full listing
   left to the workspaces ticket ((62) b); keeping the project-path
   grouping and leaving the hierarchy to the workspaces ticket ((62) c);
   for a session in a pooled worktree, listing it under the root
   worktree's row or hiding it ((71) b, c); rereading the worktree list
   on every render, or polling with no viewer ((72)); a `git status` per
   listed worktree so every row has a count, or no count at all ((81) b,
   c).
10. **Data the row needs (technical, (63)-(66) ok).**
    - Last tool call time: computed from the snapshot's lead batches (the
      newest batch's start or end, or the running call's start), lead
      calls only. An ended session's stale snapshot file still supplies
      the time (not the state); a session without a snapshot file shows
      no time. Rejected: a new field written by the adapters.
    - Last message: the newest `RingUser` or `RingReply` ring entry per
      session, cached by the ring's newest file name so an unchanged ring
      is not reread; mail, tool, compaction, and other entries are
      skipped; a reply in progress shows once finished. Ended sessions keep
      theirs. Rejected: reading every session's ring on every index render.
    - Level-1 count: running agents whose parent is the lead. The worker
      first checks real Claude and Pi snapshots for the `ParentID` value a
      lead-spawned agent carries (most likely empty) and records it in the
      Result. Rejected: the current all-levels `RunningAgents`.
    - Repository identity: ws-mcp adds the session root's git common
      directory to its dashboard registration when it registers, so the
      daemon groups linked worktrees under their repository without
      running git per session. A registration without the field (an older
      ws-mcp) falls back to the daemon's existing `.git` read
      (`worktreeMain`, `wsdashboard/peerfolder.go`), which runs no git; a
      root it cannot resolve (a bare-repository worktree, a submodule)
      shows as a plain folder. A session root inside a worktree belongs to
      the listed worktree with the longest matching path prefix
      (technical, (83)). Rejected: the daemon running `git rev-parse` for
      every session.
11. **No + buttons in this ticket (announced, (53) ok).** Adding
    workspaces, worktrees, and sessions belongs to
    `261110-feat-ws-dashboard-workspaces`; the rail ships without them.

## Constraints

- Independent of the session holder, the hosted-session flag, daemon lead
  sessions, and the terminal ((36) ok); it can land now.
- `261110-feat-ws-dashboard-workspaces` adds user-added workspaces (kept
  even with no sessions) and the + buttons onto this ticket's grouping.
- The rail shows nothing beyond what the index and the session pages
  already serve to the same viewers (technical, (73) ok): the last message
  comes from the conversation the session page shows them, and worktree
  branches come from repositories whose sessions they can already open.
- The index page `/` keeps its layout and grouping; the rail is its own
  view built from the same session data (technical, (74) ok). Rejected:
  regrouping the index too.
- The rail is one shared render for every session page, keyed like the
  index (language and theme), without the current reply ID; the page marks
  its own session on the client. A session's change wakes the shared rail
  render and the index, never another session's page regions, so the
  targeted wakes and shared renders of dc8d5566 hold (technical, (82)).
- Browser storage can be absent or throw (private windows, blocked site
  data); the page must work without it, as the existing helpers already
  guard.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`. Heavy Go builds
  and tests follow the machine's remote-build note when one applies.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261107-epic-ws-dashboard-messenger-round2 (2026-11-07, Cross-Child Decisions): "Page preferences (layout, input mode, split ratio) are browser-local (localStorage under `ws.dashboard.`); no server-side preference state." — bearing: supports
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Layout switch. A settings control in the top-right corner lets the user force either layout. Both layouts arrange the same region elements with CSS only (one grid-template-areas rule per layout)" — bearing: constrains
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Layout. The default layout is side by side ... The page itself does not scroll; each column scrolls on its own, and the conversation stays pinned to the bottom" — bearing: constrains
- 3cd05ff2 (2026-11-06, commit): "Followed the brief's two-column split (board plus panels in one column) rather than the mockup's three columns; panels sit below the board inside #bcol." — bearing: constrains
- bba7eb8d (2026-11-07, commit): "Item detail is a side sheet that overlays the drawer's right column; it closes with Esc. Rejected: pushing the drawer aside, which complicates the geometry for no layout insight." — bearing: supports
- 261110-feat-ws-dashboard-agent-panel-telemetry (2026-11-10, Decisions): "A container query on the panel's own width hides the stats from the row ... The threshold is the panel's width, not the viewport's, so the three-column layout of" the session page narrows panels — bearing: constrains
- 12831114 (2026-11-10, commit): "Detection reads `<Path>/.git` only, never runs git: a `gitdir:` file whose target's parent is `worktrees`, common dir taken from the gitdir's `commondir`" (peerfolder.go, memoized per path) — bearing: constrains
- dc8d5566 (2026-11-09, commit): "Measured (6 live sessions, 3 writes/s, 3+3 session streams + 1 index stream, 60 s): CPU 20.0% -> 6.9% of a core ... session renders 1,200 -> 336 per window" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wsdashboard/assets/dashboard.tmpl, dashboard.css, dashboard.js, pageview.go, pages.go, registry.go, gitbadge.go, wslive/ring.go, wslive/snapshot.go, mcp/dashboard_sessions.go registration, mcp/dashboard_worktrees.go |
| scope.surface | cross-module | wsdashboard rail view and region, wslive ring and snapshot reads, mcp registration writer; Registration JSON shared between ws-mcp and the daemon |
| scope.new_public_symbol | yes | a git common directory field on wsdashboard.Registration plus a rail view type and refresh route |
| scope.new_type_contract | yes | Registration JSON gains the common directory written by ws-mcp and read by the daemon; rail row view with last tool time, last message, level-1 count |
| scope.test_surface | existing | wsdashboard/pages_test.go, pageview_test.go, gitbadge_test.go, toast_test.go runNode for JS cases |
| complexity.reuse_points | confirmed | dashboard.js save and load at L18-L21, gitbadge.go gitWatcher cadence gitReadInterval 3m at L43, buildIndexView pageview.go L766, underAnyPool in mcp/dashboard_worktrees.go, peerfolder.go worktree detection |
| complexity.side_effect_risk | moderate | daemon newly runs git worktree list for rail repositories and a new ring read cache per session; localStorage draft writes with eviction and a beforeunload guard |
| risk.correctness | moderate | working-state rule differs from current view.Busy which counts permission; level-1 ParentID value for lead-spawned agents is still unverified; star ordering and draft eviction logic |
| risk.fit | moderate | session grid gains a third column at 1600px alongside existing split and auto layouts and an overlay below it; rail must not regroup the index page |
| risk.test | moderate | three-width browser check, reduced-motion behaviour and mockup match are manual; git worktree reread cadence needs a fake clock |
| risk.security_or_contract | moderate | registration schema change crosses the ws-mcp and daemon boundary; rail must expose nothing beyond what index and session pages already serve |

## Phases

### Phase 1: rail, responsive overlay, draft restore

Implement Decisions 1-11.

Verification:

- Go tests: the session page renders the rail region with sessions grouped
  by workspace and worktree, the same shared render for every session
  page; a linked
  worktree groups under its repository through the registered common
  directory; a worktree with no session appears from the worktree list,
  a pooled worktree does not unless it holds a session; ended sessions
  fold per worktree; the worktree list is reread no more than every 3
  minutes, only with a viewer, and at once on the refresh request; the
  region refreshes over the stream like other regions; one session's
  change re-renders the shared rail once and no other session's page
  regions; a registration without the common directory groups through
  the `.git` fallback; a worktree's change count comes from its session's
  snapshot and is absent for a worktree without one.
- Go tests: the last tool call time comes from the newest lead batch or
  running call, including a stale snapshot file; the last message is the
  newest user or reply ring entry and is not reread while the ring is
  unchanged; the level-1 count excludes nested agents; working excludes
  idle, permission, and adapterless sessions.
- Node-run JS cases in the existing style (`runNode`, `toast_test.go`):
  draft save, restore, clear on send, cap eviction, the images-not-restored
  notice, the leave guard armed only while unrestorable input is held, and
  star pinning within a worktree with the original order restored, and
  the page marking its own session in the shared rail.
- A browser check at three widths (at least 1600px, between 760 and 1600px,
  below 760px): the rail always shown, then sliding in and closing on Esc,
  scrim, and selection; a typed draft survives switching sessions and back;
  the badge ring and robot blink match a working lead and running
  subagents, and stop under reduced motion; rows match the mockup.

### Result (42dbeb14b) - 2026-11-10

Landed in fda6c7a1e (wsdashboard: rail, overlay, drafts, stars, i18n,
ws-mcp.md), 7b2abe3fa (mcp: registration `git_common_dir`,
`ReadRepoWorktrees`), and 42dbeb14b (review fixes, draft-glue tests).

- Decisions 1, 5: region `rail` in every session page; from 1600px a 340px
  column left of the grid (top-left arrow hidden), below it a fixed overlay
  over a scrim, closed by Esc, the scrim, a row pick, or crossing 1600px.
  The rail is one shared render (`railPageSource`, `pageKey{kind: "rail"}`,
  nil deps, the index stamp) sent over each session stream as a side part
  with its own broker subscription, so one session's change renders the
  rail once for all pages and no other session page
  (`TestSessionPagesShareOneRailRender`, `RailRenders` counter). Wake
  folding now also runs without a min-interval wait so the page part always
  renders before the rail on one wake.
- Decision 3: drafts cover the composer text, mentions, and selection, the
  board answers (topic notes included, `readAnswers` shape), the answer
  chips, images that reference written attachments, the open details, and
  the tab; saved 400 ms after an input/change/click/toggle and on
  pagehide/hidden, cleared on a successful send, capped at 50 sessions
  (oldest dropped). Pasted (unwritten) images are not kept: their markers
  are stripped on restore and `js.draft.imgslost` says how many.
- Decision 4: a `beforeunload` guard asks only while a pasted image is held.
- Decisions 6, 7: rows as in mockup v14; working = live adapter and a phase
  other than idle/permission; the robot counts level-1 running agents.
  Observed `ParentID`: both the Claude adapter's snapshots and the Pi
  adapter omit `parentId` for agents the lead spawns, so level 1 is an empty
  parent id (an orphaned parent also counts, as `subagentCounts` does).
- Decision 8: stars and collapsed workspaces in localStorage
  (`ws.dashboard.stars`, `ws.dashboard.railcol`); starred rows pin first
  within their list in server order, unstarring restores the place.
- Decision 9: workspace = repository root from `git_common_dir` (one
  `rev-parse` per session root per ws-mcp process, outside the dashboard
  lock), else the root's `.git` on disk; a bare/submodule common dir is a
  plain folder. Worktree rows from `git worktree list --porcelain` via the
  optional `RepoWorktreeSource` (pooled worktrees under the owned pool
  roots only while they hold a session; prunable ones hidden), read only
  with a streaming viewer: first viewer (skipped within 3 min of the last
  full read, so rail navigation does not reread), every 3 min, a newly
  shown repository alone, `POST /rail/refresh`. Live rows newest
  registration first (stable under activity); ended rows fold per worktree
  by last seen. Change count from the newest snapshot of a session in the
  worktree.
- Decision 10: last tool time from the newest running call or finished
  tool batch (stale snapshot included); last message from the newest
  `RingUser`/`RingReply` (64-entry scan, keeps the last found; image lines
  dropped), cached by index revision and newest ring name.
- Decision 11: no add buttons.

Verification (remote, ws-rgo): `go test ./internal/wsdashboard/
./internal/mcp/ ./internal/wsi18n/ -count=1` ok; `go vet` on wsdashboard and
mcp clean; rail/stream/WebSocket subsets ok over `-count=4`. Node cases
(`TestRailScript`, `TestRailDraftRestore`) also run locally under node and pass. Two
existing tests were adjusted for the new structure (no-harness head check
cuts at the rail; the silent-peer test counts two subscriptions per
session stream). Lite review: no Critical or Important findings; the
cheap minors fixed in 42dbeb14b.

Not done: the three-width browser check (needs a locally built daemon,
which this machine's rule forbids); the `.git` fallback memo keeps a
plain-folder answer for the daemon's lifetime (older ws-mcp registrations
only).

## Sage Review Round 1 (2026-11-10)

### Design Reviewer — concern

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Worktree change count has no data source consistent with Decision 9 | important | autonomous |
| 2 | Rail region defeats dc8d5566's targeted wakes and shared renders | important | autonomous |
| 3 | Registration common-dir field lacks an absent-field fallback | minor | autonomous |
| 4 | Pool roots are only reachable through the session source | minor | autonomous |
| 5 | Epic invariant on pooled worktrees not amended | minor | autonomous |
| 6 | Rail placement on non-live (board-first) session pages unstated | minor | autonomous |

### Completeness Reviewer — block

| # | Title | Severity |
|---|-------|----------|
| 1 | Worktree change count contradicts the git-read exception | important |
| 2 | Registration common-dir fallback for older ws-mcp or a missing field | minor |
| 3 | Rail render cost on every session page is not bounded | minor |
| 4 | Unspecified draft details | minor |
| 5 | Relative time freshness on the client | minor |
| 6 | Referenced mockup is not yet tracked | minor |
| 7 | One phase covers eleven decisions | minor |
