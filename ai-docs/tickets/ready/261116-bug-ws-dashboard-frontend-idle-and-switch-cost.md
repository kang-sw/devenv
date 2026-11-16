---
title: "Reduce dashboard frontend idle and session-switch cost"
related:
  261116-feat-ws-dashboard-restore-session-iframe-cache: "Restore the session iframe cache later, separately"
sage-review-design: completed
sage-review-completeness: completed
---

# Reduce dashboard frontend idle and session-switch cost

## Goal

Make fresh session switches responsive and reduce idle and scroll work without restoring the iframe cache here.

## Context

Trace E (`1e5c5357-01-perf-trace-e-idle-and-switch.md`, 2026-11-16) measured a short-session switch settling in 1.4 s and a long one in 4.4 s. On the long session, `stream.js` history fill's `readyHeight` caused 134 forced layouts and 1.57 s of layout and style in the first task. The short idle capture also showed continuous compositor animation costing about 18% of a core; the responsible element was not identified and capture had profiling artifacts. Clock label rollovers caused document-wide layouts. Trace D (`ec56856d-01-perf-trace-d-1230.md`) identifies scroll-path forced layouts and non-composited rail ghost animation; it is an interaction trace, not idle.

## Phases

### Phase 1: Session switch

- Batch `stream.js` history `fill`/`progress` row insertion and measure height once per batch rather than per row in `readyHeight`; coalesce late height-driven resumes. Preserve the history window and gap behavior.
- Coalesce `wbhistoryload` -> `watchBoard` -> `checkVisibility` after fill settles; avoid repeated visibility flushes.
- Gather load-time geometry reads from `drawerSync`, `restoreScroll`, lifecycle `post`, and `scrollerOf` into one read phase after writes.
- Target the trace's long-session switch from 4.4 s to about 2 s and short-session switch from 1.4 s to about 0.9 s, measured to settled/usable, not just first paint. These are estimates, not additive guarantees.

### Result (226928a56) - 2026-11-16

- Batched whiteboard history admission in 32-row speculative rounds across all topics, not just the conversation. Each round writes all topics then reads heights; surplus returns to inert gaps before binding. Preserved whole-row 1000px windows, gap order, pending frame waits, target navigation, swap restore and auto-fold.
- Coalesced history notifications and late height resumes; deferred `watchBoard` subscriptions' `checkVisibility`/scroller inspection into one read phase before owner writes. The board iframe still loads fresh and its sandboxed doc frames retain existing paced/near-view mounting; no iframe-cache changes.
- Drawer fill and lifecycle posts use rAF read-before-write passes; numeric scroll restoration batches reads and yields to input, owned scrollers, pinned conversation and asynchronous pagination.
- Verification: `node --test agents-plugin-tool/internal/wsdashboard/frontend-history.test.js agents-plugin-tool/internal/wsdashboard/frontend-geometry.test.js agents-plugin-tool/internal/wsdashboard/frontend-scroll.test.js` — 7/7 passed. The 27-topic synthetic board preserves 141 exact rows/topic with at most 7 modeled dirty-read flushes (versus roughly 3,780 row-by-row writes/reads for the same fixture); watch subscriptions perform no synchronous visibility probes. These are scheduling contracts, not browser timing measurements.
- R1: one Important pagination/restoration race; fixed with pending-restore cancellation and actual-callback interleaving regression. Fixes-only R2: resolved, no outstanding substantive findings.
- Estimate for the owner's 27-topic board: observed 3,000–4,000 ms switch -> approximately 1,800–2,400 ms settled/usable, saving roughly 1,200–1,800 ms. Trace E's longer case suggests up to ~2,400 ms saved (4,400 -> ~2,000 ms); short case ~300–500 ms. The ~450 ms history/watch visibility chain should fall to one shared read phase rather than repeated write/read flushes (~300–400 ms estimated reduction); savings overlap, not additive guarantees.
- Forward: owner capture of representative 27-topic switch and board/doc-frame load is required to confirm actual timing and visual/scroll behavior; Node cannot measure layout or iframe readiness. No local Go/ws/test executable, launcher or dev.py was run.

### Phase 2: Idle and scroll

- Identify the active infinite CSS animations (Trace E candidates include rail orbit/blink, `.spin`, `.pulse`, `.pibadge`, and composer busy spinner); use `steps()` and pause them when hidden or not running.
- Add CSS containment around clock labels so `wbAgo` (`ago.js:109/178`) does not relayout the whole document; hidden-gate it like `tickTimes`. Contain the composer dock and `#bcol`, checking layout and paint behavior.
- Remove scroll-path forced layouts: make `pill.js` show writes idempotent, cache `notePanes` visibility, and rAF-coalesce lifecycle `post` and `cards.js` update. Move the rail ghost animation to transform/opacity. Make `scrollState`/`restoreScroll` avoid forced style on each WS message.

## Constraints

- Keep history, scrolling, visibility, animation state and live updates correct. The owner wants the session iframe cache restored later under the related ticket; do not fold it into this work.
- Add focused Node/JS tests where feasible for batching, gates and scroll behavior. After the swap, have the owner capture representative switch, idle and scroll traces and compare against the baselines; check for hidden-page work and UI regressions. Containment savings and animation attribution require measurement.

## Decisions

- Optimize fresh loads first; iframe caching is a separate, later owner-gated effort.
- Light design/completeness check completed by implementing worker per explicit owner instruction (2026-11-16): scope, invariants, two phases and verification boundary are executable; no unresolved design decisions. Implementation details: use bounded speculative history batches with a write-free height read pass, returning surplus rows to the inert gap before binding; preserve exact whole-row 1000px windows, unknown-frame waits, and pending snapshots. Coalesce follow-up observers/events with rAF, not a new scheduler or configuration.
- Collect drawer and lifecycle geometry before writes; defer watch-owner geometry until after history writes. Phase 2 depends on Phase 1 and uses cached pane visibility, idempotent pill writes, event-driven rAF work, stepped running animations and hidden-page gates. Prefer layout/style containment without paint clipping of composer popovers.
- Scope is frontend JS/CSS and focused pure Node tests only; no iframe-cache restore or daemon Go edits. No protocol/store change. Per owner: one passing focused check closes each phase; R1 independent review and fixes-only R2 for substantive findings; minor findings go to Forward.
- Local Go/build/ws/test executables, launchers and dev.py are forbidden (AhnLab V3). Remote Go only via the owner's w5090 command if needed; local tests must be pure Node without ws spawning. Timing savings are trace-derived estimates pending owner switch/idle/scroll captures, not measured acceptance claims.
