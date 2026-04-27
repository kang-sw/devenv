---
name: write-code
description: >
  Core implementation primitive. Reads a target, writes a brief, optionally
  populates a plan, then runs a delegated implementer–reviewer cycle on the
  current branch. Returns commit range and test status to the caller.
argument-hint: "<ticket-path or inline description> [--ticket <stem>]"
---

# Write Code

Target: $ARGUMENTS

## Invariants

- Operates on the current branch — branch creation is the caller's responsibility.
- Implementer reads only the brief — never the ticket directly.
- Fit reviewer may reference the ticket for architectural headroom checks; correctness and test reviewers do not.
- When skeleton exists, its stubs and integration tests are the acceptance criteria.
- Ancestor loading: when implementer reads `mental-model/<domain>/<sub>.md`, it reads `mental-model/<domain>/index.md` first. Lead propagates this rule in the implementer spawn prompt.
- Reviewers write findings to files; lead reads summaries only; implementer reads files directly when non-clean.
- Implementer and reviewer sessions persist via `ws-call-named-agent` auto-resume throughout the review loop.
- Review cycle cap: 3 relays maximum. Lead adjudicates at cycle 2; user escalation at cycle 3.
- Self-cleanup: review path files are deleted before returning.
- On completion, output the commit range, test status, and brief path in the format defined in Templates.

## On: invoke

### 0. Orient

Run `ws-print-infra ws-orchestration.md` (Bash).

### 1. Read target

Parse `$ARGUMENTS`: extract ticket path or inline description, and optional `--ticket <stem>`.
If ticket-driven: read the ticket. Extract scope, stem, and phase context.

Spawn `project-survey` with the ticket path or inline description. Capture the returned `[Must|Maybe]` reference list — it informs the brief's `## References` section.

### 2. Write brief

Write `ai-docs/plans/YYYY-MM/DD-<stem>.brief.md` using the **brief template** (see Templates).
Strip ticket noise — this file is the implementer's sole context source.
Populate `## References` from the project-survey output.

### 3. Plan depth

Apply `judge: plan-depth`. Default to survey when uncertain between as-is and survey.

**as-is** — proceed to step 4.

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

1. Verify skeleton: grep for `todo!()`/`unimplemented`/`NotImplementedError` stubs or check for integration tests referencing target contracts. Apply `judge: skeleton-check`. If skeleton required but absent, stop and suggest `/write-skeleton`.
2. Collect integration test context: identify test file paths and the run command. Flows into the implementer spawn prompt.
3. Register all agent slots:
   ```bash
   ws-new-named-agent implementer --model sonnet --system-prompt "$(ws-infra-path implementer.md)"
   ws-new-named-agent reviewer-correctness --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-correctness.md)"
   ws-new-named-agent reviewer-fit --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-fit.md)"
   ws-new-named-agent reviewer-test --agent ws:code-reviewer --system-prompt "$(ws-infra-path code-review-test.md)"
   ```
4. Allocate review path slots (separate Bash call); capture all output lines as literals:
   ```bash
   ws-review-path correctness fit test
   ```
   Store as `<correctness-path>`, `<fit-path>`, `<test-path>`.

### 5. Spawn implementer

Issue the Bash call with `run_in_background: true`. Read output after notification.

```bash
ws-call-named-agent implementer - <<'PROMPT'
Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>

Read only the brief (and plan if provided). Do not read the ticket directly.

Acceptance criteria: skeleton integration tests must pass.
- Test files: <integration test paths>
- Run: <command to execute them>

Ancestor loading: when you read `ai-docs/mental-model/<domain>/<sub>.md`,
read `ai-docs/mental-model/<domain>/index.md` first.

Instructions:
- Verify integration tests pass before reporting completion or after each fix.
- Report completion in plain text. Include test results.
- For fix cycles, a follow-up call will arrive with review findings — fix and report back.
- Commit at logical checkpoints on the current branch.
PROMPT
```

After notification:
```bash
ws-print-named-agent-output implementer
```

Note the commit range from the report.

### 6. Review

#### 6a. Partition allocation

Apply `judge: partition-allocation` based on the implementer's report and the nature of changes.

#### 6b. Spawn reviewers

Spawn one reviewer per selected partition in parallel (`run_in_background: true` each).
After all notifications, read each summary via `ws-print-named-agent-output <name>`.

```bash
ws-call-named-agent reviewer-correctness - <<'PROMPT'
Diff range: <first-commit>..<last-commit>

Instructions:
- Write your full findings to: <correctness-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
PROMPT
```

```bash
ws-call-named-agent reviewer-fit - <<'PROMPT'
Diff range: <first-commit>..<last-commit>
Brief path: <brief-path>

Instructions:
- Judge whether the implementation achieves what the brief intended and leaves room for future phases.
- You may reference the ticket at <ticket-path> for architectural headroom checks (optional).
- Write your full findings to: <fit-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
PROMPT
```

```bash
ws-call-named-agent reviewer-test - <<'PROMPT'
Diff range: <first-commit>..<last-commit>

Instructions:
- Write your full findings to: <test-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
PROMPT
```

#### 6c. Relay and loop

Track relay cycle count starting at 0. Maximum 3 relay cycles.

**Entry:** All `[clean]` → exit loop, proceed to cleanup.

**Relay** (`run_in_background: true`). Increment cycle counter before each relay.

```bash
ws-call-named-agent implementer - <<'PROMPT'
Review cycle <N>: <correctness-path>, <fit-path>, <test-path>. Read each file directly.
For each finding respond with a disposition: [fixed], [won't fix: <reason>], or [deferred: <reason>].
Won't-fix allowed: style suggestions conflicting with established codebase patterns; suggestions that expand scope beyond the brief.
Won't-fix not allowed: correctness, security, or contract violations — fix or escalate these.
PROMPT
```

After notification: `ws-print-named-agent-output implementer`. Extract the won't-fix list.

**Re-review** (parallel, same paths — reviewers overwrite):

```bash
ws-call-named-agent reviewer-correctness - <<'PROMPT'
Re-review. Updated diff: <diff>
Implementer won't-fix items: <list with reasons>
For each won't-fix item: respond [accepted] or [maintained: <brief reason>].
PROMPT
```

```bash
ws-call-named-agent reviewer-fit - <<'PROMPT'
Re-review. Updated diff: <diff>
Implementer won't-fix items: <list with reasons>
For each won't-fix item: respond [accepted] or [maintained: <brief reason>].
PROMPT
```

```bash
ws-call-named-agent reviewer-test - <<'PROMPT'
Re-review. Updated diff: <diff>
Implementer won't-fix items: <list with reasons>
For each won't-fix item: respond [accepted] or [maintained: <brief reason>].
PROMPT
```

After all notifications, read summaries.

**Branch on cycle and result:**

- All `[clean]` → exit loop, proceed to cleanup.
- Cycle ≤ 2 and non-clean → go to relay.
- Cycle = 2 and maintained items exist: lead reads review files directly. For each maintained dispute: accept the won't-fix or override it. If any overrides: relay override list to implementer (counts as cycle 3 relay). Otherwise advance to cycle 3 re-review directly.
- Cycle = 3 and non-clean remain: collect unresolved findings. Proceed to cleanup, surface escalation in output.

### 7. Cleanup

```bash
rm -f <correctness-path> <fit-path> <test-path>
```

Agent registry entries need no teardown — created fresh per run via `ws-new-named-agent`.

Output the **completion report** (see Templates).

## Judgments

### judge: plan-depth

Soft judgment. Default to survey when uncertain between as-is and survey.

| Signal | Suggests |
|--------|----------|
| Brief names concrete change points; single-file or single-function scope | as-is |
| Multi-module span; cold implementer; reuse points likely but unconfirmed | survey |
| Multiple viable strategies; non-obvious cross-module side effects | research |

### judge: partition-allocation

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
| Proceed without skeleton | Brief is a small isolated change (single file, no public contracts) |
| Require skeleton | Change touches public interfaces or cross-module boundaries |

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

## References
<!-- Populated from project-survey [Must/Maybe] output. -->
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/<path>` — <relevance>
```

### Completion report format

```
Implementation complete.
Commit range: <first>..<last>
Brief: <brief-path>
Test status: pass | fail | skipped
<if escalated:> Escalation: <list of unresolved disputes>
```

## Doctrine

Write-code optimizes for **brief-to-commit throughput within a branch** —
every step exists to move a target from intent (brief) to verified code
(commits) without the caller managing internal agent state. Self-cleanup
of review paths keeps the caller's context clean. When a rule is ambiguous,
apply whichever interpretation advances the commit without widening the
caller's coordination surface.
