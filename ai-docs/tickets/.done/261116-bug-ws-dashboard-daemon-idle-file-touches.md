---
title: "Reduce dashboard daemon idle file touches"
related:
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "Earlier Windows idle CPU and discovery-cache work"
sage-review-design: completed
sage-review-completeness: completed
completed: 2026-11-16
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

### Result (fcf912cd6) - 2026-11-16

- Landed the approved T0 slice in priority order: five-minute whiteboard safety discovery with changed/poked/deadline processing; Windows metadata-only cache identity; shared two-second page checks and memory-only adapter stamps; quiet heartbeat-only index updates and redundant extra hosted snapshot/lead-registration suppression; git badge reads consolidated from six commands to two. No new filesystem probes, manifest key, ledger, event hub, frontend edits or heartbeat interval increase.
- Disk snapshot heartbeats (30 seconds) and registration heartbeats (two minutes) remain mandatory for legacy readers. Unchanged extra hosted writes may be skipped; lead registration equality is checked against the existing read under the ownership lock, never an in-memory reservation. Missing, corrupt, changed or foreign rows are handled explicitly.
- Inbox writes retain read-through snapshot admission; only presentation stamps use last-known evidence. A non-nudging external snapshot is recovered by the existing index backstop, while clock-driven expiry remains visible. Absent evidence has its own stamp token so a render's newly loaded snapshot cannot hide a subsequent stale transition.
- Verification: remote w5090 job `cc351d79887645b9a7876dbffbe7659d` passed all selected tests in wsdashboard (23.930 s), wsagent (0.065 s), mcp (2.800 s), wsstate (0.003 s), and wswhiteboard (8.001 s). Command below used the owner's `sys_lock.sh remote w5090 run exec10 25 --sync <this worktree> -- ...` wrapper. Coverage includes changed/unchanged state, expired and recovered liveness, read-through inbox gates, production index-to-stream recovery, concurrent/external folder writers, pending transaction recovery, exact deadlines, explicit pokes and last-viewer close. Intermediate failures exposed a missing test directory, registration schema normalization and a pre-render stamp baseline hole; all were corrected before the passing run.

```sh
export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ./internal/wsdashboard/... ./internal/wsagent/... ./internal/mcp/... ./internal/wsstate/... ./internal/wswhiteboard/... -run "^(TestIdleTouches.*|TestReadGitBadge|TestGitWatcher.*|TestGitRefreshRouteAndStream|TestHostedSnapshotDedupAndDiskHeartbeat|TestSnapshotDedupRequiresSlotAndSuccessfulWrite|TestRegistrationHeartbeatFailureAndLiveness|TestWhiteboardNudge.*|TestIndexRefresh.*|TestRefreshTick.*|TestHostedProcessExitRefreshesHead|TestSameFileMetadata.*|TestFileCache.*|TestSnapshotCache.*|TestFolderCache.*|TestWakeStampAndSnapshotUnchangedFilesAreNotOpened)$" -count=1 -v
```

- Comparable fixture evidence: 68 sessions x 28 stamps made **1,904 -> 0 snapshot identity checks** after warmup; a shared-page burst of 28 further refreshes made zero additional stamp calls; ten registration heartbeats made **10 -> 0 index change notifications**, retaining fresh timestamps. Real repository reads made **6 -> 2 git spawns** for clean, in-place edit, staged+modified+untracked, diverged-upstream, disabled `status.aheadBehind`, and shadowed-branch cases. Scheduler tests confirm **10 -> 1 idle discovery scans per five minutes**, while due timers and pokes are independent. Snapshot/registration tests prove extra unchanged writes are skipped, changes persist, failed writes retry, and periodic disk heartbeats still advance.
- Expected savings at the audit's comparable idle workload, not an owner-PC measurement: discovery alone about **18k touches/min** (20k -> 2k) before overlapping identity savings; ordinary no-deadline Notify work about 90% lower. The 68 x 28 stamp example removes about **1.9k stats/min plus the former Windows identity handle opens**. Windows metadata identity removes cache-hit handle opens on the remaining discovery/folder/file-cache paths; the study's ~6k/min estimate overlaps those savings and is not additive. Git badge subprocess fan-out is 67% lower; git child filesystem touches were not measured. Steady-state mandatory heartbeat writes save **zero**; only redundant extra writes are removed.
- Compatibility evidence (paths under agents-plugin-tool/internal): `wsagent/record.go:484-519` retains forced disk heartbeats and the unchanged snapshot shape; `wsagent/runner.go:1644-1655,1779-1819` retains registration cadence and actual on-disk reservations; legacy readers still use `wslive/snapshot.go:439-444` and `wsdashboard/registry.go:49-53,205-213`. No old/new persistence reconstruction is required: both read the same fields and schedules. Windows creation time remains an identity hint, not a guaranteed file ID.
- Independent R1 plus supplemental R1 found substantive registration reservation, git configuration parity, inbox admission and existing-stamp-test issues. Fixed them; fixes-only R2 cleared all retained findings. The recovery-interval finding was withdrawn under the explicit five-minute lead steer. No fresh sweep in R2.
- Deliberate deviations: no unsafe HEAD/index-only stat gating; no hidden-tab detection without the out-of-scope frontend visibility signal. Existing no-stream gating and immediate reopen/refresh remain tested. No local daemon execution/replacement, live owner-PC trace, merge or push.
- Forward: With explicit Chromium opt-in, `TestDesktopUpdateBrowser` fails on w5090 because node is absent; remote job `e36e40cc5f064510b28a8a8325b4dea4` reproduced the identical missing-node failure on candidate and a reverse-patch overlay of base `f261ef907` (new tests excluded). This is a pre-existing environment prerequisite, not a regression; default browser gates were unchanged.
- Forward: The same remote job cross-compiled Windows tests for all five packages successfully, but executed no Windows test binary. Owner-side Windows/DLP touch measurement and creation-time behavior remain unverified; the live audit numbers above are estimates, not measured acceptance claims.
- Forward (minor R1): update the operational runbook's git command fan-out description in a docs-scoped change. Hidden-tab git gating needs coordination with the separate frontend work; retain mandatory heartbeats until the owner settles crash-detection latency.

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


## Resolution (2026-11-16)

Completed the approved local T0 slice in fcf912cd6, with focused remote verification and independent R1/fixes-only R2. Result records measured fixture deltas, estimated audit-scale savings, preserved legacy disk heartbeats, and the explicit git stat-gating/hidden-tab limitations. No merge or push.
