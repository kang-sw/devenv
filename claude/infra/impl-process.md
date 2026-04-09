# Implementation Process

## Invariants

- Bookend tasks marked `[fixed]` are mandatory — never skip or reorder.
- Code review loops until no Critical/Important issues remain.
- User approves the report before doc-update tasks proceed.
- Dismiss false-positive review issues with rationale — do not apply unnecessary fixes.
- Doc pipeline runs only after the code review loop fully resolves.

## §Task List

```
[ ] [fixed] Collect context — read target files, verify contracts
  ... (implementation tasks — commit freely at logical points) ...
[ ] [fixed] Verify — full test suite, read actual output
[ ] [fixed] Code review — dispatch subagent (skip for small single-file changes)
  > if Critical/Important: fix → re-verify → re-review (loop until clean)
[ ] [fixed] Report to user — user approves before doc updates
[ ] [fixed] Update mental model — dispatch mental-model-updater subagent, wait for completion
[ ] [fixed] Update project docs — refresh ai-docs/_index.md, ticket status move
[ ] [fixed] Final commit — docs & remaining changes
[ ] [fixed] Merge & cleanup — merge --no-ff → delete branch
```

## §Code Review

Skip for small, single-file changes. Trigger: 3+ files, new public APIs, or architectural changes.

Dispatch → fix Critical/Important → re-verify → re-review. Loop until clean.

### Code review prompt

> Review the changes for production readiness.
>
> **Scope:** [files/modules changed — no design rationale]
> **Requirements:** [ticket phase, plan summary, or description]
> **Plan reference:** Read `@<plan-path>` for design and success criteria (plan-driven only).
> **Project context:** Read `CLAUDE.md` code standards. Read **every file** in `ai-docs/mental-model/` before reviewing the diff.
> **Git range:** `git diff $(git merge-base <original-branch> HEAD)..HEAD`
>
> - **Correctness** — logic errors, edge cases, error handling
> - **Plan adherence** — matches plan intent? (plan-driven only)
> - **Skeleton integrity** — if skeleton stubs/tests exist (`skeletons:` in ticket frontmatter), verify no skeleton contract was modified outside of plan's Skeleton Amendments. (Critical if violated.)
> - **Architectural fit** — respects contracts and module boundaries?
> - **Test quality** — no deceptive tests (tautological assertions, unreachable asserts, mocks bypassing code under test, expected values derived from tested logic)
> - **Duplication** — reimplements existing functionality?
> - **Code standards** — CLAUDE.md conventions
>
> Categorize: Critical / Important / Minor. Verdict: ready or fixes needed.

## §Test Failure Dispatch

Before manual debugging, dispatch a **test-verifier** subagent:

> **Failing test:** `test_name` in `path/to/test_file`
> **Implementation:** `path/to/impl_file`
> **Success criteria:** [from plan or ticket]
> **Error output:**
> ```
> [paste test runner output]
> ```

Act on the diagnosis.

## §Doc Pipeline

1. Dispatch **mental-model-updater** with changed files and summary. Always dispatch — the agent determines impact. Wait.
2. Refresh `ai-docs/_index.md` — update inventory, descriptions, and layout to reflect current state. Always run, not conditional.
3. If completing a ticket phase, move ticket status via `git mv` (load `/write-ticket` for conventions).
4. Prune aggressively — docs reflect current state only.

## §Report

Present before doc updates. User approves before proceeding.

**Process issues** (skip if nothing notable):
- Dependency doc gaps — missing/wrong/misleading in `ai-docs/deps/`
- Mental-model inaccuracies — contracts that didn't match reality
- Convention mismatches — docs diverged from actual code

**Ticket status** (always when ticket-driven):
- Remaining phases or all complete; help user decide `done/` vs `wip/`

**Plan deviations** (plan-driven only):
- Assumptions that didn't hold, steps adapted, testing strategy changes

## §Merge & Cleanup

```bash
git checkout <original-branch>
git merge --no-ff <branch> -m "$(cat <<'EOF'
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- <decision rationale, rejected alternatives, user directives>

## Ticket Updates                          # optional — only when ticket-driven
- <ticket-stem>[: <optional-label>]
  > Forward: <what future phases must know>
EOF
)"
git branch -d <branch>
```

Include `## Ticket Updates` when ticket-driven AND forward-facing findings exist. If user declines merge, keep branch intact.

## Doctrine

The process optimizes for **verified delivery** — no branch merges
until every gate (test, review, user approval) has passed in order.
When a rule is ambiguous, apply whichever interpretation better
ensures that unreviewed or unapproved changes never reach the target
branch.
