# Plan: tracker mockup - per-card view modes and a chat card placeholder

Branch `delegate/tracker-mockup-percard` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch (several commits are fine); do not merge or push.

Scope: `ai-docs/ref/tracker-mockup/` only (app.js, style.css, index.html,
README.md, and SCENARIOS.md where its steps cite the drawer-level mode
picker). It is the unstyled reference mockup of epic
`261107-epic-ws-personal-common-store`. `engine.js`, `fixtures.js`, the golden
generator, and `agents-plugin-tool/internal/wstracker` stay untouched: this is
a view-only change, and `deriveViewState` remains the only read path
(board (275)). If a mode needs data the view state lacks, stop and report it
rather than editing engine.js.

## Decisions (user, board, 2026-11-08)

1. (303) ok - view modes move from the drawer title to each card. The
   drawer has one layout, the cards layout; the drawer-level mode picker is
   removed. Each card has its own mode picker offering only the modes that fit
   its content:
   - Items: tree / kanban (item cards) / due timeline / calendar.
   - My assignments, Assigned by me, My team: list / kanban (assignment
     cards, with the pending column) / due timeline / calendar.
   - Activity: list / activity timeline.
   - Today, Received requests, Team, Data: list only (no picker).
   - Chat: conversation only (see 3).
   The existing kanban, activity timeline, due timeline, and calendar
   renderers are reused, scoped to the card's own rows and filters. The
   kanban drag rule is unchanged (only the viewer's own live assignment moves;
   done dispatches `item.close`).
2. (304) a - a card switched to a wide mode (kanban, due timeline, calendar,
   activity timeline) spans the full row of the cards layout; the remaining
   cards flow below it. No horizontal-scroll-in-card fallback and no maximize
   button now.
3. (300) a + (302) ok + (305) ok - a **chat card** placeholder for the
   future daemon-side local support agent: placement and a static sample
   only, no working chat, no fake responder, no new events.
   - It is one card in the cards layout, fixed height with internal scroll,
     newest message at the bottom, and a disabled composer row.
   - The sample conversation mimics the dashboard conversation view's entry
     shapes (user text, assistant text, a collapsed tool line) plus one new
     entry kind: a **tracker card reference** that renders a mini card of an
     item (title, status, priority, due, assignee) from the view state, and
     opens that item's thread sheet on click. Use real item ids from the
     current viewer's view state so the reference resolves.
   - Label it as a placeholder for the support-agent chat child.
4. (305) ok - each card's chosen mode is local UI state remembered per store
   (in-memory or localStorage), never an event.
5. No styling work beyond what the layout needs to be usable (masonry styling
   stays deferred).

## README

Replace the "View modes (drawer title)" description with the per-card model
(the table above, the wide-mode full-row rule, local mode memory), add the chat
card placeholder with the card-reference entry kind, and add Settled rows for
(300), (302) to (305), noting (303) supersedes the drawer-level picker of
(276) b while keeping its four modes.

## Verification

`node --check` on every script; the page loads without console errors in
headless Chromium (cached Playwright under
`~/.npm/_npx/*/node_modules/playwright`) for at least three viewers (root,
lead, member); DOM or screenshot checks show: each card's picker offers
exactly its modes, a wide mode spans the full row, a kanban drag on My
assignments still dispatches, the chat card's reference opens the right
thread sheet, and the mode choice survives a viewer round trip within the
same store.

## Return

Changed files; per decision what changed; screenshots or DOM evidence paths;
anything you could not settle.
