# Dashboard: Ctrl+Q toggles the board "한 눈에 보기" (Overview)

Owner request (2026-11-16 22:53 KST, verbatim): "엄청 자잘한 거, ctrl+q 를 "한 눈에 보기"로 만들어주세요."

The board toolbar already has an Overview toggle. Its label is `js.board.toolbar.overview`, "한 눈에 보기" in `internal/wsi18n/tables/ko.json:442`. It is backed by `overview.js`, which keeps the state in the `ws.dashboard.overview.<rid>` key, and by `window.WsCards.overview` in `cards.js:1575`. Pressing Ctrl+Q should do exactly what clicking that toolbar control does: toggle Overview on and off for the current session's board.

## Behavior

1. Ctrl+Q toggles Overview. Use the same code path as the toolbar button, so the button's pressed state, the persisted per-session state, and the layout all stay in sync. Do not add a second state.
2. macOS:
   - In a browser, Cmd+Q quits the browser, and the page cannot intercept it. Bind the physical **Ctrl**+Q on every OS, including macOS. Do not bind Cmd+Q.
   - In Electron, check `desktop-electron`'s application menu for a Quit accelerator bound to Ctrl+Q, on Windows or Linux. If one exists, the shell would quit before the page sees the key. Report it, and do not change the shell.
3. Do not fire in these cases:
   - inside editable fields (input, textarea, contenteditable, the composer, rename fields);
   - inside the terminal;
   - during IME composition (`isComposing` / keyCode 229);
   - with extra modifiers (Shift, Alt, Meta).

   Let those keypresses pass through untouched.
4. Where the key is caught:
   - The board lives in the session frame `/s/<rid>?frame=1` inside the dashboard shell. Ctrl+Q must work wherever focus is: in the session frame (board or chat area), or in the outer shell (rail) while a session is shown. The shell forwards the toggle to the visible frame's board.
   - Reuse any existing shell-to-frame key forwarding if there is one, for example the mechanism other dashboard shortcuts use. Otherwise use a minimal postMessage handled by the frame's board code.
   - When no board is visible, do nothing and do not `preventDefault`.
5. Discoverability: add "(Ctrl+Q)" to the toolbar control's tooltip/title, in both ko and en tables. Do not change the visible label.

## Verification

- Chromium test with the cached Playwright:
  - Set `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`, and set `WS_PLAYWRIGHT_MODULE` to the cached npx playwright module.
  - Extend an existing board browser test, for example `internal/wsdashboard/testdata/board-state-filter-browser.cjs`, or add one next to it.
  - Check that Ctrl+Q turns Overview on: the toolbar control shows pressed and `.board-overview` is active.
  - Check that a second Ctrl+Q turns it off.
  - Check that Ctrl+Q typed in the composer textarea does not toggle it.
  - Check that Meta+Q does nothing.
- Narrow Go tests, if templates or i18n tables change. For example: `go test ./internal/wsi18n -count=1` and `go test ./internal/wsdashboard -run 'TestDashboardFrame|TestI18n|TestBoard' -count=1`.
- Screenshot (light, 1280px) of the board after Ctrl+Q with Overview on. Save it outside the repo and list the path.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. The module is `go 1.23`.
- Before editing, read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit.
- Do not ssh, do not launch Electron or any GUI window, do not restart the shared daemon, and do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in code, tests, or commits.
- Every behavior change lands with a test. Commit on your branch only, with `## AI Context`. Do not merge.
- Return: commits, where the key is caught and how it reaches the board, tests run with results, the screenshot path, and the Electron menu finding.
