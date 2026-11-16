---
title: "Reduce dashboard daemon idle file touches"
related:
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "Earlier Windows idle CPU and discovery-cache work"
---

# Reduce dashboard daemon idle file touches

## Goal

Reduce unnecessary idle filesystem touches and git processes using only the audit's low-risk internal fixes, while preserving liveness and timely updates.

## Context

The daemon idle file-touch audit (`ec56856d-02-daemon-file-touch-audit.md`, 2026-11-16) estimated 45–60k touches/min with 68 sessions and 203 hosted conversations; counts scale with workload and were measured after the earlier discovery cache. On the owner's PC, DLP inspects every file touch, so touch count matters beyond CPU seconds.

Exec3 after-capture, per 125 s: discovery is now metadata-dominated (`os.Stat` 8.19 s, `SameFile`/`loadFileId` 5.12 s including `CreateFile` 4.28 s); 68.73 s of 70.35 s `ReadFile` is in exec output-pipe goroutines; the git subset is 22.11 s, wait-dominated. These are attribution facts, not additive CPU savings.

## Phases

### Phase 1: Eliminate redundant idle work

- Memoize or share `pages.go` `indexStamp`/`sessionStamp` via `pageCache.get` across streams on the `sse.go` 15 s ticker, with bounded staleness.
- In wsagent `runner.go` `conv.loop` (~1570–1665), skip unchanged snapshot and registration rewrites. Preserve heartbeat and stale-detection semantics, including mixed-version behavior; do not silently lengthen heartbeat intervals.
- Do not bump Index Rev or wake streams for heartbeat-only registration changes; semantic changes and liveness transitions must still reach consumers.
- Stat-gate `gitbadge.go` git calls and stop polling when no dashboard page is visible; resume and refresh on visibility. Cover git state changes without missing updates.
- Make the `whiteboard_nudges.go` 30 s reconciliation process changed boards only, while preserving explicit pokes and eventual recovery.

Verify with focused tests for unchanged and changed state, heartbeat/liveness, visible/hidden transitions, board changes and pokes. Compare idle touches, writes and git spawns before/after under a comparable workload, and confirm prompt delivery and refresh.

## Constraints

Do not expand this into the broader polling redesign; that investigation is separate. Preserve correctness under concurrent writers and mixed versions. Audit touch estimates are workload-dependent, not acceptance thresholds.

## Decisions

These are T0 internal changes. Keep the existing compatibility and liveness contracts.
