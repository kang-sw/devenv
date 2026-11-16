# Whiteboard: diff gutter on an edited panel, with the previous content on hover

Owner request (2026-11-16 21:33 KST, verbatim): "자잘한 UI 건의사항인데, wb-message에서 기존 panel을 update로 편집했을 때, 각 패널에서 이전 패널 대비 편집된 부분을 diff gutter처럼 표시해줄 수 있을까요? 그래서 마우스를 올리거나 클릭하면 이전 컨텐츠도 볼 수 있고, 뭐 그런 식으로요."

The owner approved the mockup as drawn ((1192)=ok). It is the visual and interaction contract:
`ai-docs/assets/mockups/261116/panel-diff-gutter/mockup.html` (with `mockup-light.png` and `mockup-dark.png`). It is already copied into this worktree, untracked; commit it together with the implementation. Open the HTML: card A is interactive (hover, click-to-pin, Esc, compare toggle), and the CSS blocks in it are labelled with the real source ranges they were copied from.

## Facts established by the mockup pass (verify, do not assume)

- Every panel version is stored as a full immutable entry sharing one panel ID (`internal/wswhiteboard/entries.go` around 26-28, 198-224, 254). The page template already links a version to its prior one (`page.tmpl` around 96, `.Prior`, `.Historical`). So this is render-side only: no store or schema change.
- The topic body already has block-level change marks: `diffBodies` (`changes.go` around 129), marks stored in `store.go`, bars drawn by `assets/chg.js`, CSS in `page-stream.css` around 212-223. Panels get none today (`panelHTML` in `render.go` around 630 passes no marks).
- The body's `overflow-x:auto` and the card's `overflow:hidden` clip children, so marks go in a zero-height layer after the body (as `chg.js` does), and popovers are placed in page coordinates.

## What to build (match the mockup)

1. **Marks.** On the latest version of an edited panel, compute a block diff against its immediate prior version, reusing the topic-body block pairing (`diffBodies`) and its color palette.
   - Added block: a green bar.
   - Modified block: a blue bar.
   - Removed block: a red wedge between blocks.
   - Draw the marks on the panel body's existing 1px guide line: 3px at rest, 5px on hover.
   - Only the latest version is compared, and only with the version right before it. Folded older versions get no marks. A panel with a single version gets nothing.
2. **Hover popover.**
   - Modified mark: the popover header shows "수정됨" and "vN hh:mm → vM hh:mm". The section "이전 내용" shows the old block with removed words struck through in red. Meanwhile, the new words are highlighted green in the body. This needs a word-level diff inside a modified block; keep it a small, tested helper.
   - Removed wedge: the popover shows "삭제됨" and the section "지워진 내용".
   - Added mark: only a one-line tooltip, "추가됨 · vM hh:mm".
3. **Pin.**
   - Clicking a mark pins its popover. The pin button, ×, or Esc closes it.
   - The footer shows "클릭하면 고정 · Esc로 닫기" and the link "vN 전체 보기 ↑" to the existing prior-version anchor.
4. **Header chip.**
   - Show "vN 대비 +a ~b −c" on the latest version's header. Its title is "vN · hh:mm 대비 변경 — 클릭하면 본문에서 전체 비교".
   - Clicking the chip toggles the in-place compare view. Removed blocks and words appear struck in red in place, and added parts appear green.
   - The compare view shows the note "vN · hh:mm 대비 변경을 본문에 표시하고 있습니다." with the action "비교 끄기".
5. **Framed panels.** For a panel rendered in a sandboxed iframe, a popover inside the frame would be clipped at its edge. Either place the marks and popover outside the frame in the host page, or, if that is not feasible within this change, show marks without popovers for framed panels and say so in the commit's `## AI Context`.
6. **Live updates.** When a new panel version streams in, the marks must reflect the new latest-vs-prior pair. Follow how `chg.js` refreshes topic marks.
7. **i18n.** Add every new string to every locale table under `internal/wsi18n` (Korean is the source text above), with all tables keeping the same keys. Format version and time placeholders the way existing strings do.
8. **Accessibility.** Marks are focusable buttons with an aria-label carrying the change kind. Esc closes, and the popover is reachable by keyboard.

## Verification

- Go tests (narrow `-run`) for:
  - the panel block diff and the word-level diff helper: added, modified, removed, unchanged, single-version, and multi-version cases where only latest-vs-prior counts;
  - render output (marks present only on the latest version; chip counts).
- Node or Chromium tests for:
  - hover opens the popover, click pins it, Esc closes it;
  - the chip toggles the compare view;
  - a live new version updates the marks.
- i18n key-parity test.
- Screenshots: headless Chromium (cached Playwright on this machine) of the real whiteboard page, using a fixture board whose panel has 3 versions, light and dark, 1280 px wide. Capture:
  - marks at rest;
  - hover on a modified mark;
  - a pinned removed wedge;
  - the compare view.

  Save them to a temp directory outside the repo and list the absolute paths in your reply; do not commit them. The lead compares them side by side with the mockup PNGs before merge, so match the mockup.

## Common rules

- Go tests: always `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local`. Never run the whole `./internal/wsdashboard` or `./internal/mcp` package; use narrow `-run`. Node is available.
- Read the AGENTS.md `### Implementation Conventions` manuals that cover the paths you edit before editing.
- Do not touch `agents-plugin/rsrc/whiteboard/whiteboard.md` or its wsflow mirror.
- Do not ssh anywhere, do not launch Electron or any GUI window, do not restart or replace the shared dashboard daemon, do not run `scripts/dev.py rerun`.
- No store, tool, or protocol shape change is expected. If one turns out to be needed, stop and report before making it.
- The repository is public: no machine-local paths, hostnames, PIDs, or session keys in code, tests, commits, or this plan.
- Every behavior change lands with a test.
- Commit on your branch only; do not merge. If the work needs a product decision beyond this plan, stop and report instead of expanding.
- Return: commits, tests run with results, screenshot paths, anything left unfixed and why.
