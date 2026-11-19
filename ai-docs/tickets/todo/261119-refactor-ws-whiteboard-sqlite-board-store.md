---
title: "화이트보드 2PC 폴더 저장소를 보드별 SQLite로 전환"
parent: 261119-epic-ws-daemon-sqlite-storage
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
- Legacy folder files are never deleted or rewritten by Phases 1-3.

## Open Decision Queue

Last ID used: (1822)

(1804) [open] [technical] Compatibility: on first start the new binary imports the legacy board folder into the DB once, through the existing `FolderStore.Open` (pending replay, schema guard, v1 conversion), and leaves the legacy files untouched.
- One-time import over a dual-write or shutdown export. Reasons: the importer
  reuses the verified recovery path (`folders.go:143-159`, `:400-490`), and
  dual-write would keep the per-update 2PC cost this ticket removes.
- Forward (old data, new binary): no T4. The new binary reads every old folder
  and v1 record through the retained `FolderStore` code.
- Backward (new data, old binary) on the import-only design. An older daemon
  can run against this board after a reboot or daemon exit, when an older-patch
  plugin starts first. `keepRunning` keeps any running daemon that is at least
  as new (`wsdashboard/ensure.go:210-217`). A newer starter replaces an older
  daemon, but nothing stops an older one from starting while none is running.
  That older daemon passes the folder schema guard
  (`folders.go:304-321`, schema 2 == 2) and serves the board as it stood at
  import. Every update made since then is missing. It accepts writes into the
  folder (`folders.go:957-1045`). The dashboard download zips that stale folder
  (`folders_archive.go:51-91`), and `whiteboard.load` replaces into it
  (`folders_archive.go:241-294`). When the new binary reopens, the DB exists, so
  it ignores the folder and loses the old binary's writes.
- Tier: the import-only design is not <= T2. The new data survives in the DB,
  but writes made during the downgrade window are lost with no reconstruction.
  Release Rule 1 (`release-compatibility.md:129-131`) then requires preservation
  or a T3 boundary. The remedy is open as (1805).

(1805) [open] [policy] How an older binary meets a board after import, given (1804) alone is not <= T2.
- Context: (1804) shows an older daemon can serve the stale folder and lose its
  writes on the next upgrade. Old-side refusal is already in the code: the
  guard refuses any root file whose `schema_version` is newer than 2
  (`folders.go:314-316`). Old `FolderCache`, `present()`, nudge stats, export,
  and load all pass through that guard or through `pending.json` presence
  before they use the board (`folders_cache.go:84-94`,
  `folders_archive.go:35-60`).
- Alternative a (T3): at import, add a sentinel `pending.json` with
  `schema_version: 3`. Existing legacy files stay byte-identical. An old binary
  then refuses every read and write of that board with "newer schema_version"
  (`folders.go:314-316`) and loses nothing. Needs a minor release and explicit
  owner approval (Release Rule 2).
- Alternative b (T2): export the legacy folder from memory at board close or
  daemon shutdown (the mailbox Rule 8 precedent, `performance.md:57-61`). On
  open, re-import when the folder's generation has advanced past the exported
  generation. Cost: a whole-board 2PC write per close. A crash between export
  and close leaves both sides advanced, and then a conflict rule is needed
  (keep the DB and back up the folder, with no merge).
- Alternative c: rewrite `committed.json`/`meta.json` with schema 3. T3 like a,
  but it modifies legacy files, which (1804) forbids.
> Recommend a: no file work on the close path, no merge rule, and the old side
> refuses before it writes. It costs a minor bump.

(1806) [open] [technical] A direct `ws-mcp` CLI or `serve` process (not through the daemon) never opens a board DB the daemon holds: board calls route to the daemon, or refuse with an error naming the daemon as the board owner.
- Routing or refusal over concurrent WAL access. The epic's direct-binary
  question (`cmd/ws-mcp/main.go`) is settled here for this store only. A second
  process's own page cache and checkpoints would bring back cross-process
  locking and per-call revalidation (rules 1 and 4), the costs this ticket
  removes.

(1807) [open] [technical] Open each board DB with `PRAGMA locking_mode=EXCLUSIVE` set before the first WAL access.
- EXCLUSIVE over NORMAL locking: SQLite then keeps the WAL index in heap memory
  and creates no `-shm` file, so there are no per-transaction lock calls
  (rule 4). A second opener gets `SQLITE_BUSY`, which (1806)'s refusal maps to
  its error. This differs from the `wsstore` precedent, which uses NORMAL.

(1808) [open] [technical] The DB lives at `ai-docs/whiteboards/<reply-id>/.board.sqlite`, inside the existing board folder.
- Inside the board folder over a sibling file or the cache root. Deleting the
  board stays one folder. The existing git exclude covers it
  (`whiteboard_tools.go:905-920`). The leading dot keeps an old binary's
  export and backup from copying a live DB (`folders_archive.go:117`), and it
  matches no daily-GC pattern (`whiteboard_tools.go:50-52`).

(1809) [open] [technical] Schema: rows mirror the folder files (one board row for `meta.json`, topic rows for `tN/meta.json`, append-only entry rows for log lines, immutable panel rows, revision receipt and load-floor rows), and one update commits the delta that `plan` already computes as one transaction.
- Per-file rows over one whole-state blob. `plan` already diffs exactly these
  units and skips unchanged topic metadata by byte comparison
  (`folders.go:742-895`), so the transaction holds only the changed rows
  (rule 2). A blob rewrites O(board) pages per update. Exact columns and
  indexes are left to Phase 1 under this mapping.

(1810) [open] [technical] Pragmas beyond the Decisions: `wal_autocheckpoint=0`; a PASSIVE checkpoint runs at board close and from a background trigger only after the board has been idle and the WAL has passed a size threshold; `cache_size` is a fixed small value (proposal 256 KiB).
- Explicit off-path checkpoints over SQLite's default auto-checkpoint. The
  default checkpoints inside the commit that crosses 1000 pages, which puts it
  on the update's response path (rule 3, rule 5). The idle and size
  thresholds are chosen from Phase 1 measurement and recorded in its Result.
  The board is served from memory (1811), so the page cache only serves writes
  and the open-time load.

(1811) [open] [technical] An open board is held in memory as the whole `State`, as `FolderCache` holds it today, and the DB is read only at open/import.
- Whole board in memory over a recent window plus DB history reads. This
  matches today's footprint (`folders_cache.go:58-63` caches the full State,
  history included), leaves the query and render code unchanged, and makes
  every read memory-only (rule 1). The DB keeps full history, so windowing can
  come later without a schema change.

(1812) [open] [technical] Derived `index.html` / `index.stamp.js` publication moves off the `whiteboard.update` response path to a per-board coalescing writer (latest state wins, stamp after page). A publish failure becomes a debug event and is retried by the next publish.
- Async publication over the current publish under the board lock before
  returning (`mcp/session_state.go:665-675`, `whiteboard_tools.go:785-812`).
  Rule 5 allows only the WAL append on that path. The update result keeps its
  `full:` line. The "board committed; fix the output problem" error
  (`whiteboard_tools.go:788`) is no longer returned synchronously.

(1813) [open] [technical] Zip export and `whiteboard.load` keep the legacy folder layout: export builds folder files from memory, load accepts legacy-layout zips and imports them into a fresh board DB, and the load backup is written in the same legacy layout.
- Legacy layout over shipping the `.sqlite` file. Old and new binaries trade
  zips in both directions: old load requires `meta.json` and `committed.json` at
  the zip root (`folders_archive.go:194-196`), so both zip directions stay T0.
  A held WAL DB cannot be copied consistently without the backup API, and the
  zip stays inspectable.

(1814) [open] [technical] Stat-based change detection of the board (the dashboard page watch on `meta.json`/`committed.json`, nudge targeting stats, `FolderCache` root stamps, the topic memo) is replaced by in-process notification from the board store.
- In-process notification over stat polling of a DB file. Once (1806) holds,
  the daemon is the only writer, so stats would be per-call file touches with
  nothing new to detect (rule 4). The existing hooks are
  `whiteboardChanged` (`whiteboard_tools.go:340-342`) and the nudge and watch
  sites above.

(1815) [open] [technical] This ticket takes `blocked-by: 261119-feat-ws-whiteboard-retire-embed-snapshot`, so the embed render is gone before the store switch.
- Sequencing over carrying embed state (`Embeds`, `LastEmbedSeq`, `EmbedDir`)
  and the synchronous embed render through the new store, only to delete them.
  The per-update embed render is synchronous derived output that rule 5
  forbids on the update path.

(1816) [open] [policy] When, if ever, the imported legacy board folders (`meta.json`, `committed.json`, `tN/`, revision slots) are deleted.
- Context: Phases 1-3 never delete them, per (1804). They then serve as rollback
  data, and under (1805)a they also hold the sentinel that refuses old
  binaries. Deleting them removes the old binary's refusal path and the raw
  import source.
- Alternative a: never delete automatically. A later ticket may add an explicit
  cleanup command.
- Alternative b: delete in a later minor release, after the SQLite store has
  shipped one minor.
- Alternative c: delete right after a verified import.
> Recommend a for this ticket (Phase 4 stays gated). Revisit as b once the
> store has shipped and the owner wants the disk back.

## Phases

### Phase 1: Board DB store, import, and legacy-layout export (not wired)

Build the per-board SQLite store as a new type beside `FolderStore`. Nothing
in the daemon uses it yet.

- Open with the settled pragmas (Decisions; (1807), (1810)), the schema
  ((1809)), and the in-memory whole-State model ((1811)).
- Commit path: one update is one transaction holding only changed rows. An
  unchanged state opens no transaction.
- Import: legacy folder -> DB through `FolderStore.Open`, recorded so it runs
  once. The legacy folder stays untouched apart from what (1805) settles.
- Legacy-layout materializer for export and backup ((1813)).
- Capture the "before" measurement on the base for the scenario above
  (macOS tracer and/or extended Go counters), replacing the derived table.

Verification:

- Round-trip import of fixture folders, including pending-intent replay, v1
  conversion, revision slots, and the load floor.
- An unchanged update writes zero WAL frames.
- A torn or killed transaction recovers to the last commit.
- Materialized export equals the source folder's file set and bytes under
  `boardFiles` rules.

Deferred: wiring, compatibility sentinel, measurement of "after".

### Phase 2: Switch board authority to the DB

Depends on Phase 1.

Route every writer and reader listed in `### Current store inventory` through
the board DB store:

- `whiteboard.update`, render-only refresh, dashboard answer revision, board
  notice, `whiteboard.query` and lazy history, the served page, nudges, zip
  export, and `whiteboard.load` with its backup.
- In-process change notification ((1814)).
- Handle lifetime per `(1347)=c` (long idle close, reopen on next access).
- Direct-CLI routing or refusal per (1806).
- The old-binary mechanism per (1805).

Verification:

- The old-binary compatibility harness (`wswhiteboard/testdata/compat-old`)
  exercises the settled (1805) behavior: an old store never silently writes a
  migrated board.
- Zips cross both directions between old and new.
- A second process's open fails with the (1806) error.

Deferred: async derived publish, measured report.

### Phase 3: Update-path latency and measured result

Depends on Phase 2 and (1815).

- Move derived page publication off the response path ((1812)).
- Measure "after" with the same method as Phase 1: file operations per
  `whiteboard.update`, fsync count, and update latency on macOS.
- Report before/after in the Result. Record Windows V3 measurement as
  deferred.
- Any remaining rule 1-5 gap is listed with its measured cost for the owner
  (rule 8).

### Phase 4: Legacy folder cleanup [gated]

Gated on (1816). It does nothing unless the owner chooses b or c there.
