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

## Live Reproduction (2026-07-25)

Reproduced with direct latency measurement against the Windows :4300 dogfood
daemon (`D:\dbg-ws-terminal-dogfood`, serving the WSL work roots), while a
user reported a persistent red `Refresh failed` overlay in the browser.

Measured `GET /api/dashboard/resources` from the daemon host (PowerShell
`Invoke-WebRequest`, wall-clock):

- Single call, warm: ~2.4 s.
- Single call, steady state: **13-14 s**.
- `resources` + `servers` issued **concurrently** (exactly what the 5 s
  frontend poll does via `loadServers()` + `loadResources("poll")`):
  **21-24 s**, with one round hard-timing-out at 12 s.

A single request already exceeds the client's 8 s budget (13-14 s), and
concurrency compounds it to ~22 s.

### Hypotheses #1 and #2 are contradicted by the code (2026-07-25 source audit)

The original Suspected Root Cause (blocking git on the async executor, or a
coarse lock held across the git op) does NOT match the current source:

- **#1 (executor starvation) — disproven.** The resources handler already
  offloads: `resources.rs` `local_dashboard_resources_view` wraps the entire
  git/fs sync in `tokio::task::spawn_blocking`, with an explicit comment
  ("Live discovery runs synchronous filesystem and `git` subprocess work, so
  keep it off the async worker threads"). Every git call site in the poll path
  (`discovery.rs`, `work_root_activity.rs`, `git_toolbar.rs`) sits inside a
  synchronous `fn` reached only through a `spawn_blocking`. This invariant is
  already codified in the `ws-web-dashboard` mental model (Coupling + "Change
  live resource discovery or refresh"). It is a real design rule the codebase
  follows here, and it was hardened reactively by prior bugs
  (`260710-bug-dashboard-root-picker-blocking-git-spawn`,
  `260723-bug-dashboard-terminal-blocking-pty-write-thread-starvation`).
- **#2 (coarse lock serializing) — largely disproven for the read path.**
  `OpenedWorkRoots.roots` is an `Arc<RwLock<..>>` and the resources path is a
  reader, so two concurrent `GET /resources` take concurrent read guards and
  do not serialize on it.

### Revised leading cause

The latency is the aggregate cost of spawning many `git` subprocesses (one or
more per known work root: `rev-parse --show-toplevel`, `worktree list
--porcelain`, activity `git_output`) on this environment's slow filesystem,
per single request — not executor starvation. Concurrency compounds it,
plausibly via `.git/index.lock` contention (already tracked as
`260711-idea-dashboard-git-status-polling-index-lock-contention`); note the
lock-free-git-form rule (Module Contract, mental model) is currently scoped to
the `git_toolbar.rs` poll only, not the resources discovery/activity git calls.
The real fixes therefore live elsewhere than this ticket's original framing:
bound/parallelize the per-work-root git fan-out, extend the lock-free-form rule
to the discovery/activity path, and/or the daemon-side timeout
(`260724-idea`). This ticket's "blocks the whole async API surface" framing
from the 2026-07-22 clone/refresh observation may still hold for a genuinely
long *mutating* git op (clone/worktree-add) that is NOT on the read poll path;
that narrower claim is not disproven here and remains the residual scope.

### Downstream symptom (why this surfaced)

This latency is the root cause of the user-visible persistent red
`Refresh failed` / `signal is aborted without reason` overlay:

- The resources endpoint consistently exceeds the frontend's hardcoded
  `DEFAULT_FETCH_TIMEOUT_MS = 8_000` (`ws-dashboard/frontend/src/fetchWithTimeout.ts`).
- Each 5 s poll's fetch therefore aborts at 8 s with a reason-less
  `AbortController.abort()`, producing the DOMException string
  `signal is aborted without reason`.
- `createResourceRefreshCoordinator` (`resourceRefresh.ts`) catches it and
  `setError(message)`; `App.tsx` renders it as the red
  `InlineNotice tone="error" title="Refresh failed"` banner.
- Because the backend never beats 8 s in this environment, no poll ever
  reaches the `setError(null)` success path, so the overlay is permanent
  rather than a transient flicker. It is a symptom of this daemon-side bug,
  not a frontend regression (unrelated to the 2026-07-25 agent-GUI
  suspension, which does not touch the resources/servers/timeout path).

Client-side bounding already exists (parent
`260724-bug-dashboard-git-diff-index-lock-stuck-activity-badge` Phase 2); the
daemon-side bound is tracked in
`260724-idea-dashboard-daemon-side-git-poll-response-timeout`. This ticket
remains the root-cause fix (isolate the git op so it stops blocking the API
surface); the timeout tickets only bound the symptom.

## Reporter Context

Observed during live dogfooding on 2026-07-22. Reproduced with latency
measurement on 2026-07-25 (see Live Reproduction above); still not profiled at
the source level (which call site holds the executor thread / lock is not yet
pinned).
