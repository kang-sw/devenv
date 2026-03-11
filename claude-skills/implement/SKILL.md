---
name: implement
description: Implement a ticket phase with structured task tracking, testing, and documentation. Use when starting work on a ticket or phase.
argument-hint: [ticket-path or description]
---

# Implementation Workflow

Steps to implement: $ARGUMENTS

## Step 0: Understand (MANDATORY — do this before anything else)

1. Read the ticket/description provided above.
2. Read `ai-docs/_index.md` for current project state.
3. Read relevant `ai-docs/mental-model/` domain document(s) — these contain modification
   patterns, module contracts, coupling maps, and extension points. Check `overview.md`
   if unsure which domain applies.
4. Run `git log --oneline -10` to see recent work.

## Step 1: Task List (MANDATORY — do this before writing ANY code)

For non-trivial changes, state assumptions and define success criteria before starting.

Create a task list using `TaskCreate`. Break the work into concrete steps:

- Each implementation unit as a separate task
- Testing task(s)
- Documentation update task
- Commit task

Example:

```
TaskCreate: "Implement read_task_results tool handler"
TaskCreate: "Implement edit_task_graph tool handler"
TaskCreate: "Add tests for new tools"
TaskCreate: "Run full test suite"
TaskCreate: "Update _index.md + ticket Result + affected mental models"
TaskCreate: "Commit with AI Context"
```

**Do NOT skip this step.** Update tasks as you progress (`TaskUpdate` to
`in_progress` when starting, `completed` when done).

## Step 2: Implement

For each task:

1. Set task to `in_progress`
2. Write code — follow CLAUDE.md Code Standards (simplicity, surgical changes)
3. Write tests alongside non-trivial pure logic (math, protocol, state machines).
   When tests fail, first diagnose whether the **test assumptions** or the
   **implementation logic** is wrong — don't blindly fix the implementation to match
   a bad test.
   For user-interactive features (UI, visual output), request manual testing instead.
4. Set task to `completed`

**Approval protocol:**

- Auto-proceed: bug fixes, pattern-following additions, tests, refactoring
- Ask first: new components/protocols, architectural changes, cross-module interfaces
- Always ask: deleting functionality, changing API semantics, schema changes

## Step 3: Verify

Run the project's test suite(s) — check `# MEMORY → Build & Workflow` in
CLAUDE.md for the correct commands. ALL must pass before proceeding.

If the project has a build step relevant to your changes, run it too.

## Step 4: Update Docs (MANDATORY — do not skip)

- [ ] Update `ai-docs/_index.md` if project capabilities changed
- [ ] If this change altered modification patterns, module contracts, coupling, or
      extension points in a domain, update the relevant `ai-docs/mental-model/` document
- [ ] If you discovered dependency API drift during implementation, document it in
      `ai-docs/deps/<name>[v<version>/<model>].md` and update `# MEMORY → Documented
      Dependencies`
- [ ] Update `# MEMORY` section in `CLAUDE.md` (what was done, what's next)
- [ ] If completing a ticket phase, append `### Result` to the ticket doc
- [ ] Prune aggressively — keep docs focused on current state

## Step 5: Commit

Commit message format:

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>
```

Include documentation changes in the commit.
