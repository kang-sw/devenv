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

When invoked in plan mode, embed this line at the top of the plan:

> Execute this plan using `/bugfix`. Load the skill before starting.

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

1. **Read error messages carefully.** Full stack traces, line numbers, error codes.
   Don't skip past warnings.
2. **Reproduce consistently.** Can you trigger it reliably? What are the exact
   conditions? If not reproducible, gather more data — don't guess.
3. **Check recent changes.** `git log`, `git diff`, new dependencies, config changes.
4. **Multi-component systems:** Log at each component boundary before proposing
   fixes. Run once to identify *which layer* fails.
5. **Trace data flow backward.** Where does the bad value originate? Keep tracing
   up the call chain until you find the source. Fix at source, not at symptom.

Use subagents for broad codebase searches when needed.

State your diagnosis to the user and get confirmation before proceeding:

> "I believe the root cause is [X] because [Y]. Should I proceed with a fix?"

If the bug reveals a design flaw, recommend `/design` or `/implement` instead.

## Step 2: Fix

1. Implement the minimal fix — no unrelated refactoring.
2. If automatable, write a regression test that fails before and passes after.
   Skip for GUI/interactive bugs.

**Escalation rule:** If 3+ fix attempts fail, STOP. The pattern itself may be
wrong. Suggest `/design` or raise the architectural question directly with the
user before attempting another fix.

## Step 3: Verify & Confirm

### 3a. Automated

Run the full test suite. **Read the actual output.** Claim "pass" only after
seeing the result — never "should pass" or "probably works."

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
