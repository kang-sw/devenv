# Archived decision timeline — compact detail refinement

## Outcome

Prepared one refined direction for the owner's `643=ok` density follow-up. The screenshot was read directly. It shows a summary row already containing identity, class, title and `ok`, followed by a large explanatory renderer placeholder and another `ok` badge. The revised mockup removes that repeated presentation rather than shrinking all text or clipping useful details.

This is a **mockup for MAIN's review**, not implementation or approval to ship. No source, tickets, configuration, branches, worktrees, installed assets, or live daemon state were changed.

## Artifacts

- Standalone: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/9fa609fc-01-history-decision-density-mockup.html`
- Embeddable fragment: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/9fa609fc-01-history-decision-density-mockup.fragment.html`
- Browser check: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/9fa609fc-01-history-decision-density-mockup.check.cjs`
- This report: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/9fa609fc-01-history-decision-density-mockup.md`

The fragment is **17,689 bytes**. Both artifacts are self-contained, with no external library, font, image, network request, or application runtime dependency.

## What changed in the proposed presentation

1. **Identity, class, title and answer/status appear once, in the summary row.** Expanded detail does not repeat them.
2. **A plain `ok` does not create a second answer row.** Its exact text remains in the summary badge and tooltip.
3. **No empty disclosure body.** The short technical example has no additional body or owner note, so it stays one compact row without a pointless expand affordance. A real body, reason, choices, recommendation or substantive answer requires a disclosure and is not suppressed.
4. **Substantive owner wording stays visible in expanded detail.** Policy examples retain the raw owner note once, without repeating the selected-answer pill.
5. **Assumed, withdrawn and superseded remain different states.** The assumed example keeps its interpreted value and the user's ambiguous words; the withdrawn example has a reason; the superseded link appears once in the row.
6. **Renderer placeholders are compact slot markers, not decorative blank boxes.** Existing rich content is still delegated to the real question renderer. There is no fake table/code/HTML renderer, forced detail height, clipping, line clamp or ellipsis.
7. **Narrow display retains time information.** The separate time gutter becomes inline time metadata at narrow widths instead of removing the timestamp or imposing a mandatory extra metadata row. Long titles naturally wrap.

## Examples shown

The timeline contains seven archived-item specimens, surrounded by lead logs, one batch-creation log and a folded system-event line:

- Short technical decision with plain `ok` and no extra content.
- Technical decision with an existing-renderer slot for body and substantive reasoning.
- Policy choice with existing-renderer slot for body/options/recommendation and a substantive owner note.
- Assumed answer with the ambiguous owner words and explicit interpretation.
- Withdrawn item with its reason.
- Superseded item with its replacement reference.
- Rich policy content, represented by a compact existing-renderer slot and a substantive answer.

The before/after comparison uses the screenshot's tall-placeholder arrangement for one short technical example. It is not a second design option or an implementation baseline test.

## Verification

VERIFIED using locally cached Playwright and headless Chromium; no downloads:

```text
node /Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/9fa609fc-01-history-decision-density-mockup.check.cjs
```

Both standalone and fragment passed at **1440, 1100, 800, 360 and 320px**:

- No document, row, body, title or card overflow.
- Seven archived-item specimens; five meaningful disclosures.
- No title clipping or ellipsis.
- Plain `ok` represented once, with no empty detail block.
- No duplicate status/answer pills in expanded details.
- Substantive owner note retained.
- Narrow inline time retained.
- Expand-all, collapse-all and keyboard Enter/Space disclosure behavior passed.
- No page JavaScript errors.

Illustrative short-example geometry, identical across both artifacts:

| Width | Previous mockup example | Revised short row |
|---|---:|---:|
| 1440 / 1100 | 191px | 31px |
| 800 | 201.38px | 31px |
| 360 | 201.38px | 32px |
| 320 | 201.38px | 53px, natural title wrap |

These measurements compare **mockup chrome and placeholders**, not real rich-renderer content or production performance. The 24px renderer marker is only a marker; actual body height remains content-driven.

## Decisions and implementation cautions

- Interpreted the request as removing duplicated presentation and empty expansion, **not** deleting meaningful body, rationale, alternatives, original answers, or archive events.
- Conditional disclosure is a proposed refinement: only omit an expansion when there is genuinely no additional content. Do not infer emptiness from a rough string match with the title or classify all technical `ok` items as empty.
- Keep the existing open-question region unchanged. Batch creation stays one log; archive items enter the timeline at their actual archive time. No answer/status/archiving semantics are changed.
- Sample IDs and answers are design fixtures, not current board questions or decisions.
- The original v3 was found through the bounded handoff reference and read at `/tmp/mockup-history-compact-v3.html`. No broader repository audit was performed.
- The rich existing question renderer is deliberately not reproduced. Real renderer output, keyboard order inside that renderer, live updates, clipping/chunking integration and persisted expansion behavior require implementation-phase verification after MAIN/owner review.

## Blockers

None. No commit.
