---
title: Pi custom cost footer can saturate the main thread after reload
related:
  260912-feat-ws-pi-custom-footer-cost-telemetry: regression source and intended footer contract
---

# Pi custom cost footer can saturate the main thread after reload

## Background

After the custom cost footer landed, a live Pi session became severely unresponsive after `/reload`. The same main Pi process (PID 6058) measured approximately 105.7% and 104.5% CPU during the degraded state. A one-second stack sample showed the main thread dominated by libuv timer callbacks with substantial `JSON.parse` work.

A diagnostic changed only the `agents-plugin-pi/src/index.ts` custom-footer activation call, leaving the rest of the extension active. After the owner performed an actual `/reload`, CPU fell to 29.6% and interactive responsiveness recovered. This A/B result strongly implicates the custom-footer path, while not yet proving whether the dominant cost is one active instance, reload duplication, reconciliation, render-time aggregation, or a combination.

Current evidence identifies two high-cost paths in `agents-plugin-pi/src/agent-footer.ts`:

- `watchDescendantCosts` runs a fixed 250 ms fallback reconciliation that recursively traverses ownership namespaces, reads ownership and roll-up artifacts synchronously, parses JSON, validates filesystem paths, and manages watchers.
- Every footer `render()` walks all lead session entries and performs another recursive descendant-cost aggregation with synchronous filesystem and JSON work.

The custom footer is temporarily disabled at its session-start activation seam pending a bounded fix.

## Constraints

- Restore the intended lead/subagent cost footer after fixing the performance regression; permanent feature removal is not the goal.
- Do not perform a synchronous recursive ownership/filesystem scan on every TUI render.
- Do not run a fixed 250 ms full-tree reconciliation regardless of activity.
- Preserve exact descendant cost totals across registry changes, retention, eviction, reload, and nested ownership.
- Ensure `/reload` disposes every prior footer watcher, timer, callback, and component before a replacement can arm.
- Keep expensive aggregation outside latency-sensitive input and render paths; rendering should consume bounded cached state.

## Phases

### Phase 1: Make footer telemetry event-driven and render-bounded

Profile the footer lifecycle and aggregation paths to isolate the dominant hot loop and verify whether reload can duplicate resources. Replace unconditional full-tree polling and render-time recursive aggregation with bounded cached state refreshed by explicit registry/telemetry events and narrowly scoped filesystem observation. Retain a low-frequency or targeted correctness fallback only if evidence shows it is required, and ensure it cannot overlap or continuously saturate the event loop.

Add regression coverage for repeated reload start/stop cycles, complete timer/watcher disposal, large retained ownership trees, long lead transcripts, nested descendants, and typing/render latency under unchanged telemetry. Live-verify CPU and responsiveness with the footer enabled against the diagnostic-disabled baseline.
