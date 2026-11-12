# Plan: native tooltip on whiteboard item class tags

Source: user request on the ws whiteboard (2026-11-12): hovering an item's
class tag (POLICY / TECHNICAL / ANNOUNCED) should explain what that class
means, per the workflow manual's `### Asking the user`; a browser-native
tooltip is enough; do not attach the long text to every element.

## Change

- Item class tags are rendered in
  `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`
  (`<span class="tag {{.Class}}">` in the `ack` template, and wherever a
  policy item's tag is rendered — find every `tag {{.Class}}`-style site).
- Add one delegated `mouseover` (and `focusin` if the tag is focusable)
  handler in the board page's script (the whiteboard assets already have
  `wsT`; put it in the most fitting existing file, e.g. `form.js` or
  `live.js`) that, on first hover of a class tag without a `title`, sets
  `title` from `wsT("js.wb.tag.<class>")`. One string per class per
  locale; the server-rendered HTML carries nothing extra. Make sure it
  also works for tags inserted later by live updates (delegation on the
  document handles that).
- Check whether the dashboard renders the board through the same page
  assets (topic frames / board region in
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`); if the
  dashboard renders class tags with its own markup, cover that path too
  with the same keys (the dashboard also has `wsT`).
- The static embed snapshot (no script) gets no tooltip; that is accepted.
- i18n: add `js.wb.tag.policy`, `js.wb.tag.technical`, `js.wb.tag.announced`
  to every table under `agents-plugin-tool/internal/wsi18n/tables/`
  (the unused/missing-key tests require it), with real translations, and
  make sure the keys reach the page's JS dictionary (follow how existing
  `js.wb.*` keys are shipped). Korean (ko.json) wording:
  - policy: "정책 질문 — 사용자의 선호·우선순위·제품 방향에 달린 결정입니다. 선택지 중 하나를 골라 주세요."
  - technical: "기술적 판단 — 사용자 선호와 무관해 에이전트가 기술적 근거로 고른 결정입니다. 근거는 항목에 적혀 있고, 이의가 없으면 ok."
  - announced: "기본값 안내 — 기존 제약·이전 결정·프로젝트 관례에서 답이 하나로 정해지는 항목입니다. 확인만 해 주세요."
  English: "Policy question — depends on your preferences, priorities, or product direction; pick an option." / "Technical decision — the agent's call on technical grounds, independent of your preferences; the reasoning is on the item; reply ok unless you object." / "Announced default — has one answer that follows from existing constraints, earlier decisions, or project convention; just acknowledge."
  Translate the others in the same sense.
- If `ai-docs/ref/tracker-mockup/i18n-en.js` or any generated i18n file
  covers `js.wb.*` keys, regenerate it the way its test expects.

## Verification

- A JS/Go test in the existing style (see `wswhiteboard` `*_test.go` that
  run page scripts under node, e.g. `pill_title_test.go`) that hovering a
  class tag sets the localized title once and that no title is in the
  rendered HTML.
- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 15 -- test ./internal/wswhiteboard/... ./internal/wsdashboard/... ./internal/wsi18n/...`
  (never run heavy go build/test locally).

## Boundaries

- No change to item markup beyond what the handler needs; no CSS tooltip.
- Do not touch tracker files or the composer/answer-block code (another
  delegate is changing `wsinbox` and the composer in `dashboard.js`).
