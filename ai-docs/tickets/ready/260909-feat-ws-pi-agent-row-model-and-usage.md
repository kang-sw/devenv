---
title: "Show agent model, effort, latest input tokens, and estimated cost in widget rows"
sage-review-design: completed
related:
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only, not parentage
  260905-feat-ws-pi-live-agent-widget: existing row ordering and display contract
spec:
  - pi-adapter-runtime
sage-review-completeness: completed
sage-review-design-reviewed: 09aabf4023b4e8be
sage-review-completeness-reviewed: 09aabf4023b4e8be
---

# Show agent model, effort, latest input tokens, and estimated cost in widget rows

## Background

The owner approved ready promotion of the dogfood collection's agent usage
visibility request on 2026-09-09. Rows currently expose identity, role, state,
and elapsed time but do not expose the resolved model or resource usage.

## Decisions

- Each agent row includes `<model-name> (<effort>)`, input tokens from its most
  recent model call, and cumulative estimated cost in USD attributable to that
  agent. Label the amount as an estimate, never actual subscription billing.
- Use the actual resolved model/effort when available. Unknown model, effort,
  latest-call usage, or cost fields display `—` independently; never invent
  zero usage or a price. A later spawn/resume/model change must not retain a
  stale model label.
- Latest-call input means the model usage's reported input-token field, not
  cumulative session tokens. Do not substitute a session total when per-call
  data is absent. Keep cache-token accounting faithful to Pi's usage fields.
- Cumulative cost counts child-attributable model calls once across continuation
  and dormant resume. Do not include the parent history inherited by a fork or
  double-add repeated/delta usage events. If a reliable child-only total cannot
  be recovered, show `—` rather than treating the inherited session total as
  this agent's cost.
- Collect usage outside `render(width)` from existing event/state surfaces.
  Rendering must stay side-effect free: no RPC calls, model requests, disk
  reads, or per-frame polling. Dormant/unreachable records use the most recent
  valid snapshot or `—`.
- Preserve row order, waiting-row visibility, cap semantics, question hints,
  and terminal-width bounds. This is human-facing display; no new model tool,
  billing integration, or change to delegation/settlement behavior.

## Evidence

`RpcAgentRecord` already stores modelBase/modelEffort. Pi RPC exposes session
statistics including cumulative tokens/cost, and model message events carry
usage, but current widget rows do not consume them. Inspect event aggregation
and resume provenance before choosing the smallest implementation; API presence
alone does not establish child-attributable cost.

## Spec Impact

Extend the live-agent-widget behavior in `pi-adapter-runtime` with the row
fields, latest-call versus cumulative distinction, estimate label and unknown
fallback. Record implemented behavior in the spec when this slice lands.

## Phases

### Phase 1: Add event-backed agent row telemetry

Capture normalized display snapshots through the existing child lifecycle,
render the settled fields above, and persist or re-derive valid snapshots for
resume without counting inherited history. No other ticket is a prerequisite.

Verification: model/effort resolution and missing values; two model turns where
latest input changes while estimated cost accumulates; repeated usage events
counted once; fork prefix exclusion; resume with and without recoverable usage;
40/80/120-column rendering; waiting rows and hints unchanged. Owner-live check:
a worker and a fork show actual model/effort, latest input, and estimated USD
or honest `—` values while work proceeds.
