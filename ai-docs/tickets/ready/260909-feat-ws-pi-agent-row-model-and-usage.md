---
title: "Show agent model, effort, latest input tokens, and estimated cost in widget rows"
sage-review-design: completed
related:
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only, not parentage
  260905-feat-ws-pi-live-agent-widget: existing row ordering and display contract
spec:
  - pi-adapter-runtime
  - 260910-pi-agent-row-telemetry
plans:
  phase-1: ai-docs/.plans/2026-09/10-0023-260909-feat-ws-pi-agent-row-model-and-usage.md
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

### Result (df180ce2) - 2026-09-10

Implemented observed model/effort, latest reported input, and estimated
child-attributable USD in agent rows. Durable entry identities and a saved
pre-prompt fork boundary prevent inherited-cost inclusion and replay growth.
Both recovery formats reconcile snapshots; legacy forks without a lifetime
boundary retain an independent latest-input observation floor. Collection is
outside rendering, optional state queries preserve dispatch, and narrow rows
prioritize the owner's answer cue. The final source checkpoint is
`b45621ea0406bd986ff6f8d2e47369ca247c42a3` (review range starts at `17dea8a3`).

Verification:

- Final focused telemetry, spawner, sidecar, ask, and widget suites: 536 pass,
  zero failures. Command: from `agents-plugin-pi`,
  `env -u WS_PI_SPAWN_ROLE -u WS_PI_EXPLORE_MODE node --test test/agent-telemetry.test.ts test/agent-telemetry-contract.test.ts test/agent-telemetry-lifecycle.test.ts test/spawner.test.ts test/agent-sidecar.test.ts test/ask.test.ts test/agent-widget.test.ts`.
- Full `cd agents-plugin-pi && npm test`: 1469 total, 1339 pass, 130 fail,
  zero skipped. The failures are existing baseline categories: 126 hardcoded
  Linux SDK chunk checks, one Linux SDK resource-loader check, two test-file
  startup failures on the Linux SDK package path, and one stale `ws-ask`
  exposure assertion. The full suite is not clean; no unrelated repairs made.
- Elevated regressions reproduced four failures before correction: partial,
  missing, and unreadable history lost a valid fork origin, and collector
  state-query rejection retained old selection. After correction the lifecycle
  suite passes 21 tests, including six readable-provenance contradiction cases
  and separate stale-client and stale-generation rejection guards.
- Production-boundary fixtures exercise ordinary spawn, dormant send, event
  barriers, stop, actual shutdown persistence, and both recovery formats.
  Fresh task/discussion fork fixtures exercise their shared production
  telemetry collector; source inspection verified both consumers converge
  through `spawnAgent`. These fixtures do not claim full discussion UI-spawn
  integration or owner-live acceptance. Rendering checks include 40/80/120
  columns, independent unknown/zero, Unicode, answer priority, and no I/O.

Review dispositions:

- Correctness TC-C1, TC-C2, TC-C4, TC-C6, TC-C7, TC-C8 [fixed]: independently
  resolved by the bounded Critical review sequence, final review at `fbe27f67`.
- Correctness TC-C3 and TC-C5 [fixed]: still Critical at independent review 3,
  then elevated to the large-tier implementer. Commit `b45621ea` supplies the
  red/green fixes and final verification. This is an elevated implementer
  self-report, not a fourth independent review or a clean review-3 verdict.
- Correctness TC-I1 [fixed]: relay-1 implementer reports complete post-stop
  display/provenance persistence while retaining pre-stop orphan status.
- Test TEST-TELEMETRY-UI-002 [fixed]: implementer-reported renderer coverage,
  subsequently supplemented in `82394ef6`; no Important re-review.
- Test TEST-TELEMETRY-LIFECYCLE-001 [not fixed: relay 1 added no new lifecycle
  regressions]. The original fixed claim was corrected. Required verification
  remained incomplete, so the lead explicitly authorized a bounded exception
  to the one-relay budget for tests only; `82394ef6` supplies the missing
  recovery and shared-provenance regressions. This continuation is recorded
  separately from the original disposition and is not an Important re-review.
- Fit was clean; no Minor findings. All first-round Critical and Important
  findings were relayed together. Two Critical relays and three independent
  reviews were followed by the required elevated implementation.

Deviations and closeout:

- Survey escalated to research for durable entry identity and inherited-prefix
  attribution. Production histories use read-only JSONL parsing; SDK session
  operations were confined to isolated temporary research fixtures.
- Tests were consolidated into telemetry suites instead of all originally
  named files. A narrow production-used shutdown helper enabled direct
  persistence testing without starting the MCP bridge.
- Early implementation commit bodies contain literal newline escapes; later
  structured commits have normal AI Context headings. Preserve the corrective
  history and separate spec/ticket commits rather than rewriting provenance.
- The runtime spec contains the lifecycle and attribution rules; no duplicate
  mental-model document or project-orientation change is needed. No source
  inputs changed after the final suite evidence; documentation checks cover
  closeout. The unrelated untracked `06-1203` plan remains untouched.

## Blocked (2026-09-10)

Owner-live acceptance remains outstanding: observe one worker and one fork
showing actual model/effort, changing latest input, and estimated USD or honest
unknown values while work proceeds; confirm the waiting answer cue at narrow
width. Automated fixtures do not replace this gate. Keep this ticket in
`ready/` and skip it in unattended queue selection until that evidence arrives.
