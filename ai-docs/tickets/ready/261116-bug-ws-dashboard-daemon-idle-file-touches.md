---
title: "Reduce dashboard daemon idle file touches"
related:
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "Earlier Windows idle CPU and discovery-cache work"
sage-review-design: completed
sage-review-completeness: completed
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

- Light design check (2026-11-16): use local in-memory memoization and semantic comparisons; no ledger/event hub, frontend changes, new filesystem probes, heartbeat interval increases, or store/wire changes. Existing on-disk heartbeat writes needed by older readers remain mandatory.
- Share refresh checks per cached page for a bounded two-second window; wake-driven full reads bypass the memo. Stamps use the live slot or already-read snapshot evidence in memory, without a per-session stat. Existing index reads/nudges refresh external file evidence; the 30-second index backstop plus the 15-second stream tick bounds non-nudging writers' recovery (rather than a fresh snapshot probe on every stamp).
- Registration comparisons ignore only HeartbeatAt and snapshot age; still retain the newest heartbeat and publish semantic/liveness transitions.
- Lead steer (2026-11-16 13:30): prioritize whiteboard discovery/reconciliation, then memory-only stamps, then heartbeat work. Use a hard-coded five-minute full safety scan, no manifest/config key. Keep exact scheduled board deadlines and the 30-second failed/ineligible delivery retry; missed pokes, ticket-only eligibility changes and interrupted transactions recover at the five-minute scan. Explicit pokes still validate/discover immediately and force the target due.
- In-scope addition from that steer: replace Windows cache-hit SameFile handle opens in file_cache.go, whiteboard_nudges.go and folders_cache.go with a shared comparison of the existing stat result's size, mode, mtime and Win32 creation time. Any mismatch or unavailable Windows metadata is a miss. Non-Windows retains SameFile. Creation time is a conservative metadata hint, not a unique file ID; retain existing recent-write safeguards and periodic recovery.
- Git metadata-only stat gating cannot prove working-tree contents unchanged (in-place edits do not alter HEAD/index or directory metadata), and adding probes conflicts with the owner's hard no-new-stat/read rule. Prefer reducing existing git command fan-out while retaining the correctness backstop; do not claim metadata-only gating or hidden-tab detection without a frontend visibility signal. Existing last-stream-close gating remains.
- Light design/completeness check completed against the actual call sites and the owner's steering: the single phase is executable with the explicit git/heartbeat limitations above, focused remote checks, and independent review; no new protocol or persistence contract is needed.
- Verification is remote-only on w5090. Use deterministic comparable-workload counters for touch/write/spawn deltas; do not execute or replace the owner's local daemon. Independent R1; fixes-only R2 only for substantive findings. Broader polling redesign is deferred.
