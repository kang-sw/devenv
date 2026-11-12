---
title: "Dashboard search palette: one centered box for sessions, topics, items, answers, and the tracker, with .<id> jumps"
---

# Dashboard search palette: one centered box for sessions, topics, items, answers, and the tracker, with .<id> jumps

## Background

User request (2026-11-12): the top-left magnifier on a session page is today
the tracker's quick switcher (`assets/tracker.js` `qResults` around line 5461,
button `#tkqs` mounted into `#qsslot` from `dashboard.tmpl`); it matches
tracker store names and item titles by plain substring. The user wants that
button, and a keyboard shortcut, to open a centered command-palette-style
search box over the session page:

- Agents often mention whiteboard items (`(153)`) in ordinary chat output, and
  jumping to one is tedious; typing `.153` should jump straight to that item.
- The user also wants to search the board and bounce between hits.

The mockup the user approved ("검색창 목업은 아주 좋네요", 2026-11-12) is the
reference for look and behaviour; its decisions are restated below so the
ticket does not depend on the scratch file.

## Decisions

- **One palette replaces the tracker quick switcher** (whiteboard (473)): the
  same top-left magnifier opens it; tracker results are its last group, and
  choosing a tracker row does what the quick switcher does today (open the
  store page, open the item window, or toast when the item is hidden).
  Rejected: keeping the quick switcher as a second search box (two buttons and
  two shortcuts for one job).
- **Result groups in a fixed order** (user, 2026-11-12): session labels, topic
  names, topic bodies, items (question titles), answers, tracker. Within a
  group, rows sort by fuzzy score.
- **Fuzzy matching**: a query matches when its characters appear in order
  (subsequence); contiguous runs and an early first match score higher. Body
  and answer matches whose matched characters are too scattered are dropped,
  since long text matches almost any short subsequence.
- **`.<number>` jumps to an item**: `.153` shows only item (153); Enter scrolls
  to it and flashes it. A prefix such as `.15` lists every item whose ID
  starts with it. An item in a closed topic opens that topic first. An empty
  query lists recent items and topics.
- **Scope** (whiteboard (474) a): the current session's board (topic names,
  bodies, items, answers) plus the session rail's labels; choosing a session
  label navigates to that session. Rejected: searching every session's board
  content (needs a daemon-side search index; can be added later by widening
  the scope).
- **Shortcuts** (whiteboard (475) b plus the user's note): Ctrl+Space and
  Ctrl+K both open it, and `/` opens it when focus is not in a text input,
  textarea, or contenteditable. Ctrl+Space alone was rejected because macOS
  and some IME setups consume it before the browser sees it.
- **Not in 0.49** (whiteboard (472)): implemented after the 0.49 release.

## Constraints

- Asset-only unless the page lacks data the scope needs: the board, its items
  and answers, and the rail are already rendered into the session page.
  Compatibility tier T0 (dashboard asset content).
- Keyboard: arrow keys move the selection, Enter opens, Esc closes and returns
  focus to the element that opened it; IME composition (`isComposing`) never
  triggers a shortcut or a selection.
- Layout works at phone width; the palette respects `prefers-reduced-motion`
  for the flash and any open animation.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`.

## Phases

### Phase 1: Search palette replaces the quick switcher

Build the palette in the dashboard assets per `## Decisions`, move the
tracker quick switcher's result kinds and actions into its last group, and
remove the old quick-switcher popover. Update the tests that assert the
quick switcher's markup, and add tests for the fuzzy scorer, the group order,
the `.<id>` jump (exact and prefix), and the `/` focus guard.

Verification: the wsdashboard package tests through
`~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test ./internal/wsdashboard/ -count=1`
(never run heavy Go builds or tests locally), plus a headless Chromium
screenshot of the open palette with results in each group.
