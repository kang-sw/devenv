---
title: "Pi adapter: stopped reviewer can leave worker final gate outstanding"
related:
  260912-feat-ws-pi-recursive-worker-subtree-lifecycle: owns descendant completion accounting
  260908-feat-ws-pi-agent-session-disk-retention: observed during its fix-only review closure
---

# Pi adapter: stopped reviewer can leave worker final gate outstanding

## Background

Observed 2026-09-13 during the retention worker's closure at `045da319`.
The reviewer returned a normal report with a CLEARED verdict. The worker
consumed that report, materialized its artifact, and called `ws-agent-stop`.
`ws-agent-list` then showed the sole reviewer as dormant. Nevertheless,
`ws-report-to-lead(kind: "final")` rejected completion with:

> final rejected while child results are outstanding; consume their reports,
> follow up or explicitly stop them, then submit a fresh final

A second explicit stop followed by a fresh final produced the same rejection.
The worker then continued the same reviewer to request a terminal acknowledgement
rather than starting another review. The exact outstanding counter has not been
diagnosed; a queued delivery or stale obligation may differ from registry status.

## Phases

### Phase 1: Reconcile stopped-child disposition with the final gate

Reproduce normal report -> consume -> explicit stop -> parent final with one
reviewer, including report delivery racing stop and repeated stop on a dormant
child. Inspect `publishSubtree`/`assertSubtreeFinal` in
`agents-plugin-pi/src/subtree-lifecycle.ts` and the stop/delivery transitions in
`agents-plugin-pi/src/spawner.ts`. Determine which obligation remains and either
settle it through the documented explicit-stop path or expose an actionable
reason. Preserve the final gate for genuinely outstanding descendants; do not
weaken it merely because an agent is dormant. Add a regression for the confirmed
cause. No runtime fix is authorized by this capture alone.
