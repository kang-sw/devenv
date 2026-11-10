---
title: Session record writes race across MCP processes and fail on Windows
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 2baace36e8a09149
sage-review-completeness-reviewed: 2baace36e8a09149
---

# Session record writes race across MCP processes and fail on Windows

## Background

Downstream dogfood report (2026-10-07): on Windows, two `agenda.set` calls on
different agenda keys of one session failed with a rename conflict.

A session's state (agenda blobs, todos, whiteboard, and the key's identity,
overrides, and child note) is no longer one file. The record is split into
`<cache>/sessions/<key>/{key.json, agenda.json, todos.json, whiteboard.json}`
(`internal/mcp/session_layout.go`; a key not yet migrated is still served from
the older `keys/<key>.json`). A mutation rewrites only the parts it changes, so
writes to different agenda keys still replace the same `agenda.json`, while an
agenda write and a todo write no longer replace each other's file. The writers
serialize only in process: `mutateRecord` / `mutateRecordParts`
(`agents-plugin-tool/internal/mcp/session_state.go`) and `setOverride` /
`deleteOverride` / `setReviewTrackNudgeShown`
(`internal/mcp/session_auth.go`, all three through `mutateRecord`; `setNote`
no longer exists since the `session.note` writer was retired in 9d4d20439,
`internal/mcp/session_auth.go#L61-L65`) take `s.mu`, read the parts, modify them,
and write them back through `writeParts` and `writeFileAtomic`. More than
one MCP process can hold the same key (for example a Pi execute-worker
spawned with the lead's key), and nothing serializes them. Two effects:

- **Lost update, every OS.** Two processes read the same part file, each
  applies its own change, and the later rename silently drops the earlier
  change.
- **Write failure, Windows.** `writeFileAtomic` replaces the file with a
  plain `os.Rename`. On Windows that fails with a sharing violation while
  another process has the file open, for example mid-read. The repository
  already handles this elsewhere: `wsstate.atomicReplaceFile`
  (`internal/wsstate/replace_file_windows.go`) retries `MoveFileEx` on
  sharing violations.

`enterMode` (the write behind every `route.resolve_*` tool) now writes the
agenda and the todo list as two part files, `agenda.json` then `todos.json`,
where it was one rename of one record. Cross-process atomicity of a mode
switch across those two parts is therefore weaker than before: another
process can read the new agenda with the old todos between the two renames,
and a crash between them leaves that state on disk. The in-process `s.mu`
still hides the gap from readers in the same process. The lock below does not
close that reader window; it only serializes writers.

## Decisions

- **Serialize every read-modify-write of a session record across
  processes** with a file lock in the key's directory
  (`sessions/<key>/`, beside `key.json`),
  following `wsmailbox.WithLock` (`internal/wsmailbox/store.go`, `flock` with
  a bounded timeout). The lock covers the whole read-modify-write: reading
  under the lock and writing after it would still lose updates. The in-process
  `s.mu` may stay as is.
- **Replace the record file through the shared Windows-safe replace**
  (`wsstate.atomicReplaceFile` or an equivalent exported helper), not plain
  `os.Rename`. Export the existing helper (rename it to
  `wsstate.AtomicReplaceFile` in both `replace_file_windows.go` and
  `replace_file_unix.go`, updating its in-package callers) rather than adding
  a second wrapper. One implementation keeps the Windows retry logic in one
  place. Rejected: a new exported wrapper beside the unexported helper.
- **A failed write leaves the previous record intact.** Keep the temp-file
  write and replace order, and return the call's success only after the
  replace has succeeded. A record is several part files, so this holds per
  part file: a failed replace leaves that part's previous content, and a
  multi-part write such as `enterMode` may leave earlier parts already
  replaced (the reader window the Background describes).
- Out of scope: revisions, compare-and-swap, multi-operation transactions,
  and delta reads from the same report. With one lead per key, the lock
  removes the race that motivated them.

## Constraints

- Every writer of a session record's content goes through the locked path,
  `mint` included. The mtime-only `touch` (`os.Chtimes`) does not rewrite
  content and may stay outside it.
- A lock timeout is an explicit error from the tool, never a silent skip.
- Lock placement details are the implementer's call within these limits:
  the claim writers (`claimKey` in `mint`, and `AgentKeyStore.Ensure` in
  `internal/mcp/agent_keys.go`) take the lock only after the `os.Mkdir`
  claim succeeds, since a lock file created first would make every mint
  candidate look taken; a write to a legacy `keys/<key>.json` record creates
  `sessions/<key>/` for the lock only after confirming the record exists, so
  a write to an unknown key leaves no empty directory. The lock file name and
  timeout follow `wsmailbox.WithLock` unless a test needs them injectable.
- In-package callers of the renamed `wsstate` helper include
  `internal/wsstate/paths.go` (`replaceFile`) and
  `replace_file_windows_test.go`.
- Paths without a prefix are relative to `agents-plugin-tool/`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/ws-mcp.md (declared for agents-plugin-tool/internal/mcp/)

## Prior Decisions

- 8f8f7967 (2026-10-07, commit): "Downstream proposal reviewed against code: its Windows rename conflict traces to session records being one file per key, serialized only by the in-process mutex and replaced with plain os.Rename" — bearing: supports
- 8f8f7967 (2026-10-07, commit): "the proposal's revision/CAS/delta/report-ack layers were judged unnecessary once the lock removes the race" — bearing: constrains
- 2b28eb21 (2026-11-08, commit): "Background now describes sessions/<key>/{key,agenda,todos,whiteboard}.json, writeFileAtomic, and notes enterMode writes agenda and todos as two files (weaker cross-process atomicity; the lock serializes writers only)" — bearing: supports
- 3f39e18a (2026-11-08, commit): "Rejected an O_EXCL lock-file claim: portable but changes the on-disk protocol for every platform and leaves a stuck entry after a crash; an exclusive handle is released by the OS when the process dies." — bearing: constrains
- 260929-bug-config-tune-agents-tier-lost-update (2026-09-29, Decisions): "One lock for every writer of a config file. The agents.tier set and unset paths hold the same sibling <config path>.lock gofrs/flock lock ... across load/modify/write" — bearing: supports
- 261106-feat-mailbox-history-ring (2026-11-06, Decisions): "Ring storage changed from the session record to one file per entry under <cache root>/mailbox-history/<session-key>/ ... so it does not wait on 261007-bug-session-record-cross-process-write-race" — bearing: constrains
- 260730-refactor-retire-goal-fan-out-step-and-session-note (2026-11-08, commit 9d4d2043): "session.note tool, handler, write path, and runtime gates removed (sessionRecord.Note kept because session.children still reads it)" — bearing: constrains
- 260617-refactor-mcp-stateless-subagent-context (2026-06-17, Decisions): "Same-file writes use atomic temp-write + rename; sharding by session id removes cross-session write contention (no single-file lock)." — bearing: supports

## Route Facts

| fact | allowed values | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/mcp/session_state.go, internal/mcp/session_layout.go, internal/mcp/session_auth.go mint, internal/wsstate replace helper export |
| scope.surface | cross-module | internal/mcp session store plus a new or exported helper in internal/wsstate; no MCP tool schema change |
| scope.new_public_symbol | yes | wsstate.atomicReplaceFile renamed to exported wsstate.AtomicReplaceFile (Decisions, replace bullet) |
| scope.new_type_contract | no | no tool signature changes; a lock file in sessions/<key>/ is an on-disk addition only |
| scope.test_surface | existing | agents-plugin-tool/internal/mcp/session_state_test.go, session_layout_test.go, session_auth_test.go |
| complexity.reuse_points | confirmed | wsmailbox.WithLock flock with LockTimeout internal/wsmailbox/store.go#L102-L206; wsstate.atomicReplaceFile internal/wsstate/replace_file_windows.go#L16 |
| complexity.side_effect_risk | moderate | every session-state tool write path gains a cross-process lock and a timeout error mode |
| risk.correctness | high | lock must span loadRecord through writeParts and migrateRecord, cover mint claimKey, and keep failed-replace record intact |
| risk.fit | moderate | lock must coexist with in-process s.mu and the legacy keys/ migration path |
| risk.test | moderate | multi-process or multi-store concurrency and a held-lock timeout test are timing sensitive; Windows replace path not exercisable on Linux CI |
| risk.security_or_contract | moderate | a lock timeout becomes a new explicit tool error on all session-state tools |

## Phases

### Phase 1: Locked session record read-modify-write

Route every session-record mutation through one locked read-modify-write and
the Windows-safe replace. Test:

- concurrent writers in separate processes (or separate `sessionStore`
  instances over one sessions directory) changing different agenda keys and
  todos lose no update;
- a failed replace leaves the previous record readable and unchanged;
- a held lock past the timeout returns an explicit error;
- existing session-state tests stay green.
