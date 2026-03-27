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

1. Read `ai-docs/_index.md` and `ai-docs/_memory.md` for project state.
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

- **Auto-proceed:** bug fixes, small additions following patterns, tests,
  refactoring within a single module.
- **Ask first:** new components/protocols, architectural changes, cross-module
  interfaces, anything that would be hard to undo.
- **Always ask:** deleting functionality, changing API semantics, schema changes.

### Mechanical-edit delegation

When a repetitive edit spans 3+ locations, delegate per `/implement`
conventions (sed/replace_all for regex, haiku for rigid patterns, sonnet for
structural edits). Include before/after example, target file list, and success
criteria. Rollback on failure.

## Step 2: Wrap-up (when user signals done)

Set the wrap-up task to `in_progress`, then:

1. **Run tests & verify** — run the project's test suite and build step (see
   `ai-docs/_memory.md` or `ai-docs/_index.md` for commands). Read the full
   output. Claim "pass" only after confirming the actual result — never "should
   pass" or "looks correct." Skip if changes are trivial (docs, config) or the
   project has no test suite.
2. **Code review** — for non-trivial changes (3+ files, new public APIs,
   architectural changes, or anything in the "Ask first" category), dispatch a
   review subagent. Use the same review prompt template as `/implement` (scope,
   requirements, project context via CLAUDE.md + mental-model docs, git range
   from merge-base). Fix Critical/Important issues, re-test, re-review until
   clean. Skip for small changes.
3. **Update mental model** — dispatch mental-model-updater subagent if changes
   have mental-model impact. Skip for config tweaks, typo fixes.
4. **Update docs** — `ai-docs/_memory.md`, `ai-docs/_index.md` as needed. If a
   ticket was the input, load `/write-ticket` for conventions, then append
   `### Result` to the ticket doc.
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
