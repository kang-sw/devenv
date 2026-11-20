---
title: "폴링 최소화 및 변경 원장(ledger) 연구"
related:
  261116-feat-ws-daemon-hub-first-polling: "Actionable phased implementation of board 1242=a and 1243=a"
  261116-bug-ws-dashboard-frontend-idle-and-switch-cost: "Frontend idle and switch costs"
  261116-bug-ws-dashboard-daemon-idle-file-touches: "Low-risk daemon idle touch reductions"
  261111-bug-ws-dashboard-daemon-idle-cpu-windows: "Earlier daemon idle CPU and discovery work"
---

# Research polling minimization and a change ledger

## Goal

Investigate a path from periodic discovery toward event-driven change delivery that minimizes file touches without losing updates.

## Context

Owner directive at 12:52 on 2026-11-16: minimize polling and move toward event-driven design; writers touch a single change ledger only on change, and readers fstat it and read only changed targets. The owner runs DLP that inspects every file touch, making touch count a first-class cost.

The daemon idle file-touch audit (`ec56856d-02-daemon-file-touch-audit.md`) estimated 45–60k touches/min under its 68-session, 203-conversation workload. Exec3 after-capture, per 125 s, found discovery metadata-dominated: `os.Stat` 8.19 s and `SameFile`/`loadFileId` 5.12 s including `CreateFile` 4.28 s. Of 70.35 s `ReadFile`, 68.73 s is exec output-pipe goroutines; the git subset is 22.11 s, wait-dominated. Do not treat pipe reads as discovery or sum overlapping timings as CPU.

## Topic: Alternatives and criteria

Compare (a) one shared change ledger plus fstat, with readers fetching changed targets; (b) a daemon in-memory event hub, with writers notifying it over existing IPC; and (c) Windows `ReadDirectoryChangesW` / fsnotify. Quantify DLP-visible touches, wakeups and latency; identify cross-platform tradeoffs. For each, assess cross-version compatibility tier and fallback for old binaries that do not write the ledger, multiple writers (hosted sessions, CLI ws-mcp, Pi extension), daemon restart, missed notifications, concurrent updates, crash safety and reconciliation. Specify how loss, overflow, partial writes and ledger growth are detected and recovered. Do not assume a ledger is reliable merely because its metadata changed.

## Topic: Pollers to inventory and migrate

Use the audit and Exec3 to map owners, data, cadence, current cost, change sources, fallback and migration order for:

- Per-stream `pageCache.get` / `indexStamp` / `sessionStamp` (15 s), index registration/snapshot refresh and heartbeat-triggered stream wakes; hosted `conv.loop` snapshot and registration heartbeats.
- Whiteboard nudge discovery and reconcile (30 s), identity checks and full `Notify`; explicit `Poke` paths.
- Mailbox sweep (30 s), waiter backstop peeks (10 s), session hire loop (1 s), presence and dashboard session heartbeats.
- Git badge (3 min) and rail worktree admission/listing, plus rail layout and memo reads. Separate git child-process waiting and exec output-pipe reads from file discovery.
- Conditional tracker and holder scans, observed-directory resweeps, and other pollers flagged as not fully audited (plan usage, ack checks, stream stats and ledger git calls). Establish actual activity and cost before prioritizing them.

## Constraints

Research only; do not prescribe implementation before compatibility and recovery are established. Produce a comparison, touch-count measurement plan and incremental migration/fallback proposal with tests for old/new process combinations, concurrent writers and crashes.

## Decisions

- Owner decision (board 1242=a, confirmed 2026-11-16 16:14): **hub first**. In-daemon writers notify the daemon's in-memory change hub directly and readers subscribe. Do not create a shared change-ledger file. Reuse the existing broker, targeted index lanes and mailbox wake hub rather than adding a parallel event framework.
- Owner decision (board 1243=a, confirmed 2026-11-16 16:14): external changes (git, hand edits, older/non-notifying binaries) must be recovered within **five minutes**, using a code constant and a slow safety scan. No new config-manifest key; unknown keys are a T3 compatibility hazard.
- The earlier ledger proposal in Context is historical, superseded by 1242=a. The study remains evidence/proposals except where the owner has explicitly decided above.
- Remaining implementation is collected in `261116-feat-ws-daemon-hub-first-polling`, including P4 mailbox timers/hireLoop and P5 git stat gating plus the observed daemon-startup git burst. Already landed Windows metadata identity, changed-only board reconciliation and memory-only adapter stamps are not reimplemented.
- Out of scope: P3 heartbeat cadence/flush changes await a separate owner crash-detection-latency decision; P7 OS notifications await the owner's DLP experiment. Existing disk heartbeats and liveness behavior remain intact.

## Outcome Ledger

### Verified Findings

The cited audit and Exec3 measurements above are workload-specific; the audit did not deeply examine every poller.

### Confirmed Decisions

Board 1242=a selects the in-memory hub, not a shared ledger. Board 1243=a fixes the external safety bound at five minutes in code, not configuration. See Decisions above.

### Open Questions

Heartbeat/crash-detection latency (P3) and OS-notification DLP behavior (P7) remain owner decisions; neither blocks the hub-first ticket. Per-surface compatibility and recovery are recorded in that actionable ticket before each phase lands.
