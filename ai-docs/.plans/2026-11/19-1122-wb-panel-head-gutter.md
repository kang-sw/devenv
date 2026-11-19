# Plan: whiteboard panel card header — time in the log gutter column, envelope icon instead of the bullet

## Request (owner, 2026-11-19 11:21, board page)

On the whiteboard board page, a topic's stream shows log rows and panel
cards. Log rows show the owner badge (e.g. "L") and the relative time
("44분 전") in a left gutter column, then the text. A panel card header shows
a small bullet dot at its far left, the title, and at the right
`state 44분 전 #p257` plus link/expand icons.

Owner screenshot: C:\Users\user\.cache\ws@kang-sw-devenv\dashboard\sessions\renewaldevoteetranspirefencing\inbox\attachments\ec8fb84e41526330-1.png
(red marks: the bullet at the header's left, and the "44분 전" time at the right).

Asked:
1. Move only the panel's time display so it sits in the same column as the
   time of log rows / the owner gutter (aligned vertically with the log rows'
   time), instead of at the right side of the header. Keep the panel name
   (`state`), `#pNNN`, and the link/expand icons where they are.
2. Replace the bullet with a closed-envelope / open-envelope icon pair.
   First find what the bullet currently means (e.g. unread/new vs seen, or
   folded vs expanded body) and map: closed envelope = the "closed" state of
   that meaning (unread, or folded), open envelope = the other. Report which
   meaning it is. Use the existing icon sprite system (`{{icon "..."}}` /
   sprite symbols) — add envelope symbols to the sprite if absent, matching
   the existing icon style (stroke width, size, currentColor).

## Constraints

- UI-only, surgical, T0 (no API, event or store shape change).
- Find the board page assets under agents-plugin-tool/internal/wsdashboard/
  assets/ (or the whiteboard page assets/templates) — follow existing CSS
  variables and dark theme.
- Any new user-visible string (e.g. icon titles/aria-labels) goes into every
  locale table under agents-plugin-tool/internal/wsi18n/tables/.
- NO local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  this PC. Go tests only on w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wswhiteboard/... ./internal/wsi18n/...'`
  (no Node on w5090; redirect stdout to a file under D:/Temp/wb-panel-head-gutter/
  and read it — piped output can be swallowed). Run touched JS test files
  locally with node only.
- Do NOT start, stop, rebuild or swap the shared ws daemon. For LOOK, build a
  static fixture page under D:/Temp/wb-panel-head-gutter/ that loads the real
  board CSS/JS/sprite assets from this worktree and shows a topic stream with
  log rows and two panel cards (one in each bullet state), captured with a
  headless browser (not Electron) before and after. Compare side by side with
  the owner's screenshot. Keep only before.png/after.png; delete the rest.
- Never read or print credentials (?login= token, cookies, endpoint.json, lan
  tokens).
- Commit on branch impl/develop/wb-panel-head-gutter in this worktree only,
  with `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Verification

- Existing Go tests above pass on w5090.
- Before/after LOOK captures: the panel time is in the same x column as log
  row times; envelope icons render in both states.

## Return boundary

Stop and report before changing anything beyond the panel card header layout
and the bullet icon (for example log rows, the chat summary card, or panel
body rendering).
