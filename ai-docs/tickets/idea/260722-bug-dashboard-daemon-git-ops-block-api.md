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

The concurrency signature is the key evidence: two simultaneous read requests
balloon from ~2.4 s to ~22 s (a ~10x blowup from just 2 in-flight requests),
which is the fingerprint of hypothesis #1 (a blocking git child starving the
tokio worker pool) and/or hypothesis #2 (a coarse registry lock held across
the whole git op serializing every other request). A pure per-request slowness
would add, not multiply.

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
