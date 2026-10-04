---
title: "Show detailed telemetry for descendants in the Pi agent gutter"
related:
  260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter: predecessor; recursive identity propagation and nested rendering already implemented
  260924-feat-pi-agent-channel-subtree-state: constrains; current revisioned subtree transport and lifecycle fences
  261002-feat-pi-model-output-tps-display: constrains; reuse existing TPS semantics while extending display coverage to descendants
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d508e329bc284188
sage-review-completeness-reviewed: d508e329bc284188
---

# Show detailed telemetry for descendants in the Pi agent gutter

## Background

The Pi gutter already shows grandchildren and deeper descendants as a nested
tree. The remaining gap is detail: direct children show names, classified
states, clocks, model information, context tokens, cost, and output TPS, while
propagated rows show only a short ID, role, and process liveness.

The user clarified that the desired change is detailed information, not adding
previously absent tree rows, and requested ticket capture before implementation.

Current evidence:

- `agents-plugin-pi/src/agent-widget.ts`, `buildAgentTree`, `buildAgentRows`,
  and `formatRow`: local records retain rich row information; propagated
  descendants are rendered with liveness-only state and depth gutters
  (`agents-plugin-pi/src/agent-widget.ts#L205-L275`,
  `agents-plugin-pi/src/agent-widget.ts#L351-L411`,
  `agents-plugin-pi/src/agent-widget.ts#L507-L541`).
- `agents-plugin-pi/src/subtree-lifecycle.ts`: descendant identity is bounded
  and forwarded hop by hop. The current identity contract does not carry the
  detail needed by the local row formatter: `SubtreeDescendant` has only
  `id`, `parentId`, `depth`, `role`, and `live`; the bounds are 128 rows and
  depth 8 (`agents-plugin-pi/src/subtree-lifecycle.ts#L24-L54`,
  `agents-plugin-pi/src/subtree-lifecycle.ts#L264-L284`).
- `agents-plugin-pi/src/spawner.ts`, `observeChildSubtree`: accepted child
  snapshots update cached descendant identity, propagate it upstream, and
  refresh the widget (`agents-plugin-pi/src/spawner.ts#L2744-L2762`).
- `260924-feat-pi-agent-channel-subtree-state` retired the old snapshot-file
  watcher. The current implementation uses the parent-child control channel
  (`agents-plugin-pi/src/subtree-lifecycle.ts#L190-L223`; scope-bounded search
  for `subtree.json` and `fs.watch` in `agents-plugin-pi/src/spawner.ts`
  returned no matches).

## Decisions

- **Detailed descendant rows.** Grandchildren and deeper propagated descendants
  should show the same kinds of gutter information as direct children: name,
  detailed state, model and effort, elapsed and active time, context-token
  value, estimated cost, and output TPS. Use the existing direct-child display
  semantics rather than introducing different meanings for nested rows.
  The short-ID/role/liveness-only presentation is no longer the intended
  steady-state experience when those details are available.
- **Keep the tree.** Preserve the existing depth gutters and parent grouping;
  richer information does not flatten the tree.
- **Display only.** Do not add descendant control, command approval authority,
  or cross-process conversation opening. Propagated `/audit` context remains
  non-openable, and locally owned children retain their existing affordances.
- **Attribution (1).** Each descendant's detail comes from that agent's own
  direct-child record in its owning process, not an ancestor's aggregate
  subtree usage or clocks.
- **State authority (2).** Forward the owning process's existing classified
  row state as advisory display metadata. Ancestors do not reconstruct
  lifecycle authority from that value.
- **TPS source (3).** Measure each descendant in its direct parent's existing
  RPC event tracker and forward the resulting display value. Do not forward
  raw token deltas to the root or introduce a different rate metric.
- **Wire and storage (4).** Add an optional detailed-display block to each
  descendant identity on the existing revisioned subtree channel. No new
  persistent store is introduced; absent blocks retain the legacy row
  fallback. Rejected: a separate display transport/cache or independent
  persistent descendant store, because existing bounds, ordering, reconnect,
  and teardown already cover this path.
- **Publication (5).** Publish state, identity, and teardown changes promptly;
  coalesce telemetry-only changes to at most one update per second, with at
  most one second of telemetry publication lag. Advance displayed clocks
  locally from owner-provided timing facts. Do not add per-token upstream
  messages or idle whole-tree polling, and never delay busy-fence publication
  behind the display throttle. Rejected: completion-event-only updates, which
  miss changes during long runs; periodic whole-tree polling, which restores
  unnecessary scanning and work.
- **Fallback and freshness (6).** Absent or incomplete detail falls back to
  the existing liveness/short-ID row and omits unknown telemetry. Retain the
  latest accepted detail within a launch until replacement or existing
  disconnect/relaunch/teardown invalidation; add no wall-clock expiry.
  Channel and generation validity gate accepted detail. Cached display detail
  must not grant new live-state authority during a disconnect. Rejected: a
  new freshness TTL/stale marker or suppressing incomplete rows, because the
  existing validity boundaries suffice and useful identity should remain
  available.
- **Promotion, not execution.** The user requested `ready/` promotion with
  one design-review pass. This ticket-authoring invocation does not start
  implementation.

## Constraints

- Preserve the predecessor's separation between advisory display data and
  authoritative wait/settle accounting. Detailed display state must not become
  an input to settlement, dispatch admission, parking, or delivery decisions.
- Preserve the current transport's launch-generation and revision fences,
  reconnect behavior, depth/breadth bounds, and invalidation of cached
  descendants on direct-child teardown.
- Preserve existing telemetry meanings and unavailable-value handling. In
  particular, descendant TPS must retain the existing eligible-message,
  window, model-reset, and omission semantics; it must not be inferred from
  aggregate subtree cost, token totals, or wall time.
- Preserve the prior prohibition on per-token subtree publication and gutter
  fan-out, and avoid restoring recursive session-history scans for rendering.
- `ai-docs/manuals/shipped-surface-boundary.md` applies to shipped Pi package
  text changed during implementation, as already declared by
  `261002-feat-pi-model-output-tps-display`.
- Decisions (1)-(6) settle the display-source, transport, publication, and
  fallback contracts. Concrete field/type names and implementation wiring may
  follow existing adapter patterns without changing those contracts.

## Prior Decisions

- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20, commit ab8b02f6): "Render one dim gutter lane per depth and omit clocks and telemetry for propagated rows because the Phase-1 transport does not carry those facts" — bearing: explains the existing detail gap; this ticket explicitly extends that display scope.
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (Decisions D2, D5): descendant identity is advisory and decoupled from authoritative counts; cross-process descendants are non-openable picker context — bearing: constrains lifecycle and access behavior.
- 260924-feat-pi-agent-channel-subtree-state (Decisions): the whole revisioned snapshot moves to the channel, reconnect sends its latest state, and clearing a direct child's live state removes its cached descendants — bearing: constrains transport integration and stale-row cleanup.
- 261002-feat-pi-model-output-tps-display (Decisions D1-D3, D5): TPS uses the existing visible-output metric and eligible-message window; propagated liveness-only rows were excluded; narrow rows drop TPS first — bearing: preserves the metric and presentation while this ticket explicitly expands coverage.
- 261002-feat-ws-config-adapter-schema-extension (2026-10-02, commit 1949b0e5): "the widget reads on each refresh() and its 330ms/10s ticks reuse that answer rather than calling ws-mcp per frame." — bearing: constrains
- 261002-feat-pi-model-output-tps-display (2026-10-02, Decisions): "When the row is too narrow, the TPS segment is dropped first and the rest of the telemetry group is kept" — bearing: constrains
- 260926-bug-pi-telemetry-refresh-full-session-reparse (2026-09-26, Result): "`npm test -- test/agent-telemetry*.test.ts test/agent-usage-rollup*.test.ts test/eviction-records.test.ts`: 132/132 pass." — bearing: supports
- 6f2ccffb (2026-09-24, commit): "The child knows exactly: a push wake reservation or an agent_end boundary batch owes a turn" — bearing: constrains
- 260921-feat-pi-gutter-two-clock-semantics (2026-09-21, Result): "Stream deltas update only that field and defer subtree observation, publication, and telemetry refresh, preventing local and parent-watcher gutter fan-out per token." — bearing: constrains
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20, Result): "Propagated descendants remain liveness-only and non-openable; dormant propagated rows are omitted from the live gutter." — bearing: constrains
- 260916-feat-pi-agent-gutter-active-time-placement (2026-09-16, commit da578281): "Activity now belongs to the duration field even when the remaining all-or-nothing telemetry group does not fit; protected owner cues and inspection hints retain priority." — bearing: constrains
- 260912-feat-ws-pi-custom-footer-cost-telemetry (2026-09-13, Result): "Child-attributable telemetry now persists in ownership metadata." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-pi/src/agent-widget.ts, agents-plugin-pi/src/subtree-lifecycle.ts, agents-plugin-pi/src/spawner.ts |
| scope.surface | cross-module | SubtreeDescendant and AgentTreeNode are exported adapter-internal contracts consumed across the subtree observer and gutter; no ws-mcp surface is selected. |
| scope.new_public_symbol | unknown | Concrete exported symbol names are not fixed; implementation may extend existing adapter types. |
| scope.new_type_contract | yes | Decision (4) adds an optional detailed-display block to the existing SubtreeDescendant identity contract (subtree-lifecycle.ts#L24-L31). |
| scope.test_surface | existing | agents-plugin-pi/test/agent-widget.test.ts, test/recursive-worker.test.ts, test/subtree-lifecycle.test.ts, test/spawner.test.ts, test/audit.test.ts, test/output-rate.test.ts exist; detailed propagated-row coverage must be added. |
| complexity.reuse_points | confirmed | buildAgentTree/buildAgentRows/formatRow in agent-widget.ts; observeChildSubtree in spawner.ts#L2744-L2762; publishSubtree/observeSubtreeChannel in subtree-lifecycle.ts; existing output-rate tracker semantics read. |
| complexity.side_effect_risk | high | Advisory detail must propagate through the same observer/publication spine that carries settlement and busy-fence state without per-token fan-out. |
| risk.correctness | high | Rich state and telemetry must remain coherent across reconnect, relaunch, and teardown without affecting lifecycle accounting or inventing unavailable values. |
| risk.fit | moderate | Reuses the nested tree, local formatter, per-direct-child TPS tracker, and revisioned snapshot; Decisions (1)-(6) settle their display sources and integration boundaries. |
| risk.test | high | Existing tests pin liveness-only descendants; parity requires multi-hop updates with an idle parent, stale-launch fencing, width fallback, and an interactive nested smoke. |
| risk.security_or_contract | high | The confirmed richer cross-process display contract must preserve existing fences and bounds; metadata must not grant settlement, approval, control, or conversation-opening authority. |

## Phases

### Phase 1: Detailed nested gutter rows

Make direct-child-equivalent display information available to the root gutter
for propagated descendants and render it under the existing parent/depth tree.
Cover every field listed in Decisions without changing locally owned rows or
promoting descendant display information into lifecycle authority.

Verification boundary:

- A root, direct child, and grandchild demonstrate the grandchild's name,
  classified state, model/effort, elapsed/active time, context, cost, and
  eligible output TPS with the same meanings as the direct-child row.
- Updates reach the root when its direct child has no new model output;
  details are not dependent on incidental root-side RPC activity.
- Per-agent cost and clocks remain distinct from subtree totals; state comes
  from the owning process and TPS matches its existing direct-child tracker.
- Telemetry-only publication is bounded to one update per second with a
  trailing update for the latest change; state edges and busy-fence publication
  remain prompt, and displayed clocks advance without per-token propagation.
- Absent/legacy detail, incomplete telemetry, and insufficient TPS samples
  retain the confirmed fallback instead of fabricated values or hidden rows.
- Parent grouping, depth gutters, narrow-width fallback, existing row caps,
  and local row presentation remain intact.
- Reconnect, relaunch, and teardown cannot regress details to a previous
  launch or leave a descendant visibly live after its owning process exits.
- Existing wait/settle, dispatch-fence, bounds, and non-openable `/audit`
  behavior remain green without weakening their assertions.
- An interactive nested-agent smoke confirms the detailed row in the actual
  Pi gutter, in addition to automated propagation and formatter coverage.
