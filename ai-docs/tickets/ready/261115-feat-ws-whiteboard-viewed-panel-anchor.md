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

## Implementation Checkpoint — 2026-11-15 (incomplete)

**VERIFIED:** Work remains on `impl/develop/bling-fog-scorn`, based on
`72df121fcadb5e4e027b84c15474e402f9adfc34`. The merged (789) frontier was
confirmed. The unlanded (793) move map was read for coordination only; no split
commit, source repin, or change to its scheduling boundary was imported.

- `2f8f34a82`: short, nested reading-line transactions replace ordinary keep
  sessions; persistent scroller owners handle input/cooldown and retain explicit
  jump priority. History clipping and intentional navigation use that owner.
- `ce6ad8116`: dashboard rendering, relative placement, and frame-height staging
  share a bottom pending-change bar. Main/pinned batches pre-capture through the
  owner; owned scrollers skip numeric and topic-resize fallback writers.
- `48764f937`: fetched closed-page transfers and inline pins are included.
  Eligible fetched-panel rendering can update in place while placement remains
  staged. Existing server-rendered owner receipts admit only owner rows when
  a concurrent same-topic snapshot also changes external panels; a local send
  alone grants no blanket topic exemption. Local memo/sent-state paths remain
  immediate. The exposed viewport includes rail clearance and horizontal clips.

**VERIFIED deterministic evidence at `48764f937`:** Both asset syntax checks
passed. From `agents-plugin-tool/`, with `GOTOOLCHAIN=local GOPROXY=off
GOSUMDB=off`, these commands passed under individual 120-second outer caps:

```sh
go test ./internal/wswhiteboard -count=1 -timeout=90s -run '^(TestViewKit.*UnderNode|TestHistoryScriptPanelChunksAndSwapUnderNode)$'
# ok: 0.897 s
go test ./internal/wsdashboard -count=1 -timeout=90s -run '^(TestBoardKeep.*|TestBoardFrameHeightsBatch|TestBoardTopic.*|TestBoardFramesMountLazily|Test.*Viewed.*)$'
# ok: 11.781 s
```

Initial focused failures were diagnosed as stale test doubles/wiring or a new
case retaining an earlier offscreen setup, then corrected without restoring the
superseded production writers. Each bounded command's final process-group
inspection found no survivors. These deterministic tests do not establish real
all-visible browser geometry.

**STOP (f), browser fixture non-convergence:** The new opt-in
`TestBoardViewedBrowser` and `testdata/viewed-panels.browser.cjs` in
`302e6e8c0` are an **UNVERIFIED checkpoint**, not acceptance. Two bounded runs of
`go test ./internal/wsdashboard -count=1 -timeout=150s -run
'^TestBoardViewedBrowser$' -v` failed before any current-product geometry
predicate ran. Run 1 found zero exact inline `stream.js` occurrences during
baseline substitution. Run 2 still recorded the missing stream prefix and
then timed out at the 8-second startup wait. The root cause is **INFERRED** to
be fixture interception/startup, not a demonstrated product defect. No third
attempt was made.

Full commands, environment, outputs, diagnostic capture and independent owned
process cleanup evidence are under
`ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/browser/`:
`run-1.log`, `run-2.log`, `asset-mismatch.json`, `failure.json`, `failure.png`,
`results.json`, `browser-cleanup.json`, and `supervisor-{1,2}.json`. Both outer
supervisors and the browser cleanup record have empty final process lists.
`failure.png` is a startup diagnostic, not a successful baseline/result capture.

**Pending:** Repair and independently verify the offline browser fixture,
complete the matching baseline/result geometry and captures, and run the route's
allocated one-pass lite review. No Phase 1 Result, closure, merge, push, shared
endpoint/daemon action, provider/holder use, installation/download, or deployment
is claimed. MAIN owns continuation and integration.

## Authorized Continuation Checkpoint — 2026-11-15 (incomplete)

MAIN 00:23:47 approved the shell-aware, fully pinned fixture proposal and
exactly one fail-fast offline batch. The lead initiated integration of
`5585844a794a031a770ce4171c5e05daf1bd3353`; this continuation completed that
merge as `7589b5910fc50759fa8524f267b5e0209f491bad`, preserving every incoming
path and both send receipt matching and incoming conversation follow. Static
semantic reconciliation retains independent Files ownership, the one-row
`.mhead > .history-older` header, always-shown history/system-chip folds,
30-line/3-panel/1000px budgets, `positionHistory` geometry, and recap navigation
through the explicit jump owner.

Pre-freeze static inspection also corrected initial visible frame hydration
being staged as though it were an external update (`2a2a6d73d`); swapped
`wbKeep` frames still defer their first height. Its regression is authored but
not executed in this continuation. The paired fixture uses normal shell entry
and its active content iframe, separately compiled complete asset/template
bundles, identical fixture code, exact fixture-origin permission, and request/
response diagnostic retention. Frozen result:
`b0af755624cc13929f4a384145d5787d76e3dbdb`; strict baseline remains the exact
558 merge source. No preliminary executable test, build or probe was run.

**VERIFIED failed authorized batch:**
`browser-approved/freeze.json` records the pre-start pins/source/asset hashes
and bounds. The one command was:

```sh
python3 ai-docs/tickets/assets/261115-feat-ws-whiteboard-viewed-panel-anchor/browser-approved/run.py --result b0af755624cc13929f4a384145d5787d76e3dbdb --batch approved-001
```

It stopped on the first baseline build, before any result build, binary,
fixture server, browser or HTTP request. Complete compiler output:

```text
# github.com/kang-sw/devenv/internal/wsdashboard [github.com/kang-sw/devenv/internal/wsdashboard.test]
internal/wsdashboard/gitbadge.go:410:6: signal already declared through import of package signal ("os/signal")
	internal/wsdashboard/board_viewed_browser_test.go:9:2: other declaration of signal
```

This is a fixture import-name collision, not a product/browser verdict. The
supervisor returned 1 after 1.87 seconds; no cap expired. Bounds were 180 seconds
per build, 180 seconds browser, 8 seconds browser actions, and 600 seconds overall
including a 30-second cleanup reserve. No fix, second batch, review or closure
followed failure. `browser-approved/approved-001/` retains `RESULT.md`, full raw
outputs, source/asset SHA-256 manifests and immutable local baseline build-input
archives. No executable was produced to hash or retain. Its independent
`cleanup.json` has `final_ps: []`, no cleanup signals and no errors. Existing
failed `browser/` evidence remains unchanged.

**Final integration advisory, not included in this batch:** MAIN's latest gate
is develop `1612e099c`, superseding `088ecf679` and `6db68ec00`. Before final
integration, the lead must reconcile transient board-read/gone handling and
changed internal Go signatures, topic-state overdue nudges, wsinbox selections,
and dashboard system-open menu/i18n changes plus Tauri zoom. MAIN explicitly
kept this one batch on baseline 558; no silent repin, restart, extra execution,
shared-runtime action or final integration is claimed. Further execution and
any verification beyond the approved budget need MAIN disposition. Phase 1
remains incomplete, with no Result or closure.

## Minimal Fixture Continuation — 2026-11-15 (incomplete)

MAIN01:01:56 authorized only aliasing the Go fixture's `os/signal` import and
its required references, minimal local compile/vet gate wiring, and one
`approved-002` batch after a green gate. Commit `a57290575` changes the fixture
blob from `c073da3b0` to `bff43320c` only for `ossignal`, `ossignal.Notify` and
`ossignal.Stop`. The driver/geometry/assertions/timing retain SHA-256
`c109f426e58e7e97a65aa9ce84e9bff6c34f8f92f41bd87cb9645b713aed92e9`.
No unrelated correction, silent repin or prior-artifact rewrite was made.

**VERIFIED green compile-only/vet preflight:** The exact Go 1.26.2 executable
was resolved from `approved-001/cleanup.json`'s retained compiler command,
without a version probe. With fixed `GOROOT` and offline environment,
`/opt/homebrew/Cellar/go/1.26.2/libexec/bin/go test -c ./internal/wsdashboard -o
<side.test>` compiled BOTH baseline-558/result-a572 snapshots; the same
executable ran `go vet ./internal/wsdashboard` in BOTH. All four commands exited
0 with empty full outputs. No server/browser launched during preflight.
`browser-approved/preflight-002/manifest.json` records exact cwd, commands,
executable/toolchain/source/asset hashes and both retained binaries. Elapsed
19.115 seconds including cleanup; independent final process list empty. This
required gate is fixture-local, not a shipped/global workflow rule.

**VERIFIED failed approved-002:** After reporting
`browser-approved/freeze-002.json`, the single batch reused the exact green
preflight receipt and unchanged baseline 558/result a572. Both builds exited 0;
both disposable fixture servers reached ready and returned 0 during cleanup.
Baseline 1800 startup then reported:

```text
Error: page script: Failed to read the 'serviceWorker' property from 'Navigator': Service worker is disabled because the context is sandboxed and lacks the 'allow-same-origin' flag.
```

No behavioral check or comparison ran (`checks: []`). Normal shell/content
frame documents returned 200 with baseline asset identity `f5d4d7a241df28e6`;
request URLs, frame identities, response headers/body prefixes, complete raw
stack and diagnostic screenshot are retained in
`browser-approved/approved-002/`. Underlying script attribution remains UNKNOWN;
this is not a product-contract verdict. The batch returned 1 after 12.519
seconds including cleanup. No cap expired; no repair/retry/review/closure
followed. Outer and browser cleanup records have empty final process lists,
no cleanup signals, and no errors.

**Unexpected side effect, explicitly unresolved:** Outer `cleanup.json` tracked
PID 58444 `/Users/kang-sw/.local/bin/codex app-server`, parent PID 58001 (the
baseline fixture). This was not requested by the worker/driver and violates the
intended no-provider-process isolation; do not claim that boundary held. Source
attribution and external provider/network activity are UNKNOWN. No further probe
or runtime action investigated it. Independent final process evidence confirms
that the tracked process was absent after cleanup.

`approved-002/RESULT.md` records actual commands, exits, all four retained binary
hashes (preflight and batch), artifact paths, and limitations. Exact source/asset
manifests and immutable local archives remain, as do all ten original untracked
artifacts and approved-001 evidence. Preserve the dirty worktree/artifacts before
release. Further execution requires MAIN disposition; final lead-owned sync of
`1612e099c` is still pending. Phase 1 has no Result and the ticket remains ready.
