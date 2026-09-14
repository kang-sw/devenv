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
- `260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner` — ready;
  owns the confirmed 330ms bold/plain attention cue, question title plus valid
  command hint, and approval-wait label.
- `260909-feat-ws-pi-goal-stop-controls` — ready;
  owns stop/clear/reset aliases that disarm goal continuation while preserving
  the current response, children and history.

These are inclusions, not parent/child relationships. The workset itself stays
in `idea/`; implementation-ready status applies to each actionable ticket.

## Planned References

- None. The owner confirmed the remaining stop scope on 2026-09-09 and the
  actionable goal-control ticket now owns it.

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
- Deferred: ws-ask disposition and any newly collected, unapproved requests.
