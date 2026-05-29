---
name: lead-implement
description: Use when an approved task or ready ticket should be executed in wsflow; routes plan depth and review, runs lead-owned edits with optional scoped subagents, renders delegate prompts to native subagents, closes docs, then gates merge or continuation.
---

# Implement

Target: user request

## Invariants

Scope
- Lead-owned spine: route, edit, review, run the doc pipeline, report, and gate continuation.
- Honor caller-provided scope or phase slices as hard implementation boundaries.
- Edit is lead-owned: the lead edits directly and may dispatch bounded scoped native subagents; there is no separate implementer stage.
- Direct lead edits escalate to a scoped subagent when scope grows beyond what one lead context tracks cleanly.
- Invoke only the workflow skills named in this file.

Dispatch
- Delegate prompts reach native subagents only through `wsflow/prompt.render`; never hand-paste playbook prompt text into a subagent.
- Render-eligible prompts are `project-survey`, `plan-populator-survey`, `plan-populator-research`, `code-reviewer`, and `mental-model-updater`.
- File-writing prompts receive a caller-created output path in render `context`; free-response prompts return their result as subagent text.

Branch
- After Final Action Gate, wait for user approval before merging or starting another implementation slice.
- Merge commits follow repository commit rules and include `## AI Context`.

Execution
- Emit a user-facing Implementation Verdict after Route and before Prep as a non-blocking route summary.
- Create the task list during Prep; every task is mandatory and ordered.
- Commit logical units per repository commit rules with `## AI Context`.
- `wsflow:lead-update-spec` is invoked inline as a doc sub-step and returns control here; it is not a `NEXT:` handoff.

Review
- Single Review stage; reviewer allocation comes from `judge: review-allocation`.
- Lead fixes correctness, security, contract, and regression findings; lead may reject style-only or scope-expanding findings with reasons.

## On: invoke

### 1. Route

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, artifacts, and caller-provided slice.
3. Record `<current-branch>`.
4. Apply `judge: branch-mode` to `<current-branch>`.
5. Apply `judge: plan-depth`.
6. Apply `judge: review-allocation`.

### 2. Emit Implementation Verdict

1. Emit **Implementation Verdict** using the template.
2. State only observable routing inputs and final judgment labels.
3. Do not use `NEXT:`; `wsflow:lead-implement` is starting implementation, not routing to a sibling workflow skill.
4. Do not include chain-of-thought, alternatives considered, or private scoring.
5. Continue immediately to Prep after emitting the template.

### 3. Prep

1. Reuse `<current-branch>` recorded during Route.
2. Outside `implement/*`, set `<merge-target>` to `<current-branch>`.
3. On `implement/*`, set `<merge-target>` from caller or confirm before execution.
4. If continuing on `implement/*` and branch name no longer matches selected scope, rename with `git branch -m implement/<scope>`; stop if the target branch exists or upstream tracking is ambiguous.
5. Record `<implementation-start>` with `git rev-parse HEAD`.
6. If `branch-mode` = create: create `implement/<scope>` before any source edit.
7. Call `wsflow/mental_models.find(query: <target or domain>)` or `wsflow/mental_models.status(domain: <domain>)`; read returned docs, ancestors first.
8. Call `wsflow/infra.read(name: "impl-playbook")`.
9. Identify integration test paths and their run command.
10. If `plan-depth` >= survey: dispatch `project-survey` through **Render dispatch**; capture `[Must|Maybe]` references from the subagent's returned text.
11. If `plan-depth` >= brief: write the brief at `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md` using **Brief template**; transcribe any captured survey references under `## References`, tagging each `[Must]` or `[Maybe]`; audit against the target; commit.
12. If `plan-depth` >= survey: generate a plan path with `wsflow/path.generate(kind: "plan", stems: ["<stem>"])`. At `survey`, dispatch `plan-populator-survey` through **Render dispatch** with the brief path and plan path in `context`; if it returns `[escalate-to-research]`, dispatch `plan-populator-research` through **Render dispatch** with the same brief path and plan path. At `research`, dispatch `plan-populator-research` directly through **Render dispatch** with the brief path and plan path. If a populator returns `[escalate-to-lead]`, stop and report the blocker; otherwise commit the plan.

Create and maintain this task list:

```text
[ ] Route - plan depth, branch mode, review allocation
[ ] Prep - branch, context, brief/plan (per plan-depth)
[ ] Edit - lead edits + optional scoped subagent; verify; capture commit range
[ ] Review - run reviewer dispatch according to `judge: review-allocation`
[ ] Doc pre-pass - invoke `wsflow:lead-update-spec`; update mental models
[ ] Doc commit gate - refresh ai-docs/_index.md, ticket Result, then commit docs
[ ] Doc closeout compaction - compact safe documentation-only branch-tip suffix
[ ] Final action gate - wait for merge, continue, or stop
[ ] Merge - only when approved
```

### 4. Edit

1. Apply edits directly per target, brief or plan, and impl-playbook; commit logical checkpoints on the current branch.
2. For bounded subagent-worthy work, dispatch a scoped native subagent: state scope, writable paths or modules, verification expectations, and the required changed-file summary; integrate its result and keep commits lead-owned.
3. Run tests/build; read full output before claiming pass; resolve introduced warnings per impl-playbook Verify; on failure, diagnose blame before fixing; re-run until pass or a real blocker.
4. Capture `<commit-range>` as `<implementation-start>..HEAD` and `<result-commit>` as the current source HEAD.

### 5. Review

1. If lead-only: record the rationale and skip to Doc Pre-Pass.
2. Otherwise choose partitions from `judge: review-allocation`; for each partition dispatch `code-reviewer` through **Render dispatch** with the **Reviewer context** fields injected.
3. Read each reviewer's returned findings; classify them (fix: correctness/security/contract/regression; reject: style-only or scope expansion).
4. Apply fixes directly, re-verify, and re-dispatch the non-clean partitions. Repeat until all partitions are clean or 2 cycles complete; report remaining issues after the cap.
5. Summarize review outcomes and disputes for the final report.

### 6. Doc Pre-Pass

1. Invoke `wsflow:lead-update-spec` with `<commit-range>`.
2. Update relevant mental-model docs directly when the source diff changed module contracts, coupling, extension points, common mistakes, or technical debt; for a large update, dispatch `mental-model-updater` through **Render dispatch** with the target mental-model output path in `context`.
3. Commit mental-model changes separately and mention the affected docs in `## AI Context`.

Run mental-model updates after update-spec so they see implemented-marker changes.

### 7. Doc Commit Gate

1. Call `wsflow/infra.read(name: "executor-wrapup")`.
2. Refresh `ai-docs/_index.md` for new skills, package surfaces, focus changes, or major patterns.
3. If ticket-driven, add a `### Result (<result-commit>) - YYYY-MM-DD` section to the completed phase.
4. Commit ticket and index changes through `wsflow/git.commit`. Do not re-run the doc pipeline.

### 8. Doc Closeout Compaction

1. Run this gate for every supported branch mode; implementation runs are always on scoped implementation branches.
2. Inspect commits from `HEAD` backward after Doc Commit Gate; build only the contiguous branch-tip suffix of eligible documentation closeout commits.
3. An eligible commit is non-merge, workflow-owned, and changes only `ai-docs/spec/`, `ai-docs/mental-model/`, `ai-docs/tickets/`, `ai-docs/_index.md`, or narrowly relevant `ai-docs/ref/` workflow docs.
4. Stop suffix collection at the first ineligible commit; never cross source, test, skill, runtime, generated, planning, ready-promotion, review-fix, merge, or ambiguous-authorship commits.
5. If the suffix has fewer than two eligible commits, record `skipped - fewer than two eligible closeout commits`.
6. Compact the suffix into one closeout commit only when metadata synthesis is unambiguous; preserve AI Context, ticket Result references, Updated Tickets, Updated Specs, Mental Model Notes, and doc-audit rationale from absorbed commits.
7. After compaction, verify the final tree matches the pre-compaction head; if equivalence cannot be proven, restore the pre-compaction head and record compaction as skipped with the blocker.

### 9. Final Action Gate

Report:

- implemented changes from lead edits and any scoped subagent output;
- documentation updates and ticket Result hash;
- doc closeout compaction status;
- review result;
- test status;
- deviations or open items.

Stop after reporting and wait for the user to choose merge, continue with a new
slice, or stop. If the user wants more changes, route to a new implementation
slice. Completed ticket Results are append-only; later changes add a new Result
or Edition section instead of editing prior Result text. If the user chooses
stop, leave the implementation branch unmerged and report the branch name plus
merge target.

### 10. Merge

Merge only when the user approves. Merge `implement/<scope>` to
`<merge-target>` with an equivalent non-interactive git sequence.

Use fast-forward for a single workflow-owned, message-clean commit. For multiple
commits, use `--no-ff` by default; fast-forward only when each commit is an
independent deployable and independently revertible target-history unit. Use
squash when the branch is one logical change with noisy or dependent commits.

## Judgments

### judge: branch-mode

| Decision | When |
|----------|------|
| Continue implementation branch | Current branch starts with `implement/` |
| Create implementation branch | Current branch does not start with `implement/` |

### judge: plan-depth

Default: `none` for a narrow lead edit; `survey` when survey or plan evidence is uncertain.

| Decision | When |
|----------|------|
| none | Narrow scope; clear change points; no cross-module risk |
| brief | Moderate complexity; benefits from a self-anchoring scope record |
| survey | Multi-module span; cold context; reuse points unconfirmed |
| research | Multiple viable strategies; non-obvious cross-module side effects |

Levels are cumulative: `brief` writes the brief; `survey` runs `project-survey`, writes the brief, then runs the survey plan-populator; `research` runs `project-survey`, writes the brief, then runs the research plan-populator directly.

### judge: review-allocation

Tier 1 - depth:

| Decision | When |
|----------|------|
| Lead-only (0 reviewers) | Mechanical, low-risk edit; record rationale |
| Single reviewer | Moderate complexity |
| Partitioned | Cross-module or public interface changes |

Tier 2 - partitions (only when Tier 1 = Partitioned):

| Partition | When |
|-----------|------|
| Correctness | New logic, modified error paths, contract/security surface |
| Fit | Existing components reused/modified, new pattern, or ticket-driven decision preservation |
| Test | Tests added/modified, or new code paths lack coverage |

Choose the smallest partition set that covers material risk. Reserve all three for cross-cutting behavior plus runtime/tooling plus test surface.

## Templates

### Implementation Verdict

```text
## Implementation Verdict

- **Target**: <ticket path/stem or inline target>
- **Branch Mode**: <continue implementation branch | create implementation branch>
- **Plan Depth**: <none | brief | survey | research>
- **Review Allocation**: <lead-only | single reviewer | partitioned: correctness[, fit][, test]>
- **Scope**: <selected phase, whole target, or caller-provided slice>
- **Reason**: <decisive route facts only>

Proceeding with implementation.
```

### Render dispatch

1. Call `wsflow/prompt.render(stem: "<prompt-stem>", context: { <key>: <value> })`; capture `prompt_path`. Choose `context` keys per prompt: `project-survey` gets the target or domain; `code-reviewer` gets the **Reviewer context** fields; `plan-populator-survey` and `plan-populator-research` get the brief path and plan output path; `mental-model-updater` gets the target mental-model output path.
2. Spawn a native host subagent whose only instruction is to read `prompt_path` as its full task and to stay within the stated scope.
3. For file-writing prompts (`plan-populator-survey`, `plan-populator-research`, `mental-model-updater`): the subagent writes to the caller-created output path passed in `context`, or returns the content for the lead to save when it cannot write.
4. For free-response prompts (`project-survey`, `code-reviewer`): integrate the subagent's returned text directly.

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
<settled approach that must not be reopened>

## Rejected Alternatives
<approaches ruled out and why>

## Constraints
<must-hold conditions>

## Out of scope
<explicitly excluded>

## Verification Contract
<tests, probes, command outputs required for acceptance>

## References
<!-- [Must] entries: read before starting. [Maybe] entries: consult if uncertain. -->
- `ai-docs/mental-model/<path>` - <relevance>
```

### Reviewer context

```text
Partition: <Correctness|Fit|Test>
Diff range: <implementation-start>..HEAD
<if Fit and a brief exists:> Brief path: <brief-path>
<if Fit and ticket-driven:> Ticket path: <ticket-path>

Return:
- verdict: clean | non-clean
- findings: file/path references, severity, and concise rationale
```

## Doctrine

Implement optimizes for **verified code reaching the target branch with a
continuous lead context**. The lead owns edits, review fixes, and integration;
delegate prompts reach native subagents only through rendered prompt files, never
hand-pasted text. Route decides plan depth, branch mode, and review allocation
upfront; later stages execute per those decisions without re-routing. When a rule
is ambiguous, preserve the caller-provided scope and keep mutation paths
lead-owned.
