---
title: Whiteboard toolbar state filter with preserved closed-topic counts
related:
  261114-feat-ws-whiteboard-topic-state-and-lead-nudges: prerequisite; completed state, owner, ETA and nudge model
  261114-refactor-ws-dashboard-whiteboard-asset-split: parallel pinned verification; do not import its unmerged source layout
blocked-by: 261114-feat-ws-whiteboard-topic-state-and-lead-nudges
sage-review-design: completed
sage-review: required
sage-review-completeness: completed
sage-review-completeness-reviewed: b81f7d56a8d82743
sage-review-design-reviewed: b81f7d56a8d82743
completed: 2026-11-15
---

# Whiteboard toolbar state filter with preserved closed-topic counts

## Background

The topic-state model and overdue-nudge scheduler have landed. The display follow-on makes those existing states useful as a compact toolbar filter. The owner normally uses approximately 800px-wide cards; duplicating topic counts at the right of the toolbar wastes that space.

Authority: MAIN relayed owner decision (839)=a on 2026-11-15 at 00:43:43 UTC and explicitly authorized this follow-on capture, design review, readiness and a Pi worker at 00:52:49 UTC. The completed prerequisite's three phases remain frozen; this ticket does not reopen their implementation or claim that the display already shipped with Phase 3.

## Decisions

- Use V1, the inline segmented radio filter, on the existing whiteboard toolbar. Rejected: V2 as the ordinary-width default and V3 as the selected design. Use V2's single-pill/radio-popover presentation below 540px instead.
- Remove the separate right-side `진행 중 N / 종료 M` text. Move count information into the filter rather than retaining duplicate toolbar text.
- Include an independent `종료 N` segment for the existing closed-topic lifecycle. Its count includes loaded `.closed` topics plus the existing `data-closed-more` count. `wb:close` remains the lifecycle operation; closed is not a new `wb:state` value. The V1 reference omitted this segment and instead displayed a separate fixed `종료 61`; the owner's explicit correction supersedes that part of the reference.
- Preserve the existing unfiltered board as the meaning of the new `전체` option; production has no existing state-filter/`전체` toolbar option (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L94-L133`, `#L185-L198`). Stateless topics appear only under `전체` rather than being inferred into an operational state. Closed-topic eligibility follows the existing lifecycle, including historical closed topics without state metadata; do not discard them from the closed count because their state is absent.
- Persist the selected filter and use the list-filter icon. Page data carries the existing topic state, owner and ETA so the UI consumes authoritative metadata instead of guessing from conversation text, badges or process activity.
- At approximately 800px card width with Korean labels, the toolbar occupies one row. Keep the V1 reference's compact dot/count treatment for unselected segments where needed. Below 540px, use V2 with the same selection and counts rather than wrapping or losing options.

## Approved Visual Reference

`ai-docs/tickets/assets/261115-feat-ws-whiteboard-toolbar-state-filter/834b-state-filter-toolbar.html`

The reference is committed with this ticket before handoff. Preservation verified source/copy byte equality (`cmp` exit 0) and matching SHA-256 `ca9cf8fb239343f6a7e7993962671aa89c3ce30aeb612bf5a672e22e0f36019c` on 2026-11-15. Its original `shared/mockups/834b-state-filter-toolbar.html` is a local design artifact rather than a source file at the captured HEAD.

This is a byte-preserved copy of `shared/mockups/834b-state-filter-toolbar.html`. V1 defines the selected inline-radio look, V2 defines the narrow presentation, and the reference includes light and dark variants. The decisions above override its duplicated right-side count, missing closed segment and demonstration widths/data. Preserve the original reference rather than rewriting it to look like an implemented result.

Verification must compare implementation captures side by side with the relevant reference variants and explain those explicitly approved differences. A source-level resemblance or passing selector test is not a visual comparison.

## Constraints

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)
- This is the display follow-on only. Reuse the landed topic-state/owner/ETA model and lifecycle; do not redesign the scheduler, nudge accounting, state protocol, closed lifecycle or history store.
- Compatibility: T0 for additive page-data metadata, if needed; no renamed/removed existing fields, new closed state enum or persistent-store format change. A required non-additive change stops for MAIN disposition.
- Preserve existing toolbar actions, closed-history loading, board ordering, selection, overview and scrolling behavior except for the requested visibility filter. Filtering is not deletion or mutation of topic lifecycle.
- Keep filter selection coherent across responsive presentation changes and ordinary board refreshes. Reuse existing persistence and UI accessibility conventions rather than introducing a separate general configuration system.
- Existing implementation-convention manuals apply to touched paths: `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md`, and `ai-docs/manuals/ws-mcp.md` when MCP paths are touched.
- Verification is offline and isolated, using cached dependencies and owned fixtures. Never use the shared daemon or port 29417; no provider calls, downloads, owner installation or shared-daemon build/restart/swap/smoke. MAIN owns runtime activation and develop integration. No outer timeout/kill around Go tests; use Go's own test timeout and preserve actual exit receipts.
- Asset-split ticket 261114-refactor-ws-dashboard-whiteboard-asset-split remains on its separate pinned verification branch. Do not move its pins, alter its cache supervisor/receipts or import its unmerged extraction for this display work. Adapt to the actual landed source layout.

## Prior Decisions

- 261114-feat-ws-whiteboard-topic-state-and-lead-nudges (2026-11-15, Result): "No board display, badge, ordering, grouping or toolbar-filter work was authored; owner decision 839 remains outside this Phase 3 assignment." — bearing: constrains
- 261114-feat-ws-dashboard-board-toolbar-overview (2026-11-14, Decisions): "N counts the open topics on the board and in the pin list. M is the closed topics shown plus the show-more remainder" — bearing: constrains
- 261114-feat-ws-whiteboard-compact-floating-toolbar (2026-11-14, Decisions): "Preserve the five original actions and order: Overview, New terminal, Web page, Search, and Files." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED at HEAD 1612e099c: agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/dashboard.css, assets/dashboard.js; agents-plugin-tool/internal/wswhiteboard/view.go and assets/page.tmpl; linked reference asset and existing focused tests. No asset-split extraction is present in these HEAD paths. |
| scope.surface | cross-module | VERIFIED: dashboard toolbar consumes whiteboard topic sections (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L185-L198); authoritative state currently reaches query output, not TopicView/template (agents-plugin-tool/internal/wswhiteboard/query_entries.go#L364-L365; view.go#L137-L223; assets/page.tmpl#L46). |
| scope.new_public_symbol | unknown | No new MCP command, route or state enum is requested; additive page metadata names and any exported Go projection member are not specified. |
| scope.new_type_contract | yes | INFERRED: Phase 1 requires an additive page-data state/owner/ETA projection; existing TopicState/TopicRun already define authoritative values (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L44). Exact page carrier is an implementation detail, not a new state model. |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wsdashboard/board_toolbar_test.go#L10-L65 covers pins, counts and history refresh; closed_route_test.go#L59-L84 covers closed history; scripts/board-headless.mjs is the existing browser harness. New filenames are not required by the phase. |
| complexity.reuse_points | confirmed | VERIFIED: read buildToolbar/countTopics/floatToolbar in agents-plugin-tool/internal/wsdashboard/assets/cards.js#L94-L198; guarded load/save in assets/dashboard.js#L27-L33; TopicStates in agents-plugin-tool/internal/wswhiteboard/topic_state.go#L94-L120; query projection in query_entries.go#L364-L365. |
| complexity.side_effect_risk | high | INFERRED: visibility filtering changes geometry around the existing floating toolbar, pinned topics, overview, frames and history refresh; floatToolbar still directly measures the count node (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L136-L178). |
| risk.correctness | high | VERIFIED boundary: countTopics deduplicates board/pin topics and adds data-closed-more independently of state (agents-plugin-tool/internal/wsdashboard/assets/cards.js#L185-L198). Closed eligibility is lifecycle-based even without state (agents-plugin-tool/internal/wswhiteboard/view.go#L703-L728); close resets state (topic_state.go#L118-L120, #L355-L359). INFERRED: preserving these across filtering/refresh is integration-sensitive. |
| risk.fit | moderate | VERIFIED: no existing state selector or list-filter icon matched git grep -n -E 'list-filter|state-filter|stateFilter' HEAD -- agents-plugin-tool/internal/wsdashboard/assets. INFERRED: add the requested presentation within existing toolbar/storage ownership, not a new workflow enum or imported unmerged asset layout. |
| risk.test | high | INFERRED: current count assertions do not establish state filtering, responsive radios or visual fidelity. The supplied asset has V1/V2 and both themes but fixed 1280/900px demonstrations (ai-docs/tickets/assets/261115-feat-ws-whiteboard-toolbar-state-filter/834b-state-filter-toolbar.html#L225-L236); 800px and under-540px acceptance and side-by-side comparisons remain required, not executed. |
| risk.security_or_contract | moderate | INFERRED: additive display metadata must preserve absent/diagnostic authority and escaped rendering; no persistent-store or state grammar change is approved. VERIFIED: only four operational enums exist, with owner/ETA on the optional run (agents-plugin-tool/internal/wswhiteboard/topic_state.go#L18-L44); missing metadata is not a closed enum or inferred default (#L102-L120). |

## Phases

### Phase 1: Ship the responsive state/lifecycle filter

Implement the approved toolbar presentation and wire it to the existing board topic metadata. Add the separate closed-lifecycle segment, remove duplicated right-side counts, persist selection, include the list-filter icon, and supply authoritative state/owner/ETA in page data where not already available. Keep the selected filter and its counts synchronized through topic refresh, state transitions and existing close/history-load operations.

Verification:

- Cover each existing operational state, mixed stateful/stateless topics, `전체`, and closed lifecycle independently. Include historical closed topics without state metadata, loaded closed topics plus `data-closed-more`, zero-count and multiple-count cases, and refresh/close/history-load count updates. An operational-state filter must not silently classify stateless topics.
- Verify page-data round trips for state/owner/ETA, including absent legacy metadata without invention, and persistence across reload/refresh. Exercise the same selection across the V1/V2 boundary using established keyboard/focus/radio behavior.
- Use isolated browser captures and geometry assertions at 800px card width with Korean labels and representative counts: one toolbar row, no horizontal clipping or overlap, existing actions usable. Cover both themes and selected-label variants, including `종료 N`.
- Verify the narrow V2 fallback immediately below 540px and V1 at/above the boundary, including a smaller mobile width. Compare result captures side by side with the preserved V1/V2 reference and record approved differences rather than claiming the old mockup already meets the corrected contract.
- Run relevant focused Go/JavaScript regressions plus existing affected toolbar, closed-history, overview and source-assembly contracts without weakening assertions. Preserve exact commands, complete logs, actual exit codes and any explicit omissions. Record an owned-process census and fixture cleanup outcome; a started process or controller exit is not acceptance proof.
- Obtain independent implementation review, record the actual result and retained evidence, then hand the branch to MAIN. Do not activate the shared runtime or claim installed/live acceptance.

### Result (01a121f70) - 2026-11-15

Implemented and independently reviewed on `impl/develop/ankle-blush-ashen`.
Code/test head: `01a121f707d52fe316b1b0ff2b31404b13d986be`. MAIN integrated
777 (`1b815bf673d98c32877d729896afcd23808dc25a`) through `f5c57b1ef`, then
the approved docs-equivalent rail sync through `17014f672ef2b3131b9e5fc2660b9f5a9e830512`.
No asset-split source, rail fixture fix, shared-runtime activation or parent merge
was performed by this worker.

Delivered:

- Authoritative optional `TopicView.State`/`StateDiagnostic` and escaped topic
  data attributes for state, run owner, cumulative ETA, start/deadline and run ID.
  Legacy absence and diagnostic authority remain distinct; closed reset is
  unchanged. This is T0 page metadata/assets, with no state/store/protocol change.
- Six persistent radios: All, the four existing operational states, and the
  independent closed lifecycle. Counts deduplicate board/pin topics and retain
  loaded closed history plus `data-closed-more`. Visibility marks preserve topic
  nodes, frames, pin order and lifecycle; stateless topics match only All (or
  Closed when lifecycle-closed). The duplicate right count is gone.
- Inline V1 at actual toolbar width >=540px, V2 trigger/radio-popover below it,
  list-filter icon, complete labels/counts, roving radio keys and responsive focus
  transfer. Eleven locale tables are complete. Overview restores before filter
  changes and excludes filtered topics from keyboard/motion lists.

Implementation decisions:

- All's count is the lifecycle total (loaded open + loaded closed + closed
  remainder), because All restores the existing unfiltered board, not an
  invented open-only state. Counts are independent of current visibility.
- The narrow control remains one accessible trigger with list-filter/chevron;
  choosing All clears it rather than introducing the mockup's inline decorative
  clear subcontrol. The popover retains every option, including Closed.
- Existing capability gates are preserved. Baseline `1b815bf67` readbacks verify
  `dashboard.js:716,767` Files live/fpane gating and `pages.go:1235` plus
  `terminal.js:24-25` terminal `data-term` gating. The isolated no-ring and real
  AppendRing saved-history fixtures exercise their actual modes. A refusing,
  provider-free synthetic TerminalHost advertises terminal capability (four
  inherited controls there); a separately labeled synthetic live snapshot covers
  all five controls. No terminal shell or provider action was executed.

Verification and exact receipts:

- `GOPROXY=off GOSUMDB=off go test ./internal/wswhiteboard -timeout 120s`:
  exit 0; `/tmp/whiteboard-page-data-legwarmer/go-test.{log,exit}`. The initial
  failure used an additive instead of cumulative ETA fixture; corrected fixture
  provenance remains in `go-test-first.{log,exit}`.
- `GOPROXY=off GOSUMDB=off go test ./internal/wsdashboard -run '^TestBoardStateFilter$' -count=1 -timeout 120s`:
  exit 0; `/tmp/toolbar-state-ankle/filter-unit-rerun.{log,exit}`. The first
  minimal-DOM test used unsupported unquoted selector syntax; its separate
  `filter-unit.{log,exit}` is retained.
- `GOPROXY=off GOSUMDB=off go test ./internal/wsdashboard ./internal/wswhiteboard ./internal/wsi18n -timeout 180s`:
  exit 1, solely pre-existing 777 `TestRailDraftRestore` undefined `mode` in an
  extracted fixture. Whiteboard/i18n passed. Full raw receipt:
  `/tmp/ws-toolbar-leaf-logs/test-1.{log,exit}`. MAIN explicitly excluded its fix;
  follow-up `261115-bug-ws-rail-draft-restore-undefined-mode` owns it. No
  package-wide clean claim is made.
- `GOPROXY=off GOSUMDB=off go test ./internal/wsdashboard -run 'Test(Board|Toolbar|Overview|Cards)' -timeout 180s`:
  exit 0; `/tmp/ws-toolbar-leaf-logs/focused.{log,exit}`. Original literal
  pin/count/history, floating/scroller, keyboard and motion expectations remain
  meaningful through the replacement segments, not deleted coverage.
- `GOPROXY=off GOSUMDB=off go test ./internal/wsdashboard -run '^(TestTerminalCardScript|TestTerminalButtonLivesInBoardToolbar|TestBoardCardsJoinTheTopicRail|TestCSSClassesDoNotCollideAcrossModules|TestAssetIDFollowsAssetContent|TestPagesAndStreamsCarryTheAssetID|TestClosedRoute)' -count=1 -v -timeout 180s`:
  exit 0; `/tmp/toolbar-state-ankle/final-source-assembly.{log,exit}`.
- Isolated Chromium: exact command in
  `/tmp/ws-board-state-filter-17014f672-LoQOPv/command.txt` (cached Playwright,
  `go test ./internal/wsdashboard -run '^TestBoardStateFilterBrowser$' -count=1 -v -timeout=5m`):
  exit 0; complete `go-test.log`, `go-test.exit`, `browser-process.log` and
  `browser-exit.json` in that directory. Korean light/dark: ordinary, 777 bounded
  saved-history, 135-closed and synthetic-live modes; all six selected labels at
  actual 800px; 540 inline / 539 popover / 360 mobile; keyboard/focus round trip,
  reload, real streamed zero/state/close changes, actual closed route, document
  versus bounded scrolling, floating, non-vacuous Overview traversal and two-pin
  node/frame/order preservation. `measurements.json` and capability JSONs retain
  actual geometry; 1,750 owned-origin requests, no external requests/page errors.
- Browser correction history is explicit in `correction-provenance.json`.
  Earlier directories `/tmp/ws-board-state-filter-17014f672-{GJEj1Y,tmPkN6,br7Iea,s3jhG5,ovH8fo}/`
  retain their raw failures/exit/cleanup receipts: reference relative path,
  overstrict five-action mode assumption, Page/Frame API, hidden initial saved
  board tab, and mistaken fixture text expectation respectively. Later
  `1ysiGh`, `4w4BxW`, and final `LoQOPv` are separate passing strengthened runs;
  none overwrites the failure records.
- Owned cleanup: final `controller-owned-after.json` records Go PID 51656,
  Node 51863 and Chromium 51866 gone; all four owned fixture directories gone;
  `127.0.0.1:62314` and `:62315` listeners closed. `owned-before.json`,
  `owned-after.json` and browser receipts retain the before/after census.

Visual comparison (worker and independent reviewer inspected paired images):

- Compared `/tmp/ws-board-state-filter-17014f672-LoQOPv/filterlive-light-800-discussing-contact.png`
  vs `ai-docs/tickets/assets/261115-feat-ws-whiteboard-toolbar-state-filter/834b-state-filter-toolbar.html#v1-light-900`: match.
- Compared `/tmp/ws-board-state-filter-17014f672-LoQOPv/filterordinary-dark-800-discussing-contact.png`
  vs the same reference `#v1-dark-900`: match.
- Compared `/tmp/ws-board-state-filter-17014f672-LoQOPv/filtersaved-dark-539-open-contact.png`
  vs the same reference `#v2-dark-1280`: match.
- Compared `/tmp/ws-board-state-filter-17014f672-LoQOPv/filterordinary-light-360-pill-contact.png`
  vs the same reference `#v2-light-900`: match.
- The original crops (`reference-v1-{light,dark}-900.png` and
  `reference-v2-{light,dark}-{1280,900}.png`), 120 captures including 32 paired
  contact sheets, and closed-135 selected-label captures are retained beside
  those contacts. Match is for the corrected toolbar treatment, not identical
  demonstration content: the added Closed segment, removed duplicate counts,
  lifecycle-total All, list-filter/chevron control, responsive action labels and
  actual widths/data differ as described above. No topic badges were added;
  the mockup's topic bodies/badges are not this display scope. Reference unchanged.

Independent lite review:

- Fresh reviewer `8b61b485-aed4-4a2f-99b4-94304fbd4c61` reviewed exact
  `9c34183a95c6ecef3eb5930ac8aaf3f0d3ba1df2..01a121f707d52fe316b1b0ff2b31404b13d986be`:
  no Critical, Important or Minor findings. Report retained at
  `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@32b93426/review-paths/8b529efa-01-toolbar-state-filter-lite.md`.
  It directly inspected source/tests, execution receipts, baseline capability
  readbacks, cleanup and relevant V1/V2 contact sheets. Per lite allocation no
  second review ran. The closure commit changes ticket documentation only;
  reviewed source/test inputs remain unchanged.

Unresolved: the explicitly out-of-scope pre-existing rail fixture failure above.
Omitted: shared/installed runtime acceptance, provider/shell execution, unrelated
full-repository suite and redundant full-package rerun; MAIN owns activation
and integration. No source assertion was weakened to hide a production defect.
