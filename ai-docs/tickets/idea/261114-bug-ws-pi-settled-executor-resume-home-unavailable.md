---
title: Settled Pi executor cannot resume because its owned session home is unavailable
---

# Settled Pi executor cannot resume because its owned session home is unavailable

## Background

During MAIN mockup dogfooding on 2026-11-14, executor `cd7c5264-72f5-4506-9421-c0d0c3365642` settled normally with `reason: idle` and delivered a complete cache-only artifact report. A subsequent `ws-agent-send` continuing that same assignment returned exactly `ws-pi-agent: owned session home is unavailable during resume` instead of resuming it.

The lead guide promises that settled agents park without losing their resumable identity. The evidence establishes a failed continuation, not why its owned home became unavailable or whether this is related to the separately tracked fork-lineage defect. No transcript deletion, key rewrite, or repair probe was performed.

A replacement execute-worker `6851cb21-99bc-40f4-98ae-40380a288424` received the prior artifacts and the owner's requested revision. The mockup task continues; the original executor was not repaired.

## Constraints

- Preserve the original session and ownership records before diagnosing cleanup or lifetime behavior.
- Do not conflate execute-worker continuation with native fork parent-key loss without evidence.
- Compatibility tier is unsettled; this capture authorizes no store or protocol change.

## Phases

### Phase 1: Diagnose owned-home lifetime after normal settlement

Trace the settled executor's retained registry and owned session-home lifecycle. Determine whether cleanup, relocation, ownership validation, or recovery caused the refusal. Define the smallest repair and a regression test for normal settle, park, and same-agent continuation before implementation approval.
