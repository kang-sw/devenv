# Plan: tracker mockup v1 - a layout mockup inside a dashboard-like frame

Branch `delegate/tracker-mockup-layout` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/decree-thinness-strum`). Commit on this
branch; do not merge or push.

## Why

v0 (`ai-docs/ref/tracker-mockup/`, merged ce33727a8) is a raw data page:
JSON panes, tables, browser-default styling. The user (2026-11-07) clarified:
"제가 말한 목업은 이 정도로 raw한 것 까진 아니었는데 .. 요컨대 레이아웃으로
보고 싶다는 말이긴 했어요". "Unstyled" meant no visual polish, not no layout:
the user wants to see how the tracker would actually be laid out and used,
and settle the data structures by looking at that.

## Outcome

Rework the same folder into a layout mockup, still a standalone page opened
from `file://` (index.html + plain JS + fixtures.js; no build, no fetch).

1. Frame: imitate the dashboard session page's top bar and pull-down drawer
   (read `agents-plugin-tool/internal/wsdashboard/assets/dashboard.tmpl`,
   `dashboard.css`, and the drawer part of `dashboard.js` for the real
   structure: header items left after the title, drawer handles top-right,
   drawer panels as cards). Per the epic, each tracker store is its own drawer
   handle stacking leftward from the top-right (one for `team-alpha`, one for
   the personal store, next to the existing panels handle, which can be a
   static stub). A handle shows a badge for new assignments. The area under
   the header can be a static placeholder of the conversation.
2. Store drawer content as cards, laid out for real use: at least "My
   assignments" (what I must do, with state, assigner, due actions),
   "Assigned by me / team progress" (what I handed down, with roll-up),
   "Team" (org tree with leads/followers and admins), and an item list.
   Item detail opens as a side sheet or dialog: title, body, kind, project
   reference, assignees with per-assignee state and completion mode, subtask
   tree with roll-up, event history timeline (who, when, human vs agent,
   flagged events with reason).
3. Actions as small forms/dialogs, not raw JSON: create item, assign (all
   within N levels, or chosen people within N), split into subtasks, report
   progress, close, confirm. Each appends an event and re-renders.
4. Keep the v0 replay engine (merge per-member logs, validate, flag) as the
   data layer; split it into its own file if that makes the view clean. Keep
   the "viewing as" switch (as a header control) and the raw data editor,
   but move the raw JSON behind a collapsed "data" toggle or a separate
   debug card, since the data model is still what is being settled.
5. Styling: neutral and minimal but laid out: grid, spacing, borders, card
   look, readable type, light theme is enough. Borrow the dashboard's CSS
   variables/feel where cheap. No visual polish beyond what makes the layout
   legible. UI labels in English like the dashboard.
6. README.md: update "How to open" and the view description; keep the data
   model and the Open questions list as they are (they are on the user's
   board as (144)-(163)); add nothing to them.

## Verification

`node --check` on every JS file. Headless Chromium from `file://` (cached
Playwright under `~/.npm/_npx/*/node_modules/playwright`): no console errors;
handles open their drawers; each action works through its form; item detail
opens; viewer switch changes "My assignments". Take screenshots at 1400px and
800px wide into /tmp/tracker-shots and report their paths.

## Return

Changed files, layout decisions you made, screenshot paths, verification
output, what was not checked.
