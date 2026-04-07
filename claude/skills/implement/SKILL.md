---
name: implement
description: >
  When the user provides a ticket, plan, or description for structured
  implementation, invoke this. Heavier than /sprint — use when the
  scope warrants task tracking.
argument-hint: [ticket-path or description]
---

# Implementation Workflow

Target: $ARGUMENTS

## Invariants

- Follow CLAUDE.md Code Standards in all implementation work.
- Before touching a module, verify its contracts and invariants in mental-model docs; prefer documented extension points over new abstractions.
- Claim "pass" only after reading the full test/build output — never "should pass" or "looks correct."
- Do not include design rationale in code-review prompts — the reviewer evaluates independently to avoid confirmation bias.
- Commit freely on the feature branch; the merge commit carries the final summary.
- User approves the report before doc-update tasks proceed.
- Dismiss false-positive review issues with a brief rationale — do not apply unnecessary fixes.

## On: invoke

### 1. Understand

1. Read the ticket/description/plan text.
2. Read `ai-docs/mental-model/overview.md`; read every mental-model doc touching the change area and adjacent domains. If none exist, note this for the docs task.
3. Record current branch as `<original-branch>`. If already on an `implement/` branch, treat as resumed session — infer `<original-branch>` from merge-base with `main`, skip branch creation, continue from existing task list. Otherwise create `implement/<scope>` from current branch.

Carry mental-model context forward into task breakdown and implementation. When unsure about a contract or boundary, re-read the relevant doc.

### 2. Create task list

Create tasks via `TaskCreate` using the **task list template** below. Bookend tasks (marked `[fixed]`) are mandatory. Fill implementation tasks between them. State assumptions and success criteria before the first implementation task.

Update each task as you progress (`TaskUpdate`: `in_progress` -> `completed`).

### 3. Execute tasks

Work through tasks sequentially: set `in_progress`, do work, set `completed`.

**Testable pure logic** (calculations, parsing, state transitions): define expected behavior first, write tests, then implement.
**Integration/FFI code**: implement first, then add tests for observable behavior.
When tests fail, diagnose whether test assumptions or implementation is wrong.

### 4. Mechanical-edit delegation

When a repetitive edit spans 3+ locations, delegate to conserve context window.

| Method | When |
|---|---|
| **sed / replace_all** | Pure text substitution expressible as regex |
| **haiku subagent** | Fixed pattern, no ambiguity, no judgment needed |
| **sonnet subagent** | Needs structural understanding or has any ambiguity |

Required in the delegation prompt: (1) before/after example from first instance, (2) target file list, (3) success criteria (e.g. `cargo check`), (4) bail-out condition (sonnet only): skip file and report if structure differs from example.

Execution: `Agent` with `model: "haiku"` or `"sonnet"`, no worktree (preserve build cache). Use haiku only when the pattern is rigid enough that bail-out judgment is unnecessary.

Rollback: on criteria failure, subagent runs `git checkout -- <modified-files>` and reports. Main agent reviews subagent diff before proceeding.

### 5. Verify

Run the project's test suite(s) and build step (see `ai-docs/_index.md`). Run integration tests if they exist. Skip if project has no test suite or build step. Read the full output.

### 6. Code review

Skip for small single-file changes.

Trigger when: 3+ files changed, new public APIs, architectural changes, or anything in the "ask first" approval category.

Dispatch a subagent with the **code-review prompt template** below. Fix Critical and Important issues, then re-run verify, then re-dispatch review. Loop until no Critical/Important issues remain. Minor issues are optional.

### 7. Report process issues

Report to user:
- Dependency doc gaps — missing/wrong/misleading `ai-docs/deps/` docs
- Mental-model inaccuracies — contracts that didn't match reality
- Convention mismatches — docs that diverged from actual code
- Ticket status — remaining phases or confirmation all complete; help user decide `done/` vs `wip/`

Skip process issues if nothing notable; always include ticket status when a ticket was the input. Do not silently swallow friction. Wait for user approval before proceeding.

### 8. Update docs

1. Dispatch **mental-model-updater subagent** with changed files and summary. Skip if no mental-model impact. Wait for completion.
2. Dispatch **spec-updater subagent** with base commit. Skip if `ai-docs/spec/` does not exist. Wait for completion.
3. Update `ai-docs/_index.md` if project capabilities changed.
4. If completing a ticket phase, move ticket status via `git mv` if appropriate (load `/write-ticket` for conventions).
5. Prune aggressively — keep docs focused on current state.

### 9. Final commit & merge

Commit remaining docs and cleanup. Then merge:

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

If the user declines, keep the branch intact and stop.

## Judgments

### judge: approval-gate

- **Auto-proceed:** bug fixes, pattern-following additions, tests, refactoring.
- **Ask first:** new components/protocols, architectural changes, cross-module interfaces.
- **Always ask:** deleting functionality, changing API semantics, schema changes.

## Templates

### Task list template

```
[ ] [fixed] Collect context — read target files, verify contracts against code
  ... (implementation tasks — commit freely at logical points) ...
[ ] [fixed] Run tests & verify — full test suite, read actual output
[ ] [fixed] Code review — dispatch subagent (skip for small single-file changes)
  -> if Critical/Important issues: fix -> re-run verify -> re-review (loop until clean)
[ ] [fixed] Report process issues to user — user approves before doc updates
[ ] [fixed] Update mental model with mental-model-updater subagent
[ ] [fixed] Update spec with spec-updater subagent
[ ] [fixed] Update project docs — ai-docs/_index.md, ticket status move
[ ] [fixed] Final commit — docs & remaining changes
[ ] [fixed] Merge & cleanup — merge --no-ff -> delete branch
```

### Code-review prompt template

> Review the changes for production readiness.
>
> **Scope:** [which files/modules changed — no design rationale]
> **Requirements:** [ticket phase or description]
> **Project context:** Read `CLAUDE.md` code standards. Read **every file**
> in `ai-docs/mental-model/` regardless of apparent domain relevance —
> cross-module contracts and invariants often surface in unrelated domains.
> Do this before reviewing the diff.
> **Git range:** `git diff $(git merge-base <original-branch> HEAD)..HEAD`
>
> Review as a PM + senior engineer with full project awareness:
> - **Correctness** — logic errors, edge cases, error handling
> - **Architectural fit** — respects documented contracts and module boundaries? Unintended coupling?
> - **Test quality** — adequate coverage, no deceptive tests (tautological assertions, unreachable asserts, mocks bypassing code under test, expected values derived from tested logic)
> - **Duplication** — reimplements existing functionality? Search for prior art.
> - **Code standards** — CLAUDE.md conventions
>
> Categorize: Critical / Important / Minor.
> Verdict: ready to merge, or list fixes needed.

### Merge commit template

```
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem> phase <N>
  > Forward: <what future phases must know>
```

Include `## Ticket Updates` when the execution is ticket-driven AND forward-facing findings were discovered. Omit when there are no forwards — deviations and implementation details already live in `## AI Context`.

## Doctrine

Implementation correctness depends on **verified task closure** — every
task runs through build, test, and review before the branch merges. When
a rule is ambiguous, apply whichever interpretation better preserves
verified closure of each task in the sequence.
