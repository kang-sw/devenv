---
title: "Whiteboard topic history: no inner scroll; load older entries upward in place"
blocked-by: 261113-feat-ws-dashboard-board-reorder-hold#phase1
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "(473) board size and performance; the collapse keeps loaded history from accumulating"
  261113-feat-ws-dashboard-board-reorder-hold: "prerequisite: its Phase 1 builds the shared visibility observer and keep-in-place helper this ticket reuses (its decision 8)"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ea6e424bd3bc2475
sage-review-completeness-reviewed: ea6e424bd3bc2475
completed: 2026-11-14
---

# Whiteboard topic history: no inner scroll; load older entries upward in place

## Background

Owner answers on MAIN's board, 2026-11-13: (509)=ok, (510)=b with the note
"B에 카드 전체가 시야에서 벗어나면 다시 접히는 방식이 어떨까 싶긴 해요."

Today a topic's record view scrolls inside `.wb-history`
(`agents-plugin-tool/internal/wswhiteboard/assets/page.css#L458`:
`max-height: min(65vh, 720px); overflow-y: auto`), with paging in
`assets/stream.js`. The inner scroll nests a scroll area inside the page.

## Decisions

1. **No inner scroll; "load more" above the latest window** ((510)=b, owner
   2026-11-13): the topic card shows the latest window of entries, and a
   "load more" control above it expands older history upward in place.
   Throughout this ticket "latest window" means the 640px x 1 clip
   (step count 1) of Decision 8, not a fixed entry count.
   Rejected: (a) horizontal pages. This supersedes (202)=a of
   `261113-feat-ws-whiteboard-topic-panels-and-log` (bounded-height inner
   history; rejected an unbounded document with a show-more boundary): the
   owner's later (510)=b answer chose the show-more shape, and Decision 8's
   640px clip keeps each step bounded.
2. **Collapse when the card leaves the viewport** (owner note on (510),
   adopted by MAIN as a technical call): once the whole topic card is out of
   the viewport, its loaded history collapses back to the latest window.
   Rationale: loaded history does not accumulate on screen, which also
   helps the (473) board-size concern (rendering only; see Decision 6). Rejected: keeping expanded history
   until reload.
   The trigger is the `onOut` callback of `wbHistory.watch` on the topic
   card (the contract in the prerequisite's decision 8). `onOut` fires only
   on a transition from partly visible to fully out of view, so the
   collapse arms only once the card has been in view since its last
   expansion. A card that a deep link just expanded is not collapsed by the
   observer's first report, which can still be 0 before the deep-link
   scroll lands. It collapses only after it has been in view and then fully
   left it.
3. **Scroll compensation** (constraint from MAIN; helper owner set by the
   EXEC lead, technical): an expand or collapse keeps the visible content
   stable through the keep-in-place helper and the visibility observer that
   `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 builds in
   `wswhiteboard/assets/stream.js` beside `wbHistory` (its decision 8,
   (516) on MAIN's board); this ticket reuses them and adds no second copy.
   "Upward in place" means the entry that was oldest before the click keeps
   its screen y while the added height goes above it; older pages carry
   doc-frame iframes that report their height late, so the helper re-anchors
   on resize until layout settles. A collapse of a card that scrolled out
   above the viewport is compensated the same way. The card is fully out of
   view then, so the anchor is visible content outside it: the first
   visible element below the card, the topmost visible node in the
   scroller. A card collapsing below the viewport moves nothing visible.
   The helper targets the card's nearest scroller (`#bcol`, `#bpins`, or
   the document). Rejected: whichever ticket lands first builds the helpers
   (parallel workers could build two copies).
   Load-more and collapse call the same `wbHistory.keep` the board swap,
   the hold release and the pill use. On a scroller with a running session,
   keep re-anchors that session instead of starting a second one (the
   prerequisite's decision 4, one session per scroller). So a load-more or
   collapse that overlaps a swap or a release merges into it, and this
   ticket needs no compensation coordination of its own. Decision 7's motion
   and editing guards still apply. Rejected: a topic-history-local session
   or lock, which would duplicate the merge rule and could cancel the
   swap's session as a user scroll.
4. **Both surfaces**: the served board page and the dashboard board region
   (same topic markup).
5. **Compatibility tier: T0** (asset-only).

6. **Collapse resets the clip; loaded rows stay** (UX review and feasibility
   note 2026-11-13, technical): with the clip of Decision 8, each older page is
   materialized once from its embedded template and stays in the card; a collapse only sets the step count
   back to 1 (the latest window, the 640px x 1 clip of Decision 8). The
   keep-in-place helper anchors on visible content outside the card, the
   first visible element below it (Decision 3), because a collapsing card
   is fully out of view.
   Open and closed folds keep their state. Consequence for Decision 2's
   rationale: the collapse bounds what is shown, not what is loaded, so the
   (473) benefit is limited to rendering; hidden rows stay in the DOM.
   Rejected: rebuilding the latest window from kept template copies
   (`loadOne` deletes each page template, `stream.js:109-115`, and
   `joinBoundary` merges rows, so a rebuild needs a parallel copy of the
   history; the clip needs none).
7. **Swap and editing interplay** (UX review 2026-11-13, technical): the
   inner-scroll code (`stream.js` ~28-61, 199, 210-217) and the second
   `wbHistory.restore` pass (`dashboard.js:554-555`) are removed or
   repurposed; no collapse runs while board swap motion runs or while focus
   or answer input sits inside the history (`editing()`); the observer
   watches the nodes `morphBoard` inserts and unwatches the ones it removes.
   `.wb-history` loses its `tabindex` (`page.tmpl:52`), which existed for
   the inner scroll. The clip of Decision 8 replaces the inner scroll. The
   observer watching and unwatching above is the prerequisite's Phase 1
   wiring (`wbHistory.watch` / `sweep` from `swap()`), which this ticket
   subscribes to and does not rebuild. A load-more or collapse that
   overlaps a swap's or release's compensation merges into its keep session
   (Decision 3).

8. **Height-based window** ((528) withdrawn by the owner and replaced by
   (531), announced by MAIN 2026-11-13; owner: "rather base it on height,
   for example past 640px vertical, in 640px units; if that is not possible,
   load 3 more entries at a time"). Mechanism (UX feasibility note,
   technical): the history box is clipped by CSS `max-height` = 640px x
   steps with `overflow: hidden`; its content is anchored to the bottom
   (latest entries visible, older ones above), so the clip hides the oldest
   part at the top edge, where a fade and the "load more" control sit.
   "Load more" raises steps by 1; when the loaded content is shorter than
   the new clip (`scrollHeight` < steps x 640), it loads the next older page
   (one `<template data-history-page>` of `HistoryWindow`, 50 entries,
   `stream_view.go:11`, already embedded in the page and materialized by
   `loadOne`, `stream.js:109-118`). Late iframe heights only change
   `scrollHeight`, so nothing is measured at click time. The 3-entry step
   is the owner's fallback, used only if the clip approach proves
   unworkable (the worker records why in its result).
   - **Bounded load loop** (technical): a click materializes at most one
     older page. The `scrollHeight` read right after a materialization
     predates the height reports of the new page's doc-frame iframes, so a
     synchronous loop would over-load. Instead, on each later resize of the
     history content (a ResizeObserver report: a frame height arriving, or
     the new page's own layout), it checks again: while `scrollHeight` <
     steps x 640 and an older page remains, it materializes one more page.
     The chain stops when the clip is filled or nothing older remains.
     Rejected: looping synchronously at click time until the clip fills.
     That loads pages whose frames have not reported yet, up to every
     embedded page.
   - **Control states**: the "load more" control (`.history-older`) shows
     while an older page remains OR the content is taller than the current
     clip (`scrollHeight` > steps x 640). When nothing older remains but
     content still sits above the clip, a click only raises the clip by one
     step. It hides when nothing older remains AND the content fits the
     current clip. This replaces `loadOne`'s rule, which hides the button
     as soon as no template is left (`stream.js:116-117`).
   - **Live arrival**: a new entry appends at the bottom of the
     bottom-anchored clip, and the step count is unchanged. The clip shows
     the newest content, and the oldest visible rows slide out under the
     top fade.
   - **Deep links**: `loadTarget` (and `boardJump`, `dashboard.js#L3247-L3256`)
     materializes the target's page and raises the step count until the
     target lies inside the clip. The target is then brought into view by
     the page's scroller through `wbHistory.keep(target, { offset })`, so it
     stays put while late frame heights settle **during that shared keep
     session** (EXEC technical decision, 2026-11-14, under the owner's
     overnight self-settle rule). Navigation is armed only until the existing
     session ends: quiet after 500ms, cap after 5000ms, or a user scroll. A
     report after the session ends is a new layout event and is not
     compensated or re-revealed, matching board swaps under reorder-hold
     Decision 8. Rejected: arming navigation until cap or user scroll past
     quiet, changing shared keep options/timeouts, or adding a history-local
     session. Headless stability assertions sample after paint (rAF then a
     MessageChannel or zero-delay task), because rAF geometry can precede
     ResizeObserver's pre-paint correction. Reports spaced within quiet
     (for example 300ms apart) must stay within 1px; a separate post-session
     report documents permitted movement without re-arming navigation.
     The clipped box is never scrolled: `scrollIntoView` (`dashboard.js#L3254`, `stream.js:239`) also
     scrolls `overflow: hidden` ancestors and would break the bottom
     anchoring, so it is not called on a target inside the clip.
   - **"Unworkable clip"**, for the 3-entry fallback: bottom anchoring
     cannot be kept stable across late iframe heights in the headless check
     (the latest entry's screen y in a bottom-anchored card moves more than
     1 px while older frames report their heights), or the bounded load
     loop cannot fill a step without over-loading. The worker records the
     failing headless numbers in its result before switching.
   Rejected: a fixed 10-15 entry window; keeping all 50 entries visible;
   measuring cumulative entry heights at click time (late iframe heights
   make it unreliable).

## Constraints

- Daemon assets under `agents-plugin-tool/internal/` follow
  `ai-docs/manuals/release-compatibility.md`.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, Decisions): "History paging (202=a): bounded-height history with explicit older-page loading, preserving chronological order and the viewport anchor when history or new entries arrive. Rejected: an unbounded document with a show-more boundary." — bearing: contradiction-candidate
- 261113-feat-ws-dashboard-board-reorder-hold (2026-11-13, Decisions): "One visibility source and one keep-in-place helper, shared ... the two helpers belong in wswhiteboard/assets/stream.js, beside wbHistory (for example wbHistory.watch / wbHistory.keep)" — bearing: constrains
- 261113-feat-ws-whiteboard-board-frontend-performance (2026-11-13, Decisions): "Viewport mount/unmount ((532)=a, Phase 2): open frames mount within about 1-2 viewports and unmount beyond about 4-5 ... through the visibility observer that board-reorder-hold Phase 1 builds" — bearing: constrains
- 5b8d5871 (2026-11-13, commit): "Distinguish historical disclosure keys so a repost folds a previously expanded older version without losing later manual history choices." — bearing: constrains
- 54335ef5 (2026-11-13, commit): "Known cosmetic gap: Segments cannot fold runs across page boundaries, so up to 6 rows may stay unfolded after loading older entries." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | wswhiteboard/assets/stream.js, page.css, page.tmpl, wsdashboard/assets/dashboard.js, wswhiteboard/stream_script_test.go, scripts/board-headless.mjs (scenarios added to the prerequisite's harness) |
| scope.surface | cross-module | window.wbHistory in wswhiteboard/assets/stream.js is consumed by wsdashboard/assets/dashboard.js (lines 520, 555, 3250, 3258) |
| scope.new_public_symbol | no | none named; the keep-in-place and visibility helpers are built by board-reorder-hold Phase 1 and reused (pending 261113-feat-ws-dashboard-board-reorder-hold) |
| scope.new_type_contract | no | none; asset-only per Decision 5 |
| scope.test_surface | existing | agents-plugin-tool/internal/wswhiteboard/stream_script_test.go (inner-scroll stubs and the max-height assertion at its line 269 need rewriting); wsdashboard/whiteboard_bridge_test.go |
| complexity.reuse_points | unconfirmed | helpers do not exist yet: searched stream.js for watch/keep helpers, none (pending 261113-feat-ws-dashboard-board-reorder-hold) |
| complexity.side_effect_risk | high | replaces the inner-scroll position/restore logic shared by board swap, deep-link loadTarget, and capture/restore in both surfaces |
| risk.correctness | high | scroll compensation with late iframe heights, collapse while off-screen, and clip stepping interact across swaps |
| risk.fit | moderate | depends on an unlanded prerequisite and overlaps board-reorder-hold and board-frontend-performance on stream.js and dashboard.js |
| risk.test | high | existing node harness fakes scrollTop and heights; real layout and compensation need a headless browser check |
| risk.security_or_contract | moderate | asset-only (T0): older pages are the embedded `<template data-history-page>` pages (Decision 8); no fetch route is added |

## Phases

### Phase 1: Load-more history with viewport collapse

- Stop check first: verify that
  `261113-feat-ws-dashboard-board-reorder-hold` Phase 1 has merged
  (`git log --grep=261113-feat-ws-dashboard-board-reorder-hold` shows a
  commit recording its Phase 1 result, and `wbHistory.watch` and
  `wbHistory.keep` exist in `wswhiteboard/assets/stream.js`). If not,
  stop and report; do not build the helpers here. The frontmatter
  `blocked-by: ...#phase1` gates dispatch the same way. A typed block is
  safe here because this ticket has one phase, unlike the performance
  ticket, whose prose prerequisite avoids gating its own Phase 1.
- Remove the `.wb-history` inner scroll and its `tabindex`; the latest
  window renders in the card with a "load more" control above it
  (Decision 1).
- The 640px clip and load-more stepping (Decision 8); collapse resets the
  clip (Decision 6); swap and editing interplay (Decision 7).
- Collapse back to the latest window when the whole card leaves the viewport
  (Decision 2), with scroll compensation for changes above the viewport
  (Decision 3).
- Same behavior on the served board page and the dashboard board region.

Verification: asset tests for the 640px first window (the latest window)
and each ~640px step; the next embedded page materialized only while the
content is shorter than the clip, at most one page per click, and the next
one only on a later resize (Decision 8); the control states (shown with an
older page left, shown while content above the clip remains with no page
left, hidden when nothing older remains and the content fits); a live
entry appending at the bottom with the step count unchanged; load-more
expansion and the out-of-viewport collapse (fold state in the latest window
survives it); a board swap keeps an expanded topic expanded; a topic in the
pinned column; the inner-scroll stubs and the old `max-height: min(65vh,
720px)` assertion (around line 269) in `stream_script_test.go` rewritten.

Headless checks, added as scenarios to `scripts/board-headless.mjs` (the
harness the prerequisite's Phase 1 commits; no second harness), on the
dashboard board region and on the standalone whiteboard page:
- expanding or collapsing a card above the viewport does not move the
  visible content (scroll position compensated), including a load-more
  that overlaps a board swap's compensation (one keep session);
- deep link into an older page: the target ends up visible inside the
  clip and anchored, with its screen y stable while late frame heights
  settle. The card it opened is not collapsed by the observer until it has
  been in view and then fully left it;
- bottom anchoring stays within 1 px of the latest entry's screen y while
  older frames report their heights (the "unworkable clip" criterion of
  Decision 8).

LOOK captures of a long topic before and after "load more" on both surfaces.

### Result (397f746f6) - 2026-11-14

- VERIFIED: prerequisite Phase 1 landed in `3ee1d448b` (Result recorded by
  `cb68e48ce`); the existing shared `wbHistory.watch` / `keep` are reused.
  Both surfaces now use bottom-anchored 640px clip steps without inner
  scrolling or the old history tabindex. Loaded rows and fold choices survive
  viewport collapse and board replacement. The shared `viewKit` is byte-identical
  to the base (SHA256 `7ec85535314edf6c811d145b56d6f50d91af7380c75086f21427be065f2cefcb`).
- VERIFIED: EXEC retry diagnostics attribute the native loop warning to the
  final folded-log history delivery on both hosts, not startup/keyboard.
  Control at develop merge-base `2d46f8c1d`, using the same renderer seed,
  emitted zero warnings on either host, including exhaustion by separate
  clicks. It is therefore not waived as a pre-existing defect. Deferring
  control visibility changes out of the history callback, including reentrant
  binding, eliminates the warning. INFERRED: shrinking shallower observed
  ancestors caused skipped observations; browser-internal depth scheduling
  was not instrumented. Control evidence: `/tmp/ro-baseline-0Sw0xO/evidence/`.
- VERIFIED: real Tab originally focused p156 under a rail capsule on both
  hosts (`elementFromPoint` at its center). Navigation now clears computed
  sticky head top plus height in the same keep call. Final center hits belong
  to the focused summary on both hosts; clip.scrollTop remains 0. Dashboard
  summary y=138.140625..170.140625; standalone y=86.140625..116.140625.
- VERIFIED: final native `history-regressions` passes both hosts (2.5s/1.6s),
  with zero native errors: one synchronous page, then distinct actual native
  deliveries advance 50 -> 100 -> 150 -> 200 rows at unchanged 80px height.
  A real visible-to-out collapse cancels continuation with seven pages left.
  Node contracts also cover reentrant control deferral and sticky offsets.
- VERIFIED: full `history` passes dashboard/true standalone (8.2s/4.4s), with
  zero native errors and 0px post-paint expansion, latest-entry late-height,
  above/below collapse and deep-link drift. Dashboard swap/load-more uses the
  identical keep session and the pinned case selects `#bpins`. Exhausted-control
  states, retained folds/rows, initial observer zero, live-arrival fixture and
  post-session non-rearming pass. The 640px clip remains workable; no fallback.
- VERIFIED: uncached `go test -count=1 ./internal/wswhiteboard/...
  ./internal/wsdashboard/...` passes (26.763s/129.813s); `observer` passes (3.0s),
  genuine body-frame `keep` passes (4.9s; <=0.45px, no writes after user scroll),
  `node --check scripts/board-headless.mjs` and `git diff --check` pass.
  New log-only fixtures required selecting real body frames for keep and a real
  log payload for standalone below-collapse; assertions were not relaxed.
- VERIFIED: prior independent Lite findings are resolved. Original whole-ticket
  review: `/tmp/history-lite-review-keg-reverb-giblet.md`. One fresh retry-delta
  review (`6b0819338..397f746f6`) reports 0 Critical/Important/Minor, independently
  reruns native regressions on both hosts and verifies exact served assets:
  `/tmp/history-final-dV87jh/REVIEW.md`. No further review round requested.
- VERIFIED: full logs, raw measurements, phase diagnostics, inspected before/after
  load-more LOOK captures on both hosts and after-Tab captures are indexed in
  `/tmp/history-final-dV87jh/VERIFIED.md`. Only isolated headless serve daemons
  were used; the shared daemon was never restarted or swapped.
- Omitted: non-Chromium/compositor-filmstrip runs, server-delivered live arrival
  (rendered DOM fixture used), and long cap/user-navigation overlap. Standalone
  has inline bodies and no dashboard swaps/pins. Navigation lifetime remains
  the existing quiet/cap/user keep lifetime; no new local session or helper.

## Prior blocked state - resolved by Result above (2026-11-14)

Historical stop evidence follows; its pending items are resolved above.
At that stop, no phase Result or closure was claimed. EXEC settled the previous lifetime
stop; the dated clarification is now in Decision 8's Deep links bullet.
Shared `viewKit` remains byte-identical to the starting revision (SHA256
`7ec85535314edf6c811d145b56d6f50d91af7380c75086f21427be065f2cefcb`).

- VERIFIED: corrected post-paint probes and the existing full history, pin,
  observer and keep checks passed before review (`43c5cc07d`). The old
  214.4375px dashboard rAF excursion was 0px after paint; both surfaces had
  0px post-paint deep-link and latest-entry drift. Reports inside quiet
  stayed anchored; post-session reports did not re-arm navigation.
  Evidence: `/tmp/board-history-resume-DOOjWv/VERIFIED.md` and its raw samples
  and inspected before/after LOOK captures.
- VERIFIED: one fresh independent lite review found a Critical size-neutral
  folded-log paging stall and an Important clipped keyboard-focus target.
  Report: `/tmp/history-lite-review-keg-reverb-giblet.md`. No re-review was
  requested (one-pass lite allocation).
- VERIFIED: remediation re-observes the existing content on a later frame
  after a size-neutral merge, and shares navigation with clipped focusin.
  Node contracts (`efc79e6a2`) passed both packages uncached (21.622s /
  126.772s), as reported by the test leaf. Native browser delivery tests
  (`d67ecb997`) showed 50 -> 100 -> 150 -> 200 rows at an unchanged 80px
  height, one page synchronously and each subsequent page in a distinct
  real observer delivery. Standalone collapse stopped continuation with
  seven older pages remaining.
- VERIFIED: both isolated browser runs emitted the same native ErrorEvent:
  `ResizeObserver loop completed with undelivered notifications.` The
  strict standalone no-loop assertion failed. Stop (f): do not rerun this
  warning at the same tier. No shared daemon was touched; the isolated
  daemon was stopped. Evidence and inspected keyboard/chain LOOK captures:
  `/tmp/board-history-remediation-Gcg652/VERIFIED.md` and per-surface
  `measurements.json`.
- INFERRED: hiding the exhausted button during an observer callback may
  resize shallower observed ancestors, but the first error records lack
  phase timestamps, so this cause is unproven. Final harness diagnostics
  add timestamps/phases; they have not been rerun.
- VERIFIED: real Tab focuses p156 inside the clip/page rectangles on both
  surfaces with clip.scrollTop = 0. INFERRED: sticky headers may still
  occlude it (after-Tab LOOK); a rectangle check alone does not settle the
  review finding. Add hit-testing/occlusion evidence before claiming fixed.

Pending: diagnose the native observer warning; finish keyboard visibility
and prove the review fixes; run the corrected dashboard load-event recorder
and cancellation, full history/keep/observer baselines on the final assets,
and record a Result only after green verification. Existing green browser
artifacts predate these remediation assets. The final dashboard recorder
correction is syntax-checked, not browser-verified. No full final green run,
fixed-review verdict, 3-entry fallback, or ticket completion is claimed.
