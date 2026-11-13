# Whiteboard: the check list hides answered checks by default

## Problem

Owner request 2026-11-13 (screenshot `D:/Temp/wb-checks/check-list.png`):
a topic's check list ("확인 요청", header shows "답변 3 / 4") keeps every
answered check expanded (checked box, title, `ok` chip), so the one still
waiting is pushed down. Answered checks should fold away.

## Outcome

- The check list's header row gains a toggle "hide answered requests"
  (Korean UI text e.g. "완료된 요청 숨기기"; i18n key in every
  `internal/wsi18n/tables/*.json` if the whiteboard page is localized
  through them, otherwise follow how the existing header text is localized).
  Default ON.
- ON: checks whose status is answered (ok/no/note, i.e. settled) are hidden;
  open, assumed (still vetoable), and sent-but-undelivered checks stay
  visible. The header count stays as is ("답변 3 / 4"). When every check is
  answered and hidden, the header still shows (so the toggle can reveal them).
- OFF: today's behaviour.
- The toggle state persists per browser (localStorage), one global setting
  (not per topic), and survives the live re-render/swap of the board.
- Works on the dashboard-served page (`/s/<replyid>`), which is the one the
  owner uses; the static file page/embed may keep showing everything if the
  toggle needs JS - say which.
- T0 (assets/render only).

## Out of scope

- The sticky topic head / check pill of ticket
  261112-feat-ws-whiteboard-topic-tickets-sticky-head (running separately;
  keep the change local to the check list so the two merge cleanly).
- Restarting the daemon (lead does the live check).

## Verification

- `go test` for the touched packages (`internal/wswhiteboard/...`,
  `internal/wsdashboard/...`, `internal/wsi18n/...`) on w5090 (clone note
  `remote-build-w5090`); node-backed JS tests run locally (node at
  C:/Program Files/nodejs, Go at C:/Program Files/Go/bin) under the local
  lock, confirming they ran rather than skipped.
- A test: default hides answered checks, toggle off shows them, open/assumed
  stay visible. A headless render of both states if cheap.
