# Floating toolbar — approximately 60% overall height

**MAIN-direct cache-only review mockup. Not product implementation or implementation approval.**

Owner request 652, translated into English for this tracked copy: "Perhaps this is because I have been giving numbers too vaguely. Please aim to reduce the thickness to around 60%, without making it visually awkward."

Interpretation: the visible floating toolbar should be approximately **60% of its nonfloating overall height**, not 60% shorter. A 46px desktop bar therefore aims for about 28px.

## Revision

- Nonfloating appearance remains the original: 30px buttons, 14px icons, 7px block padding. Floating uses 24px buttons, uniformly scaled 12×12px icons, 1px block padding and a 14px counts line-height. Label fonts and shortcut text retain their original sizes; glyphs are not vertically distorted.
- Desktop face is **46→28px (60.9%)**, versus the original 38px floating face and the too-subtle icons iteration's 36px.
- Preserve every action, order, count, horizontal button box, column gap and responsive label/shortcut visibility. Original container CSS is unchanged; new rules were inserted once at a unique complete declaration, not through global selector replacement.
- The same parent toolbar and buttons remain mounted. The slot reconstructs the original flex-row footprint, including wrapped counts/action rows. No card restyling, reparenting, duplicates or geometry animation.
- Floating buttons retain **30px hit regions**: 1px above and 5px below the 24px visible button. This asymmetry avoids clipping at the sticky viewport top. Regions remain disjoint, including across wrapped action rows. These are **not 44px targets**.
- When actions themselves wrap, use an 8px row gap instead of 6px to avoid fractional-pixel edge sharing. Inline geometry remains unchanged. Counts-only wrapping keeps the original gap. This intentionally limits the reduction at the narrowest fragment width.
- Terminal popover keeps its inherited face-relative anchor and Escape/opener-focus behavior. Its bottom anchor follows the smaller face.

## Measured results

Cached Playwright / headless Chromium **148.0.7778.96**, both artifacts at **1440/800/360/320px**, each with wide/phone/small previews: **24 combinations passed**, exit 0.

Heights compare the original nonfloating face to this revision's floating face:

| Artifact | Browser width | Main preview | Phone / small previews |
|---|---:|---|---|
| HTML | 1440, 800 | 46→28px · 60.9% | 76→48px · 63.2% |
| Fragment | 1440, 800 | 46→28px · 60.9% | 76→48px · 63.2% |
| HTML | 360, 320 | 76→48px · 63.2% | 76→48px · 63.2% |
| Fragment | 360 | 76→48px · 63.2% | 76→48px · 63.2% |
| Fragment | 320 | 82→60px · 73.2% | 82→60px · 73.2% |

The directly opened fragment inherits the browser's body margin, leaving a 266px face at 320px. Files wraps into a second action-bearing row. Reaching 60% there would sacrifice preserved horizontal layout, legible labels or usable nonoverlapping hit geometry; **73.2% is the measured limitation, not claimed as 60%**.

Verified against matching unchanged original artifacts:

- At-top face dimensions and every button's responsive x/width, label and shortcut visibility; floating horizontal button/count positions and widths; five actions, order and count text.
- Unchanged DOM identity and reserved slot; **0px pane-relative content-anchor error and 0px sticky-top error** in all 24 cases. Scroll padding tracks the live face height plus 10px. Native target arrival stays below the face; return-to-top geometry is restored.
- Focus survives scrolling; visible 2px keyboard outline, Enter activation, full five-action Tab order, reverse Tab, overview toggle, keyboard menu opening, Escape closing and opener focus all pass.
- Actual point ownership at three horizontal positions and five vertical positions per action, including outside the visible face, passes. Expanded rectangles are viewport-visible and disjoint. An actual mouse click in the terminal's lower extension opens its menu.
- Menu remains inside its viewport and matches the original **5px outer-border gap / 9px left inset**. No relaxed anchor constants.
- No document/pane horizontal overflow, no clipped label boxes, no page JavaScript errors.
- SHA-256 checks preserve **all 249 pre-existing files** in the artifact directory, including baseline and icons HTML, fragment, reports and checkers.
- Visually inspected desktop top/floating and 320px fragment captures; labels and uniformly reduced icons remain readable.

### Failure retained and corrected

The first generation guard found that a short CSS selector anchor occurred twice and stopped before any write. It was replaced by a unique complete declaration anchor.

Initial browser checks then caught a real point-ownership defect at 320px in the fragment: adjacent 30px hit regions mathematically touched, but fractional-pixel browser hit testing let Files win an overview edge point. Added the 2px safety gap only when action rows wrap. All prior hit, anchor, menu and horizontal/responsive assertions remain; stronger disjointness, full keyboard-order and measured-height assertions were added. Final full run passed.

Run: `node 03045922-01-panel-card-design-variants-sticky-toolbar-sixty.check.cjs`

## Artifacts

Tracked references beside this report are [approved.html](approved.html) and [approved.fragment.html](approved.fragment.html). They are unchanged copies of the approved originals, including their human-facing Korean UI. This report preserves the original findings; only the owner quotation was translated and these tracked-reference/provenance sentences were added. Its pre-approval disclaimer describes the report's original review stage; the ticket separately records MAIN's later approval relay. The checker is an original cache reference only and was not run or copied during capture.

Original artifact names, all under `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc/review-paths/`:

- `03045922-01-panel-card-design-variants-sticky-toolbar-sixty.html`
- `03045922-01-panel-card-design-variants-sticky-toolbar-sixty.fragment.html`
- `03045922-01-panel-card-design-variants-sticky-toolbar-sixty.md`
- `03045922-01-panel-card-design-variants-sticky-toolbar-sixty.check.cjs`

## Scope and limitations

Local mock controls only. Web/search/Files do not implement real application actions; terminal menu and overview retain the original demonstration semantics. No validation of production scroll-keep, document-scroller fallback, real profile menus, modal/overview integration, iframe lifecycle or live updates. No package installation, server or live daemon was needed. No application/repository/ticket/configuration/runtime/session/branch/worktree changes or commits were made. Earlier artifacts remain unchanged.
