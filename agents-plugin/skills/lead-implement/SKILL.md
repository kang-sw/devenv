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
- Honor caller-provided `write-code` dispatch as a hard lower bound.

## On: invoke

### 1. Assess

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, artifacts, and caller-provided slice.
3. Extract caller-provided implementation dispatch, dispatch reason, and branch mode when present.
4. Apply `judge: execution-mode`.
5. Apply `judge: branch-mode`.

### 2. Prepare

1. Record `<current-branch>`.
2. Outside `implement/*`, set `<merge-target>` to `<current-branch>`.
3. On `implement/*`, set `<merge-target>` from the caller or confirm it before execution.
4. If continuing on `implement/*` and the branch name no longer matches the selected scope, rename with `git branch -m implement/<scope>` before execution; stop if the target branch exists or upstream tracking is ambiguous.
5. Create and maintain this task list:

```text
[ ] Confirm scope boundary - preserve caller-provided slice or whole-target scope
[ ] Confirm dispatch boundary - preserve caller-provided write-code lower bound
[ ] Prepare branch - create, continue, or safely rename the implementation branch
[ ] Execute - invoke ws:lead-edit or ws:lead-write-code; capture commit range and result commit
[ ] Doc pre-pass - update-spec then mental-model-updater; commit each
[ ] Doc commit gate - refresh ai-docs/_index.md, ticket status, then commit docs
[ ] Final action gate - wait for merge, continue, or stop
[ ] Merge - implementation-branch modes only and only when approved
```

### 3. Execute

1. Record `<implementation-start>` before creating or editing source.
2. Delegated outside `implement/*`: create `implement/<scope>` before any source edit.
3. Execute the selected implementation mode:
   - Direct edit: invoke `ws:lead-edit` with the target and scope boundary on the current branch.
   - Delegated: invoke `ws:lead-write-code` with the target, scope boundary, and any public contract or integration-test concerns found during assessment.
4. Capture commit range from `<implementation-start>..HEAD` plus the edit/write-code completion report.
5. Capture `<result-commit>` as current source HEAD before documentation updates.

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

### 6. Final Action Gate

Report:

- implemented changes from edit/write-code output;
- documentation updates and ticket Result hash;
- review result from edit `Review:` or write-code reviewer summaries;
- test status;
- deviations or open items;
- cycle-3 unresolved disputes, if any.

Wait for merge, continue, or stop. If the user wants more changes, route to a
new implementation slice or `ws:lead-sprint`; already completed phases capture
follow-up implementation through append-only ticket Result editions. Direct-current
mode exits after docs because no implementation branch exists.

### 7. Merge

Implementation-branch modes only. Merge only when the user approves. Merge
`implement/<scope>` to `<merge-target>` with the repository merge helper or
equivalent non-interactive git sequence.

Use squash for one commit; use `--no-ff` for two or more commits.
Write the merge commit per CLAUDE.md.

## Judgments

### judge: execution-mode

| Decision | When |
|----------|------|
| Delegated -> `ws:lead-write-code` | Caller-provided implementation dispatch is `write-code` |
| Direct edit -> `ws:lead-edit` | Single file, internal-only, no callers affected, no new public symbols, no new test files, and no explicit delegation request |
| Delegated -> `ws:lead-write-code` | Public interface, cross-module boundary, or new type contract changes require concrete contract and integration-test instructions |
| Delegated -> `ws:lead-write-code` | Any direct-edit condition is unmet |

### judge: branch-mode

Pick the first matching decision.

| Decision | When |
|----------|------|
| Stop or route sprint | Caller-provided branch mode is `sprint blocked` |
| Stop or route sprint | Current branch starts with `sprint/` |
| Continue implementation branch | Caller-provided branch mode is `continue implement/*` |
| Continue implementation branch | Current branch starts with `implement/` |
| Create implementation branch | Caller-provided branch mode is `create implement/<scope>` |
| Create implementation branch | Delegated path outside `implement/` |
| Direct current branch | Direct-edit path |

## Doctrine

Implement optimizes for **verified code reaching the target branch**. Routing,
doc pipeline, approval, and final action are harness concerns; code quality
belongs to write-code and edit. When a rule is ambiguous, apply whichever
interpretation keeps harness logic out of primitives and primitive logic out of
the harness.
