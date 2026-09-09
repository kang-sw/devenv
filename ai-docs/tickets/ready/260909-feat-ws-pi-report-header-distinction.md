---
title: "Distinguish report headers within compact muted push messages"
sage-review-design: completed
related:
  260906-workset-ws-pi-dogfood-ux: source collection; includes this independent residual request
  260906-feat-ws-pi-tool-and-push-tui-polish: owns existing compact bodies and shared backgrounds; its code is present, separate live approval check remains
spec:
  - 260910-pi-report-header-distinction
sage-review-completeness: completed
sage-review-design-reviewed: 84e0e393abea1586
sage-review-completeness-reviewed: 84e0e393abea1586
---

# Distinguish report headers within compact muted push messages

## Background

The dogfood collection preserves a report-header visual-distinction request
outside the compact/muted/shared-background scope already implemented by the
polish ticket. The owner requested ready promotion of the remaining collection
items on 2026-09-09. Do not duplicate or reopen the existing body styling work.

## Decisions

- Give the `ws-agent-report` family header a distinct, theme-aware foreground
  treatment so reports are recognizable among other push families. Retain the
  existing status text; exact palette values are not fixed by the owner.
- Scope the change to the report header. Keep muted bodies, the common push
  background, ten-logical-line previews, full expansion, metadata, message
  payloads, ordering, delivery and wake/settle semantics unchanged.
- Preserve terminal sanitization, native cached rendering, width bounds and
  live theme behavior. Add no per-frame work proportional to full hidden bodies.
- Other push family headers retain their existing treatment. This does not
  alter the separate approval/question/error controls or their acceptance gate.

## Spec Impact

Extend `pi-adapter-runtime` push-render presentation with the report-header
foreground distinction while preserving the shared body/background contract.

## Phases

### Phase 1: Add report-only header distinction

Apply the theme-aware report header treatment through the existing shared push
renderer and document the visible behavior. The implemented polish renderer is
available; its pending approval-control live check does not block this slice.

Verification: report versus settled/question/approval/error headers; collapsed
and expanded reports; theme changes; 40/80/120-column bounds; payload/detail and
wake behavior unchanged; unchanged renders reuse existing preparation/layout.
Owner-live check: reports are easy to distinguish without making their bodies
compete with owner conversation under light and dark themes.

### Result (0ff4f980) - 2026-09-10

The registered report family now paints its header with Pi's existing
`customMessageLabel` role; other headers remain muted. Only renderer and test
files changed. The shared body/background, status, payload, family registration,
delivery, and wake paths remain intact. No standalone `ws-agent-error` family
was invented for testing; comparisons use the existing advisory/error surface.

Plan: `ai-docs/.plans/2026-09/10-0252-260909-feat-ws-pi-report-header-distinction.md`.
Implementation: `0ff4f980`; verification follow-up: `f0698044`.

Correctness review returned clean. Test review raised Important TST1 because the
original 40/80/120-column test counted lines through a width-ignoring fake.
The one relay replaced it with a long ANSI/multibyte report and physical-row
display-width assertions for collapsed and expanded output. TST1 is retained
as the implementer's `[fixed]` report with passing evidence; no Important
re-review ran. Fit reported one Minor: the component JSDoc still describes all
three bands as subdued despite the report-head exception. This comment-only
finding remains recorded without a Minor relay. No Critical findings occurred.

Final focused `node --test test/push-render.test.ts` from `agents-plugin-pi/`:
21/21 pass. Coverage includes registered-family role selection, bounded output,
logical preview/expansion, same-object theme mutation, and unchanged body
preparation/layout counters. Full `npm test`: 1,526 total, 1,396 pass, 130 existing
environment/stale-expectation failures; the package suite is not globally green.
`git diff --check` passed and the spawner diff is empty. Owner-live light/dark
readability acceptance has not been run.

## Blocked (2026-09-10)

Await the owner-live check under light and dark themes: compare reports with
other push families and confirm their headers are recognizable while muted
bodies remain subordinate to owner conversation. Automated rendering evidence
does not substitute for that visual acceptance. Keep the ticket in `ready/`
until the check is recorded; no additional implementation phase is proposed.
