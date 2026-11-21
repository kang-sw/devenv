---
title: "화이트보드 2PC 폴더 저장소를 보드별 SQLite로 전환"
parent: 261119-epic-ws-daemon-sqlite-storage
blocked-by: 261119-feat-ws-whiteboard-retire-embed-snapshot
related:
  261119-feat-ws-whiteboard-retire-embed-snapshot: "sibling child; removes the per-update embed render this store would otherwise carry (see (1815))"
  261116-feat-ws-whiteboard-retire-disk-pages: "Phase 2 prerequisite; retires derived pages, their publisher and refresh, owns the full: status changes, and keeps the first-open cleanup sweep"
  261117-research-ws-daemon-file-touch-map: "storage audit background (whiteboard rows)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a5a67f154108c7a6
sage-review-completeness-reviewed: a5a67f154108c7a6
---

# Whiteboard 2PC folder store -> one SQLite database per board

## Background

The whiteboard's authoritative store is a repo-rooted, git-excluded folder per
session board, `ai-docs/whiteboards/<reply-id>/`, written through a two-phase
intent protocol (`wswhiteboard/folders.go`). The epic
`261119-epic-ws-daemon-sqlite-storage` moves it to one SQLite file per board.
The owner chose this store as the epic's pilot child (2026-11-19, 23:06), so
it also sets the pattern later children follow.

Go paths below are relative to `agents-plugin-tool/internal/`. Line numbers
were read on `develop` at `9f43a8f00` and `7c26b7b0a`. They drift: re-check
each one against the implementation base before editing.

### Current store inventory

Board folder `ai-docs/whiteboards/<reply-id>/`:

| File | Role | Source |
|---|---|---|
| `meta.json` | Session metadata: schema version, generation, `next_topic`, `State` minus topics/items/labels/close results, topic refs (dir, key, `log_bytes`), item order | `wswhiteboard/folders.go:49-56` |
| `pending.json` | Commit intent: exact panel bytes, log appends with offsets, topic metadata, next session metadata; replayed before any read | `folders.go:78-91`, `:400-408`, `:957-1045` |
| `committed.json` | Receipt (`schema_version`, `generation`) that blocks a legacy fallback when `meta.json` is missing | `folders.go:93-96`, `:450-455` |
| `.lock` | Cross-process flock for every open and update (30 s queue budget) | `folders.go:225-259` |
| `tN/meta.json` | Topic metadata: doc minus entries, items, label, closed flag, close result | `folders.go:58-65` |
| `tN/log.jsonl` | Append-only entry log; panel entries carry no text | `folders.go:833-844` |
| `tN/panels/eN.json` | Immutable panel entry body per version | `folders.go:834-838`, `:977-994` |
| `t2147483647/panels/e1.json`, `e2.json` | Reserved alternating board-revision receipt slots | `folders.go:98-141`, `:873-893` |
| `t2147483647/log.jsonl` | Load-floor record appended by `whiteboard.load` | `wswhiteboard/folders_load_floor.go:11-75` |

Older input: the legacy session-store part (`whiteboard.json` under the session
key store) is converted into the folder on first open (`folders.go:456-489`).

Derived, not authoritative: `index.html` and `index.stamp.js` in the board
folder, the hashed mermaid asset beside a page that uses it, and the embed
snapshot `ai-docs/whiteboards/<reply-id>-<hash>.html` or a host embed folder
(`mcp/whiteboard_tools.go:606-692`, `:756-769`). The embed snapshot is retired
by `261119-feat-ws-whiteboard-retire-embed-snapshot`.

Readers and writers of the folder in this checkout:

- Writes: `whiteboard.update` (`mcp/session_state.go:649-675` via
  `mcp/whiteboard_tools.go:276-306`); dashboard answer revision
  (`wswhiteboard/dashboard_answer_revision.go:21-65`); board notice nudges
  (`wswhiteboard/board_notice.go:250-315`); `whiteboard.load` replace
  (`wswhiteboard/folders_archive.go:241-294`, called from
  `mcp/whiteboard_tools.go:455-500`); load-time content and close-result
  migration (`folders.go:435-447`).
- Reads: `FolderCache` (`wswhiteboard/folders_cache.go:84-226`; a hit is three
  stats: `pending.json`, `meta.json`, `committed.json`), every MCP open through
  `openWhiteboardCached` (`mcp/whiteboard_store.go:54-78`), `whiteboard.query`
  (`whiteboard_tools.go:428`), the served dashboard page and its stat watch of
  the two root files (`mcp/dashboard_page.go:292-293`,
  `mcp/whiteboard_store.go:27-30`), nudge targeting by root-file stats
  (`mcp/whiteboard_nudges.go:286-293`, `:339-345`), zip export for the
  dashboard download (`mcp/dashboard_page.go:383-391` ->
  `folders_archive.go:51-124`).
- History queries (`whiteboard.query` `entries`, `since`, `last`, `kind`,
  `panel: "eN"`, `entry: "eN"`) and lazy topic history
  (`ReadTopicHistory`, `mcp/dashboard_page.go:400`) are served from the loaded
  `State`, which already holds every entry and panel body
  (`folders.go:563-651`, `wswhiteboard/folders_memo.go:88-128`).
- Non-Go readers: none found. `claude-mod`, `desktop-electron`, and the Pi
  extension sources do not reference `meta.json`, `committed.json`, or the
  board folder. The browser-only derived-page readers retire in
  `261116-feat-ws-whiteboard-retire-disk-pages` before Phase 2.
- The derived-page ticket also replaces the daily orphan GC with its retained
  off-response-path, first-open cleanup sweep; neither the old GC nor the
  retired render refresh enters Phase 2's routing inventory.

### Before: file operations per `whiteboard.update` (derived from code)

Scenario: a top-level lead's update appends one log entry to an existing topic
on a board with `T` topics. No panel, no topic-metadata change, no
board-revision receipt change, no host embed folder, no mermaid, embed
retention already full, and topic files older than the 2 s memo window
(`folders_memo.go:36`, `:59-82`). Unit: one API-level filesystem call
(`ReadFile`, `Stat`, `MkdirAll`, open/create, write, fsync, rename, remove,
flock); `Close` is not counted. A syscall tracer sees more, for example
open+fstat+read+close per `ReadFile`.

| Step | Source | Ops |
|---|---|---|
| Unlocked guard: read `meta`, `pending` (absent), `committed`; stat each topic `meta.json` | `folders.go:327-398` | 3 + T |
| `MkdirAll` board folder; open `.lock` and flock | `folders.go:235-253` | 3 |
| Guard again under the lock | `folders.go:255` | 3 + T |
| Load: read `pending` (absent), `meta`; stat each topic `meta.json` and `log.jsonl`; reread the log the previous update grew; read both revision slots and the load-floor log | `folders.go:400-434`, `:563-651`, `:117-141`; `folders_load_floor.go:25-47` | 6 + 2T |
| Plan: stat each topic `meta.json` again; read both revision slots again (any board whose `MetadataRevision` is nonzero) | `folders.go:846-858`, `:873-874` | 2 + T |
| Commit intent: atomic `pending.json` (`MkdirAll`, 2 directory syncs, create temp, write, fsync, rename, directory sync, remove temp) | `folders.go:943`, `:1116-1155` | 12 |
| Replay: read `meta`; append to `tN/log.jsonl` (`MkdirAll`, open, stat, seek, two writes each followed by fsync, directory sync) | `folders.go:957-1005`, `:1063-1114` | 11 |
| Replay: atomic `meta.json` and atomic `committed.json` | `folders.go:1024-1032` | 24 |
| Replay: remove `pending.json`, sync the folder | `folders.go:1038-1043` | 3 |
| Unlock | `folders.go:254` | 1 |
| **Store subtotal** |  | **68 + 5T** |
| Publish embed snapshot, `index.html` (+ stat to remember it), `index.stamp.js` (`MkdirAll`, create temp, chmod, write, rename each); evict the oldest embed | `whiteboard_tools.go:785-812`, `:843-870` | 17 |
| **Total** |  | **85 + 5T** (135 at T = 10) |

Of these, 16 are fsyncs on macOS and Linux: 5 file fsyncs and 11 directory
fsyncs. On Windows the directory syncs are skipped (`folders.go:1157-1161`),
which leaves 63 + 5T ops and 5 file fsyncs; reads and removals there also
retry sharing violations (`wsstate/replace_file_windows.go:40-66`).

Additions on top of the scenario:

- +8 ops (one empty append) per topic that has never had an entry. Plan
  appends to every topic with `log_bytes == 0` (`folders.go:841-843`).
- +14 ops per changed topic `meta.json` (a body, item, label, or close edit).
- +4 to +5 ops per new panel file. The atomic write sits one directory deeper,
  which adds one more directory sync.
- Revision-slot clearing with a receipt change (`folders.go:912-942`).

Not counted here:

- Session-key-store lock and record load (`mcp/session_state.go:627-646`),
  owned by the epic's MCP session key store child.
- Render-side reads: git exclude registration and the ticket panel listing
  (`whiteboard_tools.go:620`, `mcp/whiteboard_panels.go:35-56`).
- Knob enable, the conversation-ring projection
  (`mcp/whiteboard_effects.go:15-39`), and dashboard notification.
- The next dashboard read. A changed root misses `FolderCache` and repeats
  guard + lock + load, about 15 + 4T ops.

This count is derived from source and has not been checked with a tracer.
Phase 1 replaces it with the measured value.

## Decisions

- **Storage shape, `(1325)=a` (owner, 2026-11-19 21:56):** one SQLite file per
  board (per session) with a small `cache_size`. Its "opened lazily and closed
  when idle" clause is superseded by `(1347)=c` below.
  - Rejected: one shared DB with a `board_id` column, because boards share
    nothing and a per-board file keeps zip export/load and delete trivial.
  - Rejected: memory state plus a per-board append journal like `wsmailbox`,
    because it would leave the whiteboard on a different engine from the rest
    of the epic.
- **Board DB handle lifetime, `(1347)=c` (owner, 2026-11-19 23:10):** a board's
  DB handle, and the in-memory board with it, stays open and is closed only
  after a long idle (about 30 minutes). The next access reopens it. This
  supersedes the lazy-open/short-idle-close part of `(1325)=a`; the
  one-file-per-board and small `cache_size` parts stand. Each reopen still
  breaks performance.md rule 4 once; the owner accepted that by choosing c.
  - Rejected: held for the whole session lifetime, because handles accumulate
    for abandoned boards.
  - Rejected: short idle close per `(1325)=a`, because each reopen rescans the
    DB, `-wal`, and `-shm`, which conflicts with performance.md rule 4.
- **Pilot and gate (owner, 2026-11-19 23:06, `(1346)`):** this child runs first
  as the epic's pilot. There is no Windows AhnLab V3 benchmark for now (the
  owner is away from that PC). Owner: "whatever it is, the files must avoid
  the currently known performance-degrading factors." So
  `ai-docs/manuals/performance.md` rules 1-5 are hard design constraints here.
  The numbers below are that manual's own rule numbers; Phase 3's gap list
  cites them:
  1. Rule 1, memory first: reads come from memory only. The DB serves recovery
     and history.
  2. Rule 2, delta appends: one change is one transaction, which is one WAL
     append. An unchanged state writes nothing.
  3. Rule 3, whole-file retention: one DB per board holds the board's recovery
     state, so recovery reads one board DB (the DB plus its `-wal`; there is
     no `-shm` under `(1807)`), and deleting a board deletes one folder
     (`(1325)=a`, `(1808)`). There are no per-record files and no
     in-place pruning of the live file.
  4. Rule 4, held handles: no per-call open, lock, or `stat`.
  5. Rule 5, I/O-free interactive paths: `PRAGMA synchronous=NORMAL` in WAL
     mode, so there is no fsync per commit and none on the response path.
     Checkpoints are rare and run off the interactive path. `whiteboard.update`
     latency waits on nothing beyond that single WAL append.
- **Measurement (owner, 2026-11-19 23:06):** before/after file-operation counts
  per `whiteboard.update` on macOS, using a tracer (`fs_usage`/`dtruss`) or
  Go-level counting (`wsstate.FileIOCounters`,
  `wsstate/file_io_counts.go:12-48`; it does not see direct `os.*` calls,
  flock, or SQLite's own I/O, so it must be extended or cross-checked by the
  tracer). Windows measurement is deferred and is recorded as deferred in each
  phase Result. It is not silently skipped.
- **SQLite precedent (epic Cross-Child Decisions):** build on
  `wsstore/store.go` (`modernc.org/sqlite`, `journal_mode=WAL`,
  `SetMaxOpenConns(1)`, `busy_timeout`, `SQLITE_BUSY`/`SQLITE_LOCKED` retry per
  `6539b0c01`). Rule 5 above (`synchronous=NORMAL`) overrides the precedent's
  default `synchronous` setting (`wsstore/store.go:209-224` sets none).
- **Compatibility tier T3 with a minor bump, `(1805)=a` (owner, direct,
  2026-11-19 23:22):** the one-way import adds a sentinel `pending.json` with
  `schema_version: 3` to the legacy board folder. An older binary refuses that
  board before it can read or write it.
  - Rejected: b, close-time export of the legacy layout plus an open-time merge
    (T2). It writes legacy files on every close and needs a merge rule for
    boards where both sides diverged.
  - Rejected: c, rewriting `committed.json`/`meta.json` with schema 3. It is T3
    like a, but it modifies legacy data.
- **One-way migration, medium compatibility effort (owner, direct, 2026-11-19
  23:22):** where writing back to the old format for older versions costs real
  work, migrate one way instead. Rejected under this directive:
  - every legacy write-back: close-time or shutdown export, export stamps,
    re-import or merge on open;
  - stamp or marker files kept up to date for old readers;
  - stat-based change detection kept alive for old processes.

The decisions below settled the queue on 2026-11-19 with the lead's
recommendations as assumed answers. Each line reads "owner-assumed": the owner
confirms them before this work merges to `develop`.

- **Tier reasoning per `release-compatibility.md`** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning). T3 is the highest affected
  path. Release Rule 2 requires a minor bump and explicit owner approval; the
  owner approved on 2026-11-19 at 23:22. That approval covers the `(1805)=a`
  sentinel. `(1806)`'s direct-CLI refusal is a second T3 path, and `(1844)=a`
  is the explicit owner approval Release Rule 2 requires for it.
  1. Old folder, new binary: the first open imports through the retained
     `FolderStore.Open` (`folders.go:143-159`, `:400-490`). That covers pending
     replay, the schema guard, and v1 conversion. No T4.
  2. Old binary meets a migrated board: this happens when an older daemon
     starts first (`wsdashboard/ensure.go:210-217` only stops a newer daemon
     from being replaced). Its guard reads `pending.json` and refuses
     `schema_version` 3 as "newer schema_version 3 (supported 2)"
     (`folders.go:304-321`). The guard runs before the lock and again under it,
     before any replay or write (`folders.go:225-259`, `:327-340`).
     - Every old entry point passes that guard first. `FolderCache` misses when
       `pending.json` exists (`folders_cache.go:84-94`) and then runs the guard.
       `ExportArchive` and the `whiteboard.load` replace take the guarded lock
       (`folders_archive.go:51-60`, `:241-246`). Nudge `Notify` opens through
       the cache (`board_notice.go:263`). MCP opens surface the error
       (`mcp/whiteboard_store.go:63-66`).
     - The old binary writes nothing. Its daily GC does not match
       `pending.json` or `.board.sqlite` (`whiteboard_tools.go:51-53`).
     - This is the T3 read boundary of scenario row 11
       (`release-compatibility.md:115`). Previous-minor plugins are already
       refused by the connection guard (rows 4 and 4a).
  3. New binary after an old binary's refusal: nothing changed, so it reopens
     the DB as before.
  4. Zips: T0 in both directions under `(1813)`.

  Release Rule 2 requires showing that the old side actually refuses before any
  damaging write. Phase 2's compat-old verification shows it.
- **One-way import, `(1804)`** (owner-assumed 2026-11-19 under (1802)=b; owner
  confirms in the morning):
  - On first start the new binary imports the legacy board folder into the DB
    through the existing `FolderStore.Open`, commits it, and then adds the
    `(1805)=a` sentinel.
  - A DB found without the sentinel gets it on open, after the generation
    check of `(1825)`.
  - After the import commit there is no write to the legacy folder, apart
    from what `Open` itself replays or migrates: a leftover `pending.json`,
    content and close-result migrations, and the `.lock` file
    (`folders.go:143-159`, `:400-447`).
  - Rejected: dual-write on every update, because it keeps the per-update 2PC
    cost.
  - Rejected: legacy write-back of any kind (owner directive, 23:22).
  - Rejected: a new read-only import path, because it duplicates replay
    logic and an unreplayed intent would be lost from the import.
- **The daemon-only premise confirms `(1806)`, `(1844)=a` (owner, 2026-11-20
  17:31; note 17:30).** "all MCP functions presuppose the daemon; the no-daemon
  scenario is meaningless." SQLite decision `(1806)` stands: once Phase 2
  makes the DB authoritative, non-owner processes refuse board calls. Rejected:
  retaining a supported no-daemon board mode.
- **Raw-binary updates keep committing only before Phase 2, `(1852)=a` (owner,
  2026-11-20 17:44).** After
  `261116-feat-ws-whiteboard-retire-disk-pages` lands and before this ticket's
  Phase 2, direct `ws-mcp serve --stdio` and `ws-mcp call whiteboard.update`
  still use their in-process server (`cmd/ws-mcp/main.go:19-77`,
  `cmd/ws-mcp/hosted_cli.go:508-539`), commit the update, and report only that
  the page is unavailable on `full:`, with no start-daemon hint. Phase 2 then
  applies `(1806)`. Rejected: pulling the refusal into the page-retirement
  ticket, which would make that ticket T3 and require a minor bump.
- **Direct CLI never co-opens the board DB, `(1806)`** (owner-confirmed by
  `(1844)=a` on 2026-11-20):
  - A direct `ws-mcp` CLI or `serve` process refuses board calls with an
    error naming the daemon as the board owner.
  - The refusal keys on an explicit board-owner option: a `BoardOwner bool`
    field on the mcp server options, next to `ServerOptions.Live`. Only the
    daemon sets it: the proxy host, the dashboard source, and the daemon's
    hosted one-shot CLI path (`cmd/ws-mcp/hosted_cli.go:74-80`; the implementer
    confirms that path). Every process without it refuses board calls BEFORE it
    opens the DB. The refusal does not rely on `SQLITE_BUSY`. The refusal text
    is the implementer's, under `shipped-surface-boundary.md`.
  - `ServerOptions.Live` stays ruled out as the owner signal: it is nil for a
    connection accepted before the dashboard binds (`cmd/ws-mcp/proxy.go:101-103`).
  - Rejected: routing board calls to the daemon. No proxy exists in
    `cmd/ws-mcp`, so it would be new work, and 0.50+ launchers always go
    through the daemon.
  - Rejected: concurrent WAL access from a second process, because it brings
    back cross-process locking and per-call revalidation (rules 1 and 4).
  - Rejected: detecting the second process by `SQLITE_BUSY` alone. A direct
    process that starts first would take the exclusive lock (`(1807)`) and lock
    the daemon out of its own board.
- **Sentinel write failure does not block the open, `(1828)`** (technical,
  lead ruling 2026-11-19):
  - When the sentinel write fails after the import commit (read-only folder,
    I/O error), the DB still opens.
  - The sentinel is retried on the next open, and the failure is logged. The
    retry runs inside the same locked step as the generation check of
    `(1825)`, which closes the window a missing sentinel leaves open.
  - Rejected: refusing to open, because it strands a board whose data is
    already safely in the DB.
- **The import holds the legacy lock through the sentinel and records the
  imported generation, `(1825)`** (technical, lead ruling 2026-11-19; amends
  `(1828)`):
  - The import holds the legacy folder's `.lock` from the import load through
    the sentinel write, so no legacy writer can append between the read and the
    sentinel.
  - The DB records the imported legacy generation.
  - An open that finds the DB but no sentinel (a `(1828)` failed write, or an
    older daemon that ran meanwhile) takes the folder's `.lock` and, under it:
    1. reads `committed.json` (`folders.go:93-96`) and any schema-2
       `pending.json`, which is an older daemon's unreplayed commit;
    2. logs a divergence (board, both generations) if either is ahead of the
       recorded imported generation. For a pending intent the log names the
       intent that the sentinel overwrites;
    3. writes the sentinel (the `(1828)` retry), and only then releases the
       lock.
  - The sentinel replaces `pending.json`. Replaying a schema-2 intent would
    remove that file (`folders.go:1038-1040`), the same path the sentinel
    takes, so an unreplayed intent is not replayed and the log is its only
    trace. The DB stays the board, per the owner's one-way directive.
  - Rejected: re-importing the diverged folder, because it would overwrite
    DB-side updates.
  - Rejected: refusing to open, for the reason `(1828)` gives.
- **A missing or corrupt DB under a sentinel, or a corrupt DB without one, is
  an explicit error, `(1826)`** (technical, lead ruling 2026-11-19):
  - When `.board.sqlite` is missing or corrupt but the folder has its sentinel,
    the open returns an explicit error. It names the missing or unreadable DB
    and the manual rollback of `(1816)=a` (delete `pending.json` and
    `.board.sqlite`, which hands the board back as it stood at import). The
    text is shipped text.
  - A corrupt or unreadable `.board.sqlite` with NO sentinel (possible after a
    failed sentinel write under `(1828)`) is the same explicit error. A missing
    DB with no sentinel is a first import, not an error.
  - The new binary never silently re-imports the stale folder.
  - Rejected: falling through to the `FolderStore` guard, whose "newer
    schema_version 3" message is misleading for the new binary.
- **Post-embed measurement baseline, `(1824)`** (technical, lead ruling
  2026-11-19):
  - The "before" baseline for this ticket is taken on the post-embed base,
    after `261119-feat-ws-whiteboard-retire-embed-snapshot` has landed.
  - Rejected: one shared baseline before both children, because it would
    attribute the embed savings to the store.
- **`PRAGMA locking_mode=EXCLUSIVE` before the first WAL access, `(1807)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - No `-shm` file and no per-transaction lock calls.
  - A second opener gets `SQLITE_BUSY`. That is a backstop only: `(1806)`
    refuses non-owner processes before they open the DB, so `SQLITE_BUSY` is
    left to the daemon-to-daemon handoff that `(1827)` covers.
  - Rejected: NORMAL locking as in `wsstore`.
- **Board DBs close on drain, and open retries `SQLITE_BUSY` briefly,
  `(1827)`** (technical, lead ruling 2026-11-19):
  - Every open board DB closes on daemon drain or shutdown.
  - An open retries `SQLITE_BUSY` within a short bound before it maps the
    failure to an error. Phase 1 chooses the bound and records it in its
    Result.
  - This covers the replace handoff under `locking_mode=EXCLUSIVE` (`(1807)`)
    and the long-lived handles of `(1347)=c`, where a draining daemon may still
    hold a board for a moment.
  - Rejected: failing the open at the first `SQLITE_BUSY`, because a normal
    daemon replacement would then error on its first board access.
- **DB at `ai-docs/whiteboards/<reply-id>/.board.sqlite`, `(1808)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Deleting the board stays one folder, and the existing git exclude covers it
    (`whiteboard_tools.go:905-920`).
  - The dot keeps it out of an old binary's export and backup
    (`folders_archive.go:117`) and out of the daily-GC pattern
    (`whiteboard_tools.go:51-53`).
  - Rejected: a sibling file; the cache root.
- **Schema rows mirror the folder files, `(1809)`** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning):
  - The board row stands for `meta.json`, topic rows for `tN/meta.json`, entry
    rows for log lines (append-only), panel rows for panel files (immutable),
    plus revision-receipt and load-floor rows.
  - One update commits `plan`'s delta (`folders.go:742-895`) as one
    transaction. Exact columns and indexes are left to Phase 1.
  - The topic row carries `TopicDoc` whole (`wswhiteboard/store.go:242`), for
    example a document column holding the doc minus entries, so optional
    `TopicDoc` fields survive a round trip. Example: the proof field of ready
    ticket `261114-feat-ws-content-change-relative-hover`. A topic-row schema
    with one column per current field would drop such a field.
  - Rejected: one whole-state blob, which rewrites O(board) pages per update.
- **Checkpoint and cache pragmas, `(1810)`** (owner-assumed 2026-11-19 under
  (1802)=b; owner confirms in the morning):
  - `wal_autocheckpoint=0`.
  - A PASSIVE checkpoint runs at board close, and from a background trigger
    only after the board has been idle and the WAL has passed a size
    threshold. Phase 1 measurement chooses both thresholds and records them in
    its Result.
  - `cache_size` is about 256 KiB.
  - Rejected: SQLite's default auto-checkpoint, which runs inside the commit
    that crosses 1000 pages and so lands on the response path.
- **Whole board in memory while open, `(1811)`** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning):
  - The in-memory `State` is the full board, as `FolderCache` holds today
    (`folders_cache.go:58-63`).
  - The DB is read only at open and import.
  - Rejected: a recent window in memory plus DB reads for history.
- **Derived page publish off the response path, `(1812)`** (owner-assumed
  2026-11-19 under (1802)=b; owner confirms in the morning):
  - A per-board coalescing writer publishes `index.html` / `index.stamp.js`:
    latest state wins, stamp after page.
  - A publish failure becomes a debug event and is retried by the next
    publish. Historical only: the synchronous "board committed; fix the output
    problem" error (`whiteboard_tools.go:788`) is removed by
    `261116-feat-ws-whiteboard-retire-disk-pages`, not by this store ticket.
  - Rejected: publishing under the board lock before returning
    (`mcp/session_state.go:665-675`).
- **The disk-page retirement supersedes `(1812)`'s publisher mechanism,
  `(1841)=a` (owner, 2026-11-20).** Phase 3 no longer builds or verifies a
  coalescing `index.html` / `index.stamp.js` publisher, because
  `261116-feat-ws-whiteboard-retire-disk-pages` removes those pages before this
  ticket switches the update path in Phase 2. Keep `(1812)` as decision
  history: its response-path goal still holds, but retirement satisfies it by
  removing publication rather than moving publication to a background writer.
  The retirement also owns the resulting `full:` unavailable/not-yet-served
  lines and retains its off-response-path first-open cleanup sweep. Rejected:
  keeping the publisher after its only outputs retire.
- **Zips keep the legacy folder layout, `(1813)`** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning):
  - Loading an old-layout zip reuses the import path.
  - Exporting, and the backup that `whiteboard.load` takes, write the legacy
    layout. This is a thin reuse of the existing State-to-files mapping
    `plan(nil, nil, state)` (`folders.go:742-895`). The export is
    semantically equivalent, not byte-identical (owner (1837)=a,
    2026-11-20): an old binary loading the zip, and a new binary re-importing
    it, both reach a State equal to the source (every topic, panel, item,
    answer, and log entry). File bytes, topic folder numbering, the tN
    directories, receipts, `committed.json`, load-floor history, and extra
    root files may differ, because `plan(nil, nil, state)` cannot reproduce
    them. Rejected: storing imported raw bytes in the DB for a byte-identical
    export; it adds storage and code and helps only an export taken before
    any update. The `whiteboard.load` replace calls it that way
    only when no board exists; over an existing board it calls
    `plan(&folderSession{Generation, NextTopic}, &State{}, next)` and then
    `planLoadFloor` (`folders_archive.go:254-279`). An old binary can therefore load a new
    zip, because it requires only `meta.json` and `committed.json` at the root
    (`folders_archive.go:194-196`). The zip carries no sentinel.
  - Rejected: a new-format zip that old versions refuse. With this reuse it
    saves no work.
  - Rejected: shipping the `.sqlite` file, because a held WAL DB cannot be
    copied consistently.
- **In-process change notification replaces stat watches, `(1814)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - It replaces the dashboard watch, nudge stats, `FolderCache` root stamps,
    and the topic memo, starting from `whiteboardChanged`
    (`whiteboard_tools.go:340-342`).
  - Rejected: stat polling of the DB file.
  - Rejected: keeping legacy stat detection for old processes (owner
    directive, 23:22).
- **`blocked-by: 261119-feat-ws-whiteboard-retire-embed-snapshot`, `(1815)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Rejected: carrying embed state and the synchronous embed render through the
    new store only to delete them.
- **The legacy folder is left in place untouched, with no deletion code,
  `(1816)=a`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the
  morning):
  - The folder is manual-rollback material. Deleting `pending.json` and
    `.board.sqlite` hands the board back to an older version as it stood at
    import, without the updates made since.
  - Rejected: b, deleting in a later minor release.
  - Rejected: c, deleting right after import.

## Constraints

- Manuals: `ai-docs/manuals/performance.md` (rules 1-5 are hard constraints,
  per the Decisions above; any exception follows rule 8),
  `ai-docs/manuals/release-compatibility.md` (tier settled with source lines
  before `ready/`), `ai-docs/manuals/shipped-surface-boundary.md` (error and
  result strings are shipped text), `ai-docs/manuals/ws-mcp.md` (for
  `internal/mcp/`).
- Release: T3, so this change ships only in a new minor version (Release Rule
  2; `(1805)=a`). The minor version bump is NOT part of this ticket. It happens
  at release, through the ship flow, with the owner's approval.
- The derived `index.html` / `index.stamp.js` retire in
  `261116-feat-ws-whiteboard-retire-disk-pages` before this ticket's Phase 2;
  this ticket neither carries nor recreates their publisher or refresh
  (`(1841)=a`). It keeps that ticket's off-response-path first-open sweep.
- The `whiteboard.update`, `whiteboard.query`, and `whiteboard.load` tool
  surfaces keep their arguments and result lines. Embed changes belong to the
  sibling child. Refusal semantics change in exactly these same-version ways,
  each graded under `release-compatibility.md`:
  - `261116-feat-ws-whiteboard-retire-disk-pages`, superseding `(1812)` under
    `(1841)=a`, removes the synchronous "board committed; ... fix the output
    problem" error (`mcp/whiteboard_tools.go:788`) by retiring the derived
    publisher entirely. Tier: T0. The call succeeds where a derived publish
    used to fail, and no stored data changes.
  - `(1806)` adds the direct-CLI refusal. Tier: T3. A call that worked now
    refuses. It rides the minor bump of `(1805)=a`, but the 23:22 approval
    covered only the `(1805)=a` sentinel. The owner's `(1844)=a` confirmation
    is the explicit approval Release Rule 2 requires for this T3.
  - New open and sentinel error strings (`(1828)`, `(1825)`, `(1826)`,
    `(1827)`) are shipped text under `shipped-surface-boundary.md`. Tier: T0.
    They apply only to board states the new binary creates, so no old behavior
    changes.
- Topic rows carry `TopicDoc` whole, for example a document column minus
  entries, so optional `TopicDoc` fields survive (`(1809)`). Example: the proof
  field of ready ticket `261114-feat-ws-content-change-relative-hover`.
- Existing invariants carry over unchanged: append-only entries, monotonic
  entry/panel counters, immutable panel versions, the board-revision and
  stale-nudge receipts, the load floor, and durable commit ordering
  (`folders.go:742-895`, `folders_load_floor.go`).
- Out of scope, deferred by `(1816)=a`: any deletion of legacy board folders.
  There is no deletion code.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/ws-mcp.md (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- Convention: ai-docs/manuals/performance.md (declared for `agents-plugin-tool/`)

## Prior Decisions

- 29a762990 (2026-11-19, commit): "Trace contradiction resolved: (1804)'s 'leave legacy files untouched' conflicts with (1805)=b's close-time export" (superseded by 3962f1e81 one-way migration) — bearing: supports
- 3962f1e81 (2026-11-19, commit): "whiteboard SQLite store takes T3 sentinel and one-way migration" — bearing: supports
- 261119-epic-ws-daemon-sqlite-storage (2026-11-19, Cross-Child Decisions): "Whiteboard storage shape (`(1325)=a`): One SQLite file per board (per session), opened lazily and closed when idle" (refined by `(1347)=c`) — bearing: supports
- 261119-feat-ws-whiteboard-retire-embed-snapshot (2026-11-19, Decisions): "`261119-refactor-ws-whiteboard-sqlite-board-store` carries `blocked-by:` this ticket." — bearing: constrains
- 45d559d09 (2026-11-18, commit): "fix(whiteboard): retain load floors across downgrade writes and lock waits" — bearing: constrains
- 75e35cf60 (2026-11-18, commit): "feat(whiteboard): board zip export, board.zip route, and whiteboard.load" — bearing: constrains
- 872978ce4 (2026-11-17, commit): "perf(whiteboard): serve folder-cache hits without the board flock; stamp boards by commit roots" — bearing: constrains
- 261117-research-ws-daemon-file-touch-map (2026-11-17, Whiteboard rows): "Every read takes the `.lock` flock (`wswhiteboard/folders.go:242-253`)" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard/folders*.go, board_notice.go, dashboard_answer_revision.go, mcp/whiteboard_store.go, whiteboard_tools.go, whiteboard_nudges.go, dashboard_page.go, session_state.go |
| scope.surface | cross-module | wswhiteboard store, mcp tool handlers, dashboard watch and nudges; tool surfaces stay unchanged |
| scope.new_public_symbol | yes | a new board DB store type beside FolderStore (name not fixed by the ticket) |
| scope.new_type_contract | yes | new store API, SQLite schema and sentinel pending.json schema_version 3 |
| scope.test_surface | new-files | new store tests plus existing wswhiteboard/testdata/compat-old harness and folders_*_test.go |
| complexity.reuse_points | confirmed | FolderStore.Open import, plan(nil, nil, state), wsstore/store.go SQLite setup, wsstate counters |
| complexity.side_effect_risk | high | replaces the board authority, locking model, stat watches, nudge targeting and zip path across processes |
| risk.correctness | high | one-way import, crash recovery, WAL checkpoint timing and in-process change notification replace stat detection |
| risk.fit | moderate | pilot of an epic pattern; locking_mode=EXCLUSIVE and direct-CLI refusal (1806) constrain every board caller |
| risk.test | high | needs torn-write, kill-between-commit-and-sentinel and old-binary refusal verification plus macOS measurement |
| risk.security_or_contract | high | T3 store boundary with minor bump; error and refusal strings are shipped text |

## Phases

### Phase 1: Board DB store, one-way import, and legacy-layout zip (not wired)

Prerequisite: `261119-feat-ws-whiteboard-retire-embed-snapshot` has landed
(`(1815)`, the `blocked-by` frontmatter key). The "before" baseline and the
store both start from that post-embed base (`(1824)`), so no embed state or
embed render passes through the new store.

Build the per-board SQLite store as a new type beside `FolderStore`. Nothing
in the daemon uses it yet.

- Open it with the settled pragmas (Decisions; `(1807)`, `(1810)`), the
  schema `(1809)`, and the whole-board memory model `(1811)`.
- Choose the checkpoint thresholds of `(1810)`: the idle time and the WAL size
  that trigger a background PASSIVE checkpoint. Record both in the Result.
- Open retries `SQLITE_BUSY` within a short bound (`(1827)`). Choose the bound
  and record it in the Result.
- Commit path: one update is one transaction holding only the changed rows.
  An unchanged state opens no transaction.
- One-way import (`(1804)`, `(1825)`): import the legacy folder into the DB
  through `FolderStore.Open` and commit, holding the folder's `.lock` from the
  import load through the sentinel write. Record the imported legacy
  generation in the DB. Then add the `(1805)=a` sentinel `pending.json`
  (schema 3).
- An open WITHOUT a sentinel (`(1825)`) takes the folder `.lock`, reads
  `committed.json` and any schema-2 `pending.json` (an older daemon's
  unreplayed commit), logs a divergence if either is ahead of the recorded
  imported generation (naming the overwritten intent), writes the sentinel,
  and only then releases the lock. The `(1828)` sentinel-write retry runs
  inside that same locked step.
- A sentinel with a missing or corrupt `.board.sqlite`, or a corrupt one
  without a sentinel, returns the explicit error of `(1826)`; the open never
  falls through to a re-import.
- Legacy-layout zip export and `whiteboard.load` backup through
  `plan(nil, nil, state)` (`(1813)`). Old-layout zip load goes through the
  import path.
- Capture the "before" measurement for the scenario above on the post-embed
  base (`(1824)`; macOS tracer and/or extended Go counters). Separate store I/O
  from derived-page I/O and report a store-only subtotal in addition to the
  page subtotal and combined total. The measured report replaces the derived
  table without attributing page-render cost to the store.

Verification:

- The measured post-embed baseline reports store I/O and derived-page I/O
  separately, including a store-only subtotal and the combined total.
- Round-trip import of fixture folders, including pending-intent replay, v1
  conversion, revision slots, and the load floor.
- After the import commit the legacy folder is not written again. Before it,
  only what `Open` itself replays or migrates changes (a leftover
  `pending.json`, content and close-result migrations, the `.lock` file);
  the fixtures cover each case. Then only the sentinel is added.
- A failed sentinel write still opens the DB, logs the failure, and the next
  open retries it (`(1828)`).
- The lock is held through the sentinel (`(1825)`): a legacy-layout writer that
  starts during the import waits on `.lock`, and once the sentinel exists it
  meets the schema-3 refusal, so nothing lands between the import read and the
  sentinel.
- A divergence is logged on a sentinel-less open (`(1825)`): a DB with no
  sentinel and a folder whose committed generation is ahead of the recorded one
  logs the board and both generations, stamps the sentinel, and leaves the DB
  content unchanged (no re-import). A fixture with a schema-2 `pending.json`
  (an older daemon's unreplayed commit) logs the overwritten intent, writes the
  sentinel over it, and does not replay it.
- A legacy writer holds `.lock` while the sentinel-less open waits (`(1825)`).
  After the writer releases the lock, either its commit is counted by the
  divergence check, or the writer meets the sentinel and refuses. No legacy
  commit lands unseen between the check and the sentinel. A sentinel-write
  retry (`(1828)`) runs inside the same locked step.
- A sentinel with a missing or corrupt `.board.sqlite` returns the `(1826)`
  error and neither re-imports nor writes the folder.
- A corrupt or unreadable `.board.sqlite` with no sentinel (a fixture of a
  failed sentinel write followed by corruption) returns the same `(1826)`
  error and never re-imports the stale folder.
- A round-trip fixture whose topic carries an optional `TopicDoc` field (for
  example the relative-hover proof field) keeps that field through commit,
  close, and reopen. A column-per-field topic schema would fail it
  (`(1809)`).
- An unchanged update writes zero WAL frames.
- A torn or killed transaction recovers to the last commit.
- A kill between the import commit and the sentinel write ends with the
  sentinel present after the next open.
- A PASSIVE checkpoint runs at close (`(1810)`), and the chosen thresholds are
  recorded in the Result.
- An exported zip is semantically equivalent to its source (`(1813)`,
  owner (1837)=a): the old `FolderStore` loads it into a State equal to the
  DB's State, and a new-binary re-import of it yields an equal State. Byte
  and folder-number equality is not required. The zip never contains
  `.board.sqlite` or the sentinel `pending.json`.

Deferred: wiring and the "after" measurement.

### Result (0bdc5a7b0) - 2026-11-20

Implemented `BoardDB` beside `FolderStore`; no daemon, MCP, dashboard, or
nudge caller uses it yet. The Phase 1 contract was read from `develop` at
`692a42368`, including owner `(1837)=a`; this result leaves the worktree's
existing plan text unchanged.

- VERIFIED: one held SQLite connection with `locking_mode=EXCLUSIVE` before
  WAL, `synchronous=NORMAL`, `wal_autocheckpoint=0`, and a 256 KiB cache.
  The board, whole topic documents, append-only entries, immutable panels,
  revision slots, and load floors recover into a complete in-memory State.
- VERIFIED: changed updates commit row deltas in one transaction. Unchanged
  updates open no transaction and leave the WAL bytes unchanged. Entry edits,
  removal, and moves between unrelated topics are refused; adoption retains
  the history directory and receipts do not regress. The active receipt slot
  stays in memory, so updates do not query the DB to choose it.
- VERIFIED: import holds the legacy lock through replay/conversion, DB commit,
  and schema-3 sentinel creation, and stores the imported generation.
  Fixtures compare retained legacy bytes against a control `FolderStore.Open`
  for unchanged data, content/close-result migration, pending replay, and v1
  conversion, then verify DB update/close/reopen leaves those files intact.
- VERIFIED: sentinel write failure logs and still opens the committed DB;
  reopen retries under the legacy lock. Both committed-generation divergence
  and an unreplayed pending intent are logged without re-import. A waiting
  retry sees the intervening legacy commit, and a legacy writer blocked by
  import refuses after the sentinel appears.
- VERIFIED: missing/corrupt/unreadable DB fixtures refuse with explicit manual
  rollback guidance and preserve the legacy files. Healthy owner contention
  instead gets bounded retry advice; it is not diagnosed as corruption.
- VERIFIED: process-kill fixtures recover the last committed state when killed
  before transaction commit and restore the sentinel when killed immediately
  after import commit. Optional topic proof data survives updates and reopen.
  Concurrent first importers recheck DB existence under the legacy lock.
  Duplicate legacy load-floor generations preserve FolderStore's first-record
  semantics; SQL iteration errors cannot return a partial board.
- VERIFIED: zip export and load backup reuse `plan(nil, nil, state)`.
  `LoadedAt` is cleared on the planning clone and carried by the reserved
  load-floor log. Before and after updates, `FolderStore.Open`, DB re-import,
  and DB reopen produce the same full State, including answers and panel/log
  history. Archives contain neither `.board.sqlite` nor `pending.json`.
  Load refuses zip-supplied SQLite recovery files and sidecars before import,
  so the legacy files remain the archive's authority.
  Loading an old-layout zip uses the import path; replacement preserves the
  render-sequence/receipt rules and captures the answer floor after backup
  under the lock.
- VERIFIED: close runs PASSIVE checkpoint. Background checkpoint requires
  **30 seconds idle and at least 4 MiB WAL**; a callback rechecks the latest
  idle deadline under the mutex, including when an old timer already fired.
  Import and reopen arm that timer. Tests exercise both threshold gates with
  a real WAL larger than 4 MiB. Exclusive-open retry bound: **2 seconds**;
  both release-within-bound and timeout are covered.

The inherited post-embed macOS baseline (lead-verified handoff, not rerun by
this continuation) is **122 ms first write, 75 ms steady median, 99 ms steady
p95; FileIOCounters 99 first / 107 steady**. Its harness is
`internal/mcp/whiteboard_measurement_test.go`, enabled by
`WS_WHITEBOARD_MEASURE_UPDATES`, on the post-embed stack ending at `9a0b51449`.
The harness updates one topic with a panel and log, so these measured values
are not a validation of the earlier log-only multi-topic derived table.
Counters and final-file snapshots are supporting lower-bound evidence: direct
`os.*`, flock, transient files, and SQLite VFS operations are not all counted.
A complete syscall count is unverified. The "after" measurement and live
wiring remain Phase 2/3 work. Windows AhnLab V3 measurement is deferred.

Compatibility remains the approved **T3** sentinel boundary, with the minor
bump deferred to release. The new writer stamps schema 3 in
`board_db.go:814-818`; the retained old reader/writer checks root schemas in
`folders.go:327-337` and refuses newer versions at `folders.go:314-315`, before
the callback can replay or write (`folders.go:225-258`). A new reopen reads
its unchanged committed DB (`board_db.go:169-185`). The refusal/reopen fixture
is `board_db_test.go:102-120`. Separate historical-binary verification remains
in Phase 2. Legacy-layout zip behavior is T0 in both directions under the
semantic State criterion, exercised by
`TestBoardDBLegacyArchiveExportBackupAndLoad` and `assertBoardDBArchiveState`.

Validation on the completed code, from `agents-plugin-tool` with
`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`:

- `go test ./internal/wswhiteboard ./internal/mcp -run 'Whiteboard|BoardDB|Folder' -count=1`:
  PASS (`wswhiteboard` 430.443 s; `mcp` 231.894 s).
- `go test ./internal/wswhiteboard -count=1`: PASS (510.452 s).
- `GOOS=windows go vet ./internal/wswhiteboard`: PASS.
- `go test -race ./internal/wswhiteboard -run BoardDB -count=1`: PASS (31.691 s).
- Independent `codex.medium` review: four findings fixed; final review reports
  no actionable findings. `git diff --check` passed.

### Phase 2: Switch board authority to the DB

Depends on Phase 1.

Prerequisite: `261116-feat-ws-whiteboard-retire-disk-pages` landed.

Route every remaining authoritative-store writer and reader listed in
`### Current store inventory` through the board DB store. The page-retirement
ticket has already removed `refreshWhiteboardAfter`; it is not routed or
recreated here:

- `whiteboard.update`, the dashboard answer revision, the board notice,
  `whiteboard.query` and lazy history, the served page,
  nudges, zip export, and `whiteboard.load` with its backup.
- In-process change notification (`(1814)`).
- Keep the page-retirement ticket's off-response-path first-open cleanup sweep;
  Phase 2 changes its authoritative store source but does not remove or move the
  sweep onto a response path.
- Handle lifetime per `(1347)=c`: long idle close, reopen on next access.
- Direct-CLI refusal naming the daemon as board owner per `(1806)`, keyed on
  the `BoardOwner` option introduced early by
  `261116-feat-ws-whiteboard-retire-disk-pages`, which only the daemon sets, and
  made before the DB is opened.
- Delete the now-unreachable raw-binary `full: page unavailable` branch and its
  page-retirement test when `(1806)` begins refusing every non-owner board call;
  the shipped playbook already treats every non-URL `full:` value generically,
  so this removal needs no shipped-text change.
- Close every board DB on daemon drain and shutdown (`(1827)`).

Verification:

- The old-binary harness (`wswhiteboard/testdata/compat-old`) shows that an
  old binary refuses both `read` and an update on a migrated board with the
  newer-schema error, and that the folder bytes are unchanged afterwards
  (Release Rule 2).
- An old binary loads a zip exported by the new one, and the new one loads an
  old-layout zip.
- A non-owner process's board call fails with the `(1806)` error before the DB
  file is opened, without relying on `SQLITE_BUSY`.
- A direct process starts first, then the daemon still opens the board
  (`(1806)`).
- A replacing daemon opens a board that the draining daemon still held: the
  open succeeds within the `(1827)` bound once the drain closes the DB, and an
  open that outlasts the bound returns an explicit error.
- The dashboard and the nudge see an update through in-process notification,
  with no stat of board files (`(1814)`). The disk-page retirement has already
  removed the derived `index.html` / `index.stamp.js` path (`(1841)=a`).
- An idle close followed by a reopen keeps the board's state intact
  (`(1347)=c`).

Deferred: the measured report.

### Result (d1f6f4448) - 2026-11-21

- VERIFIED. The daemon shares one registry across proxy MCP, its one-shot CLI,
  dashboard reads and answers, hosted restore rendering, and nudges.
  `cmd/ws-mcp/dashboard.go:79-110,143-148,357-365`,
  `cmd/ws-mcp/proxy.go:89-98`, `cmd/ws-mcp/hosted_cli.go:74-82`.
  Updates, queries, history, notices, and ZIP export and replacement resolve
  through that registry. `mcp/whiteboard_store.go:46-108`,
  `mcp/dashboard_page.go:374-418`, `mcp/dashboard_answer_revision.go:19-35`,
  `mcp/whiteboard_nudges.go:417-447`, `mcp/whiteboard_tools.go:363-425`.
- VERIFIED. Successful revision changes schedule an in-process index re-read
  and nudge. Page watches still track external images and tickets. Nudge
  discovery retains absent-board evidence and checks new or poked identities.
  Owner reads and writes renew the 30-minute lease. Index polling and
  unchanged nudge checks leave the current deadline intact. Expiry closes
  the DB and the next access recovers it. Daemon drain closes all held DBs
  before unlocking. `mcp/whiteboard_registry.go:14,62-209`,
  `mcp/dashboard_sessions.go:917,1029-1060`, `mcp/dashboard_page.go:274-290`,
  `mcp/whiteboard_nudges.go:451-543`, `wsdashboard/daemon_runtime.go:148-168`.
  Both exclusive-open handoff outcomes and idle reopen passed in
  `mcp/whiteboard_registry_test.go`. The first-open cleanup still runs
  asynchronously through `mcp/whiteboard_retirement.go:25-40`.
- VERIFIED. A direct CLI refuses update, query, and load before creating any
  board files, then the hosted CLI opens the same board from a daemon-owned
  registry. This uses the explicit owner option before the dispatcher runs
  a board handler. The dedicated raw `full: page unavailable` branch is gone.
  `mcp/server_dispatch.go:80-84`, `mcp/whiteboard_tools.go:281-287`,
  `cmd/ws-mcp/hosted_cli_notification_test.go:18-64`.
- **Decision (owner, 2026-11-21).** Raw `workflow_manual` and `workflow_state`
  retain their agenda and todo output and state that open-board summaries
  require the owning daemon. Daemon-hosted calls and the hosted restart
  bundle keep the summaries. VERIFIED by
  `mcp/whiteboard_registry_test.go:114` through `mcp/whiteboard_store.go:50-57`,
  `mcp/workflow_manual.go:148-152`, and `mcp/hosted_render.go:52-53,103-113`.

**Directional compatibility**

- VERIFIED, T3. The new side reads the old folder through locked legacy replay
  and conversion, records its generation, and stamps schema 3 in
  `pending.json`. `wswhiteboard/board_db.go:188-261,936-941`. The old side's
  guard sees the newer pending schema before replay or write.
  `wswhiteboard/folders.go:225-258,304-340`. A binary built from `8cd2566ec`
  refused both read and update, emitted `newer schema_version 3 (supported 2)`,
  and left the folder entries and bytes identical. Reopening with the new
  side recovered the same committed DB state.
  `wswhiteboard/board_db_compat_test.go:175-203`.
- VERIFIED, T3. Raw CLI board calls that went through the in-process server at
  `2d528c09b` now refuse before DB admission. The daemon can still open the
  board. The approved minor bump stays in the release flow.
- VERIFIED, T0 ZIP. `8cd2566ec` predates the archive API. A second historical
  binary built from the pre-switch `2d528c09b` source loaded the new ZIP
  through its real `FolderStore.LoadArchive`; the new `BoardDB.LoadArchive`
  loaded that binary's export. Full fixture State matched after accounting
  for the load sequence and answer floor.
  `wswhiteboard/board_db_compat_test.go:205-220`.

**Verification on macOS**

- VERIFIED. The merged `2d528c09b` baseline passed `go build ./...` and
  `go test ./internal/wswhiteboard -count=1` (198.183 s). The implementation
  passed the same commands (`wswhiteboard` 286.637 s), `go vet
  ./internal/wswhiteboard ./internal/mcp ./cmd/ws-mcp ./internal/wsdashboard`,
  and `gofmt -l` on changed Go files with no output. `git diff --check` passed.
- VERIFIED. `go test ./cmd/ws-mcp -run '^TestWhiteboardCLIAuthority$'
  -count=1` passed. The opt-in `TestBoardDBHistoricalCompatibility` passed
  (2.230 s), with historical build instructions in its test source.
  The MCP whiteboard and workflow selectors passed with the three loopback
  cases excluded (47.500 s). The registry and concurrent-read race selectors
  passed, including the separate index-idle test. The dashboard subset without
  browser/loopback cases and the baseline red below passed (4.080 s).
- VERIFIED. The requested wider invocations reached sandbox
  `bind: operation not permitted` on loopback. They need a run with loopback
  available. `TestBoardHistoryHooksSurroundSSESwap` also fails on the
  `2d528c09b` baseline with Node's `syncSpinnerPhases is not defined`.
  Exact commands from `agents-plugin-tool/`:

  ```sh
  GOCACHE=/tmp/ws-go-cache go test ./internal/mcp -timeout 45m -run 'Whiteboard|BoardArchive|BoardHistory|BoardIncarnation|Dashboard.*(Board|Topic)|Sublead|HostedSessionNudges|DashboardKeyOffAndKeyOn' -count=1
  PLAYWRIGHT_BROWSERS_PATH="$HOME/Library/Caches/ms-playwright" WS_PLAYWRIGHT_MODULE="$(ls -d ~/.npm/_npx/*/node_modules/playwright | head -1)" GOCACHE=/tmp/ws-go-cache go test ./internal/wsdashboard -run '^Test(Board|Whiteboard)|Whiteboard' -timeout 30m -count=1
  ```

Windows AhnLab V3 measurement remains deferred.

### Phase 3: Update-path measured result

Depends on Phase 2 and on `(1815)` (the embed sibling has landed).

- Measure "after" with Phase 1's method: file operations per
  `whiteboard.update`, the fsync count, and update latency on macOS.
- Report before and after in the Result, and record the Windows V3
  measurement as deferred.
- List any remaining rule 1-5 gap with its measured cost for the owner
  (rule 8), citing `performance.md`'s rule numbers (`(1346)`).

The trace measures the store path only. Per `(1841)=a`,
`261116-feat-ws-whiteboard-retire-disk-pages` has already removed the derived
page and stamp reads/writes; this ticket does not reintroduce them.

Verification, on a traced single `whiteboard.update` (the Phase 1 scenario):

- No per-call open, flock, or stat of board files (Rule 4), and no derived
  `index.html` / `index.stamp.js` access.
- One WAL append per update, and nothing written for an unchanged state
  (Rule 2).
- No fsync on the response path (Rule 5).
- Every remaining gap is listed per Rule 8 with its measured cost. A gap fails
  the phase unless the owner approves it.

### Result (b83c9b543) - 2026-11-21

VERIFIED on macOS 26.6.2 arm64 with Go 1.26.2. Each run issued ten
`whiteboard.update` calls using Phase 1's one-topic panel-and-log input. The
first row is a fresh board and the steady sample has nine updates. The
before run used an archive of the merged, post-retirement `2d528c09b` source.
The after run used the implementation at `d1f6f4448` with the expanded harness.
These runs supplement the original Phase 1 measurement.

| Per action | Before, folder | After, SQLite |
|---|---:|---:|
| First update wall time | 91.926 ms | 32.224 ms |
| Steady wall time p50 / p95 | 63.651 / 76.655 ms | 1.252 / 2.325 ms |
| First update `wsstate` wrapper operations | 96 | 58 |
| Steady `wsstate` wrapper operations p50 / p95 | 103 / 103 | 19 / 19 |

- VERIFIED. The per-tool wrapper totals include session-key and other caller
  I/O. Every SQLite steady action had 7 instrumented reads and 12 stats across
  the fixture. The standalone changed and unchanged `BoardDB.Update` actions
  each recorded zero `wsstate` wrapper operations. These counters leave direct
  Go file operations, flock, SQLite VFS activity, and fsync unmeasured.
  `wsstate/file_io_counts.go:13-43`, `mcp/whiteboard_measurement_test.go:83,408`.
- VERIFIED. In the board directory, every steady update added one WAL commit
  marker. Eight added 7 frames and 28,840 bytes each. The last added 10
  frames and 41,200 bytes. There were zero WAL header resets in the nine
  steady actions and no final file changes outside the WAL. The first update
  added 23 frames and 2 markers, covering SQLite schema and board creation.
  The separate store mutation added 1 frame, 1 marker and 4,120 bytes.
  A genuinely unchanged store state added zero frames and bytes. An empty
  MCP command advanced sequence metadata and added 1 frame. There were no
  derived page or stamp changes. `mcp/whiteboard_measurement_test.go:83,178,240,408`.
- VERIFIED. Ten additional paired samples over the same MCP input forced
  PASSIVE after each steady update. All ten subsequent writes reset the
  WAL header. Their p50 / p95 were 1.865 / 3.044 ms, compared with
  1.565 / 2.624 ms for the paired steady writes and 1.252 / 2.325 ms in
  the primary steady sample. In these samples the slowest reset write was
  1.31 times the primary steady p95, below the owner's 10-times Forward
  threshold. This times the whole tool action.
  `mcp/whiteboard_measurement_test.go:455-491`.

**Performance rules 1–5**

- VERIFIED by source and store test. Rules 1, 3 and 4 use the held memory
  snapshot, one board DB with WAL, and one exclusive connection for the
  handle lifetime. The warm store API goes directly to State or the held
  connection. The store test covers the absence of a shared-memory file.
  Cold import and the 30-minute idle reopen use filesystem operations,
  as approved in `(1347)=c`. Exact per-action OS open, flock, and stat
  syscall counts remain unmeasured.
  `wswhiteboard/board_db.go:154-261,360-370,644-711`,
  `wswhiteboard/board_db_test.go:695`, `mcp/whiteboard_registry.go:62-114,150-179`.
- VERIFIED for Rule 2. One logical append transaction per changed steady
  update and no transaction for identical state. SQLite writes several
  WAL frames within some transactions. The syscall count for a physical
  append remains unmeasured. `wswhiteboard/board_db.go:666-711`.
- INFERRED from the pinned SQLite source for Rule 5. `synchronous=NORMAL`
  disables ordinary per-commit WAL sync. The first write into a reused WAL
  can sync its header after a checkpoint. The Darwin arm64 implementation
  in `modernc.org/sqlite v1.30.2` sets the sync flags at
  `lib/sqlite_darwin_arm64.go:41625-41644`, restarts and syncs the header
  in `_walFrames` at `:48986-49030`, and calls `fsync` in `_full_fsync`
  at `:29390-29425`. The board runs thresholded idle PASSIVE checkpoints
  on a background timer. The owner accepted one header sync per reset cycle
  under Rule 8 on 2026-11-21. The sampled cost includes any sync and other
  work in the write. It does not isolate sync time or concurrent
  checkpoint contention. `go.mod:11`,
  `wswhiteboard/board_db.go:365-367,989-1025`.

The owner limited verification to in-process measurement. macOS refused
unprivileged `fs_usage`. Exact syscall counts and actual response-path
fsync counts remain unmeasured in both directions. Windows AhnLab V3
measurement remains deferred.

VERIFIED commands from the paired source directories, plus `go vet
./internal/mcp`, `gofmt -l`, and `git diff --check`.

```sh
# Baseline archive of 2d528c09b, from agents-plugin-tool/
GOCACHE=/tmp/ws-go-cache WS_WHITEBOARD_MEASURE_UPDATES=10 WS_WHITEBOARD_MEASURE_LABEL=pre-switch-2d528c09b go test ./internal/mcp -run '^TestWhiteboardUpdateMeasurement$' -count=1 -v

# Implementation worktree, from agents-plugin-tool/
GOCACHE=/tmp/ws-go-cache WS_WHITEBOARD_MEASURE_UPDATES=10 WS_WHITEBOARD_MEASURE_LABEL=sqlite-d1f6f4448 go test ./internal/mcp -run '^TestWhiteboard(UpdateMeasurement|MeasurementWALDelta|StoreMeasurement|CheckpointResetMeasurement)$' -count=1 -v
```

Both opt-in runs passed, in 1.544 s and 1.060 s respectively. The
Phase 2 directional T3 boundary and T0 ZIP results still apply.
Phase 3 changes only the measurement harness and this Result.

Lead-VERIFIED after Phase 2. The two loopback-blocked commands were
rerun at `22a413c9f` where loopback was available. The MCP suite
passed in 48.5 s. The dashboard suite reported the same five failures
as `develop` at `fa3c2a039` (`TestBoardHoldPillBrowser`,
`TestBoardStateFilterBrowser`, `TestBoardTopicUnitsBrowser`,
`TestWhiteboardEffectsBrowser`, and `TestBoardHistoryHooksSurroundSSESwap`).
