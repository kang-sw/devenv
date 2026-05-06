---
name: lead-write-code
description: Delegate a ticket or inline implementation target through a brief, optional plan, Codex implementer session, partitioned reviewer fanout, bounded relay loop, cleanup, and completion report.
---

# Write Code

Target: user request

## Invariants

- Operate on current branch; caller owns branch creation.
- Implementer reads only the brief, plus plan when provided; never the ticket directly.
- Fit reviewer may read the ticket for architectural headroom; correctness/test reviewers may not.
- Existing skeleton stubs and integration tests are acceptance criteria.
- Lead puts ancestor-loading rule in implementer prompt.
- Reviewers write findings to files and return summaries only.
- Lead relays review file paths, not findings content, to implementer.
- Implementer and reviewer sessions persist through `ws/agents.call` auto-resume.
- Relay cap is 3 cycles; lead adjudicates at cycle 2; caller escalation at cycle 3.
- Delete review path files before returning.
- Output commit range, test status, and brief path in completion format.

## On: invoke

### 1. Read Target

1. Parse ticket path or inline description; accept optional `--ticket <stem>`.
2. If ticket-driven: read ticket; extract scope, stem, phase context.
3. Survey project:

```text
ws/agents.register(name: "project-survey", prompts: ["project-survey"])
ws/agents.call(name: "project-survey", prompt: <ticket path or inline description>)
```

4. Capture `[Must|Maybe]` references for brief `## References`.

### 2. Write Brief

1. Write `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md` with the brief template.
2. Strip ticket noise; the brief is the implementer's sole context source.
3. Populate `## References` from project-survey output.
4. Commit the brief before plan-depth.

### 3. Plan Depth

Apply `judge: plan-depth`; default to survey when uncertain between as-is and survey.

- **as-is:** continue to Prepare.
- **survey:** register/call `plan-surveyor` with `prompts: ["plan-populator-survey"]`.
- **research:** register/call `plan-researcher` with `prompts: ["plan-populator-research"]`.

Population prompt:

```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
```

Commit the plan file before Prepare.

### 4. Prepare

1. Verify skeleton by frontmatter, integration tests, or stubs: `todo!()`, `unimplemented`, `NotImplementedError`.
2. Apply `judge: skeleton-check`; stop and suggest `ws:lead-write-skeleton` if required skeleton is absent.
3. Identify integration test paths and run command.
4. Register implementer:
   `ws/agents.register(name: "implementer", prompts: ["implementer"])`.
5. Register selected reviewers:
   `ws/agents.register(name: "<reviewer-name>", prompts: ["code-reviewer", "<partition-prompt>"])`.
6. Generate review paths:
   `ws/path.generate(kind: "review", stems: ["correctness", "fit", "test"])`.
7. Store `<correctness-path>`, `<fit-path>`, `<test-path>`.

### 5. Spawn Implementer

Call `ws/agents.call(name: "implementer", prompt: <block below>)`.
Read `ws/agents.result(name: "implementer", timeout_seconds: 600)` only if async result lacks usable summary.

```text
Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>

Read only the brief (and plan if provided). Do not read the ticket directly.

Acceptance criteria: skeleton integration tests must pass.
- Test files: <integration test paths>
- Run: <command to execute them>

Ancestor loading: when you read `ai-docs/mental-model/<domain>/<sub>.md`,
read `ai-docs/mental-model/<domain>/index.md` first.

Instructions:
- Verify integration tests pass before reporting completion and after each fix.
- Report completion in plain text. Include test results.
- For fix cycles, a follow-up call will arrive with review findings; fix and report back.
- Commit logical checkpoints on the current branch.
```

Capture commit range from implementer output.

### 6. Review

#### 6a. Allocate

Apply `judge: partition-allocation` from implementer report and changed files.
Choose the smallest reviewer set that covers material risk.
Record skipped partitions with one-line rationale.
Prepare 2-4 review focus bullets for each selected partition.

#### 6b. Spawn Reviewers

Call selected reviewers in parallel with `ws/agents.call`.
Read `ws/agents.result(name: "<reviewer-name>", timeout_seconds: 600)` only if needed.

Correctness:

```text
Diff range: <first-commit>..<last-commit>

Instructions:
- Review focus: <2-4 correctness invariants to verify>.
- Ignore outside this partition unless directly broken by the diff.
- Write your full findings to: <correctness-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
```

Fit:

```text
Diff range: <first-commit>..<last-commit>
Brief path: <brief-path>

Instructions:
- Review focus: <2-4 fit or architecture concerns to verify>.
- Ignore outside this partition unless directly broken by the diff.
- Judge whether the implementation achieves the brief and leaves room for future phases.
- You may reference the ticket at <ticket-path> for architectural headroom checks (optional).
- Write your full findings to: <fit-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
```

Test:

```text
Diff range: <first-commit>..<last-commit>

Instructions:
- Review focus: <2-4 coverage or assertion risks to verify>.
- Ignore outside this partition unless directly broken by the diff.
- Write your full findings to: <test-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
```

#### 6c. Relay Loop

Start relay cycle count at 0.

- All `[clean]`: exit loop.
- Any `[non-clean]`: increment cycle before relay.

Relay prompt:

```text
Review cycle <N>: <non-clean review paths only>. Read each file directly.
For each finding respond with a disposition: [fixed], [won't fix: <reason>], or [deferred: <reason>].
Won't-fix allowed: style suggestions conflicting with established codebase patterns; suggestions that expand scope beyond the brief.
Won't-fix not allowed: correctness, security, or contract violations - fix or escalate these.
```

After implementer returns, extract won't-fix list and re-review only partitions that returned `[non-clean]`.
Clean partitions remain accepted unless the fix commit touched their owned surface.
Reviewers overwrite their own files.

Re-review prompt:

```text
Re-review. Updated diff: <diff>
Implementer won't-fix items: <list with reasons>
For each won't-fix item: respond [accepted] or [maintained: <brief reason>].
```

Branch:

- All `[clean]`: exit loop.
- Cycle <= 2 and non-clean: relay again.
- Cycle = 2 and maintained items exist: lead reads review files; accept won't-fix or override.
- Overrides count as cycle 3 relay.
- Cycle = 3 and non-clean remains: collect unresolved findings and continue to cleanup.

### 7. Cleanup

```text
rm -f <correctness-path> <fit-path> <test-path>
```

Agent registry entries need no teardown; `ws/agents.register` creates fresh task slots per run.
Output completion report.

## Judgments

### judge: plan-depth

Default to survey when uncertain between as-is and survey.

| Signal | Suggests |
|--------|----------|
| Concrete change points; single-file/function scope | as-is |
| Multi-module span; cold implementer; likely reuse points unconfirmed | survey |
| Multiple viable strategies; non-obvious cross-module side effects | research |

### judge: partition-allocation

Soft judgment. Prefer the smallest partition set that covers material risk.
When uncertain, add one secondary partition rather than defaulting to all three.
Full review is reserved for risks spanning correctness, fit, and tests.

| Partition | Assign when |
|-----------|-------------|
| Correctness | New logic, modified error paths, contract/security surface |
| Fit | Existing components reused/modified, or new pattern others will follow |
| Test | Tests added/modified, or new code paths lack existing coverage |
| Correctness + Test | Executable behavior changed and coverage is material |
| Correctness + Fit | Workflow/API semantics changed without a meaningful test surface |
| Full | Cross-cutting behavior plus runtime/tooling plus test surface, or release/security/data-loss boundary |
| Floor | Pure mechanical change -> lead-only or one reviewer with rationale |

### judge: skeleton-check

| Decision | When |
|----------|------|
| Proceed without skeleton | Small isolated change: single file, no public contracts |
| Require skeleton | Public interface or cross-module boundary changes |

## Templates

### Brief format

Path: `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md`

```markdown
# Brief: <stem>

## Intent
<what this achieves - one paragraph>

## Approach
<macro-level how - bullets>

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
- `ai-docs/mental-model/<path>` - <relevance>
```

### Completion report format

```text
Implementation complete.
Commit range: <first>..<last>
Brief: <brief-path>
Test status: pass | fail | skipped
<if escalated:> Escalation: <list of unresolved disputes>
```

## Doctrine

Write-code optimizes for **brief-to-commit throughput within a branch**. The
brief isolates intent, persistent agents carry implementation and review state,
file paths keep findings out of lead context, and cleanup closes the loop. When
a rule is ambiguous, apply whichever interpretation advances the commit without
widening the caller's coordination surface.
