---
name: implement
description: Implement a ticket phase with structured task tracking, testing, and documentation. Use when starting work on a ticket or phase.
argument-hint: [ticket-path or description]
---

# Implementation Workflow

Target: $ARGUMENTS

## Plan Mode Protocol

**Already in plan mode** (user invoked `/implement` from plan mode): embed this
line at the top of the plan:

> Execute this plan using `/implement`. Load the skill before starting.

**Context capture:** Plan execution resets context — only the plan text survives.
Collect design decisions, user directives, domain constraints, relevant file
paths, and mental-model insights. Embed them in the plan; it must be
self-contained.

**Not in plan mode**: after completing Step 0 (Understand), decide whether to
enter plan mode via `EnterPlanMode`. **Default to plan mode** — skip it only when
the change is clearly trivial.

- **Enter plan mode** (default): changes touching 2+ files, new features,
  unfamiliar code areas, cross-module changes, design decisions, multiple viable
  approaches, or insufficient confidence.
- **Skip plan mode** (exception): single-file mechanical changes, config tweaks,
  typo fixes, or changes that exactly follow an existing pattern with zero
  ambiguity.

When in doubt, plan. The cost of planning is low; the cost of a wrong
implementation is high.

## Step 0: Understand

1. Read the ticket/description.
2. Read `ai-docs/_index.md` for current project state.
3. **Read `ai-docs/mental-model/overview.md`** to identify all relevant domains.
   Read every mental-model doc that touches the change area, including adjacent
   domains — cross-module coupling is often documented there. If no mental-model
   docs exist yet, note this for Step 4.
4. Run `git log --oneline -10` to check recent work.

**Carry mental-model context forward.** Domain invariants, extension points, and
coupling notes from this step inform the Step 1 task breakdown and Step 2
implementation decisions. When unsure about a contract or boundary during
implementation, re-read the relevant mental-model doc first.

## Step 1: Task List

For non-trivial changes, state assumptions and define success criteria first.

Create tasks via `TaskCreate`. Break work into concrete steps:
- One task per implementation unit
- Testing task(s)
- Documentation update task
- Commit task

Update each task as you progress (`TaskUpdate`: `in_progress` → `completed`).

## Step 2: Implement

For each task:

1. Set task to `in_progress`.
2. Write code following CLAUDE.md Code Standards.
   - Before touching a module, verify its contracts and invariants in the
     mental-model docs. Prefer documented extension points over new abstractions.
   - **Testable pure logic** (calculations, parsing, state transitions):
     define expected behavior first, write test cases, then implement.
   - **Integration/FFI code**: implement first, then add tests for observable
     behavior.
3. When tests fail, diagnose whether the test assumptions or the implementation
   is wrong.
4. Set task to `completed`.

**Approval protocol:**
- **Auto-proceed:** bug fixes, pattern-following additions, tests, refactoring
- **Ask first:** new components/protocols, architectural changes, cross-module interfaces
- **Always ask:** deleting functionality, changing API semantics, schema changes

## Step 3: Verify

Run the project's test suite(s) and build step. If the project has a separate
integration test (see CLAUDE.md `# MEMORY → Build & Workflow`), run that too.
**Read the full output.** Claim "pass" only after confirming the actual result —
never "should pass" or "looks correct." All tests must pass before proceeding.

## Step 3.5: Code Review (optional)

For large or cross-module changes, dispatch a code-review subagent before
committing. Skip for small, single-file changes.

**When to run:** changes touching 3+ files, new public APIs, architectural
changes, or anything in the "Ask first" approval category.

**Dispatch a general-purpose agent with:**

> Review the changes for production readiness.
>
> **What was implemented:** [summary]
> **Requirements:** [ticket phase or description]
> **Git range:** `git diff <base-sha>..HEAD`
>
> Check: correctness, edge cases, error handling, test coverage, adherence
> to CLAUDE.md Code Standards. Categorize issues as Critical / Important / Minor.
> Give a clear verdict: ready to commit, or list fixes needed.

Fix Critical and Important issues before proceeding. Minor issues are optional.

## Step 4: Update Docs

- [ ] Update `ai-docs/_index.md` if project capabilities changed.
- [ ] **Mental model**: launch the **mental-model-updater** agent in background
      with the implementation summary and base commit. Continue with remaining
      doc updates; wait for it before Step 5.
- [ ] **Dependency API drift**: launch the **document-dependency** agent in
      background with the dependency name/version, source paths, and project
      usage grep results. Continue with remaining doc updates. After it
      completes, update `# MEMORY → Documented Dependencies` in CLAUDE.md.
- [ ] Update `# MEMORY` section in `CLAUDE.md`.
- [ ] If completing a ticket phase, append `### Result` to the ticket doc.
- [ ] Prune aggressively — keep docs focused on current state.

## Step 5: Commit

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>
```

Include documentation changes in the commit.
