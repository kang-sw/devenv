---
title: Pi custom cost footer can saturate the main thread after reload
related:
  260912-feat-ws-pi-custom-footer-cost-telemetry: regression source and intended footer contract
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 75d2e7efc6c99082
sage-review-completeness-reviewed: 75d2e7efc6c99082
completed: 2026-09-13
---

# Pi custom cost footer can saturate the main thread after reload

## Background

After the custom cost footer landed, a live Pi session became severely unresponsive after `/reload`. The same main Pi process (PID 6058) measured approximately 105.7% and 104.5% CPU during the degraded state. A one-second stack sample showed the main thread dominated by libuv timer callbacks with substantial `JSON.parse` work.

A diagnostic changed only the `agents-plugin-pi/src/index.ts` custom-footer activation call, leaving the rest of the extension active. After the owner performed an actual `/reload`, CPU fell to 29.6% and interactive responsiveness recovered. This A/B result strongly implicates the custom-footer path, while not yet proving whether the dominant cost is one active instance, reload duplication, reconciliation, render-time aggregation, or a combination.

Current evidence identifies two high-cost paths in `agents-plugin-pi/src/agent-footer.ts`:

- `watchDescendantCosts` runs a fixed 250 ms fallback reconciliation that recursively traverses ownership namespaces, reads ownership and roll-up artifacts synchronously, parses JSON, validates filesystem paths, and manages watchers.
- Every footer `render()` walks all lead session entries and performs another recursive descendant-cost aggregation with synchronous filesystem and JSON work.

The custom footer is temporarily disabled at its session-start activation seam pending a bounded fix.

## Decisions

- Treat footer cost as a cosmetic, bounded monotonic estimate for the current session rather than exact lifetime accounting.
- Cache one `evictedBaseline` plus cumulative telemetry snapshots for the current in-memory agent registry. Include running, idle, and dormant records; when a record is evicted, fold its last cumulative value into the baseline exactly once.
- Update cached cost only at explicit registry, telemetry, and session lifecycle boundaries. Do not poll, watch the filesystem, recursively discover ownership, reread session JSONL, or walk history for footer refresh.
- Persist one bounded checkpoint containing the baseline and current per-agent cumulative snapshots at lifecycle boundaries. Reload reads that one checkpoint once; it never reconstructs footer state through recursive scanning.
- Accumulate lead usage incrementally from newly accepted usage or message-end data. Footer rendering consumes preformatted cached state in O(1) time.
- Count directly tracked agents only. Nested descendant costs are outside this phase and must neither be inferred nor trigger recursive discovery; adding them later requires a separate child-to-parent cost protocol.
- Label displayed monetary values as estimates. Performance and responsiveness take precedence over exact accounting.

## Constraints

- Restore the intended lead/subagent cost footer after fixing the performance regression; permanent feature removal is not the goal.
- Preserve the production-disabled mitigation until the replacement passes automated reload/performance coverage and live responsiveness verification.
- Do not perform filesystem access, JSON parsing, history traversal, registry traversal, or telemetry reduction inside `render()`.
- Do not use fixed-interval polling or `fs.watch` for footer accounting.
- Bound checkpoint size by the registry cap; unique historical agent identities must fold into the scalar baseline rather than grow retained state without limit.
- Ensure reload and shutdown dispose callbacks/components and cannot duplicate accounting or refresh subscriptions.
- A missing, partial, regressing, or unknown cumulative snapshot must fail conservatively without subtracting prior accounted cost or inventing an exact value.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-footer.ts, agents-plugin-pi/src/index.ts, agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/spawner.ts |
| scope.surface | public-interface | the owner-visible TUI footer is re-enabled at the session-start seam in agents-plugin-pi/src/index.ts#L710-L712 |
| scope.new_public_symbol | no | no new externally callable adapter symbol is specified |
| scope.new_type_contract | yes | the bounded owner-scoped estimate checkpoint is a new persisted internal data contract |
| scope.test_surface | existing | agents-plugin-pi/test/agent-footer.test.ts covers watcher, aggregation, render, reload, and shutdown seams |
| complexity.reuse_points | confirmed | existing AgentTelemetry in agents-plugin-pi/src/agent-telemetry.ts and footer lifecycle in agents-plugin-pi/src/agent-footer.ts |
| complexity.side_effect_risk | high | re-enabling ctx.ui.setFooter after reload previously saturated the main Pi process |
| risk.correctness | high | monotonic direct-child accounting must survive reload, eviction, partial snapshots, and resume without duplication |
| risk.fit | high | the replacement must preserve the intended visible footer while removing its polling, watcher, and render-time I/O behavior |
| risk.test | high | automated reload/performance coverage and live CPU/responsiveness comparison are required |
| risk.security_or_contract | moderate | persisted owner-scoped estimates must not cross lead-session ownership or overstate an unknown total |

## Phases

### Phase 1: Replace footer scans with a bounded event-driven estimate

Remove the 250 ms reconciliation loop, filesystem watchers, recursive ownership aggregation, session JSONL rereads, and render-time lead-history traversal from the footer path. Implement the confirmed cached baseline-plus-current-registry estimate, incremental lead usage, one bounded lifecycle checkpoint, and direct event-driven refresh. Consume already-cached agent telemetry or newly accepted usage deltas; do not add whole-history refreshes to streaming `message_update`, render, or input-sensitive paths. Account final agent state at the post-reconciliation stop/dormant boundary rather than raw `agent_settled`. Re-enable production footer mounting only after verification passes.

Verify registry state transitions including running, idle, dormant, resume, repeated updates, eviction, alias reuse, unknown/partial costs, reload, compaction, shutdown, and repeated lifecycle start/stop. Prove render performs O(1) formatting without filesystem/history/registry access; checkpoint size remains bounded at the registry cap; reload neither double-counts nor decreases already-accounted baseline; direct-child scope is represented honestly; and missing nested totals trigger no discovery work. Run large retained-registry and long-session performance coverage, then live-compare CPU and responsiveness with the re-enabled footer against the diagnostic-disabled baseline.

### Result (2f833ec8) - 2026-09-13

Replaced the recursive 250 ms watcher/reconciliation loop and render-time history/filesystem traversal with cached O(1) presentation state. Lead usage now advances from accepted Pi message, compaction, and tree-summary events; direct-agent totals reconcile cached telemetry through a cost-specific registry event seam. One owner-scoped `.cost-estimate/checkpoint.json` retains lead totals, a scalar evicted baseline, and at most the live registry identities, including conservative monotonic handling for missing, partial, unknown, and regressing snapshots.

Eviction and ordinary-stop boundaries persist final reconciled telemetry. Eviction folding is transactional, owned-home deletion failures retain the candidate rather than risk a later duplicate retention fold, and failed checkpoint writes are surfaced and retained in process for reload retry. Footer mounting is re-enabled at the end of `session_start`; placing the asynchronous host-TUI import earlier caused Pi to snapshot tools before later question-tool registration.

Verification passed: `npm test` reported 1,771 passed, 0 failed, and 2 intentionally skipped; focused footer/spawner/ask coverage reported 510 passed; `npm pack --dry-run` completed with validated generated skills. Regression coverage includes 10,000 cached renders without history or registry access, 64 unique evictions folding into one scalar baseline, reload with an evicted baseline, partial and failed snapshots, failed checkpoint retry, and lifecycle disposal. A live PTY `/reload` comparison showed the diagnostic-disabled baseline at 1.14% mean / 3.4% maximum CPU across eight post-reload samples and the re-enabled custom footer visible with its final five one-second samples at 0%; no sustained saturation recurred.

Independent correctness, fit, and test reviews each completed two rounds. Round-one Important findings were fixed in `b4b9bdfd`, `2513323a`, and `3fe80fff`; all round-two verifiers returned clean with no remaining findings.


## Resolution (2026-09-13)

Replaced polling and render-time recursive accounting with bounded event-driven checkpoints, re-enabled the footer, resolved two-round correctness/fit/test review, and verified automated plus live `/reload` responsiveness.
