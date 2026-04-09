---
name: delegated-implement
description: >
  Delegate a single implementation cycle to an implementer + reviewer pair.
  Use for Tier 3 work (internal implementation behind locked contracts).
  Dispatch multiple instances in parallel for concurrent work.
argument-hint: "<plan-path or inline brief> [--ticket <ticket-stem>]"
---

# Delegated Implementation

Target: $ARGUMENTS

## Invariants

- This skill delegates — the lead does not read source code or write implementation.
- Skeleton stubs and integration tests are the acceptance criteria.
- The implementer and reviewer communicate directly; the lead receives only final reports.
- One delegation cycle per invocation. For parallel work, dispatch multiple instances.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).
- Task list with `[fixed]` tasks is created at prepare and tracked to completion — no task may be skipped.
- `/team-lead` skill must be loaded before any team operations.

## On: invoke

### 0. Prerequisites

1. Load `/team-lead` skill if not already loaded.

### 1. Prepare

1. Parse arguments: extract plan path or inline brief, and optional ticket stem.
2. If plan-driven: verify the plan file exists. Read it to extract scope and branch name hint.
3. If brief-driven: the brief is the full specification.
4. Verify skeleton exists: grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs or check for integration tests that reference the target contracts. If absent, stop and suggest `/write-skeleton`.
5. Record current branch as `<original-branch>`. Create `implement/<scope>` branch.
6. If already in a team context, use the existing team. Otherwise create one:
   ```
   TeamCreate(team_name = "impl-<scope>", description = "<brief scope>")
   ```
7. Create task list — all tasks are `[fixed]`:
   ```
   [ ] [fixed] Spawn implementer — wait for completion report
   [ ] [fixed] Spawn reviewer — wait for clean report
   [ ] [fixed] Verify integration tests pass
   [ ] [fixed] Merge and report to user
   [ ] [fixed] Dispatch mental-model-updater — wait for completion
   [ ] [fixed] Update project docs — ai-docs/_index.md, ticket status
   ```
   Mark each task as completed when done. Do not skip any.

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

    Team rules:
    - Report completion to the lead via SendMessage.
    - The reviewer may message you directly with findings — fix and reply.
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

The reviewer and implementer iterate directly until the reviewer
reports clean. Wait for the reviewer's final report to the lead.

### 4. Merge and report

1. Verify all integration tests pass on the implementation branch.
2. Shut down teammates. Delete the team (`TeamDelete`) only if this invocation created it.
3. Merge back to `<original-branch>` with a summary commit per CLAUDE.md commit rules.
3. Report to the user:
   - What was implemented (from implementer report)
   - Review result (from reviewer report)
   - Test status
   - Any deviations or open items

### 5. Doc pipeline

1. Dispatch **mental-model-updater** with changed files and implementation summary.
   Provide the commit range from the implementation branch. Always dispatch — the agent determines impact. **Wait for completion before proceeding** — downstream doc updates depend on mental-model accuracy.
2. Update `ai-docs/_index.md` if project capabilities changed.
3. If ticket-driven, update ticket status.

## Judgments

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Brief is a small, isolated change (single file, no public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

## Doctrine

Delegated implementation optimizes for **contract-bounded autonomy** —
the implementer has full freedom within skeleton-locked contracts, and
the reviewer validates without lead involvement. When a rule is
ambiguous, apply whichever interpretation better preserves the
implementer's autonomy within contract boundaries.
