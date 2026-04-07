# Implementation Process

Lifecycle management for whoever owns the implementation branch.
Subagents doing focused implementation work should NOT read this file —
they follow `impl-playbook.md` only.

## Task List

Use `TaskCreate` to track work. Fixed bookend tasks are mandatory and
never skipped or reordered:

```
[ ] [fixed] Collect context — read target files, verify contracts
  ... (implementation tasks — commit freely at logical points) ...
[ ] [fixed] Verify — full test suite, read actual output
[ ] [fixed] Code review — dispatch subagent (skip for small single-file changes)
  > if Critical/Important: fix → re-verify → re-review (loop until clean)
[ ] [fixed] Report to user — user approves before doc updates
[ ] [fixed] Update mental model — dispatch mental-model-updater subagent
[ ] [fixed] Update spec — dispatch spec-updater subagent
[ ] [fixed] Update project docs — ai-docs/_index.md, ticket status move
[ ] [fixed] Final commit — docs & remaining changes
[ ] [fixed] Merge & cleanup — merge --no-ff → delete branch
```

## Code Review

Skip for small, single-file changes. Trigger when: 3+ files changed,
new public APIs, or architectural changes.

Dispatch a subagent with the prompt below. Fix Critical and Important
issues, re-run verify, re-dispatch review. Loop until clean. Dismiss
false positives with a brief rationale — do not apply unnecessary fixes.

### Code review prompt

> Review the changes for production readiness.
>
> **Scope:** [which files/modules changed — no design rationale]
> **Requirements:** [ticket phase, plan summary, or description]
> **Plan reference:** Read `@<plan-path>` for intended design and
> success criteria (include only when plan-driven).
> **Project context:** Read `CLAUDE.md` code standards. Read **every
> file** in `ai-docs/mental-model/` regardless of apparent domain
> relevance — cross-module contracts and invariants often surface in
> unrelated domains. Do this before reviewing the diff.
> **Git range:** `git diff $(git merge-base <original-branch> HEAD)..HEAD`
>
> Review as a PM + senior engineer with full project awareness:
> - **Correctness** — logic errors, edge cases, error handling
> - **Plan adherence** — does the implementation match the plan's
>   intent? (include only when plan-driven)
> - **Architectural fit** — respects documented contracts and module
>   boundaries? Unintended coupling?
> - **Test quality** — adequate coverage, no deceptive tests
>   (tautological assertions, unreachable asserts, mocks bypassing
>   code under test, expected values derived from tested logic)
> - **Duplication** — reimplements existing functionality? Search for
>   prior art.
> - **Code standards** — CLAUDE.md conventions
>
> Categorize: Critical / Important / Minor.
> Verdict: ready to merge, or list fixes needed.

## Test Failure Dispatch

Before manual debugging, dispatch a **test-verifier** subagent:

> Analyze the following test failure.
>
> **Failing test:** `test_name` in `path/to/test_file`
> **Implementation:** `path/to/impl_file`
> **Success criteria:** [from plan or ticket]
> **Error output:**
> ```
> [paste test runner output]
> ```

Act on the diagnosis.

## Doc Pipeline

Run after the code review loop fully resolves:

1. Dispatch **mental-model-updater** subagent with changed files and
   summary. Skip if no mental-model impact. Wait for completion.
2. Dispatch **spec-updater** subagent with base commit. Skip if
   `ai-docs/spec/` does not exist. Wait for completion.
3. Update `ai-docs/_index.md` if project capabilities changed.
4. If completing a ticket phase, move ticket status via `git mv`
   (load `/write-ticket` for conventions).
5. Prune aggressively — keep docs focused on current state.

## Report

Present to the user before doc updates proceed:

**Process issues** (skip if nothing notable):
- Dependency doc gaps — missing/wrong/misleading `ai-docs/deps/` docs
- Mental-model inaccuracies — contracts that didn't match reality
- Convention mismatches — docs that diverged from actual code

**Ticket status** (always when ticket-driven):
- Remaining phases or confirmation all complete
- Help user decide `done/` vs `wip/`

When plan-driven, also include **plan deviations**:
- Assumptions that didn't hold
- Steps that required adaptation
- Testing strategy changes (e.g., TDD module switched to post-impl)

## Merge & Cleanup

```bash
git checkout <original-branch>
git merge --no-ff <branch> -m "$(cat <<'EOF'
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem> phase <N>
  > Forward: <what future phases must know>
EOF
)"
git branch -d <branch>
```

Include `## Ticket Updates` when ticket-driven AND forward-facing
findings were discovered. Omit when there are no forwards.

If the user declines, keep the branch intact and stop.
