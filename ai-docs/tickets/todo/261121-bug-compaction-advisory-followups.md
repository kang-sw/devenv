---
title: "컴팩션 권고 전달의 잔여 문제 해결"
---

# Compaction advisory delivery follow-ups

## Background

Audit residual advisory delivery risks against develop `4a363ebb0`. The dedicated idle advisory turn landed in `2c2f7eef7`; the missing Codex session and Claude worker `ws/compact` lever is being addressed separately and is outside this ticket. Evidence below describes that base, not an implementation decision.

- **(a) VERIFIED — thresholds; INFERRED — too late:** The registry exposes separate lead and worker advisory and hard keys (`agents-plugin-tool/internal/mcp/config_registry.go:249`); both default to 50% and 80% (`agents-plugin-tool/internal/wsconfig/compaction.go:11`). Two milestones divide the interval into thirds (`agents-plugin-tool/internal/wsagent/compaction.go:208`): under defaults, 60% and 70%. Percent and token combinations use the earlier point (`agents-plugin-tool/internal/wsconfig/compaction.go:17`, `:51`). On a 1M-token window these points are 500k, 600k, 700k and 800k. Whether those absolute amounts are too late for effective preparation on large-context models is unverified and needs evidence, including observation timing and harness behavior.
- **(b) VERIFIED — distinct Pi lead routing; INFERRED — advisory loss:** Hosted Pi uses the runner's native compaction descriptor (`agents-plugin-tool/internal/wsagent/pi.go:27`, `catalog.go:562`); its standalone advisory is sent as a normal Pi RPC prompt unless it is a typed `/compact` (`compaction.go:484`, `pi_process.go:16`, `:43`). The Pi extension disables its own threshold trigger for hosted sessions (`agents-plugin-pi/src/goal-loop.ts:1498`); non-hosted leads instead send their own custom preparation turns (`goal-loop.ts:1477`). A manual compaction without the Pi lever is cancelled and rerouted into a preparation turn (`goal-loop.ts:1639`, `:1794`). The cited path does not establish that the runner's advisory prompt is rerouted or lost; trace an actual hosted lead through the extension and its lever, including hard compaction, before concluding delivery failure.
- **(c) VERIFIED — resolved for advisory send failure:** Staging does not latch (`agents-plugin-tool/internal/wsagent/compaction.go:431`); standalone send returns on error and consumes/latches only after successful `Send` (`compaction.go:469`, `:484`). Ordinary input likewise returns on send error before consuming (`agents-plugin-tool/internal/wsagent/runner_conv.go:1285`, `:1304`, `:1321`). This resolves the reported advisory latch-before-send cause; do not reimplement it. Hard-point latching and native command failure are separate behavior (`compaction.go:356`, `:386`) and should not be conflated with that claim.
- **(d) VERIFIED — volatile state; INFERRED — lost delivery:** The pending nudge, usage and crossing latches are conversation-loop fields explicitly not persisted per observation (`agents-plugin-tool/internal/wsagent/runner_conv.go:82`); process start resets freshness (`agents-plugin-tool/internal/wsagent/compaction.go:159`), and idle checks depend on fresh usage (`compaction.go:348`). A daemon restart therefore loses those in-memory values. Whether a particular pending crossing is recovered by subsequent usage, duplicated, or never re-observed depends on recovery and events; establish the affected restart and daemon-swap scenarios before choosing behavior.

## Decisions

- Compatibility tier for any new persisted crossing/advisory state: **undetermined**, pending the storage and recovery decision in (2078). The present volatile fields introduce no shared persisted shape. Before promotion, grade the chosen old/new readers and writers and the new-version reopen path with source evidence; T0–T2 may ship in a patch, T3 requires a minor and explicit owner approval, and T4 cannot ship. Do not label an overwrite T2 without demonstrated reconstruction.

## Constraints

- Apply `ai-docs/manuals/release-compatibility.md` to every affected cross-version surface. Apply `ai-docs/manuals/performance.md` to any persisted data: choose its storage type first, maintain memory-first interactive reads and measure file operations and latency as required.
- Preserve the dedicated idle-turn fix and the resolved send-failure latch behavior. Coordinate with the separate lever work without assuming its mechanism or landing date.

## Open Decision Queue

- (2076) [open] What evidence and threshold policy should govern soft, milestone and hard points across model window sizes, roles and harnesses, including existing overrides and defaults?
- (2077) [open] After tracing hosted and non-hosted Pi leads end to end, what delivery or reroute behavior, if any, needs to change, including interaction with Pi's own preparation and lever?
- (2078) [open] What recovery guarantee should pending and delivered crossings have across daemon swaps and restarts, and should it use persistence, reconstruction, or another mechanism? If persisted, choose the storage type and settle the directional compatibility tier with old/new read, write and reopen evidence.

## Phases

### Phase 1: Resolve residual advisory delivery risks

After the open decisions are settled, implement the chosen threshold, Pi routing and restart behavior warranted by the evidence. Verify threshold crossings and overrides across representative context windows, hosted and non-hosted Pi lead delivery and rerouting, failed sends, and restart/swap recovery including duplicate and missed advisories. Exercise cross-version recovery and measure persistence costs where applicable. Keep the separate lever work outside this phase.
