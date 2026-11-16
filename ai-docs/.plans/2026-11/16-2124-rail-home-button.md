# Rail: home button beside settings, no toggle on a repeated session-label click

Owner request (2026-11-16 21:16 KST, verbatim): "좌하단 설정 버튼 오른쪽에 홈 버튼을 하나 두고 그걸 누르면 홈으로 가게 하고, 지금 좌단 세션 리스트의 각 세션 라벨을 두번 클릭하면 토글되어버리는 UX는 없애는 게 낫겠어요. 무지 불편하네요 ..."

1. Home button. In the dashboard's left session rail, the bottom-left footer holds the settings (gear) button. Add a home button immediately to its right, using the same button style and size and an existing icon sprite if one fits (a house; add one to the existing sprite set only if none exists). Clicking it navigates to the dashboard home page (the page the app opens on: the "시작하기" start page / root). It needs a tooltip and an aria-label, translated in every locale table under internal/wsi18n (all tables keep the same keys). It must work in both the browser dashboard and the Electron shell, and from both the home page and a session page.

2. No toggle on a second click. Today clicking a session's label in the left rail list opens it, and clicking the same label again (a double click, or two clicks) toggles it closed/away. Remove that toggle: clicking the label of the session that is already open keeps it open and does nothing else. Find the code path (rail/session-list click handlers in internal/wsdashboard/assets) and remove only the toggle-on-label behavior. Keep every other way of closing or collapsing (explicit close controls, keyboard shortcuts) unchanged. If the toggle is shared with something the owner did not mention (for example a workspace or branch header row), leave those alone and report it.

Verification:
- Node or Go tests for both behaviors where the rail already has a harness (a second label click does not toggle; the home button exists with its label and navigates to home).
- Screenshots with headless Chromium (cached Playwright on this machine) of the rail footer before and after, in light and dark theme, at 1280 px wide. Save them to a temp directory and list the paths in your reply; do not commit them. The lead compares them before merge.
- Narrow `-run` for the wsdashboard and wsi18n tests you touched.

Rules: Go tests with `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`; never the whole `./internal/wsdashboard` package. Do not ssh, do not launch Electron or any GUI window, do not restart the shared daemon. No machine-local paths or keys in commits (public repo). Commit on your branch only; do not merge. Return commits, tests, and screenshot paths.
