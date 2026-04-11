---
name: implement
description: >
  Single-scope implementation cycle (implementer + reviewer pair).
  Use for focused work behind locked contracts.
argument-hint: "<plan-path or inline brief> [--ticket <ticket-stem>]"
---

# Implementation

Target: $ARGUMENTS

## Invariants

- This skill delegates — the lead does not read source code or write implementation.
- When skeleton exists, its stubs and integration tests are the acceptance criteria.
- The implementer and reviewer communicate directly; the lead receives only final reports.
- User approves the report before merge — no code reaches the target branch without user confirmation.
- Teammates (implementer, reviewer) stay alive until after doc pipeline completes; cleanup is the final step.
- One delegation cycle per invocation.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).
- Task list is created at prepare and tracked to completion — no task may be skipped or reordered.
- Planning research, when triggered, produces ephemeral implementer context — not a persistent document.
- `/team-lead` skill must be loaded before any team operations.

## On: invoke

### 0. Prerequisites

1. Load `/team-lead` skill if not already loaded.

### 1. Prepare

1. Parse arguments: extract plan path or inline brief, and optional ticket stem.
2. If plan-driven: verify the plan file exists. Read it to extract scope and branch name hint.
3. If brief-driven: the brief is the full specification.
4. Verify skeleton exists: grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs or check for integration tests that reference the target contracts. If absent, stop and suggest `/write-skeleton`.
5. Collect integration test context: identify test file paths and the command to run them. This flows into the implementer spawn prompt.
6. Record current branch as `<original-branch>`. Create `implement/<scope>` branch.
7. If already in a team context, use the existing team. Otherwise create one:
   ```
   TeamCreate(team_name = "impl-<scope>", description = "<brief scope>")
   ```
8. Create task list. All tasks are mandatory — do not skip or reorder.
   ```
   [ ] Spawn implementer — wait for completion report
   [ ] Spawn reviewer — implement → verify → review loop until clean
   [ ] Report to user — wait for approval
     > if tweaks requested: implementer fixes → re-verify → reviewer re-reviews (loop)
   [ ] Merge to original branch
   [ ] Dispatch mental-model-updater — wait for completion
   [ ] Update project docs — refresh ai-docs/_index.md, ticket status
   [ ] Cleanup — shut down teammates, delete team
   ```

### 2. Spawn implementer

```
Agent(
  name = "implementer",
  description = "Implement plan on branch",
  subagent_type = "implementer",
  model = "sonnet",
  team_name = "impl-<scope>",
  prompt = """
    Lead name: <lead-name>
    Mode: <A: plan-driven | B: inline brief>
    <Plan path | Brief text>

    Acceptance criteria: skeleton integration tests must pass.
    - Test files: <integration test paths>
    - Run: <command to execute them>

    Team rules:
    - Verify integration tests pass before reporting completion or after each fix.
    - Report completion to the lead via SendMessage. Include test results.
    - The reviewer may message you directly with findings — fix, re-verify tests, and reply.
    - Commit at logical checkpoints on the current branch.
  """
)
```

Wait for the implementer's completion report. Note the commit range.

### 3. Spawn reviewer

```
Agent(
  name = "reviewer",
  description = "Review implementation diff",
  subagent_type = "reviewer",
  model = "sonnet",
  team_name = "impl-<scope>",
  prompt = """
    Lead name: <lead-name>
    Implementer name: implementer
    Diff range: <first-commit>..<last-commit>

    Team rules:
    - SendMessage findings to the implementer by name.
    - The implementer fixes and notifies you — re-review until clean.
    - SendMessage the final report to the lead.
  """
)
```

The reviewer and implementer iterate directly. Each iteration:
implementer fixes → implementer verifies integration tests pass →
reviewer re-reviews. Loop until the reviewer reports clean. Wait for
the reviewer's final report to the lead.

### 4. Report and approval

1. Report to the user:
   - What was implemented (from implementer report)
   - Review result (from reviewer report)
   - Test status
   - Any deviations or open items
2. Wait for user approval. If the user requests tweaks:
   - Direct the implementer to fix via `SendMessage`. Implementer verifies integration tests and reports.
   - Direct the reviewer to re-review.
   - Re-report. Loop until user approves.

Implementer and reviewer remain alive throughout this loop.

### 5. Merge

1. Run `~/.claude/infra/merge-branch.sh <original-branch> <branch> "<commit-message>"`.
   The script selects strategy by commit count: squash (1 commit) or --no-ff (2+).
   Compose the commit message per CLAUDE.md commit rules.

### 6. Doc pipeline

1. Dispatch **mental-model-updater** with changed files and implementation summary.
   Provide the commit range from the implementation branch. Always dispatch — the agent determines impact. **Wait for completion before proceeding** — downstream doc updates depend on mental-model accuracy.
2. Refresh `ai-docs/_index.md` — update inventory, descriptions, and layout to reflect current state.
3. If ticket-driven, update ticket status.

### 7. Cleanup

1. Shut down teammates. Delete the team (`TeamDelete`) only if this invocation created it.

## Judgments

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Brief is a small, isolated change (single file, no public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

## Doctrine

Implementation optimizes for **contract-bounded autonomy** —
the implementer has full freedom within skeleton-locked contracts, and
the reviewer validates without lead involvement. When a rule is
ambiguous, apply whichever interpretation better preserves the
implementer's autonomy within contract boundaries.
