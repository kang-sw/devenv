---
title: "Restore the dashboard session-iframe cache behind configuration"
related:
  261116-bug-ws-dashboard-hidden-cached-page-cost: "Cache removal and fresh-load improvements; owner superseded the no-cache forward"
---

# Restore the dashboard session-iframe cache behind configuration

## Background

The owner directed removal at 09:58 KST on 2026-11-16 because hidden session
pages accumulated background cost. `257e52c8d` (merged in `e6e0ebf44`) removed
the `shell.js` cache (`LIMIT = 5`, recency `touch()` and eviction) and its
`dashboard-lifecycle.js` handling. `55cd8f6fd` later made fresh loads cheaper.

The owner stated at 12:34 KST on 2026-11-16 that the cache should be restored
once dashboard performance is clearly stable. This supersedes the related
ticket's Forward M4 and Phase 2 instruction not to reintroduce a cache.

## Decisions

- **Gate:** do not start implementation until the owner confirms dashboard
  performance is stable.
- Add the same dashboard configuration key to both `agents-plugin` and
  `agents-plugin-wsflow` `dashboard-config-manifest.json` files: default `0`
  disables caching; positive `N` enables a bounded cache of N session frames.
- Reintroduce manually, not by plain revert: `50ad47ccb`, `190255257` and
  `c658c4798` touched the same files and a revert is likely to conflict.
- Compatibility tier **T0**: additive configuration key, default off.

## Phases

### Phase 1: Restore an idle, configurable session-frame cache

After the owner gate, restore bounded recency/eviction and lifecycle handling,
retaining the current draft, navigation and leave-guard guarantees. With `0`,
behavior must be identical to today. With `N > 0`, switching to a cached
session must be instant and hidden frames must perform no steady-state work:
no timers, requestAnimationFrame or SSE work while hidden, including nested
frames. Resume correctly on reveal and evict at the configured bound.

Verify off/on, switching, eviction and draft/navigation behavior with focused
coverage; capture a representative trace proving hidden frames are idle and
cached switching is instant. Compare against the current default-off behavior.
