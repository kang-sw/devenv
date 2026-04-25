---
name: implement
description: >
  Delegated single-scope implementation cycle. An implementer-reviewer pair
  work behind locked contracts; the lead coordinates, merges, and updates
  docs.
argument-hint: "<plan-path or inline brief> [--ticket <ticket-stem>] [--main-branch <name>]"
---

# Delegated Implementation

Target: $ARGUMENTS

## Invariants

- This skill delegates — the lead does not read source code or write implementation.
- When skeleton exists, its stubs and integration tests are the acceptance criteria.
- Ancestor loading (one-level hierarchies — `<domain>/<sub>.md` only): whenever the implementer reads `mental-model/<domain>/<sub>.md`, it reads `mental-model/<domain>/index.md` first so inherited `## Domain Rules` are visible before work begins. The lead propagates this rule through the implementer spawn prompt. See `ws-print-infra executor-wrapup.md §Ancestor Loading`.
- Reviewers write findings to files; lead reads summaries only; implementer reads files directly when non-clean.
- **Main-branch mode** (invoked from `main`/`master`/`trunk`): user approves the report before merge — no code reaches the target branch without user confirmation.
- **Feature-branch mode** (invoked from any other branch): approval gate is skipped; lead auto-merges after clean review. The feature → main merge remains the user's responsibility.
- Implementer and reviewer sessions persist via `ws-call-named-agent` auto-resume throughout the review loop; `ws-new-named-agent` at step 7 resets them to the current run.
- One delegation cycle per invocation.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).
- Task list is created at prepare and tracked to completion — no task may be skipped or reordered.

## On: invoke

### 0. Orient on orchestration primitives

Run `ws-print-infra ws-orchestration.md` (Bash) to orient on `ws-new-named-agent` and `ws-call-named-agent` before orchestrating.

### 1. Prepare

0. Context survey: spawn `project-survey` with the plan path or inline brief. Capture the returned `[Must|Maybe]` reference list — include it in the implementer spawn prompt at step 2.
1. Parse arguments: extract plan path or inline brief, and optional ticket stem.
2. If plan-driven: verify the plan file exists. Read it to extract scope and branch name hint.
3. If brief-driven: the brief is the full specification.
4. Verify skeleton exists: grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs or check for integration tests that reference the target contracts. If absent, stop and suggest `/write-skeleton`.
5. Collect integration test context: identify test file paths and the command to run them. This flows into the implementer spawn prompt.
6. Record current branch as `<original-branch>`. Detect **invocation mode**:
   - `<original-branch>` matches `main`, `master`, `trunk`, or the value of `--main-branch <name>` → **main-branch mode** (approval gate active).
   - Otherwise → **feature-branch mode** (approval gate skipped; auto-merge after clean review).
   Create `implement/<scope>` branch.
7. Register all agent slots upfront:
   ```bash
   ws-new-named-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
   ws-new-named-agent reviewer-correctness --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
   ws-new-named-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"
   ws-new-named-agent reviewer-test --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-test.md)"
   ```
8. Allocate ws-review-path slots — separate Bash call, capture all output lines in lead context:
   ```bash
   ws-review-path correctness fit test   # or partition-allocated subset from judge: partition-allocation
   ```
   Store the returned paths as literals named `correctness-path`, `fit-path`, `test-path`; reference them by name throughout subsequent steps.
9. Create task list. All tasks are mandatory — do not skip or reorder.
   ```
   [ ] Spawn implementer — result arrives synchronously via ws-call-named-agent pipe
   [ ] Spawn reviewers (partition-allocated) — parallel ws-call-named-agent calls; reviewers write to files; lead reads summaries; implementer reads files directly when non-clean → re-review loop until clean
   [ ] Dispatch mental-model-updater + spec-updater in parallel — wait for both; surface ambiguous stems
   [ ] Report to user — wait for approval  ← main-branch mode only
     > if tweaks requested: implementer fixes → re-verify → reviewer re-reviews → re-run both updaters (loop)
   [ ] Merge to original branch
   [ ] Update project docs — refresh ai-docs/_index.md, ticket status
   [ ] Cleanup — review files deleted; agent registry entries left in place (fresh per run)
   ```

### 2. Spawn implementer

```bash
ws-call-named-agent implementer - <<'PROMPT'
Mode: <A: plan-driven | B: inline brief>
<Plan path | Brief text>

Acceptance criteria: skeleton integration tests must pass.
- Test files: <integration test paths>
- Run: <command to execute them>

Mental-model ancestor loading (one-level hierarchies —
`<domain>/<sub>.md` only):
- When you read `ai-docs/mental-model/<domain>/<sub>.md`, read
  `ai-docs/mental-model/<domain>/index.md` first so inherited
  `## Domain Rules` are visible before any edit.

Instructions:
- Verify integration tests pass before reporting completion or after each fix.
- Report completion in plain text. Include test results.
- For fix cycles, a follow-up call will arrive with review findings — fix and report back via your next response.
- Commit at logical checkpoints on the current branch.
PROMPT
```

Note the commit range from the implementer's report.

### 3. Review

#### 3a. Partition allocation

Apply `judge: partition-allocation` to determine which review partitions apply
based on the implementer's report and the nature of the changes.

#### 3b. Spawn reviewers

Spawn one reviewer per selected partition in parallel — issue multiple Bash
calls in the same response turn. Each reviewer loads its partition doc via
`--system-prompt`:

| Partition | System prompt |
|-----------|---------------|
| Correctness | `$(ws-infra-path code-review-correctness.md)` |
| Fit | `$(ws-infra-path code-review-fit.md)` |
| Test | `$(ws-infra-path code-review-test.md)` |

```bash
ws-call-named-agent reviewer-correctness - <<'PROMPT'
Diff range: <first-commit>..<last-commit>

Instructions:
- Report findings in plain text.
- The lead will relay a re-review request if fixes are needed — re-examine
  the updated diff and respond.
- Write your full findings to: <correctness-path>
- Return only: [clean|non-clean]: <one-line characterization of most significant issues>
PROMPT
```

```bash
ws-call-named-agent reviewer-fit - <<'PROMPT'
Diff range: <first-commit>..<last-commit>

Instructions:
- Report findings in plain text.
- The lead will relay a re-review request if fixes are needed — re-examine
  the updated diff and respond.
- Write your full findings to: <fit-path>
- Return only: [clean|non-clean]: <one-line characterization of most significant issues>
PROMPT
```

```bash
ws-call-named-agent reviewer-test - <<'PROMPT'
Diff range: <first-commit>..<last-commit>

Instructions:
- Report findings in plain text.
- The lead will relay a re-review request if fixes are needed — re-examine
  the updated diff and respond.
- Write your full findings to: <test-path>
- Return only: [clean|non-clean]: <one-line characterization of most significant issues>
PROMPT
```

#### 3c. Relay and loop

1. If all reviewers return a `[clean]` summary → exit review loop, proceed to step 4.
2. Otherwise: relay file paths to the implementer:
   ```bash
   ws-call-named-agent implementer - <<'PROMPT'
   Fix issues in these review reports: <correctness-path>, <fit-path>, <test-path>. Read each file directly.
   PROMPT
   ```
   Wait for the implementer's fix report and integration test confirmation.
3. Re-review (parallel — issue multiple Bash calls in the same response, same paths — reviewers overwrite):
   ```bash
   ws-call-named-agent reviewer-correctness - <<'PROMPT'
   Re-review. Updated diff: <diff>
   PROMPT
   ws-call-named-agent reviewer-fit - <<'PROMPT'
   Re-review. Updated diff: <diff>
   PROMPT
   ws-call-named-agent reviewer-test - <<'PROMPT'
   Re-review. Updated diff: <diff>
   PROMPT
   ```
4. Repeat from 3c.1 until all reviewers return `[clean]`.

### 4. Docs pre-pass

1. Dispatch **spec-updater** with the commit range. Wait for it to complete. If **spec-updater** reports ambiguous stems, note them for step 5.
2. Dispatch **mental-model-updater**. Wait for it to complete. Running spec-updater first ensures mental-model-updater's spec-diff check captures any 🚧 strips committed by spec-updater.

### 5. Report and approval

> **Feature-branch mode**: emit the report below, then proceed directly to step 6 (Merge) — do not wait for user approval.

1. Report to the user:
   - What was implemented (from implementer report)
   - Review result (from reviewer report)
   - Test status
   - Any deviations or open items
2. **Main-branch mode only** — wait for user approval. If the user requests tweaks:
   - Direct the implementer to fix:
     ```bash
     ws-call-named-agent implementer - <<'PROMPT'
     Fix these issues: <tweak requests>
     PROMPT
     ```
     Implementer verifies integration tests and reports.
   - Re-apply `judge: partition-allocation` and re-review per the step 3 pattern.
   - Re-run **spec-updater** with the new commit range. Wait. Then re-run **mental-model-updater**. Wait.
   - Re-report. Loop until user approves.

Implementer and reviewer sessions remain available throughout this loop via `ws-call-named-agent` auto-resume.

### 6. Merge

1. Run `ws-merge-branch <original-branch> <branch> "<commit-message>"`.
   The script selects strategy by commit count: squash (1 commit) or --no-ff (2+).
   Compose the commit message per CLAUDE.md commit rules.

### 7. Doc pipeline

Run `ws-print-infra executor-wrapup.md`. Follow §Doc Pipeline, §Doc Commit Gate, and (if ticket-driven) §Ticket Update. Pass the merge commit range.

### 8. Cleanup

1. `rm -f <correctness-path> <fit-path> <test-path>  # literal paths from lead context`
2. Agent registry entries are created fresh per run via `ws-new-named-agent` — no explicit teardown needed.

## Judgments

### judge: partition-allocation

Evaluate based on the implementer's report and the nature of the changes.

| Partition | Assign when |
|-----------|-------------|
| **Correctness** | New logic introduced, error paths modified, contracts or security surface touched |
| **Fit** | Existing components reused or modified, new patterns others will follow |
| **Test** | Test files added or modified, or new code paths added without existing coverage |
| **Default** | New feature or non-trivial cross-module change → all three partitions |
| **Floor** | Purely mechanical change (format, rename with no semantic change) → Correctness only |

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Brief is a small, isolated change (single file, no public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

## Doctrine

Implementation optimizes for **contract-bounded autonomy** —
the implementer has full freedom within skeleton-locked contracts, and
the reviewer validates without lead involvement. When a rule is
ambiguous, apply whichever interpretation better preserves the
implementer's autonomy within contract boundaries.
