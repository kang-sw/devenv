---
title: Dashboard non-board wakes reload unchanged whiteboard folders
---

# Dashboard non-board wakes reload unchanged whiteboard folders

## Background

Found while measuring the 2026-11-15 owner-reported reload/spinner
regression (the client-side causes are fixed separately). The dashboard
daemon burns about 70% of one core with no page request in flight.

Evidence (daemon pid 83992, develop 63e89802b, 2026-11-15, read-only
`/debug/pprof/profile`):

- Idle 6s profile: 4.22s of samples in 6.07s. 61.9% cumulative under
  `(*Daemon).serveWebSocket` -> `(*Daemon).stream.func1` ->
  `(*pageCache).get` -> `sessionPageSource.func1` ->
  `pageReader.readSessionWith`; 47% flat in `syscall.rawsyscalln`, mostly
  `os.OpenFile`.
- With 12 shell requests in an 8s profile: `mcp.loadRecordWith` ->
  `mcp.openWhiteboard` (2.61s) -> `wswhiteboard.FolderStore.Open` (2.27s) ->
  `FolderStore.withLock` -> `FolderStore.load` (1.90s); `FolderStore.guard`
  0.40s; `json.Marshal` of the board 0.34s.
- `openWhiteboard` (`agents-plugin-tool/internal/mcp/whiteboard_store.go:45`)
  calls `folder.Open(record.Whiteboard)` on every record load, which re-opens
  every topic file of the session board under the folder lock. A session with
  about 112 topics pays this on each full stream re-read. Quiet refresh ticks
  can skip; non-board wakes still read the board (see Diagnosis).
- Server TTFB at the same time: shell `/s/<id>` 0.34-0.47s (1.47MB), frame
  `/s/<id>?frame=1` 0.17-0.21s (9.7MB).

The path dates from the folder-store work (972766abc and later), not from
the 2026-11-15 suspects. Raw profiles: `/tmp/exec5/perf/shell.pprof`,
`/tmp/exec5/perf/idle-83992.pprof` (machine-local, may be gone).

## Diagnosis

Source references below are relative to `agents-plugin-tool/internal/`.

- **VERIFIED:** a non-live dependency wake makes `current` false. Even with an
  unchanged refresh stamp, only `current` pages or live-only wakes can skip
  the full read (`wsdashboard/stream_cache.go:176-205`). The source loads the
  board before consulting its rendered-board memo
  (`mcp/dashboard_page.go:51-70,133-153`). This memo avoids HTML work, not folder
  I/O. Both pre-lock and locked schema guards also reopen topic metadata
  (`wswhiteboard/folders.go:187-215,255-302`).
- **VERIFIED:** the refresh stamp does not open the board folder's content:
  `pageReader.sessionStamp` folds revisions/liveness, watched-file metadata,
  mail and contacts (`wsdashboard/pages.go:1019-1062`;
  `wsdashboard/stream_cache.go:256-280`). Ten quiet fixture ticks skip all page
  reads, whereas ten non-live session wakes with the same stamp cause ten
  full reads (`wsdashboard/stream_cache_idle_test.go:8-38`). This is not an
  intrinsically unmatchable stamp.
- **VERIFIED:** a pure snapshot heartbeat is excluded from the fingerprint
  (`wsdashboard/agent_sessions.go:190-207`). Changing non-board snapshot
  content in the isolated 112-topic daemon fixture produces 40 signals and
  41 total session renders, with only one additional board-memo render
  (`mcp/dashboard_folder_cache_test.go:21-119`).
- **VERIFIED:** read-only live counter snapshots showed 33 additional session
  wakes, 20 changed session reads and 21 session renders, with no additional
  refresh skips. A fresh 6.05-second profile sampled 2.32 CPU seconds, including
  0.47 seconds in `FolderStore.Open`. **INFERRED:** persisted live/agent activity
  and other non-board dependencies drive the profiled stream's repeated full
  reads; aggregate counters do not identify the exact live dependency per page.

## Decisions

- **Phase 2 compatibility: T0, VERIFIED.** Only the pending intent's existing
  optional topic-write list becomes sparse. At base `8145ec935`,
  `wswhiteboard/folders.go:685,711-799` writes every topic and publishes the
  session/receipt; its reader/replayer already iterates only supplied topic
  writes (`:306-324,384-481,767-799`). The new planner omits identical bytes
  (`folders.go:685-702`); serialization, schema and publication/recovery shapes
  are unchanged (`:376-382,727-820`). Old readers consume new committed files
  normally and old replay accepts sparse intents. Old writers may rewrite
  unchanged files again but cannot lose new data because no new fields exist.
  A new process reopens old/new stores through the unchanged load/readState
  path; no migration or lost-field recovery is needed.
- **Phase 2 comparison source: exact on-disk bytes, VERIFIED.** Read the current
  topic's metadata with `wsstate.ReadFile` and compare it with `folderJSON(topic)`
  while holding the existing writer flock (`folders.go:149-167,187-215,685-702`).
  This costs one read per stable mapping but avoids assuming that a decoded
  `before` State can reproduce the exact committed bytes. A new/changed
  `(Key, Dir, HasDoc)` mapping always publishes, including an identical orphan
  at a newly assigned path. Non-absence read errors abort planning before any
  intent. Missing files still schedule a write rather than an omission.
- **Phase 2 recovery invariant: VERIFIED.** `load` completes any earlier pending
  decision before planning (`folders.go:306-324`); flock excludes cooperating
  writers through comparison, intent installation and replay (`:149-167,187-215`).
  Every omitted file already contains the exact target bytes at the durable
  decision. The intent retains all session topic refs and changed metadata;
  replay publishes included files before session metadata and receipt
  (`:702,727-820`). An opener replays before `readState` checks mappings,
  close state and log lengths (`:306-324,384-438`), and recovery never re-runs the
  mutation or observer. External edits that bypass flock during a transaction
  remain outside the writer contract. Log append selection is already
  conditional (`:681-684`), including zero-length initialization; leave it intact.
- **Compatibility: T0, VERIFIED.** In-process read caching only; no file shape,
  schema number, MCP tool, argument, protocol, or notification change. At base
  `b6ef094dd`, `wswhiteboard/folders.go:704-797` writes pending intent, topic/log
  data and session metadata; `:299-317,377-474` replays/reads those bytes. The
  corresponding unchanged new paths are `folders.go:711-804,306-324,384-481`.
  Old and new writers therefore publish the same bytes; old readers still read
  new writes normally. A fresh new process has an empty cache and reconstructs
  the same board through `FolderStore.load`; no migration or recovery of lost
  fields is needed. Pending recovery remains that same locked replay path.
- Cache authoritative reads only in `DashboardSessionSource`
  (`mcp/dashboard_sessions.go:683,907`; `mcp/whiteboard_store.go:45-60`). Other
  callers and all mutations retain the uncached boundary. Cache entries are
  bounded to 64 boards, serialized per board, and discarded when the session
  is forgotten; returned states are independent copies.
- Validate every remembered JSON/JSONL file by identity (`os.SameFile`), mtime,
  size and mode, and require pending.json to be absent. Repeat validation under
  the existing flock before using a hit. A changed/pending store runs the
  existing guards and load/replay, with before-read stamps retained only when
  still valid (`wswhiteboard/folders_cache.go:32-129`). This detects external
  writers without a nudge, including replacements with equal timestamps and
  sizes. Linked trees conservatively use uncached reads.
- Reject wake suppression alone: non-board regions still need updates, and a
  changed revision does not prove that the board changed. Reject generation-only
  or Notify-only invalidation: these would hide direct topic edits or miss
  external/crashed writers. Full-file metadata validation preserves existing
  schema/corruption checks while removing content opens on unchanged reads.
- An in-place external edit that preserves identity, size, mode and mtime is
  outside this cache's change signal. Supported folder writers publish atomic
  replacements under flock; filesystem identity catches their writes even
  within a coarse timestamp granule. No cross-process visibility TTL is added.

## Constraints

- Follow `ai-docs/manuals/release-compatibility.md`,
  `ai-docs/manuals/ws-mcp.md`, and
  `ai-docs/manuals/shipped-surface-boundary.md` for the affected tooling paths.
- Do not replace or signal the shared daemon, start a daemon on real state, or
  push. Profiling and tests use scratch state/config roots and an OS-picked port.
- Run only scoped dashboard tests and use offline Go dependency/toolchain flags.

## Phases

### Phase 1: Avoid full whiteboard folder reloads on stream re-reads

Make a stream re-read that finds the session board unchanged reuse the
loaded board instead of re-opening every topic file. Verify with a
before/after idle daemon CPU profile and the existing whiteboard folder-store
tests.

### Result (cecd432ab) - 2026-11-15

- **VERIFIED:** implemented the T0 read cache and moved this ticket to `todo/`
  in `cecd432ab`. Twenty unchanged opens of a 112-topic board read no topic,
  session-metadata, or log content again. Regression coverage observes a
  separate writer handle and a real child-process writer, recovers pending
  intents after partial publication, rejects newer nested schemas, detects
  same-stamp atomic replacements, preserves legacy/missing-metadata behavior,
  and prevents returned-state aliasing
  (`wswhiteboard/folders_cache_test.go:40-220`). The existing dashboard lock
  contention regression still passes; cache hits do not bypass flock.
- **VERIFIED:** from `agents-plugin-tool`, with
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` on every Go command:
  - `go test ./internal/wswhiteboard/ -run 'Folder|Open|Store' -count=1`:
    PASS (41.931 s).
  - `go test -race ./internal/wswhiteboard/ -run '^TestFolderCache' -count=1`:
    PASS (27.907 s).
  - `go test ./internal/wsdashboard/ -run 'TestRefreshCacheNonBoardWake|TestRefreshTick|TestStreamsOfOnePageShareOneRender|TestTransientReadCache|TestStreamingTickChangesOnlyTheLiveRegion' -count=1 -v`:
    PASS, seven tests (14.360 s). The full dashboard package was not run.
  - `go test ./internal/mcp/ -run 'Whiteboard|Dashboard|Record' -count=1`:
    one pre-existing failure,
    `TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation`
    (`mcp/whiteboard_effects_test.go:154`, an extra topic-state event).
    Reproduced the identical failure with the original MCP source via Go's
    read-only overlay. The requested scope with
    `-skip '^TestWhiteboardEffectsKnownCreationBatchAndLegacySeparation$'`
    passes (91.420 s). No assertion was weakened or unrelated code changed.
- **VERIFIED:** paired `BenchmarkFolderIdleOpen112` runs (`-run '^$' -bench
  '^BenchmarkFolderIdleOpen112$' -benchtime=20x -count=3 -benchmem`) give median
  **8,705,075 -> 1,660,854 ns/op** (5.24x faster), and **461 -> 1 open attempts
  per refresh**. The read seam counts content-open attempts, including absent
  pending/receipt reads, plus one flock-file open. Metadata stats are excluded;
  the remaining open is the lock, not a topic file. The uncached arm executes
  the original load/guard path in the same benchmark.
- **VERIFIED:** isolated daemon profiles over approximately 10 seconds, one
  112-topic stream and 40 non-board snapshot wakes at 4 Hz, sample
  **1.24 -> 0.37 CPU seconds** (**11.93% -> 3.58%** of one core). Both runs have
  40 signals, 41 total session renders and one further board-memo render. The
  before arm uses the original MCP source overlay; the after arm uses the cache.
  `TestDashboardFolderIdleCPUFixture` is opt-in through
  `WS_DASHBOARD_IDLE_PROFILE=<output.pprof>` and creates its own state, cache,
  config, home and listener. No real session or shared daemon was modified.
- Raw profiles, counter snapshots, benchmark output, overlay inputs and test
  logs are in `/tmp/exec5/perfC/`. These are machine-local evidence, not shipped
  assets. The shared runtime was not rebuilt/replaced, so a live after-deploy
  CPU reduction is **INFERRED**, not measured. The exact per-page live wake
  origin and the unrelated baseline MCP failure remain unresolved here.

### Phase 2: Skip unchanged topic meta writes

Owner/MAIN-approved follow-up to Phase 1. A live single-board update rewrote
all 112 topic metadata files, amplifying atomic replacements and invalidating
the read cache even for unchanged topics.

Compare each planned topic's exact `folderJSON(topic)` bytes with its existing
committed metadata under the folder lock. Omit byte-identical topic metadata
from both publication and pending intent. New or remapped topics must still
publish metadata; preserve changed documents, items, labels, and close/reopen
state. Keep session metadata and receipt publication, log append conditions,
and durable-decision/replay ordering unchanged. Stop if omission violates a
recovery invariant rather than changing the recovery protocol.

Compatibility target: T0, with no file shape, new file, schema, or MCP change.
Verify a 112-topic single-topic update, recovery after pending/partial
publication, adoption/rename/close/label paths, and scoped folder/race/MCP tests
with offline Go flags. Capture before/after atomic replacement counts and raw
evidence in `/tmp/exec5/perfD/`. Do not touch the shared daemon, run a daemon on
real state, kill processes, push, or run the full dashboard package.
