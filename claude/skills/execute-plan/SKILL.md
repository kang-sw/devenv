---
name: execute-plan
description: When a tactical plan file from /write-plan is ready for execution, invoke this.
argument-hint: <plan-path>
---

# Execute Plan

Plan: $ARGUMENTS

## Invariants

- Follow the plan's contracts and decisions exactly — do not re-research alternatives or second-guess design choices.
- Implementation code is your job; the plan specifies what to build and which signatures/types to use.
- Read target files and follow referenced patterns; fix construction sites the compiler surfaces.
- Preserve the plan's step ordering and testing classifications (TDD/post-impl/manual) in derived tasks.
- Fixed tasks marked `[fixed]` in the task list are mandatory bookends — never skip or reorder them.
- Claim "pass" only after reading actual test output — never "should pass" or "looks correct."
- Commit freely at logical checkpoints on the feature branch; keep messages brief.
- Do not include design justifications in review prompts.

## On: invoke

1. Read the plan file at `$ARGUMENTS`.
2. Read mental-model docs referenced in the plan's Context section. Read additional mental-model docs only if the plan explicitly instructs it.
3. Record the current branch as `<original-branch>`. If already on an `execute/` branch, treat it as a resumed session — infer `<original-branch>` from the merge-base with `main`, skip branch creation, and continue from the existing task list. Otherwise, create branch `execute/<scope>` from the current branch.
4. Convert the plan's Steps into tasks via `TaskCreate` using the **task list format**. Fill plan-derived tasks between the fixed bookends. Include the plan's step number in each task subject.
5. Execute tasks sequentially: set to `in_progress`, do the work, set to `completed`.

### Testing execution

Route by the plan's annotation for each module:

| Annotation | Execution |
|------------|-----------|
| **TDD** | Write API stubs (signatures + unimplemented placeholder) > write tests > implement until tests pass |
| **post-impl** | Implement first, then add tests for observable behavior |
| **manual** | Implement without automated tests; note in report |

For TDD modules, follow the plan's delegation strategy if specified. Default: main agent writes complex/edge-case tests first (exemplars); if 3+ simple cases remain, delegate population to a subagent using the exemplar as pattern reference.

### Test-case delegation

When delegating test cases, the prompt must include: (1) exemplar test location, (2) cases to add (input/expected-output pairs or scenarios), (3) success criteria: compiles + fails on unimplemented stubs, (4) bail-out: skip & report if the exemplar pattern doesn't fit.

| Method | When |
|--------|------|
| **haiku subagent** | Parameter-only variation (same assertion structure, different inputs) |
| **sonnet subagent** | New scenario following the exemplar pattern |

Review the subagent's diff before proceeding.

### Test failure analysis

Before manual debugging, dispatch a **test-verifier subagent** with the **test-verifier prompt**. Act on the diagnosis.

### Mechanical-edit delegation

When a repetitive edit spans 3+ locations:

| Method | When |
|--------|------|
| **sed / replace_all** | Pure text substitution expressible as regex |
| **haiku subagent** | Fixed pattern, no ambiguity, no judgment needed |
| **sonnet subagent** | Needs structural understanding or has any ambiguity |

Prompt must include: (1) before/after example from the first instance, (2) target file list, (3) success criteria (e.g., `cargo check` passes), (4) bail-out condition (sonnet only). On failure, subagent runs `git checkout -- <modified-files>` and reports.

### Verify task

Run the project's test suite(s) and build step (see `ai-docs/_index.md` for commands). Skip if the project has no test suite. Read the full output.

### Code review task

Skip for small, single-file changes. Run when changes touch 3+ files, new public APIs, or architectural changes. Dispatch a general-purpose subagent with the **code review prompt**. Fix Critical and Important issues, re-run verify, re-dispatch review. Loop until clean. Dismiss false positives with a brief rationale.

### Mental-model-updater task

Wait for the code review loop to fully resolve. Dispatch a **mental-model-updater subagent** with the list of changed files and a summary of additions/modifications. Wait for completion before spec-updater. Skip if the change has no mental-model impact.

### Spec-updater task

Dispatch a **spec-updater subagent** with the base commit (branch point) to check whether public-facing features were affected. Skip if `ai-docs/spec/` does not exist. Wait for completion before docs task.

### Docs task

- Update `ai-docs/_index.md` if project capabilities changed.
- If completing a ticket phase, load `/write-ticket` for conventions, then append `### Result` to the ticket doc.
- Prune aggressively — keep docs focused on current state.

### Report task

Present the **report format** to the user. The user reviews and approves before doc updates proceed.

### Final commit task

Commit remaining docs and cleanup changes.

### Merge & cleanup task

The user already approved at the report step. Merge using the **merge commit format**. If the user declines, keep the branch intact and stop.

## Judgments

### judge: plan-deviation

When the plan's assumptions don't match the current codebase, classify the gap:

- **Cosmetic** (renamed param, minor signature change) — adapt silently, note in report.
- **Structural** (referenced file/type/function missing or fundamentally different interface) — ask the user before proceeding.

## Templates

### Task list format

```
[ ] [fixed] Verify plan assumptions — read target files, confirm plan's claims
  ... (tasks derived from plan's Steps) ...
[ ] [fixed] Run tests & verify — full test suite, read actual output
[ ] [fixed] Code review — dispatch subagent
  > if Critical/Important issues: fix > re-test > re-review (loop until clean)
[ ] [fixed] Report — plan deviations (if any), process issues (if any); user approves before doc updates
[ ] [fixed] Update mental model with mental-model-updater subagent
[ ] [fixed] Update spec with spec-updater subagent
[ ] [fixed] Update project docs — ai-docs/_index.md, ticket result
[ ] [fixed] Final commit — docs & remaining changes
[ ] [fixed] Merge & cleanup — merge --no-ff > delete branch
```

### Code review prompt

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
> - **Architectural fit** — does this change respect documented contracts and module boundaries? Any unintended coupling or side effects?
> - **Test quality** — adequate coverage, no deceptive tests (tautological assertions, unreachable assert paths, mocks bypassing code under test, expected values derived from the logic being tested)
> - **Duplication** — does this reimplement functionality that already exists in the codebase? Search for prior art if in doubt.
> - **Code standards** — adherence to CLAUDE.md conventions
>
> Categorize issues as Critical / Important / Minor.
> Give a clear verdict: ready to merge, or list fixes needed.

### Test-verifier prompt

> Analyze the following test failure.
>
> **Failing test:** `test_name` in `path/to/test_file`
> **Implementation:** `path/to/impl_file`
> **Success criteria:** [from the plan's Testing section for this module]
> **Error output:**
> ```
> [paste test runner output]
> ```

### Report format

**Plan deviations** (always include if any):
- Assumptions that didn't hold
- Steps that required adaptation
- Delegation strategy changes
- Testing strategy changes (e.g., TDD module switched to post-impl)

**Process issues** (skip if nothing notable):
- Dependency doc gaps — APIs missing, wrong, or misleading in `ai-docs/deps/`
- Mental-model inaccuracies — contracts or invariants that didn't match reality
- Convention mismatches — patterns described in docs that diverged from actual code

**Ticket status** (always include when a ticket was the input):
- Remaining phases or confirmation that all phases are complete

### Merge commit format

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

## Doctrine

Plan execution optimizes for **faithful contract delivery** — the plan's
decisions are the spec, and implementation details fill the gaps the plan
intentionally left open. When a rule is ambiguous, apply whichever
interpretation better preserves the plan author's stated contracts and
testing intent.
