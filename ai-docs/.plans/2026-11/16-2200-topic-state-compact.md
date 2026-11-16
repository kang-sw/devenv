# Whiteboard: compact topic-state badge for every state (proposal A)

Owner request (2026-11-16 21:42 KST, verbatim): "토픽 카드의 구현 중 상태 있잖아요? 이거 조금 산만한 느낌이라, 또 너무 길어서 자꾸 헤더를 2행으로 만들어버리기도 하고요. 전반적으로 컴팩트한 디자인을 목업해볼 수 있을까요? 생각으로는 "구현 중" 같은 고정 텍스트를 빼고, ETA도 저 파란 뱃지 안에 합쳐버리고(서로 별개의 태그처럼 보여서요) 시계 아이콘 + 5_20m (즉 최대 시간은 우하단의 작은 글씨로 붙게)처럼 그리고, etc etc 하여간에 그런 느낌으로 좀 이쁘고 컴팩트한 목업 보고 싶네요." Follow-up (21:42): "파란 뱃지뿐 아니라 다른 상태 뱃지 전체도요. `pi 작업자 5` 같은 텍스트도 마우스를 올렸을 때 팝업되게 보이는 걸로 충분하지 않나 싶고."

The owner approved **proposal A** of the mockup ((1193)=ok). Proposal B in the same file is rejected; do not implement it.
Mockup (visual and behavior contract): `ai-docs/assets/mockups/261116/topic-state-compact/mockup.html` with `mockup-light.png`, `mockup-dark.png`, and `owner-before.png` (today's look). It is already copied into this worktree, untracked; commit it with the implementation. Its section 5 ("구현 시 바뀌는 것") lists the source lines to change; verify each against the code before editing, do not assume.

## What to build (match proposal A)

1. **One pill per state.** A state icon plus, when there is a time, the time inside the same pill: `<b>elapsed</b><small>/ETAm</small>` with the ETA maximum as a small subscript at the lower right. Without an ETA but with an estimate: `~Nm`. Without any time: the icon alone (`solo`). No fixed state word ("구현 중" etc.) and no owner name in the pill. The separate `.eta` / `.state-estimate` pills go away.
2. **Icons.** Add five sprite symbols to the dashboard/whiteboard sprite used by the page, as drawn in the mockup: implementing = stopwatch, researching = magnifier, discussing = speech bubble, waiting = hourglass, awaiting-confirmation = check in a dashed circle (`i-st-impl`, `i-st-research`, `i-st-discuss`, `i-st-wait`, `i-st-confirm`). Colors per state as in the mockup. The stopwatch must not be confused with the dependency clock next to the title.
3. **Overdue.** When elapsed exceeds the ETA, the pill turns to the mockup's warning color (`over` class).
4. **Hover/focus tip.** The state word, the owner, the full ETA sentence ("ETA {0}분 중 {1}분 경과", plus "{0}분 초과" when over), extension info ("연장 {0}회 · 최초 {1}분"), and the estimate move into a tip. The topic card clips overflow, so render the tip as a fixed popover like the dependency popover (`.dw .pop`, reuse the deps.js placement logic), with the progress bar shaped like `.dw .bar`. The pill is focusable (`tabindex="0"`, `role="img"`, `aria-label` = the full sentence). Per-minute number changes update only the visible number, the `over` class and the aria-label (no live region).
5. **Server render = client render.** The first server render (`page.tmpl` topic-state block) emits the same markup as the client tick (`ago.js` `badge()`), so there is no visual jump on the first tick. `.tname` always carries `title="{{.Label}}"` so a truncated title is readable.
6. **Extension data.** Add `data-state-initial-eta-minutes` and `data-state-extensions` attributes from the existing `InitialETAMinutes` / `ExtensionCount` fields so the tip can show them. Render-only: no store, tool, or protocol shape change. If one turns out necessary, stop and report.
7. **One-row header.** Replace the old badge CSS with the mockup's rules so title + icons + pill stay on one row; a long title ellipsizes first (mockup section 3, 420px card). Remove the 600px media rule that pushed the badge to its own row if the mockup's narrow case holds without it.
8. **Fix the waiting-state class bug.** Today `ago.js` draws waiting as `stb wait` while the CSS styles `.stb.wb-state-wait`, so the live "대기" dot is a filled grey dot. Use one class name everywhere so the server and client render agree; add a test that would have caught the mismatch.
9. **i18n.** Add `js.wb.state.eta.tip`, `js.wb.state.over`, `js.wb.state.extended`, `js.wb.state.eta.max`, `js.wb.state.estimate.short` (Korean source text above; `/{0}m` and `~{0}m` for the short forms) to every locale table under `internal/wsi18n`, with proper translations and key parity. Existing state-name keys and `js.wb.state.estimate` stay in use by the tip. Do not delete `js.wb.state.eta` if anything still references it; if nothing does, leave it in place anyway (removal is out of scope).

## Verification

- Go tests (narrow `-run`) for the template output: every state renders the icon pill, time markup with and without ETA/estimate, the data attributes, `.tname` title, and the unified waiting class.
- Node or Chromium tests for `ago.js`: tick updates the number and `over` class, the aria-label sentence, the tip opens on hover and focus and closes on leave/Esc/blur, and the server markup equals the first client render.
- i18n key-parity test.
- Screenshots: headless Chromium (cached Playwright on this machine) of the real whiteboard page with a fixture board that has topics in all five states including an overdue implementing topic and a narrow (420px) card with a long title, light and dark, 1280px wide; plus one with the tip open. Save them to a temp directory outside the repo and list absolute paths in your reply; do not commit them. The lead compares them side by side with the mockup PNGs before merge, so match the mockup.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. Node is available. Chromium tests need `PLAYWRIGHT_BROWSERS_PATH=$HOME/Library/Caches/ms-playwright` and `WS_PLAYWRIGHT_MODULE` pointing at the cached npx playwright module.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- The module is `go 1.23`: do not use newer standard-library APIs (for example `t.Chdir`).
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror.
- Do not ssh anywhere, do not launch Electron or any GUI window, do not restart or replace the shared dashboard daemon, do not run `scripts/dev.py rerun`.
- Public repository: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior change lands with a test.
- Commit on your branch only; do not merge. If the work needs a product decision beyond this plan, stop and report instead of expanding.
- Return: commits, tests run with results, screenshot paths, anything left unfixed and why.
