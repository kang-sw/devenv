---
title: Dashboard stream re-read reloads the whole whiteboard folder per tick
---

# Dashboard stream re-read reloads the whole whiteboard folder per tick

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
  about 112 topics pays this on every websocket stream tick, per open page.
- Server TTFB at the same time: shell `/s/<id>` 0.34-0.47s (1.47MB), frame
  `/s/<id>?frame=1` 0.17-0.21s (9.7MB).

The path dates from the folder-store work (972766abc and later), not from
the 2026-11-15 suspects. Raw profiles: `/tmp/exec5/perf/shell.pprof`,
`/tmp/exec5/perf/idle-83992.pprof` (machine-local, may be gone).

Open questions for promotion: whether to cache the loaded board keyed by
folder change (mtime/generation or the existing Notify path), or to skip the
whiteboard part when the stream only needs non-board regions; and the
compatibility tier (expected T0, in-process only).

## Phases

### Phase 1: Avoid full whiteboard folder reloads on stream re-reads

Make a stream re-read that finds the session board unchanged reuse the
loaded board instead of re-opening every topic file. Verify with a
before/after idle daemon CPU profile and the existing whiteboard folder-store
tests.
