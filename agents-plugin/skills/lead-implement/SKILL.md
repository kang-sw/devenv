---
name: lead-implement
description: Use when an approved task or ready ticket should be executed. Routes to direct edit or delegated write-code, closes docs, then gates merge or continuation.
---

# Implement

Target: user request

## Invariants

- Harness only: route, run doc pipeline, report, and gate final action; do not implement or review code here.
- User approval gates merge or continuation.
- Merge commits follow CLAUDE.md commit rules and include `## AI Context`.
- Create the task list at prepare; every task is mandatory and ordered.
- Honor caller-provided scope or phase slices as hard implementation boundaries.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, artifacts, existing skeletons, and caller-provided slice.
3. Apply `judge: needs-skeleton`.
4. Apply `judge: execution-mode`.
5. Apply `judge: branch-mode`.

### 2. Prepare

1. Record `<current-branch>` and `<merge-target>`.
2. If continuing on `implement/*` and the branch name no longer matches the selected scope, rename with `git branch -m implement/<scope>` before execution; stop if the target branch exists or upstream tracking is ambiguous.
3. Create and maintain this task list:

```text
[ ] Confirm scope boundary - preserve caller-provided slice or whole-target scope
[ ] Prepare branch - create, continue, or safely rename the implementation branch
[ ] Resolve skeleton need - invoke ws:lead-write-skeleton on the implementation branch when required
[ ] Execute - invoke ws:lead-edit or ws:lead-write-code; capture commit range and result commit
[ ] Doc pre-pass - update-spec then mental-model-updater; commit each
[ ] Doc commit gate - refresh ai-docs/_index.md, ticket status, then commit docs
[ ] Report to user - wait for merge, continue, tweak, or stop
[ ] Final action - merge only when approved
```

### 3. Execute

1. Record `<implementation-start>` before creating or editing source.
2. Delegated outside `implement/*`: create `implement/<scope>` before any source edit.
3. If skeleton is required:
   a. Invoke `ws:lead-write-skeleton` with the target and skeleton reason.
   b. Capture the final skeleton commit hash from its completion output.
   c. Continue implementation on the same branch.
4. Execute the selected implementation mode:
   - Direct edit: invoke `ws:lead-edit` with the target and scope boundary on the current branch.
   - Delegated: invoke `ws:lead-write-code` with the target and scope boundary.
5. Capture commit range from `<implementation-start>..HEAD` plus the edit/write-code completion report.
6. Capture `<result-commit>` as current source HEAD before documentation updates.

### 4. Doc Pre-Pass

1. Invoke `ws:lead-update-spec` with `<commit-range>`.
2. Call `ws/agents.register(name: "mental-model-updater", prompts: ["mental-model-updater"])`.
3. Call `ws/agents.call(name: "mental-model-updater", prompt: "Commit range: <commit-range>")`.
4. Wait for completion; commit file changes.

Run mental-model-updater after update-spec so it sees implemented-marker changes.

### 5. Doc Commit Gate

1. Call `ws/infra.read(name: "executor-wrapup")`.
2. Refresh `ai-docs/_index.md` for new skills, agents, or major patterns.
3. If ticket-driven, follow Ticket Update using `<result-commit>`.
4. Follow Doc Commit Gate. Do not re-run Doc Pipeline.

### 6. Report And Final Gate

Report:

- skeleton draft/final commit hashes and ticket-skeleton update status when skeleton ran;
- implemented changes from edit/write-code output;
- documentation updates and ticket Result hash;
- review result from edit `Review:` or write-code reviewer summaries;
- test status;
- deviations or open items;
- cycle-3 unresolved disputes, if any.

Wait for merge, continue, tweak, or stop.

If tweaks requested:

- Direct edit: fix directly and re-verify.
- Delegated: call `ws/agents.call(name: "implementer", prompt: <block below>)`; re-review using write-code reviewer pattern.
- Re-capture `<result-commit>`, re-run documentation updates for the new range, and commit each.
- Re-report until approved.

### 7. Merge

Merge only when the user approves. Merge `implement/<scope>` to `<merge-target>`
with the repository merge helper or equivalent non-interactive git sequence.

Use squash for one commit; use `--no-ff` for two or more commits.
Write the merge commit per CLAUDE.md.

## Judgments

### judge: execution-mode

| Decision | When |
|----------|------|
| Direct edit -> `ws:lead-edit` | Single file, internal-only, no callers affected, no new public symbols, no new test files, and no explicit delegation request |
| Delegated -> `ws:lead-write-code` | Skeleton is required |
| Delegated -> `ws:lead-write-code` | Any direct-edit condition is unmet |

### judge: needs-skeleton

| Decision | When |
|----------|------|
| Skip | Ticket `skeletons:` already records a skeleton for this phase or scope |
| Skip | Small isolated change: single file, no new public contracts |
| Required | Public interface, cross-module boundary, or new type contract changes |

### judge: branch-mode

Pick the first matching decision.

| Decision | When |
|----------|------|
| Stop or route sprint | Current branch starts with `sprint/` |
| Continue implementation branch | Current branch starts with `implement/` |
| Create implementation branch | Delegated path outside `implement/` |
| Direct current branch | Direct-edit path |

## Doctrine

Implement optimizes for **verified code reaching the target branch**. Routing,
doc pipeline, approval, and final action are harness concerns; code quality
belongs to write-code and edit. When a rule is ambiguous, apply whichever
interpretation keeps harness logic out of primitives and primitive logic out of
the harness.
