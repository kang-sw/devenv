---
title: "Dashboard daemon re-renders whole pages per live-snapshot nudge, and an open page accumulates DOM and per-second work"
sage-review-design: completed
sage-review-completeness: completed
related:
  261109-bug-ws-dashboard-stream-wake-rerender-cpu: "the per-token wake, shared render cache, refresh stamp, and index board-count memo this ticket builds on"
  261109-bug-ws-tracker-external-write-refresh: "the 2 s tracker store watch and per-store tkNotify order this ticket keeps"
  261110-feat-ws-tracker-support-agent-ui-commands: "its ui markers act only when they arrive live on the support chat stream, which Decision 10 must not drop"
sage-review-design-reviewed: c1c9683997baf0a1
sage-review-completeness-reviewed: c1c9683997baf0a1
---

# Dashboard render cost and frontend load accumulation

## Background

User, 2026-11-10 whiteboard, while the Windows and WSL dashboard daemons sat
at 40-70% of a core and the machine's keyboard stalled every few seconds:
"우리 데몬 쪽에서 뭔가 폴링 기반으로 되어 있는 것들을 가능한 이벤트 기반으로
끌어오는 게 낫겠는데요 ... 성능 개선 관련해서도 좀 현황 파악을" - move what
the daemon polls onto events where possible, and survey performance; and
"프론트엔드에서도 열려있을 때 부하 누적시킬만한 요소 있는지도 좀 조사를" -
check what accumulates load while a page stays open.

Lead investigation (isolated daemon, CPU profiles; harness kept at
`/tmp/claude-1000/-home-swkang-devenv/9cc81656-83d6-4dbe-b005-34e32503ffdb/scratchpad/perf/src/agents-plugin-tool/internal/mcp/zz_e2e_test.go`
and `zz_perf_test.go`, profiles beside them under `perf/`; copy, never
commit):

- **The main cost is the event path, not the polls.** Each `dashboard.intake`
  live-snapshot change nudges the daemon, which wakes every open page stream
  and re-renders the whole page (at most 4/s, `sseMinInterval` 250 ms in
  `internal/wsdashboard/sse.go`). Each render rebuilds the whole whiteboard
  through `agentSessions.ReadPageLang` (`agent_sessions.go:64`, which forwards
  to `DashboardSessionSource.ReadPageLang`,
  `internal/mcp/dashboard_page.go#L42-L87`), which
  base64-inlines every board image (`ImageDataURL`, `image_route.go:218`) and
  escapes the result twice (goldmark URL escape, then `HTMLEscapeString` into
  each topic frame's srcdoc, `framedDoc` in `wswhiteboard/render.go`).
- The index page's `boardCounts` memo (`pages.go:789-820`) is keyed on the
  session Rev, so every snapshot change re-renders boards there as well.
- Measured: 124-219% CPU with a board holding 18 PNGs (3.2 MB), 13-19% with
  the same activity and no images, 0.5-2% idle.
- The polls (tracker watch every 2 s in `wstrackerhost/watch.go`, index
  refresh every 30 s, git watcher every 3 min, SSE 15 s tick) total under 5%
  on local disk; over the Windows-WSL 9P mount each stat is a round trip.
  The tracker remote poll (`StartPoll`, every 3 min,
  `wstrackerhost/host.go:24`) calls `TrackerChanged` even on a no-op round
  (`wstrackerhost/poll.go#L130-L142`, wired at `cmd/ws-mcp/dashboard.go:97`;
  `TrackerChanged` is `tracker_routes.go:423`); the 2 s tracker watch calls
  it only when a store stamp changed (`wstrackerhost/watch.go#L48-L55`). Pages carrying a mailbox-wait marker are
  volatile and fully re-render on every 15 s tick (the stream already skips
  re-sending unchanged regions by hash, `sse.go:382-388`; the cost is the
  render). `Volatile` is set when the marker file exists, live process or not
  (`dashboard_page.go:76-78`), and every wake-driven render leaves
  `stampOK=false` (`stream_cache.go:146-162`), so the next tick re-renders
  every page that was just woken, not only volatile ones.
- No ws-mcp tracker write sends a nudge (no call site in
  `mcp/tracker_tools.go`), and the `/nudge` body carries only
  `{reply_id, project}`, never reaching `TrackerChanged`. `PollOnce` returns no
  change result (`wstrackerhost/poll.go:77-85`).
- The git watcher's 3 min check and the heartbeat-driven repo read
  (`mcp/live_snapshot.go:410-419`) wake `gitDep(root)` after every read,
  changed or not (`wsdashboard/daemon.go:280-283`), re-rendering every session
  page on that root.
- inotify is unreliable over 9P and the module has no fsnotify dependency, so
  "event-based" here means explicit nudges from ws-mcp writers, with the polls
  kept as long backstops.

Frontend audit (code reading, not measured; `assets/dashboard.js`,
`assets/tracker.js`, `wswhiteboard/assets/`):

1. Conversation entries are never evicted from `.centries` (`mergeEntries`
   dashboard.js:404-421; history pages "never evicted", :432-454), and
   `fmtTimes` runs every second (:61) over every `time[data-ms]`, rewriting
   `textContent` and `title` (`toLocaleString`) even for clock times that
   never change. Per-second work grows with every entry over hours.
2. Each entries swap re-parses the whole 50-entry window (`frag(html)`,
   :400, :425), rebuilds the `have` map from every child (:406), and runs
   `sweepOutImgs` over the whole region and a document-wide mermaid query
   (:362, :369).
3. Each board swap (`el.innerHTML = html`, :364) recreates every topic
   iframe, unchanged ones included; a frame with mermaid loads and compiles
   the 3.5 MB mermaid bundle again, outside-root image relays re-run per new
   frame (`relayImage`, :1788), and `refetchClosed` (:516-520) re-fetches
   every closed-topic page the user ever expanded.
4. Tracker: `tkReadAll` calls `tkNotify()` per store view but the last
   (dashboard.js:2792-2796) plus once in `done` (dashboard.js:2819), and
   `adapter.check` notifies too (dashboard.js:2855), so N stores give N full
   renders per revision;
   `render()` rebuilds the drawer (`body.innerHTML = drawerHtml(...)`,
   tracker.js:3761) and forces layout (`fitDrawer`, :3633-3640) even with no
   drawer open, walking all events and comments. (dashboard.js and tracker.js
   line numbers are as of develop 4def031d0; re-pin on contact.)
5. Support chat entry boxes are never trimmed (`supRegion`,
   dashboard.js ~:3147).
6. Nothing pauses while the tab is hidden: no `visibilitychange` or
   `document.hidden` check exists in either asset tree.
7. `applySent` (:1066-1083) re-adds `sent-*` classes on every board or
   convlive swap, which trips pill.js's MutationObserver and its
   `getBoundingClientRect` pass (`wb/pill.js:92-108`, :136-137).

## Decisions

1. **A snapshot-only change does not re-render the whiteboard.** The board
   half of a page render is memoized per session, keyed on a fingerprint of
   every input to the board render other than the live snapshot. The rule is
   the principle; the list below is what it means today
   (`mcp/dashboard_page.go:59-86`, `mcp/whiteboard_tools.go:116-186`), and an
   input found later joins the key:
   - the whiteboard record's `Seq` (`mcp/dashboard_page.go:60`, bumped by
     every store write);
   - the todo and agenda contents the panels read (`rec.Todos`,
     `rec.Agenda`), fingerprinted directly, because they sit outside the
     whiteboard state and `Seq` covers them only when
     `refreshWhiteboardAfter` succeeds (`mcp/whiteboard_tools.go:372-392`);
   - the language;
   - a stat stamp of the page's watched files;
   - the mailbox listening marker's presence and its process liveness
     (`wsstate.ProcessAlive`, `mcp/whiteboard_panels.go` ~:246-252);
   - the re-ask times;
   - in dev-asset mode (`DevSourceEnv`, templates re-read per render), the
     template sources' stat stamp, or no memo.
   Not in the key: mailbox owner and identity, which are constant on the
   served page (`servedWhiteboardPanels` never sets `mailbox`,
   `mcp/dashboard_page.go:152-162`). Outside the memo entirely: `Contacts`
   (clock-dependent) and `MailBodies`, which `ReadPageLang` also returns and
   which feed the live regions; they are recomputed per render as today. A
   live-snapshot nudge then re-renders only the live regions. A test changes
   each key input in turn and asserts the memo misses, and asserts a
   contacts-only change still updates its region. A memo hit returns
   everything the last board render produced that the stream path reads,
   not only the HTML: the `PageContent.Watch` list filled during the render
   (the `DocImageSrc` wrapper and the panels' watch callback,
   `mcp/dashboard_page.go:66-73`) and the `Volatile`/marker state, so
   `sessionStamp` keeps its paths after the first hit.
   Rejected: a new content hash of the record (`Seq` already changes on every
   write and is cheaper); splitting the wake token by nudge kind alone (the
   render that a wake triggers still rebuilds the board); reusing the
   refresh stamp's inputs as the key (`sessionStamp`, `pages.go:677-709`,
   folds Rev, liveness, Watch stats, and contacts, but not `Seq`, language,
   or re-ask times).
2. **Board image data URLs are cached by (resolved path, size, mtime) in a
   bounded cache.** The key is the real path `containedImage` returns after
   resolving symlinks and checking containment (`image_route.go:274-302`),
   not `docImagePath`'s unresolved spelling, so the root check and the 2 MiB
   cap still run on every lookup's target. An over-cap result is cached too,
   so an oversized image is not re-read cap+1 bytes per render. Inlining
   stays the delivery mechanism, so the sandboxed srcdoc frame contract is
   unchanged. Rejected: serving in-root images by URL - a frame without
   `allow-same-origin` changes the image fetch and origin story, a larger
   change than this bug needs.
3. **The index page's per-session board counts come from the whiteboard
   store without rendering the board.** The index uses only `HasBoard` and
   `CountPolicy` (`pages.go:783`, `pageview.go:740-741`); `CountPolicy`
   equals the count of open policy items in the state, since waiting topics
   are never cut by the closed cap (`view.go:394-395`, :487-497). Both are
   computed in `ReadSession`, where the record is already loaded
   (`dashboard_sessions.go:446`), and the `boardCounts` memo is removed. This
   also fixes the stale count after a session leaves and rejoins (0f1872fb).
4. **Polls become backstops only where a nudge already covers them.** Each
   poll's outcome:
   - tracker store watch (2 s, `wstrackerhost/watch.go`): **keep**. It is
     the only path for external writers (no ws-mcp tracker write nudges),
     and 261109-bug-ws-tracker-external-write-refresh promised those show
     within seconds.
   - tracker remote poll (3 min, `StartPoll`): **keep the interval**;
     `PollOnce` gains a changed result (or the round compares a stamp), and
     a no-op round skips `TrackerChanged`.
   - git watcher (3 min) and the heartbeat-driven repo read: **keep the
     intervals**; wake `gitDep(root)` only when the read result changed,
     comparing the badge without `badge.At`, which `gitWatcher.run` stamps
     on every read (`gitbadge.go:346-352`; nothing in dashboard.js reads
     the rendered `data-at`).
   - index refresh (30 s): **keep** (already a backstop).
   - SSE tick (15 s): **keep the interval**; a tick re-renders a page only
     when its stamp changed. The stamp covers the listening marker's process
     liveness, not only the marker file's existence, and a wake-driven
     render takes its stamp before rendering and records it, so the next
     tick does not re-render a page it just rendered and a change during the
     render is still seen.
   Rejected: adding a tracker-write nudge in this ticket (no external writer
   could use it, so the 2 s watch would stay anyway).
5. **ws-mcp coalesces agent-only live-snapshot changes more coarsely**
   (`mcp/live_snapshot.go`, today 100/250 ms). The intake loop classifies by
   event (`mcp/live_snapshot.go:301-315`), since `wslive.Result` carries only
   Changed, Urgent, and Ring: a batch whose events are all `KindAgent` with a
   non-empty `AgentID` (subagent rows, including the convlive subagent
   counts they move) is agent-only and waits up to 1 s; anything else - a
   `KindPermission` event that is not a clear (a new prompt), a lead-level
   `KindAgent` event (lead batches shown in convlive), or `KindStatus` - is
   sent at today's latency. Whiteboard asks are not snapshot events; they
   travel through `whiteboard.update`'s own nudge and are unaffected.
6. **Frontend: per-second work touches only relative times.** Clock and
   weekday times and every `title` are formatted once per element (marked)
   and again only on a language change; the per-second pass queries only
   relative-time elements, writes only on change, and skips while
   `document.hidden`.
7. **Frontend: the conversation and support-chat entry boxes are capped.**
   While the view is pinned to the bottom, entries beyond 3x the server ring
   size (`RingCap`, 50) are dropped from the top. In the session
   conversation the history cursor (`histBefore`) and the end-of-history
   flag (`histEnd`, which makes `loadHistory` return early) are reset so
   scrolling up refetches them, and an epoch (or abort) guard drops a `loadHistory` page
   still in flight across that reset, so no non-contiguous page is
   prepended. The support chat has no history paging (`supRegion` only
   merges), so its evicted entries are gone until the chat reopens, matching
   a reopen that shows only the newest window. `pin` is computed in `swap`
   (dashboard.js:356), so the eviction runs there or takes `pin` as an
   argument. Each entries swap keeps a persistent key set and runs image
   sweep, open-state restore, and mermaid only on the inserted nodes. When
   the newest window's oldest key is newer than the newest held key (a gap
   after reconnect), the box and the history cursor reset; nodes re-inserted
   by such a reset are replayed history, never live arrivals: they run no
   live-arrival behavior (a support chat `ui` marker does not act, and no
   new-entry highlight or bottom auto-scroll fires for them beyond what the
   reset itself does).
8. **Frontend: a board swap keeps unchanged topic frames.** Each topic
   section gains an escaped `data-topic` key attribute
   (`wswhiteboard/assets/page.tmpl:29` emits none today), and the board is
   morphed by that key. "Unchanged" compares the incoming topic HTML (or its
   hash, kept per key) with the previous incoming HTML, never the live DOM,
   which ago.js, `applySent`, `writeAnswers`, and ThemeFrames mutate. An
   unchanged topic keeps its existing DOM and iframe. Closed-topic pages
   already fetched are kept across swaps, deduped by topic key against the
   new capped closed section, since closed-topic indices follow close order
   and shift when a topic newly closes: a topic never shows twice, and none
   drops out between the capped section and the kept pages.
9. **Frontend: the tracker renders once per change burst and not at all
   while closed.** Coalescing sits in the render subscriber (`AD.onChange`,
   tracker.js ~:4554, which also calls `wkRefresh()`; the coalescer covers
   both), not in `tkNotify` or `tkReadAll`: the per-store
   synchronous notify order stays as 025cf028 and
   `tracker_readall_test.go` pin it. A notify schedules one render per
   short timer tick (a `setTimeout(0)`-class deferral, not
   `requestAnimationFrame`, which browsers pause in a hidden tab and would
   leave Decision 10's hidden-tab tracker state stale); the synchronous `render()` calls from user actions stay
   synchronous and cancel a pending frame. With no drawer, page, or item
   window (`ui.win`) open, a render updates only the handles and badges and
   still keeps `X.rendered`/`supRendered` and the VS/EV/FLAGGED/UNREAD state
   current.
10. **Frontend: a hidden tab keeps its streams and defers heavy region
    swaps.** While `document.hidden`, the main EventSource stays open; a
    region event (board, entries, convlive, and the other page regions)
    records only its latest HTML and hash per region and applies nothing,
    and on `visibilitychange` to visible each region with a recorded update
    is applied once, newest only, through the usual swap path. The recorded
    HTML is kept until applied, so a reconnect while hidden may name its
    hash in `held()` without losing it. The `mode` pseudo-region is handled
    as today even while hidden (it reloads the page unless a draft is held),
    and `modeHash` stays consistent with what `held()` reports. Light events
    are still handled while hidden: `tracker` events (the store
    notifications, `dashboard.js:3517`) still run their handler, with
    Decision 9's closed-panel early-out keeping that cheap, so a later
    tab-label alert can see new notifications. The support chat stream also
    stays open while hidden (user, 2026-11-10, board (587)): a `ui` marker of
    261110-feat-ws-tracker-support-agent-ui-commands acts only when it
    arrives live, so a command sent while the user is on another browser tab
    still acts. User, 2026-11-10, board (588), replacing the first reading of
    this decision. Rejected: closing the main stream on hide (the page could
    not learn of new notifications while hidden, which blocks a tab-label
    alert); closing it and checking notification counts through a separate
    lightweight path (a second channel to keep in step); deferring the
    support chat stream's close until no reply is in flight (an extra
    in-flight condition).
11. **Frontend: `applySent` skips when the sent data is unchanged** (a
    signature of the `.sentdata` contents).

## Constraints

- The lead's measurement harness may be gone when this runs (it lives in a
  session scratchpad). If it is missing, rebuild an equivalent from the
  setup Background describes (an isolated daemon, a board with about 18 PNGs
  totalling ~3 MB, synthetic live-snapshot churn through `dashboard.intake`,
  one session stream and one index stream open) and report CPU before and
  after; there is no CPU target, only the reported comparison.
- Never touch the user's dashboard daemons (ports 29417 and 45743, including
  the Windows daemon). Measurements and headless checks use an isolated
  daemon with its own `WS_CACHE_HOME` / `WS_CONFIG_HOME`.
- Heavy go build, test, and vet run on the remote build server through
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m <minutes> -- <go
  args>`, never locally; local `node --check` and a headless-browser check
  under `nice -n 19` are allowed.
- Caller-visible behavior is unchanged: every region still updates, at the
  same or better latency for user-visible changes (Decision 5's exception).
- `agents-plugin-tool/` edits follow
  `ai-docs/manuals/shipped-surface-boundary.md`; `internal/mcp/` edits follow
  `ai-docs/manuals/ws-mcp.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)

## Prior Decisions

- 261109-bug-ws-dashboard-stream-wake-rerender-cpu (2026-11-09, Result): "targeted wake, shared render, refresh skip, index board-count memo ... review fixes: mailbox panel inputs in the refresh stamp, volatile pages, failed renders retried" — bearing: constrains
- 0f1872fb (2026-11-09, commit): "Open minor carried from the report: index board-count memo keyed by reply-id + Rev can show a stale policy count after a session leaves and rejoins until its next change." — bearing: supports
- 261109-bug-ws-tracker-external-write-refresh (2026-11-10, Result): "Page: `tkReadAll` notifies the mounted renderer as each store's view arrives; the last store's notify comes from `tkReload`'s done path." — bearing: constrains
- 025cf028 (2026-11-10, commit): "Rejected: ... moving the final notify into tkReadAll (would reorder it relative to the tkNudgedRev check and thens)." — bearing: constrains
- 261109-bug-ws-tracker-external-write-refresh (2026-11-10, Result): "Store watch: `wstrackerhost.Host.StartWatch(interval, onChange)` (`DefaultWatchInterval` 2 s) re-reads the registry each tick and stamps each store's journals (size + mtime) and the loose/packed/reftable refs" — bearing: constrains
- 261106-epic-ws-dashboard-serve-daemon (2026-11-06, Cross-Child Decisions): "Nudge plus polling. After a store write, ws-mcp sends a fire-and-forget POST /nudge ... A polling backstop of about 30 seconds catches missed nudges. Rejected: fsnotify (platform variance)." — bearing: supports
- a068fb9f (2026-11-07, commit): "Chromium sends no SameSite=Lax cookie from a sandboxed srcdoc frame, so in LAN mode the image route would refuse Now images. New View.NowImageSrc ... inlines them as data: URLs" — bearing: supports (View.NowImageSrc has since been replaced by DocImageSrc)
- 75ff0c2b (2026-11-06, commit): "Region diffing by content hash: each region element carries data-h; the client names held hashes in the stream URL, so a fresh stream or a reconnect sends exactly what changed." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wsdashboard/sse.go, pages.go, stream_cache.go, image_route.go, tracker_routes.go; internal/mcp/dashboard_page.go, live_snapshot.go; internal/wstrackerhost/poll.go; cmd/ws-mcp/dashboard.go; wsdashboard/assets/dashboard.js, tracker.js; wswhiteboard/assets/pill.js |
| scope.surface | cross-module | the board memo splits DashboardSessionSource.ReadPageLang in internal/mcp from the wsdashboard render path; nudge and poll changes span mcp, wsdashboard, wstrackerhost, cmd/ws-mcp, and the page assets |
| scope.new_public_symbol | unknown | the board memo and image cache may need an exported seam between internal/mcp and internal/wsdashboard; the ticket does not name one |
| scope.new_type_contract | unknown | Decision 1 may change the PageContent or LangPageSource split between board and live regions; not specified |
| scope.test_surface | existing | wsdashboard/stream_cache_test.go, pages_test.go, image_route_test.go, tracker_readall_test.go, mcp/dashboard_page_test.go, wstrackerhost/watch_test.go |
| complexity.reuse_points | confirmed | stream_cache.go refresh stamp and statStamp over PageContent.Watch, boardCounts in pages.go, liveSeams debounce in mcp/live_snapshot.go, existing data-h region hashing |
| complexity.side_effect_risk | high | caching the board render and closing streams on hide alter what every open dashboard page receives and when |
| risk.correctness | high | a memo key that misses a board input such as mailbox panel state, the listening marker, or contacts serves a stale board; entry eviction and the history cursor reset can drop or duplicate entries |
| risk.fit | moderate | must keep the per-token wake, shared render cache, and refresh-stamp design from 261109-bug-ws-dashboard-stream-wake-rerender-cpu |
| risk.test | high | frontend behavior is verified only by headless-browser checks and a CPU measurement, not unit tests |
| risk.security_or_contract | moderate | the image cache must keep containedImage root checks and the 2 MiB cap; Decision 5 changes live-snapshot latency for agent-only changes |

## Phases

### Phase 1: daemon render and nudges

Decisions 1-5. Verification: tests for the board memo (a snapshot-only change
reuses the board render; each key input of Decision 1 changed in turn
misses it; a contacts-only change still updates its region), the image
cache key (resolved path; an over-cap result cached), index counts from
`ReadSession` (including leave and rejoin), the tracker no-op skip, the
changed-only `gitDep` wake, the SSE tick (a page woken and rendered is not
re-rendered on the next tick when nothing changed; a dead listening process
changes the stamp), and Decision 5's split (a subagent-only batch waits up
to 1 s; a permission prompt, a lead-level event, or a status event goes out
at today's latency); `go test ./...` under `agents-plugin-tool` through
`ws-rgo`; and a before/after CPU measurement (see Constraints for the
harness), reported in the Result.

### Result (b0e680e23) - 2026-11-10

- Board memo in `DashboardSessionSource` (mcp), per reply-id and language,
  keyed by board Seq, todos, agenda, re-ask times, language, and the
  listening marker's PID and liveness, and verified by a stat stamp of the
  files the render named (taken before the render, trusted only when the
  new render named the same files). Off with dev assets. Contacts stay
  outside the memo.
- `ReadSession` carries `CountPolicy`; the index reads no session page and
  the reply-id+Rev board-count memo is removed (fixes the leave/rejoin
  stale count).
- `PageContent.Volatile` replaced by `WatchPIDs`; the refresh stamp folds
  each PID's liveness, and every render after the first takes its stamp
  first, so a woken render is not repeated on the next tick.
- `ImageDataURL` caches by resolved file + size + mtime in a 64 MiB LRU;
  over-cap results are cached as empty, read errors are not cached.
- Git watcher wakes `gitDep` only when the badge (ignoring its read time)
  changed. `PollOnce` reports change; `StartPoll` notifies only on change
  and on its first round. The 2 s watch, 15 s tick, and 30 s index refresh
  are unchanged.
- Decision 5: an intake batch whose every change is a `KindAgent` event
  with an `AgentID` waits up to `debounceAgent` (1 s); permission prompts
  (including a subagent's), lead batches, and status keep 100/250 ms.
- Verification: new tests for each item above; `go test ./...` via ws-rgo
  green. CPU (remote server, isolated daemon, 18 PNGs ~3 MB, ~20 subagent
  events/s), before -> after: images on, session+index 192.6% -> 19.9%,
  session 130.8% -> 20.6%, index 135.4% -> 10.0%; images off,
  session+index 28.0% -> 13.7%; idle <= 0.5% both. No-stream driving
  (the intake process itself) 6.9-12.3% -> 10.2-11.1%, within run noise.

### Phase 2: frontend accumulation

Decisions 6-11. Verification: `node --check` on the changed assets; `go test
./...` through `ws-rgo`; a headless-browser check on an isolated daemon that
(a) after more than 3x `RingCap` synthetic entries the session conversation
box stays capped and scrolling up still loads history, the support chat box
stays capped too, a forced gap (reconnect with a newer window) resets the
box without duplicates, and an entries swap runs image sweep, open-state
restore, and mermaid only on the inserted nodes (for example, a counter or
spy on those passes); (b) a board swap that
changes one topic keeps the other topics' iframe elements (same node
identity), and an expanded closed-topic page survives a swap, including one
where another topic newly closes, with no topic shown twice; (c) the
per-second timer leaves clock times untouched, writes nothing when no
relative time changed, and does no work while `document.hidden`; (d) while
the tab is hidden the main and support chat EventSources stay open, region
events change no DOM, a `tracker` event still reaches its handler, a `ui`
line arriving while hidden still acts, and on show each region shows its
newest content; (e) a multi-store tracker revision
triggers one drawer render, and none while the drawer and item window are
closed; and (f) an unchanged `.sentdata` swap does not touch `sent-*`
classes.

## Sage Review Round 1 (2026-11-10)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Hidden-tab close of the support chat stream (Decision 10) vs ready ticket 261110-feat-ws-tracker-support-agent-ui-commands | important | missing |
| 2 | Decision 1 memo key list does not match the board render's real inputs | important | autonomous |
| 3 | Decision 4 volatile-page sentence describes behavior that already holds | minor | autonomous |
| 4 | Polls versus nudges facts the plan needs (Decision 4) | minor | autonomous |
| 5 | Decision 2 cache key should use the resolved path | minor | autonomous |
| 6 | Decision 3 has a simpler and better seam | minor | autonomous |
| 7 | Decision 5's agent-only boundary is underspecified | minor | autonomous |
| 8 | Decision 8 needs a topic key and closed-page dedupe | minor | autonomous |
| 9 | Decision 9's seam is the render subscriber, not tkNotify | minor | autonomous |
| 10 | Decision 7 support chat and history-race details | minor | autonomous |
| 11 | Drifted or stale citations | minor | autonomous |

### Completeness Reviewer — concern

| # | Title | Severity |
|---|-------|----------|
| 1 | Decision 4 does not say which polls change | important |
| 2 | Phase 1 verification skips part of Decision 4 and all of Decision 5 | minor |
| 3 | Memo key list leaves out contacts, which Route Facts names | minor |
| 4 | Phase 2 browser check covers only some of the frontend decisions | minor |
| 5 | Measurement harness is in a session scratchpad | minor |
| 6 | No related or parent frontmatter | minor |
