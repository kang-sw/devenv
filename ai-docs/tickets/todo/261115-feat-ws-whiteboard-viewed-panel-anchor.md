---
title: "Preserve the viewed whiteboard panel with one scroll owner"
related:
  261114-refactor-ws-dashboard-whiteboard-asset-split: "Coordinate implementation with MAIN's cut line and use its published move map"
---

# Preserve the viewed whiteboard panel with one scroll owner

## Background

MAIN relayed owner choice (800)=a on 2026-11-14 UTC: capture the single-rule
viewed-panel policy from section 4 of `viewed-panel-anchor-heuristic.md` as
accepted backlog. This ticket records that contract; it does not authorize
implementation tonight. The existing untracked
`261114-feat-ws-whiteboard-center-reference-scroll` draft concerns a different
reference-centering behavior and is not changed or absorbed here.

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

- **Ticket only for this assignment.** Remain in `todo/`. Authoring and merging
  this ticket allocate no executable slot and authorize no source change,
  browser/build/test run, release, or shared-daemon action. Populate facts and
  apply the normal readiness gates before later implementation.
- **Implementation requires (789) to have merged first.** This is MAIN's direct
  item, not a ticket. Its capture-time source pointer is `e0ed9ff8b` on
  `impl/develop/pending-jump-follow`, owned by MAIN. A source commit or proposed
  test slot is not proof it landed. Confirm the actual merge frontier and
  preserve the approved jump-follow contract rather than reimplementing it.
- **Respect (793)'s cut line.** The related asset-split ticket is owned by
  EXEC1 and starts only on MAIN's cut-line signal. Do not newly start this UI
  implementation while that split is active. Coordinate with MAIN/EXEC1 and
  consume the published region-to-file move map instead of pinning work to
  stale monolithic-file positions or making this feature part of the mechanical
  split. That refactor remains behavior-preserving.
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
  forbidden parent/child DOM boundary. Do not change clipping policy, reorder
  policy or animation design as an incidental fix.
- Source claims, deterministic harness results and real browser geometry are
  different evidence. No approved screenshot/mockup is supplied for this
  behavior; later acceptance compares the actual implementation baseline and
  result, not an invented visual design.

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
answer/memo exception. Coordinate badge-click and fully-offscreen application
with the same scroll owner rather than adding another correction writer. If a
single compensation delta cannot preserve the affected visible panels on the
actual tree, report the conflicting geometry before expanding the approved
mechanism; do not silently weaken the golden rule to one reference.

Verification, in a later explicitly bounded slot:

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
