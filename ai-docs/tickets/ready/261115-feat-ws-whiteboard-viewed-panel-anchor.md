---
title: "Preserve the viewed whiteboard panel with one scroll owner"
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: "Coordinate implementation with MAIN's cut line and use its published move map"
  261113-feat-ws-dashboard-board-reorder-hold: "Owner (818)=a supersedes immediate placement of visible cards and the older ordinary anchor/session strategy; preserve unrelated behavior"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: d1800d862382a36f
sage-review-completeness-reviewed: d1800d862382a36f
---

# Preserve the viewed whiteboard panel with one scroll owner

## Background

MAIN relayed owner choice (800)=a on 2026-11-14 UTC: capture the single-rule
viewed-panel policy from section 4 of `viewed-panel-anchor-heuristic.md` as
accepted backlog. MAIN subsequently relayed owner authorization to begin
implementation on 2026-11-14 at 22:58 UTC, with parallel execution allowed and
no heavy-slot wait. Complete normal readiness gates before implementation. The
untracked root-worktree draft `261114-feat-ws-whiteboard-center-reference-scroll`
and tracked idea `261114-feat-whiteboard-center-reference-navigation` concern
reference-centering behavior and are not changed or absorbed here. The isolated
implementation worktree need not contain the root's untracked draft.

The supporting static report inspected develop
`a332217477b18a721a2857adc1a53bbe020e7192`. It found that ordinary board
preservation selects a surviving topic/card top rather than the inner panel the
user is reading, while history, frame-height, numeric restoration and navigation
paths have distinct compensation rules. Source inspection did not reproduce or
diagnose the owner's observed jump. Later develop changes, including 773 and
789, must be reconciled at implementation rather than treating that snapshot as
the current tree.

The approved contract is reproduced below so implementation does not depend on
the session-local report remaining available. MAIN relayed owner-confirmed
(802)=a on 2026-11-14 UTC: topic latest-change pull-up reordering and offscreen
panel height changes must leave already visible panels looking frozen. MAIN
then relayed owner-confirmed (803)=a: defer pending rendering changes to panels
intersecting the viewport, with an explicit bottom badge or fully-offscreen
application; the owner's own answers and memos apply immediately. This replaces
(802)'s earlier allowance for immediate changes inside visible panels, not its
external-change preservation goal. This is not a new reference-centering or
closed-topic-reveal feature.

## Decisions

- **Golden rule — owner-confirmed (802)=a.** Compensate topic latest-change
  pull-up reordering and offscreen panel height changes so panels already in
  view retain their visible positions. Preserving only the selected reading
  reference is insufficient if another visible panel moves because of those
  external changes. Apply this ordinary-preservation rule within the user-input
  and explicit jump-follow priorities below, not as another competing scroll
  writer. Visible-panel rendering follows the later confirmed (803)=a rule.
- **Defer visible-panel rendering — owner-confirmed (803)=a.** Any panel
  intersecting the exposed reading viewport by at least 1 px with a pending
  change keeps its current rendering. Show a bottom badge labeled
  `최신 변경 N건 반영`, with the pending-change count and topic name. Apply the
  pending rendering on badge click or when that panel fully leaves the
  viewport. On fully-offscreen application, use the same offscreen compensation
  rule so remaining visible panels do not jump. The owner's own answers and
  memos apply immediately, including inside the viewport. This supersedes
  immediate minimal-shift rendering for other visible-panel changes under
  (802); reject a one-third-viewport shift threshold or reading-line-only
  deferral as the eligibility rule. The reading line below selects the scroll
  reference, not which visible panels receive render deferral.
- **Defer visible-card placement — owner-confirmed (818)=a.** MAIN relayed
  the owner's verbatim answer `818,822=a` on 2026-11-14 at 23:13 UTC; only
  item (818) belongs to this ticket. Stage reorders among visible cards as well
  as rendering changes into the same bottom `최신 변경 N건 반영` bar. Release
  the staged placement on bar click or when the affected card fully leaves the
  exposed viewport. The owner's own input applies immediately. Preserve the
  all-visible golden rule and one scroll writer; do not apply incompatible
  relative-placement changes and then try to repair them with one scroll delta.
  This explicitly supersedes the older board-reorder ticket's immediate-move
  rule when the destination is visible. Server ordering remains authoritative
  once staging releases; no server reorder policy changes. Rejected: immediate
  visible-card reorder with a relaxed preservation guarantee (option b).
- **Supersede the older preservation strategy, not its unrelated behavior.**
  The approved reading-line reference and short mutation transaction replace
  the topmost-survivor selection and long-lived ordinary keep sessions from
  `261113-feat-ws-dashboard-board-reorder-hold` Phase 1. The later (803)=a
  visible-render deferral supersedes that ticket's immediate visible-content
  update expectation, and (818)=a supersedes its immediate visible-card
  placement rule. Preserve unrelated placement rules, visibility subscriptions,
  history membership and explicit navigation semantics; do not retain the
  superseded writers alongside the new owner.
- **One writer per actual scroller — (800)=a.** Use one owner with states
  `idle`, `user-scrolling`, and `explicit-jump-follow`. Normal preservation is
  a short mutation transaction, not a competing long-lived target selector.
  Main-board and independently scrolling pinned columns use their own owner
  instances and the same selection rule. Reject layered competing anchor
  heuristics or independent correction writers on an owned scroller.
- **One exposed reading viewport.** Use the actual scroller's content box
  clipped to the window, accounting for published toolbar, rail and sticky
  clearance. Select one stable panel/article crossing a reading line just below
  that clearance. If the line is in a gap, select the next intersecting panel.
  Reject a barely visible earlier topic or largest-visible-area heuristic as a
  competing fallback rule. Save stable identity and the reference's screen
  offset. An identifiable inner content point near the line is optional when
  needed for internal-content preservation; preserving only a panel's outer
  top does not establish that its internal text stayed fixed.
- **Capture before mutation; correct once.** Synchronously capture immediately
  before each known layout mutation or batch, including board morph, history
  clipping changes and iframe-height batches. Resolve that same reference
  afterward and apply one instant `scrollTop` delta through the single writer.
  If the reference is gone, skip correction rather than silently choosing a
  different reading target. Fold the existing `topicResized` and `restoreScroll`
  compensation for an owned scroller into this owner; do not leave independent
  compensation running beside it.
- **User input wins.** Wheel, touch, pointer and scroll-key navigation intent
  immediately cancels preservation or explicit jump following. Suppress normal
  capture until 200 ms after the last user-associated scroll as the approved
  starting cooldown, extending through momentum. Distinguish and ignore the
  manager's own writes. Updates during this interval may move content, but
  must not tug against active user navigation. This is a starting value, not a
  measured optimum or permission to silently tune the policy during acceptance.
- **Compose with (789), not against it.** The approved badge/navigation target
  has priority in `explicit-jump-follow` for at most 2000 ms. Preserve its
  instant corrections, early exit after at least five stable frames and 150 ms,
  and reduced-motion treatment of initial navigation. Normal mutation capture
  must not replace its target. User intent cancels it and starts the same
  cooldown. On settle or cap, discard the jump target and return to ordinary
  capture without restoring a stale pre-jump offset. Do not restart its cap
  on each mutation or resize.
- **One manual/native anchoring policy.** Keep `overflow-anchor: none` on every
  scroller this manager owns. Do not combine browser-native and manual
  compensation on a managed surface. A standalone scroller may be described
  as covered only if it is actually brought under this same owner and policy;
  this ticket does not establish existing standalone coverage by assertion.
- **Compatibility: T0, frontend asset content and focused tests only.** No MCP
  tool/argument, API, protocol, server behavior, tracker event, store shape or
  compatibility-schema change is authorized. Reject a server-side workaround
  or new cross-package workflow interface for this UI behavior.

## Constraints

- **Implementation authorization.** MAIN's 2026-11-14 22:58 UTC relay lifts
  the earlier capture-only restriction and authorizes this implementation.
  Populate facts and apply normal readiness gates before source changes.
  Owner-approved parallel execution removes the heavy-slot waiting gate;
  plan finite verification commands and process ownership before execution.
  Shared-daemon actions, deployment and release remain MAIN-owned.
- **Implementation requires (789) to have merged first.** This is MAIN's direct
  item, not a ticket. Its capture-time source pointer is `e0ed9ff8b` on
  `impl/develop/pending-jump-follow`, owned by MAIN. A source commit or proposed
  test slot is not proof it landed. Confirm the actual merge frontier and
  preserve the approved jump-follow contract rather than reimplementing it.
- **Respect (793)'s cut line.** The related asset-split ticket is owned by
  EXEC1. MAIN's 2026-11-14 22:58 UTC relay explicitly lifts the scheduling
  hold on this implementation. Confirm the actual integration baseline and
  consume the published region-to-file move map (pending 261114-refactor-ws-dashboard-whiteboard-asset-split) instead of pinning work to
  stale monolithic-file positions or making this feature part of the mechanical
  split. Coordinate any remaining source interaction with MAIN/EXEC1; that
  refactor remains behavior-preserving.
- Before editing `agents-plugin-tool/internal/`, read
  `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/release-compatibility.md`. Keep the change within downstream
  asset boundaries. If implementation reaches skills, playbooks, prompts or
  conventions, stop before expanding this approved asset-only scope.
- Reuse the existing scroller, stable entry identity, geometry and navigation
  mechanisms where appropriate. Capture-time code evidence is in
  `wsdashboard/assets/dashboard.js` (`boardView`, `boardKeep`,
  `boardKeepSettle`, `applyFrameHeights`, `topicResized`, `restoreScroll`),
  `wswhiteboard/assets/stream.js` (`keep`, history clipping/navigation), and the
  existing rail/overview/pill callers and anchoring CSS. Locate their actual
  destinations after the split; this list is not a mandate to edit every file
  or to introduce an exported module API.
- Preserve existing history membership and selection policies, disclosure and
  frame identities, focus behavior, and navigation target semantics. Optional
  inner references must not require reading an opaque child frame through a
  forbidden parent/child DOM boundary. Client-side placement staging is
  explicitly authorized by (818)=a; do not change server ordering, clipping
  policy, unrelated reorder behavior or animation design as an incidental fix.
- Source claims, deterministic harness results and real browser geometry are
  different evidence. No approved screenshot/mockup is supplied for this
  behavior; later acceptance compares the actual implementation baseline and
  result, not an invented visual design.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/release-compatibility.md (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- e0ed9ff8 (2026-11-14, commit): "Reuse the existing per-scroller keep lifecycle so board-swap anchoring cannot steal or end an explicit jump" — bearing: constrains
- 261113-feat-ws-dashboard-board-reorder-hold (2026-11-14, Result/Decisions): "boardKeepSettle picks the anchor after the morph with boardAnchor" and a visible destination moves immediately — bearing: constrains; the ordinary anchor/session and immediate visible-card placement strategies are explicitly superseded by (800)/(818) above.
- 5695371f (2026-11-13, commit): "Reuse board SSE regions and trusted frame metadata; materialize inert older/closed history only for known targets and preserve viewport anchors." — bearing: constrains
- 261113-feat-ws-whiteboard-topic-history-load-more (2026-11-13, Decisions): "No inner scroll; \"load more\" above the latest window" — bearing: constrains
- 261113-feat-ws-whiteboard-topic-panels-and-log (2026-11-13, Decisions): "Panels are append-only versions (207=b): panels are rewritten only as a full rewrite via ID override: an existing panel with the same ID folds." — bearing: constrains
- 184e0755 (2026-11-15, commit): "Equal content must have equal stuck/unstuck height; actual recap changes may legitimately change header height." — bearing: constrains
- 261114-bug-ws-whiteboard-fold-reopens-on-panel-republish (2026-11-14, Decisions): "a user fold follows stable panel identity across republishing, and the updated panel receives a conspicuous updated badge instead of reopening" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin-tool/internal/wsdashboard/assets/dashboard.js and anchoring CSS; agents-plugin-tool/internal/wswhiteboard/assets/stream.js; focused tests. The asset-split ticket is ready without a Result or published move map on this tree. |
| scope.surface | cross-module | window.wbHistory.keep/keeping/jump in agents-plugin-tool/internal/wswhiteboard/assets/stream.js#L611-L745 are consumed by dashboard board/history/frame navigation. T0 assets only. |
| scope.new_public_symbol | no | No exported module API or external tool/API change is authorized; reuse existing wbHistory and navigation entry points. |
| scope.new_type_contract | yes | Phase 1 introduces per-scroller idle/user-scrolling/explicit-jump-follow states and pending-rendering/count state with an owner-input exception; these are frontend-only contracts. |
| scope.test_surface | existing | agents-plugin-tool/internal/wsdashboard/board_keep_test.go; agents-plugin-tool/internal/wswhiteboard/view_kit_test.go (TestViewKitUnderNode, TestViewKitJumpFollowUnderNode); scripts/board-headless.mjs. |
| complexity.reuse_points | confirmed | Read boardView/boardKeep/boardKeepSettle, restoreScroll, topicResized and applyFrameHeights in dashboard.js; history collapse/navigation and viewKit keep/jump in stream.js. git merge-base --is-ancestor 5573bce92 HEAD succeeds: (789) is merged on this tree. |
| complexity.side_effect_risk | high | Replacing scroll writers and deferring visible rendering changes asynchronous board morph, history clipping and frame-height behavior on independent scrollers. |
| risk.correctness | high | One delta must preserve multiple visible panels across reorder and offscreen height changes while honoring cancellation, cooldown and explicit-jump priority. |
| risk.fit | high | The new reading-line/short-transaction policy replaces the older topmost-survivor/long-lived keep contract; the ready asset split has no landed move map here. |
| risk.test | high | Existing fake-window/source-extraction tests do not establish real multi-panel geometry, 1 px viewport intersection, momentum or frame rendering preservation; matched browser evidence is required. |
| risk.security_or_contract | moderate | Preserve trusted frame identity and opaque-frame boundaries while introducing visible-render deferral and immediate owner-input exceptions; dashboard.js#L4140-L4152 validates height-message identity. |

## Phases

### Phase 1: Unify viewed-panel preservation under the approved scroll owner

After (789) has landed, the (793) scheduling boundary is clear and MAIN grants
implementation, pin the actual integration baseline. Trace the remaining
compensation callers on that tree. Implement the one per-scroller owner and
selection rule above, with synchronous pre-mutation capture and a single delta
per transaction. Route known board, history and frame-height mutations through
it, fold the old fallback writers into it, and compose with the landed explicit
jump-follow state instead of creating a second navigation authority. Apply the
(802)=a golden rule to latest-change pull-up reorder and offscreen height
changes, and implement the confirmed (803)=a pending-rendering and bottom-badge
behavior for every viewport-intersecting panel, including the owner's immediate
answer/memo exception. Stage visible-card relative placement under (818)=a
instead of applying incompatible reorders. Coordinate badge-click and
fully-offscreen application with the same scroll owner rather than adding
another correction writer. If a
single compensation delta cannot preserve the affected visible panels on the
actual tree, report the conflicting geometry before expanding the approved
mechanism; do not silently weaken the golden rule to one reference.

Verification, using planned bounded runs under the owner-approved parallel
execution policy (no heavy-slot wait):

- Extend focused deterministic regressions for reading-line selection below
  sticky clearance, a line in a gap, a sliver of the prior topic, stable identity
  resolution after morph/reparent, and removal of the reference. A removed
  reference must produce no substituted-target correction.
- Verify one scroll writer/delta for each known mutation batch, including
  history clipping and delayed iframe-height changes, with no additional
  `topicResized`, numeric restoration or browser-native correction on that
  owned scroller. Test main and independently pinned scrollers separately.
- Verify the (802)=a golden rule for topic latest-change pull-up reordering
  and offscreen height changes above and below the viewport, including multiple
  visible panels. Compare each visible panel's before/after position, not only
  the reading reference. Distinguish external changes from the immediate
  owner-answer/memo exception and explicit badge application under (803).
  Do not infer all-visible preservation from one passing anchor sample.
- Verify (818)=a with simultaneously visible cards A then B where an external
  update would place B before A even though both destination slots are visible.
  Both positions and their relative order must remain unchanged while staged.
  Verify the same pending bar accounts for the deferred change, click applies
  it explicitly, and automatic placement applies only once the affected card
  fully leaves the exposed viewport while preserving the remaining visible
  panels. Exercise multiple queued reorders, main and pinned scrollers, and the
  immediate owner-input exception. Do not assert all-visible preservation for
  the explicit click or owner-input exceptions as if they were external updates.
- Verify (803)=a deferral for panels intersecting the viewport by exactly 1 px,
  for several visible panels, and for visible panels away from the reading
  line. Confirm current rendering stays unchanged while pending, and the bottom
  badge shows `최신 변경 N건 반영` with the correct count and topic name.
  Exercise badge-click application and automatic application only after the
  affected panel fully leaves the viewport; offscreen application must preserve
  remaining visible panels. Verify the owner's own answers and memos render
  immediately. Test these paths on main and independently pinned scrollers,
  including user scrolling/cooldown and the existing explicit-jump priority.
- Exercise wheel/touch/pointer/scroll-key cancellation, momentum and the
  200 ms post-scroll cooldown; manager writes must not be mistaken for user
  activity, and updates during user scrolling must not fight the user.
- Regress (789) target priority, instant corrections, five-stable-frame/150 ms
  early settlement, the non-renewing 2000 ms ceiling, cancellation and return
  to ordinary capture without a stale pre-jump restoration. Include initial
  navigation with reduced motion.
- Capture matching before/after browser geometry and screenshots using the
  same realistic board fixture and viewport. Include an inner panel in a
  large topic, growth above the reading reference, a disappearing reference,
  sticky clearance, a pinned column and a narrow viewport. Distinguish outer
  panel preservation from any optional inner-content-point guarantee. Verify
  focus, scroll intent, history membership and frame-origin boundaries remain
  intact. Report baseline limitations separately from new failures.
- Record exact commands, source pins, geometry assertions, actual captures and
  owned-process cleanup evidence. Test caps and process ownership are planned
  before execution; this capture authorizes no broad suite or live-owner probe.
  MAIN alone decides deployment and release.

## Sage Review Round 1 (2026-11-15)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | One delta cannot freeze panels whose relative positions change | critical | missing |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
