---
title: "Windows CI internal/mcp shards fail intermittently on code that passed the previous run"
---

# Windows CI internal/mcp shards fail intermittently

## Observation

`ws-mcp CI` Windows `internal/mcp` shards fail on commits whose Go code is
identical to an earlier green run, which blocks the ship CI gate until a rerun.

- Run 39795435120 (b8f6908bf, 2026-10-22), shard `mcp-2`:
  `TestReplayedTakeoverPrintsWarning` (ticket_index_test.go:1334) - the
  flushing `tickets.move` output lacked "ticket-index: replayed the offline
  acquire of ...". The only Go delta since the green run on b024aabe1 was the
  version string in `cmd/ws-mcp/main.go`.
- Run 39721167198 (21806a7d0, 2026-10-22), shard `mcp-1`:
  `TestMailboxRecvConcurrentCallsNeverDoubleDeliver`.
- Runs 39720636638 and 39719908503 failed the same day; their cause is not yet
  inspected.

## Questions

- Is the replayed-takeover warning lost to a timing window in the pending-log
  flush on Windows (slower file I/O, 7s test), or to test-side ordering?
- Are these the same root cause, or independent timing-sensitive tests?
- The run-level conclusion turned `failure` while three jobs still read
  `in_progress` through the API; `gh run watch --exit-status` exited at that
  point. Ship's CI gate reads job conclusions, so it should wait for every job
  to settle before judging.
