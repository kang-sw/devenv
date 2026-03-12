---
name: bugfix
description: Diagnose and fix a bug with structured root-cause analysis and user-confirmed resolution. Use when a bug report or unexpected behavior needs investigation.
argument-hint: [bug description, issue link, or error message]
---

# Bugfix Workflow

Target: $ARGUMENTS

## Constraints

1. **Understand before touching code.** Ask clarifying questions until you can
   state the root cause with confidence.
2. **User confirms before commit.** Present the fix and wait for explicit approval.

## Plan Mode Protocol

When invoked in plan mode, the plan must be self-contained. Embed this block
**verbatim** at the top of the plan:

---

### Execution Protocol (follow strictly)

> Source: @.claude/skills/bugfix/SKILL.md

1. **Task tracking**: Create tasks with `TaskCreate` for each step (diagnose,
   fix, verify, docs, commit). Track with `TaskUpdate`.
2. **Understand first**: Read `ai-docs/_index.md` and relevant
   `ai-docs/mental-model/` domain docs before coding.
3. **Clarify**: Ask questions until the root cause is clear.
4. **Root cause, not symptoms**: Fix the underlying cause.
5. **Minimal change**: Touch only what is necessary.
6. **Regression test**: Where automatable, write a test that fails before and
   passes after the fix. Skip for GUI/interactive bugs.
7. **Testing**: Run the project test suite (see CLAUDE.md `# MEMORY → Build &
   Workflow`). All tests must pass.
8. **User confirms before commit**: Present the diff and wait for approval.
9. **Commit format**: `fix(<scope>): <summary>` + body + `## AI Context` block.

---

## Step 0: Understand

1. Read the bug report/description.
2. Read `ai-docs/_index.md` and relevant `ai-docs/mental-model/` domain docs.
3. Run `git log --oneline -10` — the bug may stem from a recent change.

## Step 1: Clarify & Diagnose

Create tasks with `TaskCreate` before starting.

### 1a. Understand the Symptom

Keep asking until you fully understand the symptom: when it happens, expected vs
actual behavior, whether it worked before, reproduction steps.

For GUI/visual/interactive bugs that can't be reproduced automatically, treat the
user's description as primary source and trace the described behavior through code.

### 1b. Trace & Diagnose

- Follow the reproduction path through the code.
- Use subagents for broad codebase searches.
- Identify the root cause — not just the crashing line, but *why* it is wrong.

State your diagnosis to the user and get confirmation before proceeding:

> "I believe the root cause is [X] because [Y]. Should I proceed with a fix?"

If the bug reveals a design flaw, recommend `/discuss` or `/implement` instead.

## Step 2: Fix

1. Implement the minimal fix — no unrelated refactoring.
2. If automatable, write a regression test that fails before and passes after.
   Skip for GUI/interactive bugs.

## Step 3: Verify & Confirm

### 3a. Automated

Run the full test suite. All must pass.

### 3b. User Confirmation

Present to the user:
1. Root cause summary (one line).
2. What changed (files and brief explanation).
3. How to verify (steps to confirm the fix).

Wait for explicit approval before proceeding.

## Step 4: Update Docs (if applicable)

Only when the fix changes observable behavior or contracts:
- `ai-docs/mental-model/` if contracts or patterns changed
- `ai-docs/_index.md` if capabilities changed
- `# MEMORY` section in `CLAUDE.md`
- Append `### Result` to ticket doc if applicable

## Step 5: Commit

```
fix(<scope>): <summary>

Root cause: <one-line explanation>
Fix: <what was changed>

## AI Context
- <diagnosis reasoning, rejected hypotheses, user directives>
```
