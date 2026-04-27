---
name: implement
description: >
  Delegated implementation cycle. Lead writes a brief from the target, optionally
  populates a plan via survey or research, then an implementer-reviewer pair
  execute behind locked contracts.
argument-hint: "<ticket-path or inline description> [--ticket <ticket-stem>]"
---

# Delegated Implementation

Target: $ARGUMENTS

## Invariants

- This skill delegates — the lead does not read source code or write implementation.
- Implementer reads only the brief — never the ticket directly.
- Fit reviewer may reference the ticket for architectural headroom checks (future-phase room); correctness and test reviewers do not need the ticket.
- When skeleton exists, its stubs and integration tests are the acceptance criteria.
- Ancestor loading (one-level hierarchies — `<domain>/<sub>.md` only): whenever the implementer reads `mental-model/<domain>/<sub>.md`, it reads `mental-model/<domain>/index.md` first so inherited `## Domain Rules` are visible before work begins. The lead propagates this rule through the implementer spawn prompt. See `ws-print-infra executor-wrapup.md §Ancestor Loading`.
- Reviewers write findings to files; lead reads summaries only; implementer reads files directly when non-clean.
- User approves the report before merge — no code reaches the target branch without user confirmation.
- Implementer and reviewer sessions persist via `ws-call-named-agent` auto-resume throughout the review loop; `ws-new-named-agent` at step 5 resets them to the current run.
- One delegation cycle per invocation.
- Follow CLAUDE.md commit rules for the merge commit (including `## Ticket Updates` when ticket-driven).
- Task list is created at prepare and tracked to completion — no task may be skipped or reordered.

## On: invoke

### 0. Orient on orchestration primitives

Run `ws-print-infra ws-orchestration.md` (Bash) to orient on `ws-new-named-agent` and `ws-call-named-agent` before orchestrating.

### 1. Read target

Parse `$ARGUMENTS`: extract ticket path or inline description, and optional `--ticket <stem>`.
If ticket-driven: read the ticket. Extract scope, stem, and phase context (prior phase plans, forwards).

### 2. Write brief

Lead writes `ai-docs/plans/YYYY-MM/DD-<stem>.brief.md` directly using the **brief template** (see Templates). Strip ticket noise — this file is the implementer's sole context source.

### 3. Plan depth

Apply `judge: plan-depth` (see Judgments). Default to survey when uncertain between as-is and survey.

**as-is** — proceed to step 5.

**survey** — register and call plan surveyor (sonnet):

```bash
ws-new-named-agent plan-surveyor --model sonnet --system-prompt "$(ws-infra-path plan-populator-survey.md)"
```

```bash
ws-call-named-agent plan-surveyor - <<'PROMPT'
Brief path: <brief-path>
Plan path: ai-docs/plans/YYYY-MM/DD-<stem>.md
PROMPT
```

**research** — register and call plan researcher (opus):

```bash
ws-new-named-agent plan-researcher --model opus --system-prompt "$(ws-infra-path plan-populator-research.md)"
```

```bash
ws-call-named-agent plan-researcher - <<'PROMPT'
Brief path: <brief-path>
Plan path: ai-docs/plans/YYYY-MM/DD-<stem>.md
PROMPT
```

After the population agent returns, commit the brief and plan files before proceeding.

### 4. Prepare

0. Context survey: spawn `project-survey` with the brief path. Capture the returned `[Must|Maybe]` reference list — include it in the implementer spawn prompt at step 5.
1. Derive `<scope>` from `<stem>` for branch naming.
2. Verify skeleton exists: grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs or check for integration tests that reference the target contracts. Apply `judge: skeleton-check`. If skeleton required but absent, stop and suggest `/write-skeleton`.
3. Collect integration test context: identify test file paths and the command to run them. This flows into the implementer spawn prompt.
4. Record current branch as `<original-branch>`. Create `implement/<scope>` branch.
5. Register all agent slots upfront:
   ```bash
   ws-new-named-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
   ws-new-named-agent reviewer-correctness --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
   ws-new-named-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"
   ws-new-named-agent reviewer-test --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-test.md)"
   ```
6. Allocate ws-review-path slots — separate Bash call, capture all output lines in lead context:
   ```bash
   ws-review-path correctness fit test
   ```
   Store the returned paths as literals named `correctness-path`, `fit-path`, `test-path`.
7. Create task list. All tasks are mandatory — do not skip or reorder.
   ```
   [ ] Spawn implementer — background Bash call; read output after notification via ws-print-named-agent-output
   [ ] Spawn reviewers (partition-allocated) — parallel background Bash calls; read summaries after all notifications; implementer reads review files directly when non-clean → re-review loop until clean
   [ ] Dispatch mental-model-updater + spec-updater in parallel — wait for both; surface ambiguous stems
   [ ] Report to user — wait for approval
     > if tweaks requested: implementer fixes → re-verify → reviewer re-reviews → re-run both updaters (loop)
   [ ] Merge to original branch
   [ ] Update project docs — refresh ai-docs/_index.md, ticket status
   [ ] Cleanup — review files deleted; agent registry entries left in place (fresh per run)
   ```

### 5. Spawn implementer

Issue the Bash tool call with `run_in_background: true`. Lead continues other work until the completion notification arrives.

```bash
ws-call-named-agent implementer - <<'PROMPT'
Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>

Read only the brief (and plan if provided) for implementation context. Do not read the ticket directly.

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

After the completion notification arrives, read the implementer's report:

```bash
ws-print-named-agent-output implementer
```

Note the commit range from the report.

### 6. Review

#### 6a. Partition allocation

Apply `judge: partition-allocation` to determine which review partitions apply
based on the implementer's report and the nature of the changes.

#### 6b. Spawn reviewers

Spawn one reviewer per selected partition in parallel — issue multiple Bash
calls in the same response turn, each with `run_in_background: true`.
After all completion notifications arrive, read each reviewer's summary via
`ws-print-named-agent-output <reviewer-name>`. Each reviewer loads its partition doc via
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
Brief path: <brief-path>

Instructions:
- Report findings in plain text.
- The brief describes implementation intent — use it to judge whether the
  implementation achieves what was intended and leaves room for future phases.
- You may also reference the ticket at <ticket-path> for architectural headroom
  checks (e.g., does this implementation close off doors that planned future
  phases require?). Ticket reference is optional and supplementary.
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

#### 6c. Relay and loop

1. If all reviewers return a `[clean]` summary → exit review loop, proceed to step 7.
2. Otherwise: relay file paths to the implementer (background Bash, `run_in_background: true`):
   ```bash
   ws-call-named-agent implementer - <<'PROMPT'
   Fix issues in these review reports: <correctness-path>, <fit-path>, <test-path>. Read each file directly.
   PROMPT
   ```
   After the completion notification, read the fix report: `ws-print-named-agent-output implementer`.
3. Re-review (parallel background — multiple Bash calls in the same response, `run_in_background: true`, same paths — reviewers overwrite):
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
   After all notifications, read summaries via `ws-print-named-agent-output` for each reviewer.
4. Repeat from 6c.1 until all reviewers return `[clean]`.

### 7. Docs pre-pass

1. Dispatch **spec-updater** with the commit range. Wait for it to complete. If **spec-updater** reports ambiguous stems, note them for step 8.
2. Dispatch **mental-model-updater**. Wait for it to complete. Running spec-updater first ensures mental-model-updater's spec-diff check captures any 🚧 strips committed by spec-updater.

### 8. Report and approval

1. Report to the user:
   - What was implemented (from implementer report)
   - Review result (from reviewer report)
   - Test status
   - Any deviations or open items
2. Wait for user approval. If the user requests tweaks:
   - Direct the implementer to fix:
     ```bash
     ws-call-named-agent implementer - <<'PROMPT'
     Fix these issues: <tweak requests>
     PROMPT
     ```
     Implementer verifies integration tests and reports.
   - Re-apply `judge: partition-allocation` and re-review per the step 6 pattern.
   - Re-run **spec-updater** with the new commit range. Wait. Then re-run **mental-model-updater**. Wait.
   - Re-report. Loop until user approves.

Implementer and reviewer sessions remain available throughout this loop via `ws-call-named-agent` auto-resume.

### 9. Merge

1. Run `ws-merge-branch <original-branch> <branch> "<commit-message>"`.
   The script selects strategy by commit count: squash (1 commit) or --no-ff (2+).
   Compose the commit message per CLAUDE.md commit rules.

### 10. Doc pipeline

Run `ws-print-infra executor-wrapup.md`. Follow §Doc Pipeline, §Doc Commit Gate, and (if ticket-driven) §Ticket Update. Pass the merge commit range.

### 11. Cleanup

1. `rm -f <correctness-path> <fit-path> <test-path>  # literal paths from lead context`
2. Agent registry entries are created fresh per run via `ws-new-named-agent` — no explicit teardown needed.

## Templates

### Brief format

Path: `ai-docs/plans/YYYY-MM/DD-<stem>.brief.md`

```markdown
# Brief: <stem>

## Intent
<what this achieves — one paragraph>

## Approach
<macro-level how — bullets>

## Constraints
<must-hold conditions>

## Out of scope
<explicitly excluded from this implementation>

## Details
<interface specs, data types, public contracts at ticket-level resolution>
<required when no skeleton has been run; may be omitted when skeleton provides contracts>
```

## Judgments

### judge: plan-depth

Soft judgment. Apply the signals below; default to survey when uncertain between as-is and survey.

| Signal | Suggests |
|--------|----------|
| Brief already names concrete change points; single-file or single-function scope | as-is |
| Multi-module span; cold implementer; reuse points likely but unconfirmed | survey |
| Multiple viable strategies; non-obvious cross-module side effects | research |

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
the reviewer validates without lead involvement. The brief is the implementer's
sole context source; its authorship by the lead ensures owner intent survives
delegation without ticket noise. When a rule is ambiguous, apply whichever
interpretation better preserves the implementer's autonomy within contract boundaries.
