---
title: "개발 데몬의 승인 빌드 복구 및 긴급 롤백"
related:
  261114-bug-ws-shared-daemon-restarts-without-a-swap: prerequisite
---

# Accepted-build recovery and emergency rollback for the development daemon

## Background

This follow-up carries former Phases 2–3 of
`261114-bug-ws-shared-daemon-restarts-without-a-swap`. The parent implements
Phase 1 only: ordinary connection reuses a compatible selected runtime without
source-drift builds or equal-core replacement, while the existing dev.py command
owns explicit build and swap. It does not establish accepted-build recovery.

The owner's recovery and rollback requirements below remain confirmed, but their
state, probe and authorization design needs review before this ticket is ready.
Phase 1 here depends on the parent's implementation landing; Phase 2 here depends
on this ticket's accepted-build recovery operation from Phase 1.

## Decisions

- **(706)=a:** keep dead-daemon recovery separate from replacing a running
  daemon. Automatically restart a dead daemon using the last-known-good build,
  never an unaccepted newer candidate. Roll back a designated swap that fails
  its health checks, and record the recovery in the board log.
  Source: owner via MAIN.
- **(709)=a:** a candidate earns last-known-good status after post-start checks
  covering hello, session lookup, whiteboard read and mailbox receive, followed
  by approximately 30 minutes of quiet observation before promotion. Startup
  liveness alone does not establish session-feature correctness.
  Source: owner via MAIN.
- **(709)=a:** emergency rollback must be available through a launcher CLI
  command even when the dashboard is broken, plus a dashboard button visible
  only for development mode identified by `.local-devenv-runtime`.
  Source: owner via MAIN.
- Compatibility target **T0**: preserve existing release/protocol/store meanings;
  use additive recovery state and command surfaces if needed. Do not remove or
  reinterpret an existing protocol, manifest or tracker field. Any broader
  cross-version change stops for MAIN's approval before implementation. This is
  a target, not the parent's settled Phase 1 verdict: recovery-state readers,
  writers and old-version coexistence must be specified here with source evidence.

## Constraints

- Design capture only until the open questions below are settled and independent
  design/completeness review clears promotion. Shared-daemon activation remains
  MAIN's coordinated swap; verification must use isolated fixtures only.
- Recovery self-checks must not consume real unread user mail or overwrite
  user/session/board state. Specify safe probe ownership and cleanup before
  implementing the mailbox-receive check.
- Keep previous accepted binaries available for rollback; current staged-binary
  pruning is not a last-known-good retention guarantee. Recovery state must
  distinguish an accepted build from a candidate and a merely healthy process.
- Do not claim that a quiet interval proves every session feature correct.
  Technical design review must define failed-check handling, promotion timing,
  concurrent launcher ownership, interrupted swaps and bounded restart behavior.
- Preserve released-version compatibility and distinguish missing, dead, hung,
  incompatible and newer running daemons. Do not broaden automatic replacement
  authorization beyond the owner's confirmed recovery/swap policy.
- Mail durability and large-request failures remain separate investigations.
- Before launcher/tooling edits read `ai-docs/manuals/shipped-surface-boundary.md`,
  `ai-docs/manuals/release-compatibility.md`, and `ai-docs/manuals/ws-mcp.md` where
  applicable. Keep launcher copies in ws, wsflow and Pi consistent; read
  `ai-docs/manuals/wsflow-mirroring.md` for mirrored plugin edits. Consult Pi SDK
  integration documentation before changing Pi bootstrap behavior.

## Open questions for design review

- Recovery-state persistence: where are candidate and accepted-build identities
  stored, which versions read/write them, and how do they survive binary pruning,
  launcher restarts and old-version coexistence without losing accepted state?
- Ownership and interrupted swaps: which caller owns activation, probes, promotion
  and recovery; how are concurrent launchers serialized, an interrupted swap
  resumed, and repeated restart attempts bounded?
- Safe probes: how are disposable hello/session/whiteboard/mailbox identities
  owned and cleaned up without consuming real unread mail or changing real
  session/board state? What failures trigger rejection versus retry?
- Quiet observation: what starts, resets, persists or interrupts the approximately
  30-minute interval, who observes it across process restarts, and what precise
  failure/promotion rules apply? Liveness is not session-feature correctness.
- Guarded rollback authorization: what launcher CLI command and dashboard action
  invoke the shared recovery operation, which caller/token may authorize it,
  and how is the development marker validated when the dashboard is unavailable?
- Rollback target validation: how are retained accepted binaries validated for
  identity, compatibility, provenance and availability; what happens for unsafe,
  missing, incompatible or newer-version targets, and how is recovery recorded
  in the board log without depending on a working candidate dashboard?

## Phases

### Phase 1: Add accepted-build recovery and guarded promotion

Implement (706)/(709): retain a last-known-good runtime, check a designated
candidate through the approved session-facing operations, observe approximately
30 quiet minutes before promotion, recover a dead daemon with the accepted build,
and roll back failed candidate activation. Preserve accepted binaries through
staging/pruning and record recovery in the board log. Define internal recovery
state, safe probes and concurrent-operation behavior through technical design
review before promotion to ready; do not infer these mechanisms from old liveness
checks or claim they already exist.

Verify deterministically with isolated state, disposable probes and controlled
time: successful checks, failed checks, interrupted startup, rejected promotion,
failed candidate, retained accepted binary, dead-daemon recovery, and concurrent
callers. No test may stop or replace the shared dogfood daemon.

### Phase 2: Expose emergency rollback outside the dashboard

Provide the approved launcher CLI rollback command and development-only dashboard
button. Both target the retained accepted build through the same guarded recovery
operation; the CLI must work while the dashboard is unavailable. Preserve a usable
recovery path when the candidate UI or session controls regress.

Verify CLI behavior with a broken/unreachable dashboard, development-marker UI
visibility, rejected unsafe/missing rollback targets, recovery log visibility and
consistent operation from both controls. Run independent design/completeness and
implementation reviews before integration or activation; record exact commands,
results, compatibility boundaries and remaining acceptance gaps.
