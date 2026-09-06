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

None yet. This is a non-hierarchical collection, not an implementation target.

## Planned References

- **Report message visual distinction** — Make `ws-agent-report` easier to
  distinguish in the conversation using a separate header color and a subtle,
  theme-aware background. Keep other notification families unchanged and
  preserve model-visible message content and asynchronous delivery behavior.
  The owner selected this report-specific recommendation rather than changing
  colors for every notification family. Intended role: a rendering-only UX
  improvement. Creation condition: the owner chooses to move this collected
  request into implementation planning; do not create another ticket now.

## Focus

Capture additional owner-supplied UX requests in this same collection before
implementation planning. No styling implementation or queue promotion is
requested by this ticket. Specific palette values are not settled.

## Exit Criteria

- Done: the owner ends collection and the recorded requests have explicit
  dispositions.
- Deferred: implementation planning and any separate actionable tickets await
  the owner's direction; unspecified future requests are not inferred.
