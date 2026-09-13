---
title: "Pi subagent row token value looks like context usage but omits cached input"
related:
  260909-feat-ws-pi-agent-row-model-and-usage: introduced the compact per-agent usage display
  260912-feat-ws-pi-custom-footer-cost-telemetry: adjacent aggregate telemetry surface; does not own per-agent context semantics
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7113fc60273a2d60
sage-review-completeness-reviewed: 7113fc60273a2d60
---

# Pi subagent row token value looks like context usage but omits cached input

## Background

During a long-running bootstrap fork, the compact token value visibly moved from roughly `30k` to `0.6k`, then between `0.3k` and `3.1k` after Pi compacted the session. This looked like repeated compaction far below the model's context limit even though the session JSONL showed one normal fork-local compaction at `tokensBefore: 258094`, followed by context growth from approximately 52k to 78k.

The displayed values exactly matched the latest assistant call's uncached `usage.input`: `342 → 0.3k` and `3150 → 3.1k`. The same calls carried `cacheRead` values of `61440` and `68608`, making their prompt-input totals approximately `61.8k` and `71.8k`. The unlabeled value therefore oscillates with cache boundaries rather than representing context occupancy.

Current source intentionally records only `usage.input` as `latestInput` in `agents-plugin-pi/src/agent-telemetry.ts`, replaces it on every assistant call, formats it as `<n.n>k` in `agents-plugin-pi/src/agent-widget.ts`, and renders it without a semantic label. Existing tests pin the exclusion of cache and summary aggregate input, so this is a caller-visible contract problem rather than a transient compaction failure.

## Decisions

- Do not diagnose or present `latestInput` as current context-window occupancy.
- A context meter must use Pi's context-usage source when available. The usage fallback is `usage.totalTokens`, or `usage.input + usage.output + usage.cacheRead + usage.cacheWrite` when the total is absent.
- Handle the documented `ContextUsage.tokens === null` interval around compaction explicitly rather than briefly presenting a false near-zero context value.
- Preserve the distinction between per-call uncached input, cumulative cost/usage, and current context occupancy. If more than one is retained, each must be visibly labeled and tested as a separate metric.
- Replace the compact agent row's existing latest uncached-input value with current context occupancy. Do not retain latest uncached input elsewhere in the compact row.
- Provider differences are expected: providers without prompt caching may report `usage.input` close to occupancy, while cache-aware providers split most prompt tokens into `cacheRead` or `cacheWrite`. The row must normalize those representations rather than inherit provider-specific apparent behavior.

## Constraints

- Prefer Pi's authoritative `ContextUsage.tokens` value when available.
- When authoritative context usage is unavailable, fall back first to `usage.totalTokens`, then to `usage.input + usage.output + usage.cacheRead + usage.cacheWrite` using only present nonnegative fields.
- During compaction-time `tokens === null` or an otherwise unknown interval, retain the last valid occupancy for the same agent/session generation or render `?`; never replace it with a misleading near-zero uncached-input value.
- Reset retained occupancy when agent/session identity changes so stale values cannot cross a resumed or replaced session.
- Keep the compact row width behavior and truncation guarantees intact.

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-telemetry.ts, agents-plugin-pi/src/spawner.ts, agents-plugin-pi/src/agent-widget.ts, and existing telemetry/widget tests |
| scope.surface | public-interface | owner-visible compact subagent row metric |
| scope.new_public_symbol | no | no new public symbol requested |
| scope.new_type_contract | yes | AgentTelemetry occupancy snapshot and row projection |
| scope.test_surface | existing | agents-plugin-pi/test/agent-telemetry-contract.test.ts, agents-plugin-pi/test/agent-telemetry.test.ts, and agents-plugin-pi/test/agent-widget.test.ts |
| complexity.reuse_points | confirmed | RpcClient.getSessionStats() exposes contextUsage and current telemetry refresh already calls RpcClient.getState() |
| complexity.side_effect_risk | moderate | telemetry refresh must preserve a valid occupancy across compaction and session replacement |
| risk.correctness | high | cached-input-heavy calls and compaction null intervals can display a false occupancy |
| risk.fit | high | the compact row must preserve its independent cost metric and width and truncation contract |
| risk.test | high | regression coverage spans cached and uncached usage, compaction, resume, missing fields, and row rendering |
| risk.security_or_contract | moderate | the owner-visible token value changes from latest input to a distinct context-occupancy contract |

## Phases

### Phase 1: Make subagent token telemetry semantically honest across compaction

Settle the row's intended metric, then ensure its label, calculation, formatting, and compaction transition match that meaning. Verify cached-prefix-heavy calls, uncached calls, normal context growth, threshold compaction, the temporary null context-usage interval, dormant/resumed children, and providers with missing usage fields. Add regression coverage using the live-observed `342 + 61440` and `3150 + 68608` cases so neither can be rendered as a misleading `0.3k` or `3.1k` context value.
