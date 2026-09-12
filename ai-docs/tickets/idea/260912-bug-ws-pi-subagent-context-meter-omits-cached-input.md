---
title: "Pi subagent row token value looks like context usage but omits cached input"
related:
  260909-feat-ws-pi-agent-row-model-and-usage: introduced the compact per-agent usage display
  260912-feat-ws-pi-custom-footer-cost-telemetry: adjacent aggregate telemetry surface; does not own per-agent context semantics
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

## Open Decision Queue

- Should the compact agent row replace the existing unlabeled latest-input value with context occupancy, label and retain latest input while adding context elsewhere, or remove latest input in favor of the planned custom-footer aggregate? Settle the intended owner-visible information density before ready promotion.

## Phases

### Phase 1: Make subagent token telemetry semantically honest across compaction

Settle the row's intended metric, then ensure its label, calculation, formatting, and compaction transition match that meaning. Verify cached-prefix-heavy calls, uncached calls, normal context growth, threshold compaction, the temporary null context-usage interval, dormant/resumed children, and providers with missing usage fields. Add regression coverage using the live-observed `342 + 61440` and `3150 + 68608` cases so neither can be rendered as a misleading `0.3k` or `3.1k` context value.
