---
title: "ws-dashboard daemon: long-running git ops (clone/refresh) block the whole async API surface"
---

## Symptom

During live dogfooding on 2026-07-22, a worktree refresh triggered a slow git
operation (a fresh worktree/repo clone that took a long time to complete).
While that single git op was running, the *entire* ws-dashboard daemon API
surface appeared to stall: the frontend UI stayed partially alive (it
"twitches" — some client-side rendering/interaction continues) but backend
data-fetch endpoints failed or hung until the slow git op finished. A single
long-running git operation appears to starve or block the whole server, not
just the request that triggered it.

## Suspected Root Cause (hypothesis, not confirmed)

Two non-exclusive candidates:

1. A blocking synchronous git invocation (e.g. `std::process::Command`) is
   being run directly on an async executor thread inside an `async fn`,
   without `tokio::task::spawn_blocking` (or equivalent isolation) — this
   would starve the tokio runtime's worker threads and stall all other
   in-flight async work.
2. A coarse lock (e.g. a global mutex guarding the resource/worktree
   registry view) is held across the *entire* duration of the slow git op,
   rather than only across the in-memory mutation step — this would
   serialize every other request behind the lock for as long as the git op
   runs.

Either mechanism (or both together) would explain the observed symptom: the
long git op is not properly isolated from the request-serving path.

## Investigation Pointers (suggested follow-ups, not conclusions)

- Audit git-invoking call sites (clone, refresh, discovery) in the daemon
  for synchronous `std::process`/blocking calls made directly inside
  `async fn` without `spawn_blocking`.
- Check whether a shared lock guarding registry/resource state is held
  across the full git op instead of being scoped narrowly around only the
  in-memory mutation.
- Confirm the tokio runtime's worker-thread configuration (multi-thread vs.
  current-thread, worker count) and whether long-running git ops should be
  moved onto a dedicated blocking thread pool.

## Relation to Other Work

Adjacent to but distinct from the recently added `git worktree remove` op
work on ticket `260525` (whichever stem currently owns that scope) — that
work concerns worktree removal semantics; this ticket concerns clone/refresh
latency blocking the API, which is a broader async-hygiene concern across
the daemon's git surface generally, not limited to one operation.

## Reporter Context

Observed during live dogfooding on 2026-07-22. Not yet reproduced under
controlled conditions or profiled; this ticket captures the observation and
hypothesis for future investigation/triage.
