---
title: "P3: Remove the hidden session-iframe cache"
sage-review-design: skipped
sage-review-completeness: skipped
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "P1 render transaction and P4 composer evidence; coordinate visibility/render integration"
  261113-feat-ws-whiteboard-board-frontend-performance: "landed board performance work; retain frame laziness, history and draft guarantees"
  261116-bug-werkschaft-electron-high-resolution-lag-diagnostics: "native presentation hypothesis remains separate"
completed: 2026-11-18
---

# P3: Remove the hidden session-iframe cache

## Background

Dashboard performance finding P3, 2026-11-16, size S/M. Cached non-visible
session pages retain substantial DOM/heap and whiteboard clock work. No visual
change is intended; this is a performance-only lifecycle/render refactor.

Evidence source read: `D:/Temp/dash-scroll-perf/SUMMARY.md`. Its isolated
headless cache ablation retained the same visible board:

| State | Nodes | JS heap | 8-second Task time |
|---|---:|---:|---:|
| One session | 27,806 | 3.62 MB | 2,886.2 ms |
| Five cached pages | 131,333 | 12.53 MB | 4,390.6 ms |
| Hidden frames pruned | 27,958 | 3.93 MB | 3,842.9 ms |

Pruning cut Task time by 12.5% and script by 7.4% relative to five cached pages;
style/paint worsened. Traffic/instrumentation/load differed: this demonstrates
secondary retained-resource cost, not a clean causal explanation of severe lag
or a precise per-hidden-frame CPU cost. Total DOM nodes are not clock-label
counts. No valid frame-time distribution was captured for these ablations.

`agents-plugin-tool/internal/wsdashboard/assets/dashboard-lifecycle.js:331-347`
retains cached pages and protects unsent work beyond the nominal five-page cap.
Both browser and Electron share this cache; it is bounded retention, not proof
of an unbounded leak.

### Verified timer audit — 2026-11-16

Code was read before writing this section, including timer starters throughout
the first-party wsdashboard/wswhiteboard assets. The owner's suspicion of
thousands of independent element timers is **not** what the code does:

- `agents-plugin-tool/internal/wswhiteboard/assets/ago.js:175-178` installs
  **one guarded one-second interval per document**, not one per entry. Opaque
  body frames load their own document copy (`:2-8`); repeated board swaps do
  not add another interval to the same window.
- Each `ago(document)` tick queries **all matching nodes in that document**:
  `time.tago[data-at]` (`:91-94`), `[data-body-age]` (`:95-101`),
  `time.entry-at[data-at]` (`:104-109`), and all `section.topic[data-topic]`
  state badges (`:110-111`). Body-age pinning skips that value's update, not
  the full query. `put` at `:15` avoids unchanged text writes; badge text is
  keyed at `:55-68`, but badge traversal/attribute parsing still occurs.
- There is **no document.hidden, shell-hidden, collapsed-details, viewport or
  IntersectionObserver gate** in this clock's tick/walk. Matching labels in
  collapsed sections and hidden topics are still visited; cached hidden session
  documents and retained child documents keep scheduled callbacks, subject to
  browser throttling. This is code behavior, not a measured thousands-label
  write count. Closed/invalid badges are removed, not used as a walk filter.
- `ago.js:10-13` already displays minute/hour/day granularity ("now" below one
  minute); entry labels become cached absolute clocks after one hour at
  `:102-109`. It still walks them every second. Tooltip/message-triggered calls
  at `:124,162` and board/conversation insertions can also invoke `wbAgo`;
  visibility handling must not break held body-age tooltips.
- Separately, `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js:114-141`
  installs a second document-wide one-second clock. It caches the matching
  relative/user-time and usage-pace node sets, invalidating on relevant DOM
  mutations (`:121-129`). `tickTimes` skips **work** for `doc.hidden || shHidden`
  (`:130-131`), but its interval is not cancelled. All matching cached nodes
  are otherwise visited, not just on-screen labels; `fmtTime`/`pace` skip
  unchanged text writes (`:63-112`). No collapsed-topic or viewport gate applies.
- `dashboard-lifecycle.js:357-377` sends shown/hidden to cached session frames;
  `:569-580` updates `shHidden` and calls `tickTimes` once on reveal. That state
  does not reach `ago.js` or opaque body-frame clocks. CSS hiding an iframe is
  not a reliable `document.hidden` transition for its child.
- Other periodic starters are distinct work: `usage.js:336-337` refreshes usage
  every five minutes behind document visibility; `webframe.js:28-32` is a
  heartbeat; whiteboard `live.js:43-53` polls a change stamp, not entry ages.
  Drag-edge intervals and one-shot UI timers are not relative-label schedulers.
  Existing IntersectionObservers in whiteboard `stream.js` and dashboard
  frame/git code do not gate either relative-time tick above.

Thus there are already shared intervals, but **two independent clock schedulers
in a dashboard document that loads both scripts**, plus clocks in child
whiteboard documents. Hidden and off-screen work, not per-element timer
multiplication, is the supported candidate. Actual matched-label/visited/write
counts per document remain to be measured.

## Reproduction

In an isolated profile, open one then five session pages without drafts. Record
DOM/heap, clock callback count, matching/visited/changed label counts and CPU
under identical traffic. Repeat with unsent drafts, collapsed sections, hidden
and off-screen topics, pinned/adopted topics, opaque body frames, CSS-hidden
cached sessions and a truly hidden browser tab. Reveal each once and check clock
freshness. Compare short-lived and old entries; do not infer visibility solely
from document.hidden or remove the owner's cached frames.

## Candidate fix directions

- Consolidate/coordinate relative and state clocks through one document-owned
  scheduler; register visible work and schedule its next actual label boundary.
- Propagate effective ancestor/shell/topic visibility to retained nested frames;
  use viewport observation and fold/topic lifecycle to exclude hidden labels.
- Cancel scheduled clock callbacks while hidden, refresh once on reveal, and
  coarsen cadence with age instead of scanning old labels every second.
- Remove the hidden session-iframe cache entirely under the owner's decision
  below. Improve fresh-load performance rather than retaining smaller or paused
  background session pages. Settle draft preservation without retaining hidden
  session iframes.

## Decisions

Owner priority update (2026-11-16 10:05): performance is top priority in
parallel with exe-launch elimination. Within perf-render, session-iframe cache
removal and the shared visible-only clock scheduler come first. Render
transaction work and Electron diagnostics remain in the same priority set.
Implementation still requires the owner's explicit assignment.

Owner decision (2026-11-16 09:58, binding), verbatim:

> 그리고 숨은 세션 iframe은 그냥 매번 지우는 게 낫겠습니다. 새로고침하면 잠깐은 괜찮아지는 거보니까 도저히 백그라운드에 누적할만한 건 아닌 거 같아요. 차라리 세션 변경마다 새로고침되게 하고, 사용감이 떨어지면 새로고침 퍼포먼스 자체를 개선하는 방향으로 가죠.

Remove the hidden session-iframe cache entirely. A session switch destroys the
previous session iframe and loads the new session fresh; no background session
page accumulation. Rejected: retaining a smaller cache or merely pausing hidden
session iframes. A follow-up phase measures session-switch load time and improves
first-load performance if switching feels slow. The single visible document's
shared scheduler, visible-only labels and age-aware cadence still apply. Pausing
retained body/topic documents is clock hygiene, not an alternative to destroying
the previous session page.

Compatibility tier: **n/a** for private client scheduling/cache refactors with
unchanged daemon protocol, persisted drafts and tracker/store shapes. Reassess
if implementation proposes a persisted visibility or protocol contract.

Historical clock requirement (superseded by the 10:25 trace decision below):
**one shared scheduler per document;
only on-screen labels update; zero clock ticks in hidden iframes and hidden
topics; refresh once on reveal; cadence coarsens with age (for example,
per-second only below one minute).** A hidden guard that leaves a one-second
callback firing is not zero ticks. Visible labels retain their current meaning,
formatting and visual appearance. Do not extend this clock cancellation rule to
unrelated network/authority timers without their own analysis.

### Implementation design sanity pass — 2026-11-16

Assignment authorized promotion with a light design check, not full Sage
ceremony. Sanity check: preserve same-path hash navigation; save serializable
session drafts synchronously before detach; discard superseded loading frames;
ignore late messages/load events from detached frames. The initially considered
shared clock and clipped-visibility work was dropped after the 10:25 trace
instruction below; no scheduler code remains. Keep cache teardown private client
state, without configuration, shared-store or daemon protocol changes.

Existing clock behavior stays unchanged. Exposure/render-transaction work,
input-box work and Electron diagnostics stay outside this Phase 1.

Owner answer (2026-11-16 10:18), verbatim:

> 1206의 경우 지금도 a이긴 합니다. 이건 이미 대비된 동작이긴 해요.

Reuse the existing native beforeunload confirmation path unchanged: confirm
before discarding unpersistable content, and cancellation keeps the current
session. `WsRail.leaveGuard` already covers pasted images. Session-frame teardown
must navigate through that native guard before removing the element; simply
calling iframe.remove() would bypass it. Save serializable drafts synchronously
before starting navigation. No hidden-frame exception, IndexedDB or new custom
confirmation mechanism is selected.

### Trace-driven scope correction — 2026-11-16 10:25

Read the complete lead-supplied analysis at
`C:/Users/user/.cache/ws@kang-sw-devenv/proj/de04b4ac/review-paths/f2812641-01-perf-trace-analysis-20261116.md`.
Owner Chrome trace A covered 14.46 seconds: main thread busy 13.18 seconds,
584 layouts / 6.69 seconds, of which 449 / 5.81 seconds were forced inside JS.
Board exposure checks accounted for 44% of busy main-thread time; one scroll
rAF forced 45–140 document layouts, up to 2.41 seconds. The scroll-path board
re-swap was 3.1 seconds inclusive. Those costs belong to the separate exposure
phase of `261113-feat-ws-dashboard-frontend-performance-profile-and-optimize`.

**Drop the shared clock scheduler from this ticket.** `tickTimes` cost only
32 ms over the approximately 14-second trace; ago clocks were refuted as a
material cost. TimerFire total was 371 ms including debounces. Hidden frames
were approximately 0.5 seconds / 4% of busy time (three iframes): the cache was
not the major lag source in this trace. Nevertheless, the owner's explicit
09:58 cache-removal decision stands. Finish that change independently, do not
claim it fixes the measured exposure bottleneck, and do not touch composer grow
or slash/# enumeration (the latter was refuted by the trace).

### Implementation boundary for Phase 2 — 2026-11-16 11:02

Lead assigned a code-level fresh-load audit, contained caching/deduplication/
visibility wins, and per-session scroll restoration. Baseline is develop
`e6e0ebf44`; live Chrome/Electron trace and first-usable timing remain with the
lead after its arranged swap. The tracked audit is
`../assets/261116-bug-ws-dashboard-hidden-cached-page-cost/phase2-load-audit.md`.

- Externalize only the framed session's shared main script at a content-hashed,
  immutable URL; preserve prerequisite script order and fresh initialization.
- Share only in-flight terminal-list requests. Defer off-screen terminal
  emulator/socket/ring replay until near-view; retain near folded cards' existing
  last-line preview. Do not cache output or resume a fresh empty emulator from
  a stale sequence number.
- Retain only bounded tab-local scroll metadata, separately from drafts. Board
  scroll placement stays with the existing history owner. Do not change board
  exposure functions, frame-height application, or composer implementation.
- Compatibility: **T0** for the new `ws.dashboard.session-views` sessionStorage
  key. The previous version has no reader/writer for it; `sessionViews` and
  `saveSessionView` alone read/write it, so an intervening old client leaves its
  records intact. Existing persisted drafts are unchanged. The content-hashed
  script route and optional `wsSaveSessionView` callback are private page glue,
  not a daemon tool/protocol/store schema change.

## Open questions

- What fresh-switch latency bound should Phase 2 target?

## Constraints

- Work only in the assigned isolated worktree/impl branch; never implement on
  develop. No local ws executable, launcher, Go or test-binary execution. Local
  Node tests must spawn no ws executable. Scoped Go tests run on w5090 only.
- Do not rebuild/restart the shared daemon; live acceptance needs the lead's
  arranged swap. One R1 review, fixes-only R2; no co-author commit trailer.
- Preserve source/origin/key checks, draft format, written attachment refs,
  literal clock formats, history and same-session hash routes. Leave both clock
  implementations and unrelated network/authority timers unchanged.
- Read before implementation: ai-docs/manuals/shipped-surface-boundary.md and
  ai-docs/manuals/release-compatibility.md. Both read in the assigned worktree.
- The initially open unpersistable-content decision was answered before
  implementation: preserve the existing native confirmation path unchanged.

## Phases

### Phase 1: Remove the hidden session-iframe cache

Destroy the previous session iframe on each switch and load the target fresh;
remove the five-page cache and draft-holding cache exceptions. Save text and
other serializable drafts synchronously before switch. Preserve the existing
native beforeunload guard for pasted images: a cancelled leave retains the
current session; accepted teardown removes the old frame before loading its
replacement. Preserve same-session hash navigation and ignore removed frames'
late load/messages. Repeated and superseded loading targets must not accumulate
frames. No new custom confirmation or persistence mechanism.

Acceptance: offline Node VM tests exercise these branches against actual shell
source, including save-before-leave, cancellation/acceptance, repeated switches,
fresh return to a previous session, rapid target replacement, hashes, source/
origin checks and stale events. Scoped Go page/shell tests run remotely on w5090.
Real browser native-confirm and restored-draft acceptance remain separately
arranged by the lead, not inferred from VM mocks. Leave clock, exposure and
composer source unchanged; make no Electron-root-cause or measured-lag fix claim.

### Result (257e52c8d) - 2026-11-16

Implemented on `impl/develop/dock-book-hertz`, based on develop `0593c6b32`:

- Removed the five-page cache/recency helpers. Normal session switches save the
  draft synchronously, navigate the old frame to blank through its existing
  native beforeunload guard, remove it after acceptance, and create one fresh
  target frame. No old session document remains hidden after a successful switch.
- Cancellation leaves the current frame present; same-session hashes remain
  in place. Removed-window messages/load callbacks cannot act on the shell.
  Rapidly superseded targets and repeated switches retain one tracked frame.
- Updated Settings browser expectations from hidden-frame retention to stored
  draft restoration, fresh-session settings delivery and detached old frames.
- The experimental shared scheduler was discarded completely before commit.
  `dashboard.js`, whiteboard `ago.js` and composer/exposure source are unchanged.
  The trace-driven scope correction in Decisions is authoritative.

Verification actually executed:

- Local `node --test agents-plugin-tool/internal/wsdashboard/testdata/shell-lifecycle.test.cjs`:
  PASS, one VM contract (including actual shipped-source parsing, shell URL/message
  cases, save-before-leave, mocked native-cancel/accept outcomes, fresh return,
  repeated/rapid/loading-target switches, hash/history basics and stale events).
- Local `node --check agents-plugin-tool/internal/wsdashboard/testdata/settings-root.browser.cjs`
  and `git diff --check`: PASS. The Settings browser behavior itself was NOT run.
- Remote w5090 job `5b353498b5b04171abc80c048cce677e`:
  `go test ./internal/wsdashboard/ ./internal/wswhiteboard/ -run "^Test(SessionURLServesTheShell|IndexServesTheShell|ShellScript$|ShellSwitchesFramesThroughItsGlue$|ShellHotkeyForwarder|ShellStreamsTheRail)"`
  PASS (`wsdashboard` 3.815 s; `wswhiteboard` no matching tests).
  Node-only Go wrapper cases skip on this Node-less remote; the VM contract was
  run directly with Node locally, not inferred from that Go result.

R1 requested from the lead; not yet a review pass. Real browser native-dialog
accept/cancel, popstate/history behavior and actual text/image restoration need
the lead's arranged acceptance. A fake VM navigation outcome is not proof of
native confirmation behavior. No daemon rebuild/restart/swap, local Go/ws/test
binary, live browser measurement or push was performed. Phase 2 remains open;
this result makes no measured-lag improvement claim.

#### Edition (50ad47ccb) - 2026-11-16

Fixes-only takeover after R1 (`48d09ea57`), authorized by the lead:

- I1: a surviving document's native-beforeunload timer sends an attempt-scoped
  cancellation signal. Clear the pending leave; later same-session hash routes
  and current-row clicks no longer re-prompt/reload. Stale signals cannot cancel
  a newer attempt.
- I2: tag shell history entries with a private index. Never replace a popped
  destination entry. On Stay, restore the prior position with `history.go`;
  defer new picks until asynchronous restoration completes. Accepted and
  cancelled double Back preserve the full history list.
- I3 lead decision: a failed synchronous draft save makes nonempty unsent
  content unpersistable, just like pasted images. Propagate the write result
  through the storage adapter/draft store and widen only the existing native
  leave-guard predicate; save again during beforeunload. Successful retry or an
  empty/view-only draft does not prompt. No new UI or stored-draft shape.
- M3: Settings browser fixture now waits for the new index iframe's `#grid`
  after Back, not the old live frame's identically named root.
- RED evidence: each new I1/I2/I3 Node test failed against pre-fix production
  source: repeated save on same-session route, `[B,B]` history corruption, and
  no prompt for quota-failed text respectively.
- GREEN: `node --test agents-plugin-tool/internal/wsdashboard/testdata/shell-lifecycle.test.cjs`
  passed all four cases (existing lifecycle plus I1, I2 and I3). Includes
  cancelled current-row/hash navigation, stale attempt rejection, Back/Forward,
  accepted/cancelled double Back, asynchronous restore, denied/full storage,
  retry and pasted-image guard coverage. Settings browser script syntax check
  and `git diff --check` passed. One scoped passing check closes this fixes-only
  pass per the lead; no Go checks, local binaries, browser run or daemon change.
- Native dialog/sticky-activation behavior and real Settings browser acceptance
  still require the lead's arranged live check; VM timers/traversal are models.

Forward findings retained from R1 (not part of this fixes-only pass):

- M1: remove obsolete hidden-frame `shown:false`/`shHidden`/draft plumbing later.
- M2: strengthen the stale-load guard regression to assert no settings/shown
  delivery to a removed frame, or deliberately drop the redundant guard.
- M4 (Phase 2): measure lost scroll-position restoration, terminal ring replay
  and transient empty-header cost on fresh return; no iframe cache restoration.
- M5: live acceptance must cover dropping an image into a fresh frame without
  prior click/key activation; Chromium may suppress the native prompt. Also
  assess Electron's existing whole-app leave-dialog copy on a session switch.

#### Edition (190255257) - 2026-11-16

Focused re-review confirmed I1–I3 and M3; lead requested two race hardenings
before merge:

- Treat `stay` as tentative, not proof that navigation was cancelled. Keep the
  latest attempt on its frame: an accepted blank load still removes that frame
  and starts its destination even after the same attempt's `stay`. If Back's
  position was already restored, or restoration is still in flight, reconcile
  history to the accepted destination without rewriting entries.
- Bound history restoration to 1000ms. Missing shell popstate no longer queues
  every later navigation forever: clear the restoring state and flush the
  latest queued pick. Normal completion cancels the timeout.
- RED: both new Node tests failed on `94a4aaaf6`: the destination frame was
  missing after `stay` then blank load, and the queued pick never retried after
  the timeout. GREEN: all six cases in
  `node --test agents-plugin-tool/internal/wsdashboard/testdata/shell-lifecycle.test.cjs`
  pass, covering rail/Back accepted-load races, synchronous/deferred history
  restoration, fallback timing and queue release. `git diff --check` passes.
- Node-only hardening; no local Go/ws/binary, browser run or daemon operation.
  Native-dialog/live-browser acceptance remains with the lead. The previously
  recorded Forward findings remain unchanged.

### Phase 2: Measure and improve fresh session-switch load performance

Depends on Phase 1's landed Result. Measure cold/warm session switches, initial
HTML/parse/render, first usable composer/board and draft restoration under
representative short/long sessions in browser and Electron. If switching feels
slow, optimize the measured first-load bottleneck while preserving the fresh
load on every session change; do not reintroduce a cache. The acceptable latency
bound remains an explicit owner question. No visual change is intended. This
docs capture runs no tests, browsers, builds or live cache mutation.

### Result (c658c4798) - 2026-11-16

Phase 2 implementation boundary on `impl/develop/dock-book-hertz`, based on
merged develop `e6e0ebf44`. Full code-level request/server/render/parse/stream/
terminal/draft breakdown and limitations:
[phase2-load-audit.md](../assets/261116-bug-ws-dashboard-hidden-cached-page-cost/phase2-load-audit.md).

- Framed sessions reference a shared content-hashed main-script URL with
  immutable caching; all prerequisite modules retain their order. Shell,
  index and standalone rendering retain the existing inline path. Main script
  bytes are unchanged by transfer and still initialize in every fresh document.
- Concurrent terminal-list readers share only the in-flight request. Off-screen
  terminal cards defer emulator/font/socket/full-ring replay until near-view;
  near folded cards still replay for their last-line preview. Fresh emulators
  continue from sequence zero, avoiding incomplete terminal buffers.
- Fresh frames save/restore at most 50 tab-local per-session scroll records,
  independently of drafts. Includes hidden pane offsets and conversation
  bottom-follow; explicit hash routes and user input take precedence. Board
  scroll placement uses its existing history owner. No old frame/output cache,
  board exposure/frame-height implementation or composer change.

Short before/after table (code-level counts/raw source bytes, not a live trace):

| Bucket | Before | After |
|---|---|---|
| Main-script tag per framed HTML | 591,999 B | 64 B: 591,935 B removed from this HTML bucket |
| Shared main-script fetch | Inline in every no-store document | +1 cold GET, 595,238 B; 0 network GETs when immutable cache is warm |
| Two concurrent terminal-list readers | 2 GETs | 1 GET, both callbacks |
| N restored terminals, K near-view | N emulators/sockets/ring replays immediately | K immediately; N-K postponed |
| Return scroll metadata | Lost with removed document | Bounded per-session tab-local records |

The cold main-script bucket grows by 3,303 raw bytes and one request, including
new scroll logic and URL-tag overhead. Other inline code/CSS remains, and
JavaScript initialization is not removed. Terminal backlog travels over its
WebSocket, not a separate HTTP fetch; ring sizes and end-to-end timing remain
unmeasured. Existing immutable Xterm/Mermaid/font/body-frame assets need no new
cache layer.

Verification executed:

- Local `node --test` for `testdata/session-load.test.cjs` and
  `testdata/shell-lifecycle.test.cjs`: **11/11 PASS**. Pins deduplication,
  lazy terminal activation/disposal and folded preview, independent session
  scroll records, owned board placement, hash/input priority, bottom-follow,
  denied storage and the 50-record cap, static asset wiring, and all Phase 1
  guard/history races.
- Remote w5090 job `9e8eceae540b4c028691da80d9c1a34d`:
  `go test ./internal/wsdashboard/ -run "Test.*(Asset|FreshSession|TerminalCardScript|WebCardScriptRides|SessionURL|Shell|Settings|SessionPage|IndexPage|DashboardSourceAssembly)" -count=1`
  **PASS** (16.625s). Old inline-script order/dev-source assertions were updated
  to check the external reference and exact fetched bytes. Node/browser-only
  cases skip on the remote as expected.
- Final embedded-source check on w5090 job `533b612e23e1484995c2535e6db19de7`:
  fresh-session/static-asset/terminal/web-card/source-assembly/dev-asset cases
  **PASS** (1.404s). Remote `gofmt -d` and local `git diff --check` passed.
- No local Go/ws/test binary, launcher, browser run or daemon lifecycle action.
  No worker-created temporary files.

Pending lead acceptance: independent review, then cold/warm Chrome/Electron
trace after the arranged swap, including cache hits, first usable composer/
board, near/off-screen terminals and return scrolling. Scroll records restore
current-layout offsets; changed content can clamp them, and older conversation
pages outside the fresh newest window are not proactively fetched to recreate
an old history anchor. Exact long-history restoration and the numeric latency
bound remain explicit follow-up/owner questions, not claimed measured wins.

#### Edition (cad7b26e5) - 2026-11-16

Phase 2 R1 fixes-only pass (I1, I2, M1, M4):

- I1 supersedes the initial numeric board restoration: save scroller/topic/
  panel identities and their reading offset; resolve the mounted panel through
  existing board key/live-panel helpers and call `w.place(node, {readingOffset})`.
  Missing identities alone use the explicitly approximate numeric fallback.
  Snapshot a valid pending `BOARD_RELOAD` record before either rAF executes and
  skip session-view board/document placement even if the earlier reload rAF
  consumes that record. The old comment claiming numeric restoration follows
  late frame heights was removed.
- I2 supersedes numeric conversation restoration: save the top crossing entry's
  `data-entry` identity and in-view offset. A shifted newest window resolves that
  identity; a missing entry or an at-bottom leave restores bottom-follow instead
  of a stale pixel offset. No older-history fetch or buffer cache is introduced.
- M4: follow state and pane-offset seeding occur only in the final,
  non-cancelled restore callback. Key/pointer input that cancels placement cannot
  silently disable bottom-follow.
- M1: coalesced terminal-list callbacks receive the shared request's actual
  start time; reconcile no longer uses a later joining caller's timestamp to
  delete a card born after the request.
- Local `node --test` on `testdata/session-load.test.cjs` and
  `testdata/shell-lifecycle.test.cjs`: **14/14 PASS** after correcting a missing
  `KIND` constant in the new terminal test fixture. New cases model board growth
  above the saved panel, earlier asset-reload placement/record consumption,
  four new conversation entries while away, missing/at-bottom fallback,
  input cancellation and a terminal created during a shared list request.
  `git diff --check` passed. No Go, ws/launcher/test binary, browser run or
  daemon operation in this pass. Board exposure/frame-height and composer source
  remain untouched.

Forward (R1; deliberately not implemented in this fixes-only pass):

- M2: a main-script hash that disappears between HTML and its GET can leave an
  inert frame until shell asset reload or another switch; consider bounded
  one-shot failure recovery or a previous-build grace window separately.
- M3: off-screen terminals' exit/closed status is delayed until near-view or a
  later reconcile. Confirm the accepted tradeoff or seed status from the list.
- M5: hidden board pane offsets still seed the existing `paneScroll` path;
  `revealPanes` can apply those numerically on a later narrow-layout tab reveal.
  Coordinate its owner-aware handling separately with the board executor.
- M6: the static-asset client test remains source-regex-shaped and scroll tests
  model the placement owner. Real integrated reload/late-frame/window-shift
  acceptance remains with the lead; these VM cases are not a live trace.

The prior byte table describes the initial Phase 2 snapshot, not this larger
anchor-hardening script. No new measured latency/cache-hit claim is made.


## Resolution (2026-11-18)

Both phases landed (Phase 2 merged in 55cd8f6fd). Phase 1's no-cache direction was then reversed by owner (1244) = a: 62b168e34 restored a bounded five-document session iframe cache and verified that hidden pages stay quiet. The numeric latency-bound question is moot under that design.
