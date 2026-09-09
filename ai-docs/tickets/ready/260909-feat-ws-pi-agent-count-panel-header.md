---
title: "Move the agent count above the agent list"
sage-review-design: completed
related:
  260906-workset-ws-pi-dogfood-ux: source collection; inclusion only
  260905-feat-ws-pi-live-agent-widget: existing count and list semantics
  260908-feat-ws-pi-attention-alert-when-agents-wait-on-owner: coordinate the count display destination without duplicating animation ownership
spec:
  - pi-adapter-runtime
sage-review-completeness: completed
sage-review-design-reviewed: 4cc99468a585c8d2
sage-review-completeness-reviewed: 4cc99468a585c8d2
---

# Move the agent count above the agent list

## Background

The owner wants the agent count visually grouped above the list instead of
appearing after another extension's footer status. The current implementation
uses separate below-editor widget rows and a footer setStatus count, so their
relative placement is not list ordering. Ready promotion was authorized on
2026-09-09 through the dogfood collection.

## Decisions

- Move the existing `ws: N agents` count, including its pending-question suffix,
  to the first line inside the existing agent widget, immediately above rows.
  Clear the old agent-count footer segment so there is one count presentation.
- Preserve the existing deduplicated, uncapped count semantics and question
  count. The heading is not an agent row and does not consume the row cap.
  Preserve hidden-running-row summaries and always-visible waiting rows.
- Hide the panel/count when there are no agents or pending questions. Preserve
  other extensions' widgets/footer segments and the separate goal-loop status.
- Keep 40/80/120-column bounds and live count refresh. Attention styling is
  owned by its existing ticket and follows the count to this destination;
  this ticket does not add a separate animation or config switch.

## Spec Impact

Update the live-agent-widget section of `pi-adapter-runtime`: count location
moves from the footer into the list heading; its numeric meaning is unchanged.

## Phases

### Phase 1: Render the count as the agent-panel heading

Reuse count construction and render it before the existing rows; remove only
the old agent-count status segment. No dependency on row telemetry or the
attention feature; coordinate the shared display seam if both land together.

Verification: empty, one, many/capped, pending-question-only and mixed rows;
count matches uncapped deduplicated rows; old footer segment cleared; unrelated
footer keys untouched; widths and refresh on spawn/settle/close. Owner checks
the count appears above the list with another usage widget installed.
