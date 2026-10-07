---
title: "Pi child-runtime ensurer wiring is untested and the child failure diagnostic can race stop()"
related:
  261007-bug-pi-child-start-fails-after-version-bump: origin; release-gate review of its range (67a2a0777..538d503e1) returned NEEDS FIX
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 48984ac7fc6fc7ea
sage-review-completeness-reviewed: 48984ac7fc6fc7ea
completed: 2026-10-07
---

# Pi child-runtime ensurer wiring is untested and the child failure diagnostic can race stop()

## Background

The ws 0.46.33 release gate reviewed `67a2a0777..538d503e1`, whose payload
is `261007-bug-pi-child-start-fails-after-version-bump` (commit 91acbaca7).
The verdict was NEEDS FIX on one Important test finding, plus Minor findings.

**Important: the production wiring that fixes the bug has no test.**

- `startBridge` (`agents-plugin-pi/src/bridge.ts`, the `ensureChildRuntime`
  block after `buildLocalDevenvBootstrap`) creates the ensurer only when a
  startup local-devenv build ran. It passes `baselineHash` from
  `runtimeContractHash(readFileSync(opts.runtimeJsonPath))` read at startup,
  `runBuild: runGoBuild`, and `runLauncher` bound to `runLauncherOnce`
  (`python3 <launcher>` with `opts.pluginDir` as cwd). It exposes the
  ensurer as `BridgeHandle.ensureChildRuntime`.
- `registerAgentTools` (`spawner.ts`) calls
  `registerChildLaunchPreflight(rpcRegistry, bridge.ensureChildRuntime)`.
- No test code references `ensureChildRuntime`, `runLauncherOnce`, or
  `baselineContractHash` (the only hit is the header comment of
  `agents-plugin-pi/test/child-launch-guard.test.ts#L3`; `grep -rn` over
  `agents-plugin-pi/test` finds nothing else). The ensurer's unit tests (`local-devenv.test.ts`)
  inject a fake baseline, build and launcher. The spawn and resume tests
  (`child-launch-guard.test.ts`) register a hand-built preflight.
- So any of these regressions leaves the suite green and the original bug
  back:
  - deleting the `registerChildLaunchPreflight` call
  - dropping `ensureChildRuntime` from the returned handle
  - a wrong gate condition
  - a baseline hashed from the wrong bytes
  - a wrong `runLauncherOnce` argv or cwd

**Minor: the "Child reported:" diagnostic can be dropped by a stop race.**

- The fresh-spawn catch in `spawnAgent` reads the captured notify after
  `client.stop()`, under the comment "Read after stop(): the child's last
  notify has been drained by then." That comment is wrong.
- `RpcClient.stop()` (`@earendil-works/pi-coding-agent`
  `dist/modes/rpc/rpc-client.js`) calls `stopReadingStdout()` synchronously
  before anything else.
- The child's `exit` handler rejects pending requests. libuv can deliver that
  exit before the last stdout chunk is read in the same poll batch. The
  rejection then reaches the catch, and `stop()` detaches the line reader
  before the queued notify line is parsed.
- The launch error then falls back to the bare "Agent process exited". The
  dormant-resume catch in `sendToAgent` has the same window.
- The window is narrow: the child's drain-before-exit (`index.ts`
  `drainStdoutBeforeExit`) makes it uncommon (inferred from code; not
  reproduced).

**Minor: the contract-hash claim lacks a test.** The origin ticket keys drift
on the sha256 of the `runtime.json` bytes, "so a tool-inventory change
without a version bump is caught too". Every drift test in
`local-devenv.test.ts` changes `plugin_version`, so that claim has no test.

## Decisions

- **One standalone bug ticket, single phase.** The origin ticket is closed in
  `.done/`, so the fix cannot extend it.
- **Build stand-in through an explicit `BridgeOptions` seam.** Add an optional
  `runBuild` to `BridgeOptions`, defaulting to `runGoBuild`, and pass it to
  both the startup `buildLocalDevenvBootstrap` call and the ensurer.
  Production callers do not set it.
  - Rejected: (a) the `mailbox-bootstrap.integration.test.ts` trick of
    copying `bridge.ts` and string-replacing `runBuild: runGoBuild,`. It
    breaks on any reformat and tests a rewritten copy, not the shipped
    module.
  - Rejected: (b) a real `go build` behind an opt-in flag. `npm test` skips
    it, so it guards no regression.
- **Bounded stdout-end wait before `stop()` on a child-exit failure.** In
  both the `spawnAgent` catch and the `sendToAgent` dormant-resume catch,
  when the launch failed because the child process exited, wait for the
  child's stdout `end`/`close` before calling `client.stop()`. The wait is
  bounded (about 500 ms), so the queued notify line is parsed and the
  "Child reported:" decoration applies.
  - Reach the private process handle through one narrow typed accessor.
  - Fall back to an immediate `stop()` when the handle is absent, so an
    upstream rename degrades to today's behaviour.
  - Correct the wrong "drained by then" comment.
  - Rejected: (a) a `setImmediate` yield before `stop()`. It is only
    probabilistic.
  - Rejected: (b) a comment-only fix that accepts the window.
- **Tools-only drift test.** Add a case to `local-devenv.test.ts`: a
  `runtime.json` change with the same `plugin_version` and only `tools`
  changed must trigger a rebuild. This pins the origin ticket's contract-key
  decision (sha256 of the `runtime.json` bytes).

## Constraints

- `bin/ws-mcp-launcher.py` and `agents-plugin-pi/src/version-check.ts` are
  not edited (carried from the origin ticket).
- The ensurer and runtime wiring do not change behaviour for a session
  without the local-devenv marker, or one with no contract drift (carried from
  the origin ticket). This does not cover the stdout-end wait below. That wait
  applies to every child-exit launch failure whether or not the marker is
  present, because the origin ticket's failure decoration is not
  marker-gated.
- Out of scope, recorded as follow-ups from the release-gate review:
  - a stop that lands while the launch preflight is still pending (spawn and
    resume)
  - the `rebootstrap` rejection paths: `plugin_version` missing or
    non-string, and a launcher non-zero exit or timeout with artifact cleanup
  - the wall-clock `elapsed` bound in `session-bootstrap-guard.test.ts`

## Prior Decisions

- 971bceb2 (2026-10-07, commit): "Open Minor items carried as reported: bridge.ts production wiring is covered only through injected stand-ins (the next real release bump is the live check), and an earlier error notify from a still-live child could be appended to an unrelated launch error." — bearing: supports
- 261007-bug-pi-child-start-fails-after-version-bump (2026-10-07, Decisions): "A Pi child's bootstrap failure reaches the parent through the RPC event channel, not stderr. The child lets its error-level `ui.notify` ... flush before `exit(1)`" — bearing: supports
- 91acbaca (2026-10-07, commit): "The ensurer exists only when the bridge built at startup (marker present, lead/fork role), so no-marker sessions and worker/explore children take exactly today's path." — bearing: constrains
- 91acbaca (2026-10-07, commit): "spawnAgent runs the preflight before the channel bind and record registration, not right before `new RpcClient`: stopAgent only acts on a record with a client" — bearing: constrains
- 91acbaca (2026-10-07, commit): "Child drain polls writableLength (10ms) as well as 'drain', because small pipe writes never emit 'drain'; Pi's RPC output guard queues lines on a promise tail" — bearing: supports
- 260924-bug-pi-review-sweep-correctness-fixes (2026-09-24, Decisions): "A stop during a launch is the single terminal. `stopAgent` marks an in-flight launch as stopped." — bearing: constrains
- b00f7536 (2026-09-24, commit): "agent-channel.test.ts probe case now reads the parent's private preHello set ... chosen over a timer margin because the ticket's load gate forbids wall-clock-dependent assertions." — bearing: constrains
- 260921-bug-pi-child-bootstrap-error-loses-diagnostic (2026-10-07, Resolution): "Absorbed into Phase 2 of 261007-bug-pi-child-start-fails-after-version-bump: the child's error notify is flushed before exit and captured by the parent over the RPC event channel, not stderr" — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/bridge.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/test/local-devenv.test.ts, one new integration test file |
| scope.surface | cross-module | optional runBuild added to exported BridgeOptions (bridge.ts#L49-L68) consumed through BridgeHandle.ensureChildRuntime into spawner.ts registerAgentTools |
| scope.new_public_symbol | no | none named; runBuild is a field on the existing exported BridgeOptions and the private-handle accessor is not specified as exported |
| scope.new_type_contract | yes | optional BridgeOptions.runBuild field (default runGoBuild) and a narrow typed accessor for the RpcClient private process handle |
| scope.test_surface | new-files | new integration test driving startBridge plus registerAgentTools; extends existing agents-plugin-pi/test/local-devenv.test.ts |
| complexity.reuse_points | confirmed | startBridge harnesses in test/bridge.test.ts and test/lead-key-resume.test.ts, createChildRuntimeEnsurer, captureChildBootstrapFailure all read |
| complexity.side_effect_risk | moderate | the launch-failure path gains a bounded wait before client.stop() in two catches, and the BridgeOptions seam must leave production callers unchanged |
| risk.correctness | moderate | the race fix depends on private RpcClient process internals and must keep the stopped-launch terminal and clearLiveState ordering in spawnAgent and sendToAgent |
| risk.fit | moderate | the accessor reaches into an upstream package private field with a fallback to immediate stop, and the seam follows the existing injected-deps pattern |
| risk.test | high | the stop race is inferred and not reproduced, the new integration test spans bridge and spawner with fake launcher and build, and wall-clock assertions are barred by the load gate |
| risk.security_or_contract | low | launcher, version-check and protocol are not edited per Constraints; only an optional test seam and failure-path timing change |

## Phases

### Phase 1: Test the bridge-to-spawner ensurer wiring and close the diagnostic race

- Add an integration test that drives production `startBridge` with a
  local-devenv marker, a build stand-in (the `BridgeOptions.runBuild` seam) and a
  launcher stand-in. Follow `bridge.test.ts` and `lead-key-resume.test.ts`,
  which already start production `startBridge` against a fake launcher.
  Then call `registerAgentTools` with the returned handle.
  - With the marker and `runtime.json` unchanged: assert the first
    `spawnAgent` invokes neither the build nor the launcher `version` run.
    This catches a baseline hashed from the wrong bytes.
  - Then, in the same session, rewrite `runtime.json` (bump
    `plugin_version`) and assert the next `spawnAgent` rebuilds and runs the
    launcher install before the child starts.
  - Without the marker: assert that no build or launcher install runs before
    a child launch.
  - Exercise `runLauncherOnce` itself: real argv and cwd, not a test-local
    closure.
- Close the stop race with the bounded stdout-end wait at every site that
  decorates a launch failure, and correct the comment. The sites are:
  - the `spawnAgent` launch catch (its first prompt is inside that try)
  - the `sendToAgent` dormant-resume launch catch
  - the `sendToAgent` first-prompt catch, which decorates a child whose
    bootstrap failed after hello ("surfaces its exit here")
- Trigger the wait only when the child process has already exited: the
  client recorded an exit, the same signal `recordedExitError` reads. A stop
  (`launch.stopped()`), a handshake timeout or any other failure with the
  child still alive stops immediately, as today. Model the narrow typed
  accessor for the process handle on `recordedExitError`.
- Test the race fix deterministically, with no wall-clock assertion
  (b00f7536). Use the `child-launch-guard` `RpcClient` patch harness or an
  equivalent fake process whose `exit` fires before its stdout ends, then
  emits the error notify line and ends. Assert that the launch error carries
  "Child reported:", and that with no process handle the catch stops
  immediately and falls back to the undecorated error.
- Add the tools-only drift case.
- Verification:
  - `npm test` in `agents-plugin-pi` passes.
  - The new integration test fails when the `registerChildLaunchPreflight`
    call in `registerAgentTools` is removed.
  - The race test fails when the stdout-end wait is removed.
  - Record both checks in the Result.

### Result (2d1ca1770) - 2026-10-07

- `BridgeOptions.runBuild` (optional, default `runGoBuild`) feeds both the
  startup local-devenv build and the child-runtime ensurer.
- New `agents-plugin-pi/test/child-runtime-wiring.integration.test.ts` drives
  production `startBridge` and `registerAgentTools`, with only the build seam
  stubbed. The fake launcher sits under `plugin/bin/` so a cwd regression is
  observable. The test covers:
  - with the marker, no build or install before the first spawn;
  - after a `plugin_version` bump, a rebuild plus one real `runLauncherOnce`
    run (argv `[launcher, "version"]`, cwd = plugin dir) before the child
    starts, and nothing on the next spawn;
  - without the marker, nothing before either spawn.
- `spawner.ts` gains `awaitExitedChildStdout`, called at three sites:
  - Before `client.stop()` in the `spawnAgent` launch catch and in the
    `sendToAgent` dormant-resume catch.
  - Before decoration in the `sendToAgent` first-prompt catch.
  - It waits, bounded at 500 ms, for the child's stdout `end`/`close`. It
    applies only when the client recorded an exit (`recordedExitError`) and
    the launch was not stopped.
  - It returns at once when the private process handle is missing.
  - The wrong "drained by then" comment is gone.
- New race tests in `child-launch-guard.test.ts`:
  - spawn and resume, each at start and at the first prompt: the exit is
    recorded before the last stdout line, and the harness `stop()` detaches
    the reader first, as upstream does;
  - an exit with no process handle stops at once and keeps the undecorated
    error;
  - a failure with the child still alive stops at once.
  - No wall-clock assertions.
- New tools-only drift case in `local-devenv.test.ts`.
- Verification:
  - `npm test` in `agents-plugin-pi`: 2113 tests, 2110 pass, 0 fail,
    3 skipped.
  - Removing the `registerChildLaunchPreflight` call in `registerAgentTools`
    makes the wiring test fail ("the drift rebuilt and installed before the
    child started").
  - These mutations also fail it:
    - `ensureChildRuntime` dropped from the handle;
    - an inverted gate;
    - a baseline hashed from trimmed text instead of the bytes;
    - an extra launcher argv;
    - launcher-dir cwd.
  - Making `awaitExitedChildStdout` a no-op fails all four new race tests;
    the other cases still pass.
- Decisions:
  - `mailbox-bootstrap.integration.test.ts` boots through `index.ts`, which
    does not set the seam. Its source-rewrite anchor moved from
    `runBuild: runGoBuild,` to `opts.runBuild ?? runGoBuild;`.
  - A gate that is always on with no marker is not observable: the ensurer
    is a no-op without a marker. The no-marker test therefore guards the
    observable contract (no build, no install).
