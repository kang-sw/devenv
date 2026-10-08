---
title: "ws git.merge switches a root that a worker occupies on an impl/* branch"
---

# ws git.merge switches a root that a worker occupies on an impl/* branch

## Observation (dogfood, 2026-10-08)

- A Pi lead session ran a worker directly in the main checkout
  (`/home/swkang/devenv`), which switched it to `impl/develop/catty-hertz-trout`
  at 13:34:15 with uncommitted edits in progress.
- A separate Claude lead session, whose last knowledge of that checkout was
  "on develop", called `ws/git.merge(branch: impl/develop/pi-summary-title,
  target: develop)`. The tool switched the shared checkout from the worker's
  impl branch to `develop` (reflog 13:36:29), then the merge itself failed on
  the overlapping dirty files. The checkout was left on `develop` with the
  worker's uncommitted edits carried along until someone switched back at
  13:39:27. No commit landed in the window, so no history damage occurred.
- The workflow manual already says a root on `impl/*` is the worker's until
  the lead restores it, but nothing enforced it at the merge call.

## Expected

`git.merge` should refuse (before any switch) when the caller's checkout is on
an `impl/*` branch other than the merge source — or at least when that branch
is leased to / occupied by another session — with a diagnostic naming the
occupant and pointing at a pooled worktree. A failed merge should also not
leave the checkout switched.

## Open questions

- Should the guard key off the `impl/*` name alone, or a lease/occupancy
  record (the occupied-root banner's source)?
- Should `git.merge` take an `expected_branch`-style guard like `git.commit`?
