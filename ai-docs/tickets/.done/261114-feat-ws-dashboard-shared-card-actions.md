---
title: "Dashboard: shared card collapse, expand and raise actions with topic-relative preview opening"
related:
  261110-feat-ws-dashboard-file-preview: "preserve existing preview reuse and sandbox; opening placement changes here"
  261112-feat-ws-dashboard-pin-column: "preserve existing pin/flow grouping"
  261112-feat-ws-whiteboard-topic-tickets-sticky-head: "preserve topic ticket badges and state displays"
  261113-feat-ws-dashboard-board-reorder-hold: "interaction reference only; frozen Phase 4 is not reopened"
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 7d5a6e0295ba02c2
sage-review-design-reviewed: 7d5a6e0295ba02c2
completed: 2026-11-14
---

# Shared card actions and topic-relative preview opening

## Background

The owner requested one framework for topic, terminal and file-preview card operations: collapse/expand the whole card, raise it, and open a preview immediately above the topic from which it was opened. Folding only a topic's content leaves its history and question regions consuming space. Existing topic-specific ticket badges must remain visible and extensible rather than being lost in a generic rewrite.

Owner approvals on 2026-11-14: (660)=ok, (661)=a, (662)=a, (663)=ok. There is no approved new visual mockup; this ticket captures an interaction/structure contract and preserves existing renderer/chrome rather than treating unrelated toolbar/header mockups as approval for this work. This standalone unit does not wait for hosted-agent commonality or mention notes.

## Decisions

1. **Adopt existing topic DOM into a shared action layer.** Preserve server-rendered topic body and renderer; share collapse/expand/raise behavior with terminal and preview cards through type-specific header/body/badge/lifecycle extension points. Rejected: replacing the whole topic renderer or hiding its ticket metadata to fit a generic card. Authority: (660).
2. **Whole-card collapse.** Hide body, history and question regions, leaving title, ticket badges, state and action controls. Restore existing internal disclosure states on expansion. Rejected: content-only collapse or resetting every inner disclosure. Authority: (663).
3. **Raise icon.** Put a raise-to-top icon action in the upper-right common card controls. It moves the card to the top of its existing pinned or ordinary region, without changing pin state. Authority: the original request and (662)=a.
4. **Topic-relative opening is one-time placement.** A preview opened from a topic is placed immediately above that topic when the topic is in the ordinary region; this applies to new cards and reuse/replacement of the existing preview slot. The preview is independently movable afterward. Rejected: a persistent follow/attachment relation. Authority: (661)=a.
5. **Pin preservation wins at the boundary.** Opening an ordinary preview from a pinned topic places it at the ordinary-region head rather than silently pinning it. Existing matching-pinned-preview reuse remains pinned. Raise never changes pin state. Rejected: changing pin state merely to guarantee adjacency across regions. Authority: (662)=a.
6. **Compatibility is T0.** Reuse existing card placement/pin persistence where possible; no non-additive store/protocol change or new persistent parent/follow relation is authorized.
7. **Remove the redundant content-only header control.** After inspecting the actual isolated candidate, the owner requested removal of the upper-right content-only fold button now that whole-card fold exists. Preserve independent inner history/panel disclosures. Rejected: retaining two confusing header fold actions.
8. **Distinguish raise visually.** Replace the inverted-fold-chevron raise icon with an upward arrow pointing toward a horizontal top line. Preserve its raise behavior and accessible action label. Rejected: using a flipped fold icon for a different action.
9. **Owner visual acceptance permits source integration after these two fixes.** The owner reported no apparent usability problem in the interactive preview and explicitly directed fixing Decisions 7-8, then merging, with subsequent bugs addressed during use. The known 420px pointer-drag assertion failure and unexecuted remaining browser matrix must remain disclosed; do not report them as passing or keep repeating that matrix as a merge prerequisite. Focused final regression/build checks and the allocated independent lite review remain applicable. This approves source integration, not shared/live daemon replacement or push.

## Constraints

- Keep type-owned lifecycle behavior. Generic card close is not the workflow operation that closes a whiteboard topic; existing terminal disposal and preview cleanup remain their own operations.
- Preserve existing preview sandboxing, one-unpinned-slot behavior and matching pinned reuse. Files explorer's floating sheet is not automatically converted into a generic card by this ticket.
- Carry the validated originating topic through direct links, link menus and sandbox frame-link dispatch; a detached/invalid origin falls back to normal ordinary-region opening instead of borrowing an unrelated topic.
- Preserve existing source-update, pin/drag/manual placement and maximum-view behavior outside the specifically approved placement/collapse/raise changes. No persistent topic-preview relationship is introduced.
- Existing frozen board-reorder Phase 4 and header/ResizeObserver work are not reopened. If common actions cannot be integrated without changing a frozen contract, return that precise conflict to the lead rather than retrying the frozen lane.
- Applicable implementation manuals: `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md`. No skill/template/convention change is required by this interaction ticket.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Art

- `agents-plugin-tool/internal/wsdashboard/assets/cards.js`: generic cards, placement/pin/drag/maximize and existing card state (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L161-L215`, `#L337-L365`, `#L395-L417`, `#L428-L549`, `#L595-L697`, `#L753-L781`, `#L867-L891`). Topic decoration currently adds pin/maximize/back, while generic card headers own fold and type badges; the shared raise and topic whole-card collapse are new work.
- `agents-plugin-tool/internal/wsdashboard/assets/preview.js` and `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`: preview opening and origin/event dispatch. The existing preview slot replaces/reuses ordinary content via `card.bump()` and reuses a matching pinned card without bumping (`preview.js#L218-L265`); direct links and link menus currently pass no originating topic (`dashboard.js#L3092-L3103`, `#L3150-L3179`). Frame-link dispatch validates the source frame and document key, then creates a detached link for the menu (`dashboard.js#L3900-L3911`, `#L3958-L3971`, `#L4066-L4096`); origin must come from that validated frame, not message-supplied topic identity. Preview sandboxing and type labels remain kind-owned (`preview.js#L60-L69`, `#L111-L115`, `#L156-L168`); terminal state badges and close/disposal remain terminal-owned (`agents-plugin-tool/internal/wsdashboard/assets/terminal.js#L278-L282`, `#L392-L430`).
- `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`: server-owned topic DOM and topic-specific header/badges (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L38-L43`, `#L105-L108`). The `.tophead > .tbar` contains title/ticket/state controls, while checks, document, history and question regions are separate descendants; collapsing only `.tdoc` cannot collapse the whole card (`agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl#L36-L43`, `#L52-L52`).
- Topic morph/update integration is in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js#L800-L928`, not a dashboard `assets/stream.js` (that path does not exist). `agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L354-L405` owns history capture/restore, including inner fold states; retain these owners and the existing renderer. General keyed disclosure restoration is in `dashboard.js#L195-L215`. The common maximum-view lifecycle already supports cards, topics and panels without moving their DOM (`agents-plugin-tool/internal/wsdashboard/assets/cards.js#L595-L697`).

## Prior Decisions

- 01088661 (2026-11-14, commit): "Shared, not separate: the sheet lives in cards.js maximize() (one scrim, one Escape handler, one `maxed`)" — bearing: constrains
- 541fe405 (2026-11-14, commit): "Retain target-level dropdown/menu/search Escape consumption and the document-level sheet Escape handler; dismiss the Files sheet before an existing preview/card reveal" — bearing: constrains
- 8f24cec5 (2026-11-14, commit): "Saved entry IDs replace cumulative 640px snapping so later geometry can grow the clip without changing its membership." — bearing: constrains
- 0325a7aa (2026-11-14, commit): "panel fold state now persists in localStorage ws.dashboard.panelshut (capped LRU 200, key <rid>/<data-key>)." — bearing: constrains
- 701cf55a (2026-11-12, commit): "The page decides \"outside\" from markup the server rendered (file link, an absolute path, no path under the root), and the daemon decides on the click whether the path is really linked" — bearing: constrains
- 261110-feat-ws-dashboard-file-preview (2026-11-11, Result): "Unpinning a preview card makes it the slot and closes the other unpinned preview card." — bearing: constrains
- 5695371f (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: constrains
- 702e2f9d (2026-11-13, commit): "Structure is built server-side (EntryPage.Segments) rather than by client JS wrapping, because the dashboard's frameGraft needs matching DOM shapes along a frame's path." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/cards.js, assets/preview.js, assets/dashboard.js; agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl and assets/stream.js are renderer/history preservation boundaries |
| scope.surface | cross-module | window.WsCards action/placement layer, window.WsPreview.open callers, and server-rendered wswhiteboard topic DOM |
| scope.new_public_symbol | unknown | Type-specific extension points and origin plumbing are authorized, but no new exported JS name is specified; no Go/MCP export is requested |
| scope.new_type_contract | yes | Shared topic/header/body/badge/lifecycle extension contract and originating-topic context across preview entry paths; concrete signatures remain implementation details |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/cards_test.go#L74-L100, preview_outside_test.go#L64-L100, whiteboard_bridge_test.go; scripts/board-headless.mjs can host focused browser regressions; no new test file required by the phase |
| complexity.reuse_points | confirmed | cards.js order/headTime, topic decoration, persist, Card.fold/bump, maximize and move; preview.js open/slot/pinned matching; dashboard.js morphBoard/docFrame/frameLink; wswhiteboard/assets/stream.js capture/restore were read |
| complexity.side_effect_risk | high | Shared placement and collapse touch persistent view state, live topic morphing, focus and iframe/terminal sizing across pin/flow and maximum-view transitions |
| risk.correctness | high | One-time adjacency must cover slot replacement and pinned reuse without changing pin state or losing inner fold state on updates |
| risk.fit | moderate | Existing topic DOM and type-owned badges/lifecycles must be adopted into WsCards, not rebuilt; frozen lanes remain outside this unit |
| risk.test | high | Node placement tests cannot establish real hidden-body geometry, focus, iframe continuity or narrow/wide maximum-view behavior; browser verification is still future work |
| risk.security_or_contract | moderate | Origin propagation crosses validated sandbox-frame dispatch and detached link menus; keep docFrame/document-key validation and preview capability/sandbox boundaries unchanged |

## Phases

### Phase 1: Unify card actions and implement approved opening placement

Implement shared action behavior and a topic adapter without replacing the existing topic renderer. Wire collapse/expand/raise consistently across topic, terminal and preview cards. Carry the originating topic through preview-opening entry paths and apply the approved one-time placement and pin-boundary rules.

Verification:

- Browser coverage at wide and narrow widths with topic, terminal and preview cards, including a topic with ticket badges, open questions, history and individually expanded entries.
- Collapse visibly removes all body/history/question regions; expand restores each inner disclosure state; title/badges/state/actions remain usable. Verify keyboard focus and accessible action labels.
- New ordinary preview, reused ordinary slot and replaced preview slot open immediately above an ordinary originating topic. Afterward independent drag and topic updates do not create persistent following.
- Pinned-origin fallback goes to the ordinary-region head without changing pin state. Reusing a matching pinned preview preserves its pin; raise moves only within the current region. Include empty regions and maximum-view transitions.
- Cover direct link, context/menu and sandbox-frame link opening plus invalid/disappeared origin fallback. Preserve URL/file sandbox boundaries.
- Verify reload behavior using the existing placement persistence contract, and verify normal live topic updates without resetting card/inner disclosure state or duplicating event handlers.
- Run affected existing dashboard/card/preview tests and new focused browser regressions. Do not use a frozen-lane retry or live daemon replacement as acceptance. Keep source verification separate from actual runtime delivery.
- Verify that the redundant content-only header fold action is absent, whole-card fold and independent inner disclosures remain usable, and raise uses an upward-arrow-to-top-line icon at wide/narrow widths. Capture the updated controls for visual inspection. Under Decision 9, preserve the known narrow drag failure and remaining matrix omissions as residual evidence rather than weakening assertions or claiming full browser acceptance.

### Result (1ce546502) - 2026-11-14

- Shared whole-card fold/expand and region-preserving raise now cover renderer-
  owned topics, terminals and file previews. Exact live topic origins reach
  direct/menu/validated sandbox dispatch, and one-time preview placement uses
  independent persisted snapshots rather than a follow relation. Type-owned
  badges, disposal, topic workflow lifecycle, sandboxing and inner disclosures
  remain intact.
- Owner visual acceptance and Decisions 7-9 govern completion. Final polish
  removes the dashboard-only content disclosure (standalone retains it), keeps
  its body under whole-card fold, and uses the exact vendored Lucide
  `arrow-up-to-line` icon with unchanged accessible labels. Renderer-owned DOM
  shape preserves incoming/live frame-graft agreement; no client-only unwrap
  or frozen-lane implementation change was introduced.
- Final affected checks passed: `GOPROXY=off go test ./internal/wsdashboard
  ./internal/wswhiteboard ./internal/wsi18n -count=1 -timeout=15m`
  (143.815s/19.339s/1.130s), `GOPROXY=off go build -o
  /tmp/shared-card-polish-source ./cmd/ws-mcp`, changed JavaScript syntax and
  `git diff --check`. Full logs: `/tmp/shared-card-polish-packages.log`,
  `/tmp/shared-card-polish-build.log`, `/tmp/shared-card-polish-static.log`.
- Focused `shared-controls` browser checks passed at 1800x900 and 420x900 on
  the actual private source fixture: removed content control/padding marker,
  title/ticket/state chrome, independent history/panel interaction and state,
  all three icons/labels and ordinary/pinned raises, whole fold/focus, topic
  maximum/back and production source-update retention. Logs:
  `/tmp/shared-card-polish-adapter-1800.log` and
  `/tmp/shared-card-polish-adapter-420.log`. Updated clear header closeups and
  expanded/folded/maximum captures are under
  `/tmp/shared-card-polish-artifacts/adapter-1800/` and `adapter-420/`.
- One fresh independent lite review of
  `ef0a5ea3718be5333ac3fb346fd5574a0f3972fb..1ce5465020acbd0c14da7ad718d92cf11bbb74f7`
  returned clean, with no actionable Critical, Important or Minor findings.
  Report: `/Users/kang-sw/.cache/ws@kang-sw-devenv/proj/17da6bdc@2b250898/review-paths/3a58f150-01-shared-card-actions-lite.md`.
- Residual evidence is not a pass: the earlier complete 420px `shared-actions`
  pointer-drag assertion failed (`/tmp/shared-card-671-adapter-420.log`), and
  that cause and the historical repeated-update failure remain unclassified.
  The assertions and earlier evidence are preserved. The remaining broad
  narrow/board-only browser matrix was not rerun under Decision 9; full
  automated browser acceptance is not claimed. MAIN owns disposition/tracking
  of those disclosed residuals after closure, rather than treating owner
  acceptance as a waiver of a newly diagnosed defect.
- Only the owned private preview was refreshed for source `e2d38b538`:
  `http://127.0.0.1:65281/s/detestrockfishswingingacts`, launcher 57186,
  daemon 57804; metadata/logs are `/tmp/shared-card-polish-launch.json` and
  `/tmp/shared-card-polish-serve.log`. Earlier owner-inspected captures remain
  preserved. No shared/live daemon, provider, installer, push or source merge
  was performed. Owner-approved source integration remains MAIN-owned;
  private preview delivery is distinct from actual shared runtime delivery.

## Historical block (2026-11-14): source verification stopped on non-convergence

Implementation checkpoints `62455a048`, `20e647c3a`, `c7c9c0f06`,
`760df43a2`, and `0c416f221` are retained on
`impl/docs/settled-common-contracts-660-665/outer-buddy-hush`. Phase 1 has no
Result and is not accepted. No shared/live runtime was changed.

- Shared topic/terminal/preview fold and raise, exact-node origin propagation,
  additive placement snapshots (including tied/zero timestamps and closed-tail
  placement), and focused Node/browser regressions have landed. Topics retain
  their renderer, ticket/state chrome, inner disclosures and workflow lifecycle.
- The real-pointer browser regression exposed a pre-existing generic-card drag
  eligibility bug; `760df43a2` limits the closed-topic rejection to topics.
  The same commit fixes zero-time snapshot reload and removes an unnecessary
  whiteboard-owned CSS class reference rejected by the namespace guard.
  Focused placement/card/preview/frame/CSS tests pass after these changes.
- An open source defect remains: maximizing a folded terminal leaves its body
  hidden because the terminal-specific folded-body selector overrides the
  common maximum-view rule in `assets/dashboard.css`. The proposed narrow fix
  is to exclude `[data-max]` from that kind-specific fold selector; it was not
  attempted before this stop.
- `node scripts/board-headless.mjs shared-actions --url
  http://127.0.0.1:63974/s/cauterizeprovabletrekkersway --width 1800
  --capture-dir /tmp/shared-card-final-artifacts --case persistence` failed
  twice at the repeated-topic-update fold/action-state assertion, after one
  test-timing correction. Its cause remains unresolved (test pre-toggle state
  assumption versus product behavior). The worker stopped under protocol (f)
  rather than making another attempt or weakening the assertion. First and
  second attempt evidence is preserved in `/tmp/shared-card-attempt-evidence.md`
  with exact transcript line/timestamp pointers; the second attempt overwrote
  the shared browser log. The assertion remains at
  `scripts/board-headless.mjs:1227-1228`. No diagnosis is claimed.
- Final wide evidence before that failure covers real direct/context/frame
  origin dispatch, identity rejection, new/reused/replaced preview placement,
  pin/raise boundaries, pointer drag, topic/preview maximum view, inner
  disclosure retention and stable manually placed preview time across a changed
  topic update. Logs: `/tmp/shared-card-final-adapter-1800.log` and
  `/tmp/shared-card-persistence-adapter-1800.log`; captures:
  `/tmp/shared-card-final-artifacts/`. Earlier narrow evidence is not final
  acceptance of the expanded scenario.
- Final narrow/board-only runs and downstream manual-order reload,
  invalid/disappeared/empty-region browser assertions remain unexecuted.
  Full affected-package/build verification must be rerun on the final candidate;
  the earlier suite found the subsequently fixed CSS namespace conflict.
  The isolated source fixtures were stopped. Their serve logs report
  `ENOTEMPTY` cleanup errors; `/tmp/shared-card-cleanup.log` subsequently records
  absent fixture daemon PIDs 81550/94179, no fixture processes, and removal of
  both private temporary trees after making their read-only Go module caches
  writable. Logs and captures were retained. The terminal maximum CSS repair
  was not made. No live delivery, provider, installer, frozen Phase 4/649,
  history or ResizeObserver changes were made.

Resume at a higher tier: diagnose the repeated action-state failure before
editing its expectation, repair the terminal maximum-view specificity defect,
finish the source verification matrix, and run the allocated independent lite
review. Review and closure have not run.

### Owner-authorized resumption - 2026-11-14

Owner whiteboard answer (671)=a explicitly authorizes one bounded additional
round on the same retained worktree/branch at `7356b35ac`, at the existing xlarge
tier. This supersedes the earlier higher-tier resume instruction, not the stop
evidence. Diagnose the repeated-update action-state failure against actual
behavior before changing code or expectations; then narrowly repair terminal
fold/maximum specificity, finish the browser/package/build verification, and
run the already-allocated independent lite review. Historical logs/captures
remain preserved. No Result or closure is authorized without full acceptance;
P4/649/history/ResizeObserver and live/shared runtime boundaries remain frozen.

### Bounded continuation outcome - 2026-11-14

Owner671's additional round stopped without Phase 1 acceptance. Candidate
`34592e8b325d690eb75bbee155475f95abd31696` is retained; no Result, closure,
integration or live delivery is claimed.

- Two read-only browser diagnostic runs at the retained implementation passed
  the historical repeated-update assertion. Synchronous pre/post evidence was
  `base`/expanded with one fold and one raise control, then `fold` after one
  click (`/tmp/shared-card-671-state.log`). Detailed action/placement traces are
  in `/tmp/shared-card-671-diagnosis.log`. The earlier failure's cause remains
  unclassified; no timing explanation was promoted to a diagnosis, no product
  fold code was changed, and the original postcondition was not weakened.
- That diagnostic exposed a separate deterministic fixture defect downstream:
  reload replaces `__bh`, but the scenario reinstalled `OVERVIEW_PAGE` without
  `IN_PAGE`, leaving `board_` undefined. `34592e8b3` restores both helper layers,
  adds explicit pre-toggle state assertions, and excludes `[data-max]` from
  the terminal-specific folded-body CSS mask. Focused card/preview/frame/CSS
  tests, JS syntax and diff checks passed.
- The complete source-fixture adapter scenario at 1800x900 passed, including
  real pointer drag, preview and terminal folded/maximum geometry/focus,
  repeated-update state and unique controls, reload/manual placement, invalid
  and disappeared origins, and empty-region raise. Evidence:
  `/tmp/shared-card-671-adapter-1800.log`; captures:
  `/tmp/shared-card-671-artifacts/adapter-1800/`.
- The next command failed: `BOARD_HEADLESS_PLAYWRIGHT=/Users/kang-sw/.npm/_npx/705bc6b22212b352/node_modules/playwright node scripts/board-headless.mjs shared-actions --url http://127.0.0.1:64455/s/ascensioncatalogsidinghelping --width 420 --capture-dir /tmp/shared-card-671-artifacts/adapter-420`.
  Output: `FAIL shared-actions: actual pointer drag independently reorders an
  opened preview without pinning` (`scripts/board-headless.mjs:1187`). All
  preceding narrow checkpoints passed. The assertion combines changed time,
  pin state and drag-marker cleanup without recording their separate values;
  the failure's cause is unclassified, not asserted to be product or fixture.
  Full output: `/tmp/shared-card-671-adapter-420.log`. No retry or test/source
  adjustment followed this failure.
- Board-only runs, remaining narrow checkpoints, final affected-package/build
  checks and the allocated independent lite review remain unexecuted. The
  verification-only child made no edits or commits. Cleanup evidence in
  `/tmp/shared-card-671-cleanup.log` records absent launcher PID90919 and daemon
  PID91552 and removal of the private `board-headless-AZHJNs` temporary tree.
  Retained671 evidence uses new paths and does not overwrite the earlier
  attempt logs. No frozen-lane or shared/live runtime action occurred.

Further work requires MAIN disposition; this owner-authorized round is exhausted.

### Owner visual acceptance and source-integration direction - 2026-11-14

After inspecting the isolated actual candidate under the preview-first direction,
the owner reported no apparent usability problem, requested removal of the
redundant content-only header fold button and a distinct upward-arrow-to-line
raise icon, then explicitly directed source merge and subsequent bug discovery
during use. Decisions 7-9 record that direction. This supersedes the operational
hold on those bounded fixes and source integration, not the preserved failures
or the frozen/live boundaries. No additional blanket approval is required.

The earlier 420px drag failure remains unclassified. The owner did not turn it
into a passing test or authorize assertion weakening. Keep final verification,
review results, residual risks and actual delivery status distinct.
