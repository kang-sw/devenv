---
title: "Dashboard composer placeholder is clipped on first render"
---

# Dashboard composer placeholder is clipped on first render

## Background

Owner report, macOS, 2026-11-13 (cosmetic): the session composer textarea
sometimes renders its placeholder ("이 세션에 메시지를 보냅니다. Enter로 전송,
Shift+Enter로 줄 바꿈. 보드 답변과 붙여넣은 이미지가 함께 첨부됩니다.") wrapped
to two lines but sized for one, so the second line is cut off at the bottom.
The composer should be sized to fit its placeholder from the first render.

## Prior Decisions

- 591aa4b0 (2026-11-06, commit): "Auto-grow is JS (height = scrollHeight on input and on reveal); CSS-only field-sizing is not yet universal. Resize handle and scrollbar are off." — bearing: constrains
- 4d86dce6 (2026-11-07, commit): "a thin icon-only send button on every device, growing with the textarea, so a phone soft keyboard can send in either input mode." — bearing: constrains
- 783d58c5 (2026-11-07, commit): "button 34px wide, 38px tall empty and 114px with five lines, equal to the textarea each time" — bearing: constrains
- 5df23430 (2026-11-09, commit): "The support chat card builds its own .composer.supcomp with a plain .crow textarea and its own handlers." — bearing: constrains
- 0d777938 (2026-11-09, commit): "Hover changes colour only, never width, so nothing reflows (the composer's mention layer measures the textarea's scrollbar width)." — bearing: constrains
- 62304312 (2026-11-12, commit): "the composer bound and DR_MIN floor are unchanged. drawer_height_test.go stubs drMax" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js (grow, setInput, renderCmd, init) and possibly assets/dashboard.css; the i18n tables need no edit |
| scope.surface | internal | dashboard asset content only, no exported Go symbol |
| scope.new_public_symbol | no | none |
| scope.new_type_contract | no | none |
| scope.test_surface | existing | wsdashboard/slash_command_test.go and composer_toast_test.go stub grow() under node; no test measures textarea height, so a new case or file may be needed |
| complexity.reuse_points | confirmed | grow() at dashboard.js#L1330; placeholder set at setInput dashboard.js#L316-L321 and renderCmd dashboard.js#L1678; ResizeObserver already used at dashboard.js#L195 |
| complexity.side_effect_risk | moderate | grow() is called from the input handler, send, restore and command sites; a width-triggered ResizeObserver that sets height can loop if it observes height |
| risk.correctness | moderate | scrollHeight of an empty textarea ignores a wrapped placeholder in some engines, so the fix needs a measurement the engines agree on, and only a real browser can confirm it |
| risk.fit | moderate | the 2026-11-06 decision keeps auto-grow in JS because field-sizing is not universal; CSS field-sizing alone would reverse it for older engines |
| risk.test | moderate | node harness tests stub layout; headless browser verification is needed and no existing test measures placeholder height |
| risk.security_or_contract | low | asset-only change, tier T0, no route or wire change |

## Lead

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` `grow()`
  (line 1330) sizes the textarea from `scrollHeight`, which tracks the
  value; an empty textarea's `scrollHeight` need not include a wrapped
  placeholder. `grow()` is never called at first render or on a width change:
  it runs only from the `input` handler (dashboard.js#L2226) and after a
  programmatic value set or clear (L1710, L1924, L2013, L2031, L6883); no
  `resize` or `ResizeObserver` is wired to it. The height before any call is
  just `rows="1"` plus the `min-height: 38px` / `max-height: 160px` rule at
  `assets/dashboard.css#L435` (textarea markup: `assets/dashboard.tmpl#L22`).
- The placeholder is also swapped at runtime without a `grow()` call:
  `setInput()` (dashboard.js#L316-L321, from the input-mode setting, run once
  at init L6904) writes `HINT[mode]`, and `renderCmd()` (L1678) writes the
  `js.cmd.args` text or the saved `ctextHint`. The strings are keys
  `js.hint.enter` / `js.hint.mod` / `js.cmd.args` in
  `agents-plugin-tool/internal/wsi18n/tables/*.json` (line 311-312, 1638;
  ten tables with key parity); longer translations wrap sooner.
- A second textarea, the support-chat card (dashboard.js#L4741), has its own
  inline grow and is a separate site. The tracker `cbox` editor is separate
  too.

## Expected

On first render and on width change, an empty composer is tall enough for
its whole placeholder (still capped at the existing 160px).

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin-tool/)
- Convention: ai-docs/manuals/release-compatibility.md (declared for agents-plugin-tool/internal/)
