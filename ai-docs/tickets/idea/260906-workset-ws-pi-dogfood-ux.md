---
title: Pi adapter dogfood UX collection
---

# Pi Adapter Dogfood UX Collection

## Context

Collect the owner's Pi adapter UX requests in one ticket while dogfooding
inside Pi. More requests are forthcoming; record only supplied requests here.
The owner explicitly requested ticket capture only on 2026-09-06, superseding
the earlier instruction to implement report styling immediately.

## Tickets

- `260906-feat-ws-pi-tool-and-push-tui-polish` — ready; owns the implemented
  compact/muted push bodies and shared backgrounds. Its separate approval
  control acceptance remains; no duplicate body-styling ticket is created.
- `260909-feat-ws-pi-agent-row-model-and-usage` — ready;
  owns actual model/effort, latest-call input tokens and cumulative estimated
  USD with independent `—` fallbacks.
- `260909-feat-ws-pi-agent-count-panel-header` — ready;
  moves the count above the list without a duplicate agent-count footer.
- `260909-feat-ws-pi-report-header-distinction` — ready;
  owns the residual report-only header distinction, preserving the existing
  compact body and shared background.
- `260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner` — ready
  preparation; owns the separately confirmed 330ms bold/plain attention cue.

These are inclusions, not parent/child relationships. The workset itself stays
in `idea/`; implementation-ready status applies to each actionable ticket.

## Planned References

- **Goal stop/reset control** — Owner authorized ready preparation on
  2026-09-09. The remaining decision is whether stopping disarms only the goal
  reminder/rearm loop or also interrupts the current response and children.
  Create and promote the actionable ticket after that stop scope is confirmed.
  Do not interpret `/goal clear` as a new replacement goal in the intended UI.

## Focus

The owner approved ready promotion of this collection's concrete UX work on
2026-09-09; the earlier capture-only restriction is superseded for the included
slices. Review and promote those actionable tickets independently, keeping
existing live gates visible. Exact palette values are not fixed here.

`ws-ask` remains deferred as a deprecation candidate, together with the hidden
`ws-resolve` companion. Do not use or reactivate either for this workflow;
use the current owner conversation for unresolved decisions. The separate
`260908-research-ws-pi-ws-ask-removal` owns their eventual disposition.

## Exit Criteria

- Done: the owner ends collection and the recorded requests have explicit
  dispositions.
- Deferred: ws-ask disposition and any newly collected, unapproved requests;
  unresolved stop-scope decisions remain explicit during ready preparation.
