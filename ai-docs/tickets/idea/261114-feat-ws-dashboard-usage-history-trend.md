---
title: Design usage history storage and a baseline-colored popover trend chart
related:
  261114-feat-ws-dashboard-usage-footer-meter: Phase-1 latest-value footer; this is the separate phase-2 scope
  261113-feat-ws-dashboard-rail-usage-gutter: Existing latest-only source, privacy boundary and popover
---

# Design usage history storage and a baseline-colored popover trend chart

## Background

MAIN relayed owner decision (738)=a on 2026-11-14: deliver the latest-value
footer first, then usage history storage and a popover trend chart. Current
`agents-plugin-tool/internal/wsdashboard/plan_usage.go:21-40,59-66,101-103`
holds and persists latest window values, not a series. Past observations
cannot be reconstructed from that state.

This is the phase-2 design capture. Storage design and its compatibility tier
must be reported to MAIN and settled before implementation.

## Approved visual reference

[usage-meter-732 mockup](../assets/261114-feat-ws-dashboard-usage-footer-meter/mockup.html),
section B, is the approved popover layout and cumulative-usage reference.
Its history is explicitly authored fixture data, not runtime evidence.

The owner approved (737) with this correction: the actual line should
transition gradually toward red above the ideal-burn baseline and toward
green below it; the area above the baseline should have a translucent red
cellophane-like fill. The original mockup's blue line/fill is superseded by
that correction, not authority to retain a fixed blue line in the result.

Verification must compare matching-dimension result captures side by side
against B and separately demonstrate the corrected line/fill behavior.

## Decisions

- (738)=a: history plus the popover trend belongs to phase 2, separate from
  the T0 latest-value footer. Rejected: implementing storage implicitly in
  the footer change or treating fixture history as collected observations.
- (737)=ok with the explicit line-color and above-baseline fill correction
  recorded above. The comparison reference is the ideal-burn baseline, not
  a separately invented fixed safety threshold.
- (739)/(740) remain applicable when showing latest values: after 30 minutes
  use clock/dotted/neutral stale treatment and freeze stale pace delta at
  the last observation. Stale data must not imply a fresh measurement.
- Compatibility tier for history storage is not settled. Report the proposed
  store shape and mixed-version behavior before choosing a tier or implementing
  it; this ticket does not inherit T0 merely from the footer's T0 scope.

## Constraints

- Do not choose a history schema, retention policy, collection cadence,
  cross-account identity scheme or migration behavior in this capture.
  These remain design work requiring settlement, not implementation defaults.
- Preserve the existing usage-source and privacy boundary. Auth documents,
  tokens, account identifiers and subprocess output are not chart history.
- Reference charts contain fictional samples. A real trend must be based on
  collected observations; the approved reference's no-history state uses the
  current observation and baseline rather than fabricating a trend.
- No product edits, storage migration, live account query, browser/daemon
  activation or runtime test is authorized by this documentation capture.
- Before product edits, read `ai-docs/manuals/shipped-surface-boundary.md`
  and `ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Settle history storage and the corrected trend contract

Trace existing latest-value acceptance, reset-window identity and popover
rendering. Report a proposed observation-history design and compatibility tier
through MAIN before implementation; settle unresolved storage, lifecycle and
mixed-reader/writer choices. Preserve the approved layout, cumulative-usage
meaning and gradual baseline-relative colors without inventing a numerical
color-transition formula as an already-confirmed decision.

The settled verification plan must cover real observations versus absent
history, window resets, stale values and frozen delta, and both sides of a
baseline crossing. Demonstrate gradual red/green line changes and red
translucent fill only above the baseline, with side-by-side visual comparison
to B plus the explicit correction. Storage tests must match the chosen
compatibility/migration contract. This idea is not ready for implementation;
no implementation Result is claimed.
