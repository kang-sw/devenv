# UI polish: mention button + rail unattached-branch dashed border

Owner feedback (verbatim, Korean), 2026-11-14:
1. "이 선택 멘션 캡슐, 캡슐 모양이 별로 안 예뻐요. 색감을 메인 대화창의 이런 톤으로 가져가고, 멘션 텍스트는 남기되(혼선이 있을 수 있으니까) 옆에 라벨 모양 아이콘도 붙게 하고, 모양도 라운드 사각형이 입력칸을 채우게."
2. "왼쪽 세션 리스트 바의 세션 안 붙어있는 브랜치의 점선이 너무 크게 눈에 띄어요. (완전 어그로) 이거 점선만 지워도 충분할 거 같습니다. 이미 dim 되어 있으니까."

Screenshots (read them):
- Current mention popup: /Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/745c945b89b477ba-1.png
  (whiteboard selection-mention popup: memo textarea "선택한 내용에 대한 메모 (선택 사항)" + a bright periwinkle pill button "멘션").
- Target tone: /Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/745c945b89b477ba-2.png
  (main dashboard conversation composer send button: muted dark-slate rounded-rectangle background, accent-colored icon, same height as the input box).
- Rail: /Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/sweatradartakingoverwrite/inbox/attachments/745c945b89b477ba-3.png
  (session rail: branch rows with no attached session, e.g. "fix/reconnect-serve-de…", "main · 루트", are drawn dimmed AND inside a large dashed rounded border).

## Task 1 — mention button (whiteboard page selection-mention popup)
- Locate the popup's submit button (search the whiteboard/dashboard assets for the memo placeholder string / i18n key and the "멘션" label).
- Restyle to match the main conversation composer send button: REUSE that button's existing CSS variables/tokens (background, icon color, radius, hover/focus/disabled states) rather than inventing colors; light and dark themes both.
- Shape: rounded rectangle (same radius family as the composer send button) that stretches to the full height of the memo textarea (align-self: stretch or equivalent), not a pill.
- Content: keep the text label "멘션" (i18n unchanged) AND add a label/tag-shaped icon beside it (inline SVG, tag/label glyph, consistent with existing icon style/size; aria-hidden on the icon; button accessible name unchanged).
- Behavior unchanged (click/keyboard submit, disabled state, focus ring).

## Task 2 — rail unattached-branch rows
- Remove ONLY the dashed border from branch rows that have no attached session. Keep the dim styling, layout spacing (no row height/position jump — replace the border with a transparent one of the same width or adjust padding so geometry stays identical), hover/focus behavior and everything else.

## Constraints
- Dashboard/whiteboard asset content only (T0). No Go behavior change, no protocol/store change, no i18n text change except a possible new aria label if strictly needed (avoid).
- Read the manuals the AGENTS.md Implementation Conventions table requires for these paths before editing.
- Static only: do NOT run go test, node, browsers, builds, or start daemons (a shared heavy-test slot is in use). `git diff --check` is allowed.
- Add/adjust a focused Go/asset test only if an existing test pins these selectors/styles; list deferred test commands in your final message.
- Two commits (one per task) on this branch, with ## AI Context. Reply with the hashes, files touched, and deferred verification commands (incl. a headless screenshot command the lead can run later).
