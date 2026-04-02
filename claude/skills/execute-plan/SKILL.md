---
name: execute-plan
description: When a tactical plan file from /write-plan is ready for execution.
argument-hint: <plan-path>
---

# Execute Plan

Plan: $ARGUMENTS

## Doctrine

You are a **plan executor**. The plan specifies **contracts and decisions**
(what to build, which types/signatures, design choices). Follow these exactly.
Do not re-research alternatives or second-guess design decisions.

The plan does **not** specify implementation code — that is your job. Read
target files, follow referenced patterns, fix construction sites the compiler
surfaces, and write tests for the plan's key scenarios. Use your judgment
for all implementation details within the plan's constraints.

**When to deviate:** only when the plan's assumptions provably don't match
the current codebase. Threshold:
- **Cosmetic** (renamed param, minor signature change) — adapt silently,
  note in report.
- **Structural** (referenced file/type/function missing or fundamentally
  different interface) — ask the user before proceeding.

## Step 0: Load Plan

1. Read the plan file at `$ARGUMENTS`.
2. Read mental-model docs referenced in the plan's Context section. Read
   additional mental-model docs only if the plan explicitly instructs it.
4. Record the current branch as `<original-branch>`. If already on an
   `execute/` branch, treat it as a resumed session — infer
   `<original-branch>` from the merge-base with `main`, skip branch creation,
   and continue from the existing task list. Otherwise, create a feature branch:
   `execute/<scope>` from the current branch.

## Step 1: Task List

Convert the plan's Steps into tasks via `TaskCreate`. The tasks marked
`[fixed]` are mandatory bookend tasks — do not skip or reorder them.
Fill plan-derived tasks between them:

```
[ ] [fixed] Verify plan assumptions — read target files, confirm plan's claims
  ... (tasks derived from plan's Steps) ...
[ ] [fixed] Run tests & verify — full test suite, read actual output
[ ] [fixed] Code review — dispatch subagent
  > if Critical/Important issues: fix > re-test > re-review (loop until clean)
[ ] [fixed] Update mental model with mental-model-updater subagent
[ ] [fixed] Update spec with spec-updater subagent
[ ] [fixed] Update project docs — ai-docs/_index.md, ticket result
[ ] [fixed] Final commit — docs & remaining changes
[ ] [fixed] Report — plan deviations (if any), process issues (if any)
[ ] [fixed] Merge & cleanup — user confirms > merge --no-ff > delete branch
```

**Plan-derived tasks** must preserve the plan's ordering and testing
classifications (TDD/post-impl/manual). Include the plan's step number
in each task subject for traceability.

## Step 2: Execute Tasks

Work through tasks sequentially. For each:

1. Set task to `in_progress`.
2. Do the work as specified by the plan.
3. Set task to `completed`.

### Following the plan's Testing section

The plan classifies modules into testing approaches. Follow them exactly:

| Plan annotation | Execution |
|----------------|-----------|
| **TDD** | Write API stubs (signatures + unimplemented placeholder) > write tests > implement until tests pass |
| **post-impl** | Implement first, then add tests for observable behavior |
| **manual** | Implement without automated tests; note in report |

For TDD modules, the plan may specify delegation strategy. If it does,
follow it. If not, use this default:

1. Main agent writes complex/edge-case tests first (exemplars).
2. If 3+ simple cases remain, delegate population to a subagent using the
   exemplar as the pattern reference.

### Test-case delegation

When the plan or the default strategy calls for test delegation:

**Required in the delegation prompt:**
1. Exemplar test location (written by the main agent)
2. Cases to add (input/expected-output pairs or scenario descriptions)
3. Success criteria: compiles + fails on unimplemented stubs
4. Bail-out: if the exemplar pattern doesn't fit a case, skip & report

| Method | When |
|--------|------|
| **haiku subagent** | Parameter-only variation (same assertion structure, different inputs) |
| **sonnet subagent** | New scenario following the exemplar pattern |

**Main agent responsibility:** Review the subagent's diff before proceeding.

### Test failure analysis

When tests fail after implementation, before manual debugging, dispatch a
**test-verifier subagent** with a prompt like:

> Analyze the following test failure.
>
> **Failing test:** `test_name` in `path/to/test_file`
> **Implementation:** `path/to/impl_file`
> **Success criteria:** [from the plan's Testing section for this module]
> **Error output:**
> ```
> [paste test runner output]
> ```

The verifier analyzes whether the failure is a test defect or an
implementation bug, and returns a diagnosis. Act on the diagnosis.

### Mechanical-edit delegation

When a repetitive edit spans 3+ locations:

| Method | When |
|--------|------|
| **sed / replace_all** | Pure text substitution expressible as regex |
| **haiku subagent** | Fixed pattern, no ambiguity, no judgment needed |
| **sonnet subagent** | Needs structural understanding or has any ambiguity |

**Required in the delegation prompt:**
1. Before/after example (extracted from the first instance)
2. Target file list
3. Success criteria (e.g., `cargo check` passes)
4. Bail-out condition (sonnet only)

**Rollback:** On failure, subagent runs `git checkout -- <modified-files>`
and reports.

### Verify task

Run the project's test suite(s) and build step (see `ai-docs/_index.md` for commands). Skip if the project has no test suite.
**Read the full output.** Claim "pass" only after confirming the actual
result — never "should pass" or "looks correct."

### Code review task

Skip for small, single-file changes. **When to run:** changes touching
3+ files, new public APIs, or architectural changes.

**Dispatch a general-purpose subagent with:**

> Review the changes for production readiness.
>
> **Scope:** [which files/modules changed — no design rationale]
> **Requirements:** [plan summary or ticket reference]
> **Plan reference:** Read `@<plan-path>` for intended design and success criteria.
> **Project context:** Read `CLAUDE.md` code standards. Read **every file**
> in `ai-docs/mental-model/` regardless of apparent domain relevance —
> cross-module contracts and invariants often surface in unrelated domains.
> Do this before reviewing the diff.
> **Git range:** `git diff $(git merge-base <original-branch> HEAD)..HEAD`
>
> Review as a PM + senior engineer with full project awareness:
> - **Correctness** — logic errors, edge cases, error handling
> - **Plan adherence** — does the implementation match the plan's intent?
> - **Architectural fit** — does this change respect documented contracts
>   and module boundaries? Any unintended coupling or side effects?
> - **Test quality** — adequate coverage, no deceptive tests (tautological
>   assertions, unreachable assert paths, mocks bypassing code under test,
>   expected values derived from the logic being tested)
> - **Duplication** — does this reimplement functionality that already
>   exists in the codebase? Search for prior art if in doubt.
> - **Code standards** — adherence to CLAUDE.md conventions
>
> Categorize issues as Critical / Important / Minor.
> Give a clear verdict: ready to merge, or list fixes needed.

Do **not** include design justifications in the review prompt.

Fix Critical and Important issues. Re-run verify, then re-dispatch review.
Loop until clean. Dismiss false positives with a brief rationale.

### Mental-model-updater task

Wait for the code review loop to fully resolve before starting — fixes
may change the final implementation.

Dispatch a **mental-model-updater subagent** with the list of files changed
and a summary of what was added/modified. Wait for completion before the
spec-updater task — docs must reflect the final mental-model state. Skip if
the change has no mental-model impact (e.g., config tweaks, typo fixes).

### Spec-updater task

Dispatch a **spec-updater subagent** with the base commit (branch point) to
check whether public-facing features were affected. Skip if `ai-docs/spec/`
does not exist. Wait for completion before the docs task.

### Docs task

- Update `ai-docs/_index.md` if project capabilities changed.
- If completing a ticket phase, load `/write-ticket` for conventions, then
  append `### Result` to the ticket doc.
- Prune aggressively — keep docs focused on current state.

### Intermediate commits

Work happens on a feature branch — commit freely at logical checkpoints.
Keep messages brief.

### Final commit task

Commit remaining docs and cleanup changes.

### Report task

Report to the user:

**Plan deviations** (always include if any):
- Assumptions that didn't hold
- Steps that required adaptation
- Delegation strategy changes
- Testing strategy changes (e.g., TDD module switched to post-impl)

**Process issues** (skip if nothing notable):
- Dependency doc gaps — APIs that were missing, wrong, or misleading in
  `ai-docs/deps/` docs
- Mental-model inaccuracies — contracts or invariants that didn't match reality
- Convention mismatches — patterns described in docs that diverged from actual
  code

**Ticket status** (always include when a ticket was the input):
- Remaining phases or confirmation that all phases are complete

The user reviews this report before confirming merge.

### Merge & cleanup task

After the user has reviewed the report, ask for final confirmation.

```bash
git checkout <original-branch>
git merge --no-ff execute/<scope> -m "$(cat <<'EOF'
<type>(<scope>): <summary>

<what changed — brief>

## AI Context
- Plan: <plan-path>
- <deviations, decisions made during execution>
EOF
)"
git branch -d execute/<scope>
```

If the user declines, keep the branch intact and stop.
