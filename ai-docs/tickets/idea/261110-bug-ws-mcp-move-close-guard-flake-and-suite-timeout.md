---
title: "internal/mcp: TestMoveCloseGuard fails under full-suite load, and the suite overruns go test's default 10m timeout"
---

# TestMoveCloseGuard flake and internal/mcp suite timeout

## Background

Seen 2026-11-10 by the worker for 261106-bug-sage-stamp-missing-restamp-loop
(branch impl/develop/late-twice-blob, merged f1f8c86a2):

- `go test ./internal/mcp/... -count=1 -timeout 900s` failed only
  `TestMoveCloseGuard` (`internal/mcp/ticket_index_view_test.go:478`, the
  ticket-index lease warning). Run on its own with `-count=3`, it passes, so
  it fails only under full-suite load. The sage change did not touch it.
- The full `internal/mcp` suite takes about 760s, so `go test`'s default 10m
  timeout kills it unless the caller raises `-timeout`.

Several parallel workers were building and testing in worktrees at the same
time, which may have added to the load.

## Open questions

- Is the flake a timing assumption in the lease-warning path (an index cache
  TTL, a clock, a subprocess deadline), or contention on a shared resource?
- Should the suite be split or sped up, or should the documented verification
  command carry an explicit `-timeout`?

## Investigation (2026-11-13)

The guard's ownership read uses the index's 1.5-second default read timeout;
a timed-out first read can produce an absent or unknown view. This is a
possible load-sensitive precondition, not a confirmed cause of the reported
failure. On w5090 three separate `TestMoveCloseGuard` runs with `-count=3`
passed; the full suite also passed with `-timeout 900s` and four known tests
excluded, with `internal/mcp` taking 114 seconds. Neither symptom reproduced,
and no clear cause was established, so no fix or suite-timeout change was made.
