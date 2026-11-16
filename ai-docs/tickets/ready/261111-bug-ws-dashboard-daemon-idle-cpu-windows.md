---
title: "Dashboard daemon holds about 15% of a core while idle on Windows"
related:
  261111-feat-ws-mailbox-wait-without-process-relaunch: "sibling from the same CPU probe (board (288) b); that ticket covers the per-wake process cost, this one the daemon's own CPU"
  261116-bug-ws-exe-launch-elimination: "separate lead-owned exe-spawn elimination; count legitimate git subprocesses without conflating them with ws exe probes"
  261113-feat-ws-dashboard-frontend-performance-profile-and-optimize: "client render/input measurements remain separate from backend CPU"
  261116-bug-ws-dashboard-daemon-idle-file-touches: "follow-up for metadata/file-touch costs exposed by the Phase 1 Windows after capture"
  261116-research-ws-polling-minimization-event-ledger: "separate design study for polling minimization; no event-ledger mechanism is selected here"
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 7ce830ae41c31c47
sage-review-completeness-reviewed: 7ce830ae41c31c47
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
disk every 30 seconds**. The board item/profile artifact ID was not supplied;
the earlier artifact remains unavailable. A new approved REST capture and
read-only identity inventory now provide independent evidence below; do not
reinterpret the old capture or call the active shared workload idle.

Lead-owned read-only REST CPU capture: PID 28968, daemon hash
`c34fbf9413a3a53e9997760e19762e7d17380f63840f538cb19f365630838a6f`, captured
2026-11-16 01:39:30Z–01:41:35Z (125 seconds), same daemon before and after.
Remote `go tool pprof` analysis found 49.28 seconds of CPU samples over the
125-second wall interval (39.42% sample/wall ratio, not a controlled idle CPU
measurement). `WhiteboardNudges.discover` accounts for 4.90 seconds cumulative
(9.94% of samples); its `loadRecordWith` path accounts for 4.72 seconds,
`readRecordFile` 4.66 seconds, and file-open/create work 3.86 seconds. Overall
`pageCache.get` is 13.45 seconds and `WhiteboardNudges.run` 6.62 seconds; these
are overlapping call-stack totals, not additive percentages.

A separate read-only inventory at 01:47:23Z found 3,624 unique identity names:
980 split directories and 2,644 legacy files; 187 parsed top-level lead
identities and 3,437 child/delegate records. All 3,624 parsed in that inventory;
it did not run the full authority/delivery gate. There were 7 whiteboard
folders in this project's board root. This independently supports the scale of
the earlier approximately 3,500 observation without claiming identical workload.

Raw capture, health metadata and remote pprof summaries are temporarily retained
under `D:/Temp/exec3-perf/` for the approved after comparison. The before PID,
port and content hash were unchanged at both ends. Source revision e525b0738 was
supplied by the coordinating lead, not independently inferred from /health.
The shared workload included active sessions/pages; no process CPU, physical
I/O counters, per-discovery runtime read count or idle classification was captured.

A second context-bearing before capture at 01:53:32Z–01:55:37Z used the same
PID/hash and 125-second CPU window. Identity names were 3,633 then 3,637 (legacy
2,644 unchanged); goroutine snapshots showed 6 page streams and 1 rail stream
at both ends; hosted process counts were 14 then 16, with 7 then 6 running
conversation states. Existing stream-counter deltas were 25 nudges, 883 sends,
860 session renders, 423 changed and 251 unchanged session reads. Discovery
accounts for 7.65 seconds cumulative, record reads 7.25 seconds and create-file
work 5.98 seconds. This sample has 361.61 total sampled seconds, dominated by
syscall waits (249.76 seconds in WaitForSingleObject, including holder-process
wait stacks); do not present that total/wall ratio as actual process CPU or a
controlled idle measurement. Both raw captures remain retained. After capture
must include the same context and compare focal discovery costs with activity
and identity counts; global percentages alone are not causal acceptance.

Source anchors verified:
`agents-plugin-tool/internal/mcp/whiteboard_nudges.go:92-143` reads discovery
directories and identity records; `:21` sets the 30-second reconciliation
interval, and the run loop at `:232-243` calls discovery on the periodic scan
and again for pending-target validation. `discover` validates lead identity
through `loadRecordWith` (`agents-plugin-tool/internal/mcp/session_layout.go:231-288`);
optimization must not turn stale registry or board folders into authority to
recover/mint keys.

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
handling, pending revalidation and authority checks. These were pre-implementation
candidates; the selected process-local cache and measured outcome are recorded
below. No persisted index was introduced.

## Decisions

Owner priority update (2026-11-16 10:05): perf-daemon discovery indexing and
before/after pprof measurement are top priority alongside perf-render,
perf-input and the separate exe-launch elimination effort. Implementation
was explicitly assigned to Exec3 by the owner-designated coordinating lead
on 2026-11-16, with new worktree isolation and remote-only Go verification.

Compatibility tier: **n/a** for in-process indexing/caching and private
performance instrumentation with unchanged identity files, MCP/hello, stores
and tracker/config fields. If a persistent index or protocol/config extension
is proposed, classify concrete old/new writers/readers before readiness; prefer
T0 and do not assume old writers maintain an index they do not know exists.
This is backend performance-only work with no intended visual/behavior change.

- **Discovery cache (technical, delegated):** choose a daemon-local,
  metadata-gated cache of parsed identity-only values, retaining no whiteboard
  snapshots, rather than a persisted index or new filesystem watcher. Enumerate
  both roots and check each effective identity file's metadata so ordinary
  in-place writes, replacements, migration and deletes invalidate correctly;
  directory mtime alone is insufficient. Bound memory to identities still
  discoverable and discard unreadable/corrupt/unsupported identities safely.
  Rejected: lifecycle hooks alone (legacy/external writers bypass them), a
  directory-only stamp (misses in-place writes), or persistent state old writers
  would not maintain. Pin same-size replacements and recent timestamp races in
  tests; uncertain metadata must not become cached authority.
- **Authority is uncached at delivery:** the index is only discovery evidence.
  Preserve ReplyID collision suppression and all current lead/root/scope/schema
  checks. Keep identity, registration and hosted-process revalidation before
  enqueue fresh; never touch, recover, mint, migrate or refresh a key as a
  performance shortcut. Pending discovery must see changed identities as well
  as periodic reconciliation does.
- **Acceptance (technical, measurement-led):** at a representative 3,500-identity
  split/legacy fixture, reduce warm unchanged record-content reads by at least
  90% against the old full reread, and report metadata-stat counts, allocations
  and benchmark timing separately. Insert/update/delete, same-size in-place
  writes, legacy fallback/precedence, invalid records, pending targets,
  collision changes and no-resurrection semantics must retain deterministic
  coverage. No nudge deadline regression is authorized. Capture a fresh matched
  125-second live pprof after a coordinating-lead-approved build/swap, recording
  identity count, session/subscriber activity and focal discovery cost against
  the context-bearing second baseline. These are observed comparable windows,
  not a controlled matched workload; normalize activity and report mismatches.
  Do not claim a causal global idle-CPU improvement from a differently loaded
  daemon or from Windows syscall-wait sample totals.
  A tested code candidate can be reported before that approval, but Phase 1
  cannot claim its before/after acceptance or completion without after evidence.
- **Remaining ranking:** defer selection of Phase 2's hotspot until Phase 1
  lands and a fresh profile establishes it. The current 13.45-second page-cache
  stack is a candidate, not authorization to edit UI or expand this phase.

## Constraints

- Hard Windows security rule (AhnLab V3): no local Go test/build, test binaries,
  ws-mcp/dashboard executable or launcher runs. Repeat this in every worker
  brief. Go tests/benchmarks run only on w5090 through
  `bash D:/sys_lock.sh remote w5090 run <who> <min> --sync '<worktree>' --
  'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ...'`.
  Scope checks to the changed nudge/discovery behavior; never run whole slow
  packages. Read-only profiling uses existing REST/pprof only.
- Ask the coordinating lead before any daemon rebuild or swap. The lead
  sequences swaps with Exec1; only then capture after. Never replace, shut down
  or launch a shared daemon from a worker or test.
- Exec1 owns ws-mcp-launcher.py, stage/daemon-start, plan_usage_probe.go and
  pi.go in parallel. Do not edit them; coordinate before needing those files.
- Work only in the acquired full worktree on `impl/develop/phony-eel-cramp`;
  no shared-root commits/branch changes, merges, pushes or worktree release.
  Temporary artifacts stay under `D:/Temp/exec3-perf/`; remove after use and
  stop if D: free space is below 20 GB.
- Manuals: `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/ws-mcp.md`.
- Convention: `ai-docs/manuals/shipped-surface-boundary.md` (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: `ai-docs/manuals/ws-mcp.md` (declared for `agents-plugin-tool/internal/mcp/`)
- Convention: `ai-docs/manuals/release-compatibility.md` (declared for `agents-plugin-tool/internal/`)

## Prior Decisions

- 19b0567d7 (2026-11-11, commit): "Profile the daemon and fix the hot path, as its own ticket beside the mailbox-wait relaunch ticket." — bearing: supports
- 261111-feat-ws-mailbox-wait-without-process-relaunch (2026-11-13, Proposed Approach): "Unlike the daemon's own idle CPU (`261111-bug-ws-dashboard-daemon-idle-cpu-windows`), the cause is known." — bearing: constrains
- f064acbe3 (2026-11-16, commit): "Keep rendering, composer, and daemon measurement scopes explicit." — bearing: supports
- 0593c6b32 (2026-11-16, commit): "perf-render, perf-input and perf-daemon are top priority in parallel with exe-launch elimination." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | `agents-plugin-tool/internal/mcp/whiteboard_nudges.go`; `agents-plugin-tool/internal/mcp/session_layout.go`; existing tests at `agents-plugin-tool/internal/mcp/whiteboard_nudges_test.go` |
| scope.surface | internal | Private implementation in `agents-plugin-tool/internal/mcp`; no exported symbol or protocol change specified |
| scope.new_public_symbol | no | No public symbol is proposed |
| scope.new_type_contract | no | No public type or signature contract is proposed |
| scope.test_surface | existing | `agents-plugin-tool/internal/mcp/whiteboard_nudges_test.go` covers discovery behavior |
| complexity.reuse_points | confirmed | Reuses `discover` and `loadRecordWith`; source: `whiteboard_nudges.go:92-143`, `session_layout.go:231-288` |
| complexity.side_effect_risk | moderate | Discovery/cache invalidation changes filesystem observation and scheduling without selecting persistent state |
| risk.correctness | high | Must retain identity authority, stale/deleted record handling, pending revalidation, and corruption behavior |
| risk.fit | moderate | Several measured candidates remain and the selected optimization/acceptance bounds are open |
| risk.test | high | Insert/update/delete, in-place writes, legacy roots, invalid identities, pending targets, and invalidation races require coverage |
| risk.security_or_contract | high | Identity discovery gates lead authority; persistent indexes would alter compatibility assumptions |

## Blocked (2026-11-16)

The approved live after capture is complete and its actual outcome is recorded
under Phase 1. The cache reduced record-content work, but the Windows capture
did not establish a performance win; metadata operations now dominate. Keep
this bug open. The coordinating lead explicitly holds further optimization
here while the related idle-file-touch ticket and polling-minimization design
study settle the next work. Phase 2 has not started; do not dispatch another
discovery optimization or daemon rebuild/swap from this ticket.

## Phases

### Phase 1: Index or cache whiteboard nudge discovery with before/after pprof

Use the independently captured before profile above as the new baseline over
multiple scan cycles; the older board artifact remains unavailable. Implement
the selected in-process metadata-gated identity discovery cache
without weakening identity/authority validation. Record before/after pprof,
record reads/stats, CPU/allocation/I/O and nudge latency with the same identity
and session population. Future fixtures cover insert/update/delete, in-place
identity writes, absent roots, legacy data, invalid identities, pending targets
and invalidation races. Do not introduce a persisted index as an incidental
optimization without settling its compatibility.

#### Implementation candidate — pre-after checkpoint (2026-11-16)

Commits `c3715b0bd` and `692d07930` add an in-process identity-only
metadata-gated discovery cache. Split files retain precedence over legacy;
file identity, size, mtime, a two-second quiet interval and a post-read stat
prevent stable stamps from surviving ordinary same-size writes, replacements
and observed read races. Unreadable records and root-list failures fail closed;
delivery still reads identity and registration fresh. The cache retains no
legacy board snapshot or mutable session fields, writes no key, and introduces
no new persisted or protocol surface (compatibility **n/a**).

Remote w5090 focused `^TestWhiteboardNudge` scope passed after both commits.
The 3,500-identity fixture has 2,644 legacy and 856 split records, with many
nonlead/delegate entries. At five iterations per benchmark side, warm cached
versus the mirrored prior full-discovery loop: **0 vs 3,500 successful record
content reads per scan (100% fewer, above the 90% candidate threshold)**;
**7,311 vs 1,167 metadata stats per scan** (identity and board existence),
**22.1 vs 50.5 ms/op**, **5.66 vs 8.64 MB/op**, and **47,963 vs 93,050
allocs/op**. The numbers are remote Linux source-level counters/benchmark
results, not Windows physical-I/O, process CPU, nudge latency or matched live
pprof evidence. Metadata stats are nonzero I/O; the old path also performs
file-open work not counted as stats. Existing precise-timer and delivery tests
passed; no deadline policy changed.

Independent Lite R1 reported two Important test/benchmark integrity findings;
`692d07930` fixes both (full-workload comparison and quiet same-size in-place
mtime change). Two initial R2 CLI attempts timed out without a verdict; a new
bounded fixes-only R2 recovered both findings and independently marked each
FIXED. It confirmed the mirrored old loop includes listing/board/collision
work, and the same-size/same-inode changed-mtime test now asserts a content
reread while both timestamps are quiet. No new broad review was claimed.

Lead verification reran the remote scoped WhiteboardNudge tests and a one-pass
3,500-identity benchmark: PASS; cached/old were 27.7/60.9 ms, 0/3,500 successful
content reads, 7,311/1,167 explicit metadata stats, 5.62/8.71 MB and
47,961/93,050 allocations per scan. Different iteration counts are reported
separately, not combined into one timing comparison.

**Windows measurement caveat:** `os.SameFile` can lazily call CreateFile to
resolve a fresh os.Stat value's file identity (`os/types_windows.go`,
`fileStat.loadFileId`). This implicit handle-open work is not a content read or
an explicitly counted metadata stat in these fixtures. The identity comparison
protects same-size/equal-mtime replacements, but zero warm content reads does
not establish zero Windows file opens or a Windows CPU win. Preserve that
correctness check until live focal after evidence shows whether it is a cost
that needs another measured correction.

This checkpoint preceded the approved live after measurement. The Result below
records the observed outcome rather than treating the Linux fixture result as
Windows acceptance; Phase 2 remains held by the coordinating lead.

### Result (4f5716178) - 2026-11-16

- The identity-only cache and its scoped tests landed through merge
  `4f5716178`. Remote w5090 tests/benchmark and independent fixes-only R2 passed
  as recorded above. Warm Linux record-content reads fell from 3,500 to 0 per
  scan, but explicit metadata stats increased from 1,167 to 7,311. Those are
  source-level fixture counters, not Windows file-touch or physical-I/O counts.
- After the coordinating lead's batched rebuild/swap and hosted-session restart,
  the approved REST-only after capture ran at 03:48:54Z–03:50:59Z for 125.01 s.
  PID 2516 and binary hash
  `fbafb6d6e9847038857a55d1465fb35895dcb66a4d144ac4c9277395e02dda81`
  were unchanged at both ends; source `7bdf11e74` was supplied by the lead.
  Profiling was enabled by `WS_MCP_DASHBOARD_PPROF=1`, not a config key.
  The earlier PID 27736 attempt returned pprof HTTP404 and supplied no CPU
  evidence; it is excluded from the comparison. No daemon was started,
  rebuilt, swapped or shut down by this profiling session.
- **Observed Windows outcome: no performance win established.** Compared with
  the contextual second 125 s baseline, `WhiteboardNudges.discover` increased
  from **7.65 s to 13.60 s cumulative** (about 78%). The 7.25 s
  `readRecordFile` discovery stack no longer appears in the low-threshold after
  view; this is not proof of zero runtime read calls. Metadata now dominates:
  `os.Stat` is **8.19 s versus 0.18 s** (`GetFileAttributesEx` 8.15 s after),
  and `SameFile`/`loadFileId` adds **5.12 s**, including **4.28 s CreateFile**.
  This confirms the warned-about implicit Windows handle-open path. Fewer JSON
  content reads alone did not remove the expensive file touches. The owner
  reports a DLP agent inspecting every touch; reads, stats, writes and legitimate
  git subprocesses are therefore first-class measurement costs.
- The windows are observed, not controlled or causally isolated. Identity names
  were 3,633–3,637 before versus 3,691 after (legacy 2,644 unchanged); page/rail
  stream snapshots were 6/1 versus 2/1, hosted processes 14–16 versus 10–9,
  running conversation states 7–6 versus 6–3, and nudge deltas 25 versus 11.
  Builds, launcher freshness and other activity also changed. No absolute idle
  process-CPU, physical-I/O, runtime syscall/spawn-count or nudge-latency
  improvement is claimed from sampled seconds.
- Low-threshold after attribution: global `CreateFile` 87.43 s,
  `GetFileAttributesEx` 61.82 s, `os.Stat` 60.31 s, `os.ReadFile` 38.70 s and
  `loadRecordWithBoard` 35.67 s are overlapping filesystem/handle-bound stacks,
  not additive disk-operation counts. **68.73 s of the 70.35 s ReadFile stack
  sits in exec output-pipe reader goroutines**, not ordinary disk reads.
  The git-focused subset is **22.11 s**, including tracker `RunGit` 12.23 s,
  `wsgit.RunGit` 8.78 s and git-badge `RunGit` 1.16 s; it is wait-dominated
  (18.17 s `WaitForSingleObject`). Global exec includes `Cmd.Run` 50.35 s,
  `CreateProcess` 6.73 s and `WaitForSingleObject` 43.51 s. Windows wait stacks
  can inflate totals; none of these sample durations establishes invocation
  counts or actual process utilization.
- Raw before/after profiles, health/workload metadata, zero-threshold views and
  the comparison summary remain as small gate evidence under
  `D:/Temp/exec3-perf/` (`comparison.md`, `after-cpu.pprof`,
  `after-metadata.json`, `analysis/*discovery-precise.txt`, filesystem/exec
  views). Analysis used remote Go pprof only; no local Go test/build, test
  executable, ws executable or launcher run occurred. This Result records the
  cache experiment and its remaining cost; **the idle-CPU bug is not fixed or
  closed**, and no new optimization phase starts here.

#### Forward

- `pageCache.get` took **13.45 s cumulative in the first 125 s before window**
  and **22.47 s in the second**. Page streaming/SSE/hidden-frame share was not
  broken down; syscall/`WaitForSingleObject` stacks may inflate totals. It was
  kept outside the discovery-cache phase. The after value is 82.29 s under a
  different workload/build, not an isolated page-cache regression claim.
- Per-identity stat plus `SameFile` metadata cost is the next measured target.
  Follow-up implementation belongs to related
  `261116-bug-ws-dashboard-daemon-idle-file-touches`; polling-minimization
  design belongs to `261116-research-ws-polling-minimization-event-ledger`.
  Link those threads rather than selecting or restating their mechanisms here.

### Phase 2: Optimize the remaining candidates ranked by measurement

Depends on Phase 1's landed Result and a fresh matched profile. Rank rail
fan-out per event, board reconciliation replay, mailbox full rereads and
per-request git spawns; fix only the demonstrated next hotspots. Count legitimate
git invocations and coordinate process-lifecycle changes with the separate
exe-spawn ticket. Preserve delivery, reconciliation, authority and tool results;
report before/after profiles and any deferred candidates. Phase 2 has not
started; the coordination gate above reserves further work to the linked
follow-up tickets and the coordinating lead's next direction.
