---
name: implement
description: Execute an implementation with structured task tracking, testing, and documentation. Use when starting work on a ticket, plan, or description.
argument-hint: [ticket-path or description]
---

# Implementation Workflow

Target: $ARGUMENTS

## Step 0: Understand

1. Read the ticket/description/plan text.
2. Read `ai-docs/_index.md` for current project state.
3. Read `ai-docs/mental-model/overview.md` to identify relevant domains.
   Read every mental-model doc that touches the change area, including adjacent
   domains — cross-module coupling is often documented there. If no mental-model
   docs exist yet, note this for the docs task.
4. Run `git log --oneline -10` to check recent work.
5. Record the current branch as `<original-branch>`. If already on an
   `implement/` branch, treat it as a resumed session — infer
   `<original-branch>` from the merge-base with `main`, skip branch creation,
   and continue from the existing task list. Otherwise, create a feature branch:
   `implement/<scope>` from the current branch.

Carry mental-model context forward into task breakdown and implementation. When
unsure about a contract or boundary, re-read the relevant mental-model doc.

## Step 1: Task List

Create tasks via `TaskCreate` using the template below. The **bookend tasks**
(marked `[fixed]`) are mandatory and must not be skipped. Fill implementation
tasks between them.

```
[ ] [fixed] Collect context — read target files, verify contracts against code
  ... (implementation tasks — commit freely at logical points) ...
[ ] [fixed] Run tests & verify — full test suite, read actual output
[ ] [fixed] Code review — dispatch subagent (skip for small single-file changes)
[ ] [fixed] Update mental model with mental-model-updater subagent
[ ] [fixed] Update project docs — CLAUDE.md # MEMORY, ai-docs/_index.md, ticket result
[ ] [fixed] Final commit — docs & remaining changes
[ ] [fixed] Merge & cleanup — user confirm → merge --no-ff → delete branch
[ ] [fixed] Report process issues to user
```

State assumptions and success criteria before the first implementation task.

Update each task as you progress (`TaskUpdate`: `in_progress` → `completed`).

## Step 2: Execute Tasks

Work through tasks sequentially. For each:

1. Set task to `in_progress`.
2. Do the work.
3. Set task to `completed`.

### Implementation guidance

- Follow CLAUDE.md Code Standards.
- Before touching a module, verify its contracts and invariants in the
  mental-model docs. Prefer documented extension points over new abstractions.
- **Testable pure logic** (calculations, parsing, state transitions):
  define expected behavior first, write test cases, then implement.
- **Integration/FFI code**: implement first, then add tests for observable
  behavior.
- When tests fail, diagnose whether the test assumptions or the implementation
  is wrong.

### Mechanical-edit delegation

When a repetitive, mechanical edit spans 3+ locations, delegate to a sonnet
subagent to conserve the main agent's context window.

**Required in the delegation prompt:**
1. Before/after example (extracted from the first instance)
2. Target file list
3. Success criteria appropriate to the stage (e.g., `cargo check` passes,
   grep confirms pattern applied)
4. Bail-out condition: "If a target file's structure differs from the example
   or the expected pattern is not found — skip that file and report it back"

**Execution:** `Agent` with `model: "sonnet"` (no worktree — preserve build cache)

**Rollback:** On criteria failure or unexpected structure, the subagent runs
`git checkout -- <modified-files>` to revert, then reports the failure reason.

**Main agent responsibility:** Review the subagent's diff before proceeding.
Intermediate commits are safe — work is on a feature branch.

### Approval protocol

- **Auto-proceed:** bug fixes, pattern-following additions, tests, refactoring
- **Ask first:** new components/protocols, architectural changes, cross-module interfaces
- **Always ask:** deleting functionality, changing API semantics, schema changes

### Verify task

Run the project's test suite(s) and build step (see CLAUDE.md
`# MEMORY → Build & Workflow`). If the project has a separate integration test,
run that too. Skip if the project has no test suite or build step.
**Read the full output.** Claim "pass" only after confirming the actual result —
never "should pass" or "looks correct." All tests must pass before proceeding.

### Code review task

Skip for small, single-file changes.

**When to run:** changes touching 3+ files, new public APIs, architectural
changes, or anything in the "Ask first" approval category.

**Dispatch a general-purpose subagent with:**

> Review the changes for production readiness.
>
> **What was implemented:** [summary]
> **Requirements:** [ticket phase or description]
> **Git range:** `git diff $(git merge-base <original-branch> HEAD)..HEAD`
>
> Check: correctness, edge cases, error handling, test coverage, adherence
> to CLAUDE.md Code Standards. Categorize issues as Critical / Important / Minor.
> Give a clear verdict: ready to merge, or list fixes needed.

Fix Critical and Important issues before proceeding. Minor issues are optional.

### Mental-model-updater task

Dispatch a **subagent** to update mental-model docs based on the
changes made and wait for it to finish. Prompt it with: the list of files
changed, a summary of what was added/modified, and the path to
`ai-docs/mental-model/overview.md`. The subagent reads existing mental-model
docs and updates them to reflect the new state. Skip if the change has no
mental-model impact (e.g., config tweaks, typo fixes).

### Docs task

- Update `ai-docs/_index.md` if project capabilities changed.
- Continue with doc updates while the mental-model-updater subagent runs.
- Update `# MEMORY` section in `CLAUDE.md`.
- If completing a ticket phase, append `### Result` to the ticket doc.
- Prune aggressively — keep docs focused on current state.

### Intermediate commits

Work happens on a feature branch, so commit freely at logical checkpoints.
Keep messages brief — the merge commit carries the final summary.

### Final commit task

Wait for mental-model-updater to finish before the last commit.
Commit remaining docs and cleanup changes.

### Merge & cleanup task

After all tasks pass, ask the user for final confirmation.

```bash
git checkout <original-branch>
git merge --no-ff implement/<scope> -m "$(cat <<'EOF'
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>
EOF
)"
git branch -d implement/<scope>
```

The merge commit message serves as the conventional-commit record.
If the user declines, keep the branch intact and stop.

### Report task

After merging, report to the user any **process issues** encountered during
implementation:

- Dependency doc gaps — APIs that were missing, wrong, or misleading in
  `ai-docs/deps/` docs
- Mental-model inaccuracies — contracts or invariants that didn't match reality
- Convention mismatches — patterns described in docs that diverged from actual
  code

Skip if nothing notable. This is the last task — do not silently swallow
friction that could be fixed for next time.
