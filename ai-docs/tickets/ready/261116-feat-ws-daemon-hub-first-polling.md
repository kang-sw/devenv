---
title: "Move daemon readers to hub-first notification with five-minute safety recovery"
related:
  261116-research-ws-polling-minimization-event-ledger: "Owner-selected hub-first design and safety bound"
  261116-bug-ws-dashboard-daemon-idle-file-touches: "Landed metadata identity, board reconciliation and memory-only adapter stamps"
sage-review-design: completed
sage-review-completeness: completed
---

# Move daemon readers to hub-first notification with five-minute safety recovery

## Goal

Reduce idle filesystem discovery and git spawning by notifying in-process readers directly after successful writes. Reuse the existing daemon broker and mailbox wake hub, with startup recovery and a hard-coded maximum five-minute safety backstop for external/non-notifying writers. No shared change-ledger file.

## Evidence

- Owner board 1242=a: hub first; in-daemon writers notify the in-daemon hub and readers subscribe. Board 1243=a: maximum external staleness five minutes, in code rather than a new config key. Confirmed by assignment on 2026-11-16 16:14.
- Read-only study: `4967e0e8-01-polling-min-design-study.md`, 2026-11-16, originally at develop f261ef907. Its P0/P1/P2 subset has since landed through 96847df20; do not redo it. Its estimates are not measurements of this change.
- Owner capture `D:/Temp/daemon-swap-1116c/summary-final.txt`: fresh daemon from 529c0255e, PID 2404, directly parented 17 + 11 git.exe starts in adjacent startup burst windows (28 total). The broader window had 85 git.exe rows across multiple parents; do not attribute all of them to the daemon or equate process counts with CPU/touch counts.
- Execution base for Phase 1: develop f18c773ab, worktree `D:/Workspace/Repos/.ws-worktrees/devenv/pretext-shopping-example`, branch `impl/develop/polling-hub-phase1`.

## Decisions

- Use the existing `wsdashboard` broker (subscription, dependency tokens and sequence numbers), per-session Index lanes, hosted snapshot slots and `wswake.Mailbox`. A small direct notification entry point is enough; no parallel generic bus, persistent ledger, dependency framework, new service or config-manifest key.
- Five minutes is a maximum external safety-scan period, not permission to postpone explicit changes. Startup loads authoritative files immediately. Successful in-process writes notify directly; old HTTP nudges still work. Coalesced/lost notifications and external edits recover by the safety scan. Existing faster foreign-file checks may remain until Phase 4.
- Preserve all existing disk heartbeat intervals and crash/liveness behavior. The slower filesystem index scan must not slow clock-only liveness transitions: continue aging already-read registration/snapshot evidence in memory at the prior 30-second index cadence. This changes no writer heartbeat/flush policy and does not decide P3.
- Do not replace a correctness gate with a cache hint: inbox admission and registration ownership still read/lock as before. A direct notification identifies an invalidation, never authorizes a file mutation. Cross-cache notifications must fall back to the existing external path.
- Git metadata is only a spawn-suppression hint: HEAD/index/directory metadata misses in-place worktree edits and untracked contents. Phase 3 must force a complete refresh at least every five minutes even when metadata is unchanged, and explicit refresh must bypass the gate. No fetch is added.
- Light design/completeness check: current direct/HTTP writer paths, broker subscriptions, index lanes and recovery seams support the bounded phases below. The owner explicitly chose the architecture and safety bound. Implement Phase 1 only in this run; later phases remain ready.

### Compatibility by cross-version surface

- **In-process notification interfaces: T0.** Optional Go-side notification capability on the hosting daemon is not serialized. Existing hosted snapshot-slot APIs remain valid; absent capability falls back to the existing HTTP path. Replacement daemons reconstruct the index from unchanged files.
- **HTTP `/nudge`, MCP tools and proxy hello: T0, unchanged.** Retain the existing body (`reply_id`, `project`), route status/validation and old caller behavior (`wsdashboard/daemon.go`, `handleNudge`; `index.go`, `Nudge`; `proxy.go`, hello validation). No new mandatory field, renamed tool or protocol/version boundary.
- **Registration, snapshot, mailbox/inbox and hire stores: T0, unchanged schemas and writers.** Old/new binaries read and write the same fields; legacy non-notifying writes recover within five minutes. Disk heartbeat schedules and `Registration.Live`/`Snapshot.Stale` semantics remain unchanged; no lost-field reconstruction or migration is needed. Sources: `wsdashboard/registry.go`, `wslive/snapshot.go`, `wsmailbox/store.go`, `wsdashboard/session_hire.go`.
- **SSE/WebSocket and tracker events: T0, unchanged wire vocabulary.** Reuse the current broker dependency tokens internally and existing full/delta region delivery; add no tracker event or client capability requirement (`wsdashboard/sse.go`, `stream_ws.go`).
- **Config/rsrc manifests: T0 because untouched.** Keep the safety bound as a code constant; adding an unknown manifest field/key can impose a T3 old-reader boundary and is prohibited here.
- **Git state: T0, read-only.** Metadata gates and per-root coalescing do not alter refs, index, worktrees or stored formats; old binaries continue reading normally. The forced safety read preserves external change delivery.

## Constraints

- Apply `ai-docs/manuals/shipped-surface-boundary.md` and `ai-docs/manuals/release-compatibility.md`; MCP changes also follow `ai-docs/manuals/ws-mcp.md`.
- **Out of scope:** P3 heartbeat cadence, disk flush suppression or crash-detection-latency policy; P7 OS notification/RDCW/fsnotify pending the owner's DLP experiment. No shared change-ledger file, config key, release/protocol change or unrelated frontend work.
- Do not introduce a faster polling loop or replace bulk reads with equivalent per-session probes. Prefer memory-only work; justify any new filesystem probe by removing more existing work.
- No local Go tests, test executables, ws/ws-mcp executable or launcher execution. Go verification runs only on w5090 using `bash D:/sys_lock.sh remote w5090 run exec10 25 --sync <worktree> -- 'export PATH=$HOME/sdk/go/bin:$PATH; cd agents-plugin-tool && go test ...'`. No runtime replacement on the owner's PC.
- Temp artifacts only under `D:/Temp/<purpose>/`, removed after conclusions are recorded. No Co-Authored-By trailer, merge to develop or push.
- One passing full affected-package check closes a phase. Independent R1; fixes-only R2 only for substantive findings; minor findings go to Forward. Report the retained branch HEAD at the phase boundary.

## Phases

### Phase 1: Direct daemon notification and a five-minute index safety scan

- Add the smallest optional direct notification entry point on the hosting daemon, validating the state-directory boundary without an endpoint-file lookup or loopback HTTP round trip. Route MCP and hosted CLI store-write nudges through it when supported; preserve HTTP fallback for old/non-hosting/foreign-cache paths and notification outcome bookkeeping.
- Reuse the current per-session Index lanes and broker subscriptions so successful writes schedule targeted reads and consumers wake after the read lands. Keep existing snapshot/ring, mailbox and whiteboard Poke paths direct; wire hosted CLI board writes to the same Poke callback as hosted MCP.
- Change the default filesystem Index recovery scan from 30 seconds to a five-minute code constant. Keep initial load, missing-session discovery, removals, retries, concurrent writers and restart recovery. Retain 30-second clock-only registration/snapshot aging in memory, preserving the old liveness cadence without file probes or heartbeat changes.
- Verify direct delivery with no endpoint file/HTTP request, foreign-cache and absent-capability fallback, coalescing, subscribers seeing semantic changes, no per-tick store reads, clock expiry/recovery, missed notification/external mutation recovery and restart reconstruction. Run full affected Go package checks remotely; report fixture read/spawn deltas, not unmeasured owner-PC savings.

### Phase 2: Deadline-driven mailbox maintenance and hires (study P4)

Depends on Phase 1.

- Replace the unconditional 30-second undelivered sweep (`cmd/ws-mcp/mailbox_sweep.go`, `wsmailbox/undelivered.go`) with startup discovery, wake-driven changed-store processing and a timer for the earliest pending delivery deadline. Retain a five-minute full external safety scan, exited-recipient notices, store locking, durable receipts and mailbox wake delivery.
- Mailbox waiters subscribe to existing in-process wake signals and check immediately after a real change; reduce redundant 10-second idle peeks while keeping non-notifying external mail recoverable within five minutes. Do not change presence heartbeat cadence under this phase.
- Arm `wsdashboard/session_hire.go` expiry/liveness timers only while proposals are pending. Creation, decisions and relevant session changes poke the existing channel; an empty queue does not reread hires.json every second. External hire-file changes recover within five minutes.
- Verify empty idle read counts, exact deadlines, wake races, concurrent send/claim, deleted/exited recipients, cancellation, shutdown/restart and external edits with lost notifications. One passing full remote check and independent review close the phase.

### Phase 3: Git metadata gates and the daemon-startup burst (study P5)

Depends on Phase 1; independent of Phase 2.

- Trace and coalesce the actual startup git call chains before changing them: registrations' common-directory discovery, runner recovery, badges, rail worktree listing/admission and other observed roots. Cache immutable per-root discovery where safe; do not run status or duplicate common-dir discovery per recovered conversation when no visible consumer needs it.
- Gate badge/rail re-reads using worktree and common git metadata, including `.git` pointer files, linked-worktree HEAD/index/commondir, loose refs, packed-refs and worktree records. Keep per-root single-flight and explicit refresh, error/missing-path recovery and a forced complete safety refresh no later than five minutes for worktree contents the gate cannot cover.
- Stop display-only polling without visible consumers and refresh on resumption; preserve old clients' stream-lifetime behavior if they provide no visibility signal. Coordinate only the minimal visibility glue if required; do not alter unrelated frontend layout.
- Verify all ref/index/linked-worktree cases, inplace edits and untracked files, hidden/visible transitions, no-consumer startup, multiple sessions sharing a root, explicit refresh and restart. Count all injected git command launches in a reproducible cold-start fixture; compare to the 28-process capture only with clear workload/process attribution, not an invented numerical acceptance result.

### Phase 4: Finish separating clock/dependency reads from foreign-file safety work (study P6 remainder)

Depends on Phases 1–3.

- Keep frequent liveness/dependency checks in memory and direct broker wakes prompt. Separate the remaining refresh inputs owned by in-daemon writers from foreign watched paths in `pages.go`/`stream_cache.go`; do not stat all known state on each stream tick.
- Share and schedule foreign file reconciliation across readers within the hard five-minute bound, including tickets, docs and other legacy/non-notifying inputs. Keep explicit refresh, unknown/failed-input retry and sequence-number race recovery. Keep functional mailbox/nudge timers independent of page visibility.
- Verify no-op multi-stream idle probes, atomic replacements and inplace edits, no-wake changes recovered within five minutes, and no liveness/input-admission regression. Record the remaining conditional tracker/holder/observed pollers rather than changing heartbeat or OS notification policies implicitly.
