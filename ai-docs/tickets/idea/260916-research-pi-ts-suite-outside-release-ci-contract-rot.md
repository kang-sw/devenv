---
title: agents-plugin-pi TS suite is outside release CI, so its contract tests rot unnoticed
related:
  260915-feat-ws-rationale-query-and-prior-decisions: the tool addition that pushed the stale bridge.test.ts contract from 56 to 57
---

# agents-plugin-pi TS suite is outside release CI, so its contract tests rot unnoticed

## Status of this ticket

Direction / investigation. Surfaced during the ws 0.46.8 ship-gate sweep. No
owner agreement required to investigate, but the CI-scope and contract-shape
choices below are observable-workflow changes — confirm the approach before
implementing.

## Observation

The release workflow (`.github/workflows/ws-mcp-release.yml`) runs only
`go test ./...` in `agents-plugin-tool` (plus a Windows Go smoke). It never runs
the `agents-plugin-pi` TypeScript suite (`npm test`). As a result, hand-maintained
contract assertions in that suite can go red and stay red across releases without
anyone noticing.

Concrete instance found at the 0.46.8 ship: `agents-plugin-pi/test/bridge.test.ts`
asserted `bundledToolNames.length === 54` and deep-equalled a hand-maintained
`LIVE_TOOL_NAMES` snapshot of 54 tools, but `runtime.json` already carried 56
tools at the 0.46.7 ship (`worktree.acquire`, `worktree.release` had been added
without updating the snapshot) and 57 after this batch added `rationale.query`.
The test had therefore been shipping red since at least 0.46.7. It was fixed
reactively in this ship (commit 160d8ec4), but only because a reviewer happened
to run the full pi suite by hand.

A second, different failure mode lives in the same suite:
`test/web-startup.test.ts` is a real-process startup test that fails in any
environment without a compatible `ws-mcp` binary (it failed locally during the
ship), so "the pi suite is green" is not currently a clean, portable signal even
setting the rot aside.

## Threads to decide

- **CI scope.** Should the release gate (or a pre-merge CI) run the pi TS suite?
  If yes, it must first be made deterministic in CI (see the web-startup thread),
  or a red environment-only test would block every release. Weigh against the
  reason it is Go-only today (the release artifact is the Go `ws-mcp` binary; the
  TS adapter ships separately via Pi git-install).
- **Contract shape.** The `bundledToolNames.length === N` magic number is the
  part that rots. Options: (a) drop the redundant count and rely on the
  `deepEqual(LIVE_TOOL_NAMES, bundledToolNames)` cross-check plus the explicit
  present/absent pins, which already carry the real drift signal; (b) keep a
  count but derive it so it cannot rot (loses the "force a human to acknowledge a
  surface change" tripwire value); (c) keep the hand-maintained snapshot but gate
  it in CI so it cannot rot silently. The snapshot's stated purpose is to mirror
  the opt-in `develop-marker.integration.test.ts` live capture cheaply — decide
  whether that value survives if CI never runs it.
- **Environment-only tests.** `web-startup.test.ts` (and similar real-process
  tests) should follow the `develop-marker.integration.test.ts` opt-in pattern
  (`WS_PI_VERIFY_*` env guard) so the default suite is portable-green.

## Open questions

- Is there any other hand-maintained contract in the pi suite currently red for
  the same reason? (Audit `bridge.test.ts`'s sibling `*.test.ts` count/snapshot
  assertions.)
- Does wsflow's Python suite have the same CI-scope gap?
