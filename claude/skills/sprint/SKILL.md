---
name: sprint
description: >
  If the user starts requesting code changes without loading a workflow
  skill, invoke this. For ad-hoc feature requests, tweaks, and
  exploratory work.
argument-hint: "[ticket-path, topic, or description]"
---

# Sprint

Target: $ARGUMENTS

## Doctrine

Sprint is the **informal implementation mode** — a tight loop of discussion
and small implementations with real-time task tracking. Trade ceremony for
velocity. Load context on-demand, commit freely, track everything in the
task list.

**Task list is the live dashboard.** Create, update, and cancel tasks
aggressively as the conversation evolves. The user should be able to glance
at the task list at any time and see the current state of the sprint.

## Step 0: Bootstrap

1. Run `bash ai-docs/list-active.sh` (falls back to `find ai-docs -type f
   -name '*.md' | sort` if the script is missing) and read `ai-docs/_index.md`.
2. If `$ARGUMENTS` references a ticket, read it.
3. Create a feature branch: `sprint/<scope>` from the current branch. Record
   the current branch as `<original-branch>`. If already on a `sprint/` branch,
   treat as a resumed session — infer `<original-branch>` from the merge-base
   with `main`, skip branch creation, and continue from the existing task list.
4. Create an initial task:
   ```
   [ ] [fixed] Sprint wrap-up — test, review, docs, merge
   ```

## Step 1: Sprint Loop

Repeat until the user signals done:

- **Discussion:** Contribute actively — propose approaches, surface risks,
  suggest alternatives. Dispatch Explore agents for codebase details beyond
  what mental-model docs cover. Read mental-model or spec docs on-demand as
  topics emerge.
- **Implementation:** Set task to `in_progress`, read target files, implement,
  commit at logical checkpoints, set task to `completed`.
- **Task discipline:** Create tasks eagerly for any actionable item. Update
  status in real-time. Cancel stale tasks immediately with a brief reason.
  Split tasks that grow larger than expected.

## Step 2: Wrap-up (when user signals done)

Set the wrap-up task to `in_progress`. Execute in order:

1. **Test & verify** — run test suite and build. Read full output. Skip if
   trivial (docs, config) or no test suite.
2. **Code review** — for non-trivial changes (3+ files, new public APIs,
   architectural changes), dispatch a review subagent. Load `/implement` for
   the review prompt template and conventions.
3. **Update mental model** — dispatch mental-model-updater subagent. Skip for
   config/typo changes. Wait for completion before step 4.
4. **Update spec** — dispatch spec-updater subagent. Skip if `ai-docs/spec/`
   does not exist. Wait for completion before step 5.
5. **Update docs** — `ai-docs/_index.md` as needed. If a ticket was the input,
   load `/write-ticket` for conventions, then append `### Result`.
   If no ticket was the input but the changes relate to an existing ticket,
   ask the user whether to append a `### Result` entry.
6. **Final commit** — docs and remaining changes.
7. **Report** — summarize to the user: what was implemented, process issues
   (if any), ticket status (if applicable).
8. **Merge** — ask user for confirmation, then:
   ```bash
   git checkout <original-branch>
   git merge --no-ff sprint/<scope> -m "<conventional-commit message>"
   git branch -d sprint/<scope>
   ```
   If no commits were made, skip merge and delete the branch.

Set wrap-up task to `completed`.

## Rules

- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **Stay lightweight.** No ceremony beyond task tracking and commits until
  wrap-up. The point of sprint is velocity.
- **Context conservation.** Delegate broad searches and mechanical edits to
  subagents. Keep the main context for synthesis and decision-making.
