---
title: "Whiteboard topic history: no inner scroll; load older entries upward in place"
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "(473) board size and performance; the collapse keeps loaded history from accumulating"
---

# Whiteboard topic history: no inner scroll; load older entries upward in place

## Background

Owner answers on MAIN's board, 2026-11-13: (509)=ok, (510)=b with the note
"B에 카드 전체가 시야에서 벗어나면 다시 접히는 방식이 어떨까 싶긴 해요."

Today a topic's record view scrolls inside `.wb-history`
(`agents-plugin-tool/internal/wswhiteboard/assets/page.css:417`:
`max-height: min(65vh, 720px); overflow-y: auto`), with paging in
`assets/stream.js`. The inner scroll nests a scroll area inside the page.

## Decisions

1. **No inner scroll; "load more" above the latest window** ((510)=b, owner
   2026-11-13): the topic card shows the latest window of entries, and a
   "load more" control above it expands older history upward in place.
   Rejected: (a) horizontal pages.
2. **Collapse when the card leaves the viewport** (owner note on (510),
   adopted by MAIN as a technical call): once the whole topic card is out of
   the viewport, its loaded history collapses back to the latest window.
   Rationale: loaded history does not accumulate, which also helps the (473)
   board-size and performance concern. Rejected: keeping expanded history
   until reload.
3. **Scroll compensation** (constraint from MAIN): an expand or collapse
   above the viewport keeps the visible content stable through scroll
   compensation, the same mechanism as the pending board-reorder-hold design
   ((516) on MAIN's board). If that ticket lands first, reuse its mechanism;
   if this one lands first, the reorder-hold ticket reuses this one's.
4. **Both surfaces**: the served board page and the dashboard board region
   (same topic markup).
5. **Compatibility tier: T0** (asset-only).

## Constraints

- Daemon assets under `agents-plugin-tool/internal/` follow
  `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: Load-more history with viewport collapse

- Remove the `.wb-history` inner scroll; the latest window renders in the
  card with a "load more" control above it (Decision 1).
- Collapse back to the latest window when the whole card leaves the viewport
  (Decision 2), with scroll compensation for changes above the viewport
  (Decision 3).
- Same behavior on the served board page and the dashboard board region.

Verification: asset tests for load-more expansion and the out-of-viewport
collapse; a headless check that expanding or collapsing a card above the
viewport does not move the visible content (scroll position compensated);
LOOK captures of a long topic before and after "load more" on both surfaces.
