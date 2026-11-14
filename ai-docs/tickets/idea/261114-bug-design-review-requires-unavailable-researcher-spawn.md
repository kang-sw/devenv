---
title: "Design reviewer requires researcher spawning unavailable at its delegation depth"
related:
  261113-feat-ws-whiteboard-board-frontend-performance: "encountered during the mandatory Phase 3 verification-accommodation design gate"
---

# Design reviewer requires researcher spawning unavailable at its delegation depth

## Background

During delegated ticket correction on 2026-11-14, the xlarge design reviewer for `261113-feat-ws-whiteboard-board-frontend-performance` reported that its governing playbook requires dispatching the `explore` researcher through the native spawn mechanism, but its tool surface exposes neither spawn nor send. It declined to approve the design despite finding the scoped accommodation coherent. The calling fork preserved the incomplete gate without stamping or committing.

Evidence: root session `subtext-pound-cytoplasm`, correction fork `448f1670-bb86-4aab-adcc-a48e05950f57`, reviewer `f7145ae7`, and the fork's `f149a973-01-performance-phase3-resumption-gate.md` artifact. The missing capability was reported, not independently reproduced by the root. No downgrade, prompt bypass or substitute approval was attempted. Root-direct reviewer dispatch is the bounded recovery to evaluate; this ticket authorizes no harness change.

This differs from the already captured trusted-shipped-provenance render rejection: rendering succeeded here, but a mandatory procedural dependency was unavailable in the resulting reviewer.

## Constraints

- Keep the mandatory design gate incomplete until an appropriately capable reviewer actually completes it.
- Preserve capability ceilings and evidence boundaries; do not silently skip mandatory research or interpret concern as approval.
- No workflow, API or delegation-depth change is selected by this idea capture.

## Phases

### Phase 1: Investigate reviewer procedure and dispatch capability compatibility

Trace the rendered design procedure and actual capability/depth assignment for this incident. Determine why a reviewer could be dispatched with a mandatory unavailable dependency. Report the smallest compatible correction and regression coverage before proposing implementation or changing workflow semantics.
