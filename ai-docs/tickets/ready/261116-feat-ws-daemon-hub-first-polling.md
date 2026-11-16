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
- Preserve all existing disk heartbeat intervals and crash/liveness behavior. The slower filesystem index scan must not slow liveness transitions: examine already-read registration/snapshot evidence in memory at the prior 30-second index cadence. When that evidence would expire/recover, schedule a targeted authoritative read rather than falsely expiring a legacy writer whose newer disk heartbeat was not notified. Already-stale, non-ended sessions retain the prior 30-second targeted recovery cadence so a legacy resume is not delayed to five minutes; ended rows remain on the safety/nudge path. Healthy hosted heartbeats move their snapshot deadlines directly in memory. This changes no writer heartbeat/flush policy and does not decide P3.
- Direct notification retains the HTTP path's existing registration/project check through Index.Nudge, but removes endpoint-file lookup and loopback HTTP. Snapshot hashing and expiry use one parsed copy; an in-memory handoff generation prevents an older in-flight source read from rolling back a newer hosted deadline. Neither mechanism is persisted or changes heartbeat policy.
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

## Blocked: Phase 1 unfiltered full-check acceptance

Phase 1 implementation and independent review are complete, but **phase closure is not claimed**: the required unfiltered full affected-package check is red at both the execution base and candidate. Await an explicit baseline waiver or a separately owned baseline repair before adding the phase Result. Do not begin Phase 2 on the strength of the qualified green check alone.

### Implementation checkpoint for Phase 1 - 2026-11-16

- Direct own-cache MCP/hosted-CLI notifications use the existing broker/index lanes without endpoint lookup or loopback HTTP, retaining registration/project validation and foreign/unsupported/stopped-host fallback. CLI board writes now use the existing whiteboard Poke callback. No new bus, ledger, wire field, manifest key or persistence schema.
- Default full index recovery is five minutes; healthy hosted snapshot clock checks remain memory-only at the prior 30-second cadence. Due/stale non-ended legacy entries retain targeted authoritative rechecks at that cadence so crash/resume behavior is not silently changed. Disk writer heartbeat schedules are unchanged.
- Snapshot hash, update time and expiry come from one parsed generation. The index retains a newer in-memory clock handoff across both existing-entry reads and first insertion, and clears it on authoritative removal. These are volatile hints, not registration authority.
- R1 found invalid direct success bookkeeping, delayed legacy recovery after stale state, and an in-flight clock rollback race. Fixed all three. Fixes-only R2 refined the same clock race for first insertion; that controlled interleaving was fixed too, and the reviewer's R2 continuation cleared it by inspection. No fresh sweep or new feature scope in R2.
- Full remote check: `go test ./internal/wsdashboard/... ./internal/mcp/... ./cmd/ws-mcp/... -count=1` failed ten tests. Remote comparison job `4ef2e4b6d8d5444391a61df3d6eb1c97` ran all three complete packages on an overlay of exact base `f18c773ab` (new files excluded), then candidate. All ten baseline failures reproduced; all ten new regression tests passed. The candidate additionally hit an unrelated test false positive because its generated reply ID contained the word `error`. The test now checks the expected `ids: (01)` success prefix rather than searching opaque URLs; no production behavior was changed to satisfy it.
- Qualified available-suite check: remote w5090 job `3fdde4f5d3be48dabef3d0b8ac12de5d` passed all three packages (207.569 s / 196.953 s / 136.327 s), excluding **only** the ten exact base-proven failures below via `-skip`. This is not an unfiltered green suite and is not a self-issued acceptance waiver.
- Base-proven exclusions: `TestCSSClassesDoNotCollideAcrossModules`, `TestDependencyPopoverCSSScope`, `TestPagesCarryTheTracker`, `TestPagesCarryTheToastStackAndTitleHost`, `TestRefreshCacheNonBoardWake`, `TestConfigListAgentProfileKnob`, `TestHostedConnectionSeeingALANChangeReplacesTheDaemon`, `TestLauncherRunsMailboxWaitThroughTheDaemonLikeTheDirectCLI`, `TestLauncherMailboxStreamEndsWhenPOSIXParentIsKilled`, `TestLauncherMailboxStreamSurvivesArrivalsAndTimeoutThenSignal`. They concern existing CSS/script assertions, the old memo skip counter, the builtin profile count, and daemon/launcher fixture shutdown/resource-sentinel failures. No attribution to absent node is invented.
- Comparable fixture evidence: 68 healthy hosted sessions over a five-minute window make **680 -> 68 source reads** after warmup (90% fewer safety reads); in-memory heartbeat/deadline handoffs add no probes. Direct-transport tests succeed without any endpoint file and coalesce duplicate queued notifications; old/foreign paths still use HTTP. This measures fixture reads, not owner-PC touches or total daemon savings. Stale legacy rows still recheck at 30 seconds, so their cost is deliberately not included in the saving claim.
- Forward: baseline repairs/waiver belong to the lead; no unrelated CSS/frontend/profile/launcher repair was folded into this phase. The startup git burst and git metadata gates remain Phase 3, mailbox/hire timers Phase 2, remaining foreign-path stamp work Phase 4. P3 heartbeat policy and P7 OS notify remain excluded.
- No local Go tests/builds, test executables, ws executables or launchers ran. No runtime replacement, merge or push. Temporary verification/review conclusions are preserved here; worker-created temporary files are removed at handoff.

## Phases

### Phase 1: Direct daemon notification and a five-minute index safety scan

- Add the smallest optional direct notification entry point on the hosting daemon, validating the state-directory boundary without an endpoint-file lookup or loopback HTTP round trip. Route MCP and hosted CLI store-write nudges through it when supported; preserve HTTP fallback for old/non-hosting/foreign-cache paths and notification outcome bookkeeping.
- Reuse the current per-session Index lanes and broker subscriptions so successful writes schedule targeted reads and consumers wake after the read lands. Keep existing snapshot/ring, mailbox and whiteboard Poke paths direct; wire hosted CLI board writes to the same Poke callback as hosted MCP.
- Change the default filesystem Index recovery scan from 30 seconds to a five-minute code constant. Keep initial load, missing-session discovery, removals, retries, concurrent writers and restart recovery. Retain 30-second registration/snapshot aging in memory, preserving the old liveness cadence. Only a due transition requests a targeted authoritative read; an ordinary healthy clock tick adds no probes. Do not change writer heartbeats.
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
