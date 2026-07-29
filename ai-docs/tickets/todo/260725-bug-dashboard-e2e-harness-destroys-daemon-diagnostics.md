---
title: e2e browser-acceptance harness destroys the daemon diagnostics it needs on failure
related:
  260725-feat-dashboard-nav-row-two-line-open-state: found-during
  260725-bug-dashboard-terminal-socket-path-length-unguarded: found-during
---

# e2e browser-acceptance harness destroys the daemon diagnostics it needs on failure

## Background

`ws-dashboard/frontend/e2e/daemonHarness.ts::startDaemon()` scrapes the
spawned daemon's stdout/stderr only long enough to find the owner pairing
URL, then stops listening and drains the rest into no-op handlers
(`daemonHarness.ts:286-291`: `child.stderr?.on("data", () => {})` /
`child.stdout?.on("data", () => {})`). Nothing captures or persists daemon
output for the remainder of the run.

Separately, the daemon's file log (`logging.rs`, written to
`<state_dir>/logs/daemon.log.<date>`) lives inside the `WS_DASHBOARD_STATE_HOME`
temp directory that `dashboard-acceptance.spec.ts`'s `afterAll` deletes
unconditionally (`rmSync(stateHome, { recursive: true, force: true })`,
regardless of test outcome).

Net effect: any daemon-side ERROR line — including the one Phase 1 of
`260725-bug-dashboard-terminal-platform-macos-unsupported` added specifically
to distinguish "helper wrote a registry entry but the daemon could not
connect" from "helper never wrote a registry entry" — is generated during the
run and then unconditionally destroyed before a failing test's output can be
inspected. This is exactly why the macOS socket-path-length root cause
required a standalone manual reproduction outside the harness instead of
being diagnosable from a failed CI/local run's logs.

## Phases

### Phase 1: Persist daemon diagnostics across a run, at least on failure

Keep capturing daemon stdout/stderr (or tail the on-disk log file) for the
full run instead of draining it into a no-op once the pairing URL is found,
and write it to a location outside the deleted temp `WS_DASHBOARD_STATE_HOME`
(e.g. alongside the existing `e2e/.artifacts/evidence.txt` output) — at
minimum on test failure, ideally always for consistency with existing
artifact-capture. Preserve the harness's existing behavior of not letting a
full pipe buffer block the daemon.
