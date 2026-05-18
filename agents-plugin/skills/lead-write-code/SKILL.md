---
name: lead-write-code
description: Delegated implementation primitive used by lead-implement for broader changes; writes a brief, runs implementer/reviewer relay, cleanup, and completion report.
---

# Write Code

Target: user request

## Invariants

- Operate on current branch; caller owns branch creation.
- Invoke `ws:lead-workflow-manual` first when workflow primitives are not already in context.
- Implementer reads only the brief, plus plan when provided; never the ticket directly.
- Brief strips ticket noise, never selected-scope binding decisions.
- Brief includes concrete contract and integration-test instructions for public or cross-module changes.
- Fit reviewer reads the ticket when ticket-driven; correctness/test reviewers may not.
- Honor caller-provided scope or phase slices as hard implementation boundaries.
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
2. If ticket-driven: read ticket; extract scope, stem, phase context, and caller-provided boundary.
3. Survey project:

```text
ws/agents.register(name: "project-survey", prompts: ["project-survey"])
ws/agents.call(name: "project-survey", prompt: <ticket path or inline description>)
```

4. Capture `[Must|Maybe]` references for brief `## References`.

### 2. Write Brief

1. Write `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md` with the brief template.
2. Strip ticket noise; preserve every selected-scope binding decision in the brief.
3. Populate `## References` from project-survey output.
4. Populate `## Contract Instructions` with concrete files/modules, public surface, call shapes, boundaries, reuse targets, and forbidden temporary wiring.
5. Populate `## Integration Test Instructions` with the required boundary type, test location strategy, and pass criteria.
6. Audit the brief against the target; every settled caller-visible contract, implementation strategy decision, rejected alternative, and verification expectation must appear in the brief or be explicitly out of scope/deferred.
7. Commit the brief before plan-depth.

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

1. Identify integration test paths and verification command from the brief's integration-test instructions.
2. Run baseline verification only when the referenced tests or command already exist.
3. Register implementer:
   `ws/agents.register(name: "implementer", prompts: ["implementer"])`.
4. Generate review paths:
   `ws/path.generate(kind: "review", stems: ["correctness", "fit", "test"])`.
5. Store `<correctness-path>`, `<fit-path>`, `<test-path>`.

### 5. Spawn Implementer

Call `ws/agents.call(name: "implementer", prompt: <block below>)`.
Read `ws/agents.result(name: "implementer", timeout_seconds: 600)` only if async result lacks usable summary.

```text
Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>

Read only the brief (and plan if provided). Do not read the ticket directly.
Implement only the brief's scope boundary; leave later ticket phases untouched.

Acceptance criteria:
- Brief `## Contract Instructions` must be implemented or explicitly escalated.
- Brief `## Integration Test Instructions` must be satisfied.
- Test files: <integration test paths>
- Run: <command to execute them>

Ancestor loading: when you read `ai-docs/mental-model/<domain>/<sub>.md`,
read `ai-docs/mental-model/<domain>/index.md` first.

Instructions:
- Verify integration tests pass before reporting completion and after each fix.
- Do not replace brief contract instructions with temporary, fallback, or mock-data behavior.
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

For each selected partition, call `ws/agents.register(name: <Reviewer name>, prompts: <Prompts>)` from the table:

| Partition | Reviewer name | Prompts | Output path |
|-----------|---------------|---------|-------------|
| Correctness | `reviewer-correctness` | `["code-reviewer", "code-review-correctness"]` | `<correctness-path>` |
| Fit | `reviewer-fit` | `["code-reviewer", "code-review-fit"]` | `<fit-path>` |
| Test | `reviewer-test` | `["code-reviewer", "code-review-test"]` | `<test-path>` |

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
- Judge whether the implementation satisfies brief contract instructions and leaves room for future phases.
<if ticket-driven:> Read the ticket at <ticket-path>; report any selected-scope binding decision omitted from the brief or violated by the implementation.
- Write your full findings to: <fit-path>
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
```

Test:

```text
Diff range: <first-commit>..<last-commit>

Instructions:
- Review focus: <2-4 coverage or assertion risks to verify, including brief integration-test instructions>.
- Judge whether required integration tests exist and prove the specified boundary.
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
| Fit | Existing components reused/modified, new pattern others will follow, or ticket-driven decision preservation must be checked |
| Test | Tests added/modified, or new code paths lack existing coverage |
| Correctness + Test | Executable behavior changed and coverage is material |
| Correctness + Fit | Workflow/API semantics changed without a meaningful test surface |
| Full | Cross-cutting behavior plus runtime/tooling plus test surface, or release/security/data-loss boundary |
| Floor | Pure mechanical change -> lead-only or one reviewer with rationale |

## Templates

### Brief format

Path: `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md`

```markdown
# Brief: <stem>

## Intent
<what this achieves - one paragraph>

## Scope Boundary
<selected slice and explicit deferred or excluded ticket scope>

## Caller-Visible Contract
<observable behavior, public API/protocol/UI/doc output/lifecycle contract>
<write "None beyond existing behavior" only when the target is internal-only>

## Contract Instructions
<files/modules that must change; public types/functions/handlers/tools; visibility; call shape; input/output shape; lifecycle boundaries>
<existing mechanisms to reuse before adding new paths>
<temporary, fallback, or mock-data wiring that is forbidden>
<write "None beyond Caller-Visible Contract" only when no public or cross-module contract is introduced>

## Integration Test Instructions
<required boundary type: parser, CLI, MCP tool, doc convention, skill routing, runtime lifecycle, agent relay, or other>
<existing test to extend or new integration test file to create>
<assertions or observable pass criteria that prove the contract works>
<write "Existing verification only" only when no new integration boundary is introduced>

## Implementation Strategy Decisions
<settled approach, optimization, reuse, abstraction, or boundary choices the implementer must not reopen>

## Rejected Alternatives
<approaches already ruled out and why; omit only when none are settled>

## Approach
<macro-level how - bullets>

## Constraints
<must-hold conditions>

## Out of scope
<explicitly excluded from this implementation>

## Details
<interface specs, data types, public contracts at ticket-level resolution>
<supporting detail that does not fit Contract Instructions>

## Verification Contract
<tests, probes, screenshots, command outputs, compatibility checks, or review gates required for acceptance>

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

Write-code optimizes for **brief-to-commit fidelity within a branch**. The
brief isolates executable contract, test, and implementation decisions;
persistent agents carry implementation and review state; file paths keep
findings out of lead context; cleanup closes the loop. When a rule is
ambiguous, preserve selected-scope decisions without widening the implementer's
coordination surface.
