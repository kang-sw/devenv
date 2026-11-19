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
  (`whiteboard_tools.go:50-52`, `:930-967`).

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

The decisions below settled the queue on 2026-11-19 with the lead's
recommendations as assumed answers. Each line reads "owner-assumed": the owner
confirms them before this work merges to `develop`.

- **Compatibility tier T2, `(1805)=b`** (owner-assumed 2026-11-19 under
  (1802)=b; owner confirms in the morning):
  - Graceful close exports the board once in the legacy folder layout.
    Graceful close means the `(1347)=c` idle close and daemon shutdown.
  - The DB records the exported folder generation as the export stamp.
  - On open, if the folder's committed generation has moved past that stamp,
    the new binary imports only what the folder gained and merges it. Boards
    are append-only: log entries, panel versions, items.
  - The close-time export writes files, which breaks performance.md rule 2. It
    is owner-approved under rule 8 on the mailbox store precedent
    (`performance.md:57-61`), and it runs off the interactive path.
  - Rejected: a, a schema-3 sentinel `pending.json` (T3). It needs a minor bump
    that locks out installed previous-minor sessions, and AGENTS.md prefers
    T0-T2.
  - Rejected: c, rewriting `committed.json`/`meta.json` with schema 3. It is T3
    and also modifies legacy data.
- **Tier reasoning per `release-compatibility.md` (T2 is the highest affected
  path, so a patch is allowed under Release Rule 1)** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning):
  1. Old folder, new binary: the first open imports through the retained
     `FolderStore.Open` (`folders.go:143-159`, `:400-490`). That covers pending
     replay, the schema guard, and v1 conversion. No T4.
  2. New data, old binary: an older daemon can run when it starts first
     (`wsdashboard/ensure.go:210-217` only stops a newer daemon from being
     replaced). It reads the folder last exported at graceful close through its
     unchanged schema-2 guard (`folders.go:304-321`). It never sees
     `.board.sqlite`: the dot prefix keeps it out of `boardFiles`
     (`folders_archive.go:117`), and old code opens only named files.
  3. Old writes, then the new binary reopens: an old write advances the folder
     generation (`folders.go:745-747`). Entries and panels stay append-only
     (`folders.go:810-812`, `:977-994`). The new open compares the folder's
     `committed.json` generation (`folders.go:93-96`) with the export stamp and
     merges what the folder gained. The retained raw data is the DB (new
     values) plus the folder (old values), and the merge is the new reader's
     reconstruction. That makes it T2 (`release-compatibility.md:63`, `:67-71`).
  4. Crash without a graceful close: the folder stays at its last export. An
     older daemon serves that stale state and may write on it, while the DB
     holds newer updates. Both sides diverge, and the open-time merge imports
     what the folder gained. The cases it does not yet settle are the Phase 1
     stop condition.
  5. Zips: T0 in both directions (`(1813)`).

  `release-compatibility.md:118-122` requires a concrete trace of the value
  through the old write and the new reconstruction. That trace is Phase 2's
  compat-old verification.
- **Import, then folder written only by the export, `(1804)`, amended by
  `(1805)=b`** (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the
  morning):
  - On first start the new binary imports the legacy board folder into the DB
    through the existing `FolderStore.Open`.
  - After that, the folder is written only by the graceful-close export, and it
    is never deleted (`(1816)=a`). The drafted "leaves the legacy files
    untouched" no longer holds under `(1805)=b`.
  - Rejected: dual-write on every update, because it keeps the per-update 2PC
    cost this ticket removes.
- **Direct CLI never co-opens the board DB, `(1806)`** (owner-assumed
  2026-11-19 under (1802)=b; owner confirms in the morning):
  - A direct `ws-mcp` CLI or `serve` process routes board calls to the daemon,
    or refuses with an error naming the daemon as the board owner.
  - Rejected: concurrent WAL access from a second process, because it brings
    back cross-process locking and per-call revalidation (rules 1 and 4).
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
    (`whiteboard_tools.go:50-52`).
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
  - A PASSIVE checkpoint runs at board close, and from a background trigger only
    after the board has been idle and the WAL has passed a size threshold.
    Phase 1 measurement chooses both thresholds and records them in its Result.
  - `cache_size` is about 256 KiB.
  - Rejected: SQLite's default auto-checkpoint, which runs inside the commit that
    crosses 1000 pages and so lands on the response path.
- **Whole board in memory while open, `(1811)`** (owner-assumed 2026-11-19
  under (1802)=b; owner confirms in the morning):
  - The in-memory `State` is the full board, as `FolderCache` holds today
    (`folders_cache.go:58-63`).
  - The DB is read only at open, import, and merge.
  - Rejected: a recent window in memory plus DB reads for history.
- **Derived page publish off the response path, `(1812)`** (owner-assumed
  2026-11-19 under (1802)=b; owner confirms in the morning):
  - A per-board coalescing writer publishes `index.html` / `index.stamp.js`:
    latest state wins, stamp after page.
  - A publish failure becomes a debug event and is retried by the next publish.
    The synchronous "board committed; fix the output problem" error
    (`whiteboard_tools.go:788`) goes away.
  - Rejected: publishing under the board lock before returning
    (`mcp/session_state.go:665-675`).
- **Zips and the load backup keep the legacy folder layout, `(1813)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Export builds folder files from memory. Load accepts legacy-layout zips into
    a fresh board DB. The close-time export of `(1805)=b` uses the same
    materializer.
  - Rejected: shipping the `.sqlite` file. An old load requires `meta.json` and
    `committed.json` at the root (`folders_archive.go:194-196`), and a held WAL
    DB cannot be copied consistently.
- **In-process change notification replaces stat watches, `(1814)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - It replaces the dashboard watch, nudge stats, `FolderCache` root stamps, and
    the topic memo, starting from `whiteboardChanged`
    (`whiteboard_tools.go:340-342`).
  - Rejected: stat polling of the DB file.
- **`blocked-by: 261119-feat-ws-whiteboard-retire-embed-snapshot`, `(1815)`**
  (owner-assumed 2026-11-19 under (1802)=b; owner confirms in the morning):
  - Rejected: carrying embed state and the synchronous embed render through the
    new store only to delete them.
- **Legacy folders are never deleted automatically, `(1816)=a`** (owner-assumed
  2026-11-19 under (1802)=b; owner confirms in the morning):
  - The folder is the downgrade path under `(1805)=b`.
  - Rejected: b, deleting in a later minor release.
  - Rejected: c, deleting right after import.

## Constraints

- Manuals: `ai-docs/manuals/performance.md` (rules 1-5 are hard constraints,
  per the Decisions above; any exception follows rule 8),
  `ai-docs/manuals/release-compatibility.md` (tier settled with source lines
  before `ready/`), `ai-docs/manuals/shipped-surface-boundary.md` (error and
  result strings are shipped text), `ai-docs/manuals/ws-mcp.md` (for
  `internal/mcp/`).
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
- Legacy folder files are never deleted. After the first import they are
  written only by the graceful-close export (`(1805)=b`).
- Out of scope, deferred by `(1816)=a`: any automatic deletion of legacy board
  folders. A later ticket may add an explicit cleanup command.

## Phases

### Phase 1: Board DB store, import, close-time export, and merge (not wired)

Build the per-board SQLite store as a new type beside `FolderStore`. Nothing
in the daemon uses it yet.

- Open it with the settled pragmas (Decisions; `(1807)`, `(1810)`), the
  schema `(1809)`, and the whole-board memory model `(1811)`.
- Commit path: one update is one transaction holding only the changed rows. An
  unchanged state opens no transaction.
- Import: the legacy folder goes into the DB through `FolderStore.Open`,
  recorded so that it runs once.
- Legacy-layout materializer, used by the zip export, the load backup
  (`(1813)`), and the close-time export (`(1805)=b`).
- Close-time export: write the in-memory board into the legacy folder through
  the existing `FolderStore` commit protocol, so `plan` writes only what changed
  since the last export. Then record the export stamp, the folder generation,
  in the DB.
- Open-time merge: when the folder's `committed.json` generation is past the
  export stamp, import the log entries, panel versions, and items the folder
  gained. Record the new stamp.
- Capture the "before" measurement for the scenario above on the base (macOS
  tracer and/or extended Go counters). It replaces the derived table.

Stop condition: `(1805)=b` does not yet settle the following merge cases. A
worker stops and reports rather than inventing a rule.

- Both sides minted the same entry, panel, or item ID. This only happens in the
  crash case, and item IDs are user-visible.
- Both sides changed the same mutable field: an item's status or answer, a
  topic body, label, or close state.
- The folder was replaced by an old `whiteboard.load` (a fresh history, not an
  extension of the export).

Verification:

- Round-trip import of fixture folders, including pending-intent replay, v1
  conversion, revision slots, and the load floor.
- An unchanged update writes zero WAL frames.
- A torn or killed transaction recovers to the last commit.
- Materialized export equals the source folder's file set and bytes under the
  `boardFiles` rules.
- Export, then a folder-side append, then open: the merge imports exactly the
  appended entries, panels, and items.

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
- Close-time export at that idle close and at daemon graceful shutdown, and the
  open-time merge (`(1805)=b`).
- Direct-CLI routing or refusal per `(1806)`.

Verification:

- The old-binary harness (`wswhiteboard/testdata/compat-old`) shows that an
  old-binary write made after import survives the next new-binary open. It must
  hold both after a graceful close and after a crash without export, within the
  cases the Phase 1 stop condition does not hold back.
- Zips cross between old and new in both directions.
- A second process's open fails with the `(1806)` error.

Deferred: async derived publish and the measured report.

### Phase 3: Update-path latency and measured result

Depends on Phase 2 and on `(1815)` (the embed sibling has landed).

- Move derived page publication off the response path (`(1812)`).
- Measure "after" with Phase 1's method: file operations per
  `whiteboard.update`, the fsync count, and update latency on macOS. Also
  report the close-time export's file operations separately; it is off the
  interactive path, under rule 8.
- Report before and after in the Result, and record the Windows V3
  measurement as deferred.
- List any remaining rule 1-5 gap with its measured cost for the owner
  (rule 8).
