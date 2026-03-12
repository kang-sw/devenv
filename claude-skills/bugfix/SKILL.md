---
name: bugfix
description: Diagnose and fix a bug with structured root-cause analysis and user-confirmed resolution. Use when a bug report or unexpected behavior needs investigation.
argument-hint: [bug description, issue link, or error message]
---

# Bugfix Workflow

Target: $ARGUMENTS

## Core Principles

1. **Do NOT touch code until the bug is fully understood.** Ask the user
   clarifying questions until you can state the root cause with confidence.
   Do not guess at fixes — understand first.
2. **Do NOT commit until the user confirms the fix.** After applying the fix,
   present what was changed and wait for the user to verify. Proceed to commit
   only after explicit user approval.

## Plan Mode Protocol

When this skill is invoked in plan mode (or when asked to create a plan before
fixing), the plan **MUST be self-contained**. After plan approval, context is
cleared and the executor only sees the plan text — not this skill file.

**Embed the following block verbatim at the top of every plan you produce:**

~~~markdown
## Execution Protocol (follow strictly)

> Source: @.claude/skills/bugfix/SKILL.md — do NOT skip these steps.

1. **Task tracking**: Before writing any code, create tasks with `TaskCreate` for
   each step (diagnose, fix, verify, docs, commit). Track progress with
   `TaskUpdate` (`in_progress` → `completed`).
2. **Understand first**: Read `ai-docs/_index.md` and relevant
   `ai-docs/mental-model/` domain docs before coding.
3. **Clarify with the user**: Ask questions about the symptom until you can state
   the root cause with confidence. Do NOT proceed to coding while the symptom is
   ambiguous.
4. **Root cause, not symptoms**: Fix the underlying cause, not surface behavior.
5. **Minimal change**: Touch only what is necessary to fix the bug.
6. **Regression test**: Where automatable, write a test that fails before the fix
   and passes after. For GUI/interactive bugs, skip automated test — the user
   will verify manually.
7. **Testing**: Run the project test suite after fix
   (see CLAUDE.md `# MEMORY → Build & Workflow`). All tests must pass.
8. **User confirms before commit**: Present the diff and wait for the user to
   verify the fix. Do NOT commit until the user explicitly approves.
9. **Commit format**: `fix(<scope>): <summary>` + body + `## AI Context` block.
~~~

Do NOT paraphrase or abbreviate this block — copy it as-is into the plan.

## Step 0: Understand (MANDATORY — do this before anything else)

1. Read the bug report/description provided above.
2. Read `ai-docs/_index.md` for current project state.
3. Read relevant `ai-docs/mental-model/` domain document(s) for the affected area.
4. Run `git log --oneline -10` to see recent work — the bug may stem from a
   recent change.

## Step 1: Clarify & Diagnose

Create tasks with `TaskCreate` before starting:

```
TaskCreate: "Understand the symptom (user Q&A)"
TaskCreate: "Trace code path & identify root cause"
TaskCreate: "Implement fix"
TaskCreate: "Write regression test (if automatable)"
TaskCreate: "Run full test suite"
TaskCreate: "User confirms fix"
TaskCreate: "Update docs if needed"
TaskCreate: "Commit with AI Context"
```

**Do NOT skip this step.** Update tasks as you progress (`TaskUpdate` to
`in_progress` when starting, `completed` when done).

### 1a. Understand the Symptom

**Keep asking until you fully understand the symptom.** Example questions:

- "When does this happen — always, or only under specific conditions?"
- "What is the expected behavior vs. what actually happens?"
- "Did this work before? If so, when did it break?"
- "Can you walk me through the steps to trigger this?"
- "Are there related symptoms I should look for?"

Many bugs occur in GUI, visual, or interactive contexts that cannot be
reproduced automatically. In these cases, treat the user's description as the
primary source and focus on **tracing the described behavior through the code**
rather than attempting automated reproduction.

If a test or command *can* reproduce the bug, run it to confirm. Otherwise,
rely on the user's description and proceed to code-level tracing.

### 1b. Trace & Diagnose

- Follow the user's described reproduction path through the code.
- Use subagents (Explore, general-purpose Agent) for broad codebase searches
  to keep the main context focused.
- Identify the **root cause** — not just the line that crashes, but *why* it
  is wrong (logic error, missing edge case, stale assumption, etc.).

Once you believe you have found the root cause, **state it to the user and get
confirmation** before proceeding:

> "I believe the root cause is [X] because [Y]. Does this match your
> understanding? Should I proceed with a fix?"

Do NOT proceed to Step 2 until the user agrees with the diagnosis or redirects
you.

If the bug reveals a design flaw requiring architectural change, recommend
`/discuss` or `/implement` instead of a patch.

## Step 2: Fix

1. **Implement the minimal fix** — change only what is necessary. Do not
   refactor surrounding code, add unrelated improvements, or "clean up while
   here."
2. **Regression test** — if the bug is automatable (pure logic, API, CLI, state
   machine, etc.), write a test that fails before the fix and passes after. For
   GUI/interactive/visual bugs, skip — the user will verify manually in Step 3.

## Step 3: Verify & User Confirmation (MANDATORY — do NOT skip)

### 3a. Automated Verification

Run the project's full test suite(s) — check `# MEMORY → Build & Workflow` in
CLAUDE.md for the correct commands. ALL must pass.

If the fix touches a build-relevant area, run the build too.

### 3b. User Confirmation

**Do NOT commit yet.** Present the following to the user:

1. **Root cause summary** — one-line reminder of what was wrong.
2. **What changed** — list of modified files and a brief explanation of each
   change.
3. **How to verify** — steps the user can take to confirm the fix (run a
   command, test a scenario, check UI behavior, etc.).

Wait for the user to explicitly confirm the bug is resolved.

- If the user reports the fix is incomplete or wrong, return to the appropriate
  earlier step (diagnose or fix).
- If the user approves, proceed to Step 4.

## Step 4: Update Docs (if applicable)

Only update docs when the fix **changes observable behavior or contracts**:

- [ ] `ai-docs/mental-model/` if the fix changes a module contract, coupling,
      or modification pattern
- [ ] `ai-docs/_index.md` if capabilities changed
- [ ] `# MEMORY` section in `CLAUDE.md`
- [ ] If fixing a ticket-tracked bug, append `### Result` to the ticket doc

Skip doc updates for purely internal fixes that don't alter any documented
contract.

## Step 5: Commit

Commit message format:

```
fix(<scope>): <summary>

Root cause: <one-line explanation of why the bug existed>
Fix: <what was changed>

## AI Context
- <diagnosis reasoning, rejected hypotheses, user directives>
```

Include regression test and documentation changes in the commit.
