---
title: "보드·채팅 작은 UX 3건: 내 최근 메시지 점프, 카드 안 Ctrl+F, 질문 보류 표시"
related:
  261114-bug-ws-dashboard-search-palette-minors: sibling board-search work from the same idea triage (kept separate)
completed: 2026-11-21
---

# Board and chat small UX: jump to my latest message, in-card find, local hold on open asks

## Background

Three small dashboard/whiteboard UX ideas from the owner's idea memo
(2026-11-21 triage, board topic `idea-board-ux`). All three are
front-end only: no server, MCP, or store-shape change.

1. Chat: a quick way back to the user's own most recent message that is
   scrolled out of view.
2. Board cards: find text inside one card (panel) with Ctrl/Cmd+F.
3. Board: let the user mark an open ask as "on hold" for themselves, so a
   deliberately parked ask stops reading as an unanswered one, without
   telling the agent anything.

## Decisions

- Jump control sits at the chat view's **top-left**; the top-right overlaps
  the agent list (owner, board 2026-11-21 17:57).
  Rejected: top-right placement.
- Jump is a single arrow that goes to the most recent of the user's own
  messages **not currently visible**; it usually points up, points down when
  that message is below the viewport, and hides when the latest own message
  is already visible (owner, 17:57; design v2 approved by (1482)=a).
- In-card find: Ctrl/Cmd+F is intercepted only while focus or the pointer is
  inside a board card; it opens a small find bar in that card, highlights
  matches, Enter / Shift+Enter step next/previous, Esc closes. Outside a card
  the browser or Electron default is untouched (design v2, (1482)=a).
- Hold is **browser-local state only**: never sent to the agent, no new item
  status, no whiteboard store change (owner, 17:57). Rejected: a server-side
  `held` status or flag (owner: the purpose is the user's own awareness).
- Hold surfaces in the existing open-question pill
  (`agents-plugin-tool/internal/wswhiteboard/assets/pill.js`, categories
  open / answered / noted) as an added **held** category the user can jump to
  in one step; held items leave the open count and the tab-title open total
  (owner, 17:57).
- Hold auto-clears when the item's content changes (the agent patches or
  re-asks it), and is dropped when the item is answered or withdrawn
  ((1481) recorded as the reading of the owner's 17:57 note, confirmed with
  (1482)=a). Rejected: clearing on any topic update (a progress log line
  would drop holds almost immediately).

- Holds persist in `localStorage` keyed by board identity + item ID, with a
  fingerprint of the item's content at hold time; the fingerprint is how an
  agent edit is detected without server help ((1483), owner ok 18:12).
  Rejected: `sessionStorage` (holds must survive a reload).
- In-card find is a custom find bar scoped to one card ((1484), owner ok
  18:12). Rejected: the native browser find (cannot be scoped to one card and
  has no page API).

## Constraints

- Compatibility: T0 - no MCP, `/mcp` hello, store shape, tracker event, or
  manifest change. If implementation finds it needs any of those, stop and
  report.
- Read before editing: `ai-docs/manuals/shipped-surface-boundary.md`
  (dashboard strings ship), `ai-docs/manuals/performance.md` (no new file
  I/O on interactive paths; local state stays in the browser).
- New user-visible strings go through `wsi18n` tables in every shipped
  language, following the existing `js.pill.*` keys.
- UI work: the owner chooses the worker tier when the run starts.
- Out of scope: board search improvements (sibling ticket), whiteboard-only
  chat mode, custom command slots (separate ideas).

## Phases

### Phase 1: jump arrow, in-card find, local hold

- Chat jump arrow in the chat view (locate the conversation scroller and the
  user-message rows; dashboard-conversation / dashboard-stream assets are the
  likely home).
- In-card find bar on whiteboard cards (`wswhiteboard/assets/`).
- Hold toggle on open ask rows plus the **held** pill category in `pill.js`,
  localStorage persistence with content fingerprint, auto-clear on change,
  drop on answered/withdrawn.

Verification:
- Node/JS unit tests next to the existing pill and dashboard asset tests for:
  held excluded from the open count; hold cleared on fingerprint change;
  hold dropped on answered/withdrawn; jump target selection (latest
  off-screen own message, direction, hidden when visible).
- Narrow Go asset tests if assets are embedded and checked
  (`GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`, narrow `-run`; never the
  whole `./internal/mcp` or `./internal/wsdashboard` package).
- Headless Chromium capture of the chat view with the arrow, a card with the
  find bar open, and the pill showing a held count, for the owner to compare
  against this ticket.

### Result (8666743ce) - 2026-11-21

- Chat jump arrow at the top-left of the conversation view: targets the latest
  off-screen own message, points up or down, hides when that message is
  visible, scrolls it to center on click.
- In-card Ctrl/Cmd+F find bar (`card-find.js`): intercepts only while focus
  or the pointer is inside a non-terminal board card, including sandboxed
  doc-frame bodies via authenticated `postMessage`; Enter / Shift+Enter step,
  Esc closes and restores text nodes.
- Browser-local ask hold: `localStorage` keyed by board identity + item ID
  with a content fingerprint; auto-clears on edit or re-ask, dropped on
  answered/sent/withdrawn; held asks leave the open count and tab title and
  get a **held** pill category. 13 new strings in all 11 locales. T0.
- Worker: agy.large. Verified: Node tests (pill hold, user jump, scroll,
  interaction) 16/16; `go test ./internal/wsi18n`; narrow `./internal/wswhiteboard`
  and `./internal/wsdashboard` runs including a light + dark Chromium
  acceptance test; light/dark before/after captures reviewed by the lead.
