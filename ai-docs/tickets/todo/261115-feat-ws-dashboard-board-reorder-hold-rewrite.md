---
title: "Dashboard board: hold what the user reads when the server reorders it (fresh re-implementation)"
related:
  261113-feat-ws-dashboard-board-reorder-hold: superseded; its branch impl/develop/perm-unlit-icon is the reuse source (see Reuse map)
  261115-feat-ws-whiteboard-viewed-panel-anchor: develop staging this ticket builds on (boardStage*, boardPending, boardFlushViewed)
---

# Dashboard board: hold what the user reads when the server reorders it (fresh re-implementation)

## Background

Supersedes the stalled implementation of `261113-feat-ws-dashboard-board-reorder-hold`.
Only its Phase 1 (scroll compensation on every board swap, 0b58bed9a) landed on
develop. Phases 2-4 were implemented on `impl/develop/perm-unlit-icon` (22
commits, 1104 behind develop; Phase 2/3 Result 0520c1301). Phase 4 stopped for
non-convergence and the owner-approved (694) repair round also failed. The owner
chose to extract the spec and re-implement on current develop, not merge the branch.

The server orders open topics by last update, so an update moves its topic to the
board head and a close moves it to the closed section. The user must not see the
cards they are reading move.

Current develop already changed the ground (read before designing):
- `261115-feat-ws-whiteboard-viewed-panel-anchor` (owner (800), (802), (803), (818)=a)
  landed a staging mechanism in `wsdashboard/assets/dashboard-board.js`
  (`boardStageInput`, `boardStageOrder`, `boardPending`, `boardFlushViewed`,
  `boardReleaseDue`; bottom badge "최신 변경 N건 반영", hard-coded Korean, no i18n),
  a transaction-style `wbHistory.keep({capture:false})` and single scroll owner in
  `wswhiteboard/assets/stream.js`, `topicResized` in `dashboard-frames.js`, and
  `restoreScroll` in `dashboard.js`. Visible panels/cards freeze; changes release on
  the bottom badge click or when the panel fully leaves view. That ticket's Phase 1
  stays open pending owner acceptance on screen.
- dashboard.js was split (793): board code is in `dashboard-board.js`,
  `dashboard-frames.js`, `dashboard-stream.js`. Branch patches to dashboard.js do not apply.
- Not on develop: "moved ↑"/"closed" tags and accent, the nudge, the top-center
  back-to-top badge (idle and pending forms), `js.board.hold.*` keys, explicit
  closing-hold semantics (verify whether staging already covers a visible topic closing).

## Decisions (binding only)

Owner decision (1100)=a (2026-11-15), settling the former O1/O2. Verbatim option text:
"v2 목업 배지로 바꾸고 하단 배지는 없앱니다. 맨 위에서도 화면을 그대로 두고, 변경이 오면 맨
위에서도 "보드 업데이트됨 · N" 배지를 띄웁니다. 누를 때 반영합니다((593)/(694) 유지)".
Owner's reason (verbatim, on the current bottom badge): "지금 거 솔직히 애니메이션도 없고,
디자인도 별로고, 눈에도 잘 안 띄고".
- One pending/apply surface: the v2 top-center badge (`board-totop-badge-mockup-v2.html`);
  its form is superseded by Decision 11 (merged into the nav pill), the one-surface rule stands.
  develop's bottom "최신 변경 N건 반영" bar is removed; everything it counted/applied (viewed-panel
  content changes and visible reorders from 803/818) is counted and applied by the top badge.
- At the top (`scrollTop <= 4`) the visible layout stays stable ((593)/(694)); pending changes
  show the "Board updated · N" badge at the top too, and apply only on click. The (629) at-top
  auto-insertion clause and the mockup's "no badge at the top" behaviour are void.
- Compatibility tier: T0 (frontend assets + i18n keys, no MCP/store/tracker change).

Binding decisions:
1. Closing topics hold too ((515)=b). A visible topic the server closes takes the server's
   folded closed rendering, stays at its last open-flow place (`hold.at`), and moves to the
   closed section on release. `.closed-page` contents and `data-closed-next` counts never
   change versus the no-hold path. Reopen while held drops the close hold and is re-evaluated
   as a rise.
2. Release is batched and observer-driven ((517)=ok): every involved card (the held topic and
   the cards around its destination slot the move would shift) fully off-screen, visibility
   measured below sticky toolbar/heads (partial intersection = visible, header-covered =
   off-screen). All holds that meet the condition release in ONE settle. Selection,
   `:focus-within`, `:hover`, `pageHidden()` and overview gates defer release and retry when
   they end.
3. Compensation continues until layout settles ((516)=ok): 500 ms quiet window, 5 s cap, ends at
   first user scroll. Momentum guard: swaps, releases and late frame heights wait until the
   scroller has been quiet for about 120 ms; a late frame height never restarts a session a user
   scroll ended. Manual compensation over native `overflow-anchor` (stays `none`).
4. Pinned topics never move or hold ((447)). Maximized card stays put; no compensation under the
   maximize sheet. Editing/drag defer the swap.
5. Hold state is ephemeral: not in `persist()`, not in localStorage; a reload drops it.
6. Indicator ((525)=b; v2 badge (572)=ok, (590)=ok, (573)=a, (574)=b; mockups under
   `ai-docs/tickets/assets/261113-feat-ws-dashboard-board-reorder-hold/`):
   held rising topic gets an accent left border and a "moved ↑" tag (closing hold: "closed");
   one 3 px nudge, 300 ms, at hold start and on each re-update, none under reduced motion.
   Badge: 32x32 round ↑ when scrolled down with nothing pending; hidden at the top only when
   nothing is pending (with pending changes it shows at the top too, (1100)=a); pending form "Board updated"
   (+ " · N" for N>=2), capsule translateX sway (-4/+4/-3/+2/-1 px, 0.9 s, loop 2.6 s), none under
   reduced motion. N counts distinct topics from data, never from mounted DOM nodes
   (`fix/pill-count-churn` rule). Click = release all in one settle ((622): end any keep session,
   release, smooth-scroll to `top: 0`, flash counted topics once ~0.6 s, clear N; instant under
   reduced motion; no keep session during the to-top scroll). A hand scroll to top clears N and
   flashes topics in view but releases nothing. Hidden in overview.
7. i18n: `js.board.hold.pill` "Board updates" (ko 보드 업데이트; Decision 11 form "보드 업데이트 N"),
   `js.board.hold.pill.title` "Back to top" (ko 맨 위로), `js.board.hold.moved` "moved ↑", `js.board.hold.closed` "closed",
   in every `wsi18n/tables/*.json`; non-English entries flagged unchecked.
   The " N" count suffix is composed in JS (always shown while pending, N>=1). Develop's hard-coded Korean bottom-bar string is removed
   with the bar ((1100)=a).
8. Hidden tab/iframe-shell frame: `pageHidden()` gates swaps, holds, sessions and release.
9. (621)/(632) dropped: new topics land at the head above the view; no insertion hold for them.
10. (800)=a single writer per scroller applies: no second compensation writer may be added.
11. Badge merges with the existing nav pill (owner, 2026-11-15 19:11, verbatim:
   "목업 뱃지가 이 응답 뱃지랑 잘 섞여야 하는데, 그러면 위쪽 화살표는 항상 살아있따가(일종의 홈
   버튼) 응답이 생기면 해당 사이즈로 매끄럽게 미끄러지고(물방울처럼), 보드 업데이트됨 시점에는
   응답이나 미결보다 더 우선시되는 방향을 과죠"). The board-updated badge is not a separate
   top-center control: it merges with the floating nav pill in
   `agents-plugin-tool/internal/wswhiteboard/assets/pill.js`, which already shows an up arrow and
   the open/answered/noted chips (미결/응답/노트). The up arrow is always present as a home button.
   When chips appear, the pill grows smoothly to their size, like a droplet. While board updates are
   pending, the "보드 업데이트 N" state takes priority over the answered/open chips.
   This supersedes the form and visibility clauses of Decision 6 that describe a standalone v2
   badge: "32x32 round ↑ when scrolled down", "hidden at the top only when nothing is pending", and
   the capsule as its own element. Decision 6's behaviour stays binding: N counted from data, click
   releases everything in one settle and scrolls to top, counted topics flash, a hand scroll to top
   clears N, hidden in overview, reduced motion.
   Form, fixed by the owner-confirmed v4 mockup plus one restoration (owner (1108), 2026-11-15:
   v3's double capsule rejected, "보드 업데이트됨 텍스트 캡슐은 빼죠. 이중 캡슐은 보기 좀
   별로에요."; then on v4, verbatim: "네 확인. "보드 업데이트 N" 텍스트는 복원해주세요. (캡슐만
   빼란 말이었어요) 추가 목업은 없이 이대로 구현 진행 바랍니다."):
   - ONE capsule (the nav pill). No inner capsule for the board-updated state.
   - Pending board updates: the capsule gets an amber border and sways; inside it the ↑ and the
     plain text "보드 업데이트 N" (i18n `js.board.hold.pill` + " N") are amber, with no own
     background or border. The answered/open chips shrink to small count bubbles at the end.
   - Click on ↑ or on the text = release all + scroll to top (Decision 6 behaviour).
   Acceptance reference (side-by-side comparison required in Phase 1):
   `ai-docs/tickets/assets/261115-feat-ws-dashboard-board-reorder-hold-rewrite/nav-capsule-accepted.html`
   (v4 with the restored text; states via `tt.set({ans,open,upd})`, light/dark, 420/1240 widths).
   The unmodified owner-reviewed v4 is kept beside it as `nav-capsule-mockup-v4.html`.
   Rejected: v3's separate amber inner capsule "보드 업데이트됨 · N" (owner (1108)); v4's bare
   count without text (owner (1108) restored the text); the standalone v2 top-center badge.

## Constraints

- Read first: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, `ai-docs/manuals/skill-authoring.md` only if
  prompts are touched (they should not be). Assets + i18n only; T0.
- UI fast-feedback rule: each phase ends with something the owner can look at on screen
  (isolated `node scripts/board-headless.mjs serve`, screenshots at 1440/700 px, light/dark,
  reduced motion). Do not stack phases before the owner has seen the previous one.
- Do not add a competing scroll writer. Route any new compensation through the existing owner
  in `stream.js` (`wbHistory.keep`/`jump`) and `topicResized`; reconcile, do not layer.
- **ResizeObserver wrapper rule (from the (694) diagnosis, 32a27c0b9).** Native ResizeObserver
  calls its callback as `(entries, observer)`. Never register an internal function whose second
  parameter means something else (the branch's `topicResized(list, request)` took the observer
  as a request and dereferenced an undefined scroller). Register a wrapper that drops the
  observer argument, and call the private function with an explicit request. Also never consume
  a pending marker (`wbKeep`) before its first-height observer delivery, and never replace a
  lifetime object with `true` across mount/unmount: both lost user-cancellation ownership and
  let a cancelled first height escape through a deferred band write.
- Tests: assert behaviour (scroll writes, screen y, DOM order), not source-string equality
  (the branch's `TestBoardKeepWiring` failed on an exact `if (kept) f.wbKeep = false;` match).
  Use the live document clock in harness timing; no fixed 100-500 ms thresholds without
  tolerance; retain page-error stacks.
- The branch's retained failing `history-cards` check (659 px vs exact 640 px) is a separate,
  unresolved item; do not attribute it to this work, and report it as not rerun if untouched.
- Moving a node reloads frames where `Element.moveBefore` is missing: add no move beyond the
  release move of the held topic itself.
- Mirror/doc obligations from AGENTS.md rows for `agents-plugin-tool/` and
  `agents-plugin-tool/internal/`.

## Phases

Reuse map. Reusable pure logic and tests on `impl/develop/perm-unlit-icon`:
- `308f6c5ce`, `6ae3b64d8` and `board_hold_test.go`, `board_hold_review_test.go`: placement with holds,
  start decision table, lifecycle, batching, held pagination.
- `6369d1ecd` cards.js hold map + `order()` override (about 42 lines): reuse the idea and the
  weave integration; rewrite against current cards.js (1484 lines now).
- `e8b32ec48`, `0520c1301`: closing hold in weave, closed-page release base
  (`WsCards.closedPage`), whitespace-tolerant `order()`, `board_close_hold_test.go`,
  `board_close_fold_test.go` additions. Highest reuse value.
- `7f9d99736`, `173b13826`, `6ae3b64d8`: `scripts/board-headless.mjs` scenarios `hold`,
  `hold-selection`, `hold-batch`, `hold-hidden`, `hold-blur`, `hold-page`, `hold-maximize`,
  `hold-close`, `hold-reopen`, `serve --closed 65`, plus `board-headless.test.mjs`. Port onto
  develop's `scripts/board-headless/` layout.
- `ca025566b`: indicator CSS and 4 strings x 11 tables. Reuse nearly as is (move CSS to
  `dashboard-board.css`/`dashboard-layout.css`).
- `32a27c0b9`: the ResizeObserver wrapper and lifetime fix and its regressions
  (`board_frame_bands_test.go`, `view_kit_test.go`); the idea is reusable, the code is not
  (keep helper has since been rewritten).
- `00423ea6c`, `60f18c458`, `19c09f0d1`, `1afb2c139`: Phase 4 unification; DO NOT reuse (the
  dual-writer attempt that failed).
- Rewrite: everything touching `dashboard.js` board/keep code, `stream.js` keep, `topicResized`.

### Phase 0: owner decisions and gap survey (no code)
O1/O2 answered by (1100)=a. Run develop's viewed-panel staging on a seeded fixture (isolated
`node scripts/board-headless.mjs serve` or a disposable Go/Chromium fixture; never the shared
daemon) and record which of these already work on develop: rise-hold, closing hold, release
batching, and at-top freeze (Decision (1100)=a: visible layout unchanged at `scrollTop <= 4`).
No product code changes in this phase; scratch harness files are deleted or kept out of the commit.
Verification: a short table recorded in the Phase 0 Result (behaviour, covered by develop yes/no,
evidence screenshot path and scenario steps), screenshots at 1440 px. The run stops after Phase 0
and reports; Phase 1 waits for the confirmed mockup (Decision 11).

### Result (5314fb8e7) - 2026-11-15

Survey only; the heading identifies the inspected develop revision. Execution HEAD
was `226f466f4` on `exec5/reorder-hold`, whose only delta from develop was this
ticket. No product code or committed tests changed; the ticket remains in `todo/`.

Evidence root: `/tmp/exec5/reorder0/` (all screenshot paths below are relative to
it). A disposable Go overlay modelled on `board_viewed_live_browser_test.go`
served the production renderers/assets through `httptest.NewServer` on private
Port 0 origins, without constructing a daemon. It seeded 25 open and 65 closed
topics through `wswhiteboard.Apply`, then sent server-rendered updates/closes over
its own WebSocket. Instrumentation only observed morphs, scroll setters and
staging state. Chromium captures are 1440x900, light, reduced motion; rise was
checked on both adapter (`#bcol`) and board-only (document) scrollers, other cases
on the adapter. These are survey observations, not acceptance of later phases.

| Behaviour | Covered by develop | Evidence | Gap for later phases |
|---|---|---|---|
| Rise-hold | **partial — VERIFIED** | `rise-before.png` → `rise-after.png`: scroll T04 to y=150; server appends a log and canonically moves T04 to head. Old content stays staged, but the frozen T04–T06 interval moves to head and T04 ends at y=108 (scroll 1346→0; requested correction -42 clamps). `rise-document-before.png` → `rise-document-after.png`: y=150→106. `supplement/rewrite-before.png` → `supplement/rewrite-after.png` reproduces with a panel rewrite. | Preserve the actual open-flow slot and absolute screen y, not just relative order within the exposed interval; avoid the head/clamp failure. |
| Closing hold, including closed-page accounting | **yes, staged variant — VERIFIED** | `close-before.png` → `close-held.png`: expand the first closed page, return to T04 at y=150, then server-close it. T04 stays open/staged at y=150 with unchanged open-flow order. The held page's topic list equals the offscreen-close control (`close-control.png`): 31 fetched topics and `data-closed-next=61` in both (60 before close). `close-released.png`: explicit apply admits the closed state. `supplement/close-reopened.png` and JSON: reopen restores the page offset to 60. | Decision 1's folded closed rendering is not shown during this hold: the old open rendering is retained. Reopen hits the rise/clamp gap above; past-cap hold eviction was not surveyed. |
| Batched release and deferral gates | **partial — VERIFIED** | `batch-pending.png` → `batch-applied.png`: T04 updated twice, then T05; one click drains all pending changes in one morph and one scroll correction. `automatic-before.png` → `automatic-after.png`: scrolling away also drains them in one morph. However `close-held.png` → `close-released.png` has one morph but two corrections, the second from late frame heights. `supplement/report.json`, `gate-*` scenarios/screenshots: editing and overview defer morphs and retry on blur/exit; selection, a focused topic button, hover and simulated `pageHidden()` do not prevent an already-pending scroll-triggered release. | Complete the involved-source/destination-card eligibility and gate/retry rules; do not treat one morph as proof of one total scroll correction. **INFERRED:** current release scheduling is scroll/resize+rAF restaging, not an involved-card observer lifecycle (`agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js:1153–1181`). |
| At-top freeze, explicit apply only | **no — VERIFIED** | `top-visible-rise-before.png` → `top-visible-rise-after.png`: a visible T01 rise happens to remain staged at scroll 0. But `top-offscreen-rise-before.png` → `top-offscreen-rise-after.png` (server updates T20) inserts it at head immediately and changes scroll 0→380; `top-new-before.png` → `top-new-after.png` inserts NEW and changes scroll 0→165. A pending bar appears, but insertion already happened. `supplement/top-held-before-scroll.png` → `supplement/top-held-auto-applied.png`: even the initially staged top-origin update later releases by scrolling away, without clicking apply. | Add the explicit-only top freeze policy, including stable layout/head insertion and pending signalling. Ordinary visible-interval staging is not that policy. |

Pending/apply surface today:

- **VERIFIED:** the fixed bottom button reads `최신 변경 N건 반영 · <names>`;
  the two T04 revisions show N=1 then N=2, and the T05 revision makes N=3
  (`batch-explicit-and-count` in `report.json`). N is not distinct topics. The
  listed pending names can include many unchanged, placement-affected topics
  even when N=1 (`rise-after.png`). Click force-applies the pending snapshot;
  it is not the planned merged home/nav pill (batch apply ended at scroll 411,
  not 0).
- **INFERRED from source:** the numeric value is `boardChanges.length`, a ledger
  of immutable previous/incoming topic-hash changes plus an order-only fallback,
  not a count of mounted DOM nodes. Live exposure/placement builds `boardPending`
  and filters which ledger entries survive. See
  `agents-plugin-tool/internal/wsdashboard/assets/dashboard-board.js:1004–1017,1111–1134`.
  **VERIFIED:** close→reopen also produced three ledger entries, T04/T04/C35,
  because the capped-page transfer participates (`supplement/report.json`);
  N is not simply the number of server events either.

Surprises and limits:

- **VERIFIED:** close staging already protects the sampled reader and pagination;
  closing-hold work should reuse it rather than add a second placement/scroll owner.
- **VERIFIED with simulated visibility:** a newly arriving hidden-page event is
  deferred until show (`supplement/hidden-incoming-before-show.png` →
  `supplement/hidden-incoming-shown.png`), unlike release of an already-pending
  update while `pageHidden()` is true. Actual background-tab throttling was not
  tested. Overview defers/retries, but the bottom pending bar remains visible.
- **INFERRED:** exposure accounts for sticky heads but requires at least 1 px
  intersection; subpixel partial intersections need later verification
  (`agents-plugin-tool/internal/wswhiteboard/assets/stream.js:447–473`).
- **VERIFIED:** all 25 browser scenarios completed with zero page/scenario errors
  (`browser.log`, `supplement.log`); `verification.log` checks the observations,
  including the failures above. Eight focused `TestBoardViewed*` tests and five
  focused closed-page/frame-batch tests passed (`viewed-tests.log`,
  `closed-tests.log`). No whole-package test run. `history-cards` and WebKit were
  not rerun; 700 px/dark/normal-motion acceptance remains for later phases.
  The rejected hand-seeded pilot is retained under `pilot-invalid-seed/` and is
  not evidence for this result; final fixtures use normal server commands.

No new owner decision is needed for Phase 0. Phase 1 still waits for Decision 11's
confirmed merged-pill mockup; this survey does not authorize implementation.

### Phase 1: indicator and badge, visible first
Accent, tags, nudge, and the merged nav-pill board-updated state (Decision 11) wired to develop's
`boardPending` from data; i18n keys; reuse `ca025566b` for the accent/tag CSS and strings. Click on
the ↑ or the text = existing `boardFlushViewed(true)` path plus scroll to top per Decision 6.
Bottom "최신 변경 N건 반영" bar removed; its pending set feeds the pill.
N counts distinct pending topics (Decision 6). Phase 0 found develop's bar counts ledger entries
(`boardChanges.length`: T04 updated twice = 2); the pill must show 1 for that case.
Verification: `go test ./internal/wsi18n/...`; browser tests (Go-driven .cjs, like
`board_viewed_live_browser_test.go`): ↑ present with nothing pending (home button) after scrolling
down; "보드 업데이트 N" state shown at the top with pending changes and the visible layout
unchanged until click; bottom bar absent; same topic updated twice shows N=1, two topics N=2;
answered/open chips present together with board updates render as small count bubbles after the
text; click releases in one settle and lands at `scrollTop` 0, no keep session during the to-top
scroll; reduced motion has no sway/nudge and instant scroll; zero mount churn on a static viewport;
hidden in overview. Owner looks at captures at 1440/700 px, light/dark, reduced motion, each
compared side by side with the matching state of
`ai-docs/tickets/assets/261115-feat-ws-dashboard-board-reorder-hold-rewrite/nav-capsule-accepted.html`
(Decision 11; replaces the v2 mockups as the badge reference).

### Phase 2: rise hold gaps
Only what Phase 0 shows missing: visible topic rising holds in place, releases when all involved
cards are off-screen, batched, deferrals (selection, focus, hover, hidden, overview, maximize).
Reuse hold tests (`308f6c5ce`) as a spec; implement through the develop staging path.
Verification: headless `hold`, `hold-selection`, `hold-batch`, `hold-hidden`, `hold-blur`,
`hold-maximize` on adapter (`#bcol`) and board-only (document) pages; drift under 1 px; one settle
per batch.

### Phase 3: closing hold
Decision 1; reuse `e8b32ec48`/`0520c1301` logic and tests.
Verification: `board_close_fold_test.go`, `closed_route_test.go`, `board_close_hold_test.go`; headless
`hold-close`, `hold-reopen` with closed pages expanded: no duplicate or missing topic,
`data-closed-next` equal to the no-hold control (branch measured 61 after close, 60 after reopen),
past-cap eviction drops the hold.

### Phase 4: momentum guard, one-writer audit, matrix
Momentum guard (about 120 ms quiet), user-ended session never restarted by a late frame height,
ResizeObserver wrapper rule, one-writer audit (no duplicate scroll writes for keep and band
corrections).
Verification: node regression delivering ResizeObserver callbacks in native shape
`(entries, observer)`; headless `keep` cancellation case on adapter and document scrollers, recording
setter stacks and write counts (the branch failure: user scroll then 4025 -> 4004, script writes 8 -> 9);
no write during continuous scrolling; WebKit not run unless stated.
