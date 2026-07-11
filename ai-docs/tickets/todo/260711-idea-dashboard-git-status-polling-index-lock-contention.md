---
title: "Overlapping git status polling risks .git/index.lock contention, especially on Windows"
parent: 260525-feat-ws-dashboard-workroot-polishing-backlog
related:
  260525-feat-ws-dashboard-workroot-polishing-backlog: Phase 2's "status polling load" candidate area this ticket makes concrete
---

# Overlapping git status polling risks .git/index.lock contention, especially on Windows

## Background

Owner-reported dogfood symptom (2026-07-11, Windows): `.git/index.lock`
files are observed to persist. Investigated the daemon/frontend git-status
polling path to find a root-cause hypothesis (see Findings) before
proposing a fix direction.

## Findings

- `frontend/src/gitToolbar.ts:231-253` (`startGitRefreshScheduler`) fires a
  fixed 5s `setInterval`, plus `visibilitychange` and `window focus`
  listeners, all calling the same `refreshGit` callback
  (`App.tsx:5524-5575`).
- `refreshGit` has **no in-flight guard on the request side** — it fires
  `fetchWorkRootGitStatus` + `fetchWorkRootGitBranches` in parallel on
  every invocation. The only existing guard (`requestSeq.current`,
  `App.tsx:5527-5533,5540-5564`) discards *stale responses* client-side; it
  does not prevent a new request from firing while a previous one is still
  pending. An interval tick and a focus/visibility event can trivially
  overlap on a slow request.
- Server-side, `git_status`/`git_branches` (`crates/daemon/src/
  git_toolbar.rs:134-160`) each do a `tokio::task::spawn_blocking` running
  5-8 **serial, blocking** `Command::output()` calls per request
  (`branch --show-current`, `rev-parse`, `rev-list`, `diff --numstat`, and
  critically `status --porcelain` at `git_toolbar.rs:455`). `run_git`
  (`git_toolbar.rs:536-557`) uses plain `.output()` with **no timeout, no
  `.kill()`, no cancellation** — ruling out a forced-kill mid-operation as
  the mechanism (git's own `index.lock` cleanup runs fine on normal exit).
- No per-repo `Mutex`/semaphore anywhere in `git_toolbar.rs`/`router.rs`
  serializes concurrent git invocations against the same repo (the only
  mutex found, `router.rs:75` `registry_persist_lock`, is unrelated).
- Plain `git status` (no `--no-optional-locks` / `GIT_OPTIONAL_LOCKS=0`
  used anywhere) still attempts the index's stat-cache refresh-and-rewrite
  via the standard `index.lock` create-then-rename dance, even though it
  looks read-only from the caller's side. Overlapping polls (5s timer +
  focus-triggered refresh racing on a slow/large repo, or with AV-added
  I/O latency) are the classic trigger for `index.lock: File exists`
  contention; Windows' stricter file-rename/delete-on-open-handle
  semantics make a stray/stuck lock file more visible there than on POSIX,
  matching the owner's Windows-specific report.
- No `notify`/filesystem-watch prior art exists anywhere in the daemon
  today (confirmed via `Cargo.lock`/source search) — all git-state refresh
  is pure interval polling.

## Root-cause hypothesis

Not a forced-kill/cancellation bug (none exists in this path). Most likely:
overlapping, unguarded `git status`/`git branch`/etc. invocations against
the same repo — from the 5s interval timer stacking with focus/visibility-
triggered refreshes — racing on the index lock, with Windows surfacing the
resulting contention/stale-lock symptom more visibly than POSIX would.

## Candidate fix directions (not decided, needs a design pass)

- Add a per-work-root single-flight guard (client-side: skip/coalesce a
  `refreshGit` call already in flight for that work root; and/or
  server-side: a per-repo `Mutex`/semaphore around git invocations) so
  overlapping polls queue or no-op instead of racing.
- Use `--no-optional-locks` / `GIT_OPTIONAL_LOCKS=0` for read-only status
  checks so polling never contends for the index lock in the first place —
  worth doing regardless of the single-flight fix, since it directly
  targets the lock contention rather than just reducing its odds.
- Longer-term: move from fixed-interval polling to a `notify`-crate-based
  watch on `.git/index`, `.git/HEAD`, `.git/refs/**`, invalidating/
  refreshing only on change instead of every 5s. Feasible — the daemon
  already resolves a canonical root path per WorkRoot for other git
  operations — but does not eliminate the need for a single-flight guard:
  a watch still needs to run an actual `git status`-shaped call to compute
  the summary once triggered, and Windows `ReadDirectoryChangesW`-backed
  watchers can surface spurious/duplicate events during git's own
  lock-rename sequence, so debouncing is required either way.

## Decisions

- **Near-term fix** (owner, 2026-07-11): apply `--no-optional-locks`
  (equivalently, `GIT_OPTIONAL_LOCKS=0` on the subprocess environment) to
  the specific polling invocations that touch the index while looking
  read-only — `status --porcelain` (`git_toolbar.rs:455`) and `diff
  --numstat`. Judged the best near-term cost/benefit: it removes the
  actual lock-taking behavior from routine polling directly, rather than
  just narrowing the collision window like a queue/guard would. No
  correctness risk identified: the only cost is that git skips writing
  back its refreshed index stat-cache on these specific calls, so each
  poll may re-stat working-tree files instead of reusing a cached mtime
  snapshot — a minor, bounded perf cost, not a correctness concern, and it
  does not touch any real git operation a human runs (commit, add, etc.),
  since those are separate invocations outside the polling path. Calls
  that never touch the index (`branch --show-current`, `rev-parse`,
  `rev-list`) don't need the flag and are unaffected.
- **Long-term direction** (owner, 2026-07-11): the `notify`-crate-based
  watch approach (see Findings/Candidate fix directions above) is the
  target architecture — replace fixed-interval polling with
  change-triggered refresh on `.git/index`/`.git/HEAD`/`.git/refs/**`.
  Not scheduled yet; the near-term flag fix ships first since it is
  independent, small, and lower-risk, and does not block or get
  superseded by the later watch-based work (the watch-triggered refresh
  would still want `--no-optional-locks` on its own status calls).
- The per-repo single-flight guard (candidate direction 1) is no longer
  the primary target now that the near-term fix removes the lock-taking
  mechanism directly rather than just reducing collision odds. It may
  still be worth revisiting later purely to cut redundant polling load
  (CPU/subprocess churn), but that is a performance concern distinct from
  the lock-contention bug this ticket was filed for, and is not an
  immediate priority.

## Non-Goals

- Scheduling or scoping the long-term watch-based rearchitecture as its
  own implementation-ready ticket — tracked here as the accepted target
  direction only; a follow-up ticket should pick this up when prioritized.
- Revisiting the single-flight guard as a load-reduction optimization —
  separate concern, not blocking the near-term fix.
