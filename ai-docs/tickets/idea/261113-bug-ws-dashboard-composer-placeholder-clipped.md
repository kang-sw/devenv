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

## Lead

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js` `grow()`
  (near line 1330) sizes the textarea from `scrollHeight`, which tracks the
  value; an empty textarea's `scrollHeight` need not include a wrapped
  placeholder, and `grow()` may not run on first render or on width change.
- The placeholder string lives in `internal/wsi18n/tables/*.json`; longer
  translations wrap sooner.

## Expected

On first render and on width change, an empty composer is tall enough for
its whole placeholder (still capped at the existing 160px).
