---
title: "Whiteboard topic history: no inner scroll; load older entries upward in place"
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "(473) board size and performance; the collapse keeps loaded history from accumulating"
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite: its Phase 1 builds the shared visibility observer and keep-in-place helper this ticket reuses (its decision 8)"
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
3. **Scroll compensation** (constraint from MAIN; helper owner set by the
   EXEC lead, technical): an expand or collapse keeps the visible content
   stable through the keep-in-place helper and the visibility observer that
   `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 builds in
   `wswhiteboard/assets/stream.js` beside `wbHistory` (its decision 8,
   (516) on MAIN's board); this ticket reuses them and adds no second copy.
   "Upward in place" means the entry that was oldest before the click keeps
   its screen y while the added height goes above it; older pages carry
   doc-frame iframes that report their height late, so the helper re-anchors
   on resize until layout settles. A collapse of a card that scrolled out
   above the viewport is compensated the same way. The helper targets the
   card's nearest scroller (`#bcol`, `#bpins`, or the document). Rejected:
   whichever ticket lands first builds the helpers (parallel workers could
   build two copies).
4. **Both surfaces**: the served board page and the dashboard board region
   (same topic markup).
5. **Compatibility tier: T0** (asset-only).

6. **Collapse rebuilds from kept copies** (UX review 2026-11-13, technical):
   `loadOne` deletes each page template (`stream.js:109-115`) and
   `joinBoundary` moves older rows into the tail page's folds, so undoing the
   loads cannot restore the latest window. When a card first loads, it keeps
   a copy of the server-rendered tail page and the older-page templates, and
   a collapse swaps the kept tail back in. Open and closed folds inside the
   latest window keep their state across a collapse. Rejected: reversing the
   loads one by one (no pristine state survives `joinBoundary`).
7. **Swap and editing interplay** (UX review 2026-11-13, technical): the
   inner-scroll code (`stream.js` ~28-61, 199, 210-217) and the second
   `wbHistory.restore` pass (`dashboard.js:551-552`) are removed or
   repurposed; no collapse runs while board swap motion runs or while focus
   or answer input sits inside the history (`editing()`); the observer
   watches the nodes `morphBoard` inserts and unwatches the ones it removes.
   `.wb-history` loses its `tabindex` (`page.tmpl:47`), which existed for
   the inner scroll.

## Open Decision Queue

Last ID: (528)

(528) [open] [policy] Size of the latest window a topic card shows before "load more" (today `HistoryWindow` = 50 entries, `stream_view.go:11`, and with the inner scroll gone every card would show all 50) - on MAIN's board, awaiting the owner; options 10-15 vs 50.

## Constraints

- Daemon assets under `agents-plugin-tool/internal/` follow
  `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Phases

### Phase 1: Load-more history with viewport collapse

- Remove the `.wb-history` inner scroll and its `tabindex`; the latest
  window renders in the card with a "load more" control above it
  (Decision 1).
- Collapse from kept copies (Decision 6); swap and editing interplay
  (Decision 7).
- Collapse back to the latest window when the whole card leaves the viewport
  (Decision 2), with scroll compensation for changes above the viewport
  (Decision 3).
- Same behavior on the served board page and the dashboard board region.

Verification: asset tests for load-more expansion and the out-of-viewport
collapse (fold state in the latest window survives it); a board swap keeps an
expanded topic expanded; a topic in the pinned column; a deep link into an
older page; the inner-scroll stubs in `stream_script_test.go` rewritten; a headless check that expanding or collapsing a card above the
viewport does not move the visible content (scroll position compensated);
LOOK captures of a long topic before and after "load more" on both surfaces.
