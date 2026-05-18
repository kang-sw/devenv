---
name: lead-write-code
description: Delegated implementation primitive used by lead-implement for broader changes; writes a brief, runs implementer/reviewer relay, cleanup, and completion report.
---

# Write Code

Target: user request

## Invariants

Branch
- Operate on current branch; caller owns branch creation.

Context
- Invoke `ws:lead-workflow-manual` first when workflow primitives are not already in context.
- Honor caller-provided scope or phase slices as hard implementation boundaries.

Brief
- Implementer reads only the brief, plus plan when provided; never the ticket directly.
- Brief strips ticket noise, never selected-scope binding decisions.
- Brief includes concrete contract and integration-test instructions for public or cross-module changes.

Review
- Fit reviewer reads the ticket when ticket-driven; correctness/test reviewers may not.
- Reviewers write findings to files and return summaries only.
- Lead relays review file paths, not findings content, to implementer.
- Relay cap is 3 cycles; lead adjudicates at cycle 2; caller escalation at cycle 3.
- Delete review path files before returning.

Agents
- Lead puts ancestor-loading rule in implementer prompt.
- Implementer and reviewer sessions persist through `ws/agents.call` auto-resume.

Output
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

1. Apply `judge: plan-depth`.
2. If `as-is`: continue to Prepare.
3. If `survey`: register/call `plan-surveyor` with `prompts: ["plan-populator-survey"]`.
4. If `research`: register/call `plan-researcher` with `prompts: ["plan-populator-research"]`.
5. For `survey` or `research`, use **Plan population prompt**.
6. Commit the plan file before Prepare.

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

Capture `<first-commit>..<last-commit>` from implementer output.

### 6. Review

Run **Review Relay**.

### 7. Cleanup

```text
rm -f <correctness-path> <fit-path> <test-path>
```

Agent registry entries need no teardown; `ws/agents.register` creates fresh task slots per run.
Output completion report.

## On: Review Relay

1. Run **Allocate Review**.
2. Run **Spawn Reviewers**.
3. Run **Relay Loop**.

## On: Allocate Review

1. Apply `judge: partition-allocation` from implementer report and changed files.
2. Choose the smallest reviewer set that covers material risk.
3. Record skipped partitions with one-line rationale.
4. Prepare 2-4 review focus bullets for each selected partition.

## On: Spawn Reviewers

1. For each selected partition, register the reviewer from **Reviewer partition table**.
2. Call selected reviewers in parallel with `ws/agents.call`.
3. Use **Reviewer prompt frame** for every reviewer.
4. Read `ws/agents.result(name: "<reviewer-name>", timeout_seconds: 600)` only if needed.

## On: Relay Loop

1. Start relay cycle count at 0.
2. If all reviewers return `[clean]`, exit.
3. If any reviewer returns `[non-clean]`, increment cycle before relay.
4. Call implementer with **Review relay prompt**.
5. After implementer returns, extract won't-fix list.
6. Re-review only partitions that returned `[non-clean]`.
7. Keep clean partitions accepted unless the fix commit touched their owned surface.
8. Call reviewers with **Re-review prompt**; reviewers overwrite their own files.
9. If all reviewers return `[clean]`, exit.
10. If cycle < 2 and non-clean remains, repeat relay.
11. If cycle = 2 and non-clean remains, lead reads review files; accept won't-fix or override.
12. If lead overrides, run one final cycle 3 relay.
13. If cycle = 3 and non-clean remains, collect unresolved findings and continue to cleanup.

## Judgments

### judge: plan-depth

Default: `survey` when uncertain between `as-is` and `survey`.
Output: `as-is`, `survey`, or `research`.

| Signal | Suggests |
|--------|----------|
| Concrete change points; single-file/function scope | as-is |
| Multi-module span; cold implementer; likely reuse points unconfirmed | survey |
| Multiple viable strategies; non-obvious cross-module side effects | research |

### judge: partition-allocation

Goal: choose the smallest partition set that covers material risk.
Uncertain: add one secondary partition rather than defaulting to all three.
Full review: reserve for risks spanning correctness, fit, and tests.

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

### Brief Template

Path: `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md`

```markdown
# Brief: <stem>

## Intent
<what this achieves - one paragraph>

## Scope Boundary
<selected scope and explicit deferred or excluded ticket scope>

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

### Plan Template

#### Plan population prompt

```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
```

### Review Templates

#### Reviewer partition table

| Partition | Reviewer name | Prompts | Output path | Required check |
|-----------|---------------|---------|-------------|----------------|
| Correctness | `reviewer-correctness` | `["code-reviewer", "code-review-correctness"]` | `<correctness-path>` | Verify correctness invariants. |
| Fit | `reviewer-fit` | `["code-reviewer", "code-review-fit"]` | `<fit-path>` | Verify brief contract instructions, future-phase fit, and ticket-driven binding decisions. |
| Test | `reviewer-test` | `["code-reviewer", "code-review-test"]` | `<test-path>` | Verify coverage, assertions, and brief integration-test instructions. |

#### Reviewer prompt frame

```text
Review partition: <Correctness|Fit|Test>
Diff range: <first-commit>..<last-commit>
<if Fit:> Brief path: <brief-path>
<if Fit and ticket-driven:> Ticket path: <ticket-path>
Findings path: <partition-output-path>

Review focus:
- <2-4 partition-specific risks>

Required checks:
- <required check from Reviewer partition table>
- <if Fit and ticket-driven:> Report any selected-scope binding decision omitted from the brief or violated by the implementation.

Instructions:
- Ignore outside this partition unless directly broken by the diff.
- Write full findings to the findings path.
- Return only: [clean|non-clean]: <one-line summary of most significant issues>
```

#### Review relay prompt

```text
Review cycle <N>: <non-clean review paths only>. Read each file directly.
For each finding respond with a disposition: [fixed], [won't fix: <reason>], or [deferred: <reason>].
Won't-fix allowed: style suggestions conflicting with established codebase patterns; suggestions that expand scope beyond the brief.
Won't-fix not allowed: correctness, security, or contract violations - fix or escalate these.
```

#### Re-review prompt

```text
Re-review. Updated diff: <diff>
Implementer won't-fix items: <list with reasons>
For each won't-fix item: respond [accepted] or [maintained: <brief reason>].
```

### Report Template

#### Completion report format

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
