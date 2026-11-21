# Board and chat small UX: Phase 1

Ticket: `ai-docs/tickets/todo/261121-feat-ws-board-chat-small-ux.md` (read
it fully; its Decisions are binding, including placement and the Rejected
alternatives).

## Scope

Implement the ticket's Phase 1 as written: the chat jump arrow (top-left),
the in-card Ctrl/Cmd+F find bar, and the browser-local hold with the held
pill category. Front-end assets only. Compatibility T0: if any change to an
MCP tool, store shape, tracker event, or manifest looks necessary, stop and
report instead.

Do not move the ticket status or write a `### Result` section. Do not launch
Electron. Headless Chromium via the cached Playwright
(`~/.npm/_npx/*/node_modules/playwright`) is fine for captures; save PNGs
under `ai-docs/assets/captures/261121/board-chat-small-ux/` and do not commit
them.

## Verify

- The ticket's Verification list: Node unit tests next to the existing pill
  and dashboard asset tests (`node --test <files>`), and narrow Go asset tests
  with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local` and a narrow `-run` (never
  the whole `./internal/wsdashboard` or `./internal/mcp` package).
- Headless captures: chat view with the arrow, a card with the find bar open,
  the pill showing a held count.

## Report

Commit on the current branch. Report: commit sha, test commands and results,
capture paths, and any deviation from the ticket.
