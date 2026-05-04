---
name: lead-implement
description: Implementation harness. Routes to direct-edit or delegated write-code, then runs the shared doc pipeline, report/approval, and merge.
---

# Implement

Target: user request

## Invariants

- This skill harnesses - it routes and merges; it does not implement or review code itself.
- User approves the report before merge - no code reaches the target branch without user confirmation.
- Follow CLAUDE.md commit rules for the merge commit, including `## AI Context`.
- Task list is created at prepare and tracked to completion - no task may be skipped or reordered.

## On: invoke

### 1. Assess

Parse `user request`: extract ticket path or inline description.
If ticket-driven: read the ticket. Extract scope, stem, and existing artifact references.

Apply `judge: execution-mode`.

### 2. Prepare

1. Record current branch as `<original-branch>`.
2. Create task list. All tasks are mandatory - do not skip or reorder.

```text
[ ] Execute - invoke ws:lead-edit or ws:lead-write-code; capture commit range
[ ] Doc pre-pass - update-spec then mental-model-updater; commit each
[ ] Report to user - wait for approval; loop on tweaks
[ ] Merge to original branch (delegated path only)
[ ] Doc commit gate - executor-wrapup Doc Commit Gate and Ticket Update
[ ] Update project docs - refresh ai-docs/_index.md, ticket status
```

### 3. Execute

**Direct-edit:** Invoke `ws:lead-edit` with the target as arguments.

**Delegated:** Create `implement/<scope>` branch. Invoke `ws:lead-write-code` with the target as arguments.

Capture the commit range from the skill's completion report.

### 4. Doc pre-pass

1. Invoke `ws:lead-update-spec` with args `<commit-range>`. Lead-driven; runs inline.
2. Call `ws/agents.register(name: "mental-model-updater", prompts: ["mental-model-updater"])`, then call `ws/agents.call(name: "mental-model-updater", prompt: "Commit range: <commit-range>")`.
3. Wait for completion. Commit any file changes.

Run mental-model-updater after update-spec so it sees any implemented spec marker changes.

### 5. Report and approval

Report to the user:

- What was implemented from the edit/write-code completion report.
- Review result from edit's `Review:` line or write-code's reviewer summaries.
- Test status.
- Deviations or open items.
- If write-code escalated at cycle 3: list each unresolved dispute; the user decides fix or accept.

Wait for user approval. If tweaks requested:

- For direct-edit: apply fixes directly and re-verify.
- For delegated: call `ws/agents.call(name: "implementer", prompt: <block below>)`; re-review with the write-code reviewer prompt pattern.
- Re-invoke `ws:lead-update-spec` with the new commit range; re-dispatch mental-model-updater; commit each.
- Re-report. Loop until approved.

### 6. Merge (delegated path only)

Merge `implement/<scope>` to `<original-branch>` using the repository merge helper or the equivalent non-interactive git sequence.
The merge strategy is squash for one commit and `--no-ff` for two or more commits.
Compose the commit message per CLAUDE.md commit rules.

### 7. Doc commit gate

Call `ws/infra.read(name: "executor-wrapup")`. Follow Doc Commit Gate and, if ticket-driven, Ticket Update.
Do not re-run Doc Pipeline - update-spec and mental-model-updater already ran in step 4.

### 8. Update project docs

Refresh `ai-docs/_index.md` if new skills, agents, or major patterns were introduced.
Update ticket status if ticket-driven.

## Judgments

### judge: execution-mode

| Decision | When |
|----------|------|
| Direct edit -> `ws:lead-edit` | Change is confined to a single file AND purely internal (no callers affected, no new public symbols, no new test files) AND user has not explicitly requested delegation |
| Delegated -> `ws:lead-write-code` | Any condition above is unmet - cross-file touch, new public contract, new test file, or explicit delegation requested |

## Doctrine

Implement optimizes for **verified code reaching the target branch** - routing,
doc pipeline, and merge are the harness concerns; code quality is owned by
write-code and edit. When a rule is ambiguous, apply whichever interpretation
keeps harness logic out of the primitives and primitive logic out of the harness.
