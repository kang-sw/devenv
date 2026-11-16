---
title: "Research polling minimization and a change ledger"
related:
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

The owner's direction is polling minimization toward an event-driven design. The ledger, IPC hub and OS notifications remain alternatives to evaluate, not an implementation decision.

## Outcome Ledger

### Verified Findings

The cited audit and Exec3 measurements above are workload-specific; the audit did not deeply examine every poller.

### Open Questions

Which option and compatibility tier can safely reduce touches across old and new binaries, and how will missed changes be recovered without restoring the original polling cost?
