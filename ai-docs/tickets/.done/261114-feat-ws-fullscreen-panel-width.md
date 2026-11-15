---
title: Center shared fullscreen panels with an explicit full-width extension
related:
  261114-feat-ws-whiteboard-compact-floating-toolbar: "prerequisite: implement only after its source lands; shared cards.js/dashboard.css surfaces"
  261114-feat-ws-dashboard-shared-card-actions: "existing shared maximize ownership and controls"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: a565125a5d9343aa
sage-review-completeness-reviewed: a565125a5d9343aa
completed: 2026-11-15
---

# Center shared fullscreen panels with an explicit full-width extension

## Background

Card fullscreen toggles currently open nearly viewport-wide sheets. On very
wide displays, the owner wants a centered default width limit and a separate
explicit way to use the available full width. Preserve the existing shared
maximize lifecycle rather than introducing a second kind of fullscreen panel.

Read-only scouting traced the shared implementation to `maximize()` and the
`.bc-max` / `.d-max` / `.d-min` actions in
`agents-plugin-tool/internal/wsdashboard/assets/cards.js`, including the Files
pane's `maximize({ files: true, el: el, back: files })` path
(`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L118-L123`). Sheet geometry lives in
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`; current wide
horizontal insets are 16px and the narrow rules use 8px at viewport widths
of 640px or less (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css#L662-L681`,
`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css#L929-L934`).
Dashboard-framed whiteboard entry panels with nonempty text expose fullscreen
controls; standalone whiteboard pages do not
(`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L63-L63`).
The shared-card action implementation landed in `f227e6d9b` (merge commit
`f227e6d9b1187ad299e5eea61272495630709e1e`).

## Decisions

- **(733)=a — all shared maximize surfaces.** Apply the same width behavior to
  preview, terminal and web cards, board topics, dashboard-framed whiteboard
  entry panels, and the Files pane. Rejected: a card-only subset that leaves
  other shared sheets inconsistent. MAIN relayed the owner's exact answer
  `733,734=a` on 2026-11-14 at 12:34.
- **(734)=a — no remembered extension.** Extended width resets on close,
  minimize and reload. Rejected: session memory or browser-persisted state.
- **(735), refined by (763)=a — centered category defaults.** Normal sheet
  width is `min(category width, 1920px, available width)`, centered. Files and
  Terminal use their current owning-column horizontal span, measured before
  maximizing. Files uses its conversation/grid pane; Terminal uses its board
  or pinned-column host, not a whiteboard entry column. If that span cannot be
  measured, Terminal falls back to **960px** and Files to **1280px**.
  Other defaults are prose/Markdown 1280px, code 1440px, diff 1760px,
  web/image/video 1600px, topic/topic panel 1440px, approval/system 1024px, and
  unknown category 1920px. MAIN relayed the owner's exact answer on
  2026-11-14 at 13:15:29Z:
  `763=a^터미널 960, 탐색기 1280으로 정정.`
  The owner corrected the proposed Terminal 1600px / Files 1920px fallbacks;
  those proposals are not the contract. The explicit extension uses available
  full width and may exceed the normal 1920px cap.
- **(736), announced default — narrow behavior unchanged.** Preserve existing
  narrow/mobile layout, including the existing at-or-below-640px rules
  (`agents-plugin-tool/internal/wsdashboard/assets/dashboard.css#L666-L681`,
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css#L934-L934`). Show the
  separate extension button beside the existing fullscreen controls only on
  wide layouts where extending beyond the effective normal category width
  provides additional width. Do not show an ineffective extension button.
- **(820), MAIN-settled technical decision: category precedence.** Use the
  existing renderer's semantic kind, preserving containing topic/panel identity
  before inner content; unknown categories use the approved 1920px default.
  Reject MIME guessing, which would let incidental nested content override the
  owning surface's approved category. Approved category widths are unchanged.
- **(821), MAIN-settled technical decision: measurement lifetime.** Measure the
  owning-column span once before maximize, then clamp against the current
  viewport on resize. Preserve the at-or-below-640px rules and do not remount
  content. Reject repeated owning-column measurement while maximized, when the
  original layout is no longer the measurement source.
- **(823), MAIN-settled technical decision: integration order.** Start fullscreen
  source work after topic-state Phase 2. If 793's asset split is merged by then,
  use that split head as the baseline; otherwise use current develop. There is
  no dependency on TASK3 checkpoint `ed226dcfb`. Whichever change lands second
  reconciles overlapping files, rather than waiting for unrelated completion.
  MAIN settled (820), (821) and (823) on 2026-11-14 at 23:09:28Z.
  MAIN's 23:43:17Z order note restates (823) for this handoff: start on
  current `develop` unless the 793 split has merged by then; no separate
  topic-state Phase 2 wait applies to EXEC4's start.
- **(822)=a, owner policy: in-sheet toggle.** The same extension button
  switches the open sheet between its category-normal width and the extended
  full width, in both directions, without leaving fullscreen; close, minimize
  and reload still reset to normal. The button exposes its current state
  (pressed/expanded) and its label/title names the action it will take.
  Rejected: (b) one-way extension until minimize/close, which forces leaving
  fullscreen to undo a mis-click. MAIN relayed the owner's answer `818,822=a`
  on 2026-11-14 at 23:43:17Z.
- **Compatibility: T0.** This is a dashboard-asset UI addition. No protocol,
  MCP argument, daemon store, shared configuration or stored-state change is
  part of this ticket. Width extension is transient and not persisted.

## Constraints

- MAIN relayed owner-approved source and review authority for this ticket on
  2026-11-14 at 23:43:17Z; ownership moved from EXEC1 to EXEC4. Per (823),
  the 793 asset split (`261114-refactor-ws-dashboard-whiteboard-asset-split`)
  is still in `ready/` and unmerged, so the baseline is current `develop`;
  whichever change lands second reconciles overlapping files.
  The source-landing prerequisite for
  `261114-feat-ws-whiteboard-compact-floating-toolbar` is satisfied by
  `cacd771c1ab69c01fbc33ffb1db74df0bb6dc3d9`, an ancestor of `develop`. Its
  ticket remains `todo` without a Result; use the landed source, not ticket
  closure, as this prerequisite. `261114-feat-ws-whiteboard-unified-whole-history`
  is not on `develop` (its `ready` copy and source-only checkpoint live at
  `ed226dcfb`, not an ancestor); per (823) there is no TASK3 or
  whole-ticket-completion gate. Its retained constraints still bind any
  overlapping edit: independent panel/version disclosures, stable-pN
  folds/unread badges, and scroll compensation.
- Preserve maximize/minimize/close ownership, focus and Escape behavior,
  panel disclosure state, scroll/anchor compensation, sticky headers and the
  Files pane lifecycle. Existing Overview exclusion while a sheet is maximized
  remains intact. Extension changes width, not maximize ownership.
- No additional fullscreen controls are added to standalone whiteboard pages.
  No remembered preference, new settings surface, daemon restart or deployment
  belongs to this change.
- Before implementation under `agents-plugin-tool/`, read
  `ai-docs/manuals/shipped-surface-boundary.md` and, for its internal paths,
  `ai-docs/manuals/release-compatibility.md` as declared by the project.
  Shipped UI behavior must remain downstream-neutral.

- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 01088661 (2026-11-14, commit): "Shared, not separate: the sheet lives in cards.js maximize() (one scrim, one Escape handler, one `maxed`)" — bearing: constrains
- 541fe405 (2026-11-14, commit): "Reuse the private shared maximize lifecycle for the existing explorer node rather than duplicate/remount the browser or add a public export/module/API." — bearing: constrains
- 261114-feat-ws-dashboard-shared-card-actions (2026-11-14, commit): "Adopt the server-rendered topic header/body/badges in place: shared fold and raise operate on its view record, never invoke topic workflow close, and leave inner disclosure owners unchanged." — bearing: constrains
- 261114-feat-ws-whiteboard-compact-floating-toolbar (2026-11-14, commit): "Reuse the existing rail host inset and native scroll-padding for clearance." — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-10, commit): "Escape cannot leave maximize while focus is inside a sandboxed frame (the page sees no key event from it); the minimize button and the scrim remain." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, Decisions): "Interleaved layout (69=b): panels and log lines occupy one time-ordered column below content. Topic cards use full horizontal width, with no topic side rail." — bearing: constrains
- 0bb0f9d3 (2026-11-14, commit): "A local overlay z-index cannot escape the session iframe's stacking context." — bearing: constrains
- 760df43a (2026-11-14, commit): "Whole fold now masks any header disclosure body generically, retaining header state/summary without claiming a whiteboard-owned class." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | VERIFIED: agents-plugin-tool/internal/wsdashboard/assets/cards.js and dashboard.css; framed markup preservation boundary in agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L63-L63. |
| scope.surface | cross-module | VERIFIED: private maximize coordinates registered card kinds, renderer-owned topic/panel DOM and the Files pane; cards.js#L718-L862 and page.tmpl#L63-L63. No MCP/protocol change is requested. |
| scope.new_public_symbol | no | VERIFIED: Phase 1 reuses private shared actions and sheet layout; no new exported symbol is requested. |
| scope.new_type_contract | no | VERIFIED: Decisions specify T0 transient UI state, not a new API signature, schema or stored contract. Category selection still needs grounding, not a new public type. |
| scope.test_surface | existing | VERIFIED: agents-plugin-tool/internal/wsdashboard/cards_test.go#L730-L849 contains TestFilesToolbarSheet and TestBoardSheet; board_toolbar_test.go#L183-L223 covers floating Files clearance. No new test file is specified. |
| complexity.reuse_points | confirmed | VERIFIED: maximize/grow/shrink/follow/sheetClick read at agents-plugin-tool/internal/wsdashboard/assets/cards.js#L718-L862; three sheet CSS owners read at dashboard.css#L662-L681 and #L929-L934. |
| complexity.side_effect_risk | high | INFERRED: Width changes reflow framed content and sticky headers across live replacement, terminal sizing and Files toolbar geometry; cards.js#L753-L843. |
| risk.correctness | high | INFERRED: Measured owning-column widths, category caps, live sheet replacement and transient reset must agree across all three existing sheet paths. |
| risk.fit | high | INFERRED: Category metadata/precedence is not specified for every renderer-owned panel; shared sheet identity and separate kind owners must remain intact. |
| risk.test | high | INFERRED: Existing mocked lifecycle tests do not prove real owning-column measurement, responsive centering or frame/scroll geometry; browser fixture readiness is unresolved in 261115-bug-ws-shared-card-browser-harness-bounded-readiness. |
| risk.security_or_contract | moderate | INFERRED: T0 UI scope still must preserve sandboxed previews, existing Escape consumption and renderer-owned disclosure/identity contracts; cards.js#L845-L862 and agents-plugin-tool/internal/wsdashboard/assets/preview.js#L155-L165. |

## Phases

### Phase 1: Apply category widths and provide explicit wide extension

Implement the confirmed shared width behavior and the adjacent explicit
extension toggle across every surface listed in (733), on current `develop`
per (823). Keep state transient, resetting it whenever the shared sheet
closes/minimizes and on reload. Reuse the shared actions and sheet layout
rather than adding independent per-card fullscreen lifecycles. This is the
complete behavior slice.

Verification must cover centered category defaults and the normal 1920px cap,
Files/Terminal owning-column capture before maximize, and the corrected 960px
Terminal / 1280px Files fallbacks when no usable column measurement exists.
Check board and optional pinned-column Terminal ownership and Files' grid pane.
Verify explicit extension to available full width, including widths above
1920px, and reset after minimize/close, reopen and reload. Check preview,
terminal, web, topic, framed entry-panel and Files ownership paths. Check
narrow/mobile unchanged geometry and absence of an ineffective extension button
when the available width does not exceed the effective category width. Cover
(820)'s semantic-kind precedence, mixed content and unknown fallback; (821)'s
pre-maximize single measurement and viewport resize clamping without remount.
Cover (822): the same button toggles normal -> extended -> normal inside one
open sheet, and its pressed state and label track the current width. Retain
focus/Escape, Overview exclusion, panel disclosure, sticky-header and scroll
behavior. Use focused existing tests (`go test -run`, never the whole
`./internal/wsdashboard` package) and representative browser evidence:
headless screenshots at 1280px and 1900px viewport widths in both light and
dark themes, showing normal and extended sheets, plus one at or below 640px
showing unchanged narrow geometry. Run only isolated fixtures; never touch the
shared daemon or port 29417.

### Result (95b9246c1) - 2026-11-15

- Landed the shared transient category-normal width and two-way extension
  toggle from checkpoint `82f254274`, integrated with develop `9c34183a9`
  by `c52d92a1d`. Semantic owner identity wins over nested content; normal
  width is centered and capped at 1920px. Files and Terminal snapshot their
  owning columns before maximize, with the approved 1280px/960px fallbacks.
  Narrow geometry, the single maximize owner and no-persistence behavior remain.
- **Retry diagnosis: fixture bug, not product bug.** Before editing, the
  retained sequence was run against merged HEAD and separately against assets
  read with `git show 9c34183a9:<path>` (no stash or checkout). Both produced
  exactly the same button box `(342.328125, 236.25, 24, 24)` and center
  `(354.328125, 248.25)` hitting `#bcol`. Button and all ancestors had
  `pointer-events: auto` and `visibility: visible`; button opacity was `.45`,
  ancestor opacity `1`. The missing session rail placed the main grid in the
  340px rail track, reducing `#r-board` and the topic to zero width. Full
  ancestor boxes/styles and before/after hit-test stacks are in
  `/tmp/exec4/fs-shots/diagnostic-{merged,develop}.json`.
- Corrected only the browser fixture on retry: mirror the real rail/main/
  sibling `.bpcol` structure; drive maximize, minimize, pin and width controls
  with Playwright locator clicks, never forced clicks or synthetic dispatch.
  The corrected topic center receives the pointer, recorded in
  `/tmp/exec4/fs-shots/topic-click-hit-test.json`. A distinct resize-readiness
  assertion initially raced event delivery; the diagnostic measured the
  correct 738.34375px width after delivery. Geometry now waits on the exact
  observable width/centering contract rather than sleeping or weakening it.
  No production fix or product-scope deviation was necessary on retry.
- Browser acceptance passed: 34 geometry observations plus category,
  Files/Terminal column, optional pin column, resize, two-way toggle, focus,
  Escape, Overview handoff, frame identity, disclosure, sticky header,
  scroll preservation, reopen and reload checks. Evidence:
  `/tmp/exec4/fs-shots/browser-acceptance.log` and `browser-results.json`.
- Captured and inspected all nine required images under
  `/tmp/exec4/fs-shots/`: `{1280,1900}-{light,dark}-{normal,extended}.png`
  and `640-dark-normal.png`. Normal and extended widths visibly differ;
  640px retains 8px horizontal insets and no extension control. No approved
  visual reference is linked by this ticket, so no reference comparison applies.
- Focused offline verification passed with `GOPROXY=off GOSUMDB=off
  GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2`:
  `go -C agents-plugin-tool test ./internal/wsdashboard ./internal/wswhiteboard
  -run '^(TestFullscreenPanelWidth|TestBoardSheet|TestFilesToolbarSheet|TestBoardCard|TestPreview|TestPanelMaximizeOnlyOnDashboardBoard)'
  -count=1 -timeout=20s` — dashboard 1.539s, whiteboard 0.675s. The latter
  includes standalone/embed maximize-control exclusion. Full output:
  `/tmp/exec4/fs-shots/go-focused.log`. `git diff --check` passed.
- Every diagnostic/browser/test run ended with a `ps` scan of its observed
  owned process tree; all `*-process-scan.json` files report no remaining
  processes. Used only isolated ephemeral-port fixtures and cached Chromium;
  no shared daemon/29417, downloads, whole-dashboard-package tests, merge,
  push or deployment. Independent lite review is omitted under the binding
  print-mode adaptation: the lead runs it after the worker report.
- Unresolved implementation or acceptance findings: none. The earlier
  checkpoint below is retained as historical evidence, not current status.

## Implementation checkpoint - 2026-11-15

Source and focused regression work is checkpointed on
`impl/exec4/fullscreen-panel-width/chug-diner-dill`, not yet accepted or
independently reviewed. The private shared maximize owner now captures column
spans, applies semantic category widths, and owns a transient two-way extension
button. Preview rendering supplies its semantic category; English/Korean labels
and wide-only CSS accompany it. No protocol, stored-state, or daemon changes.

- Focused offline verification passed:
  `GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOMAXPROCS=2 GOFLAGS=-p=2 go test ./internal/wsdashboard -run '^(TestFullscreenPanelWidth|TestBoardSheet|TestFilesToolbarSheet|TestBoardCard|TestPreview)' -count=1 -timeout=20s`
  from `agents-plugin-tool/` (1.093s). Full output:
  `/tmp/exec4/fs-shots/go-cards.log`.
- The first Node regression attempt failed because the synthetic unknown kind
  named `constructor` collided with inherited registry membership on reload.
  The fixture now uses `other`; production registry semantics were not changed.
- The isolated browser harness
  `agents-plugin-tool/internal/wsdashboard/testdata/fullscreen-width.browser.cjs`
  passed its preliminary preview/card geometry assertions, then twice failed
  clicking `section.topic > .tophead .d-max`: the owning `#bcol`/grid intercepted
  pointer events. Removing accumulated synthetic cards and selecting the board
  tab did not resolve that failure. The fixture/implementation cause remains
  unproven. Worker stop (f) applies; no third attempt was made.
- Browser output: `/tmp/exec4/fs-shots/browser.log`. Owned Chromium/server
  process groups were retired and scanned after each run; final scan:
  `/tmp/exec4/fs-shots/browser-process-scan.log` (none remain).
- Required screenshots were not reached; no screenshots were produced.
  Browser lifecycle/mobile acceptance and the standalone whiteboard regression
  remain pending. Independent review belongs to the lead under the print-mode
  handoff adaptation. No phase Result or closure is recorded.
