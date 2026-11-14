---
title: Harden proxy registration ownership against predecessor end writes
related:
  261114-bug-ws-shared-daemon-restarts-without-a-swap: coordinated daemon replacement and recovery lane
---

# Harden proxy registration ownership against predecessor end writes

## Background

Review sweep (794), pinned at `459fd070b2043fa45430eed12a1379f95fc630d5`, found an allowed replacement interleaving: an idle predecessor proxy stream closes, a successor restores its registration as live, and delayed predecessor `ServeStdio` cleanup writes `EndedAt` afterward. `dashboardWriteRow` protects a hosted takeover but does not distinguish proxy generations. The successor caches a non-idle registration, so an ordinary keyed call does not immediately rewrite the row; its heartbeat later repairs it.

Verified source: `internal/wsdashboard/proxy.go` `drainProxy`, `internal/mcp/server.go` `ServeStdio` cleanup, and `internal/mcp/dashboard_sessions.go` `dashboardReconnect`, `dashboardWriteRow`, and `dashboardEnd`. `Registration.Live` immediately rejects any end mark; the pinned `HeartbeatInterval` is two minutes. The source permits a wrong liveness interval after a successful reconnect. Runtime occurrence, distribution and exact duration were not reproduced or measured by the static sweep; do not report a guaranteed two-minute outage.

This is a known documented deferral, not a newly discovered undisclosed mechanism. Source `fb1074fb7` explicitly records that row locks do not fence proxy generations and that a successor repairs a late predecessor overwrite on heartbeat. The earliest changed repair landing in this sweep is `5a3758f36ae4681b9dedf4cd2f7fd66734134ab5`, through `fix/rail-reload-ended`; the underlying writer predates the sweep.

## Decisions

- MAIN requested a hardening capture in EXEC3's daemon-replacement lane. This ticket authorizes no patch, execution, replacement or status promotion.
- Compatibility target: **T0**, using an in-process ownership boundary without changing existing protocol, MCP arguments, persisted registration/store shape or field meanings. The precise mechanism remains unsettled; any broader cross-version surface requires renewed approval and tier adjudication before implementation.
- Preserve the documented self-healing heartbeat and original idle deadline while preventing a predecessor from undoing successor liveness. Do not silently reclassify this as permanent session loss.

## Constraints

- Keep hosted takeover protection, terminal session ownership and replacement policy separate. This capture does not authorize changing the approved build/swap/recovery design in the related ticket.
- Read `ai-docs/manuals/shipped-surface-boundary.md`, `ai-docs/manuals/release-compatibility.md`, and `ai-docs/manuals/ws-mcp.md` before matching tooling/MCP edits.
- Shared dogfood daemon operations remain MAIN-owned. Any eventual verification uses isolated state and controlled processes; heavy runs and remote work require their assigned authority.
- Sweep severity is adjudicated separately using actual user-visible liveness consequences and the existing recovery interval, not merely the existence of a documented deferral.

## Phases

### Phase 1: Preserve successor registration ownership through predecessor cleanup

Before readiness, ground the current writer and replacement lifecycle and settle the T0 in-process ownership mechanism through normal ticket review. Prevent delayed predecessor heartbeat/end writes from undoing a successor's restored registration. Retain valid current-owner end writes, hosted takeover protection and idle-age behavior. This is a hardening plan, not authority to add persisted generation fields or change compatibility semantics.

Verification expectations: deterministically hold predecessor cleanup, restore the successor, then release the stale write and inspect durable registration liveness. Cover the opposite ordering, legitimate current-owner end, delayed heartbeat, hosted takeover, original idle expiry and failed initial restoration write. Assert the observable row state without waiting for a later heartbeat to conceal the stale overwrite. Preserve emergency fixture cleanup and do not touch the shared daemon. Record runtime evidence and any residual recovery window before claiming implementation acceptance.
