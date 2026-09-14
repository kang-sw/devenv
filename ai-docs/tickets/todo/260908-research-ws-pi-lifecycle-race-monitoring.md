---
title: Pi lifecycle race monitoring and archived live-acceptance references
related:
  260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction: archived implementation and compaction/carry-forward acceptance evidence
  260906-bug-ws-pi-goal-reminder-races-child-push-at-settle: archived implementation and simultaneous-settle/provider-context acceptance evidence
  260906-bug-ws-pi-push-wake-can-run-before-report-delivery: archived implementation and provider request/response-count acceptance evidence
---

# Pi lifecycle race monitoring and archived live-acceptance references

## Background

During the 2026-09-08 dogfood inventory review, the owner requested closing
three implementation-complete race tickets rather than keeping separate
owner-monitoring gates in ready. This todo research ticket is the single
monitoring and recovery reference if a related issue recurs. It is not a new
implementation project, an automated monitoring feature, or evidence that the
outstanding live checks passed.

## Closure decision and evidence boundary

The owner accepts closing the three source tickets with their live acceptance
still unverified and consolidating that follow-up here. Their original phase
plans, Results, review evidence, and known limitations remain in the archived
tickets. Their closure resolutions supersede the historical instructions to
keep each ticket in ready until those gates pass. No provider trace, race
reproduction, token-savings measurement, or new test run is claimed by this
inventory cleanup.

## Monitoring references

### Compaction, report delivery, and verbatim carry-forward

Reference: `260906-bug-ws-pi-goal-loop-reinject-races-manual-compaction`.
Recorded implementation: Results `81463a7d` and `bfcf850b`; the latter records
118 focused tests and 894 full-suite tests passing.

Unverified live observations retained for a recurrence:

- Near-empty one-word goal: inspect compaction failure handling rather than
  assuming a failed compaction is harmless.
- Filled-session lever compaction: verify a late summary does not replace
  post-lever conversation and a child report arriving during compaction is
  delivered afterwards.
- Supply a distinctive carry-forward string and compare it byte-for-byte in
  the next eligible goal reminder. Earlier owner input or held pushes can
  precede that reminder; paraphrase or a TUI notification is not proof.

The original ticket explicitly accepts the owner-typed `/compact` pre-hook
window while a goal is armed; the documented goal compaction lever is the
intended path. Archival does not claim that accepted window was fixed.

### Simultaneous settle and provider-context continuity

Reference: `260906-bug-ws-pi-goal-reminder-races-child-push-at-settle`.
Recorded implementation: Results `271949cb` and `438f2f0b`; the latter records
908 full-suite tests passing.

Unverified live observations retained for a recurrence:

- A child and lead settling together during a goal run must not produce an
  `already processing` error or runaway reminder loop; Esc must still interrupt
  the real run, with the settling status visible.
- After a child push wakes an idle lead, inspect actual provider/system context
  on a subsequent model call in the same run for the manual block and fresh
  skill list. Offline hook wiring is not proof of provider-context lifetime.

### Blind push wakes and request/response counts

Reference: `260906-bug-ws-pi-push-wake-can-run-before-report-delivery`.
Recorded implementation: Result `9b992772` and relay `f7226a7`; focused 509
and full 935 tests, pack and diff checks are recorded passing.

Unverified live observations retained for a recurrence:

- One report to an idle lead: inspect that the first model request includes
  the report rather than generating a bare acknowledgement first.
- Two reports in a controlled no-tool-call run: inspect FIFO delivery and no
  more than one response per report. Pi's one-at-a-time steering need not put
  the entire batch before the first response.
- The predecessor's provider-context and simultaneous-settle observations
  remain relevant; neither measured token savings nor live count acceptance
  is established by the offline queue model.

## Scope boundaries

This consolidation changes the inventory and acceptance ownership only; it
changes no runtime, notification delivery, display policy, or safety guard.
The separate mirror-drift failure remains open, and compact muted push
presentation is a separate feature. No additional implementation is authorized
by this monitoring ticket.
