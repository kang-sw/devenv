---
title: "Avoid re-discovering every opened work root on every open-work-root call"
parent: 260710-epic-ws-dashboard-terminal-ux-polishing
related:
  260710-bug-dashboard-root-picker-blocking-git-spawn: fixed blocking-on-async-runtime for this same call cluster
  260710-perf-dashboard-git-discovery-combined-rev-parse: fixed per-work-root spawn count (4 -> 2); this idea addresses the remaining O(N) factor
---

# Avoid re-discovering every opened work root on every open-work-root call

## Background

After two sibling perf fixes (spawn_blocking dispatch, and combining the
three `rev-parse` queries into one), live-measured on the native-Windows
daemon (2026-07-10):

- `GET /api/dashboard/resources` with N=4 registered work roots: ~619ms.
- `POST /api/dashboard/work-roots/open` with the same N=4 registered work
  roots: still ~1.5-2.3s.

The remaining gap is structural, not a spawn-count bug:
`open_work_root`, `set_work_root_activation`, and `remove_workspace`
(`ws-dashboard/crates/daemon/src/root_picker.rs`) each call
`resources::local_dashboard_resources_view(&state)`, which re-runs full git
discovery for *every currently opened work root*, not just the one being
mutated. So every single open/activation/removal call costs O(N) git
discovery work, and this cost grows linearly as the user opens more folders
in a session -- a session with 10 opened work roots will see ~2.5x the
per-call latency of one with 4, on every call, forever, even when nothing
about the other 9 work roots changed.

The CONTRACT comment in `open_work_root` explains *why* the full aggregated
view is returned (so the immediate response matches later
`GET /api/dashboard/resources` refreshes), but returning the full view does
not require *re-discovering* every other work root's git state on every
call -- most of that state did not change since the last time it was
discovered.

## Questions To Resolve

- Can already-discovered work root state be cached with a short TTL or
  invalidated only on relevant events (workRoot opened/closed/activation
  changed, or an explicit refresh request), instead of being unconditionally
  rediscovered on every mutation of any single work root?
- If cached, what invalidates a cache entry? Filesystem watchers are
  probably out of scope/complexity for this; a simple TTL (e.g. a few
  seconds) or "only rediscover the mutated work root, reuse last-known state
  for the rest unless explicitly refreshed via `GET /resources`" may be
  sufficient.
- Does the browser-visible contract actually require every open/activate/
  remove response to carry live-rediscovered state for *all* work roots, or
  would carrying the last-known (possibly slightly stale) state for
  untouched work roots be acceptable, given `GET /resources` already exists
  as the canonical live-refresh route the frontend can poll/call for a fresh
  view?
- Is there a correctness reason (e.g. a work root disappearing from disk
  between calls) that requires full rediscovery on every call, or is that an
  edge case already tolerable at `GET /resources` polling cadence?

## Non-Goals

- Changing the per-spawn git process-creation cost itself (already addressed
  by the combined-rev-parse fix; further reduction, if any, is out of scope
  here).
- Introducing filesystem-watcher-based invalidation (likely excessive for
  the actual latency this solves).
- Changing the response shape/contract of any route.

## Evidence To Collect

- Wall-clock timing of `open_work_root`/`set_work_root_activation`/
  `remove_workspace` at a few registered-work-root counts (e.g. N=1, 5, 10,
  20) to confirm the O(N) growth pattern before committing to a caching
  design.
- Whether the frontend already re-fetches `GET /resources` shortly after
  any open/activate/remove call (if so, returning slightly-stale aggregated
  state from the mutation response itself may be low-risk).
