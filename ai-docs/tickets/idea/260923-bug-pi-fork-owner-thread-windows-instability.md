---
title: Investigate unstable fork owner-side question threads on Windows
---

# Investigate unstable fork owner-side question threads on Windows

## Background

During Windows dogfooding, the owner-side thread opened when a task fork asks its owner a question was reported to be buggy and unstable. The user suspects an interaction when a fork starts `ws-execute`, but the causal relationship has not been established. Avoid conflating the fork question thread with the separate execute-worker approval path or the existing accepted-approval hang (`260923-bug-pi-execute-approval-accepted-worker-hangs`).

Current inspection found no dedicated config switch to disable only fork-originated owner threads. Suppressing `ws-fork` avoids new task-fork threads but also disables all task forks. The question callback registers a `fork-raised` thread (`agents-plugin-pi/src/ask.ts` and `src/index.ts`); an execute-worker spawned by a fork routes approval through that fork's own process (`src/spawner.ts` and `src/execute-gateway.ts`). No Windows reproduction has been captured for this report.

## Phases

### Phase 1: Reproduce and isolate the Windows failure

Compare a fork owner-question/reply and a fork-spawned `ws-execute` approval separately, then together, using harmless commands and capturing thread, approval, and process lifecycle events. Determine whether the failure is in owner-thread binding/delivery, nested approval delivery, or an independent command/decision-file path; record the Windows environment and the loaded extension version. Preserve the ability to stop a stalled child without replaying an ambiguous command. Decide the smallest correction or a separate follow-up ticket after the cause is known; a selective disable control is an option to evaluate, not a confirmed design decision.
