# HISTORY header correction — centered floating capsule

**Review-only revision following 650=no.** This replaces the rejected right-aligned rectangular-button proposal, not approved 649=A panel styling.

Owner line: `650=no^이전 더보기는 지금의 플로팅 캡슐이 중앙에 뜬 형태를 유지하면 좋겠어요. 위아래로 컴팩트해지기만 하게요.`

## Artifacts

Same allocated cache directory:

- `03045922-01-panel-card-design-variants-history-header-centered.html`
- `03045922-01-panel-card-design-variants-history-header-centered.fragment.html`
- `03045922-01-panel-card-design-variants-history-header-centered.check.cjs`

Prior A/B/C, compact-A, and rejected history-header artifacts are preserved.

## Corrected visual

- Keeps `이전 더 보기` as a **rounded, floating capsule at the geometric horizontal center**. It is not a small right-aligned rectangular utility button.
- A compact band places `기록 N` left, the pill center, and panel/log totals right.
- Uses symmetric side columns, not `space-between` with unequal left/right widths:

```css
.history-head {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto minmax(0, 1fr);
  align-items: center;
  column-gap: 8px;
  min-height: 28px;
}
.history-older {
  grid-column: 2;
  justify-self: center;
  border-radius: 999px;
  min-height: 28px;
  padding-inline: 12px;
  /* Keep original floating appearance, contrast and focus treatment. */
}
```

- Totals remain visible and may wrap inside the right-hand equal-width column at narrow sizes. The centered pill does not move or overlap them.
- Exhaustion hides only the capsule; the 28px band and always-open history remain. Individual panel/system/log-group collapse remains independent.
- Includes an illustrative old centered-pill/separate-row reference to compare vertical whitespace, plus revised available/exhausted and 360/320px samples. This is a shell study, not a pixel-perfect reimplementation of the production renderer.

## Verification

Both standalone and fragment passed cached-Playwright/headless-Chromium tests at **1440, 800, 360 and 320px**:

- All revised header bands were **28px tall**.
- Capsule horizontal-center error: **0px** in every visible-pill sample.
- No document/header overflow or label/pill/totals overlap.
- Rounded capsule radius retained at 999px.
- Available→exhausted leaves header height unchanged.
- Individual panel collapse works; whole history/log content remains visible.
- No JavaScript errors.

Command: `node .../03045922-01-panel-card-design-variants-history-header-centered.check.cjs`.

## Feasibility notes

The previous header report's ownership findings still apply: keep the actual `.history-older` within `.wb-history`, because `stream.js` looks up the button inside that owner and delegates clicks from it. Removing whole-history `<details class="topic-stream">` also requires ignoring obsolete saved whole-history open/closed state, without touching descendant fold state or current history-fit chunking. See the preserved `-history-header.md` report for exact selectors and line references.

This correction changes presentation direction only. No source, tickets, runtime, configuration, commits or implementation approval.
