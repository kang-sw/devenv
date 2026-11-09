---
title: "ws dashboard daemon: every stream re-renders its whole page on any session change"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 1b8518755d8f7989
sage-review-completeness-reviewed: 1b8518755d8f7989
completed: 2026-11-09
---

# ws dashboard daemon: every stream re-renders its whole page on any session change

## Background

The user saw the Windows dashboard daemon (`ws-mcp-dashboard-*.exe`) hold
CPU steadily (~0.64 core); the WSL daemon averages ~16%. A 10 s kernel-file
ETW trace of the Windows daemon showed ~1,500 file opens/s, ~2,600 reads/s
(9.3 MB/s), ~7,000 query-info/s, no child processes, 23 open connections.
Opens concentrate on other projects' `ai-docs/tickets/{ready,todo}`
(listings plus each ticket file), `mailbox-history/<key>/*.json`,
`mailbox-replyids.json`, `dashboard/sessions/<rid>/{ring,inbox/drained,snapshot.json}`,
and `sessions/<key>/{agenda,todos,whiteboard,key}.json`.

A 30 s CPU profile of the WSL daemon through the opt-in pprof (aa0ccf288,
`WS_MCP_DASHBOARD_PPROF=1`) puts ~54% of samples under
`(*Daemon).stream` -> `handleSessionEvents` -> `pageReader.readPage` ->
`wswhiteboard.RenderRegions` (goldmark parse, template execution,
`HTMLEscape`, regexp), and most of the rest in GC driven by those
allocations.

Code read (develop at aa0ccf288):

- `broker.wake(replyID)` (`internal/wsdashboard/sse.go` ~:71) ignores its
  argument and signals every subscribed stream.
- `sessionChanged` (`daemon.go` ~:350), the index's change hook, calls it
  for any session whose fingerprint changed; `agentChanged`, the git
  watcher, the inbox and tracker routes call it too.
- Each stream's `send()` re-reads that page's session state from disk and
  re-renders every region, then compares region hashes and sends only the
  changed ones. The 15 s `sseRefreshInterval` tick does the same per
  stream.

So the work per change is (open streams) x (full page read + render), and
a change in any session wakes streams of every other session. With many
live sessions (each MCP write and snapshot heartbeat is a change) and
several tabs, the daemon re-reads and re-renders continuously.

## Decisions

Numbers in parentheses are the lead session's whiteboard item IDs.

- **Measure first** (115, confirmed by the user on the board 2026-11-09):
  measure before and after on a dev daemon with several sessions and
  streams (CPU and file opens over a fixed window, wake counts per source),
  and fix further only what the measurement shows. Further fixes are bounded:
  a cost the measurement finds outside the wake and render path (the
  snapshot-heartbeat rate, the 30 s index poll) is measured and reported in
  the Result and goes to a follow-up ticket, not into this one.
  Rejected: guessing at further caches - the snapshot-heartbeat rate and the
  30 s index poll may matter as much as the render.

- **Targeted wake** (116 a, user 2026-11-09): a change in session A wakes
  only the streams whose page can show A: A's own page, pages that list A
  as a peer or in their agent / delegate tree, and the sessions index. A
  wake with an empty replyID (tracker_routes.go) and the git, inbox and
  tracker wakes still reach every page that shows that state.
  Auto-confirmed (design review, 2026-11-09): where the target set comes
  from. The index holds no peer or agent-tree data (peers come from each
  page's Contacts through the PageSource, the agent tree from its snapshot's
  Agents), so the set cannot be computed from the index. Each stream records
  its dependency set from its last render: its own replyID plus every replyID
  that render looked up (pageInputs.Lookup). A page that newly starts showing
  A is woken by its own snapshot or mailbox change, which re-renders it and
  refreshes its set. The sessions index page depends on every session.
  This reverses 75ff0c2bc "One broker wakes every stream on any index change
  ... Rejected: per-session subscriptions, which would miss peer and delegate
  changes". The dependency set answers that concern by waking the pages that
  show A, not only A's own page.
  Rejected: keeping the broadcast and relying only on the shared render and
  the refresh skip. With 15+ live sessions most wakes are irrelevant to most
  pages, and the broadcast still runs a check per stream per change.
- **Shared in-memory render** (117 a, user 2026-11-09): every stream of the
  same session and language shares one render per change. The render is held
  in memory until that session's next change or refresh tick. The cache is
  derived only from disk state and starts empty, so a replacement daemon
  still renders the same pages; what changes is that a render no longer
  always re-reads. The stream render path takes no viewer class today
  (handleSessionEvents calls renderSessionRegions without localViewer), so
  the key is session + language unless the worker finds a viewer-dependent
  region.
  Auto-confirmed (design review, 2026-11-09): the theme. The stream render
  is per-viewer through the theme cookie: `view.Theme = requestTheme(r)` and
  the board region renders `ThemedBoard` (`wswhiteboard.ThemeFrames`), which
  pins each topic-body frame's srcdoc to the theme. So the key also carries
  the theme, or the board is cached theme-neutral and `ThemeFrames` is
  applied per stream; the worker picks. A stream with a `?regions=` filter
  shares the full render and filters a copy: `regionFilter.apply` deletes
  from the map it is given, so it must never run on the shared set.
  This reverses 75ff0c2bc "The daemon holds no page state: every render
  re-reads, so a replacement daemon renders the same pages".
  Rejected: keeping no page state. N tabs on one session would cost N full
  renders per change.
- **Refresh skip** (118, acknowledged by the user 2026-11-09): the 15 s
  refresh tick skips the read and render when the session fingerprint, the
  ticket directory mtimes, and the liveness state computed at tick time are
  all unchanged since the last render.
  Auto-confirmed (design review, 2026-11-09): the key must cover every file
  input the render reads that no fingerprint catches within 15 s. A directory
  mtime does not move on an in-place edit, and the index fingerprint covers
  ticket panels only on its 30 s poll; in-root topic-body images are inlined
  as `data:` and are in no fingerprint. So the key also holds the mtimes of
  the ticket files the page lists and of the images the render inlined (or
  the skip is narrowed until it covers them). Any state left uncovered keeps
  today's unconditional 15 s re-render.
  Rejected: re-rendering unconditionally. 75ff0c2bc keeps the tick because
  liveness windows follow the clock, so liveness is part of the skip key; the
  other parts of the key only catch missed changes.

## Constraints

- `ai-docs/manuals/shipped-surface-boundary.md` applies to every file under
  `agents-plugin-tool/`.
- No page may show a state older than today's behavior allows: a cached
  render must not outlive the existing 15 s refresh for state the daemon
  cannot fingerprint (e.g. ticket lists of the session's project).
- Region hashes and the SSE event protocol the pages consume stay unchanged.
- Dev daemons use isolated `WS_CACHE_HOME` / `WS_CONFIG_HOME` and a spare
  port; never the user's daemons on 29417 / 45743.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)

## Prior Decisions

- 75ff0c2b (2026-11-06, commit): "One broker wakes every stream on any index change (a peer's change shows in another page's tree)... Rejected: per-session subscriptions, which would miss peer and delegate changes." — bearing: contradiction-candidate
- 75ff0c2b (2026-11-06, commit): "The daemon holds no page state: every render re-reads, so a replacement daemon renders the same pages." — bearing: contradiction-candidate
- 75ff0c2b (2026-11-06, commit): "Region diffing by content hash: each region element carries data-h; the client names held hashes in the stream URL, so a fresh stream or a reconnect sends exactly what changed." — bearing: constrains
- 126fa9d9 (2026-11-08, commit): "Hooked in the session stream's render rather than in the index change hook: the render already reads the snapshot (view.Band.Repo...), and a stream exists exactly while the root is watched" — bearing: constrains
- ef0138ad (2026-11-08, commit): "The tracker nudge rides the session page's existing SSE stream as an `event: tracker` rather than a second EventSource" — bearing: constrains
- 261108-feat-ws-tracker-support-agent-chat (2026-11-09, Result): "The session stream now takes `?regions=entries,convlive` and sends only the regions it names. With a filter, the mode pseudo-region is dropped and git is not watched" — bearing: constrains
- b150fe83 (2026-11-07, commit): "Lead-reply markdown moved out of conversation() into the cached entry render (ConversationView.markdown); the streaming reply still renders every tick since it changes." — bearing: supports
- 261107-feat-ws-dashboard-i18n 93a7c38a (2026-11-07, commit): "held-draft language switch leaves the open SSE stream in the old language until reload" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/sse.go, daemon.go, inbox_routes.go, tracker_routes.go, gitbadge.go wake callers, pages.go readPage |
| scope.surface | internal | broker, stream and sessionChanged are unexported; ticket keeps region hashes and the SSE event protocol unchanged |
| scope.new_public_symbol | no | none; any render cache stays inside package wsdashboard |
| scope.new_type_contract | no | none; broker.wake already takes a replyID argument it ignores at internal/wsdashboard/sse.go#L71 |
| scope.test_surface | existing | internal/wsdashboard/pages_test.go, gitbadge_test.go, inbox_routes_test.go, tracker_routes_test.go already open /events streams |
| complexity.reuse_points | confirmed | broker subscribe/wake in sse.go#L55-L80, region hash diff in stream sse.go#L204-L250, index OnChange hook index.go#L59-L61 |
| complexity.side_effect_risk | moderate | a shared cross-goroutine render cache and targeted wake can leave a page stale or miss peer and delegate changes |
| risk.correctness | high | wake routing must still reach pages that show peers, delegates, git, inbox and tracker state; tracker_routes.go#L245 wakes with an empty replyID |
| risk.fit | moderate | reverses the stateless-daemon and broadcast-wake choices recorded in 75ff0c2b |
| risk.test | moderate | tests must pin render counts and timing across concurrent streams, sseMinInterval and the 15 s refresh |
| risk.security_or_contract | moderate | a shared render keyed by viewer class must never hand a loopback-only render to a LAN viewer |

## Phases

### Phase 1: targeted wake and shared renders

- Measure the current wake and render rate on a dev daemon and record it.
  Setup, reused for the re-measure: about 6 registered sessions with live
  MCP writes, 3 SSE streams on each of 2 sessions plus one sessions-index
  stream, a fixed 60 s window. CPU from the daemon's /proc/<pid>/stat (or a
  pprof profile), file opens with `strace -f -c -e trace=openat` on the
  daemon pid, wake and render counts from temporary counters (removed before
  commit, or behind the existing pprof opt-in).
- Apply the targeted wake, the shared render and the refresh skip, then
  whatever further fix in the wake and render path the measurement points at
  under (115). Costs outside that path are reported, not fixed (see Decisions).
- Re-measure with the same setup and report both numbers.

#### Verification

From `agents-plugin-tool/`: `gofmt -l ./internal ./cmd`, `go vet
./internal/...`, `go test ./internal/wsdashboard/ ./internal/wswhiteboard/
./cmd/ws-mcp/ -count=1`. Go tests pin:
- A change in session A does not render a session B stream whose page does
  not show A.
- A change in A still wakes: A's own streams, a page listing A as a peer, a
  page with A in its agent / delegate tree, and the sessions index. A wake
  with an empty replyID, and the git, inbox and tracker wakes, still reach
  every page that shows that state.
- Two streams of one session, language and theme share one render per
  change; a stream with a different theme cookie gets topic frames in its
  own theme; a `?regions=` stream sharing the render does not strip regions
  from another stream.
- If any stream region turns out to depend on the viewer class, a LAN
  stream never receives a loopback render.
- The refresh tick skips when every key part is unchanged; it re-renders on
  a liveness-only change, after a new ticket file, and after an in-place
  edit of a listed ticket file.
Before / after CPU, file-open and wake/render numbers from the dev daemon,
with the setup above, in the Result.

### Result (f26637400) - 2026-11-09

Landed in 6431890de (counters behind the pprof opt-in, at
`/debug/pprof/streams`), dc8d5566e (targeted wake, shared render, refresh
skip, index board-count memo), f26637400 (review fixes: mailbox panel
inputs in the refresh stamp, volatile pages, failed renders retried).

- Targeted wake: the broker numbers wakes per token (a reply-id, or
  `git:<root>` for the git watcher). Each stream narrows to the tokens its
  last render looked up (own reply-id, peers, delegates), and a wake that
  lands during a render re-signals it. The index stream and an empty-token
  wake reach everything; the tracker signals every stream without staling
  any render.
- Shared render: `stream_cache.go` keys a page by reply-id (empty for the
  index), language, and theme. Theme went into the key instead of a
  theme-neutral board. No region depends on the viewer class, so the key
  holds none. `regionFilter.apply` copies.
- Refresh skip: the stamp covers looked-up sessions' presence, Rev,
  liveness, and adapter state; the stat of every path the page source names
  in the new `PageContent.Watch` (ticket dirs and files, inlined images,
  mailbox stores, history dir, listening marker); contact liveness via the
  new `ContactSource`. A page with an armed wait is `Volatile` and
  re-renders every tick, as before.
- Further fix the measurement pointed at: after the first three, each of
  200 index renders per window still re-read and re-rendered all six
  session pages for `HasBoard`/`CountPolicy`. The index now memoizes those
  per reply-id and Rev.

Measurement (script outside the repo; 6 `serve --stdio` sessions with live
adapter snapshots and peer mail, ~3 MCP writes/s, 3 streams on each of 2
sessions plus 1 index stream, 60 s window, isolated cache/config, spare
port). strace is not installed, so file reads are `/proc/<pid>/io` syscr,
which also counts socket reads.

| binary | CPU (% of a core) | read syscalls/s | session renders | index renders | shared | refresh skips | region sends |
|---|---|---|---|---|---|---|---|
| before (2 runs) | 20.4 / 19.5 | 2,401 / 2,408 | 1,200 / 1,204 | 200 / 201 | 0 | 0 | 653 |
| wake + share + skip | 14.0 | 1,348 | 337 | 199 | 658 | 16 | 653 |
| + index memo (2 runs) | 6.4 / 7.5 | 714 / 713 | 337 / 335 | 200 | 659 / 656 | 15 / 14 | 653 / 652 |
| final, after review fixes | 4.3 | 716 | 338 | 201 | 660 | 16 | 654 |

Wake counts were unchanged: 198 session wakes, no agent/git/inbox/tracker
wakes in the window. The same region sends mean the pages got the same
updates. Outside the wake and render path, the after-profile puts the
nudge-driven `ReadSession` fingerprint at ~5% of samples (~0.5% of a core).
That is too small for a follow-up ticket. The remaining cost is the session
render per real change to a viewed page.

Known minor: the board-count memo's Rev restarts at 1 when a session leaves
and rejoins the index, so a stale count can show until its next change.
