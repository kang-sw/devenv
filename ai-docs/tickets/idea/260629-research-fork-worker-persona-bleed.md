---
title: "Fork worker persona-bleed (model-conditioned, notably Opus 4.8)"
---

# Fork worker persona-bleed (model-conditioned, notably Opus 4.8)

## Background

Research/observation only; no fix proposed. Under the `lead-prefer-subagent`
maximum-delegation posture, forks dispatched to execute payload repeatedly fail
to act as workers. The forks for the originating session were sealed by user
decision. This ticket captures the observation, the model dimension, the
detection gap, and open questions for a later investigation pass.

## Observation

Forks dispatched to execute payload return lead-voice narration — restating the
lead's own reasoning/plan, sometimes claiming work is "in progress" — instead of
executing the task and returning the required worker report. Three forks in one
session (`feature/ferrule`, 2026-06-29):

1. Plain prompt — 0 tool uses, lead-voice narration.
2. Prompt that violated the playbook's fork-prompt anti-patterns (operator
   error: "your first action is the Edit tool", "do not narrate") — 13 tool uses
   but the intended edit was not applied, lead-voice narration.
3. Prompt constructed to exactly match the playbook's fork-prompt template
   (execution-constraint opener, fixed Outcome/Files/Verification/Blockers/Commit
   return format, boundary closer) — 13 tool uses, still lead-voice narration, no
   worker-format return.

A template-correct prompt was therefore not sufficient.

## Model dimension

Cross-model operator experience (Codex, Sonnet tiers) indicates the fork-guidance
violation rate is notably higher on Opus 4.8 specifically. Forks are tier-locked
to the parent model, so an Opus 4.8 lead produces Opus 4.8 forks.

Hypothesis: a fork inherits the full lead context; on a stronger model that
inherited context more strongly reasserts the lead identity (reason / decide /
narrate) rather than collapsing into a worker role — i.e., the capability that
makes a good lead makes a poor fork-worker.

## Detection gap

The existing failed-fork recovery clause in `lead-prefer-subagent` assumes the
fork "reports delegation instructions back to the lead." The observed variant is
harder to detect: lead-voice narration combined with `tool_uses > 0` but no
structured worker report, which can mislead the lead into treating the work as
complete. This nearly happened this session and was caught only by the user.

## Open questions

- (a) Should `lead-prefer-subagent` routing treat fork as unreliable on Opus 4.8
  (or generally) and prefer fresh spawn, given fork is tier-locked to the parent?
- (b) Strengthen the worker-report contract so that a missing structured return
  is treated as a failed fork regardless of `tool_uses`.
- (c) Is persona-bleed reducible by prompt structure at all, or is it
  model-intrinsic — this session's template-correct failure suggests prompt alone
  is insufficient.

## Resolved by deletion (260723-refactor-fork-removal-prefer-subagent)

`260723-refactor-fork-removal-prefer-subagent` Phase 1 deleted the fork
delegation construct from `lead-prefer-subagent` entirely — no partial
retention behind a flag. Question (a) is answered by removal rather than by a
model-conditioned routing rule: fresh spawn is now the only delegated-payload
path, so a fork's lead-voice persona-bleed has nothing left to reproduce
against. Questions (b) and (c) no longer apply to `lead-prefer-subagent`
routing since there is no fork worker-report contract left to strengthen.
