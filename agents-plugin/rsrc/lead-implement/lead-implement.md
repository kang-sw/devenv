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
- Name workflow skill handoffs with `{{.SkillNamespace}}:<skill>` notation.
- Follow the returned `raw` verdict and `next_instruction`; do not re-derive deterministic labels.
- Treat the installed todo list as the ordered runbook; do not create a parallel task list.
- Plans preserve selected-scope decisions about files, public interfaces, tests, exclusions, and accepted or rejected approaches.
- Delegate prompts are self-contained; resume is optional latency optimization. To continue a delegate, use the host's native continuation mechanism (e.g. SendMessage on Claude Code). If no such mechanism exists, re-spawn with a recap of the prior exchange — artifacts are self-contained so no information is lost. Mercenary delegates (prefer_mercenary mode) provide a host-neutral stateful resume path via backend session; re-spawn cold-context cost applies only to native-subagent delegates.

Review
- Reviewer count and partitions come from `review_alloc`.
- Lead aggregates reviewer severity verdicts and records final review status.
- Fix correctness, security, contract, and regression findings; reject style-only or scope-expanding findings with reasons.
- Relay only unresolved Critical/Important findings; record minor findings in the review summary.
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
| `facts.scope` | `span`, `surface`, `new_public_symbol`, `new_type_contract`, `test_surface`, `explicit_delegation_request`, `explicit_direct_edit_request` |
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

`explicit_direct_edit_request`: set to `yes` when the human or caller explicitly instructed direct edit (no delegation); overrides all other scope facts to `direct-edit`. Set to `no` when they explicitly requested delegation. Leave `unknown` otherwise.

**Fact-source rule**: Fill `facts.scope` fields from the ticket description before reading any source file. If a fact cannot be determined from the ticket alone, leave it `unknown`. Do not update `facts.scope` fields after reading source. An `unknown` span, surface, new_public_symbol, new_type_contract, or test_surface yields `delegated` by default.

**Edit gate**: No Edit or Write tool call is permitted until `enter.implement` has returned a `direct-edit` verdict. On a `delegated` verdict, source reading is permitted for routing, brief, and plan quality only — source mutation is owned by the implementer agent.

### 2. Execute Verdict

1. Read `raw`, `next_instruction`, warnings, and the installed todos.
2. Warnings explain inputs; the verdict still owns branch, delegation, plan, review, and docs.
3. Use `{{.McpNamespace}}/todo.read(key: "<todo-key>")` or `{{.McpNamespace}}/todo.list(mode: "full")` when an instruction is truncated.
4. If the verdict says `Branch Action: stop`, report the blocker before source edits.
5. Execute todos in order:
   - `{route}` / branch setup
   - `{prep}` / direct confirmation or delegated plan
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
3. For direct edit, keep implementation lead-owned from the ticket and do not create a planner artifact unless the route escalates.
4. For delegated work, call `{{.McpNamespace}}/path.generate(kind: "plan", stems: ["<ticket-stem-or-task>"])`; capture `<plan-path>`.
5. Render `plan-populator-survey` with `ticket_path`, `selected_phase`, and `plan_path`; dispatch it through **Delegate dispatch** and **Plan prompts**.
6. If survey returns `[escalate-to-research]`, render `plan-populator-research` with the same `ticket_path`, `selected_phase`, and `plan_path`; dispatch it through **Delegate dispatch** and **Plan prompts**.
7. Stop for unresolved binding decisions before source edits.
8. If a plan artifact was created, commit it before Edit.

### 4. Edit And Verify

1. Run the `{edit}` instruction.
2. For delegated work, use **Delegate dispatch** with **Implementer spawn prompt**.
3. Verify before reporting success; fix or escalate real blockers.
4. Commit logical checkpoints; record `<commit-range>` and `<result-commit>` for review and final reporting, or `none` with reason.

### 5. Review

1. Run `{review}` when installed.
2. Dispatch reviewers with **Reviewer table** and **Reviewer prompt frame**.
3. If `review_alloc=lead-only`, review directly and record the verdict; if `single`, dispatch `reviewer`; if `partitioned`, dispatch selected table rows.
4. For non-clean Critical/Important findings, classify accepted, rejected, and deferred dispositions; dispatch **Review relay dispatch** for accepted fixes, then use **Re-review prompt** for affected partitions.
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

1. Render the delegate playbook: `{{.McpNamespace}}/playbook.render(name: "<playbook>")`; capture prompt path and `recommended-tier` as dispatch metadata.
2. For `implementer`, pass only file-first render inputs: `PlanPath`, `VerificationHint`, `ResultExpectations`, and `CommitRangeHint`; `RoleModel` is declared in the prompt and tool-injected from tier metadata.
3. Native default: spawn a fresh subagent with only the rendered prompt path and task-specific input; choose the worker tier from dispatch metadata, but do not include `recommended-tier` in worker-facing task text.
4. Collect the normal completion report. If `ResultExpectations` names an output file, additionally require the output-file path plus a short summary.
<!-- ws:full-only:start -->
5. Mercenary path: register once with the rendered prompt file and `recommended-tier`, call with task-specific input, collect with `{{.McpNamespace}}/mercenary.result(name: "<name>", timeout_seconds: 600)`, and keep relay prompts self-contained.
<!-- ws:full-only:end -->
6. Task input mapping: `reference-discovery` gets target/domain when `{prep}` requests discovery; `implementer` gets **Implementer spawn prompt**; `implementer-relay` gets **Review relay dispatch**; reviewers get **Reviewer prompt frame**; plan populators get **Plan prompts** when `{prep}` requests a plan; `mental-model-updater` gets commit range plus output path.

### Plan contract

Path: `ai-docs/.plans/YYYY-MM/DD-hhmm-<stem-or-short-slug>.md`

Required sections: `Relevant Ticket Contract`, `Out of Scope`, `Codebase Findings`, `Implementation Plan`, `Verification Plan`, and `Escalations`.

### Plan prompts

**Survey:**
```text
Ticket path: <ticket-path>
Selected phase: <selected-phase>
Plan path: <plan-path>
```

**Research route:**
```text
Ticket path: <ticket-path>
Selected phase: <selected-phase>
Plan path: <plan-path>
Read the survey output at the same plan path, then refine or replace it.
```

### Implementer spawn prompt

```text
Rendered implementer prompt: <prompt-path>

Read that prompt file and execute it. It contains the plan path, verification
expectations, commit-range guidance, and reporting requirements.
```

### Reviewer table

| Partition | Reviewer name | Render playbook | Required check |
|-----------|---------------|-----------------|----------------|
| Correctness | `reviewer-correctness` | `code-review-correctness` | Ticket, plan, and correctness invariants |
| Fit | `reviewer-fit` | `code-review-fit` | Ticket decisions, plan guardrails, and future-phase fit |
| Test | `reviewer-test` | `code-review-test` | Ticket and plan verification coverage |

### Reviewer prompt frame

**Delegated or escalated route with generated plan:**
```text
Review partition: <Correctness|Fit|Test>
Diff range: <parent-of-first-commit>..<last-commit>
Ticket path: <ticket-path>
Plan path: <plan-path>
Findings path: <partition-output-path>

Review focus:
- <2-4 partition-specific risks>

Required checks:
- <required check from Reviewer table>
- Review the ticket contract, plan contract, and diff together.
- Plan guardrails were not bypassed.
- Binding ticket decisions were not omitted or violated.

Instructions:
- Ignore outside this partition unless directly broken by the diff.
- Write detailed findings to the findings path.
- In the message response, return only: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

**Direct edit with no generated plan:**
```text
Review partition: <Correctness|Fit|Test>
Diff range: <parent-of-first-commit>..<last-commit>
Ticket path: <ticket-path>
Findings path: <partition-output-path>

Review focus:
- <2-4 partition-specific risks>

Required checks:
- <required check from Reviewer table>
- Review the ticket contract and diff together.
- Binding ticket decisions were not omitted or violated.

Instructions:
- Do not require a plan artifact for direct-edit review.
- Ignore outside this partition unless directly broken by the diff.
- Write detailed findings to the findings path.
- In the message response, return only: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

### Review relay dispatch

```text
Render `implementer-relay` with declared inputs: PlanPath, ReviewCycle,
CommitRange, ReviewPaths, DispositionNotes, VerificationHint, and
ResultExpectations. Capture prompt path and recommended-tier.

Rendered review relay prompt: <prompt-path>

Read that prompt file and execute it. It contains the review findings paths,
disposition notes, verification expectations, and reporting requirements.
```

### Re-review prompt

```text
Re-review. Rely only on this prompt and named paths.
Updated diff range or patch path: <range-or-path>
Prior findings and dispositions path: <path>
Findings path: <partition-output-path>
Verify [fixed] items and report new issues introduced by the updated diff.
For each [won't fix], respond [accepted] or [maintained: <short reason>].
Write detailed findings to the findings path.
In the message response, return only: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

### 9. Branch Cleanup

Run after a confirmed merge to reduce branch accumulation.

1. Verify the implementation branch is a strict ancestor of the merge target: `git merge-base --is-ancestor <branch> <target>`.
2. Skip deletion and report retained when any of the following hold: the branch is currently checked out, it is linked to an active worktree, the merge target was ambiguous, or the branch has commits not reachable from the merge target.
3. If no skip condition holds, ask the user whether to delete the branch.
4. Delete only on explicit user approval: `git branch -d <branch>`.
5. Report each retained branch with its skip reason so cleanup debt stays visible.

## Doctrine

Implement optimizes for **execution attention**: route facts go to MCP, verdict-specific work goes to todos, and the playbook keeps only shared gates, ownership boundaries, and reusable templates.
