---
title: Settled Pi executor cannot resume because its owned session home is unavailable
---

# Settled Pi executor cannot resume because its owned session home is unavailable

## Background

During MAIN mockup dogfooding on 2026-11-14, executor `cd7c5264-72f5-4506-9421-c0d0c3365642` settled normally with `reason: idle` and delivered a complete cache-only artifact report. A subsequent `ws-agent-send` continuing that same assignment returned exactly `ws-pi-agent: owned session home is unavailable during resume` instead of resuming it.

The lead guide promises that settled agents park without losing their resumable identity. The evidence establishes a failed continuation, not why its owned home became unavailable or whether this is related to the separately tracked fork-lineage defect. No transcript deletion, key rewrite, or repair probe was performed.

A replacement execute-worker `6851cb21-99bc-40f4-98ae-40380a288424` received the prior artifacts and the owner's requested revision. The mockup task continues; the original executor was not repaired.

### Second occurrence — EXEC 1, 2026-11-14

At 06:55:21Z, peer EXEC 1 reported that continuing settled read-only precheck executor `6154f4ea-cd82-4f8e-8750-31ad7f994ce4` through `ws-agent-send` returned the same exact error: `ws-pi-agent: owned session home is unavailable during resume`. This is a peer-reported second instance, not an independently reproduced cause.

The continuation was intended to start immutable-source preparation for commit `28d8576226cc62312bf2d85d9c4cb306882f3df0`. It failed before any preparation command ran; the prior read-only precheck evidence remains available. EXEC 1 is using a replacement gated execute-worker, without a fork, repeated incident probes, harness repair, or daemon activation.

### Third continuation occurrence — EXEC 1, 2026-11-14

At 07:27:13Z, EXEC 1 reported that `ws-agent-send` to settled availability-check executor `ac764368` returned exactly `ws-pi-agent: owned session home is unavailable during resume`. No new command ran: the authorized one-file metadata copy and single offline graph attempt had not started. EXEC 1 is using a replacement gated executor with the same finite scope, without a fork, retry, or repair probe. This is a peer-reported continuation failure, distinct from the transcript-access observation below; a common cause remains unproven.

### Related evidence-access observation — EXEC 1, 2026-11-14

At 07:23:22Z, EXEC 1 reported that the transcript-pointer tool for settled, quiescent preparation executor `1b5e42c1` returned `history unavailable — owned session home is busy, gone, or unreadable; retry the reference`. The tool was not retried. This is a peer-reported evidence-access failure, not another confirmed continuation failure or proof that the owned home was deleted; its relationship to the earlier error remains unproven.

The retained provenance is EXEC 1's transcript at `/Users/kang-sw/.cache/ws@kang-sw-devenv/dashboard/sessions/suspectcaboosetalonunretired/pi/session.jsonl`, containing the executor's `Dependency-prep result:` and `Terminal report:` messages and approved command ID `call_ad6ecf3f229c483482fc8057590014cb|fc_0c1eec8117dc4b60016af80bd4a48887d09fd5399ac06d1360`. No further history probe, harness investigation, or repair was authorized by recording this observation.

### EXEC 3 continuation occurrence — 2026-11-14

EXEC 3 directly observed `ws-agent-send` to settled execute-worker
`fc0209c4-5d5c-429e-862a-ba618424c27d` return exactly
`ws-pi-agent: owned session home is unavailable during resume`. That worker
had settled normally after identifying a production-scope blocker, with no
source edits or tests. The attempted continuation carried MAIN's subsequently
approved static `Event.CommandError` plumbing scope; no continuation command
ran.

Replacement gated execute-worker `f898037a-b07f-4e60-94f5-8f5ce384eca6` received
the updated ticket, immutable plan and static-only scope. No repeated resume,
capability workaround, ownership-record edit, bridge/daemon activation or
harness repair was attempted. This confirms another failed continuation,
not the owned home's lifetime failure mechanism.

### Fourth continuation occurrence — EXEC 1, 2026-11-14

After MAIN's 08:39 direction, EXEC 1's root `ws-agent-send` to settled operational executor `2dacc5af-1cc8-4739-9ce0-dcb8e390fa29` returned exactly `ws-pi-agent: owned session home is unavailable during resume`. The requested continuation was text-only transaction command preparation, not execution. No retry, probe or foreign-key bypass occurred, and no command, allocation or runtime action ran.

EXEC 1 instead successfully sent command-text preparation to the same retained implementer `09ac01b1`. This is another reported continuation occurrence, not evidence of a proven common cause. Recording it adds no diagnosis or repair scope.

## Constraints

- Preserve the original session and ownership records before diagnosing cleanup or lifetime behavior.
- Do not conflate execute-worker continuation with native fork parent-key loss without evidence.
- Compatibility tier is unsettled; this capture authorizes no store or protocol change.

## Phases

### Phase 1: Diagnose owned-home lifetime after normal settlement

Trace the settled executor's retained registry and owned session-home lifecycle. Determine whether cleanup, relocation, ownership validation, or recovery caused the refusal. Define the smallest repair and a regression test for normal settle, park, and same-agent continuation before implementation approval.
