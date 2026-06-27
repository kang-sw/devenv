---
kind: print
delegates: true
---
# Implement

Target: user request

## Invariants

Scope
- Honor caller-provided scope or phase slices as hard boundaries.
- Stop before source edits when direct-edit scope expands into public API or cross-module pattern work.

Branch
- Wait for user approval before merge or another implementation slice.
- Merge commits follow repository commit rules and include `## AI Context`.

Execution
- Call `{{.McpNamespace}}/enter.implement` once after facts are complete.
- Follow the returned `raw` verdict and `next_instruction`; do not re-derive deterministic labels.
- Treat the installed todo list as the ordered runbook; do not create a parallel task list.
- Briefs preserve selected-scope decisions about files, public interfaces, tests, exclusions, and accepted or rejected approaches.
- Delegate prompts are self-contained; resume is optional latency optimization.

Review
- Reviewer count and partitions come from `review_alloc`.
- Lead owns the clean decision from severity verdicts.
- Fix correctness, security, contract, and regression findings; reject style-only or scope-expanding findings with reasons.
- Relay only new Critical/Important findings; record minor findings in the review summary.
- Preserve settled or deferred dispositions.
- Summarize review evidence before deleting temporary review files.

## On: invoke

### 1. Route

1. Parse target: ticket path or inline description.
2. If ticket-driven, read the ticket and extract stem, selected scope, artifacts, and caller slice.
3. Gather `target`, `facts`, and explicit caller `policy` for `{{.McpNamespace}}/enter.implement`; use `unknown` unless a value is directly supported by the caller, ticket, loaded docs, file contents, command output, or MCP verdict.
4. Do not set decision fields for delegation, branch mode, plan depth, review allocation, review need, or documentation need.
5. Use the current lead `session_key` from `{{.McpNamespace}}/workflow_manual`; stop if no lead key is available.
6. Call `{{.McpNamespace}}/enter.implement` with `session_key`, `target`, `facts`, `policy`, and `format: "json"`.

```json
{
  "session_key": "<lead key>",
  "target": {"kind": "<ticket|inline|unknown>", "...": "..."},
  "facts": {"scope": {}, "complexity": {}, "risk": {}},
  "policy": {"branch": {}, "review": {}, "docs": {}},
  "format": "json"
}
```

Fact groups:

| group | fields |
| --- | --- |
| `target` | `kind`, `label`, `ticket_stem`, `ticket_path`, `scope_label`, `scope_slug` |
| `facts.scope` | `span`, `surface`, `new_public_symbol`, `new_type_contract`, `test_surface`, `explicit_delegation_request` |
| `facts.complexity` | `change_points`, `reuse_points`, `strategy_shape`, `side_effect_risk`, `cold_context` |
| `facts.risk` | `correctness`, `fit`, `test`, `security_or_contract` |
| `policy.branch` | `merge_target`, `allow_rename` |
| `policy.review` | `override` |
| `policy.docs` | `mode`, `reason` |

Use the live MCP tool schema for enum values; this table is a checklist, not a schema copy.

Policy rules:
- Set `policy.branch.merge_target` only when already on `implement/*` or the user names it.
- Set `policy.branch.allow_rename=yes` only when the caller accepts pre-edit branch rename.
- Set `policy.review.override` only for explicit caller review policy.
- Set `policy.docs.mode=skip-with-reason` only with an explicit reason.

### 2. Execute Verdict

1. Read `raw`, `next_instruction`, warnings, and the installed todos.
2. Warnings explain inputs; the verdict still owns branch, delegation, plan, review, and docs.
3. Use `{{.McpNamespace}}/todo.read(key: "<todo-key>")` or `{{.McpNamespace}}/todo.list(mode: "full")` when an instruction is truncated.
4. If the verdict says `Branch Action: stop`, report the blocker before source edits.
5. Execute todos in order:
   - `{route}` / branch setup
   - `{prep}` / brief or plan
   - `{edit}` / direct edit or implementer dispatch
   - `{review}` / lead, single, or partitioned review
   - `{doc-pre-pass}` / spec and mental-model updates
   - `{doc-commit-gate}` / ticket, index, and wrapup docs
   - `{doc-closeout}` / safe documentation-only suffix compaction
   - `{final-action-gate}` / completion report
   - `{merge}` / user-approved merge

### 3. Prep

1. Run the `{prep}` instruction.
2. Load required mental models, migration anchor, and `{{.McpNamespace}}/infra.read(name: "impl-playbook")` when instructed.
3. For delegated work, write the **Brief template** and optional plan before implementation.
4. Stop for unresolved binding decisions before source edits.
5. If brief or plan artifacts were created, commit them before Edit.

### 4. Edit And Verify

1. Run the `{edit}` instruction.
2. For delegated work, use **Delegate dispatch** with **Implementer spawn prompt**.
3. Verify before reporting success; fix or escalate real blockers.
4. Commit logical checkpoints; record `<commit-range>` and `<result-commit>` for review and final reporting, or `none` with reason.

### 5. Review

1. Run `{review}` when installed.
2. Dispatch reviewers with **Reviewer table** and **Reviewer prompt frame**.
3. If `review_alloc=lead-only`, review directly and record the verdict; if `single`, dispatch `reviewer`; if `partitioned`, dispatch selected table rows.
4. For non-clean Critical/Important findings, classify, fix or reject, then use **Review relay prompt** and **Re-review prompt** for affected partitions.
5. Record verdicts, minor findings, unresolved disputes, accepted/rejected findings, and review-clean status.

### 6. Documentation

1. Run installed doc todos only; absent doc todos mean documentation was skipped by verdict.
2. For `{doc-pre-pass}`, print and execute `{{.McpNamespace}}/playbook.print(name: "lead-update-spec")`, then dispatch `mental-model-updater` when workflow behavior, reusable domain rules, or modification guidance changed.
3. For `{doc-commit-gate}`, run `{{.McpNamespace}}/infra.read(name: "executor-wrapup")`, update ticket Result when ticket-driven, and refresh `_index.md` only for new skills, agents, or major patterns.
4. Commit spec and mental-model changes separately when both changed.

### 7. Closeout

1. Run `{doc-closeout}` when installed.
2. If `{doc-closeout}` instructs compaction, squash only consecutive documentation-only branch-tip commits; verify final tree equivalence or report skipped.
3. Run `{final-action-gate}` and report changes, branch, merge target, docs, ticket Result hash, review, tests, deviations, disputes, and skipped closeout.
4. Stop for the user's choice: merge, new slice, sprint, or stop.

### 8. Merge

Run `{merge}` only after user approval. Merge to the verdict or caller-approved target, write merge commits per repository rules, then report merge target, hash, verification, branch status, and skipped cleanup.

## Templates

### Delegate dispatch

1. Render the delegate playbook: `{{.McpNamespace}}/playbook.render(name: "<playbook>")`; capture prompt path and `recommended-tier`.
2. Native default: spawn a fresh subagent with only the rendered prompt path and task-specific input; collect summary or required output file.
<!-- ws:full-only:start -->
3. Mercenary path: register once with the rendered prompt file and `recommended-tier`, call with task-specific input, collect with `{{.McpNamespace}}/mercenary.result(name: "<name>", timeout_seconds: 600)`, and keep relay prompts self-contained.
<!-- ws:full-only:end -->
4. Task input mapping: `reference-discovery` gets target/domain when `{prep}` requests discovery; `implementer` gets **Implementer spawn prompt**; reviewers get **Reviewer prompt frame**; plan populators get **Plan prompts** when `{prep}` requests a plan; `mental-model-updater` gets commit range plus output path.

### Brief template

Path: `ai-docs/.plans/YYYY-MM/DD-<stem-or-short-slug>.brief.md`

```markdown
# Brief: <stem>

## Intent
<one paragraph>

## Scope Boundary
<selected scope and explicit exclusions>

## Caller-Visible Contract
<observable behavior, public API/protocol/UI/doc output/lifecycle contract>

## Contract Instructions
<files/modules; public types/functions/handlers/tools; visibility; call shape>
<existing mechanisms to reuse before adding new paths>
<temporary, fallback, or mock-data wiring that is forbidden>

## Integration Test Instructions
<boundary type; test files; pass criteria>

## Implementation Strategy Decisions
<settled approach the implementer must not reopen>

## Rejected Alternatives
<approaches ruled out and why>

## Approach
<macro-level bullets>

## Constraints
<must-hold conditions>

## Out of scope
<explicitly excluded>

## Details
<interface specs, data types, public contracts>

## Verification Contract
<tests, probes, command outputs required>

## References
<!-- [Must] read before starting. [Maybe] consult if uncertain. -->
- `ai-docs/mental-model/<path>` - <relevance>
```

### Plan prompts

**Survey:**
```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
```

**Research route:**
```text
Brief path: <brief-path>
Plan path: ai-docs/.plans/YYYY-MM/DD-<stem>.md
Read the survey output, then replace the file with a research plan.
```

### Implementer spawn prompt

```text
Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>

Read the brief, provided plan, and all `[Must]` References.
Do not read the ticket directly. Do not read unlisted docs unless authorized.
Implement only the brief's scope boundary.

Acceptance:
- Implement or escalate Brief `## Contract Instructions`.
- Satisfy Brief `## Integration Test Instructions`.
- Test files: <paths, or None with reason>
- Run: <command, or None with reason>

Ancestor loading: read `ai-docs/mental-model/<domain>/index.md` before any nested domain page.

Instructions:
- Verify before reporting completion and after each fix.
- Do not use temporary, fallback, or mock-data behavior for brief contracts.
- Escalate known wrong contracts or failed plan guardrails.
- Commit logical checkpoints on the current branch.
- Report commits and test results.
```

### Reviewer table

| Partition | Reviewer name | Render playbook | Required check |
|-----------|---------------|-----------------|----------------|
| Correctness | `reviewer-correctness` | `code-review-correctness` | Correctness invariants |
| Fit | `reviewer-fit` | `code-review-fit` | Brief contract, future-phase fit, ticket decisions |
| Test | `reviewer-test` | `code-review-test` | Coverage, assertions, integration instructions |

### Reviewer prompt frame

```text
Review partition: <Correctness|Fit|Test>
Diff range: <parent-of-first-commit>..<last-commit>
<if Fit:> Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>
<if Fit and ticket-driven:> Ticket path: <ticket-path>
Findings path: <partition-output-path>

Review focus:
- <2-4 partition-specific risks>

Required checks:
- <required check from Reviewer table>
- <if plan exists:> Plan guardrails were not bypassed.
- <if Fit and ticket-driven:> Binding decisions were not omitted or violated.

Instructions:
- Ignore outside this partition unless directly broken by the diff.
- Write full findings to the findings path.
- Return only: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

### Review relay prompt

```text
Review cycle <N>. Rely only on this prompt and named paths.
Brief path: <brief-path>; implemented range: <commit-range>.
Non-clean review paths: <paths>. Read each file directly.
For each finding, return [fixed], [won't fix: <reason>], or [deferred: <reason>].
Commit fixes, run verification, and report commit hashes plus test results.
Won't-fix allowed: style conflicts with codebase patterns; scope expansion beyond brief.
Won't-fix not allowed: correctness, security, contract, regression, or required-test violations.
```

### Re-review prompt

```text
Re-review. Rely only on this prompt and named paths.
Updated diff: <diff>
Prior findings and dispositions: <findings with [fixed] / [won't fix: <reason>] / [deferred: <reason>]>
Findings path: <partition-output-path>
Verify [fixed] items and report new issues introduced by the updated diff.
For each [won't fix], respond [accepted] or [maintained: <brief reason>].
Write full findings to the findings path.
Return only: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

## Doctrine

Implement optimizes for **execution attention**: route facts go to MCP, verdict-specific work goes to todos, and the playbook keeps only shared gates, ownership boundaries, and reusable templates.
