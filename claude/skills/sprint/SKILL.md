---
name: sprint
description: Discussion-driven iterative implementation. Tight loop of discuss → implement → update, with real-time task tracking. Use when exploring and building simultaneously.
argument-hint: "[ticket-path, topic, or description]"
---

# Sprint

Target: $ARGUMENTS

## Doctrine

You are in **sprint mode** — a hybrid of `/discuss` and `/implement`. The user
will alternate between discussing ideas and requesting small implementations.
Follow the conversation's rhythm: discuss when they discuss, build when they
say build.

**Task list is the live dashboard.** Create, update, and cancel tasks
aggressively as the conversation evolves. The user should be able to glance at
the task list at any time and see the current state of the sprint.

## Step 0: Bootstrap

1. Read `ai-docs/_index.md` for project state.
2. Read `ai-docs/mental-model/overview.md` and any domain docs relevant to the
   topic. Read more mental-model docs as the sprint touches new areas.
3. If `$ARGUMENTS` references a ticket, read it.
4. Run `git log --oneline -10` to check recent work.
5. Create a feature branch: `sprint/<scope>` from the current branch. Record
   the current branch as `<original-branch>`. If already on a `sprint/` branch,
   treat as a resumed session — infer `<original-branch>` from the merge-base
   with `main`, skip branch creation, and continue from the existing task list.
6. Create an initial task:
   ```
   [ ] [fixed] Sprint wrap-up — review, merge, docs
   ```
   This bookend task stays at the bottom throughout the sprint.

## Step 1: Sprint Loop

Repeat until the user signals done:

### On discussion

- Contribute actively — propose approaches, surface risks, suggest alternatives.
- Dispatch subagents for codebase details beyond mental-model docs.
- **When an actionable item emerges from discussion, immediately create a task.**
- When discussion invalidates a prior task, cancel it immediately with a note.

### On implementation request

1. Set the relevant task to `in_progress` (create one if it doesn't exist).
2. Read target files and verify contracts against mental-model docs before
   modifying a module for the first time in this sprint.
3. Implement. Keep changes small and focused.
4. Commit at each logical checkpoint — brief messages. The merge commit carries
   the final summary.
5. Set task to `completed`.
6. Briefly report what was done and any observations.

### Task discipline

- **Create tasks eagerly.** Any concrete action item from discussion becomes a
  task immediately.
- **Update tasks in real-time.** Status changes happen as they happen, not in
  batches.
- **Cancel stale tasks.** When direction changes, cancel outdated tasks with a
  brief reason rather than leaving them dangling.
- **Split tasks that grow.** If an implementation task turns out to be larger
  than expected, split it before continuing.
- Tasks are ordered by creation, not priority. Use task descriptions to convey
  sequencing if needed.

### Approval protocol

- **Auto-proceed:** bug fixes, pattern-following additions, tests, refactoring.
- **Ask first:** new components/protocols, architectural changes, cross-module
  interfaces.
- **Always ask:** deleting functionality, changing API semantics, schema changes.

### Mechanical-edit delegation

When a repetitive edit spans 3+ locations:

| Method | When |
|--------|------|
| **sed / replace_all** | Pure text substitution expressible as regex |
| **haiku subagent** | Fixed pattern, no ambiguity, no judgment needed |
| **sonnet subagent** | Needs structural understanding or has any ambiguity |

Required in the delegation prompt: before/after example, target file list,
success criteria. On failure, subagent runs `git checkout -- <modified-files>`
and reports.

## Step 2: Wrap-up (when user signals done)

Set the wrap-up task to `in_progress`, then:

1. **Run tests & verify** — run the project's test suite and build step (see
   `ai-docs/_index.md` for commands). Read the full
   output. Claim "pass" only after confirming the actual result — never "should
   pass" or "looks correct." Skip if changes are trivial (docs, config) or the
   project has no test suite.
2. **Code review** — for non-trivial changes (3+ files, new public APIs,
   architectural changes, or anything in the "Ask first" category), dispatch a
   general-purpose review subagent with:

   > Review the changes for production readiness.
   >
   > **Scope:** [which files/modules changed]
   > **Requirements:** [sprint topic or ticket reference]
   > **Project context:** Read `CLAUDE.md` code standards. Read **every file**
   > in `ai-docs/mental-model/` regardless of apparent domain relevance —
   > cross-module contracts and invariants often surface in unrelated domains.
   > Do this before reviewing the diff.
   > **Git range:** `git diff $(git merge-base <original-branch> HEAD)..HEAD`
   >
   > Review as a PM + senior engineer:
   > - Correctness — logic errors, edge cases, error handling
   > - Architectural fit — documented contracts and module boundaries
   > - Test quality — no deceptive tests (tautological assertions, mocks
   >   bypassing code under test)
   > - **Duplication** — does this reimplement functionality that already
   >   exists in the codebase? Search for prior art if in doubt.
   > - Code standards — CLAUDE.md conventions
   >
   > Categorize issues as Critical / Important / Minor.
   > Give a clear verdict: ready to merge, or list fixes needed.

   Do not include design justifications in the review prompt. Fix Critical and
   Important issues, re-test, re-review until clean. Skip for small changes.
3. **Update mental model** — dispatch mental-model-updater subagent if changes
   have mental-model impact. Skip for config tweaks, typo fixes. **Wait for
   subagent to finish before step 3a** — review fixes may have changed the
   implementation.
3a. **Update spec** — dispatch spec-updater subagent with the base commit to
   check whether public-facing features were affected. Skip if `ai-docs/spec/`
   does not exist. Wait for completion before step 4.
4. **Update docs** — `ai-docs/_index.md` as needed. If a
   ticket was the input, append a `### Result (<short-hash>) - YY-MM-DD`
   subsection to the completed phase recording what was implemented, deviations,
   and key findings.
5. **Report** — summarize to the user before merge:
   - What was implemented (brief).
   - Process issues encountered (doc gaps, mental-model inaccuracies) — skip if
     nothing notable.
   - **Ticket status** (always include when a ticket was the input): remaining
     phases or confirmation that all phases are complete.
6. **Final commit** — remaining docs and cleanup.
7. **Merge:**
   ```bash
   git checkout <original-branch>
   git merge --no-ff sprint/<scope> -m "$(cat <<'EOF'
   <type>(<scope>): <summary>

   <what changed — brief>

   ## AI Context
   - Sprint session: <topic summary>
   - <key decisions, alternatives rejected>
   EOF
   )"
   git branch -d sprint/<scope>
   ```
   Ask the user for confirmation before merging. If declined, keep branch.
   If no commits were made during the sprint, skip merge and delete the branch.

Set wrap-up task to `completed`.

## Rules

- **Language:** All code, commits, and docs in English regardless of
  conversation language.
- **Stay lightweight.** No ceremony beyond task tracking and commits until
  wrap-up. The point of sprint is velocity.
- **Ticket updates during sprint.** If working off a ticket, update it with
  progress notes as significant milestones are reached — don't wait for wrap-up.
- **Context conservation.** Delegate broad searches and mechanical edits to
  subagents. Keep the main context for synthesis and decision-making.
