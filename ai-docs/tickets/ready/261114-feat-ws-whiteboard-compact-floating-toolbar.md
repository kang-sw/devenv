---
title: Compact floating toolbar for the whiteboard
related:
  261114-feat-ws-dashboard-board-toolbar-overview: 'prior toolbar implementation; preserves Search event handling'
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: db2079dab866d6c5
sage-review-completeness-reviewed: db2079dab866d6c5
---

# Compact floating toolbar for the whiteboard

## Background

Owner decision 652 requests a floating toolbar at approximately 60% of its nonfloating overall height, not a 60% reduction. MAIN relayed approval of the final visual at 08:01 on 2026-11-14 and continued capture authority at 08:39. This dedicated ticket captures that approved intent for promotion and implementation through the normal review and run flow. Runtime activation remains separately gated.

The approved visual and its full report accompany this ticket in the following reference assets:

- [Approved standalone HTML](../assets/261114-feat-ws-whiteboard-compact-floating-toolbar/approved.html).
- [Approved fragment HTML](../assets/261114-feat-ws-whiteboard-compact-floating-toolbar/approved.fragment.html).
- [Approved visual report, with an English translation of the owner quotation](../assets/261114-feat-ws-whiteboard-compact-floating-toolbar/approved-report.md).

The HTML copies retain the original human-facing UI and demonstration disclaimers. Those disclaimers describe mockup scope, not a denial of MAIN's later visual approval. The report remains mockup evidence rather than production acceptance.

## Evidence and provenance

**VERIFIED:** The complete approved report was read during capture. It records 24 passing mockup combinations across standalone/fragment artifacts, browser widths of 1440, 800, 360 and 320 pixels, and wide/phone/small previews. Its measurements are 46 to 28 pixels (60.9%) on desktop, 76 to 48 pixels (63.2%) with normal wrapping, and 82 to 60 pixels (73.2%) for the directly opened 320-pixel fragment. These are reported prior measurements, not a new test run. See the linked report's “Measured results” and “Scope and limitations” (`ai-docs/tickets/assets/261114-feat-ws-whiteboard-compact-floating-toolbar/approved-report.md#L21-L38`, `ai-docs/tickets/assets/261114-feat-ws-whiteboard-compact-floating-toolbar/approved-report.md#L65-L67`).

**VERIFIED:** Local source builds the toolbar in `agents-plugin-tool/internal/wsdashboard/assets/cards.js`, searchable by `function buildToolbar()`. Overview precedes terminal and web slots, Search, conditional Files, and counts. The toolbar is infrastructure rather than a card or storage record (`cards.js:92-130`). Existing availability conditions remain part of the original actions; this ticket does not require unavailable actions to be invented.

**VERIFIED:** The production base styles use 30-pixel buttons, 14-pixel icons, 7-pixel block padding, and a 540-pixel container breakpoint for responsive labels (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css:765-778`). Production shortcut/count typography and terminal popover geometry differ from the mockup (`dashboard.css:771-782`); the mockup does not authorize replacing them wholesale.

**VERIFIED:** Production wires existing toolbar actions and overview through `WsCards.mount` and `mountBoardOverview` (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:730-781`). Its `boardKeep`/`boardKept` code explicitly handles board-column and document scrollers, late layout changes and maximized-card exclusions (`dashboard.js:1245-1301`). Existing toolbar tests describe placement, action availability and popover bounds (`agents-plugin-tool/internal/wsdashboard/board_toolbar_test.go`, searchable by `TestToolbarPopoverFitsViewport` and `TestToolbarPageActionHooks`). These files were inspected read-only; no tests were run.

**INFERRED:** Production integration needs verification beyond the standalone visual because it has real action slots, scroll-keep, overview, modal and frame lifecycle behavior. The inspected source supplies integration boundaries, not proof that the approved mockup already satisfies them or a choice of new production mechanism.

## Decisions

- The floating face should remain approximately 60% of the original height where preserved layout permits. A 60%-shorter toolbar and the earlier, too-subtle 38-pixel/36-pixel floating treatments are not the approved target.
- Preserve the five original actions and order: Overview, New terminal, Web page, Search, and Files. Preserve counts, original availability, behavior, horizontal button boxes, column gaps, and responsive label/shortcut visibility.
- Preserve readable label and shortcut font sizes. The approved visual reduces visible buttons from 30 to 24 pixels, icons uniformly from 14 to 12 pixels, block padding from 7 to 1 pixel, and the floating counts line-height to 14 pixels. Glyphs must not be vertically distorted.
- Preserve 30-pixel hit regions, including the approved one-pixel extension above and five-pixel extension below each visible 24-pixel button. These are not 44-pixel targets. Hit regions must remain viewport-visible, disjoint and operable at their edges.
- Preserve the same mounted toolbar and buttons and the reserved original toolbar slot, including wrapped rows. Do not substitute duplicate toolbars, reparent existing actions, or animate geometry. Card restyling is outside this ticket.
- Preserve the disclosed narrow limitation. In the 320-pixel standalone fragment, body margins leave a 266-pixel face and Files wraps onto another action-bearing row. The approved result is 73.2%, not 60%. An 8-pixel gap between action-bearing rows avoids fractional-pixel hit sharing; counts-only wrapping retains the original gap. Readability, original horizontal layout and safe hit regions take priority over forcing every narrow case to 60%.
- Compatibility tier is T0: this is dashboard asset content, with no approved protocol, API, tracker event or persisted-store change.

## Constraints

This ticket leaves frozen P4, blocked owner item 649, the separately authorized owner 659 observation, rail ownership and unrelated drafts unchanged. This ticket does not choose an unconfirmed product mechanism. MAIN owns new questions.

Before eventual product edits, read the applicable repository manuals: `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` for `agents-plugin-tool/internal/` work, plus any additional path-matched manuals required by `AGENTS.md` for the eventual edit scope.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261114-feat-ws-dashboard-board-toolbar-overview (2026-11-14, commit 420891f1): "stopPropagation on the search callback is preferred over setTimeout (no async gap)" — bearing: constrains
- 541fe405 (2026-11-14, commit): "Reuse the private shared maximize lifecycle for the existing explorer node rather than duplicate/remount the browser or add a public export/module/API." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED: The ticket names agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard.css, assets/dashboard.js and board_toolbar_test.go, plus the three approved reference assets. |
| scope.surface | internal | VERIFIED: Phase 1 changes dashboard presentation and preserves existing action semantics; buildToolbar is private in agents-plugin-tool/internal/wsdashboard/assets/cards.js#L94-L132. No external interface change is approved. |
| scope.new_public_symbol | no | VERIFIED: Phase 1 requests no new public symbol and preserves the existing mounted toolbar and buttons. |
| scope.new_type_contract | no | VERIFIED: The Decisions exclude protocol, API, tracker event and persisted-store changes; Phase 1 requests no new signature or type. |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wsdashboard/board_toolbar_test.go contains TestBoardToolbarPlacementAndCounts, TestToolbarTerminalAvailability, TestToolbarPopoverFitsViewport and TestToolbarPageActionHooks. No new test file is specified. |
| complexity.reuse_points | confirmed | VERIFIED: buildToolbar and fixedNode were read in agents-plugin-tool/internal/wsdashboard/assets/cards.js#L92-L132; mountBoardOverview and boardKeep/boardKept were read in assets/dashboard.js#L768-L783 and #L1279-L1293. These are existing integration boundaries, not a selected floating mechanism. |
| complexity.side_effect_risk | high | INFERRED: Changing the toolbar footprint can affect scroll compensation, wrapped hit ownership, focus and popover containment across the Phase 1 production cases. |
| risk.correctness | high | INFERRED: Reserved wrapped heights and floating transitions must preserve reading position and target arrival while production frames change height; assets/dashboard.js#L1245-L1301 already coordinates late-layout compensation. |
| risk.fit | moderate | INFERRED: Phase 1 preserves existing DOM and action ownership, but its floating mechanism remains to be grounded; the reference's scoped demo CSS is not production CSS. |
| risk.test | high | INFERRED: Existing toolbar tests cover placement, hooks and popover bounds, while ai-docs/tickets/assets/261114-feat-ws-whiteboard-compact-floating-toolbar/approved-report.md#L65-L67 explicitly excludes the production integration cases required by Phase 1. |
| risk.security_or_contract | moderate | INFERRED: No protocol or store change is approved, but Phase 1 must preserve action availability, keyboard focus and disjoint operable targets; cards.js#L110-L130 and board_toolbar_test.go constrain those existing actions. |

## Phases

### Phase 1: Integrate and verify the approved compact floating toolbar

Implement the approved floating treatment while retaining the original nonfloating appearance, mounted DOM identity, reserved slot and existing action semantics. Use the tracked HTML and report as the visual reference, not as production code to import wholesale. The implementation mechanism remains to be grounded in production source before execution.

Acceptance requires side-by-side captures of production and the tracked approved reference at desktop, normal-wrap and narrow widths, including the disclosed 320-pixel fragment limitation. Record measured nonfloating/floating heights, readable labels and uniform icons, unchanged horizontal widths/positions and responsive visibility, counts, no horizontal overflow or clipped labels, and return-to-top restoration. Explain any production/reference differences rather than treating the mockup's numbers as a production pass.

Verify production interactions with all available original actions: full forward/reverse keyboard order, visible focus, focus retained across floating transitions, Enter activation, pointer ownership throughout every 30-pixel hit region, and disjoint wrapped-row targets. Check the real terminal/profile menu's placement, viewport containment, Escape handling and opener-focus restoration. Preserve existing production menu behavior; the mockup's five-pixel border gap and nine-pixel inset are mockup measurements, not new production anchor constants.

Verify production scroll-keep and navigation arrival in the board scroller and document-scroller fallback, including transitions into/out of floating state and wrapped heights. Confirm the reserved slot prevents content jumps and target headings/controls remain reachable below the face. Check modal/maximized-card and Files interactions, overview entry/exit, iframe identity/lifecycle, live updates and late frame-height changes without regressions to reading position or existing behavior. The report explicitly did not validate these production cases, so separate production evidence is required.

Run the project's applicable tests and production interaction checks, and record the production evidence against these acceptance boundaries.
