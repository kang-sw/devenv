# Dashboard and whiteboard: scope Ctrl/Cmd+A "select all" to the content the user is in

Owner request (2026-11-16 22:05 KST, verbatim): "파일 다이얼로그 안에서라던지, 아니면 패널 바깥쪽이라던지, ctrl(cmd) + a 누르면 전체 선택되어버리는 경우가 꽤 잦은데, 이런 단축키의 "전체 선택" 같은 액션을 일단 바깥쪽 패널은 선택이 안 되게 하고, 각 카드 안쪽 텍스트, 말풍선 안쪽, 이런 걸로 한정하면 좋겠습니다."

Today, pressing Ctrl+A (Cmd+A on macOS) outside a text field selects the whole page: rail, headers, buttons and every card. That also happens inside the file preview dialog. The owner wants select-all limited to the content block the user is working in, and never the outer chrome.

## Behavior to build

1. **Inside editable fields:** Ctrl/Cmd+A is unchanged, so it selects the field's text. This covers input, textarea, and contenteditable, including the composer and rename fields.
2. **Otherwise, pick a scope** for the keypress. Use the first that applies:
   - the content block that holds the current selection or caret, if any;
   - else the block that holds the most recently clicked point;
   - else the element under the pointer, if the keypress comes while the pointer is over a block.

   A "content block" is the innermost readable unit:
   - a whiteboard topic card's body;
   - a single log entry or panel body;
   - one chat bubble or message;
   - the file preview dialog's content pane;
   - a memo or notes body;
   - the terminal is excluded, since it has its own handling.

   Find the real container classes in the dashboard and whiteboard assets, and list them in the commit `## AI Context`.
3. **Apply the scope:**
   - If a block was found, select exactly that block's text (a `Range` over its contents) and `preventDefault`.
   - If none was found (outer panel, rail, empty area), `preventDefault` and select nothing.
   - Pressing Ctrl/Cmd+A again while a block is fully selected keeps the same block; it does not widen to the page.
4. **Dialogs:** while a modal or dialog (file preview, etc.) is open, scope to its content pane. Never select the page behind it.
5. **Belt and braces:** give the outer chrome (rail, column headers, toolbars, button rows) `user-select: none` where it is not already set, so a drag or select-all cannot paint them. Do not apply it to any readable content, item IDs or paths users copy, or anything that is copyable today. If unsure, leave it selectable.
6. **Panels in sandboxed iframes:** Ctrl+A inside the frame stays the frame's own behavior. Do not inject into frames.
7. **Electron:** the desktop shell loads the same pages, so no shell change should be needed. If the shell's application menu binds Select All to `selectAll` in a way that bypasses the page's keydown, report it rather than changing the shell.

## Verification

- **Chromium tests** (cached Playwright; `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`, `WS_PLAYWRIGHT_MODULE` = the cached npx playwright module) on the real dashboard and whiteboard pages with fixtures:
  - Ctrl+A after clicking inside a topic card body selects only that body's text. The selection string equals the body's text and contains no rail or header text.
  - The same for a chat bubble.
  - The same for the file preview dialog content.
  - Ctrl+A on the outer panel or empty area selects nothing.
  - Ctrl+A in the composer textarea selects the textarea's text.
  - On macOS, the Meta+A variant behaves the same.
- **Narrow Go tests**, if templates change.
- **Screenshots** (light, 1280px), saved to a temp dir outside the repo with their paths listed:
  - a card after Ctrl+A;
  - the file preview after Ctrl+A.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. The module is `go 1.23`.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror.
- Do not ssh, do not launch Electron or any GUI window, do not restart the shared daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior change lands with a test. Commit on your branch only; do not merge. If the work needs a product decision beyond this plan, stop and report.
- Return: commits, the container list, tests run with results, screenshot paths, anything left unfixed.
