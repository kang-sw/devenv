---
title: "화이트보드 2PC 폴더 저장소를 보드별 SQLite로 전환"
parent: 261119-epic-ws-daemon-sqlite-storage
blocked-by: 261119-feat-ws-whiteboard-retire-embed-snapshot
related:
  261119-feat-ws-whiteboard-retire-embed-snapshot: "sibling child; removes the per-update embed render this store would otherwise carry (see (1815))"
  261116-feat-ws-whiteboard-retire-disk-pages: "derived index.html / index.stamp.js stay files here; their retirement is that ticket's scope"
  261117-research-ws-daemon-file-touch-map: "storage audit background (whiteboard rows)"
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
were read on `develop` at `9f43a8f00`; they drift.

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
  `mcp/whiteboard_tools.go:276-306`); render-only refresh after todo/agenda
  tools (`whiteboard_tools.go:570-589`); dashboard answer revision
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
  board folder. Browsers read only the derived `index.html` and poll
  `index.stamp.js`.
- The daily orphan GC deletes only root-level files of dead sessions matching
  `^([a-z0-9]+)(?:\.html|\.stamp\.js|-[0-9a-f]+\.html)$`, never directories
  (`whiteboard_tools.go:51-53`, `:930-967`).

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
  `ai-docs/manuals/performance.md` rules 1-5 are hard design constraints here:
  1. Reads come from memory only. The DB serves recovery and history.
  2. One change is one transaction, which is one WAL append. An unchanged state
     writes nothing.
  3. `PRAGMA synchronous=NORMAL` in WAL mode, so there is no fsync per commit.
     Checkpoints are rare and run off the interactive path.
  4. Handles are held: no per-call open, lock, or `stat`.
  5. `whiteboard.update` latency waits on nothing beyond that single WAL
     append.
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
  `6539b0c01`). Rule 3 above overrides the precedent's default `synchronous`
  setting (`wsstore/store.go:209-224` sets none).
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
  owner approved on 2026-11-19 at 23:22.
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
  - A DB found without the sentinel gets it on open.
  - After the import commit there is no write to the legacy folder, apart
    from what `Open` itself replays or migrates: a leftover `pending.json`,
    content and close-result migrations, and the `.lock` file
    (`folders.go:143-159`, `:400-447`).
  - Rejected: dual-write on every update, because it keeps the per-update 2PC
    cost.
  - Rejected: legacy write-back of any kind (owner directive, 23:22).
  - Rejected: a new read-only import path, because it duplicates replay
    logic and an unreplayed intent would be lost from the import.
- **Direct CLI never co-opens the board DB, `(1806)`** (owner-assumed
  2026-11-19 under (1802)=b; owner confirms in the morning):
  - A direct `ws-mcp` CLI or `serve` process refuses board calls with an
    error naming the daemon as the board owner.
  - Rejected: routing board calls to the daemon. No proxy exists in
    `cmd/ws-mcp`, so it would be new work, and 0.50+ launchers always go
    through the daemon.
  - Rejected: concurrent WAL access from a second process, because it brings
    back cross-process locking and per-call revalidation (rules 1 and 4).
- **Sentinel write failure does not block the open, `(1823)`** (technical,
  lead ruling 2026-11-19):
  - When the sentinel write fails after the import commit (read-only folder,
    I/O error), the DB still opens.
  - The sentinel is retried on the next open, and the failure is logged.
  - Rejected: refusing to open, because it strands a board whose data is
    already safely in the DB.
- **Post-embed measurement baseline, `(1824)`** (technical, lead ruling
  2026-11-19):
  - The "before" baseline for this ticket is taken on the post-embed base,
    after `261119-feat-ws-whiteboard-retire-embed-snapshot` has landed.
  - Rejected: one shared baseline before both children, because it would
    attribute the embed savings to the store.
- **`PRAGMA locking_mode=EXCLUSIVE` before the first WAL access, `(1807)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - No `-shm` file and no per-transaction lock calls.
  - A second opener gets `SQLITE_BUSY`, which `(1806)`'s refusal maps to its
    error.
  - Rejected: NORMAL locking as in `wsstore`.
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
    publish. The synchronous "board committed; fix the output problem" error
    (`whiteboard_tools.go:788`) goes away.
  - Rejected: publishing under the board lock before returning
    (`mcp/session_state.go:665-675`).
- **Zips keep the legacy folder layout, `(1813)`** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning):
  - Loading an old-layout zip reuses the import path.
  - Exporting, and the backup that `whiteboard.load` takes, write the legacy
    layout. This is a thin reuse of the existing State-to-files mapping:
    `plan(nil, nil, state)` already yields every folder file's exact bytes
    (`folders.go:742-895`). The `whiteboard.load` replace calls it that way
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
  2; `(1805)=a`).
- The derived `index.html` / `index.stamp.js` stay files while `file://` links
  remain (epic Non-Scope). Their retirement belongs to
  `261116-feat-ws-whiteboard-retire-disk-pages`.
- The `whiteboard.update`, `whiteboard.query`, and `whiteboard.load` tool
  surfaces (arguments, result lines, refusal semantics) do not change in this
  ticket. Embed changes belong to the sibling child.
- Existing invariants carry over unchanged: append-only entries, monotonic
  entry/panel counters, immutable panel versions, the board-revision and
  stale-nudge receipts, the load floor, and committed-before-publish ordering
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

Build the per-board SQLite store as a new type beside `FolderStore`. Nothing
in the daemon uses it yet.

- Open it with the settled pragmas (Decisions; `(1807)`, `(1810)`), the
  schema `(1809)`, and the whole-board memory model `(1811)`.
- Commit path: one update is one transaction holding only the changed rows.
  An unchanged state opens no transaction.
- One-way import (`(1804)`): import the legacy folder into the DB through
  `FolderStore.Open` and commit. Then add the `(1805)=a` sentinel
  `pending.json` (schema 3). A DB without the sentinel gets it on open.
- Legacy-layout zip export and `whiteboard.load` backup through
  `plan(nil, nil, state)` (`(1813)`). Old-layout zip load goes through the
  import path.
- Capture the "before" measurement for the scenario above on the post-embed
  base (`(1824)`; macOS tracer and/or extended Go counters). It replaces the
  derived table.

Verification:

- Round-trip import of fixture folders, including pending-intent replay, v1
  conversion, revision slots, and the load floor.
- After the import commit the legacy folder is not written again. Before it,
  only what `Open` itself replays or migrates changes (a leftover
  `pending.json`, content and close-result migrations, the `.lock` file);
  the fixtures cover each case. Then only the sentinel is added.
- A failed sentinel write still opens the DB, logs the failure, and the next
  open retries it (`(1823)`).
- An unchanged update writes zero WAL frames.
- A torn or killed transaction recovers to the last commit.
- A kill between the import commit and the sentinel write ends with the
  sentinel present after the next open.
- An exported zip equals the source folder's file set and bytes under the
  `boardFiles` rules.

Deferred: wiring and the "after" measurement.

### Phase 2: Switch board authority to the DB

Depends on Phase 1.

Route every writer and reader listed in `### Current store inventory` through
the board DB store:

- `whiteboard.update`, the render-only refresh, the dashboard answer revision,
  the board notice, `whiteboard.query` and lazy history, the served page,
  nudges, zip export, and `whiteboard.load` with its backup.
- In-process change notification (`(1814)`).
- Handle lifetime per `(1347)=c`: long idle close, reopen on next access.
- Direct-CLI refusal naming the daemon as board owner per `(1806)`.

Verification:

- The old-binary harness (`wswhiteboard/testdata/compat-old`) shows that an
  old binary refuses both `read` and an update on a migrated board with the
  newer-schema error, and that the folder bytes are unchanged afterwards
  (Release Rule 2).
- An old binary loads a zip exported by the new one, and the new one loads an
  old-layout zip.
- A second process's open fails with the `(1806)` error.

Deferred: async derived publish and the measured report.

### Phase 3: Update-path latency and measured result

Depends on Phase 2 and on `(1815)` (the embed sibling has landed).

- Move derived page publication off the response path (`(1812)`).
- Measure "after" with Phase 1's method: file operations per
  `whiteboard.update`, the fsync count, and update latency on macOS.
- Report before and after in the Result, and record the Windows V3
  measurement as deferred.
- List any remaining rule 1-5 gap with its measured cost for the owner
  (rule 8).
