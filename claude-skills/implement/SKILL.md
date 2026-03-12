---
name: implement
description: Implement a ticket phase with structured task tracking, testing, and documentation. Use when starting work on a ticket or phase.
argument-hint: [ticket-path or description]
---

# Implementation Workflow

Target: $ARGUMENTS

## Plan Mode Protocol

When invoked in plan mode, the plan must be self-contained. Embed this block
**verbatim** at the top of the plan:

---

### Execution Protocol (follow strictly)

> Source: @.claude/skills/implement/SKILL.md

1. **Task tracking**: Create tasks with `TaskCreate` for each implementation
   unit, testing, doc updates, and commit. Track with `TaskUpdate`.
2. **Understand first**: Read `ai-docs/_index.md` and relevant
   `ai-docs/mental-model/` domain docs, relevant tickets before coding.
3. **Code standards**: Follow CLAUDE.md Code Standards (simplicity, surgical changes).
4. **Testing**: Run the project test suite after implementation
   (see CLAUDE.md `# MEMORY → Build & Workflow`). All tests must pass.
5. **Doc updates**:
   - `ai-docs/_index.md` if capabilities changed
   - Mental-model updates: delegate to the **mental-model-updater** agent in background
   - Dependency API drift: delegate to the **document-dependency** agent in background
     with usage context. Update `# MEMORY → Documented Dependencies` after completion.
   - `# MEMORY` section in `CLAUDE.md`
   - Append `### Result` to ticket doc if completing a phase
6. **Commit format**: `<type>(<scope>): <summary>` + body + `## AI Context` block.
   Include doc changes in the commit.

---

## Step 0: Understand

1. Read the ticket/description.
2. Read `ai-docs/_index.md` for current project state.
3. Read relevant `ai-docs/mental-model/` domain docs. Check `overview.md`
   if unsure which domain applies.
4. Run `git log --oneline -10` to see recent work.

## Step 1: Task List

For non-trivial changes, state assumptions and define success criteria first.

Create tasks with `TaskCreate`. Break work into concrete steps:
- Each implementation unit as a separate task
- Testing task(s)
- Documentation update task
- Commit task

Update tasks as you progress (`TaskUpdate` to `in_progress` → `completed`).

## Step 2: Implement

For each task:

1. Set task to `in_progress`
2. Write code — follow CLAUDE.md Code Standards
3. Write tests alongside non-trivial pure logic. When tests fail, diagnose
   whether the test assumptions or the implementation is wrong.
4. Set task to `completed`

**Approval protocol:**
- Auto-proceed: bug fixes, pattern-following additions, tests, refactoring
- Ask first: new components/protocols, architectural changes, cross-module interfaces
- Always ask: deleting functionality, changing API semantics, schema changes

## Step 3: Verify

Run the project's test suite(s). All must pass before proceeding.
If the project has a relevant build step, run it too.

## Step 4: Update Docs

- [ ] `ai-docs/_index.md` if project capabilities changed
- [ ] **Mental model update**: Launch the **mental-model-updater** agent in
      background with the implementation summary and base commit. Continue with
      remaining doc updates; wait for it before Step 5.
- [ ] **Dependency API drift**: Launch the **document-dependency** agent in
      background with the dependency name/version, source paths, and project
      usage grep results. Continue with remaining doc updates. After it
      completes, update `# MEMORY → Documented Dependencies` in CLAUDE.md.
- [ ] Update `# MEMORY` section in `CLAUDE.md`
- [ ] If completing a ticket phase, append `### Result` to the ticket doc
- [ ] Prune aggressively — keep docs focused on current state

## Step 5: Commit

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>
```

Include documentation changes in the commit.
