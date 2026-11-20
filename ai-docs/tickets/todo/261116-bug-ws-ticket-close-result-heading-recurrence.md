---
title: "중첩된 phase 결과에 대해 tickets.close가 여전히 Result 누락 경고를 내는 문제"
related:
  260917-bug-ticket-close-checker-result-heading-level-drift: "closed predecessor; fresh recurrence, not an edit to its historical resolution"
---

# tickets.close still warns about missing Result for nested phase outcomes

## Background

Dogfood finding #6, 2026-11-16. `tickets.close` warns `no ### Result` even when
the ticket has `#### Result` sections under its phases. The heading-level mismatch
produces a false unresolved-phase advisory from the caller's perspective.
The exact incident ticket/close output was not supplied; recover it before
claiming a new checker regression.

The closed predecessor records the same symptom and a 2026-11-08 resolution:
worker and verifier both use `### Result`. Today's ticket conventions also name
`### Result`. A nested heading is therefore evidence of producer/checker drift
or historical input, not authority to silently change the canonical heading.

## Reproduction

Use isolated ticket fixtures with `### Phase 1` followed by `#### Result`, then
compare `### Result`, a genuinely missing Result, fenced heading examples, and
multiple phases. Trace the exact authoring producer and close warning path.
Do not close a real unrelated ticket merely to collect the warning.

## Candidate fix directions

- Align the emitting worker/template with the current canonical heading if it
  still writes nested results.
- Consider tolerant recognition of historical nested results, with clear phase
  association, if that compatibility is desired.
- Share heading recognition between close/verify/query paths rather than letting
  their completion judgments drift.

## Decisions

Compatibility tier: **n/a** for detection/authoring consistency with unchanged
MCP arguments and ticket format. A deliberate change to persisted ticket
semantics or cross-version completion gates needs a concrete tier assessment
before readiness. This ticket does not choose a new canonical heading.

## Open questions

- Which producer wrote today's nested results, and on which runtime version?
- Should historical h4 results be accepted, normalized, or receive a specific
  format advisory distinct from an unresolved-phase warning?
- Which completion and prerequisite gates must agree on recognition?

## Phases

### Phase 1: Reconcile result production and completion detection

Recover the incident, compare the predecessor fix with current producers and
consumers, settle historical-heading treatment, and implement the smallest
consistent correction. Future verification covers both heading levels, actual
missing results, phase boundaries, fenced text, and close/verify/query agreement.
Preserve historical Result/Edition entries. No tests or close mutations are
performed during this docs capture.
