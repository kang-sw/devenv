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
- Wait for user approval before merge or another implementation slice, unless the resolved verdict's merge confirm is `skip`, in which case proceed with that merge without asking.
- Merge commits follow repository commit rules and include `## AI Context`.

Execution
- Call `{{.McpNamespace}}/route.resolve_implement` once after facts are complete.
- No Edit or Write tool call is permitted until `route.resolve_implement` returns a `direct-edit` verdict. On `delegated`, source reading is permitted for routing, brief, and plan quality only — source mutation is owned by the implementer agent.
- Name workflow skill handoffs with `{{.SkillNamespace}}:<skill>` notation.
- Follow the returned `raw` verdict and `next_instruction`; do not re-derive deterministic labels.
- Treat the installed todo list as the ordered runbook; do not create a parallel task list.
- Plans preserve selected-scope decisions about files, public interfaces, tests, exclusions, and accepted or rejected approaches.
- Delegate prompts are self-contained; resume is optional latency optimization. To continue a delegate, use the host's native continuation mechanism (e.g. SendMessage on Claude Code). If no such mechanism exists, re-spawn with a recap of the prior exchange — artifacts are self-contained so no information is lost. Mercenary delegates (prefer_mercenary mode) provide a host-neutral stateful resume path via backend session; re-spawn cold-context cost applies only to native-subagent delegates.

Review
- Reviewer count and partitions come from `review_alloc`.
- Lead aggregates reviewer severity verdicts and records final review status.
- Fix correctness, security, contract, and regression findings; reject style-only or scope-expanding findings with reasons.
- Critical is must-fix: bounded to 3 review rounds (review #1 plus up to 2 Critical-scoped re-reviews); a Critical still non-clean after review #3 unconditionally elevates to `implementer-elevated` — never a hard stop, and the run continues.
- Important is best-effort: relayed once, in relay #1 alongside Critical; a still-non-clean Important after that relay is not re-reviewed — it stands on the implementer's own `[not fixed: <reason>]` record.
- Minor drives no relay; record it in the review summary only.
- Preserve settled or deferred dispositions.
- Summarize review evidence before deleting temporary review files.

## Fact Contract

`{{.McpNamespace}}/route.resolve_implement`'s published schema is opaque
(`params: object`); this table is the authoritative field contract the
resolver reads. Send an outer `session_key` and one `params` object containing
`target`, `facts`, optional `policy`, and `format`; do not put a `session_key`
in `params` or mix typed or legacy fields into the outer envelope. Unwrapped
typed calls and the unwrapped legacy mode-entry call remain compatible; a
wrapped call always uses typed routing.

`target`
| Field | Type | Notes |
|-------|------|-------|
| `kind` | `ticket\|inline\|unknown` | |
| `label` | string\|null | |
| `ticket_stem` | string\|null | |
| `ticket_path` | string\|null | |
| `scope_label` | string\|null | Selected implementation scope label. |
| `scope_slug` | string\|null | Kebab-case branch suffix; ignored with a warning for ticket targets (deterministic word-key is authoritative). |

`facts.scope`
| Field | Enum |
|-------|------|
| `span` | `single-file\|multi-file\|unknown` |
| `surface` | `internal\|public-interface\|cross-module\|unknown` |
| `new_public_symbol` | `yes\|no\|unknown` |
| `new_type_contract` | `yes\|no\|unknown` |
| `test_surface` | `none\|existing\|new-files\|unknown` |
| `explicit_delegation_request` | `yes\|no\|unknown` |
| `explicit_direct_edit_request` | `yes\|no\|unknown` (overrides all other scope facts to direct-edit when `yes`) |

`facts.complexity`
| Field | Enum |
|-------|------|
| `change_points` | `clear\|partially-known\|unknown` |
| `reuse_points` | `confirmed\|unconfirmed\|not-applicable\|unknown` |
| `strategy_shape` | `single-obvious\|multiple-viable\|unknown` |
| `side_effect_risk` | `low\|moderate\|high\|unknown` |
| `cold_context` | `yes\|no\|unknown` |

`facts.risk`
| Field | Enum |
|-------|------|
| `correctness` | `low\|moderate\|high\|unknown` |
| `fit` | `low\|moderate\|high\|unknown` |
| `test` | `low\|moderate\|high\|unknown` |
| `security_or_contract` | `low\|moderate\|high\|unknown` |

`policy`
| Field | Enum/Type |
|-------|-----------|
| `low_ceremony_if_safe` | `yes\|no\|unknown` |
| `branch.merge_target` | string; required only while already on an implementation branch |
| `branch.allow_rename` | `yes\|no\|unknown` (defaults to allowed) |
| `branch.merge_confirm` | `skip\|ask\|unknown` (defaults to ask) |
| `review.override` | `auto\|lead-only\|single\|partitioned` |
| `docs.mode` | `standard\|skip-with-reason\|unknown` |
| `docs.reason` | string; required when `docs.mode=skip-with-reason` |

`format`: `text` (default) \| `json`. Groups and fields are optional;
unknown/null values are normalized by the resolver. MCP observes Git branch
state itself — do not pass observed branch facts as caller policy.

## On: invoke

### 1. Route

1. Parse target: ticket path or inline description.
2. If ticket-driven, read the ticket and extract stem, selected scope, artifacts, and caller slice.
3. Gather `target`, `facts`, and explicit caller `policy` for `{{.McpNamespace}}/route.resolve_implement`; use `unknown` unless a value is directly supported by the caller, ticket, loaded docs, file contents, command output, or MCP verdict.
4. Do not set decision fields for delegation, branch mode, plan depth, review allocation, review need, or documentation need.
5. Use the current lead `session_key` from `{{.McpNamespace}}/workflow_manual`; stop if no lead key is available.
6. Call `{{.McpNamespace}}/route.resolve_implement` with outer `session_key` and `params: {target, facts, policy, format: "json"}`.

Policy rules:
- Set `policy.branch.merge_target` only when already on an implementation branch (`impl/*`, or legacy `implement/*`) or the user names it.
- `policy.branch.allow_rename` defaults to `yes`; set it to `no` only when the caller has explicitly asked to preserve the current branch name.
- Map a clear request to streamline implementation to `policy.low_ceremony_if_safe=yes`; labels such as `hotfix`, `tweak`, or `small fix` alone do not count.
- Low-ceremony preference affects branch selection only; it waives no other policy or gate.
- Map an explicit documentation skip and its non-empty reason to `policy.docs`.

`explicit_direct_edit_request`: set to `yes` when the human or caller explicitly instructed direct edit (no delegation); overrides all other scope facts to `direct-edit`. Set to `no` when they explicitly requested delegation. Leave `unknown` otherwise.

**Fact-source rule**: Freeze `facts.scope` before the `route.resolve_implement` call. For tickets, use the ticket description only; for inline targets, use the accepted caller contract, loaded context, focused source inspection, and command output. Unsupported facts stay `unknown`; do not revise frozen facts after the call. An `unknown` span, surface, new_public_symbol, new_type_contract, or test_surface yields `delegated` by default.

### 2. Execute Verdict

1. Read `raw`, `next_instruction`, warnings, and the installed todos.
2. Use `{{.McpNamespace}}/todo.read(key: "<todo-key>")` or `{{.McpNamespace}}/todo.list(mode: "full")` when an instruction is truncated.
3. If the verdict says `Branch Action: stop`, report the blocker before source edits.
4. Execute installed todos in order; each todo's `Instruction` field is a complete how-to for that step — do not restate or supplement it from memory. The subsections below add only what the instruction omits.

### 3. Prep

- Stop for unresolved binding decisions before source edits.
- If a plan artifact was created, commit it before Edit.

### 4. Edit And Verify

- Commit logical checkpoints; record `<commit-range>` and `<result-commit>`, or `none` with reason.

### 5. Documentation

- `{doc-pre-pass}`: print and execute `{{.McpNamespace}}/playbook.read(name: "lead-update-spec")`.
- `{doc-commit-gate}`: if `ai-docs/_index.md` exists, refresh it only for new skills, agents, or major patterns (pre-dissolution coexistence only); otherwise update `AGENTS.md`'s `## Project Orientation` section only when repo identity, topology, or canonical flows changed.
- Commit spec and mental-model changes separately when both changed.

### 6. Closeout

- After doc-closeout compaction, verify final tree equivalence, or report skipped.
- `{final-action-gate}` report: changes, branch, merge target, docs, ticket Result hash, remaining phases or ticket-complete, review, tests, deviations, disputes, skipped closeout.
- Stop for the user's choice: merge or stop; offer a next-slice continuation only when this ticket still has an unfinished phase.

### 7. Merge

Report: merge target, hash, verification, branch status, skipped cleanup.

### 8. Branch Cleanup

Run after a confirmed merge to reduce branch accumulation.

1. Verify the implementation branch is a strict ancestor of the merge target: `git merge-base --is-ancestor <branch> <target>`.
2. Skip deletion and report retained when any of the following hold: the branch is currently checked out, it is linked to an active worktree, the merge target was ambiguous, or the branch has commits not reachable from the merge target.
3. If no skip condition holds and the branch name matches `impl/*`, delete without asking: `git branch -d <branch>`.
4. Otherwise (no skip condition held but the branch does not match `impl/*`, including legacy `implement/*`), ask the user whether to delete the branch, and delete only on explicit user approval: `git branch -d <branch>`.
5. Report each retained branch with its skip reason so cleanup debt stays visible.

## Templates

### Delegate dispatch

1. Render the delegate playbook: `{{.McpNamespace}}/playbook.render(name: "<playbook>")`; capture prompt path and `recommended-tier` as dispatch metadata.
2. For `implementer`, pass only file-first render inputs: `PlanPath`, `VerificationHint`, `ResultExpectations`, and `CommitRangeHint`; `RoleModel` is declared in the prompt and tool-injected from tier metadata.
3. Native default: spawn a fresh subagent with only the rendered prompt path and task-specific input; choose the worker tier from dispatch metadata, but do not include `recommended-tier` in worker-facing task text.
4. Collect the normal completion report. If `ResultExpectations` names an output file, additionally require the output-file path plus a short summary.
<!-- ws:full-only:start -->
5. Mercenary path: register once with the rendered prompt file and `recommended-tier`, call with task-specific input, collect with `{{.McpNamespace}}/mercenary.result(name: "<name>", timeout_seconds: 600)`, and keep relay prompts self-contained.
<!-- ws:full-only:end -->
6. Task input mapping: `reference-discovery` gets target/domain when `{prep}` requests discovery; `implementer` gets **Implementer spawn prompt**; `implementer-relay` gets **Review relay dispatch**; `implementer-elevated` gets **Review relay dispatch** when the Critical ceiling fires (review #3 still reports the Critical finding non-clean); reviewers get **Reviewer prompt frame**; plan populators get **Plan prompts** when `{prep}` requests a plan; `mental-model-updater` gets commit range plus output path.

### Plan contract

Path: `ai-docs/.plans/YYYY-MM/DD-hhmm-<stem-or-short-slug>.md`

Required sections: `Relevant Ticket Contract`, `Out of Scope`, `Codebase Findings`, `Implementation Plan`, `Verification Plan`, and `Escalations`.
For inline plans, `Relevant Ticket Contract` contains the accepted inline contract.

### Plan prompts

Choose one authority row and leave the other authority fields empty:

| Target kind | Planner authority inputs |
|-------------|--------------------------|
| `ticket` | `ticket_path`, `selected_phase`, empty `inline_contract`, `plan_path` |
| `inline` | empty `ticket_path`/`selected_phase`, self-contained `inline_contract`, `plan_path` |

The inline contract contains accepted scope, constraints, non-goals, and verification boundary. Research receives the same authority inputs and plan path, plus:
```text
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
| Full scope | `reviewer` | `reviewer` (includes `code-reviewer`) | Correctness, fit, and test against the supplied authority |
| Correctness | `reviewer-correctness` | `code-review-correctness` | Supplied authority, plan, and correctness invariants |
| Fit | `reviewer-fit` | `code-review-fit` | Supplied authority decisions, plan guardrails, and future-phase fit |
| Test | `reviewer-test` | `code-review-test` | Supplied authority and plan verification coverage |

### Reviewer prompt frame

**Generated plan:** choose exactly one authority line.
```text
Read First: <rendered reviewer playbook path>
Review scope: <Full scope: correctness, fit, test|Correctness|Fit|Test>
Diff range: <parent-of-first-commit>..<last-commit>
Authority: Ticket path <ticket-path>
Authority: Inline contract <accepted scope, constraints, non-goals, verification boundary>
Plan path: <plan-path>
Findings path: <review-output-path>

Review focus:
- <2-4 scope-specific risks>

Required checks:
- <required check from the Reviewer table>
- Review the supplied authority, plan contract, and diff together.
- Plan guardrails were not bypassed.
- Each specified authority requirement is implemented, or carries an explicit, authorized deferral.

Instructions:
- For inline authority, do not read or require a ticket path.
- For a partition, ignore outside it unless directly broken by the diff.
- Write detailed findings to the findings path.
```

**Direct edit with no generated plan:** use the same frame, omit `Plan path`, and require authority-plus-diff review without a plan artifact.

### Review relay dispatch

```text
Render `implementer-relay` with declared inputs: PlanPath, ReviewCycle,
CommitRange, ReviewPaths, DispositionNotes, VerificationHint, and
ResultExpectations. Capture prompt path and recommended-tier.

When the Critical ceiling fires (review #3 still reports the Critical finding
non-clean), render `implementer-elevated` in place of `implementer-relay`, with
those same declared inputs plus PriorFixCommits and PriorDispositions.

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
For each [fixed], respond [resolved] or [unresolved: <short reason>].
For each [won't fix], respond [accepted] or [maintained: <short reason>].
Write detailed findings to the findings path.
In the message response, return only: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

## Doctrine

Implement optimizes for **execution attention**: route facts go to MCP, verdict-specific work goes to todos, and the playbook keeps only shared gates, ownership boundaries, and reusable templates.
