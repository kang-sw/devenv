---
name: delegate-implement
description: >
  Delegated single-scope implementation cycle. An implementer-reviewer pair
  work behind locked contracts; the lead coordinates, merges, and updates
  docs.
argument-hint: "<plan-path or inline brief> [--ticket <ticket-stem>]"
---

# Delegated Implementation

Target: $ARGUMENTS

## Invariants

- This skill delegates — the lead does not read source code or write implementation.
- When skeleton exists, its stubs and integration tests are the acceptance criteria.
- Reviewers report to the lead only — never directly to the implementer. The lead consolidates findings and sends a single list to the implementer.
- User approves the report before merge — no code reaches the target branch without user confirmation.
- Teammates (implementer, reviewer) stay alive until after doc pipeline completes; cleanup is the final step.
- One delegation cycle per invocation.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).
- Task list is created at prepare and tracked to completion — no task may be skipped or reordered.
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
   [ ] Spawn reviewers (partition-allocated) — parallel review → lead consolidates → implementer fixes → re-review loop until clean
   [ ] Dispatch mental-model-updater — wait for completion
   [ ] Report to user — wait for approval
     > if tweaks requested: implementer fixes → re-verify → reviewer re-reviews → re-run updater (loop)
   [ ] Merge to original branch
   [ ] Update project docs — refresh ai-docs/_index.md, ticket status
   [ ] Cleanup — shut down teammates, delete team
   ```

### 2. Spawn implementer

```
Agent(
  name = "implementer",
  description = "Implement plan on branch",
  subagent_type = "general-purpose",
  model = "sonnet",
  team_name = "impl-<scope>",
  prompt = """
    Run `load-infra implementer.md` first.

    Lead name: <lead-name>
    Mode: <A: plan-driven | B: inline brief>
    <Plan path | Brief text>

    Acceptance criteria: skeleton integration tests must pass.
    - Test files: <integration test paths>
    - Run: <command to execute them>

    Team rules:
    - Verify integration tests pass before reporting completion or after each fix.
    - Report completion to the lead via SendMessage. Include test results.
    - The lead will send you consolidated review findings — fix, re-verify tests, and report back to the lead.
    - Commit at logical checkpoints on the current branch.
  """
)
```

Wait for the implementer's completion report. Note the commit range.

### 3. Review

#### 3a. Partition allocation

Apply `judge: partition-allocation` to determine which review partitions apply
based on the implementer's report and the nature of the changes.

#### 3b. Spawn reviewers

Spawn one reviewer per selected partition in parallel. Each reviewer
loads its partition doc via `load-infra`:

| Partition | Infra doc |
|-----------|-----------|
| Correctness | `code-review-correctness.md` |
| Fit | `code-review-fit.md` |
| Test | `code-review-test.md` |

```
Agent(
  name = "reviewer-<partition>",
  description = "Review implementation — <Partition> partition",
  subagent_type = "code-reviewer",
  model = "sonnet",
  team_name = "impl-<scope>",
  prompt = """
    Run `load-infra code-review-<partition>.md` first.

    Lead name: <lead-name>
    Diff range: <first-commit>..<last-commit>

    Team rules:
    - SendMessage your findings report to the lead only.
    - Do not contact the implementer directly.
  """
)
```

Wait for all reviewers to complete.

#### 3c. Consolidate and relay

Read all reviewer reports. Produce a single consolidated findings list —
deduplicate overlapping issues, assign final severity. SendMessage the
consolidated list to the implementer.

Wait for the implementer's fix report and integration test confirmation.

#### 3d. Re-review loop

If Critical or Important issues remain unresolved: re-apply
`judge: partition-allocation` (same partitions) and spawn fresh reviewers.
Repeat 3b–3d until no Critical or Important issues remain.

### 4. Docs pre-pass

1. Dispatch **mental-model-updater** with changed files and implementation summary.
   Provide the commit range from the implementation branch. Always dispatch — the agent determines impact. **Wait for completion before proceeding** — the report step should reflect the final doc state.

### 5. Report and approval

1. Report to the user:
   - What was implemented (from implementer report)
   - Review result (from reviewer report)
   - Test status
   - Any deviations or open items
2. Wait for user approval. If the user requests tweaks:
   - Direct the implementer to fix via `SendMessage`. Implementer verifies integration tests and reports.
   - Re-apply `judge: partition-allocation` and spawn fresh reviewers per the step 3 pattern. Wait for consolidated findings and implementer fix report.
   - Re-run **mental-model-updater** with the new commit range. Wait for completion.
   - Re-report. Loop until user approves.

Implementer and reviewer remain alive throughout this loop.

### 6. Merge

1. Run `merge-branch <original-branch> <branch> "<commit-message>"`.
   The script selects strategy by commit count: squash (1 commit) or --no-ff (2+).
   Compose the commit message per CLAUDE.md commit rules.

### 7. Doc pipeline

1. Refresh `ai-docs/_index.md` — update inventory, descriptions, and layout to reflect current state.
2. If ticket-driven:
   1. Append `### Result (<short-hash>) - YYYY-MM-DD` to each completed phase. Content: what was implemented, deviations from plan, key findings for future phases. Short hash = merge commit.
   2. Move ticket to the next status directory (`git mv`) if all phases are complete.

### 8. Cleanup

1. Shut down teammates. Delete the team (`TeamDelete`) only if this invocation created it.

## Judgments

### judge: partition-allocation

Evaluate based on the implementer's report and the nature of the changes.

| Partition | Assign when |
|-----------|-------------|
| **Correctness** | New logic introduced, error paths modified, contracts or security surface touched |
| **Fit** | Existing components reused or modified, new patterns others will follow |
| **Test** | Test files added or modified, or new code paths added without existing coverage |
| **Default** | New feature or non-trivial cross-module change → all three partitions |
| **Floor** | Purely mechanical change (format, rename with no semantic change) → Correctness only |

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
