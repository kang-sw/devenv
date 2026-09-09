---
title: "Distinguish report headers within compact muted push messages"
sage-review-design: completed
related:
  260906-workset-ws-pi-dogfood-ux: source collection; includes this independent residual request
  260906-feat-ws-pi-tool-and-push-tui-polish: owns existing compact bodies and shared backgrounds; its code is present, separate live approval check remains
spec:
  - pi-adapter-runtime
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
