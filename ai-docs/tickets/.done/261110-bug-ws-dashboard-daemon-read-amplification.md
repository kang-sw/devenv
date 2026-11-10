---
title: "Dashboard daemon read amplification: re-reads of unchanged small files on every render and nudge"
related:
  261110-bug-ws-dashboard-daemon-read-storm: "shipped in 0.48.15 (about 10x less I/O); this ticket cuts the residual it left, measured on a 0.48.15 daemon"
  261110-bug-ws-dashboard-render-and-frontend-load-accumulation: "its decision to keep the 2 s tracker store watch constrains Decision 7"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: eb0988dd8a51a423
sage-review-design-reviewed: eb0988dd8a51a423
completed: 2026-11-10
---

# Dashboard daemon read amplification

## Background

After 0.48.15 the user's Windows dashboard daemon (PID 688, port 29417,
5 open SSE streams) still does about 346 read ops/s and 2,757 other ops/s
(24 consecutive 10 s samples, 2026-11-10 15:48-15:52 KST, Win32 process I/O
counters), and the user finds hundreds of reads per second a concern
(whiteboard, topic "0.48.15 배포 후 재측정").

A read-only investigation (CPU and allocs profiles, `/debug/pprof/streams`,
a Win32 calibration of per-call op counts) attributed it:

- 97-99% of read ops are the daemon re-reading the same small files.
  Cumulative ReadTransferCount (627.9 MB) matches the allocs profile's
  `os.ReadFile` allocation (628.45 MB) over the same uptime. Git child pipes
  are 1-2% of reads; socket traffic counts under "other", not read.
- Calibration: Go's `os.Stat` costs 2 other ops and 0 reads; an
  `os.ReadFile` costs 2 reads and 4 other ops.
- The rate scales with page re-renders: about 39 read ops and about 350
  other ops per render (regression over the 24 samples), so at 7.1
  renders/s renders drive about 80% of reads and 90% of other ops. A nudge
  wakes about 2.9 renders, because a page wakes on any change in any session
  it looked up.
- Each wake runs `pageCache.get` (`internal/wsdashboard/stream_cache.go`):
  the change check `sessionStamp` re-reads the snapshot of the session and of
  every session the page looked up, re-reads the contacts, and stats every
  watched path (`statStamp`; about 50 ticket files on the busiest project).
  The render then reads all of it again: the session record parts
  (`lookupRecord`, `internal/mcp/dashboard_sessions.go`, `whiteboard.json`
  up to 150 KB), the contacts registry, the board's `boardWatchStamp`
  (`internal/mcp/dashboard_page.go`) stats of the same ticket files, and
  the snapshots again.
- By bytes over 327 s of uptime: session snapshots (`pageReader.readSnapshot`,
  `internal/wsdashboard/pages.go`) about 55 files/s; session record parts
  about 23 files/s (263 MB); `mailbox-replyids.json` (60 KB) and other
  mailbox files about 19 files/s (270 MB).
- `mailbox-replyids.json` is on every board's watch list
  (`mailboxPanel` in `internal/mcp/whiteboard_panels.go` calls
  `in.watch(ReplyRegistryPath)`) and was rewritten 7 times in 30 s. Each
  rewrite invalidates the board memo, so the next render rebuilds the board,
  which reads every ready/ and todo/ ticket through `TicketsList` (773 MB
  allocated over the uptime). This is the likely cause of an outlier sample
  of 57 MB read in 10 s.
- The git badge runner (`gitBadgeRunner.RunGit`,
  `internal/wsdashboard/gitbadge.go`) still calls
  `exec.CommandContext(ctx, "git", ...)`, so every spawn searches PATH
  (`os/exec` stat calls, about 20% of the stat CPU). The tracker host
  already resolves git once (`internal/wstrackerhost/runner.go`, read-storm
  Decision 4).
- The tracker store watch (`internal/wstrackerhost/watch.go`) reads
  `stores.json` (`watchStamps` through `Host.Stores`) and, per store,
  `storeStamp` reads `packed-refs` / `reftable/tables.list` and every loose
  ref file under `refs/heads/` and `refs/ledger/known/` every 2 s
  (`DefaultWatchInterval`; `internal/wstrackerhost/watch.go#L16`,
  `internal/wstrackerhost/watch.go#L70-L128`,
  `internal/wstrackerhost/registry.go#L146-L152`), about 10 read ops/s.

## Decisions

1. **Scope is the low-risk cuts below; the change-check skip and narrower
   wakes are out until a remeasure (user, (620) a).** "(620)" is the
   whiteboard item that put the scope to the user, with options a (this
   scope), b, and c below; later decisions citing it are lead calls inside
   that scope. In scope: a
   stat-checked parse cache (Decision 2), a listing-based ticket stamp
   (Decision 3), the reply registry off the board watch list (Decision 4),
   skipping unused snapshot reads (Decision 5), the git badge's resolved git
   path (Decision 6), and a stat-based tracker watch (Decision 7). Rejected:
   also dropping the pre-render change check in `pageCache.get` and waking a
   page only when a looked-up peer's shown fields change, in the same ticket
   (b); accepting the current level (c). Those two touch a deliberate rule
   (check before render, so no change slips in between) and are judged
   after this ticket's effect is measured.
2. **Unchanged small files are served from a parsed cache, re-read only
   when a stat shows a change.** Cover the session snapshot
   (`pageReader.readSnapshot`), the session record parts (`lookupRecord`;
   keep the raw bytes the fingerprint hashes), the reply registry
   (`loadReplyRegistry` and `mailboxPanel`'s direct `wsmailbox.LoadReplyStore`
   call), the snapshot read in `HashLiveStores`
   (`internal/wsdashboard/agent_sessions.go`, on every `ReadSession`), and the
   contacts registry and scope stores read by the change check and the
   render. A cache hit costs one stat; a changed
   name, size, or modification time re-reads. Writers replace these files
   atomically, so every write changes the modification time; the accepted
   miss is the read-storm ticket's (an edit keeping size and modification
   time identical within the filesystem's timestamp resolution). Entries of a
   session are dropped when its registration is removed, so memory does not
   grow with ended sessions. Correctness rules:
   - stat before reading, so a write landing between the stat and the read
     is re-read on the next check instead of caching new bytes under an old
     stamp;
   - compare stamps by equality, never "newer" (concurrent atomic replaces
     can move the modification time backwards);
   - never cache a failed read or parse; the existing retry behavior (for
     example `wsmailbox` Windows sharing-violation retries) is kept;
   - parsed values are shared across renders and goroutines, so callers
     treat them as read-only, or the cache hands out copies where a caller
     mutates.
   Rejected: a time-based cache (serves stale data
   for its lifetime); caching only bytes and re-parsing (the parse is a
   measurable share of the render CPU). (Lead technical call from the
   investigation, within (620) a.)
3. **Ticket stamps come from the ready/ and todo/ listings, not one stat per
   ticket file.** `boardWatchStamp` and the `statStamp` loop in
   `sessionStamp` stamp the ticket folders from their directory listings
   (entry names, sizes, modification times), as `ticketBoardPrint` already
   does for the nudge fingerprint. A watched path that is not a ticket file
   keeps its own stat. `wsdashboard` has no ticket knowledge, so the shape is
   the implementer's choice between a generic rule (a watched file whose
   parent directory is also watched is covered by that directory's listing)
   and a separate page-content list of directories to stamp by listing,
   filled by the ticket panel. On Linux `DirEntry.Info()` still costs an
   lstat per entry, so the saving is mostly on Windows, the measured
   target. Rejected: stamping only the folders' own modification
   time (misses an in-place edit of a ticket file). (Lead technical call,
   within (620) a.)
4. **The reply registry leaves the board memo's file stamp, not the page's
   refresh stamp.** The board memo key folds in only what the board shows
   from the registry, this session's `Contacts` and
   `len(Queues[replyID])`, never the whole entry (its heartbeat-driven
   `LastSeen` would rebuild the board on every refresh). So a rewrite for
   another session, or a heartbeat, does not rebuild the board. The page's
   refresh stamp (`sessionStamp`, checked on the 15 s refresh tick) keeps a
   signal for this session's unread count: the same print of `Contacts` and
   queue length, read through the Decision 2 cache. Mail to a reply-id does
   not nudge the recipient, so without it an idle recipient's unread count
   would wait for the 30 s index poll, slower than today. `wsdashboard`
   knows nothing about the registry and reads it today only through
   `ContactSource.ReadContacts` on live pages with contacts, while the print
   must reach the stamp for every page, live or not; the implementer picks
   between a small optional source seam (a per-registration registry print
   beside `ContactSource`) and widening the existing contacts call.
   Rejected: keeping the watch and throttling
   rebuilds (keeps the full ticket re-read, only less often). (Lead technical
   call, within (620) a.)
5. **A looked-up session's snapshot is read only where its content is
   used.** Call sites that use only the lookup's `ok` or the session's
   `HasBoard` (for example `internal/wsdashboard/pageview.go`, the linked
   agent and peer checks) do not trigger a snapshot read. (Lead technical
   call, within (620) a.)
6. **The git badge runner reuses the tracker host's resolved git path.**
   Move `gitExecutable` from `internal/wstrackerhost/runner.go` into
   `internal/wsgit` as an exported helper (both `wsdashboard` and
   `wstrackerhost` already import `wsgit`) and use it from
   `gitBadgeRunner.RunGit` and the tracker host. Keep its current semantics:
   resolution is cached per `PATH`+`PATHEXT` value (a process-wide cache
   pinned the first git in the fake-git `PATH` tests), a failed lookup is not
   cached, and the command then falls back to bare `git`. Rejected:
   duplicating the helper in `wsdashboard` (two copies of a cache whose keying
   was a bug fix). Rejected here: collapsing the badge's five git spawns into
   one `git status --porcelain=v2 --branch` (a larger change whose saving
   falls mostly outside the daemon's own counters; it can be its own
   ticket). (Lead technical call, within (620) a; helper placement
   auto-confirmed from the fact populator's gap.)
7. **The tracker store watch stats the loose refs instead of reading
   them.** `storeStamp` derives its stamp from the names, sizes, and
   modification times of the files it reads today (`packed-refs` or
   `reftable/tables.list`, and the loose refs under `refs/heads` and
   `refs/ledger/known`) instead of their content, for the loose refs only:
   `packed-refs` and `tables.list` are one file each and stay
   content-based. A loose ref is always 41 bytes, so its name and
   modification time carry the change; the accepted miss is two updates of
   one ref within one timestamp granule with a watch tick between them,
   caught by the next change. `stores.json` stays read through
   `watchStamps` as today. The watch itself
   stays: it is the only path for external writers (prior decision below).
   (Lead technical call, within (620) a.)
8. **`/debug/pprof/streams` gains hit and miss counters for the Decision 2
   cache.** One pair per cached file kind, snake_case like the existing
   counters, reached through the same optional stats seams the read-storm
   ticket added, so the remeasure after this ticket can attribute what is
   left. No new route or env knob. (Lead technical call, within (620) a.)

## Constraints

- Server-side Go only (`internal/wsdashboard`, `internal/mcp` dashboard
  source, `internal/wstrackerhost`); no frontend or shipped-text change.
- Every page-visible change must still reach the page as fast as today: a
  cached file whose content changed is re-read on the next check, and the
  pre-render change check stays (Decision 1).
- Never run go build/test/vet on the lead machine; run them remotely:
  `~/.local/bin/ws-rgo -C <checkout>/agents-plugin-tool -m 30 -- test
  <packages> -count=1 -timeout 30m`.
- Never touch processes or ports 29417 / 45743 (the user's daemons) from
  tests or probes; a dev daemon uses an isolated WS_CACHE_HOME /
  WS_CONFIG_HOME and its own port.
- Out of scope: the pre-render change-check skip and narrower peer wakes
  (Decision 1); collapsing the git badge spawns (Decision 6); bounding or
  pruning ring directories; the 30 s refresh's registration re-reads.
- `ai-docs/manuals/ws-mcp.md` applies to `internal/mcp/` edits; update its
  dashboard paragraph if a behavior it describes changes.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for
  agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for
  agents-plugin-tool/internal/mcp/)

## Prior Decisions

- e8916a25 (2026-11-10, commit): "Decision 5 (A): the 30 s refresh plus each nudge built every panel (ticket list parse) only to hash it; metadata gives the same change signal." — bearing: supports
- 261110-bug-ws-dashboard-daemon-read-storm (2026-11-10, Result): "Decision 4: git resolved once per `PATH`+`PATHEXT` (keyed after the fake-git `PATH` tests showed a process-wide cache pins the first git); a failed lookup is not cached and falls back to bare `git`." — bearing: constrains
- 261110-bug-ws-dashboard-daemon-read-storm (2026-11-10, Result): "Decision 7: `nudges`, `session_reads_changed`/`_unchanged`, `tracker_reloads`/... on `/debug/pprof/streams` via optional `wsdashboard.CounterSource`." — bearing: supports
- b0e680e2 (2026-11-10, commit): "The memo lives in mcp, where the render inputs are known, rather than in wsdashboard's page cache... A stamp is taken over the previous render's watch list before rendering and only trusted when the new render named the same files" — bearing: constrains
- b0e680e2 (2026-11-10, commit): "Rejected: a tracker-write nudge (no external writer could use it), an fsnotify dependency (unreliable over 9P), caching images by requested path (aliases and symlink retargets)." — bearing: supports
- 261110-bug-ws-dashboard-render-and-frontend-load-accumulation (2026-11-10, Decisions): "tracker store watch (2 s, wstrackerhost/watch.go): keep. It is the only path for external writers" — bearing: constrains
- dc8d5566 (2026-11-09, commit): "Reverses two choices of the original live-update design: the broadcast wake ... and the stateless daemon (the cache starts empty and derives only from disk, so a replacement daemon renders the same pages)." — bearing: constrains
- 261007-bug-session-record-cross-process-write-race (2026-11-10, Result): "`wsstate.atomicReplaceFile` is renamed to the exported `wsstate.AtomicReplaceFile`... `writeFileAtomic` now replaces part files through it." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | internal/wsdashboard/stream_cache.go, internal/wsdashboard/pages.go, internal/wsdashboard/pageview.go, internal/wsdashboard/gitbadge.go, internal/mcp/dashboard_sessions.go, internal/mcp/dashboard_page.go, internal/mcp/whiteboard_panels.go, internal/wstrackerhost/runner.go, internal/wstrackerhost/watch.go |
| scope.surface | cross-module | spans wsdashboard, mcp, and wstrackerhost; Decision 6 moves gitExecutable into internal/wsgit |
| scope.new_public_symbol | yes | Decision 6 moves gitExecutable into internal/wsgit as an exported helper; cache types stay unexported |
| scope.new_type_contract | unknown | counters reuse the existing wsdashboard.CounterSource seam; cache types are internal; Decision 4 may add a small optional registry-print source seam beside ContactSource |
| scope.test_surface | existing | internal/wsdashboard/stream_cache_test.go, pages_test.go, gitbadge_test.go, internal/mcp/dashboard_page_test.go, dashboard_sessions_test.go, whiteboard_panels_test.go, internal/wstrackerhost/watch_test.go, runner_test.go |
| complexity.reuse_points | confirmed | ticketBoardPrint in internal/mcp/dashboard_sessions.go#L557-L579, gitExecutable in internal/wstrackerhost/runner.go, CounterSource in internal/wsdashboard/stream_stats.go#L36-L38 |
| complexity.side_effect_risk | moderate | new process-lifetime caches of files other processes rewrite, with per-session eviction tied to registration removal |
| risk.correctness | high | a stale cache hit or a board memo key missing the unread count would hide a page-visible change the pre-render check must still catch |
| risk.fit | moderate | board memo key change in mcp and a possible git-resolver relocation across packages must respect the memo-in-mcp and per-PATH resolution decisions |
| risk.test | moderate | mtime-resolution misses and Windows I/O behavior are hard to exercise; tests must run remotely through ws-rgo |
| risk.security_or_contract | low | no route, wire, or env change; only new counter keys under the existing debug streams route |

## Phases

### Phase 1: cut re-reads of unchanged files

Implement Decisions 2-8.

Verification:

- Unit tests per cache and stamp: an unchanged file is not re-read (counted
  through a read seam or the Decision 8 counters); a rewritten file with new
  content is re-read on the next check; a removed session's entries are
  dropped.
- The board is not rebuilt when the reply registry changes only another
  session's entry, and the unread count still updates when this session's
  entry changes.
- A ticket file edited in place (same name, new size or mtime) changes the
  listing-based stamp.
- The page's unread print changes when this session's unread queue length
  or contacts change, and not when only an unrelated session's entry (one
  that is not a contact) or this session's `LastSeen` changes; the existing
  contacts stamp, which rightly follows a contact's own entry, keeps its
  behavior (Decision 4).
- A lookup that uses only `ok` or `HasBoard` does not read the snapshot
  (Decision 5).
- The git badge runner spawns the `wsgit`-resolved git path; a changed
  `PATH` resolves again and a failed lookup falls back to bare `git`
  (Decision 6; the existing runner tests move with the helper).
- `storeStamp` changes when a loose ref is replaced and when `packed-refs`
  content changes, without reading loose ref content (Decision 7).
- The Decision 8 counter keys appear on `/debug/pprof/streams`.
- The existing dashboard, mcp dashboard-source, and wstrackerhost suites
  pass remotely through `ws-rgo`.

Lead follow-up after merge and release (not a phase gate): remeasure the
Windows daemon (the restart needs the user's approval) and decide on the
Decision 1 follow-ups.

### Result (ceaeeb66f) - 2026-11-10

Landed in ffa20da9c (Decisions 6, 7), 89eb4b898 (wsdashboard side of 2, 3, 4, 5, 8),
c1afbe86c (mcp source side of 2, 3, 4, 8; ws-mcp.md), and ceaeeb66f (review fixes).

- Decision 2: `wsdashboard.FileCache[T]` (exported, generic) is a stat-checked
  parse cache: stat before load, size+mtime equality, failures never cached, a
  file that does not stat goes to its loader uncached and counts as neither hit
  nor miss. Added beyond the ticket: a file modified within 2 s of the read is
  returned but not cached (racy-git guard), so the accepted same-granule miss
  cannot occur. `wsdashboard.SnapshotCache` caches the parsed snapshot and the
  canonical document `HashLiveStores` folds; the daemon holds one (pages,
  stamps, agent re-reads) and the mcp source another (its `HashLiveStores`).
  The mcp source caches record parts (bytes kept for the fingerprint) through
  a new `loadRecordWith` reader seam, the reply registry, and scope stores.
  Entries drop on session removal (`Daemon.sessionChanged`, `ForgetSession`).
- Decision 3: `wsdashboard.StampWatched` uses the generic rule: a watched
  directory with watched files directly inside is stamped by its listing and
  covers those files; `sessionStamp` and `boardWatchStamp` both use it. A
  symlinked entry is followed with `os.Stat`.
- Decision 4: the reply registry left the board's watch list; the board memo
  key and the ReadSession board print fold `replyPrint` (this session's
  `Contacts` and `len(Queues[replyID])`). New optional seam
  `wsdashboard.MailPrintSource` (forwarded by `agentSessions`) feeds the same
  print into every page's refresh stamp.
- Decision 5: page lookups are a `sessionLookup` (index-only `get`, lazy
  `snap`); `agentTree` and `conversation` read no snapshot.
- Decision 6: `wsgit.Executable()`; the badge's `badgeGitCommand` and the
  tracker host use it; the resolver test moved to wsgit.
- Decision 7: loose refs are stamped by name, size, and mtime;
  `packed-refs`/`tables.list` stay content-based.
- Decision 8: `/debug/pprof/streams` serves `snapshot_cache_*` (daemon) and
  `record_cache_*`, `reply_registry_cache_*`, `scope_store_cache_*`,
  `source_snapshot_cache_*` (source), each `_hits`/`_misses`.

Verification (remote, ws-rgo): `go test ./internal/mcp/ ./internal/wsdashboard/
./internal/wstrackerhost/ ./internal/wsgit/ ./cmd/... -count=1` ok; `go vet` on
the four packages clean; `go test -race` on the dashboard-focused subsets of mcp
and wsdashboard ok; after the review fixes, mcp, wsdashboard, and wstrackerhost
re-run ok. Lite review: two Important test-integrity findings fixed (badge
test now checks `Args[0]`; loose-ref test proves content is not read).

Open (minor, from review): `ForgetSession` evicts record-cache entries by the
keys `byReply` holds at forget time; a key pruned and rescanned out of
`byReply` before its registration leaves the index keeps its cached parts.
