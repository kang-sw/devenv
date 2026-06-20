---
title: Implement merge target discovery can select the wrong parent branch
related:
  260514-epic-ws-web-dashboard-mvp: dashboard dogfood exposed nested implementation branch merge risk
---

# Implement merge target discovery can select the wrong parent branch

## Background

Dashboard dogfood exposed a dangerous merge-target case: an `implement/*` branch
derived from `ws-dashboard-dev` attempted to merge toward `main`. The observed
branch ancestry did not support that target. For
`implement/ws-dashboard-workroot-registry-activation`,
`git merge-base ws-dashboard-dev implement/ws-dashboard-workroot-registry-activation`
returns the implement branch tip, while `git merge-base main
implement/ws-dashboard-workroot-registry-activation` returns an older shared
ancestor. A final-action merge target chosen as `main` would bypass the parent
dashboard branch and mix unrelated branch lifecycle boundaries.

The current `lead-implement` text says an `implement/*` branch should use a
caller-provided merge target or confirm before execution. The failure mode
suggests one of these gaps:

- the caller can provide a stale or default target such as `main`;
- the skill lacks a mechanical way to validate that a supplied target is the
  actual parent branch;
- dashboard-spawned or nested worktree flows may need branch ancestry metadata
  that survives the handoff.

## Proposed Direction

Treat "merge base hash" and "workflow merge target" as different contracts.
The runtime already exposes `git.merge_base`, which answers the read-only Git
question for two explicit revisions. That is useful for validation, but it does
not decide which branch is the intended parent when several branches contain or
pre-date the implementation branch.

Consider adding a higher-level read-only Git/workflow API that resolves a
candidate merge target for the current branch. Possible shape:

- inputs: `head` defaulting to `HEAD`, optional `candidates`, optional
  `prefer_upstream`, optional expected branch prefix;
- output: selected target branch, merge-base hash, confidence, evidence, and
  ambiguity reasons;
- safety: never defaults to `main` merely because it is the default branch;
- ambiguity: returns "needs confirmation" when multiple candidates are plausible
  or when the best candidate is only an old ancestor.

Skill guidance should then require `lead-implement` to validate any
caller-provided merge target before the Final Action Gate and stop for explicit
user confirmation when the target is not proven by branch ancestry or captured
handoff metadata.
