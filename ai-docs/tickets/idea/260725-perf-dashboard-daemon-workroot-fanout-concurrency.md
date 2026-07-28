---
title: Daemon per-work-root filesystem/git fan-out is serial, so resource discovery latency scales with work-root count and starves overall responsiveness on slow filesystems
related:
  260722-bug-dashboard-daemon-git-ops-block-api: root-cause record; its revised leading cause is exactly this serial fan-out cost. This ticket is the improvement direction.
  260711-idea-dashboard-git-status-polling-index-lock-contention: concurrent git status polls contend on .git/index.lock; parallelizing fan-out must not worsen lock contention.
  260724-idea-dashboard-daemon-side-git-poll-response-timeout: daemon-side bound; complementary symptom bound, not a latency fix.
  260710-perf-dashboard-git-discovery-combined-rev-parse: prior fan-out reduction (three git spawns -> one combined rev-parse per root); same optimization axis.
related-mental-model:
  - ws-web-dashboard
---

# Daemon work-root filesystem/git fan-out concurrency

## Background

Live measurement on the Windows :4300 dogfood daemon (2026-07-25, recorded in
`260722`): `GET /api/dashboard/resources` takes ~13-14 s for a single request
and ~21-24 s under the frontend's concurrent resources+servers poll, versus the
client's 8 s fetch timeout — which is why the persistent red `Refresh failed`
overlay appears. The resources handler is already correctly offloaded via
`spawn_blocking` (not executor starvation), so the latency is the aggregate
cost of the per-work-root filesystem + `git` subprocess fan-out on a slow
filesystem.

Owner direction (2026-07-25): overall daemon responsiveness is degraded, not
just this one endpoint, so filesystem access needs more concurrency rather than
a serial per-root walk.

## Idea

Make the per-work-root discovery/activity fan-out concurrent instead of serial,
so latency scales with the slowest single root rather than the sum across all
roots. Candidate directions (not yet decided):

- Parallelize the per-root `git`/`fs` probes inside the existing
  `spawn_blocking` (bounded worker pool / `rayon` / a bounded set of blocking
  tasks), keeping the whole batch off the async worker threads.
- Cache/memoize per-root activity and git metadata with a short TTL so a 5 s
  poll does not re-spawn the full git fan-out every tick when nothing changed.
- Reduce per-root spawn count further (continue the `260710` combined-rev-parse
  direction; batch or long-lived `git` where feasible).

## Constraints / open questions

- Must not worsen `.git/index.lock` contention (`260711`): more concurrent git
  invocations against the same worktrees can increase lock pressure. Pair with
  the lock-free-form rule now generalized in the `ws-web-dashboard` Domain Rules
  (`--no-optional-locks`, `diff-index` plumbing on all poll-path git).
- Concurrency degree, cancellation on stale/superseded polls, and whether the
  resources poll and the git-toolbar/activity polls should share a bounded
  work-root probe pool are open and belong to an implementation-ready pass.
- Whether the slow filesystem itself (this daemon reaching its work roots) is a
  separate environmental factor to isolate before optimizing fan-out.
