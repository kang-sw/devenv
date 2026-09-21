---
title: "Pi adapter: isolate subtree publication failures from nested lifecycle"
related:
  260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter: predecessor — introduced cross-process subtree publication and observation
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 3ad6ae289513ae33
sage-review-completeness-reviewed: 3ad6ae289513ae33
completed: 2026-09-21
---

# Pi adapter: isolate subtree publication failures from nested lifecycle

## Background

On Windows, nested subagent spawning can fail repeatedly when private `subtree.json` replacement exhausts its bounded `EPERM` or `EBUSY` retry. A child worker that cannot spawn its grandchild has no live descendant to keep it in `waiting-on-children`, so it can settle immediately after the failed tool call.

The publication failure also crosses unsafe boundaries. An exception raised while an RPC listener refreshes or republishes subtree state can prevent later event and settlement handling, while the installed Pi RPC client catches and discards the listener exception. The filesystem watcher path can instead surface the same exception as uncaught. Publication during cleanup can also obscure the original spawn failure.

The file handle audit found no long-lived read-modify-write critical section: synchronous reads close before parsing and derivation, writes serialize in memory and close the temporary file before replacement, and each nested process reads its child's channel while writing its own upstream channel. The immediate repair therefore targets redundant publication and failure isolation rather than shortening handle lifetime or replacing the transport.

## Decisions

- Skip a `subtree.json` replacement when the effective snapshot is unchanged. Token streaming, duplicate filesystem notifications, and render refreshes must not create equivalent writes.
- Preserve the initial `beginSubtreeDispatch` busy publication as a fail-closed gate. If the process cannot tell its parent that a nested dispatch has begun, it must not start the grandchild.
- After that gate succeeds, a secondary publication failure must not prevent RPC event application, settlement processing, watcher cleanup, or the reporting of the original spawn error.
- Contain publication failures raised from filesystem watcher callbacks so they do not become uncaught process errors.
- A cleanup publication failure must not replace the primary spawn or RPC failure. Preserve the primary error and report the secondary publication failure separately.
- When Windows replacement retries are exhausted, emit bounded diagnostics that identify at least the path, writer PID, channel nonce, snapshot revision, and attempt count.
- Keep the existing bounded Windows retry behavior. Async writer queues, writer leases, alternate transports, and broader registry recovery are outside this hotfix.

## Constraints

- Failure isolation must remain conservative: it must not synthesize `waitingOnChildren: false` from an unreadable, mismatched, or unpublished snapshot.
- Dedupe must compare the effective transport state, not object identity, and must not suppress a revision or lifecycle transition that changes parent wait accounting.
- Spawn-failed records may remain non-waiting when no grandchild process was started; the hotfix must not fabricate a child merely to hold the parent open.
- A grandchild process must never start after its initial busy publication failed.
- Diagnostics must not introduce another throwing path or expose prompt or transcript content.
- Preserve the per-edge channel and nonce validation contract.

## Prior Decisions

- 08ac495b (2026-09-21, commit): "The streamed-delta path now skips subtree observation, telemetry refresh, and subtree publication." — bearing: constrains
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20, Resolution): "Implemented bounded recursive subtree identity propagation, shared nested tree rendering in the live gutter and `/audit`, push notifications across process hops, non-openable propagated audit context rows, and regression coverage for lifecycle invariants and guard limits." — bearing: supports
- b318a127 (2026-09-21, commit): "Windows watcher-driven subtree publication can encounter transient EPERM or EBUSY during renameSync." — bearing: constrains
- 260912-feat-ws-pi-recursive-worker-subtree-lifecycle (2026-09-13, commit): "Direct-parent enqueue acceptance clears terminal obligations; held or failed delivery remains recoverable, while explicit nested stop is treated as synchronous disposition." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/subtree-lifecycle.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/fork-context.ts, and existing tests |
| scope.surface | internal | private Pi adapter transport and lifecycle modules only |
| scope.new_public_symbol | no | none planned; publishSubtree and writePrivateJson already exist |
| scope.new_type_contract | no | no new caller-facing type is required; effective snapshot comparison can remain publisher-private |
| scope.test_surface | existing | agents-plugin-pi/test/recursive-worker.test.ts, test/spawner.test.ts, and test/fork-context.test.ts |
| complexity.reuse_points | confirmed | publishSubtree, beginSubtreeDispatch, refreshObservedSubtree, watchObservedSubtree, and writePrivateJson are present in the cited source modules |
| complexity.side_effect_risk | high | an error boundary can otherwise suppress nested settlement or launch accounting |
| risk.correctness | high | the hard initial busy publication and conservative subtreeWaiting contract must remain distinct from later best-effort publication |
| risk.fit | moderate | the repair extends the existing private JSON transport and retry seam without changing transport selection |
| risk.test | high | Windows rename exhaustion, watcher callback containment, and primary-error preservation need deterministic fault injection |
| risk.security_or_contract | moderate | diagnostics must retain channel nonce validation while excluding prompt and transcript content |

## Phases

### Phase 1: Bound publication churn and isolate lifecycle handling

Add effective-snapshot deduplication to subtree publication, then establish explicit hard and soft publication boundaries: the initial busy transition remains mandatory, while observer refresh, event follow-up, watcher, and cleanup publications cannot mask or abort authoritative lifecycle processing. Add bounded retry-exhaustion diagnostics with the agreed transport identifiers.

Verify at least:

- Repeated publication of an identical effective snapshot performs one physical write.
- Meaningful dispatch, active, outstanding, delivery, descendant, nonce, and revision changes remain observable.
- Initial busy publication failure prevents grandchild launch and returns that failure.
- RPC event application and settlement still execute when a secondary publication fails.
- Watcher-triggered publication failure is contained and diagnosed rather than becoming uncaught.
- Cleanup publication failure preserves the original spawn failure.
- Retry exhaustion diagnostics contain path, PID, nonce, revision, and attempt count without sensitive content.
- Existing recursive waiting, nested gutter, fork, Explore, and execute-worker tests remain green, followed by the full Pi package suite.

### Result (820ef482) - 2026-09-21

Implemented effective-snapshot deduplication that advances only after successful replacement, preserving retries and every meaningful lifecycle transition. The initial busy publication remains a fail-closed spawn gate; later event, settlement, watcher, and cleanup publications are best-effort so they cannot mask authoritative lifecycle work or primary failures.

Added bounded retry-exhaustion diagnostics containing the private channel path, writer PID, nonce, revision, and attempt count without prompt or transcript data. Root sessions retain owner notifications, while nested RPC sessions receive the same failure through a non-recursive `ws-agent-advisory` fallback; diagnostic deduplication and its cap advance only after successful delivery.

Verification:

- `npm test -- test/fork-context.test.ts test/recursive-worker.test.ts test/spawner.test.ts`: 234 passed, 0 failed.
- `npm test`: 1,631 passed, 2 expected skips, 0 failed.
- `git diff --check`: passed.
- Partitioned correctness, fit, and test review completed. Round-one findings were fixed in `8992901f` and `820ef482`; round-two correctness and test verification reported no remaining findings.

Decisions:

- Kept the synchronous private JSON transport and bounded Windows rename retry instead of introducing a queue, lease, or alternate transport.
- Used direct Pi-session advisory delivery for nested diagnostics rather than `pushToLead`, which would republish the same failing subtree.
- Added a production-defaulted watcher-function seam so watcher failure coverage invokes the exact callback deterministically without platform-dependent `fs.watch` timing.


## Resolution (2026-09-21)

Implemented subtree snapshot deduplication, retained the initial fail-closed dispatch publication, isolated later lifecycle publication failures, and added bounded nested-session diagnostics. Focused and full Pi package suites pass; partitioned review completed with all round-one findings resolved and round-two verification clean.
