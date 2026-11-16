---
title: "Dashboard daemon holds about 15% of a core while idle on Windows"
related:
  261111-feat-ws-mailbox-wait-without-process-relaunch: "sibling from the same CPU probe (board (288) b); that ticket covers the per-wake process cost, this one the daemon's own CPU"
  261116-bug-ws-exe-launch-elimination: "separate lead-owned exe-spawn elimination; count legitimate git subprocesses without conflating them with ws exe probes"
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "client render/input measurements remain separate from backend CPU"
---

# Dashboard daemon idle CPU on Windows

## Background

User, 2026-11-11: the Windows machine showed sustained CPU use and asked
whether ws was the cause. A delegated probe (board topic `cpu-windows`)
measured, on native Windows:

- `ws-mcp-dashboard-*.exe`: about 15% of one core in a 10 s sample, 10.6%
  averaged over its 7 h lifetime. It hosts every MCP session and mailbox
  wait, so the CPU that per-session executables used to spend is now
  concentrated here; whether 15% idle is that sum or a hot loop is unknown.
- The ws Python launchers (35 processes): about 0% CPU, about 30 MB each;
  no busy loop in their code (blocking readline / WaitForSingleObject).
- `D:/sys_lock_remote.py` at about 20% is the user's own tool, outside ws.

Candidate periodic work in the daemon, unmeasured: the mailbox wait
backstop (10 s) and nudge (1 s), the session index (30 s), the tracker
watch (10 s), and SSE keepalives (15 s).

User decision (board (288) b): profile the daemon and fix the hot path, as
its own ticket beside the mailbox-wait relaunch ticket.

## Additional backend profile evidence — 2026-11-16

Duplicate capture: the daemon-performance request belongs in this existing
accepted backend CPU ticket rather than a new family. The owner reports an
earlier backend profile posted to the lead's board. Its top candidate is
whiteboard nudge discovery: approximately **3,500 identity records read from
disk every 30 seconds**. The board item/profile artifact ID was not supplied
and the raw pprof was not independently read in this docs capture; retain that
evidence boundary rather than inventing percentages or a causal proof.

Source anchors verified:
`agents-plugin-tool/internal/mcp/whiteboard_nudges.go:100` reads discovery
directories, `:21` sets the 30-second reconciliation interval, and the run loop
around `:226-243` calls discovery on the periodic scan and again for pending-target
validation. The `discover`/`loadRecordWith`
path validates lead identity; optimization must not turn stale registry or board
folders into authority to recover/mint keys.

Other profile candidates, **each still requiring measurement**:

- rail fan-out work per event and number of subscribers;
- board reconciliation replay and entries traversed;
- mailbox-store full rereads and bytes/records read;
- per-request git process spawns, counted with latency and call attribution.
  Git is a legitimate tool; do not conflate its use with avoidable ws executable
  launches. Cross-reference `261116-bug-ws-exe-launch-elimination`, owned by the
  lead, rather than duplicate that effort.

Reproduction: with a separately authorized representative daemon/session set,
retain a pprof CPU profile spanning multiple 30-second discovery cycles, plus
identity count, disk-read/stat counts, nudge latency, event/subscriber load,
mailbox activity and git-spawn counts. Repeat under identical workload after
Phase 1. `/debug/pprof/` is available when the daemon runs with
`WS_MCP_DASHBOARD_PPROF=1`; record actual build/PID/flag and capture interval.
A release daemon without that flag may return 404. This capture does not probe
or restart a daemon to enable profiling.

Candidate Phase 1 fixes: an incremental discovery index, or a cache invalidated
by appropriate directory/record mtime or explicit identity lifecycle events.
Directory mtime alone does not detect every in-place record change. Preserve
new/changed/deleted identities, both current and legacy roots, corruption/error
handling, pending revalidation and authority checks. No persistence or cache
invalidation policy is selected here.

## Decisions

Owner priority update (2026-11-16 10:05): perf-daemon discovery indexing and
before/after pprof measurement are top priority alongside perf-render,
perf-input and the separate exe-launch elimination effort. Implementation
still awaits the owner's explicit assignment.

Compatibility tier: **n/a** for in-process indexing/caching and private
performance instrumentation with unchanged identity files, MCP/hello, stores
and tracker/config fields. If a persistent index or protocol/config extension
is proposed, classify concrete old/new writers/readers before readiness; prefer
T0 and do not assume old writers maintain an index they do not know exists.
This is backend performance-only work with no intended visual/behavior change.

## Open Questions

Settle before promotion to `ready/`:

- Which board profile/build/workload establishes the 3,500-record observation,
  and how much CPU/I/O is attributable to periodic versus pending discovery?
- Should discovery be incremental, mtime-gated or lifecycle-invalidated, and
  what catches in-place updates, deletes and legacy identities safely?
- What idle/active CPU, I/O and nudge-latency acceptance bounds apply, and at what
  session/identity counts?
- Which remaining candidate ranks highest after the first measured correction?

## Constraints

- Heavy Go builds run through `~/.local/bin/ws-rgo`; the profile itself is
  taken on the user's Windows machine
  (`ai-docs/manuals/windows-dogfood.md`).
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`.

## Phases

### Phase 1: Index or cache whiteboard nudge discovery with before/after pprof

Recover the board profile and establish a matched baseline over multiple scan
cycles. Implement the selected incremental or mtime-gated discovery index/cache
without weakening identity/authority validation. Record before/after pprof,
record reads/stats, CPU/allocation/I/O and nudge latency with the same identity
and session population. Future fixtures cover insert/update/delete, in-place
identity writes, absent roots, legacy data, invalid identities, pending targets
and invalidation races. Do not introduce a persisted index as an incidental
optimization without settling its compatibility.

### Phase 2: Optimize the remaining candidates ranked by measurement

Depends on Phase 1's landed Result and a fresh matched profile. Rank rail
fan-out per event, board reconciliation replay, mailbox full rereads and
per-request git spawns; fix only the demonstrated next hotspots. Count legitimate
git invocations and coordinate process-lifecycle changes with the separate
exe-spawn ticket. Preserve delivery, reconciliation, authority and tool results;
report before/after profiles and any deferred candidates. No builds, tests,
executables, profiles or live daemon mutations are run by this docs capture.
