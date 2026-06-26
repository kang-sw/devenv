---
kind: print
delegates: true
---
# Implement

Target: user request

## Invariants

Scope
- Honor caller-provided scope or phase slices as hard implementation boundaries.
- Direct-edit escalates to delegated when scope becomes multi-file with new public API or cross-module new pattern.

Branch
- After Final Action Gate, wait for user approval before merging or starting another implementation slice.
- Merge commits follow repository commit rules and include `## AI Context`.

Execution
- Emit a user-facing Implementation Verdict after Route and before Prep as a non-blocking route summary.
- Create the task list during Prep; every task is mandatory and ordered.
- Delegated: implementer receives only the brief and optional plan as task input; extra docs must be listed in brief References; never read the ticket directly.
- Brief preserves every selected-scope binding decision; audit before commit.
- Treat every delegate dispatch as stateless; loop continuity is lead-owned via commit `## AI Context`, and same-agent resume is only a latency optimization, never a correctness dependency.

Review
- Single Review stage for all paths; reviewer count from `judge: review-allocation`.
- Lead fixes correctness, security, contract, and regression findings; may reject style-only or scope-expanding findings with reasons.
- The lead owns the clean decision from each reviewer's severity verdict (`clean` / `clean with N minor remaining` / `non-clean: M critical/important`, M = count of Critical/Important issues); `clean with N minor remaining` is clean unless the lead elects another cycle for a minor item.
- Do not re-relay a finding already settled (won't-fix/accepted) or deferred in a prior cycle — recognize it from the disposition record (the accumulated `[fixed]`/`[won't fix]`/`[deferred]`/`[accepted]` markers from prior cycles, also kept in fix-commit `## AI Context`); relay only genuinely new Critical/Important findings, let minors flow to the final report, with the per-path cycle cap as backstop.
- After capturing review result and disputes, delete review path files before Final Action Gate.

## On: invoke

### 1. Route

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, artifacts, and caller-provided slice.
3. Apply `judge: needs-delegation`.
4. Record `<current-branch>`.
5. Apply `judge: branch-mode` to `<current-branch>`.
6. Apply `judge: plan-depth`.
7. Apply `judge: review-allocation`.

### 2. Emit Implementation Verdict

1. Emit **Implementation Verdict** using the template.
2. State only observable routing inputs and final judgment labels.
3. Do not use `NEXT:`; the lead-implement procedure is starting implementation, not routing to a sibling workflow skill.
4. Do not include chain-of-thought, alternatives considered, or private scoring.
5. Continue immediately to Prep after emitting the template.

### 3. Prep

1. Reuse `<current-branch>` recorded during Route.
2. Outside `implement/*`, set `<merge-target>` to `<current-branch>`.
3. On `implement/*`, set `<merge-target>` from caller or confirm before execution.
4. If continuing on `implement/*` and branch name no longer matches selected scope, rename with `git branch -m implement/<scope>`; stop if target branch exists or upstream tracking is ambiguous.
5. Record `<implementation-start>` with `git rev-parse HEAD`.
6. If `branch-mode` = create: create `implement/<scope>` before any source edit.
7. Call `{{.McpNamespace}}/enter.implement(session_key: <lead key>, delegation: <needs-delegation result>, plan_depth: <plan-depth>, branch_mode: <branch-mode>, review_alloc: <review-allocation>, current_branch: <current-branch>, merge_target: <merge-target>, start_commit: <implementation-start>, need_review: <review-allocation != lead-only>, need_doc: true, active_agents: [])` to record implement-mode context and replace the session-state todo list with the derived implement checklist. This mirrors (does not replace) the host task list created below.
8. Call `{{.McpNamespace}}/mental_models.find(query: <target or domain>)` or `{{.McpNamespace}}/mental_models.status(domain: <domain>)`; read returned docs, ancestors first.
9. If the target or ticket touches plugin architecture, host-neutral migration, spawn-removal, or adapter boundaries, read `ai-docs/tickets/idea/260605-research-ws-native-subagent-pivot.md`.
10. Call `{{.McpNamespace}}/infra.read(name: "impl-playbook")`.
11. If an implementation choice depends on a documented decision, prior rejection, architecture fact, or cross-ticket constraint absent from the target or loaded docs, search the ticket/spec/mental-model cascade before editing and report a blocker instead of inferring.
12. Identify integration test paths and their run command.
13. If `plan-depth` ≥ survey: discover reference docs by dispatching `reference-discovery` per **Delegate dispatch** (task input: target or domain); capture `[Must|Maybe]` doc references. This delegate reads docs only; source-level reference mapping happens in step 14 via `plan-populator-survey`.
14. If delegated or `plan-depth` ≥ brief: write brief at `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md` using **Brief template**; include survey references when available; if the migration anchor was read, copy every binding implementation constraint into the brief and add the anchor as a `[Must]` reference; audit against target before committing or running plan population; commit.
15. If `plan-depth` ≥ survey: run the plan populator by dispatching `plan-populator-survey` per **Delegate dispatch** with **Plan prompts** as the task input; if survey returns `[escalate-to-research]`, re-dispatch `plan-populator-research`; if plan returns `[escalate-to-lead]`, stop and report blocker; commit plan.
16. If delegated: render the implementer prompt via `{{.McpNamespace}}/playbook.render(name: "implementer")` per **Delegate dispatch**; save the rendered prompt path and `recommended-tier` for the Edit stage.
17. Create and maintain task list:

```text
[ ] Route - delegation, plan depth, branch mode, review allocation
[ ] Prep - branch, context, brief/plan (per plan-depth)
[ ] Edit - direct-edit or spawn implementer; capture commit range
[ ] Review - run reviewer relay according to `judge: review-allocation`
[ ] Doc pre-pass - call `{{.McpNamespace}}/playbook.print(name: "lead-update-spec")` and execute inline, then `mental-model-updater`; commit each
[ ] Doc commit gate - refresh _index.md, ticket status, then commit docs
[ ] Doc closeout compaction - compact safe documentation-only branch-tip suffix
[ ] Final action gate - wait for merge, continue, or stop
[ ] Merge - only when approved
```

### 4. Edit

1. If direct-edit: edit directly per target and impl-playbook; commit logical checkpoints.
2. If direct-edit: run tests/build; read full output before claiming pass; resolve introduced warnings per impl-playbook Verify; on failure, diagnose blame before fixing; re-run until pass or real blocker.
3. If delegated and referenced tests exist: run baseline verification.
4. If delegated: spawn the implementer per **Delegate dispatch** with the **Implementer spawn prompt** as the task-specific input.
5. If delegated: read the implementer result only if the async result lacks a usable summary; capture `<first-commit>..<last-commit>`.
6. Capture `<commit-range>` and `<result-commit>`.

### 5. Review

1. If lead-only: record rationale; skip to step 8.
2. If single: dispatch the `reviewer` playbook per **Delegate dispatch** (the general reviewer; its shared base covers correctness, standards, contract, and security); generate path via `{{.McpNamespace}}/path.generate(kind: "review", stems: ["direct"])`.
3. If partitioned: choose partition subset from Tier 2; for each, dispatch its partition playbook from the **Reviewer partition table** per **Delegate dispatch**; generate paths via `{{.McpNamespace}}/path.generate(kind: "review", stems: ["correctness", "fit", "test"])`.
4. Call reviewer(s) with **Reviewer prompt frame**.
5. The lead decides clean from each reviewer's severity verdict (see Review invariants); when the run is clean, proceed to step 8 (review cleanup).
6. If non-clean and single: read review path; classify findings (fix: correctness/security/contract/regression; reject: style-only or scope expansion); apply fixes; re-verify; re-call the reviewer with the **Re-review prompt**, populating its prior-findings-and-dispositions from these fix/reject classifications. Repeat until clean or 2 cycles.
7. If non-clean and partitioned: relay to implementer with **Review relay prompt**; extract the disposition list; re-review non-clean partitions with the **Re-review prompt**; keep clean partitions accepted unless a fix touched their surface. Repeat until clean or 3 cycles; lead adjudicates at cycle 2; caller escalation at cycle 3.
8. Summarize review outcomes and disputes for the final report, then delete all review path files.

### 6. Doc Pre-Pass

1. Call `{{.McpNamespace}}/playbook.print(name: "lead-update-spec")` and execute the returned procedure inline with `<commit-range>`.
2. Dispatch `mental-model-updater` per **Delegate dispatch** (task input: `Commit range: <commit-range>` plus the target output path).
3. Wait for completion; commit file changes.

Run mental-model-updater after update-spec so it sees implemented-marker changes.

### 7. Doc Commit Gate

1. Call `{{.McpNamespace}}/infra.read(name: "executor-wrapup")`.
2. Refresh `ai-docs/_index.md` for new skills, agents, or major patterns.
3. If ticket-driven, follow Ticket Update using `<result-commit>`.
4. Commit doc changes per executor-wrapup. Do not re-run Doc Pipeline.

### 8. Doc Closeout Compaction

1. Run this gate for every supported branch mode; implementation runs are always on scoped implementation branches.
2. Inspect commits from `HEAD` backward after Doc Commit Gate; build only the contiguous branch-tip suffix of eligible documentation closeout commits.
3. An eligible commit is non-merge, workflow-owned, and changes only `ai-docs/spec/`, `ai-docs/mental-model/`, `ai-docs/tickets/`, `ai-docs/_index.md`, or narrowly relevant `ai-docs/ref/` workflow docs.
4. Stop suffix collection at the first ineligible commit; never cross source, test, skill, runtime, generated, planning, ready-promotion, review-fix, merge, or ambiguous-authorship commits.
5. If the suffix has fewer than two eligible commits, set `<doc-compaction-status>` to `skipped - fewer than two eligible closeout commits` and continue.
6. Compact the suffix into one closeout commit only when metadata synthesis is unambiguous; preserve AI Context, ticket Result references, Updated Tickets, Updated Specs, Mental Model Notes, and doc-audit rationale from absorbed commits.
7. After compaction, verify the final tree matches the pre-compaction head; if equivalence cannot be proven, restore the pre-compaction head and report `<doc-compaction-status>` as skipped with the blocker.

### 9. Final Action Gate

Report:

- implemented changes from direct-edit or implementer output;
- documentation updates and ticket Result hash;
- doc closeout compaction status;
- review result;
- test status;
- deviations or open items;
- unresolved disputes from review relay, if any.

Stop after reporting and wait for the user to choose merge, continue with a new
slice, or stop. If the user wants more changes, route to a new
implementation slice or `{{.SkillNamespace}}:lead-sprint`; completed phases capture follow-up
through append-only ticket Result editions. If the user chooses stop, leave the
implementation branch unmerged and report the branch name plus merge target.

### 10. Merge

Merge only when the user approves. Merge `implement/<scope>` to
`<merge-target>` with the repository merge helper or equivalent non-interactive
git sequence.

Use fast-forward for a single workflow-owned, message-clean commit. For multiple
commits, use `--no-ff` by default; fast-forward only when each commit is an
independent deployable and independently revertible target-history unit. Use
squash when the branch is one logical change with noisy or dependent commits.
For squash or `--no-ff`, write the merge commit per repository commit rules.

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
| Create implementation branch | Current branch does not start with `implement/` |

### judge: plan-depth

Default: `none` for direct-edit; `survey` for delegated when uncertain.
Delegated implementation has minimum plan-depth `brief`; direct-edit may use `none`.

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

| Partition | Default tier | When |
|-----------|--------------|------|
| Correctness | large | New logic, modified error paths, contract/security surface |
| Fit | medium | Existing components reused/modified, new pattern, or ticket-driven decision preservation |
| Test | medium | Tests added/modified, or new code paths lack coverage |

Choose the smallest partition set that covers material risk.
Full review (all three): reserve for cross-cutting behavior plus runtime/tooling plus test surface.
Default reviewer tier per partition in first-class vocabulary (`small`/`medium`/`large`/`xlarge`); raise a partition's tier for unusually subtle risk. When a delegate playbook declares its own `tier:`, the `recommended-tier` returned by `{{.McpNamespace}}/playbook.render` is the source of truth for that delegate.
Native delegation treats the tier as a model-selection guide.
<!-- ws:mercenary-on:start -->
Mercenary delegation passes the recommended capability tier to `ws.mercenary.register`, which resolves it directly to a concrete per-harness model via capability-keyed config.
<!-- ws:mercenary-on:end -->

## Templates

### Implementation Verdict

```text
## Implementation Verdict

- **Target**: <ticket path/stem or inline target>
- **Mode**: <direct edit | delegated>
- **Branch Mode**: <continue implementation branch | create implementation branch>
- **Plan Depth**: <none | brief | survey | research>
- **Review Allocation**: <lead-only | single reviewer | partitioned: correctness[, fit][, test]>
- **Scope**: <selected phase, whole target, or caller-provided slice>
- **Reason**: <decisive route facts only>

Proceeding with implementation.
```

### Delegate dispatch

Canonical render+spawn idiom for every bundled delegate (`reference-discovery`,
`implementer`, `reviewer` / review partitions, `mental-model-updater`,
`plan-populator-survey`/`plan-populator-research`). Every dispatch is a fresh
spawn — there is no session continuation between calls; the lead owns loop
continuity via commit `## AI Context`. Native is the default.
<!-- ws:full-only:start -->
Mercenary is controlled by `"workflow.prefer_mercenary"`: `hide` suppresses the
public surface, `off` exposes on-request use, and `on` makes mercenary the
primary implementer/reviewer guidance. When exposed, it provides host-neutral
stateful continuation by reusing the same mercenary name across relay calls.
Use the Mercenary dispatch item below instead of the Native item, reusing the
registered name from that delegate role's initial dispatch.
<!-- ws:mercenary-on:start -->
When `"workflow.prefer_mercenary"` is `on`, treat the Mercenary dispatch item as
the primary implementer/reviewer path unless the task requires native-only
capabilities.
<!-- ws:mercenary-on:end -->
<!-- ws:full-only:end -->

1. Render the delegate playbook: `{{.McpNamespace}}/playbook.render(name: "<playbook>")`; capture the rendered prompt path and the returned `recommended-tier`. Pass no `context` — these delegates declare only model-alias vars, which the tool auto-injects; caller-supplied undeclared keys error. For a lead `session_key` the rendered prompt already carries the minted child-key credential block, so the delegate's ws calls are pre-keyed.
1. Native (default): spawn a **fresh** native subagent whose instruction is to read the rendered prompt as its full role, then act on the task-specific input below; treat `recommended-tier` as the model-selection guide. Every relay to a prior delegate role is also a fresh spawn with a self-contained prompt. If dispatching as a fork (subagent_type: fork), always end the prompt with: "**You are a forked agent. Execute all work directly — do not sub-delegate.**"
<!-- ws:full-only:start -->
1. Mercenary (when selected): `ws.mercenary.register(name: "<name>", system_prompt_text: <rendered prompt>, tier: <recommended-tier>)`, then `ws.mercenary.call(name: "<name>", prompt: <task-specific input>)`; collect with `ws.mercenary.result(name: "<name>", timeout_seconds: 600)`. If collection times out or returns no usable result, report the failure and continue with native dispatch or stop for lead judgment.
<!-- ws:full-only:end -->
1. Task-specific input is handed to the worker, never to the render call: `reference-discovery` ← target or domain; `implementer` ← the **Implementer spawn prompt**; `reviewer` / partitions ← the **Reviewer prompt frame**; `plan-populator-*` ← the **Plan prompts**; `mental-model-updater` ← `Commit range: <commit-range>` plus the target output path. File-writing delegates write to their caller-created output path or return content; free-response delegates return text the lead integrates.

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

Read the brief, any provided plan, and all `[Must]` References listed in the brief.
Do not read the ticket directly or unlisted docs unless the brief or plan explicitly authorizes escalation.
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

| Partition | Reviewer name | Render playbook | Required check |
|-----------|---------------|-----------------|----------------|
| Correctness | `reviewer-correctness` | `code-review-correctness` | Verify correctness invariants. |
| Fit | `reviewer-fit` | `code-review-fit` | Verify brief contract, future-phase fit, ticket-driven decisions. |
| Test | `reviewer-test` | `code-review-test` | Verify coverage, assertions, integration-test instructions. |

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
- Return only the severity verdict: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`; the lead decides clean.
```

### Review relay prompt

Send to a **fresh implementer spawn** (not a continuation of the prior implementer session).

```text
Review cycle <N> (self-contained — rely only on this prompt and the paths it names, not on prior conversation).
Brief path: <brief-path>; implemented range: <commit-range>.
Non-clean review paths: <paths>. Read each file directly.
For each finding: [fixed], [won't fix: <reason>], or [deferred: <reason>].
Won't-fix allowed: style suggestions conflicting with codebase patterns; scope expansion beyond brief.
Won't-fix not allowed: correctness, security, or contract violations.
```

### Re-review prompt

```text
Re-review (self-contained — rely only on this prompt and the paths it names, not on prior conversation).
Updated diff: <diff>
Prior findings and dispositions: <each finding with [fixed] / [won't fix: <reason>] / [deferred: <reason>]>
Verify whether each [fixed] item was actually addressed, and report any new issue the updated diff introduced, with severity. Do not classify findings as regression-vs-preexisting.
For each [won't fix] item: respond [accepted] or [maintained: <brief reason>]. [deferred] items need no response.
```

## Doctrine

Implement optimizes for **verified code reaching the target branch**. Route
decides delegation, plan depth, and branch mode upfront; subsequent stages
execute per those decisions without re-routing. Code quality lives in the Edit
stage (direct or implementer); review quality lives in the unified Review stage.
When ambiguous, keep routing out of execution and execution out of routing.
