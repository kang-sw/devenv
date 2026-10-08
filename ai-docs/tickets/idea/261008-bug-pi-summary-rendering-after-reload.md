---
title: Diagnose display summary rendering after Pi reload or resume
related:
  261007-feat-pi-display-summary: original summary feature
  261008-feat-pi-display-summary-sidecar-persistence: persistence dogfood context
---

# Diagnose display summary rendering after Pi reload or resume

## Background

After merging sidecar persistence in 0243f8cfe, the user reloaded Pi and asked
for a tool call. A fresh git-status tool row did not visibly become summarized.
The user also suspects custom rendering generally stops after reload or
resume/continue. Whether the symptom affects restored rows, new rows, or both
is not yet established.

## Constraints

- Capture the observed dogfood defect without asserting a cause or repair.
- Distinguish sidecar loading, summarizer initialization/provider completion,
  and actual host renderer registration/rebuild before changing source.
- Offline persistence tests passed before this observation; they do not prove
  live TUI reload behavior. Do not treat removed/stale probe logs as current
  runtime evidence.
- Preserve the existing summary layout/background and the Previous conversation
  block exclusion; do not expose private conversation or summary contents in
  diagnosis logs.

## Phases

### Phase 1: Diagnose and repair reload/resume summary rendering

Establish affected row types and lifecycle steps using current Pi 1.0.4 host
and adapter evidence. Capture a reproducible failing boundary and a bounded
repair with lifecycle regression coverage before making this execution-ready.
Live rendering verification must distinguish new summary generation from
restoration of summaries that were actually persisted.
