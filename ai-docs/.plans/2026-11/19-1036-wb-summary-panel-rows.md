# Plan: panel rows stand out in the chat whiteboard-update summary card

## Request (owner, 2026-11-19 10:35, dashboard)

In the session conversation view, a whiteboard update is shown as a summary
card (topic title + stem, a "토픽" link, then rows: a panel row "패널 <recap>
#pNNN" and log rows "ⓘ <log line>", plus "보드에서 열기"). The panel row looks
almost the same as the log rows, so the owner keeps missing panel updates.
Improve the readability of panel-update rows so they are clearly distinct from
log rows. Do NOT embed the panel body (it may be HTML that does not inline
well); clicking the row already jumps to that panel on the board, so keep
that behaviour.

Owner screenshot: C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\3a389edeb220b01f-1.png

## Outcome

- A panel-update row reads as a distinct, scannable element: e.g. a panel icon
  and an accent left border or tinted background, the recap in brighter /
  slightly heavier text, and a small badge saying whether it is a new panel or
  an updated version of an existing one (only if that information is already
  available to the renderer without new data plumbing; otherwise skip the
  badge). The `#pNNN` link stays.
- Log rows stay muted and visually secondary.
- Several panel rows in one card stay readable and ordered as today.
- Works in the dark theme used by the dashboard; follow existing CSS variables
  and style of the surrounding chat cards.
- Clicking a panel row still jumps to that panel update on the board.

## Approach

- Find where the chat-side whiteboard update summary card is rendered
  (likely agents-plugin-tool/internal/wsdashboard/assets/, search for the
  "보드에서 열기"/"패널" i18n keys in agents-plugin-tool/internal/wsi18n/tables/
  and their js.* key users). Change styling/markup of panel rows only.
- Surgical, T0 (UI only, no API, event or store shape changes).

## Constraints

- NO local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  this PC (AhnLab V3). Go tests only on w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard/...'`
  (w5090 has no Node). Run only touched JS test files locally with node.
- Do NOT start, stop, rebuild or swap the shared ws daemon. For the LOOK
  capture, build a static fixture page under D:/Temp/wb-summary-panel-rows/
  that loads the real dashboard CSS/JS assets from this worktree and renders a
  summary card with 2 panel rows (one new, one updated) and 2 log rows; capture
  before and after with a headless browser (not Electron), and compare them
  side by side with the owner's screenshot.
- Temp files only under D:/Temp/wb-summary-panel-rows/; delete when done
  except the final before/after PNGs (report their paths).
- Never read or print credentials (?login= token, cookies, endpoint.json, lan
  tokens).
- New user-visible strings go into every locale table under
  agents-plugin-tool/internal/wsi18n/tables/.
- Commit on branch impl/develop/wb-summary-panel-rows in this worktree only,
  with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Verification

- Existing dashboard Go tests pass on w5090.
- Before/after LOOK captures, side by side.

## Return boundary

Stop and report before changing anything beyond the summary card's panel and
log row presentation (for example the board page itself or the update API).
