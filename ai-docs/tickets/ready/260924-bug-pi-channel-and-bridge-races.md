---
title: Pi channel socket sweep, bridge EPIPE, and direct-settle races
related:
  260924-bug-pi-subtree-held-settle-stall: origin of the direct-settle forward finding; its release rule and counters are reused here
  260924-feat-pi-agent-channel-subtree-state: constrains; fail-closed subtree view and settle fences stay intact
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 9691c8b6a778089b
sage-review-completeness-reviewed: 9691c8b6a778089b
---

# Pi channel socket sweep, bridge EPIPE, and direct-settle races

## Background

Three independent races in `agents-plugin-pi`, each found while chasing intermittent test failures. They are bundled because each is small and all three share one verification gate (`npm test` in `agents-plugin-pi/`).

### A. The socket sweep deletes live sockets

`bindPipe` (`agents-plugin-pi/src/agent-channel.ts`) sweeps the socket directory before every non-Windows bind. `sweepStaleChannelSockets` unlinks any `.sock` file whose connect probe returns `ECONNREFUSED`.

- The directory is `defaultChannelSocketDir()`, `$TMPDIR/ws-pi-<uid>`. Every Pi process of the OS user shares it: other sessions, sessions in other working directories, and nested agents. Each of these binds when it spawns a child.
- A peer's socket file exists between its `bind()` and its `listen()`. A probe in that window gets `ECONNREFUSED` and the sweep unlinks a socket that is about to be live.
- The parent keeps listening on the unlinked inode. The child's connect fails with `ENOENT`, its hello fails, the child Pi exits, and the spawn fails. Nothing retries.
- Reproduced on macOS with the real `bindChannelEndpoint`/`sweepStaleChannelSockets`:
  - one process sweeping continuously unlinked 82 of 150 live sockets, each at bind time;
  - the production pattern (one sweep per bind) lost 8 of 1200 binds across 4 processes and 39 of 2400 across 8, at a ~20 ms bind cadence.
- This is the mechanism behind the intermittent `connect ENOENT` in `test/agent-telemetry-lifecycle.test.ts`. Parallel test files bind in the shared default directory.
- Windows is unaffected: it binds named pipes and never sweeps.

### B. A dead MCP launcher surfaces as `write EPIPE`

`McpStdioClient.request` (`agents-plugin-pi/src/mcp-stdio-client.ts`) rejects the pending call immediately when the stdin write callback reports an error.

- When the launcher has already exited before the `initialize` write, the caller sees `write EPIPE`. Otherwise it sees the exit handler's `ws-mcp process exited unexpectedly (code=…, signal=…)`.
- Which message wins depends on scheduling. `test/session-bootstrap-guard.test.ts` asserts the exit message. It failed 1 of 20 runs with 32 CPU-bound processes on a 16-core machine, and 0 of 20 on an idle one.
- Failure behavior is otherwise identical (loud notification, child exits). The EPIPE text drops the exit code, which is the useful diagnostic. Windows can report the dead pipe with a different error code, so the message is also platform dependent.

### C. A direct settle can report a previous generation's answer

`attachEventListener` (`agents-plugin-pi/src/spawner.ts`) admits a stdout `agent_settled` directly whenever `record.waitingOnChildren` is false. It skips the child-authoritative check that `releaseSettlementHold` applies to held settles (`!turnOwed && turnsStarted <= observedTurnStarts`).

Example:

1. Child C dispatches grandchild G and ends turn 1 with an interim message ("waiting for G").
2. C's stdout falls behind its socket. Over the socket the parent sees G finish, C run its whole wake turn 2, and a snapshot that is not waiting, owes no turn, and reports `turnsStarted = 2`.
3. Turn 1's `agent_settled` then arrives on stdout. The view reads not-waiting, so it is admitted directly.
4. The lead receives turn 1's interim message as C's final answer. The real answer follows as a second `ws-agent-settled` once turn 2's events arrive.

No data is lost, but the lead may act on the interim answer. This requires stdout to trail the socket by a whole LLM turn, so it is rare.

The held-settle ticket's Result records a reverted attempt at this check. It broke the three `production fork lifecycle` tests in `test/fork-lifecycle.integration.test.ts`: their substituted `RpcClient` relays hand-picked events to the parent and never delivers the in-process child's `agent_start`. In production, Pi's RPC stdout carries every `agent_start`, and the held-settle release already depends on that count.

## Decisions

- **A: Sweep on owner death, not on refusal alone.**
  - The socket file name carries the binding process's pid, for example `<pid>-<random>.sock`.
  - The sweep unlinks a file only when both hold:
    - its pid is dead: `process.kill(pid, 0)` throws `ESRCH`. `EPERM` or success counts as alive;
    - its connect probe returns `ECONNREFUSED`.
  - A file whose name has no parsable pid, such as one bound by an older plugin version, is never unlinked. It may leak in the temp directory, but it is never a live socket deleted.
  - Pid reuse can only keep a stale file, never delete a live one.
  - Accepted residual: an older-version Pi process of the same user still sweeps on refusal alone and can unlink a new-format socket in its bind window. The race persists only until every Pi process of the user runs the fixed version.
  - Rejected: an age threshold on the file's mtime. A clock-based cut-off still races a slow bind and needs a tuned constant.
  - Rejected: per-process socket directories. They would leave stale directories and sockets from SIGKILLed parents that no one sweeps.
- **B: The exit handler owns the rejection.**
  - A stdin write error no longer rejects the pending call by itself. The `exit` or `error` handler rejects it with its existing message.
  - A bounded fallback still rejects with the write error when neither handler fires within a short bound. The worker picks the bound, at most a few seconds. A write error on a still-running process cannot hang the call.
  - The stream-level `stdin` `error` listener stays, so EPIPE never becomes an unhandled exception.
  - The fix is in production code. The test keeps asserting the exit message.
  - Rejected: loosening the test to accept either message. It leaves the diagnostic scheduling- and platform-dependent.
- **C: A direct settle applies the same child-authoritative check.**
  - When stdout settles while the view is not waiting, the settle is held, not admitted, if the latest accepted subtree snapshot for the current launch reports `turnOwed` or `turnsStarted > observedTurnStarts`.
  - Either condition alone holds the settle, and the gate keeps both. The owed condition covers the likelier form: C ends turn 1 while waiting on G, G's terminal clears the wait and reserves C's wake, and that not-waiting, owed, same-count snapshot reaches the parent before stdout's turn-1 settle. This needs stdout to trail only by the settle-to-clear interval, not a whole turn.
  - A held direct settle is a held settle: it records `heldGeneration` like the held path, so every later accepted not-waiting snapshot runs `releaseSettlementHold`. A settle held on an older owed snapshot is released by the next snapshot, which the child's `agent_start` flush always sends.
  - The wake turn's `agent_start` opens a new work generation, and that turn's own settle admits. The earlier generation is never reported.
  - The latest snapshot's turn facts are scoped to the launch like `observedTurnStarts`: a relaunch starts with none. A record with no channel snapshot for the current launch admits directly, as today, so the check never turns a channel-less child fail-closed.
  - Rejected: a timeout release. The held-settle ticket rejected it because it reports the previous generation's answer ahead of a slow wake turn.
  - Rejected: gating on `turnsStarted` alone. It leaves the owed-only form above reporting the interim answer.
- **C1: `owed` is exact at the raw settle, so it cannot leak.**
  - Today an `agent_end` boundary batch whose continuation never starts (`agent.continue()` throws) leaves `owed` set until the next prompted turn. The held-settle ticket accepted that as fail-closed for held settles. Under C it would also stall every later direct settle of a dormant child, which the lead sees as a hang.
  - Fix on the child side, keeping the held-settle rule that the facts are exact. In the `agent_settled` handler of `registerPushFlush`, after its flush, `owed` becomes exactly "a push wake reservation is outstanding". A settled Pi loop has no continuation coming, so a boundary batch still unstarted at that point is not owed a turn. It starts with the next prompted turn.
  - More generally, `owed` from a wake reservation lives exactly as long as the reservation. Wherever the reservation clears without a turn starting, such as `reserveWakeStart`'s timeout when its retry exits early, `owed` clears too. The worker may derive it from the reservation's lifetime instead of setting it at each site.
  - Recomputing `owed` rides the settled snapshot the child already publishes and adds no send. This holds only while `registerPushFlush`'s `agent_settled` handler runs before the per-event `publishSubtree` handler (`agents-plugin-pi/src/index.ts`). Pin that order with a comment at the registration site and with the C1 test. If the order cannot be guaranteed, send the flip on its own: under the held-settle ticket's rule, exactness outranks send volume.
  - Consequence, accepted: when a continuation throws, the unstarted batch may carry a grandchild's terminal. The child's settle is then admitted and the lead may receive the interim answer until the next prompted turn processes the batch. This is today's direct-path behavior for that rare path, kept instead of a hang.
  - Precondition: Pi's raw `agent_settled` fires only after any post-run continuation has started or been abandoned. The worker confirms this in the Pi SDK source before relying on it. If it does not hold, stop and report instead of choosing another rule.
- **C2: The observed-start count must see every child turn.**
  - `observedTurnStarts` counts only `agent_start` events delivered after `attachEventListener`. Both launch sites attach before the first prompt. That ordering becomes an invariant, stated at the attach sites and pinned by a test, because under C a lagging count holds every direct settle of that launch.
- **C3: The fork lifecycle harness models production process isolation.**
  - The three `production fork lifecycle` tests relay the child's `agent_start` to the parent, as Pi's RPC stdout does in production. The check is not weakened to fit the harness.
  - `ownTurnRef` is module-scoped (`spawner.ts`). The harness runs the lead and every child in one process from one module instance, and lists `src/index.ts` twice as an extension path. A child's snapshot may therefore count the lead's and siblings' turns, or count each start twice. The worker confirms this first.
  - Preferred fix: give each in-process session its own module instance, for example a per-session plugin copy, matching production's one-process-per-child. Moving own-turn state into the factory closure is acceptable only if the harness cannot isolate modules. It must then move every piece of state that `requestPushWake` and `submitHeldPushBatch` touch together, not `ownTurnRef` alone.
- **Out of scope:** the `claude-delegate.ts` cancel-reported-as-timeout race (user dropped it).
- **Threat model:** same-user forgery stays out of scope. A same-user process can still plant or remove files in the private socket directory. That baseline is unchanged: the directory stays owner-only (0700) and is refused if it is not.

## Prior Decisions

- 260924-bug-pi-subtree-held-settle-stall (2026-09-24, Result): "A settle that arrives while the view is already not waiting is still admitted directly. If the socket runs ahead of stdout by a whole settle, wake and start interval, the previous generation's..." — bearing: supports
- 97cce876 (2026-09-24, commit): "Correctness reviewer suggested also holding a settle at settle time when the socket-reported turn count runs ahead of stdout... Tried it: it failed the three in-process production fork lifecycle tests" — bearing: constrains
- 6f2ccffb (2026-09-24, commit): "turnsStarted vs parent-observed agent_start count covers socket-ahead-of-stdout: the parent waits for stdout to deliver the start, which opens a new work generation whose own settle admits." — bearing: supports
- 990a9ca9 (2026-09-24, commit): "The release is scoped rather than blanket: ... a blanket replay would report the previous generation's answer early and then a second terminal." — bearing: constrains
- 47b274ef (2026-09-24, commit): "The initial view is waiting until the first snapshot or resume hello arrives: a not-yet-connected child is never permission to settle (fail closed)." — bearing: constrains
- 156f12be (2026-09-24, commit): "Correctness I3 (socket directory ownership): lstat after mkdirSync requiring a directory owned by process.getuid() with mode & 0o077 === 0; failure is a pipe bind failure..." — bearing: constrains
- 260924-feat-pi-agent-channel-transport (2026-09-24, ad03ac41): "The child channel is held in the factory closure, not a module singleton, because fork-lifecycle.integration.test.ts loads several in-process children from one module instance." — bearing: constrains
- 0d47b71f (2026-09-03, commit): "PendingRequestRegistry.register() takes resolve/reject callbacks directly rather than returning a Promise itself, so McpStdioClient's request() shape needed only a minimal edit" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-channel.ts, agents-plugin-pi/src/mcp-stdio-client.ts, agents-plugin-pi/src/spawner.ts (attachEventListener direct settle; registerPushFlush agent_settled owed recompute; attach-site ordering), agents-plugin-pi/test/fork-lifecycle.integration.test.ts (agent_start relay and per-session module isolation), plus new cases in existing test files |
| scope.surface | cross-module | three independent modules; exported sweepStaleChannelSockets and bindChannelEndpoint keep their signatures; the on-disk socket file name format changes and is read by every Pi process of the OS user, including older plugin versions |
| scope.new_public_symbol | no | none required by the plan; a pid-parse helper may stay module-private |
| scope.new_type_contract | no | no type or signature change; the socket name gains a pid prefix, with legacy names handled by the never-unlink rule |
| scope.test_surface | existing | test/agent-channel.test.ts, test/mcp-stdio-client.test.ts, test/session-bootstrap-guard.test.ts, test/subtree-lifecycle.test.ts, test/recursive-worker.test.ts, test/fork-lifecycle.integration.test.ts |
| complexity.reuse_points | confirmed | releaseSettlementHold rule and per-client observedTurnStarts counter at agents-plugin-pi/src/spawner.ts#L2714-L2765; probeUnixSocket at agents-plugin-pi/src/agent-channel.ts#L195-L207; the observer's latest accepted snapshot lives in agents-plugin-pi/src/subtree-lifecycle.ts#L195-L215 and is not stored on the record today, which keeps only subtreeRevision and subtreeDescendants |
| complexity.side_effect_risk | moderate | the sweep runs against a directory shared by every Pi process of the user, and the direct-settle path gates terminal delivery for every channel-launched child |
| risk.correctness | high | settle admission depends on unordered socket vs RPC stdout delivery; a prior settle-time hold was reverted, and a wrong gate either stalls fail-closed or reports twice |
| risk.fit | moderate | the fork lifecycle harness must be corrected to relay agent_start as production RPC stdout does, without weakening the check to fit it, and give each in-process session its own module instance (or, as fallback, move all push-machinery state into the factory closure together) because ownTurnRef is module-scoped at agents-plugin-pi/src/spawner.ts#L1037 |
| risk.test | high | all three are timing races; the plan requires deterministic ordering drives and repeated runs under CPU load, and the origin symptoms were intermittent flakes |
| risk.security_or_contract | moderate | sweep unlink semantics and process.kill pid probing in the owner-only socket directory, plus a cross-version socket name format; threat model stated unchanged |

## Phases

### Phase 1: Fix the three races

Implement Decisions A, B, C, C1, C2 and C3.

Verification:

- **A, live socket survives a concurrent sweep:** a test binds sockets in one process while a second process sweeps the same private `socketDir` continuously. No live socket is unlinked. It must be cross-process: in one process every socket carries the test's own live pid, so the pid gate alone protects it and the refusal window is never exercised.
- **A, dead owner is swept:** a socket file named for a dead pid with no listener is unlinked. A file named for a live pid with no listener is kept.
- **A, legacy name is kept:** a `.sock` file without a parsable pid is never unlinked.
- **B, deterministic message:** a launcher that exits before the first request is written rejects with the exit-coded message whether the write error or the exit event is observed first. Drive both orders deterministically, not by timing.
- **B, fallback:** a write error with no following exit or error rejects within the bound.
- **C, socket ahead by a whole wake turn:** stdout's turn-1 settle arrives after a snapshot reporting `turnsStarted` beyond the parent's observed count. Turn 1 is not admitted; turn 2's settle admits exactly once.
- **C, owed only:** a not-waiting snapshot with `turnOwed` true and an unchanged `turnsStarted` arrives before stdout's turn-1 settle. Turn 1 is not admitted; the wake turn's settle admits exactly once. A `turnsStarted`-only gate must fail this test.
- **C, stale owed releases:** a direct settle held on an older owed snapshot is released by the next accepted not-waiting, not-owed snapshot, with no further child turn.
- **C, no snapshot:** a child with no current-launch snapshot still admits its direct settle at once.
- **C1, continuation that never starts:** a boundary batch whose continuation throws leaves `owed` false after the raw settle, unless a wake reservation is outstanding. The settled snapshot the parent receives reports `turnOwed` false, and the child's direct settle is admitted, not stalled.
- **C1, reservation timeout:** a wake reservation that times out without starting a turn leaves `owed` false.
- **C2, attach ordering:** a test pins that the parent's event listener is attached before the child's first prompt at both launch sites.
- **C3, fork lifecycle:** the three `production fork lifecycle` tests pass with the harness relaying `agent_start`, and a child's reported `turnsStarted` counts only that child's turns.
- **Full suite:** `npm test` in `agents-plugin-pi/` passes.
- **Load:** each touched test file runs at least 50 times with CPU-bound processes at twice the core count, with zero failures. The Result records the counts.

### Result (928a48fa) - 2026-09-24

All three races are fixed. Verification passes, including the reduced load runs noted below.

Commits:

- `98c51a40` (A): the socket sweep unlinks a file only when its owner pid is dead and its probe is refused. Legacy names are never unlinked. This commit lacks the `Co-Authored-By` trailer. The history was deliberately not rewritten to add it.
- `9b869b5b` (B): the exit or error handler owns the rejection. A write error only arms a 1500 ms fallback (`WRITE_ERROR_FALLBACK_MS`, unref'd).
- `ef0961fc` (C, C1, C2, C3): a direct settle is held when the snapshot owes a turn or reports more started turns than the parent observed. The child-side `owed` is derived exactly. Attach-order invariants are recorded as comments. The fork lifecycle harness now relays `agent_start` and gives each session a fresh module.
- `928a48fa`: fixes from review round 1.

Decisions made while implementing:

- `subtreeOutstanding` also counts a held settle of the current generation (`heldSettlementGeneration === workGeneration`). The ticket text did not ask for this. Without it, a middle process holding its child's settle reads quiescent upstream while that child's terminal is still coming. The parent one hop up would then admit the middle process's interim answer, which is the same bug one hop up. The count also closes the same gap on the waiting-held path. Review found it correct and in scope.
- `owed` is `boundaryTurnOwed || pushWakeReserved`.
  - The raw `agent_settled` clears the boundary part after its flush.
  - The reservation part lives exactly as long as the reservation.
  - `registerPushFlush` takes a `publish` option, so a reservation that lapses with an early-exit retry publishes that change itself.
- C1 precondition confirmed in Pi's `agent-session.js`. `_runAgentPrompt` loops `continue()` and emits `agent_settled` in `finally`.
- C3 isolation uses Pi's `clearExtensionCache()` (`dist/core/extensions/loader.js`) before each session's reload. It was chosen over a per-session plugin copy.
  - Pi's own `reload()` calls the same function, and jiti runs with `moduleCache: false`.
  - The export is outside the package `exports` map. An SDK change fails loudly: removing the call fails all three fork lifecycle tests.
  - "A child counts only its own turns" is asserted indirectly: two starts were relayed and the fork reaches `dormant`. The adapter module instance is not reachable from the test.
- `observeChildSubtree` releases a held settle before it publishes. This matches the order on the direct admission path.

Verification:

- `npm test` in `agents-plugin-pi/` at `928a48fa`: 1828 pass, 0 fail, 2 skipped. The skipped tests are pre-existing opt-in tests.
- Each Verification bullet has a test.
- Mutation checks, all failing as expected:
  - A: without the pid gate, the cross-process sweep test fails.
  - B: rejecting at once on a write error fails the write-first, fallback and spawn-error tests.
  - C: a `turnsStarted`-only gate fails the owed-only and stale-owed tests. Dropping the direct gate fails all three direct-settle tests.
  - C1: keeping the boundary `owed` at settle fails the continuation test. Dropping the timeout's clear or its publish fails the timeout test.
  - C2: moving either attach after its prompt fails that path's test.
  - C3: removing the cache clear or the relay fails all three fork lifecycle tests.
  - The `subtreeOutstanding` generation guard reduced to `!== undefined` fails the new guard test.
- Load: 32 CPU-bound processes on 16 cores (twice the core count).

  | Test file | Clean runs | Failures |
  |---|---|---|
  | `agent-channel.test.ts` | 50 | 0 |
  | `agent-channel.integration.test.ts` | 50 | 0 |
  | `mcp-stdio-client.test.ts` | 50 | 0 |
  | `session-bootstrap-guard.test.ts` | 50 | 0 |
  | `recursive-worker.test.ts` | 50 | 0 |
  | `subtree-lifecycle.test.ts` | 50 | 0 |
  | `agent-channel-launch.test.ts` | 41 | 0 |
  | `fork-lifecycle.integration.test.ts` | 20 | 0 |

  - `agent-channel-launch.test.ts` ran 41 clean runs. The 42nd was interrupted by the stop, not failed.
  - The C runs for `agent-channel-launch` and `fork-lifecycle` are below the required 50. The user approved the shorter run.
  - The review-fix commit `928a48fa` had no extra load runs.

Review:

- Round 1 found no critical or important issues in the correctness, fit and test reviews. Its minor findings were fixed in `928a48fa`.
- Round 2 was clean for correctness. The test review left one minor item: no test pins the observer's release-before-publish order. On the ordinary path the upstream count is the same either way.

Accepted residuals:

- If a parent ever misses a child's `agent_start`, every settle of that launch stays held, and every ancestor now reads waiting too. This would take a turn started before `attachEventListener`, for example by a third-party extension at child startup. C2 pins attach-before-first-prompt at both launch sites.
- The liveness probe does not cover a record held on turn facts. That record always has a channel, so a silent child death is caught by the channel disconnect.
- The sweep's pid check sees only its own pid namespace. A same-user Pi in another pid namespace that shares the temp directory could still be swept in its bind window.
- The index.ts registration-order pin is a text check. The C1 behavior test also fails if the order is wrong.
