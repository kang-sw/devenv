# Plan: session rail context menu closes reliably

## Request (owner, 2026-11-19 10:27, dashboard)

The session right-click context menu in the dashboard session rail (items:
rename (dashboard label), copy mailbox address, add to favourites, move under
another session, "sessions the dashboard runs" settings: model / permission
mode / effort selects, then stop, archive, hand over to terminal, close) is hard
to dismiss. Clicking inside the session dashboard main area (an iframe) does not
close it; only clicking the outer shell does. Fix it if possible. Also, besides
the dropdowns (selects), the other buttons are one-shot actions: clicking one
should also close the context menu.

## Outcome

1. Any pointer-down outside the menu closes it, including inside the embedded
   session dashboard iframe(s) and anywhere in the shell (Electron shell and
   plain browser page both). Escape and window blur caused by focusing an
   iframe also count. Interacting with the menu's own selects (model,
   permission mode, effort) keeps it open; changing a select does not close it.
2. One-shot action buttons (rename, copy mailbox address, favourite toggle,
   stop, archive, hand over to terminal, close, and any similar action rows)
   close the menu when clicked; a submenu opener ("move under another
   session >") still opens its submenu, and picking a target in it closes the
   whole menu. An action that opens a confirm/prompt dialog closes the menu
   before or as the dialog opens.

## Approach (worker decides details)

- Locate the rail context menu implementation (likely under
  agents-plugin-tool/internal/wsdashboard/assets/, and possibly the Electron
  shell under desktop-electron if the shell hosts the rail). Find why iframe
  clicks do not reach the dismiss handler (document-level listeners do not see
  events inside a cross-document iframe). Typical robust fixes: also close on
  the parent window `blur` (fired when focus moves into an iframe) and/or
  attach a pointerdown listener in same-origin iframes / postMessage from the
  child page. Prefer the simplest mechanism that covers both the browser and
  Electron shell cases; keep interactive paths free of file I/O.
- Keep the change surgical; follow existing style; T0 (UI only, no event, store
  or API shape changes).

## Constraints

- NO local `go test`, Go tools, gofmt, ws/ws-mcp exes, launcher or Electron on
  this PC (AhnLab V3). Go tests only on w5090:
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync '<this worktree path>' -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard/...'`
  (w5090 has no Node). Do NOT run the full desktop-electron npm test locally;
  run only touched JS test files with node if any exist.
- Do NOT start, stop, rebuild or swap the shared ws daemon.
- Temp files only under D:/Temp/rail-ctxmenu-close/; delete when done.
- Never read or print credentials (?login= token, cookies, endpoint.json, lan
  tokens).
- i18n: any new user-visible string goes into every locale table under
  agents-plugin-tool/internal/wsi18n/tables/.
- Commit on branch impl/develop/rail-ctxmenu-close in this worktree only, with
  `Co-Authored-By: Claude Sonnet 5.5 <noreply@anthropic.com>`.

## Verification

- Existing Go tests touching the dashboard assets pass on w5090.
- A pure-JS check (node, local) for any extracted dismiss logic if practical.
- Report the root cause, the files changed, and what could not be verified
  live (the owner verifies in the running dashboard after the next daemon
  update; the daemon is NOT swapped today).

## Return boundary

Stop and report before any change outside the rail context menu / its iframe
dismiss path, or if the fix needs a protocol or Go route change.
