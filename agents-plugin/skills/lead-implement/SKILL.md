---
name: lead-implement
description: Unified implementation spine. Routes to direct-edit or delegated implementation, reviews, closes docs, then gates merge or continuation.
---

# Implement

Target: user request

## Invariants

Scope
- Honor caller-provided scope or phase slices as hard implementation boundaries.
- Direct-edit escalates to delegated when scope becomes multi-file with new public API or cross-module new pattern.

Branch
- User approval gates merge or continuation.
- Merge commits follow CLAUDE.md commit rules and include `## AI Context`.

Execution
- Create the task list at Prep; every task is mandatory and ordered.
- Delegated: implementer reads only the brief and optional plan; never the ticket directly.
- Brief preserves every selected-scope binding decision; audit before commit.

Review
- Single Review stage for all paths; reviewer count from `judge: review-allocation`.
- Lead fixes correctness, security, contract, and regression findings; may reject style-only or scope-expanding findings with reasons.
- Delete review path files before Final Action Gate.

## On: invoke

### 1. Route

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, artifacts, and caller-provided slice.
3. Apply `judge: needs-delegation`.
4. Apply `judge: branch-mode`.
5. Apply `judge: plan-depth`.
6. Apply `judge: review-allocation`.

### 2. Prep

1. Record `<current-branch>`.
2. Outside `implement/*`, set `<merge-target>` to `<current-branch>`.
3. On `implement/*`, set `<merge-target>` from caller or confirm before execution.
4. If continuing on `implement/*` and branch name no longer matches selected scope, rename with `git branch -m implement/<scope>`; stop if target branch exists or upstream tracking is ambiguous.
5. Record `<implementation-start>` with `git rev-parse HEAD`.
6. If `branch-mode` = create: create `implement/<scope>` before any source edit.
7. Call `ws/mental_models.find(query: <target or domain>)` or `ws/mental_models.status(domain: <domain>)`; read returned docs, ancestors first.
8. Call `ws/infra.read(name: "impl-playbook")`.
9. Identify integration test paths and their run command.
10. If `plan-depth` ≥ survey: survey project via `ws/agents.register(name: "project-survey", prompts: ["project-survey"])` → `ws/agents.call`; capture `[Must|Maybe]` references.
11. If `plan-depth` ≥ brief: write brief at `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md` using **Brief template**; include survey references when available; audit against target; commit.
12. If `plan-depth` ≥ survey: run plan populator with **Plan prompts**; if survey returns `[escalate-to-research]`, re-run as research; if plan returns `[escalate-to-lead]`, stop and report blocker; commit plan.
13. If delegated: register implementer via `ws/agents.register(name: "implementer", prompts: ["implementer"])`.
14. Create and maintain task list:

```text
[ ] Route - delegation, plan depth, branch mode, review allocation
[ ] Prep - branch, context, brief/plan (per plan-depth)
[ ] Edit - direct-edit or spawn implementer; capture commit range
[ ] Review - reviewer relay per review-allocation
[ ] Doc pre-pass - update-spec then mental-model-updater; commit each
[ ] Doc commit gate - refresh _index.md, ticket status, then commit docs
[ ] Final action gate - wait for merge, continue, or stop
[ ] Merge - implementation-branch modes only and only when approved
```

### 3. Edit

1. If direct-edit: edit directly per target and impl-playbook; commit logical checkpoints.
2. If direct-edit: run tests/build; read full output before claiming pass; resolve introduced warnings per impl-playbook Verify; on failure, diagnose blame before fixing; re-run until pass or real blocker.
3. If delegated and referenced tests exist: run baseline verification.
4. If delegated: call `ws/agents.call(name: "implementer", prompt: ...)` with **Implementer spawn prompt**.
5. If delegated: read `ws/agents.result(name: "implementer", timeout_seconds: 600)` only if async result lacks usable summary; capture `<first-commit>..<last-commit>`.
6. Capture `<commit-range>` and `<result-commit>`.

### 4. Review

1. If lead-only: record rationale; skip to step 8.
2. If single: register reviewer via `ws/agents.register(name: "reviewer", prompts: ["code-reviewer", "code-review-correctness", "code-review-fit"])`; generate path via `ws/path.generate(kind: "review", stems: ["direct"])`.
3. If partitioned: choose partition subset from Tier 2; for each, register reviewer from **Reviewer partition table**; generate paths via `ws/path.generate(kind: "review", stems: ["correctness", "fit", "test"])`.
4. Call reviewer(s) with **Reviewer prompt frame**.
5. If all `[clean]`: skip to step 8.
6. If non-clean and single: read review path; classify findings (fix: correctness/security/contract/regression; reject: style-only or scope expansion); apply fixes; re-verify; re-call reviewer with rejected list. Repeat until `[clean]` or 2 cycles.
7. If non-clean and partitioned: relay to implementer with **Review relay prompt**; extract won't-fix list; re-review non-clean partitions with **Re-review prompt**; keep clean partitions accepted unless fix touched their surface. Repeat until all `[clean]` or 3 cycles; lead adjudicates at cycle 2; caller escalation at cycle 3.
8. Delete all review path files.

### 5. Doc Pre-Pass

1. Invoke `ws:lead-update-spec` with `<commit-range>`.
2. Call `ws/agents.register(name: "mental-model-updater", prompts: ["mental-model-updater"])`.
3. Call `ws/agents.call(name: "mental-model-updater", prompt: "Commit range: <commit-range>")`.
4. Wait for completion; commit file changes.

Run mental-model-updater after update-spec so it sees implemented-marker changes.

### 6. Doc Commit Gate

1. Call `ws/infra.read(name: "executor-wrapup")`.
2. Refresh `ai-docs/_index.md` for new skills, agents, or major patterns.
3. If ticket-driven, follow Ticket Update using `<result-commit>`.
4. Commit doc changes per executor-wrapup. Do not re-run Doc Pipeline.

### 7. Final Action Gate

Report:

- implemented changes from direct-edit or implementer output;
- documentation updates and ticket Result hash;
- review result;
- test status;
- deviations or open items;
- unresolved disputes from review relay, if any.

Wait for merge, continue, or stop. Direct-current mode exits here; no merge
stage. If the user wants more changes, route to a new implementation slice or
`ws:lead-sprint`; completed phases capture follow-up through append-only ticket
Result editions.

### 8. Merge

Implementation-branch modes only. Merge only when the user approves. Merge
`implement/<scope>` to `<merge-target>` with the repository merge helper or
equivalent non-interactive git sequence.

Use squash for one commit; use `--no-ff` for two or more commits.
Write the merge commit per CLAUDE.md.

## Judgments

### judge: needs-delegation

| Decision | When |
|----------|------|
| Direct-edit | Single file, internal-only, no callers affected, no new public symbols, no new test files, and no explicit delegation request |
| Delegated | Public interface, cross-module boundary, or new type contract changes |
| Delegated | Any direct-edit condition is unmet |

### judge: branch-mode

Pick the first matching decision.

| Decision | When |
|----------|------|
| Continue implementation branch | Current branch starts with `implement/` |
| Create implementation branch | Delegated path outside `implement/` |
| Direct current branch | Direct-edit path |

### judge: plan-depth

Default: `none` for direct-edit; `survey` for delegated when uncertain.

| Decision | When |
|----------|------|
| none | Narrow scope; clear change points; no cross-module risk |
| brief | Moderate complexity; benefit from self-anchoring scope record |
| survey | Multi-module span; cold context; reuse points unconfirmed |
| research | Multiple viable strategies; non-obvious cross-module side effects |

Levels are cumulative: `brief` writes the brief; `survey` writes brief then runs survey plan; `research` writes brief then runs research plan.

### judge: review-allocation

Tier 1 — depth:

| Decision | When |
|----------|------|
| Lead-only (0 reviewers) | Mechanical, low-risk edit; record rationale |
| Single reviewer | Direct-edit path; moderate complexity |
| Partitioned | Delegated path or cross-module/public interface changes |

Tier 2 — partitions (only when Tier 1 = Partitioned):

| Partition | When |
|-----------|------|
| Correctness | New logic, modified error paths, contract/security surface |
| Fit | Existing components reused/modified, new pattern, or ticket-driven decision preservation |
| Test | Tests added/modified, or new code paths lack coverage |

Choose the smallest partition set that covers material risk.
Full review (all three): reserve for cross-cutting behavior plus runtime/tooling plus test surface.

## Templates

### Brief template

Path: `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md`

```markdown
# Brief: <stem>

## Intent
<what this achieves - one paragraph>

## Scope Boundary
<selected scope and explicit deferred or excluded ticket scope>

## Caller-Visible Contract
<observable behavior, public API/protocol/UI/doc output/lifecycle contract>

## Contract Instructions
<files/modules; public types/functions/handlers/tools; visibility; call shape>
<existing mechanisms to reuse before adding new paths>
<temporary, fallback, or mock-data wiring that is forbidden>

## Integration Test Instructions
<boundary type; existing test to extend or new test file; pass criteria>

## Implementation Strategy Decisions
<settled approach the implementer must not reopen>

## Rejected Alternatives
<approaches ruled out and why>

## Approach
<macro-level how - bullets>

## Constraints
<must-hold conditions>

## Out of scope
<explicitly excluded>

## Details
<interface specs, data types, public contracts at ticket-level resolution>

## Verification Contract
<tests, probes, command outputs required for acceptance>

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/<path>` - <relevance>
```

### Plan prompts

**Survey:**
```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
```

**Research:**
```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
```

**Research route** (when survey escalates):
```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
The existing plan file contains survey output that requested research.
Read it, then replace the file with a research plan.
```

### Implementer spawn prompt

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
- Respect plan risk signals; escalate instead of implementing a known wrong contract.
- Report completion in plain text. Include test results.
- For fix cycles, a follow-up call will arrive with review findings; fix and report back.
- Commit logical checkpoints on the current branch.
```

### Reviewer partition table

| Partition | Reviewer name | Prompts | Required check |
|-----------|---------------|---------|----------------|
| Correctness | `reviewer-correctness` | `["code-reviewer", "code-review-correctness"]` | Verify correctness invariants. |
| Fit | `reviewer-fit` | `["code-reviewer", "code-review-fit"]` | Verify brief contract, future-phase fit, ticket-driven decisions. |
| Test | `reviewer-test` | `["code-reviewer", "code-review-test"]` | Verify coverage, assertions, integration-test instructions. |

### Reviewer prompt frame

```text
Review partition: <Correctness|Fit|Test>
Diff range: <first-commit>..<last-commit>
<if Fit:> Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>
<if Fit and ticket-driven:> Ticket path: <ticket-path>
Findings path: <partition-output-path>

Review focus:
- <2-4 partition-specific risks>

Required checks:
- <required check from partition table>
- <if plan exists:> Verify plan guardrails were not bypassed.
- <if Fit and ticket-driven:> Report any binding decision omitted or violated.

Instructions:
- Ignore outside this partition unless directly broken by the diff.
- Write full findings to the findings path.
- Return only: [clean|non-clean]: <one-line summary>
```

### Review relay prompt

```text
Review cycle <N>: <non-clean review paths only>. Read each file directly.
For each finding: [fixed], [won't fix: <reason>], or [deferred: <reason>].
Won't-fix allowed: style suggestions conflicting with codebase patterns; scope expansion beyond brief.
Won't-fix not allowed: correctness, security, or contract violations.
```

### Re-review prompt

```text
Re-review. Updated diff: <diff>
Won't-fix items: <list with reasons>
For each won't-fix item: respond [accepted] or [maintained: <brief reason>].
```

## Doctrine

Implement optimizes for **verified code reaching the target branch**. Route
decides delegation, plan depth, and branch mode upfront; subsequent stages
execute per those decisions without re-routing. Code quality lives in the Edit
stage (direct or implementer); review quality lives in the unified Review stage.
When ambiguous, keep routing out of execution and execution out of routing.
