---
title: "P3: Reduce hidden cached-page overhead and make relative clocks visibility-aware"
sage-review-design: skipped
sage-review-completeness: skipped
related:
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "P1 render transaction and P4 composer evidence; coordinate visibility/render integration"
  261113-feat-ws-whiteboard-board-frontend-performance: "landed board performance work; retain frame laziness, history and draft guarantees"
  261116-bug-werkschaft-electron-high-resolution-lag-diagnostics: "native presentation hypothesis remains separate"
---

# P3: Reduce hidden cached-page overhead and make relative clocks visibility-aware

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

Owner-required outcome (2026-11-16): **one shared scheduler per document;
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
ignore late messages/load events from detached frames. Use the existing
whiteboard age asset for one document clock, including dashboard time labels;
observe visible labels instead of walking every label on a timer. Propagate
clipped visibility to opaque body frames using parent/source and document-key
checks, preserving existing tooltip holds. Keep this private client state,
without configuration, shared-store or daemon protocol changes.

Cadence keeps current label formats: second-level freshness below one minute,
minute-level thereafter (older minute/hour/day age labels use their next actual
boundary). Reveal refreshes once at the current time rather than replaying ticks.
Render-transaction work, input-box work and Electron diagnostics stay outside
this Phase 1.

Owner answer (2026-11-16 10:18), verbatim:

> 1206의 경우 지금도 a이긴 합니다. 이건 이미 대비된 동작이긴 해요.

Reuse the existing native beforeunload confirmation path unchanged: confirm
before discarding unpersistable content, and cancellation keeps the current
session. `WsRail.leaveGuard` already covers pasted images. Session-frame teardown
must navigate through that native guard before removing the element; simply
calling iframe.remove() would bypass it. Save serializable drafts synchronously
before starting navigation. No hidden-frame exception, IndexedDB or new custom
confirmation mechanism is selected.

## Open questions

- What fresh-switch latency bound should Phase 2 target?

## Constraints

- Work only in the assigned isolated worktree/impl branch; never implement on
  develop. No local ws executable, launcher, Go or test-binary execution. Local
  Node tests must spawn no ws executable. Scoped Go tests run on w5090 only.
- Do not rebuild/restart the shared daemon; live acceptance needs the lead's
  arranged swap. One R1 review, fixes-only R2; no co-author commit trailer.
- Preserve source/origin/key checks, draft format, written attachment refs,
  literal clock formats, history and same-session hash routes. Cancel timers
  on hidden/pagehide; do not pause unrelated network/authority timers here.
- Read before implementation: ai-docs/manuals/shipped-surface-boundary.md and
  ai-docs/manuals/release-compatibility.md. Both read in the assigned worktree.
- The initially open unpersistable-content decision was answered before
  implementation: preserve the existing native confirmation path unchanged.

## Phases

### Phase 1: Remove the session cache and make visible-document clocks efficient

Destroy the previous session iframe on each switch and load the target fresh;
remove the five-page cache and draft-holding cache exceptions. Resolve unsent
work handling explicitly without retaining hidden session documents. Implement
the required clock outcome within the visible session and its body/topic
frames, measuring visits/writes and callbacks before/after with identical
content and traffic. Integrate DOM swaps, folds, filters, pinned/adopted topics,
viewport and opaque body visibility; unregister on unmount. Keep reveal catch-up
to one refresh and preserve body-age holds.

Future acceptance asserts no retained previous-session iframe after switches,
no accumulation across repeated switches, one clock scheduler per document,
zero hidden clock callbacks, no hidden/off-screen label writes, one reveal
refresh, coarse old-label cadence, correct visible ages and the settled unsent
work behavior. Compare historical 1-versus-5-page resource evidence with the
new fresh-session lifecycle without claiming it proves the Electron root cause.

### Phase 2: Measure and improve fresh session-switch load performance

Depends on Phase 1's landed Result. Measure cold/warm session switches, initial
HTML/parse/render, first usable composer/board and draft restoration under
representative short/long sessions in browser and Electron. If switching feels
slow, optimize the measured first-load bottleneck while preserving the fresh
load on every session change; do not reintroduce a cache. The acceptable latency
bound remains an explicit owner question. No visual change is intended. This
docs capture runs no tests, browsers, builds or live cache mutation.
