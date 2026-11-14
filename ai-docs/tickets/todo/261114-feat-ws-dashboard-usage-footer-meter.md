---
title: Dashboard footer usage meter with per-window pace and stale-state display
related:
  261113-feat-ws-dashboard-rail-usage-gutter: Existing latest-value usage sources and footer/popover contract
  261114-feat-ws-dashboard-usage-history-trend: Separate phase-2 history and popover trend design
---

# Dashboard footer usage meter with per-window pace and stale-state display

## Background

MAIN relayed the owner's approved usage-meter direction on 2026-11-14,
whiteboard topic `usage-meter`, decisions (732), (737)-(740). This ticket
captures phase 1 of (738)=a: improve the footer using existing latest values
only. History storage and the popover trend chart are a separate design task.

Current source `agents-plugin-tool/internal/wsdashboard/plan_usage.go:21-40`
holds each window's kind, percent, reset time and observation time in latest
per-harness entries. `:59-66` persists those entries; `:101-103` replaces an
accepted Claude window rather than appending history. This is not a time
series and does not support drawing a real trend from past observations.

## Approved visual reference

Canonical portable reference:
[usage-meter-732 mockup](../assets/261114-feat-ws-dashboard-usage-footer-meter/mockup.html).
The HTML contains authored fixture data, not live account data. Its A section
is the phase-1 footer reference, including normal, over-pace, stale and
weekly-only variants. The B section is phase 2, not authority to add history
in this ticket. Preserve the original reference; the later owner-approved
chart color/fill correction is recorded in the phase-2 ticket.

Verification must compare result captures side by side against A's variants
at the same dimensions; an external cache path alone is not an acceptance
reference.

## Decisions

- (732): each harness has one column per available window (5-hour and weekly).
  The top row is current usage percentage; the smaller bottom row is the
  pace delta `usage percentage - elapsed-window percentage`. A negative
  delta is green and a positive delta is red. A weekly-only harness has one
  column, not an empty 5-hour placeholder.
- (737)=ok approves the mockup, with the graph correction assigned to the
  separate phase-2 chart task rather than the phase-1 footer.
- (738)=a: phase 1 is this latest-value footer meter; phase 2 is usage history
  plus a popover trend chart. Rejected: adding history storage or a fabricated
  historical line to the footer-only implementation.
- (739)=ok: after 30 minutes the value is stale and shows a clock icon,
  dotted treatment and neutral color rather than a fresh green/red cue.
- (740)=ok: the stale pace delta is frozen at its last observation, not
  recomputed from advancing wall time and an unchanged usage value.
- Compatibility: T0 dashboard asset/display content using existing latest
  values. No history-store shape, MCP, protocol or tracker-event changes.

## Constraints

- Preserve existing usage acquisition, credential handling, viewer access,
  source labels and reset-window semantics from the completed usage-gutter
  ticket. This display task does not authorize live account queries, new
  polling cadence, token refresh or account switching.
- Keep existing unavailable/missing-window states truthful; the visual mockup
  supplies fixtures, not missing runtime data. Ground unresolved edge cases
  at ready promotion rather than inventing new data acquisition behavior.
- No browser/daemon/build/test run is authorized by this documentation
  capture. Implementation and verification await MAIN's scheduling after
  the current Important pair and busy-board-read-gone work.
- Before product edits, read `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`.

## Prior Decisions

- `261113-feat-ws-dashboard-rail-usage-gutter` supplies the existing footer,
  usage sources, latest-value cache and privacy boundary. Its old thin-bar
  display is superseded here only by the approved footer presentation.

## Phases

### Phase 1: Render the approved latest-value footer meter

Implement the approved per-harness, per-window columns and pace/stale states
using existing values. Keep history collection and the trend popover out of
scope. Before ready promotion, populate exact source facts, settle any edge
cases not fixed by the confirmed contract, and complete the configured design
and completeness reviews.

Verification: deterministic fixtures for both windows, weekly-only data,
negative/positive pace, the 30-minute stale boundary, clock/dotted/neutral
stale styling, and delta frozen at the last observation as wall time advances.
Check normal and unavailable data without real credentials or network calls.
Capture the result and compare side by side with A's approved variants at
matching dimensions, including the narrow rail; preserve existing popover and
keyboard behavior. Run appropriate focused checks only in the assigned slot.
No implementation Result is claimed by this documentation capture.
