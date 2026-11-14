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

## Blocked — 2026-11-14 — Worker Protocol (f)

Phase 1 is incomplete and has no Result. Repeated browser failures with the same Web-popover containment root cause triggered stop (f). MAIN confirmed the stop: preserve the implementation and assertions; do not retry, weaken checks, expand scope or merge. This ticket is returned to `todo/` so it is not an advanceable ready-queue item. MAIN owns disposition before any new implementation dispatch or ready promotion.

### Retained candidate and attribution

- **VERIFIED:** Test commit `1b670c9824e4d672510f83f0479ad7c4ba2651ea` and incomplete production checkpoint `5c00e800c7ea17801a9d61cb3b649cae14b9c787` are retained on `impl/docs/compact-floating-toolbar-652/shy-ovary-tarot`, based on `395242d80815f01d4aad5661547b42676f021ff5`. The five production files are `wsdashboard/assets/{cards.js,dashboard.css,overview.js,terminal.js}` and `wswhiteboard/assets/rail.js` under `agents-plugin-tool/internal/`. The original toolbar root reserves its wrapped slot around one mounted face; no duplicate actions or floating scroll writer were introduced. This checkpoint is not acceptance.
- **VERIFIED:** The failing assertion is `scripts/board-headless.mjs:1370`, “Web Enter opens the real viewport-fitting URL/path popover.” Current production failed at adapter width 800 and width 320 with a constrained 266px toolbar face. The 800px menu ends at x=853.71875; the 320px menu ends at x=380.125. See the exact logs and commands below.
- **VERIFIED:** Browser-side substitution of all five production assets from `395242d80` reproduces the same 800px menu bounds in the nonfloating baseline. This establishes that this containment defect predates the candidate. The diagnostic records bounds and exits successfully; it is not a passing baseline assertion suite. No 320px baseline assertion run was made, and baseline reproduction does not establish absence of other regressions (`/tmp/compact-toolbar-evidence/web-baseline.cjs`, `web-baseline.json`).
- **INFERRED:** The coordinate-space mismatch is the root cause: `wsdashboard/assets/webcard.js:421-437` computes viewport coordinates and writes them to an absolutely positioned `.wpop` inside the relative toolbar slot. `webcard.js` was not changed. No production Web fix was attempted or authorized here.

### Exact stopped-run commands and evidence

The following commands are historical evidence, not instructions to retry. Run location was the repository root unless stated otherwise; the now-stopped fixture URL is intentionally retained verbatim.

```sh
# Current production: both commands failed the retained Web assertion.
PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright node scripts/board-headless.mjs toolbar --url http://127.0.0.1:59668/s/theftawaretackinessgong --width 800 --capture-dir /tmp/compact-toolbar-evidence/adapter-800
PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright node scripts/board-headless.mjs toolbar --url http://127.0.0.1:59668/s/theftawaretackinessgong --width 320 --face-width 266 --capture-dir /tmp/compact-toolbar-evidence/adapter-320-266
# Baseline/current bounds diagnostic: recorded the pre-existing 800px overflow; no assertion verdict.
PLAYWRIGHT_MODULE=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright node /tmp/compact-toolbar-evidence/web-baseline.cjs
# From agents-plugin-tool/: failed before the final test-selector correction; not rerun.
GOPROXY=off go test ./internal/wsdashboard ./internal/wswhiteboard -count=1 -timeout=10m
```

**VERIFIED:** Full outputs were read at:

- `/tmp/compact-toolbar-evidence/adapter-800.log`
- `/tmp/compact-toolbar-evidence/adapter-320-266.log`
- `/tmp/compact-toolbar-evidence/web-baseline.log`
- `/tmp/compact-toolbar-evidence/web-baseline.json`
- `/tmp/compact-toolbar-evidence/go-full.log`
- `/tmp/compact-toolbar-evidence/adapter-1900.log`
- `/tmp/compact-toolbar-evidence/board-1440.log`
- `/tmp/compact-toolbar-evidence/keep-board.log`
- `/tmp/compact-toolbar-evidence/keep-adapter.log`

Baseline/current screenshots are `/tmp/compact-toolbar-evidence/web-baseline-top.png`, `/tmp/compact-toolbar-evidence/web-baseline-floating.png`, `/tmp/compact-toolbar-evidence/web-current-top.png` and `/tmp/compact-toolbar-evidence/web-current-floating.png`. The baseline “floating” filename names the requested probe position, not a floating baseline implementation.

Production measurements and captures are retained at:

- `/tmp/compact-toolbar-evidence/adapter-1900/toolbar-1900-adapter-compact-result.json` and `/tmp/compact-toolbar-evidence/adapter-1900/toolbar-1900-adapter-compact-result.png`
- `/tmp/compact-toolbar-evidence/adapter-800/toolbar-800-adapter-compact-result.json` and `/tmp/compact-toolbar-evidence/adapter-800/toolbar-800-adapter-compact-result.png`
- `/tmp/compact-toolbar-evidence/adapter-320-266/toolbar-320-adapter-compact-result.json` and `/tmp/compact-toolbar-evidence/adapter-320-266/toolbar-320-adapter-compact-result.png`
- `/tmp/compact-toolbar-evidence/board-1440/toolbar-1440-board-compact-result.json` and `/tmp/compact-toolbar-evidence/board-1440/toolbar-1440-board-compact-result.png`

These files are local stopped-run evidence, not tracked acceptance assets. The independent reference comparison has not been performed.

### Partial verification and cleanup

- **VERIFIED:** Complete toolbar scenarios passed for adapter 1900px (7.4s) and board-only 1440px (4.4s). Their complete outputs above cover keyboard traversal, hit samples, terminal menu, available actions, Overview reading anchors, native arrival, return-to-top and frame identity. The synthetic browser-local filesystem/terminal responses and injected production WebSocket packets are not host-filesystem, real PTY or end-to-end publication acceptance.
- **VERIFIED:** Adapter 1900px measured 46→28px with the original 46px slot retained; adapter 320px/face266 measured 82→60px with the original 82px slot retained. The latter command still failed Web containment. Neither measurement is a production/reference visual match claim.
- **VERIFIED:** Board-only scroll-keep passed (3.7s, within 1px through swaps and a late frame height). Adapter keep failed its fixture precondition, “need two framed topics above for the late-height probe”; its behavior contract did not execute.
- **VERIFIED:** The full Go run failed at `terminal_page_test.go:111`, which expected the old container on `.board-toolbar`. The container now belongs to `.board-toolbar-face`; the authorized assertion correction is committed, but no full rerun occurred. The whiteboard package passed in that run (20.037s); the dashboard package did not pass (145.318s). See `go-full.log`.
- **VERIFIED:** The test delegate stopped isolated supervisor PID 14316 and daemon PID 15913. Subsequent `ps -p 14316,15913 -o pid=,ppid=,stat=,command=` returned no rows. `/tmp/compact-toolbar-evidence/serve.log` records shutdown and preservation of `/var/folders/qz/mxbhy9fs5zq_ymx5vr1_ztv80000gn/T/board-headless-6Z0D12`. No installed/shared daemon or runtime activation was changed.
- **Omitted due to stop:** Final full Go rerun; complete width/page matrix including normal counts wrapping; history-target arrival and complete both-scroller late-height/transition acceptance; side-by-side comparison against `approved.html` and `approved.fragment.html`; independent lite review. No phase Result, ticket closure, integration, push or production acceptance is claimed.

## Continuation authority — 2026-11-14, 10:59

The preceding Blocked section is retained historical evidence, not the current execution disposition. Owner-reassigned Claude MAIN `id:sweatradartakingoverwrite` explicitly assigned EXEC1 to finish this ticket end to end on the same branch from `c2c79715`, superseding the prior stop-(f) HOLD for this bounded continuation. The lead relayed this authority directly; it is not an automatic retry or a new product-design decision.

- Preserve the T0 tier, original action/layout contract and full assertions. Establish equivalent `395242d80` baseline evidence at 320px as well as 800px. A proven equivalent baseline containment failure is a known baseline finding, not an acceptance blocker; capture it in an idea follow-up. Baseline equivalence does not establish absence of other regressions. Candidate-only failures require minimal fixes and verification.
- Complete the applicable Go/browser matrix, normal wrapping, both-scroller late-height/transitions, history-target arrival, approved-reference comparison and independent lite review. Add real source-rendered before/after captures at 1280, 800 and 320px showing a Korean `응답 N` reply pill, including top/floating overlap measurements. Mockups alone are not production acceptance.
- MAIN explicitly approves source merge after clean independent lite review and no regressions; report `merge_confirm: skip`. The lead owns merge/release. Do not change the primary checkout, push, or rebuild/swap any shared/installed daemon. Source fixture processes remain isolated. New architecture, API, deletion, T2 or irreversible work remains outside this authority.

## Phases

### Phase 1: Integrate and verify the approved compact floating toolbar

Implement the approved floating treatment while retaining the original nonfloating appearance, mounted DOM identity, reserved slot and existing action semantics. Use the tracked HTML and report as the visual reference, not as production code to import wholesale. The implementation mechanism remains to be grounded in production source before execution.

Acceptance requires side-by-side captures of production and the tracked approved reference at desktop, normal-wrap and narrow widths, including the disclosed 320-pixel fragment limitation. Record measured nonfloating/floating heights, readable labels and uniform icons, unchanged horizontal widths/positions and responsive visibility, counts, no horizontal overflow or clipped labels, and return-to-top restoration. Explain any production/reference differences rather than treating the mockup's numbers as a production pass.

Verify production interactions with all available original actions: full forward/reverse keyboard order, visible focus, focus retained across floating transitions, Enter activation, pointer ownership throughout every 30-pixel hit region, and disjoint wrapped-row targets. Check the real terminal/profile menu's placement, viewport containment, Escape handling and opener-focus restoration. Preserve existing production menu behavior; the mockup's five-pixel border gap and nine-pixel inset are mockup measurements, not new production anchor constants.

Verify production scroll-keep and navigation arrival in the board scroller and document-scroller fallback, including transitions into/out of floating state and wrapped heights. Confirm the reserved slot prevents content jumps and target headings/controls remain reachable below the face. Check modal/maximized-card and Files interactions, overview entry/exit, iframe identity/lifecycle, live updates and late frame-height changes without regressions to reading position or existing behavior. The report explicitly did not validate these production cases, so separate production evidence is required.

Run the project's applicable tests and production interaction checks, and record the production evidence against these acceptance boundaries.
