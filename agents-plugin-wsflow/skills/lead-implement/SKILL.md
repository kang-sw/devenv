---
name: lead-implement
description: Use when an approved task or ready ticket should be executed in wsflow; routes to direct edit, closes docs, then reports completion.
---

# Implement

Target: user request

## Invariants

- Lead-owned harness only: route, run doc pipeline, report, and gate continuation.
- Execute code changes through `wsflow:lead-edit`; `lead-edit` owns the implementation strategy.
- Honor caller-provided scope or phase slices as hard implementation boundaries.
- Do not route to excluded workflow skills.
- Create the task list at prepare; every task is mandatory and ordered.
- Commit logical units per repository commit rules with `## AI Context`.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, caller-provided slice, and phase results.
3. Record `<implementation-start>` with `git rev-parse HEAD`.
4. Apply `judge: branch-mode`.

### 2. Prepare

Create and maintain this task list:

```text
[ ] Confirm scope boundary - preserve caller-provided slice or whole-target scope
[ ] Prepare branch - continue current branch or create an implementation branch when requested
[ ] Execute - invoke wsflow:lead-edit; capture commit range and result commit
[ ] Doc pre-pass - invoke wsflow:lead-update-spec; update mental models directly when source changes require it
[ ] Doc commit gate - refresh ai-docs/_index.md and ticket Result, then commit docs
[ ] Final report - summarize commits, verification, review, deviations, and next slice
```

### 3. Execute

1. If branch preparation is needed, perform it before source edits.
2. Invoke `wsflow:lead-edit` with the target and scope boundary.
3. Capture commit range from `<implementation-start>..HEAD`.
4. Capture `<result-commit>` as current source HEAD before documentation updates.

### 4. Doc Pre-Pass

1. Invoke `wsflow:lead-update-spec` with `<commit-range>`.
2. Update relevant mental-model docs directly when the source diff changed module contracts, coupling, extension points, common mistakes, or technical debt.
3. Commit mental-model changes separately and include `(mental-model-updated)` in the commit body.

### 5. Doc Commit Gate

1. Call `wsflow/infra.read(name: "executor-wrapup")`.
2. Refresh `ai-docs/_index.md` for new skills, package surfaces, queue changes, or major patterns.
3. If ticket-driven, add a `### Result (<result-commit>) - YYYY-MM-DD` section to the completed phase.
4. Commit ticket and index changes through `wsflow/git.commit`.

### 6. Final Report

Report:

- implemented changes from edit output;
- documentation updates and ticket Result hash;
- review result from edit `Review:`;
- test status;
- deviations or open items;
- next unfinished phase, if any.

Stop after reporting. If the user wants more changes, route to a new
implementation slice; completed ticket Results are frozen.

## Judgments

### judge: branch-mode

| Decision | When |
|----------|------|
| Continue current branch | Current branch is suitable for direct edits or already matches the requested implementation scope. |
| Create implementation branch | The user explicitly asks for branch isolation or repository rules require it. |

## Doctrine

Implement optimizes for **verified code reaching the current workflow branch**.
The harness keeps routing and documentation explicit while `lead-edit` owns
implementation strategy, source integration, and review. When a rule is
ambiguous, preserve the caller-provided scope and keep mutation paths explicit.
