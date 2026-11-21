# Tracker unhide: dashboard UI part of Phase 1

Ticket: `ai-docs/tickets/todo/261121-feat-ws-tracker-unhide.md` (read it
fully; its Decisions are binding, especially the "show hidden" entry point).
The backend is already committed on this branch (`eacd19d33`); do not change
Go code under `internal/wstracker/`.

## Backend contract (from the backend worker)

- Unhide: `POST /tracker/stores/<id>/dispatch` with
  `{"events":[{"type":"item.unhide","payload":{"item":"<id>"}}]}` or
  `{"events":[{"type":"comment.unhide","payload":{"comment":"<id>"}}]}`.
- Summary/delta rows: `data.item.hidden` (bool) and `data.item.canUnhide`
  (bool); hidden items are already included in `itemOrder` and `children`.
- Detail bundle: `data.comments[id].hidden` (hide-event ID or `null`) and
  `canUnhide` (true or omitted). Legacy `/view` exposes the same fields.
  Missing `canUnhide` means false.
- Refusal: HTTP 409 with `refusal.reason`; the hidden-parent reason is
  `unhide the parent first`.

## Scope

- A "show hidden" toggle on the tracker list (off by default). Off: hidden
  items and comments stay out of view exactly as today. On: they render
  dimmed in place, and an "unhide" action appears where `canUnhide` is true.
- The "unhide" action on hidden items (row menu, matching where hide lives
  today: `tracker-menu.js`, `tracker-item-rows.js` `canHide`) and on hidden
  comments in the item detail.
- Opening a hidden item while the toggle is off keeps today's "not visible"
  toast.
- Surface a 409 refusal reason through the existing refusal display path.
- New strings through `wsi18n` in every shipped language.
- Whether the toggle state persists is your call: browser-local only (no
  server or store change); state your choice in the commit's AI Context.

Do not move the ticket status or write a `### Result` section. Do not launch
Electron. Headless Chromium via the cached Playwright
(`~/.npm/_npx/*/node_modules/playwright`) is fine for captures; save PNGs
under `ai-docs/assets/captures/261121/tracker-unhide/` and do not commit
them.

## Verify

- Node tracker tests (`node --test` on the tracker test files you touch or
  that cover the touched modules).
- Narrow Go tests for embedded assets / i18n tables if you touch them,
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` with a narrow `-run` (never the
  whole `./internal/wsdashboard` or `./internal/mcp` package).
- Headless captures: list with the toggle off, on (dimmed hidden item with
  the unhide action), and a hidden comment with its unhide action.

## Report

Commit on the current branch. Report: commit sha, test commands and results,
capture paths, and any deviation from the ticket.
