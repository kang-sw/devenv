---
title: "Show detailed telemetry for descendants in the Pi agent gutter"
related:
  260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter: predecessor; recursive identity propagation and nested rendering already implemented
  260924-feat-pi-agent-channel-subtree-state: constrains; current revisioned subtree transport and lifecycle fences
  261002-feat-pi-model-output-tps-display: constrains; reuse existing TPS semantics while extending display coverage to descendants
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
  descendants are rendered with liveness-only state and depth gutters.
- `agents-plugin-pi/src/subtree-lifecycle.ts`: descendant identity is bounded
  and forwarded hop by hop. The current identity contract does not carry the
  detail needed by the local row formatter.
- `agents-plugin-pi/src/spawner.ts`, `observeChildSubtree`: accepted child
  snapshots update cached descendant identity, propagate it upstream, and
  refresh the widget.
- `260924-feat-pi-agent-channel-subtree-state` retired the old snapshot-file
  watcher. The current implementation uses the parent-child control channel.

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
- **Capture only for now.** This invocation creates accepted backlog in
  `todo/`; it does not promote the ticket or start implementation.

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
- No new wire schema, refresh cadence, or storage mechanism is selected in this
  capture. Ground the implementation contract before `ready/` promotion and
  settle any resulting product or protocol choices through the Open Decision
  Queue rather than assuming them.

## Prior Decisions

- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (2026-09-20,
  commit ab8b02f6): "Render one dim gutter lane per depth and omit clocks and
  telemetry for propagated rows because the Phase-1 transport does not carry
  those facts" — bearing: explains the existing detail gap; this ticket
  explicitly extends that display scope.
- 260914-feat-ws-pi-subagent-subtree-propagation-and-nested-gutter (Decisions
  D2, D5): descendant identity is advisory and decoupled from authoritative
  counts; cross-process descendants are non-openable picker context — bearing:
  constrains lifecycle and access behavior.
- 260924-feat-pi-agent-channel-subtree-state (Decisions): the whole revisioned
  snapshot moves to the channel, reconnect sends its latest state, and
  clearing a direct child's live state removes its cached descendants —
  bearing: constrains transport integration and stale-row cleanup.
- 261002-feat-pi-model-output-tps-display (Decisions D1-D3, D5): TPS uses the
  existing visible-output metric and eligible-message window; propagated
  liveness-only rows were excluded; narrow rows drop TPS first — bearing:
  preserves the metric and presentation while this ticket expands coverage.

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
- Missing telemetry and insufficient TPS samples retain existing omission or
  unknown-value behavior instead of fabricated values.
- Parent grouping, depth gutters, narrow-width fallback, existing row caps,
  and local row presentation remain intact.
- Reconnect, relaunch, and teardown cannot regress details to a previous
  launch or leave a descendant visibly live after its owning process exits.
- Existing wait/settle, dispatch-fence, bounds, and non-openable `/audit`
  behavior remain green without weakening their assertions.
- An interactive nested-agent smoke confirms the detailed row in the actual
  Pi gutter, in addition to automated propagation and formatter coverage.
