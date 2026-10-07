---
title: Session record writes race across MCP processes and fail on Windows
---

# Session record writes race across MCP processes and fail on Windows

## Background

Downstream dogfood report (2026-10-07): on Windows, two `agenda.set` calls on
different agenda keys of one session failed with a rename conflict.

Every piece of a session's state (agenda blobs, todos, overrides, the child
note) lives in one file, `keys/<session-key>.json`, so writes to different
agenda keys still replace the same file. The writers serialize only in
process: `mutateRecord` (`agents-plugin-tool/internal/mcp/session_state.go`)
and `setOverride` / `deleteOverride` / `setNote` /
`setReviewTrackNudgeShown` (`internal/mcp/session_auth.go`) take `s.mu`, read
the record, modify it, and write it back through `writeRecordAtomic`. More
than one MCP process can hold the same key (for example a Pi execute-worker
spawned with the lead's key), and nothing serializes them. Two effects:

- **Lost update, every OS.** Two processes read the same record, each
  applies its own change, and the later rename silently drops the earlier
  change.
- **Write failure, Windows.** `writeRecordAtomic` replaces the file with a
  plain `os.Rename`. On Windows that fails with a sharing violation while
  another process has the file open, for example mid-read. The repository
  already handles this elsewhere: `wsstate.atomicReplaceFile`
  (`internal/wsstate/replace_file_windows.go`) retries `MoveFileEx` on
  sharing violations.

## Decisions

- **Serialize every read-modify-write of a session record across
  processes** with a file lock beside the record (`<session-key>.json.lock`),
  following `wsmailbox.WithLock` (`internal/wsmailbox/store.go`, `flock` with
  a bounded timeout). The lock covers the whole read-modify-write: reading
  under the lock and writing after it would still lose updates. The in-process
  `s.mu` may stay as is.
- **Replace the record file through the shared Windows-safe replace**
  (`wsstate.atomicReplaceFile` or an equivalent exported helper), not plain
  `os.Rename`.
- **A failed write leaves the previous record intact.** Keep the temp-file
  write and replace order, and return the call's success only after the
  replace has succeeded.
- Out of scope: revisions, compare-and-swap, multi-operation transactions,
  and delta reads from the same report. With one lead per key, the lock
  removes the race that motivated them.

## Constraints

- Every writer of a session record's content goes through the locked path,
  `mint` included. The mtime-only `touch` (`os.Chtimes`) does not rewrite
  content and may stay outside it.
- A lock timeout is an explicit error from the tool, never a silent skip.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Phases

### Phase 1: Locked session record read-modify-write

Route every session-record mutation through one locked read-modify-write and
the Windows-safe replace. Test:

- concurrent writers in separate processes (or separate `sessionStore`
  instances over one keys directory) changing different agenda keys and
  todos lose no update;
- a failed replace leaves the previous record readable and unchanged;
- a held lock past the timeout returns an explicit error;
- existing session-state tests stay green.
