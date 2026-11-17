# Perf: board readers must not wait on the whiteboard writer's flock; cut per-render board walks

## Context

The owner's PC runs AhnLab V3, so every file touch costs 10-100x. The owner
measured in DevTools on 2026-11-17:
- the session frame took 5.9 s;
- topic requests took 0.4-2.3 s;
- the WS events handshake took up to 6 s.

A read-only diagnosis captured goroutine samples during a 5.9 s handshake.
Artifacts are in `D:/Temp/daemon-swap-1117f/`: `gd/g1-g7`, `cap/cpu30.pprof`
and `poll.txt`. The diagnosis found this chain on develop:

1. **The writer holds the board flock through two full renders.**
   - A lead MCP tool call that dirties the whiteboard runs
     `refreshWhiteboardAfter`. See `mcp/server_dispatch.go:142` and
     `mcp/whiteboard_tools.go:464-483`.
   - That calls `mutateRecordParts`, then
     `FolderStore.UpdateAndPublishObserved` (`wswhiteboard/folders.go:194`).
   - It holds the board's cross-process flock while `renderWhiteboardPages`
     (`whiteboard_tools.go:509-585`) runs. That function does the following:
     - one `git rev-parse` subprocess (`registerWhiteboardExclude`, :757);
     - goldmark plus a full template render of the 64-topic board;
     - `whiteboardPublished` (:595-602), which does `os.ReadFile` on the
       2.43 MB `index.html` to compare (1.78 s CPU per 30 s, nearly all in
       CreateFile under V3);
     - a second full render and a 2.4 MB write.
2. **Readers take the same flock even on a cache hit.**
   - `FolderCache.openRevision` (`wswhiteboard/folders_cache.go:134-155`)
     takes the per-board `slot.mu` first.
   - It then calls `withGuard` (`folders.go:232-264`), which does `MkdirAll`
     and a flock `TryLockContext`. The lock polls every 10 ms.
3. **One blocked reader blocks all the others.**
   - The blocked reader holds `slot.mu`, so every other reader of that board
     queues behind it: the frame document, topic, stream renders and the WS
     first render (`sse.go:534` runs before the 101 at `sse.go:704-708`).
   - `poll.txt` shows a frame that started at a board commit taking 6.43 s.
     The same frame normally takes 0.06-0.6 s.
4. **Every board render also walks the whole board folder.**
   - `watchWhiteboardFiles` (`mcp/whiteboard_store.go:23-30`, called from
     `mcp/dashboard_page.go:281`) runs WalkDir over the whole 335-file /
     7.4 MB board folder on every render. It has no I/O counter.
   - `boardWatchStamp` (`dashboard_page.go:254`,
     `wsdashboard/stream_cache.go:357-377`) then stats about 405 watched
     paths, with a 1 s TTL.

## Outcome

1. **(a) A FolderCache hit is served without the flock and without
   MkdirAll.**
   - Validate the cached revision by its stamp alone: `pending.json` is
     absent, and the meta/committed identity is unchanged. Use whatever
     cheap identity the cache already records, such as size, mtime or
     generation.
   - On a hit, return the cached state.
   - A miss still takes the flock and reloads as today.
   - **Prove consistency in the commit's `## AI Context`.** A reader that
     races a writer must still get a consistent committed state. Base the
     argument on the commit protocol: commits replace both roots before
     removing pending. Cite the lines. If the protocol does not give that
     guarantee, stop and report instead of weakening it.
   - Update the FolderCache doc comment that says "each hit still acquires
     the writer's flock".
   - Do not hold `slot.mu` across a flock wait. If a miss must lock, readers
     of a still-valid cached revision must not queue behind it.
2. **(b) Build the board watch list from the cached State instead of
   WalkDir.** Alternatively, memoize the walk keyed by the FolderCache
   revision. Either way, an unchanged board does no ReadDir per render.
3. **(c) Stamp board files through `meta.json` and `committed.json` only.**
   Every commit changes these files. Keep per-path stats for external
   watches such as tickets and images.
4. **Writer-path trims, both T0.**
   - `whiteboardPublished`: compare against an in-memory hash plus stat of
     the last written `index.html`. Do not read the 2.4 MB file back.
     Read it back only when no memo exists or the stat changed.
   - `registerWhiteboardExclude`: memoize the `git rev-parse` result per
     root.
   - Do not move rendering outside the lock and do not change how or when
     the writer writes. That is a write-strategy change awaiting the owner's
     approval.

## Constraints

- **Tier.** Nothing on disk changes, so this is expected to be T0. Cite
  source lines. The change to the lock protocol (readers stop taking the
  flock on a hit) affects only in-process readers. State why an older binary
  that writes the same board stays safe: it still writes under the flock and
  follows the same commit order.
- **Owner approval basis.** (1297) approves read caches and skip-if-unchanged
  writes. Any other write-strategy change: stop and report.
- **Tests.** Add Go tests for:
  - a cache hit takes no flock and does no MkdirAll (use a seam or the
    `file_io_site` counters);
  - a reader during a held writer flock returns the last committed state
    without waiting;
  - a commit invalidates the hit;
  - `pending.json` present forces the locked path;
  - an unchanged board render does zero ReadDir for the watch list;
  - an unchanged publish does not read `index.html`.
- **Coordination: do NOT edit these files.** Other workers own them.
  - `wswhiteboard/folders_memo.go` (hot-stat worker);
  - `mcp/whiteboard_nudges.go`, `mcp/session_layout.go`,
    `wslive/observed_cache.go` (hot-stat worker);
  - `wslive/*` (observed-fold worker);
  - tracker view code (tracker worker);
  - image-serving code (image worker);
  - dashboard JS/CSS.
- **No local runs on this PC.** No `go test`, Go test exes, ws/ws-mcp exes,
  launcher or daemon (antivirus rule).
- **Go runs only on w5090:**
  `D:/sys_lock.sh remote w5090 run board-read 30 --sync 'D:/Workspace/Repos/.ws-worktrees/devenv/unfixable-vitalize-barterer' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go vet ./... && GOOS=windows go vet ./internal/... && go test -count=1 ./internal/wswhiteboard ./internal/mcp ./internal/wsdashboard'`
  - Redirect the output to a file under `D:/Temp/board-read/`.
  - Wait for the run to finish before ending your turn. Do not end your
    turn while a remote run is pending.
  - Accepted reds: only TestTopicStateBadgeResearchingMetadata and
    TestTopicStateBadgeClockAndReconciliation.
- **Process.** Simplest complete change. R1 review (focus on lock and
  consistency correctness), then a fixes-only R2.
- **Commit.** Commit on `impl/develop/board-read-unlocked` with your own
  model's Co-Authored-By trailer, and state the tier. Do not push.
- **Temp files** go only under `D:/Temp/board-read/`.
- **Urgency.** The owner's critical path: every page and topic waits on
  this.

## Report

- (a) the hit validation and the consistency argument (file:line);
- (b) and (c), plus the writer trims;
- per render: ReadDir, stat and flock attempts, before → after;
- the tests;
- the commit and files;
- the w5090 result;
- the tier;
- anything left alone.
