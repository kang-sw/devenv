---
title: "Restore a bounded, idle dashboard session-iframe cache"
sage-review-design: completed
sage-review-completeness: completed
related:
  261116-bug-ws-dashboard-hidden-cached-page-cost: "Cache removal and fresh-load improvements; owner superseded the no-cache forward"
  261116-bug-ws-dashboard-frontend-idle-and-switch-cost: "Hidden animation/clock gates and fresh-load geometry batching"
---

# Restore a bounded, idle dashboard session-iframe cache

## Goal

Avoid reloading a previously visited session's outer dashboard iframe without multiplying hidden-page idle work. Preserve current draft, native leave-guard, history, navigation and reading-position guarantees.

## Background

The owner removed the old five-frame cache at 09:58 KST on 2026-11-16 because hidden pages accumulated background cost (`257e52c8d`, merged in `e6e0ebf44`). Fresh-load and leave-guard fixes subsequently changed the same lifecycle code. `8c3687d9c` added hidden animation/clock gates. Owner (1244)=a explicitly authorized restoration at 16:21 KST; the earlier stability gate is satisfied.

## Decisions

- Restore manually on develop `f18c773ab`, not by reverting removal: preserve later native cancellation/history/draft and fresh-load fixes.
- Supersede the former configuration-key decision: use a single frontend code constant `CACHE_LIMIT = 5` in `shell.js`; zero is the testable fresh-load/off fallback. Add no config-manifest key, setting, persistence record or daemon protocol.
- Compatibility tier **T0**, frontend-only: `shell.js` and `dashboard-lifecycle.js` exchange private same-origin UI state. No store/config schema is read or written differently; old pages lacking the cache-safety hook retain fresh-load navigation. Newer pages reconnect through the existing stream hashes. In particular, the previous claim that a new manifest key was automatically T0 was incorrect; strict manifest readers make that a T3 hazard, avoided here.
- Bound retained documents to five (including the active page); one in-flight destination may exist while it loads. Reuse only fully loaded session frames by path, update hash/settings/theme on reveal, evict least-recently-shown hidden frames. Index/worktree and unshown/superseded loading frames are not retained.
- Cache only pages that can safely become idle: save drafts synchronously; reject pasted/unpersisted images or failed draft storage, all framework/runtime cards, visible/maximized Files views, foreign iframes, support-chat state, pending tracker reads and pending terminal list/create operations. These pages keep the current fresh-load/native `beforeunload` path. Recheck eligibility at destination load: an expired admission suspends the never-shown destination and obtains native leave acceptance; Stay discards that destination while preserving the accepted-blank race authority. This deliberately favors safe cache hits over a broad new subsystem for suspending arbitrary tools.
- Hidden cached outer documents stop their session stream/retry work and recurring clock intervals, pause animations, and suppress shell-post rAF work; reveal reconnects using current hashes and catches up once. No global timer monkeypatch or new scheduler.
- Arbitrary scripts in opaque nested doc frames cannot be reliably frozen by browser page code. Unmount those nested documents on hide using existing placeholder/height machinery and remount paced on reveal. Preserve outer board membership, drafts and scroll; nested mockup/script transient state and selection are intentionally not retained. Do not touch whiteboard rendering or its stream implementation while the panel-first worker is active. `ago.js` is a shared whiteboard asset but any edit here is clock lifecycle only; flag it in the handoff.
- Verification uses one complete focused check set: pure Node lifecycle/stream/clock contracts plus an isolated Chromium/CDP switch and 10-second idle trace, comparing the code-constant off/on paths. Report main-thread tasks/style/layout, timer/rAF/stream activity and document reuse; do not substitute skipped clock writes for absence of wakeups. A controlled fixture is not a claim about every owner session or arbitrary nested content.
- Independent R1, then fixes-only R2 for substantive findings; minor findings go to Forward. No local Go/build/test/ws executables or launcher, no daemon mutation, no merge/push. Go checks, if needed, run only through w5090/sys_lock. Temporary browser/profile/trace files live under `D:/Temp/iframe-cache/` and are deleted.

## Phases

### Phase 1: Restore bounded session reuse with hidden-page quiescence

Implement the bounded cache and conservative eligibility above without bypassing native leave confirmation for unsafe pages. Cover session/hash/Back/Forward navigation, draft storage failures and pasted images, eviction, superseded loads, stale messages and off-mode behavior. Stop hidden recurring work and remount opaque documents only when visible. Measure cached versus fresh switch latency and all-frame idle activity in an isolated browser using the same Chrome tracing/CDP categories as the earlier profiling (no screenshots, CPU profiler or mouse activity). Record exact evidence, limits, and any file overlap before closing the phase.

## Constraints

- Frontend lifecycle/assets and their focused tests only; no new config-manifest key and no whiteboard renderer/stream changes.
- Read `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md` before edits.
- Existing native leave guards remain the authority for destructive navigation. Never silently remove a frame holding unpersisted work.
- Do not claim measured zero idle work for arbitrary third-party documents; unload/exclude them as above and state the measurement boundary.
