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
   Rationale: loaded history does not accumulate on screen, which also
   helps the (473) board-size concern (rendering only; see Decision 6). Rejected: keeping expanded history
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

6. **Collapse resets the clip; loaded rows stay** (UX review and feasibility
   note 2026-11-13, technical): with the clip of Decision 8, rows load from
   the server once and stay in the card; a collapse only sets the step count
   back to 1, and the keep-in-place helper anchors the oldest visible entry.
   Open and closed folds keep their state. Consequence for Decision 2's
   rationale: the collapse bounds what is shown, not what is loaded, so the
   (473) benefit is limited to rendering; hidden rows stay in the DOM.
   Rejected: rebuilding the latest window from kept template copies
   (`loadOne` deletes each page template, `stream.js:109-115`, and
   `joinBoundary` merges rows, so a rebuild needs a parallel copy of the
   history; the clip needs none).
7. **Swap and editing interplay** (UX review 2026-11-13, technical): the
   inner-scroll code (`stream.js` ~28-61, 199, 210-217) and the second
   `wbHistory.restore` pass (`dashboard.js:551-552`) are removed or
   repurposed; no collapse runs while board swap motion runs or while focus
   or answer input sits inside the history (`editing()`); the observer
   watches the nodes `morphBoard` inserts and unwatches the ones it removes.
   `.wb-history` loses its `tabindex` (`page.tmpl:47`), which existed for
   the inner scroll.

8. **Height-based window** ((528) withdrawn by the owner and replaced by
   (531), announced by MAIN 2026-11-13; owner: "rather base it on height,
   for example past 640px vertical, in 640px units; if that is not possible,
   load 3 more entries at a time"). Mechanism (UX feasibility note,
   technical): the history box is clipped by CSS `max-height` = 640px x
   steps with `overflow: hidden`; its content is anchored to the bottom
   (latest entries visible, older ones above), so the clip hides the oldest
   part at the top edge, where a fade and the "load more" control sit.
   "Load more" raises steps by 1; when the loaded content is shorter than
   the new clip (`scrollHeight` < steps x 640), it fetches the next server
   page (the server's `HistoryWindow`, 50 entries, `stream_view.go:11`) and
   repeats until the content is taller or nothing older is left. Late iframe
   heights only change `scrollHeight`, so nothing is measured at click
   time. The 3-entry step is the owner's fallback, used only if the clip
   approach proves unworkable (the worker records why in its result).
   Rejected: a fixed 10-15 entry window; keeping all 50 entries visible;
   measuring cumulative entry heights at click time (late iframe heights
   make it unreliable).

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
- The 640px clip and load-more stepping (Decision 8); collapse resets the
  clip (Decision 6); swap and editing interplay (Decision 7).
- Collapse back to the latest window when the whole card leaves the viewport
  (Decision 2), with scroll compensation for changes above the viewport
  (Decision 3).
- Same behavior on the served board page and the dashboard board region.

Verification: asset tests for the 640px first window and each ~640px step,
the next server page fetched only while the content is shorter than the
clip, load-more expansion and the out-of-viewport
collapse (fold state in the latest window survives it); a board swap keeps an
expanded topic expanded; a topic in the pinned column; a deep link into an
older page; the inner-scroll stubs in `stream_script_test.go` rewritten; a headless check that expanding or collapsing a card above the
viewport does not move the visible content (scroll position compensated);
LOOK captures of a long topic before and after "load more" on both surfaces.
