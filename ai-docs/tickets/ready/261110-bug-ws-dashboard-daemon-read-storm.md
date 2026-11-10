---
title: "Dashboard daemon read storm: tracker late-reload loop, per-nudge ticket reads, uncached immutable entries"
related:
  261110-bug-ws-dashboard-render-and-frontend-load-accumulation: "shipped in 0.48.14; this ticket fixes the server-side I/O it left, measured on a 0.48.14 daemon"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3bf93d831f667f8c
sage-review-completeness-reviewed: 3bf93d831f667f8c
---

# Dashboard daemon read storm

## Background

A downstream Windows machine running the ws 0.48.14 dashboard daemon
(`ws-mcp dashboard --port 29417`, 27 registered sessions, 7-8 active, about
21 open browser connections, the busiest project holding about 50 ready+todo
tickets) shows about 14,000 file operations per second from the daemon.
Measured 2026-11-10 from WSL through Win32_Process I/O counters: 2,000-3,100
ReadFile ops/s plus 8,000-12,400 other ops/s (open, attribute query,
directory enumeration), in bursts of 600-2,000 reads every 1-2 s.

A 20 s CPU profile of the same daemon (`WS_MCP_DASHBOARD_PPROF=1`,
`/debug/pprof/profile`) attributes it as follows (cum % of 24.8 s samples):

- 50% `wstrackerhost.(*Host).reloadLoop` -> `view` -> `batchRunner.RunGit`:
  36% `wsledger.(*Ledger).Reload` -> `gitRepo.listRefs` (reading git's
  stdout pipe counts as ReadFile ops), 14% `countCommits`, 14%
  `loadAttachRefs`.
- 14% `os/exec.LookPath` (`exec.Command("git", ...)` resolves `git` across
  every PATH entry and PATHEXT extension on each run: `GetFileAttributesEx`
  and `CreateFile` calls, the bulk of the "other" ops).
- 9% HTTP handlers (session page renders, nudges).

`/debug/pprof/streams` over the same 25 s: `wake_session` +18,
`session_renders` +30, `wake_tracker` +5.

The dominant cause is a reload loop in `internal/wstrackerhost/pageview.go`:
`ViewWithin` serves the previous snapshot after `dashboardViewWait` (500 ms;
the constant and the `ViewWithin` callers live in
`agents-plugin-tool/cmd/ws-mcp/tracker_host.go#L26`, `#L68`, `#L94`)
and marks the reload `late`; when the reload lands, `reloadLoop` calls
`h.changed()` for every late reload whether or not the view changed; that
tells every tracker tab to fetch again, which queues the next reload. On
Windows a reload (several git subprocesses) takes longer than 500 ms, so
every reload is late and the loop never settles while a tracker tab is open.

Secondary, verified in code (not individually measured):

- Every nudge runs `Index.Nudge` (`internal/wsdashboard/index.go`, under the
  global `rereadMu`) -> `Source.ReadSession`
  (`internal/mcp/dashboard_sessions.go`), which builds the served whiteboard
  panels only to fingerprint them; `ticketPanel`
  (`internal/mcp/whiteboard_panels.go`) calls `wsdoc.TicketsList`, reading
  every ready/ and todo/ ticket in full, plus mailbox history, the session
  record, and the contacts registry. A busy lead nudges up to about 10 times
  a second with no browser open; the 30 s refresh repeats it for every
  registration, ended ones included.
- Every session page render (`internal/wsdashboard/pages.go`, `ReadRing` and
  the inbox reads) re-reads the newest ring entries and drained inbox
  entries from disk, although an entry file is never rewritten (its name is
  its identity, as `HashLiveStores` in `agent_sessions.go` already relies
  on).

## Decisions

1. **Scope is A+B+C+D below; E (pausing tracker reloads in hidden tabs) is
   out.** (User, whiteboard (603) a.) Rejected: A alone first (leaves the
   measured primary cause); adding E (C removes the loop, and E would reopen
   the hidden-tab behavior of the related ticket's Decision 10).
2. **Ship as hotfix 0.48.15 right after the merge.** (User, whiteboard (604)
   a.) Everything on `develop` at that point ships with it.
3. **C - a late reload signals a change only when the view changed.** In
   `reloadLoop`, call `h.changed()` for a late reload only when the landed
   view differs from the snapshot the late readers were served (`st.snap`
   before the swap). The identity covers every input the view is built
   from in `host.go` `view()` / `fill()`: the folded ledger including
   uncommitted journal events (`ledger.Pending`, journal files), the refs,
   `host-sync.json` (sync time, error, wiki warnings), the commit counts, and
   the attach refs; the simplest correct form is the marshalled `StoreView`
   with `View.Now` zeroed (`wstracker/view.go#L35`, `#L431`), since `Now`
   differs on every reload and would keep the loop alive. A ref-map-only or
   revision-only identity is not enough: a journal-only write by another
   process (the watch stamp is consumed by the first, late fetch) and a
   `host-sync.json` change (which `watch.go` deliberately does not stamp)
   reach the page only through this signal. A reload that errors still
   signals nothing, as today. Rejected: raising `dashboardViewWait` (only
   moves the threshold; a slower machine loops again); rate-limiting
   `changed()` (keeps the wasted reloads); a ref-map-only identity (misses
   journal and sync-state changes). (Lead technical call from the profile;
   identity scope from design review.)
4. **C2 - resolve the git executable once per process.** The tracker host's
   `wstrackerhost.gitCommand` (`internal/wstrackerhost/runner.go#L21-L46`)
   resolves `git` through `exec.LookPath` once (cached, e.g. `sync.Once` or a
   small helper), and builds each command from the resolved path; a
   resolution failure falls back to the current per-call behavior so a later
   PATH fix still works. `wsledger` `gitRepo.run` needs no change: it spawns
   nothing itself and delegates to its `wsgit.Runner`, which in the tracker
   host is `batchRunner` -> `gitCommand` (`internal/wsledger/gitio.go#L29-L31`,
   `internal/wstrackerhost/host.go#L119`), so the profiled path is covered.
   Other git spawners in the daemon (for example `wsgit.ExecRunner`,
   `internal/wsgit/git.go#L34`) may adopt the same helper only when it is a
   one-line change. Rejected: leaving it (14% of the profile and most of the
   non-read ops); changing `wsledger` separately (no spawn site there).
   (Lead technical call from the profile; scope of the helper auto-confirmed
   from the fact populator's gap.)
5. **A - the nudge fingerprint stops reading ticket bodies.** The change
   detector behind a nudge and the 30 s refresh fingerprints the ticket
   board from directory metadata (file names, sizes, modification times of
   the ready/ and todo/ ticket files), and the mailbox history from entry
   names, instead of building the served panels. The full panel build runs
   only when the fingerprint changed (the render path, unchanged). The
   fingerprint keeps every other input today's panel hash covers: the
   session record, the contacts registry, the board / todo / agenda /
   snapshot / ring stores (62f64cda), mailbox unread queues (the named store
   and the reply registry's queues), and listening-marker presence and PID
   liveness. Reuse the board memo's stat-based check
   (`boardMemoBase` plus `boardWatchStamp` in `internal/mcp/dashboard_page.go`)
   where it already covers an input. The 30 s refresh skips a registration
   only when the indexed entry already holds the same ended registration
   (`EndedAt` set, unchanged since the last apply); the first refresh after
   `EndedAt` is set still applies once, a resumed session is re-read, and
   the cheap project-root stat stays so an ended session whose project
   disappeared is still dropped. Heartbeat-expiry non-liveness
   (`Registration.Live`) alone does not skip. A fingerprint must still change on every
   edit that changes what the page shows: a ticket edit that keeps size and
   mtime identical within the filesystem's timestamp resolution is the only
   accepted miss, caught by the next real change or the next refresh of an
   active session.
6. **B - immutable entries are cached by name.** Ring entries, drained inbox
   entries, and mailbox history entries are parsed once per daemon and served
   from an in-memory cache keyed by session and entry name; the cache drops a
   session's entries when the session's registration is removed, and is
   bounded per session to the window the page reads (the newest
   `wslive.RingCap` ring entries, the drained entries the page shows, and
   `mailboxHistoryCap` history entries), so memory does not grow with
   history. The mailbox history cache lives on `DashboardSessionSource`
   (threaded through `whiteboardPanelInput`), not in the shared
   `readMailboxHistory` reader the `mailbox.history` tool also uses; its
   eviction follows the index's registration removal into the mcp source,
   mapping reply-id to session key as the source already does. Held (not
   drained) inbox entries and the snapshot keep being read each time, since
   they change.
7. **D - counters for the new paths.** `/debug/pprof/streams` gains
   counters for: nudges received, `ReadSession` calls whose fingerprint
   changed (and so lead to a board render), fingerprint-only checks that
   found no change, tracker reloads, late reloads, and late reloads that
   signalled a change. Names follow the existing snake_case counters. The
   per-server `streamStats` in `wsdashboard` is unexported, and the tracker
   and session counts originate behind the `wsdashboard.TrackerHost`
   interface (implemented in `cmd/ws-mcp`) and in the mcp
   `DashboardSessionSource`: reach them through an optional stats interface
   on those seams, read by the streams route. No new route or env knob.

## Constraints

- Server-side Go only (`internal/wstrackerhost`, `cmd/ws-mcp` tracker host,
  `internal/wsdashboard`, `internal/mcp` dashboard source); no frontend or
  shipped-text change beyond what a counter needs.
- Never run go build/test/vet on the lead machine; run them remotely:
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- test
  <packages> -count=1 -timeout 30m`.
- Never touch processes or ports 29417 / 45743 (the user's daemons) from
  tests or probes; a dev daemon uses an isolated WS_CACHE_HOME /
  WS_CONFIG_HOME and its own port.
- `ai-docs/manuals/ws-mcp.md` applies to `internal/mcp/` edits; update its
  dashboard paragraph if a described behavior (tracker revision bumps,
  refresh) changes.
- Out of scope: E (hidden-tab reload pause); git-badge cadence; frontend
  fetch logic.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 3bb96191 (2026-11-10, commit): "Ticket Decision 1/2: the watch lives in wstrackerhost so wsdashboard stays tracker-free; the bound is a separate method (not a Host option) so the MCP reads (View) keep waiting for fresh state." — bearing: constrains
- 261109-bug-ws-tracker-external-write-refresh (2026-11-10, Result): "Minors left open: ... a late reload that fails signals nothing (the page waits for the next watch or poll change)" — bearing: supports
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Decisions): "Polls become backstops only where a nudge already covers them. tracker store watch (2 s, wstrackerhost/watch.go): keep. It is the only path for external writers" — bearing: constrains
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Result b0e680e23): "Board memo in DashboardSessionSource (mcp), per reply-id and language, keyed by board Seq, todos, agenda, re-ask times, language, and the listening marker's PID and liveness, verified by a stat stamp" — bearing: supports
- 261106-feat-ws-dashboard-daemon-core (2026-11-06, Decisions): "Nudge plus polling. After a store write, ws-mcp sends a fire-and-forget POST /nudge; the daemon re-reads that session. A polling backstop of about 30 seconds catches missed nudges." — bearing: constrains
- 62f64cda (2026-11-06, commit): "Phase 3 widens DashboardSessionSource's fingerprint to the panel stores and the sibling's snapshot and ring, so panel-only nudges reach SSE clients." — bearing: constrains
- 261107-feat-ws-dashboard-conversation-history (2026-11-07, Decisions): "History lives in the existing ring dir: the live view reads the newest 50 entries ... entries are already immutable, stamp-named files written lock-free by several processes" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wstrackerhost/pageview.go, internal/wstrackerhost/runner.go, internal/wstrackerhost/host.go, cmd/ws-mcp/tracker_host.go, internal/wsdashboard/index.go, internal/wsdashboard/pages.go, internal/wsdashboard/stream_stats.go, internal/mcp/dashboard_sessions.go, internal/mcp/whiteboard_panels.go |
| scope.surface | cross-module | packages wstrackerhost, wsdashboard, mcp, cmd/ws-mcp; streamStats in wsdashboard/stream_stats.go is unexported and per-server, so tracker and session counters arrive through optional stats interfaces on the TrackerHost and session-source seams |
| scope.new_public_symbol | yes | optional stats interfaces on the wsdashboard TrackerHost / session-source seams and new snake_case keys in /debug/pprof/streams |
| scope.new_type_contract | unknown | fingerprint and cache seams are not specified; the plan may add a seam field or helper signature for counting TicketsList and entry reads |
| scope.test_surface | existing | internal/wstrackerhost/pageview_test.go, runner_test.go, internal/wsdashboard/index_test.go, stream_cache_test.go, internal/mcp/dashboard_sessions_test.go |
| complexity.reuse_points | confirmed | wslive.RingCap at wslive/snapshot.go#L73, HashLiveStores entry-name hashing in wsdashboard/agent_sessions.go#L104-L107, existing StreamStats counters |
| complexity.side_effect_risk | moderate | a missed fingerprint input or cache eviction bug silently leaves the dashboard stale; git resolution change touches every tracker git spawn |
| risk.correctness | high | fingerprint must change on every page-visible edit and the late-reload identity must not suppress real changes; concurrency under viewMu and rereadMu |
| risk.fit | moderate | spans four packages and replaces the panel-build fingerprint that 62f64cda deliberately widened |
| risk.test | moderate | needs slow fake runner, resolution counting, and read-counting seams that do not exist yet |
| risk.security_or_contract | low | no route, env, or protocol change; only additive debug counters; cached LookPath keeps per-call fallback |

## Phases

### Phase 1: Break the tracker loop, cheapen git spawns, fingerprint without reads, cache immutable entries, count

Implement Decisions 3-7.

Verification expectations:

- A `wstrackerhost` test with a slow fake runner (reload longer than the
  wait): a late reload whose view is unchanged does not call the change
  callback, and one whose view changed does, including a journal-only
  change (same refs, a new pending event) and a `host-sync.json` change;
  the loop settles (bounded reload count) with a reader re-fetching on every
  change signal.
- A test that the git path is resolved once across many commands (inject or
  count resolutions), and that a failed resolution retries per call.
- A test that a nudge with no on-disk change does not read ticket bodies
  (count `TicketsList`/panel builds through a seam), that a ticket add,
  remove, rename, or edit changes the fingerprint, and that the refresh
  skips an already-applied ended registration but still applies the first
  refresh after it ended, re-reads a resumed one, and drops one whose
  project root is gone.
- A test that a second render of an unchanged session reads no ring or
  drained entry file again, that a new entry is read once, and that a removed
  registration drops its cache.
- The new counters move in those tests or in a streams handler test.
- Remote run of the touched packages' tests and vet.
- After the hotfix ships, the lead re-measures the downstream daemon with the
  same Win32 I/O probe and `/debug/pprof/streams`; that check is the lead's,
  not the worker's.

### Result (e8916a258) - 2026-11-10

Implemented Decisions 3-7 in two commits (3343077ef tracker host, e8916a258
dashboard).

- Decision 3: a late reload signals only when its view (JSON of `StoreView`
  with `View.Now` cleared) differs from a view served to a late reader; a
  late read whose context ended counts as differing (old behavior kept).
  Compared against the views actually served rather than the snapshot
  before the swap, so a reader late on `next` while `cur` landed a change
  still gets its signal.
- Decision 4: git resolved once per `PATH`+`PATHEXT` (keyed after the
  fake-git `PATH` tests showed a process-wide cache pins the first git); a
  failed lookup is not cached and falls back to bare `git`. `wsgit.ExecRunner`
  left unchanged.
- Decision 5: `ReadSession` fingerprints from metadata (record, live-store
  hashes, ready/todo listing name/size/mtime, board memo base incl. marker,
  reply-id unread count, history names, contacts; one registry load) with no
  panel build. Only the reply-registry queue is fingerprinted: served panels
  never read the named mailbox store. Refresh skips an applied unchanged
  ended registration while its project root exists.
- Decision 6: ring/drained (wsdashboard `entryFiles`) and mailbox history
  (mcp `mailboxHistoryCache`) cached by name, pruned to the shown window,
  dropped on index removal via the optional `SessionForgetter`.
- Decision 7: `nudges`, `session_reads_changed`/`_unchanged`,
  `tracker_reloads`/`_late_reloads`/`_late_reloads_changed` on
  `/debug/pprof/streams` via optional `wsdashboard.CounterSource`.
- `ai-docs/manuals/ws-mcp.md` dashboard, tracker, and profiling paragraphs
  updated.

Verification: remote `ws-rgo` test and vet of `./internal/wstrackerhost/
./internal/wsdashboard/ ./internal/mcp/ ./cmd/ws-mcp/ ./internal/wslive/
./internal/wsinbox/` pass; new tests also pass `-race -count=4`. Lite
review: clean, three Minor left open:

- An ended session whose session record vanished now stays in the index
  until its registration is pruned (previously dropped on refresh).
- A page render racing a session removal can re-create that session's
  entry/history cache (bounded, one window per race).
- A client aborting every fetch keeps forcing late-reload signals (old
  behavior, loop still settles).

Forward: the lead re-measures the downstream daemon with the Win32 I/O probe
and `/debug/pprof/streams` after the hotfix ships.
