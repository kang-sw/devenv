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
- Light design/completeness check: current direct/HTTP writer paths, broker subscriptions, index lanes and recovery seams support the bounded phases below. The owner explicitly chose the architecture and safety bound. Phase 1 is accepted. The current run implements Phase 2 only; Phases 3–4 remain ready.

- Phase 2 spine: extend the existing mailbox hub with opt-in coalesced store-path invalidations. Publish only after a successful queue change, comparing queues already held in the writer lock; presence/contact-only heartbeats publish nothing and keep their existing cadence. Ordinary wait subscribers retain no path history and retain their existing hard-wake policy.
- Phase 2 scheduling: discover all queue stores at startup and every five minutes, then recheck only changed stores or their earliest pending deadline under existing locks. Keep failed-notice retry at 30 seconds and let earlier future deadlines win. Resolve cache/config authority once; foreign-cache/config store hints cannot broaden it. Re-read the existing undelivered threshold per processing batch and at safety recovery, with no new config key.
- Phase 2 wait/hire policy: wake-backed waits use a five-minute lost-notification backstop (standalone non-wake polling unchanged). Hire reconciliation is poked by writes/session changes, armed for pending expiry/liveness or failed-side-effect retry, with a five-minute external-file scan even when empty. No heartbeat or crash-latency policy change.
- Phase 2 review correction: startup approval replay fails closed until the requester is indexed and no registration/source read remains uncertain; `Loaded()` alone permits a partial refresh. Track failed registration reads from the listing's existing read, without an additional probe, and retain approval intent for the existing active-work retry. The public best-effort registration listing remains unchanged.

### Compatibility by cross-version surface

- **In-process notification interfaces: T0.** Optional Go-side notification capability on the hosting daemon is not serialized. Existing hosted snapshot-slot APIs remain valid; absent capability falls back to the existing HTTP path. Replacement daemons reconstruct the index from unchanged files.
- **HTTP `/nudge`, MCP tools and proxy hello: T0, unchanged.** Retain the existing body (`reply_id`, `project`), route status/validation and old caller behavior (`wsdashboard/daemon.go`, `handleNudge`; `index.go`, `Nudge`; `proxy.go`, hello validation). No new mandatory field, renamed tool or protocol/version boundary.
- **Registration, snapshot, mailbox/inbox and hire stores: T0, unchanged serialized schemas/write formats.** Old/new binaries read and write the same fields; legacy non-notifying writes recover within five minutes. Phase 2's `wsmailbox.WithLock`/`WithReplyLock` publish in-memory queue-path hints only after their existing atomic replacement; `UndeliveredSweep.Sweep` retains the existing receipt field and queue locks, and `saveHires` retains the same proposal journal. Deadline maps and index read-uncertainty tracking are memory-only, reconstructed at startup; old readers see no added field and old writers need no hook. Disk heartbeat schedules and `Registration.Live`/`Snapshot.Stale` semantics remain unchanged; no lost-field reconstruction or migration is needed. Sources: `wsdashboard/registry.go`, `wslive/snapshot.go`, `wsmailbox/{store,replyid,undelivered}.go`, `wsdashboard/session_hire.go`.
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
- Change the default filesystem Index recovery scan from 30 seconds to a five-minute code constant. Keep initial load, missing-session discovery, removals, retries, concurrent writers and restart recovery. Retain 30-second registration/snapshot aging in memory, preserving the old liveness cadence. Only a due transition requests a targeted authoritative read; an ordinary healthy clock tick adds no probes. Do not change writer heartbeats.
- Verify direct delivery with no endpoint file/HTTP request, foreign-cache and absent-capability fallback, coalescing, subscribers seeing semantic changes, no per-tick store reads, clock expiry/recovery, missed notification/external mutation recovery and restart reconstruction. Run full affected Go package checks remotely; report fixture read/spawn deltas, not unmeasured owner-PC savings.

### Result (4feda9b3b) - 2026-11-16

- Accepted Phase 1 implementation `6835795f8`, merged as `4feda9b3b`: direct own-cache MCP/hosted-CLI notifications reuse the broker and targeted index lanes, preserving registration/project validation and HTTP fallback. Hosted CLI board writes reuse the existing Poke callback. No new wire/store/config shape or ledger (T0).
- Full index safety recovery is five minutes. Healthy hosted snapshot deadline handoffs are in memory; due/stale non-ended legacy entries retain targeted 30-second authoritative rechecks, preserving crash/resume behavior without changing heartbeat writers. Hash/update/expiry use one parsed generation; handoffs survive concurrent reads and first insertion.
- Independent R1 and fixes-only R2 cleared registration outcome, legacy recovery and snapshot-generation fixes. Comparable healthy 68-session fixture: **680 -> 68 source reads per five minutes** after warmup; direct transport tests need no endpoint file. This is fixture evidence, not an owner-PC touch measurement.
- Owner waived the ten base-proven failures and merged Phase 1; baseline repair is now merged as `2a7881931`. Final baseline remote job `8d2856cab0f3449ca0c77891cd2311cd` ran unfiltered `go test ./... -count=1`: every package passed except the two Node-only tests below. The separate final `cmd/ws-mcp` package run passed in 144.513 s (job `34f305d0e7594f5596e9d18f540d8cde`). The former full-check blocker is resolved; details are in `ai-docs/ref/261116-baseline-test-repair.md`.
- Forward (owner accepted): `TestTopicStateBadgeResearchingMetadata` and `TestTopicStateBadgeClockAndReconciliation` require Node, absent on w5090. Retain their guards; these two environment-only reds do not block phase acceptance.

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
