# Rail session rows: favorite via context menu + star badge, rename right after create, tighter icon spacing

Owner request (2026-11-16 21:58 KST, verbatim):

> 이 별모양을 우클릭 했을 때 별도의 즐겨찾기 토글 행으로 두고, 아이콘도 하네스 아이콘 좌하단에 작은 별로 표시되게 했음녀 좋겠어요. 자꾸 잘못 눌려서 열받네요. 별표시는 자주 하는 것도 아닌데. // 세션 추가 직후에, 세션의 이름을 변경할 수 있는 상태(즉 rename 버튼 누른 상태)로 가면 좋겠고요. // 이거 지금 서브에이전트 숫자를 위해서 마진이 어정쩡하게 더 벌어져있는 상태인데 이거 그냥 서브에이전트 숫자 늘어서 이름이 오른쪽으로 조금 밀려나더라도 그냥 서브에이전트 숫자 없을 땐 붙어있는 게 낫겠습니다.

The owner approved the mockup: (1198)=a, "implement as the mockup shows". The mockup is `ai-docs/assets/mockups/261116/session-row-tweaks/`: `mockup.html` (with `?theme=dark|light`), `mockup-light.png`, `mockup-dark.png`, and `owner-before.png`, the owner's screenshot of today's state. The lead copied this folder into your worktree. Commit it unchanged with your first commit. Read `mockup.html` end to end; it is the spec. Its "원인" and "변경" sections name the code.

## Defaults the owner accepted (announced with the approval; no objection)

1. All UI wording says "즐겨찾기", with the matching en string ("Favorite"/"Unfavorite" or the existing en convention). It replaces "별표" everywhere it appears for this feature.
2. When the rail is collapsed (narrow screens below 1600px), creating a new session opens the page header's rename field instead of a rail-row rename.
3. The new session row is appended at the end of its worktree's list.

## Known code facts (verify)

- Spacing cause: `internal/wsdashboard/assets/dashboard-rail.css:362`, where `.hiw` is `width:28px`, and `:363`, where `.sbn` is `position:absolute`. The icon wrapper reserves room for the subagent count even when there is none. Change it so the name sits right next to the icon when there is no count. When a count exists, it may push the name right.
- A row context menu already exists: `rpRowMake` / `railRowMenu` in `dashboard-rail-popovers.js`, around lines 537-683. Add the favorite toggle row there. Remove the inline star button from the row.
- Favorite badge: a small star at the bottom-left of the harness icon, drawn only for favorited sessions, sized and placed as in the mockup. Favorite sorting and persistence stay exactly as today; only the control moves.
- New-session signal: `dashboard-leads.js:224-232` writes a sessionStorage key, then `railMarks()`, then `railRename`. Use this path to enter rename mode on the new row right after creation.
  - Enter it only for a session this page just created. Never enter it on reload or for sessions created elsewhere.
  - Escape or blur keeps the default name.

## Verification

- Chromium tests with cached Playwright (`PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright`, `WS_PLAYWRIGHT_MODULE` = the cached npx playwright module). Extend the existing rail browser tests where possible.
  - The row has no star button.
  - Right-click, then "즐겨찾기", favorites the session: the badge appears and the favorite order or section updates as today. The menu then shows the un-favorite wording, and toggling again clears the badge.
  - Creating a session leaves its new row in rename mode with the input focused and its text selected. In the collapsed rail it opens the header rename instead.
  - Name-to-icon gap: measured with the count absent, it equals the mockup's value. With a count present, the name shifts right.
- Narrow Go tests if templates or i18n change (e.g. `go test ./internal/wsi18n -count=1`; `go test ./internal/wsdashboard -run 'TestRail|TestDashboardFrame|TestI18n' -count=1`).
- Screenshots (light and dark, 1280px and a narrow width) of rows with and without favorites and counts, the open context menu, and rename-after-create. Save them outside the repo and list the paths. The lead compares them side by side with `mockup-light.png` / `mockup-dark.png` before merge.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. The module is `go 1.23`.
- Before editing, read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit. New CSS classes must not collide across module stylesheets (`TestCSSClassesDoNotCollideAcrossModules`): use the dashboard `d-` prefix or scope them under an existing container.
- Do not ssh, do not launch Electron or any GUI window, do not restart the shared daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits.
- Every behavior change lands with a test. Commit on your branch only with `## AI Context`; do not merge. Do not remove any functionality beyond the inline star button the owner asked to move.
- Return: commits, tests run with results, screenshot paths, and anything left unfixed.
