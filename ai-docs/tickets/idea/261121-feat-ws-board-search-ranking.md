---
title: "보드 검색 개선: 숫자 ID 우선 매칭, 선택지·카드 제목·본문 필드 분리, 파일 검색"
related:
  261114-bug-ws-dashboard-search-palette-minors: sibling leftovers of the same search palette (bug fixes, kept separate)
  261114-feat-ws-dashboard-unified-search-palette: the landed palette this extends
---

# Board search: numeric-ID matching, field-split results, file search

## Background

From the owner's idea memo (2026-11-21 triage, board topic
`idea-board-ux`). Ideas for searching the whiteboard:

- A numeric-only query (e.g. `14`) tries item IDs first, smallest matching ID
  first, with include and fuzzy matches after the exact one.
- Results are split by field: options, card (panel) titles, bodies, and so on,
  rather than one undifferentiated hit list.
- File search is supported from the same entry point.
- "etc." in the memo: more refinements are expected.

## Decisions

- Recorded as its own idea instead of being folded into
  `261114-bug-ws-dashboard-search-palette-minors`: that ticket holds four
  bug-sized leftovers, and mixing feature work into it would blur its scope
  (lead's placement call, 2026-11-21).

## Constraints

- Before promotion, check what the unified search palette already indexes
  for the board and which of the above it already does; the palette landed
  in `261114-feat-ws-dashboard-unified-search-palette`.
- UI work: the owner chooses the worker tier before implementation.

## Phases

### Phase 1: Settle scope

Survey the landed palette's board and file search, settle which ideas
remain, then plan them as a `todo/` ticket.
