---
kind: print
delegates: true
---
# Implement

Target: user request

## Invariants

Scope
- Honor caller-provided scope or phase slices as hard implementation boundaries.
- If direct editing discovers multi-file public API or cross-module pattern work, stop and report that the slice must be re-routed or delegated.

Branch
- After Final Action Gate, wait for user approval before merging or starting another implementation slice.
- Merge commits follow repository commit rules and include `## AI Context`.

Execution
- Read the MCP-authored Implementation Verdict after Route and follow its `Next:` instruction.
- Treat the todo list installed by `{{.McpNamespace}}/enter.implement` as the single runbook; every installed todo is mandatory and ordered.
- Delegated: implementer receives only the rendered implementer playbook, brief path, optional plan path, and task-specific prompt fields below.
- Brief preserves every selected-scope binding decision; audit before commit.
- Treat every delegate dispatch as stateless; loop continuity is lead-owned via commit `## AI Context`, and same-agent resume is only a latency optimization, never a correctness dependency.

Review
- Single Review stage for all paths; reviewer count and partitions come from the MCP verdict's `review_alloc`.
- Lead fixes correctness, security, contract, and regression findings; may reject style-only or scope-expanding findings with reasons.
- The lead owns the clean decision from each reviewer's severity verdict (`clean` / `clean with N minor remaining` / `non-clean: M critical/important`, M = count of Critical/Important issues); `clean with N minor remaining` is clean unless the lead elects another cycle for a minor item.
- If `review_alloc` names a partition outside the Reviewer partition table, stop and report an unsupported review allocation.
- Do not re-relay a finding already settled (won't-fix/accepted) or deferred in a prior cycle — recognize it from the disposition record (the accumulated `[fixed]`/`[won't fix]`/`[deferred]`/`[accepted]` markers from prior cycles, also kept in fix-commit `## AI Context`); relay only genuinely new Critical/Important findings, let minors flow to the final report, with the per-path cycle cap as backstop.
- After review completes, capture verdicts, unresolved disputes, and accepted or rejected findings, then delete temporary review path files before Final Action Gate.

## On: invoke

### 1. Route

1. Parse target: ticket path or inline description.
2. If ticket-driven: read ticket; extract scope, stem, artifacts, and caller-provided slice.
3. Gather normalized `target`, `facts`, and `policy` for `{{.McpNamespace}}/enter.implement`; do not choose final labels such as delegation, branch mode, plan depth, review allocation, review need, or documentation need.
4. Use `unknown` for facts that cannot be defended from the ticket, conversation, or loaded docs.
5. Put only explicit caller/user policy in `policy`; do not mirror observable Git state.
6. Use the lead `session_key` supplied by the rendered credential block; if absent, stop before `{{.McpNamespace}}/enter.implement` and report that routing cannot start.
7. Call `{{.McpNamespace}}/enter.implement`:

```json
{
  "session_key": "<lead key>",
  "target": {
    "kind": "<ticket | inline | unknown>",
    "label": "<ticket path, ticket stem, or inline summary>",
    "ticket_stem": "<ticket stem or null>",
    "ticket_path": "<ticket path or null>",
    "scope_label": "<selected phase, whole target, or caller slice>",
    "scope_slug": "<kebab branch suffix or null>"
  },
  "facts": {
    "scope": {
      "span": "<single-file | multi-file | unknown>",
      "surface": "<internal | public-interface | cross-module | unknown>",
      "new_public_symbol": "<yes | no | unknown>",
      "new_type_contract": "<yes | no | unknown>",
      "test_surface": "<none | existing | new-files | unknown>",
      "explicit_delegation_request": "<yes | no | unknown>"
    },
    "complexity": {
      "change_points": "<clear | partially-known | unknown>",
      "reuse_points": "<confirmed | unconfirmed | not-applicable | unknown>",
      "strategy_shape": "<single-obvious | multiple-viable | unknown>",
      "side_effect_risk": "<low | moderate | high | unknown>",
      "cold_context": "<yes | no | unknown>"
    },
    "risk": {
      "correctness": "<low | moderate | high | unknown>",
      "fit": "<low | moderate | high | unknown>",
      "test": "<low | moderate | high | unknown>",
      "security_or_contract": "<low | moderate | high | unknown>"
    }
  },
  "policy": {
    "branch": {
      "merge_target": "<required only when already on implement/*, otherwise null>",
      "allow_rename": "<yes | no | unknown>"
    },
    "review": {"override": "<explicit lead-only | single | partitioned, or null>"},
    "docs": {"mode": "<standard | skip-with-reason | unknown>", "reason": "<reason or null>"}
  },
  "format": "json"
}
```

### 2. Follow Implementation Verdict

1. Read the returned `raw` Implementation Verdict and `next_instruction`.
2. Treat MCP warnings as explanatory only; do not override the verdict's delegation, branch, review, plan, or documentation decisions.
3. Treat the todo list replaced by `{{.McpNamespace}}/enter.implement` as authoritative; do not recreate a separate manual task list.
4. Use `{{.McpNamespace}}/todo.read(key)` or `{{.McpNamespace}}/todo.list(mode: "full")` when the focused instruction is not already visible.
5. Execute todos in order; their instructions carry the verdict-reachable branch, prep, edit, review, doc, final-action, and merge runbook.
6. If `Branch Action: stop`, stop before source edits and report the blocker.
7. Do not call `{{.McpNamespace}}/enter.implement` again for this implementation instance.

### 3. Prep

1. Execute the `{prep}` todo; it names the verdict-reachable preparation path.
2. Load required mental models, migration anchor, and implementation runbook before source edits when the todo calls for them.
3. If the todo calls for brief or plan work, use **Brief template**, **Delegate dispatch**, and **Plan prompts** as named.
4. If prep surfaces an unresolved binding decision, stop before source edits, report the specific decision needed, and update the brief/plan or return to Route after the decision.
5. Commit prepared brief or plan artifacts before moving to Edit when prep created or changed them.

### 4. Edit

1. Execute the `{edit}` todo.
2. Use direct editing or delegated implementation exactly as the todo says.
3. When delegated, use **Delegate dispatch** with **Implementer spawn prompt**.
4. Verify before reporting success; on failure, diagnose blame before fixing and re-run until pass or real blocker.
5. Commit logical checkpoints and capture `<commit-range>` plus `<result-commit>`.

### 5. Review

1. Execute the `{review}` todo when present; lead-only review records the same severity verdict format with a short findings/disposition summary.
2. For reviewer dispatch, use **Reviewer partition table**, **Reviewer prompt frame**, and generated review paths.
3. For non-clean results, classify findings, fix or reject by Review invariants, and use **Review relay prompt** plus **Re-review prompt** when a relay is needed.
4. The lead decides clean from severity verdicts, deduplicates settled findings, enforces cycle caps, and escalates unresolved Critical/Important findings.
5. Summarize review outcomes and disputes for the final report before deleting review path files.

### 6. Doc Pre-Pass

1. Execute `{doc-pre-pass}` when the todo exists; absent doc todos mean the verdict skipped documentation.
2. For standard documentation, call `{{.McpNamespace}}/playbook.print(name: "lead-update-spec")` and execute the printed update-spec instructions directly; then run `mental-model-updater` per **Delegate dispatch** when mental-model updates are required or defensible.
3. Commit update-spec and mental-model changes separately when both produce changes, otherwise commit the single changed set.

### 7. Doc Commit Gate

1. Execute `{doc-commit-gate}` when the todo exists.
2. Call `{{.McpNamespace}}/infra.read(name: "executor-wrapup")` for standard documentation closeout.
3. Refresh `ai-docs/_index.md` only for new skills, agents, or major patterns.
4. If ticket-driven, update the ticket result section according to the repository ticket convention.
5. Commit doc changes per executor-wrapup; do not re-run Doc Pipeline.

### 8. Doc Closeout Compaction

1. Execute `{doc-closeout}` when the todo exists.
2. Compact only a contiguous branch-tip suffix of safe documentation-only closeout commits.
3. Skip compaction when fewer than two eligible commits exist or metadata synthesis is ambiguous.
4. Verify tree equivalence after compaction; restore and report skipped status when equivalence cannot be proven.

### 9. Final Action Gate

1. Execute `{final-action-gate}`.
2. Report implemented changes, implementation branch, merge target if known, documentation updates or skip reason, ticket Result hash when applicable, doc closeout status, review result, test status, deviations, and unresolved disputes.

Stop after reporting and wait for the user to choose merge, continue with a new
slice, or stop. If the user wants more changes, route to a new
implementation slice or `{{.SkillNamespace}}:lead-sprint`; completed phases capture follow-up
through append-only ticket Result editions. If the user chooses stop, leave the
implementation branch unmerged and report the branch name plus merge target.

### 10. Merge

Execute `{merge}` only after user approval. Merge to the MCP verdict's merge
target, or to the caller-approved merge target when the verdict leaves it unset,
using the repository merge helper or equivalent non-interactive git sequence.
For squash or `--no-ff`, write the merge commit per repository commit rules.
After merge, report merge target, merge commit hash, verification status, branch
status, and skipped cleanup or unresolved follow-up.

## Fact Guidance

- Facts describe observed or judged input state only; do not derive final labels locally.
- Use `unknown` when the fact cannot be defended from the ticket, conversation, or loaded docs.
- Use `policy.branch.merge_target` only when already on `implement/*` or when the user explicitly names a merge target.
- Use `policy.branch.allow_rename=yes` only when the caller explicitly accepts a safe pre-edit branch rename.
- Omit `policy.review.override` or set it to `null` unless caller/user review policy is explicit.
- Use `policy.docs.mode=skip-with-reason` only with an explicit reason.
- Treat `review_alloc` partitions returned by MCP as the review dispatch plan.
- Default reviewer tiers are `small`, `medium`, `large`, and `xlarge`; raise a partition's tier only for unusually subtle risk.
- When a delegate playbook declares its own `tier:`, the `recommended-tier` returned by `{{.McpNamespace}}/playbook.render` is the source of truth for that delegate.
- Native delegation treats the tier as a model-selection guide.
<!-- ws:mercenary-on:start -->
Mercenary delegation passes the recommended capability tier to `{{.McpNamespace}}/mercenary.register`, which resolves it directly to a concrete per-harness model via capability-keyed config.
<!-- ws:mercenary-on:end -->

## Templates

### Delegate dispatch

Canonical render+spawn idiom for every bundled delegate (`reference-discovery`,
`implementer`, `reviewer` / review partitions, `mental-model-updater`,
`plan-populator-survey`/`plan-populator-research`). Native initial dispatch is
a fresh spawn. The lead owns loop continuity via commit `## AI Context`;
same-agent resume is a latency optimization, not a correctness dependency.
Native is the default.
<!-- ws:full-only:start -->
Mercenary is controlled by `"workflow.prefer_mercenary"`: `hide` suppresses the
public surface, `off` exposes on-request use, and `on` makes mercenary the
primary implementer/reviewer guidance. When exposed, it provides host-neutral
transport-level continuation by reusing the same mercenary name across relay calls,
but every relay prompt remains self-contained and correctness never depends on
prior agent conversation state.
Use the Mercenary dispatch item below instead of the Native item, reusing the
registered name from that delegate role's initial dispatch.
<!-- ws:mercenary-on:start -->
When `"workflow.prefer_mercenary"` is `on`, treat the Mercenary dispatch item as
the primary implementer/reviewer path unless the task requires native-only
capabilities.
<!-- ws:mercenary-on:end -->
<!-- ws:full-only:end -->

1. Render the delegate playbook: `{{.McpNamespace}}/playbook.render(name: "<playbook>")`; capture the rendered prompt path and the returned `recommended-tier`. Pass no `context` — these delegates declare only model-alias vars, which the tool auto-injects; caller-supplied undeclared keys error. For a lead `session_key` the rendered prompt already carries the minted child-key credential block, so the delegate's ws calls are pre-keyed.
1. Native (default): spawn a **fresh** native subagent with only the rendered prompt path plus the task-specific input below; provide no prior conversation transcript, loaded document excerpts, ticket body, or lead-only notes unless the task-specific input includes them or names them by path. Do not use inherited conversation context for delegated implementation or review relays. Instruct it to read the rendered prompt as its full role, then act on the task-specific input; treat `recommended-tier` as the model-selection guide. Wait for completion, capture the returned summary or output, and treat missing or unusable output as a lead judgment blocker unless the delegate wrote the required output file. For fix or re-review relays, prefer resuming the prior implementer or reviewer when the host supports it; otherwise use a fresh spawn. Every relay prompt must be self-contained.
<!-- ws:full-only:start -->
1. Mercenary (when selected): on the first dispatch for a delegate role, read the rendered prompt file, call `{{.McpNamespace}}/mercenary.register(name: "<name>", system_prompt_text: <rendered prompt file contents>, tier: <recommended-tier>)`, then `{{.McpNamespace}}/mercenary.call(name: "<name>", prompt: <task-specific input>)`; on relay calls, reuse the registered name and call/result only, but keep the relay prompt self-contained. Collect with `{{.McpNamespace}}/mercenary.result(name: "<name>", timeout_seconds: 600)`. If collection times out or returns no usable result, report the failure and continue with native dispatch or stop for lead judgment.
<!-- ws:full-only:end -->
1. Task-specific input is handed to the worker, never to the render call: `reference-discovery` ← target or domain; `implementer` ← the **Implementer spawn prompt**; `reviewer` / partitions ← the **Reviewer prompt frame**; `plan-populator-*` ← the **Plan prompts**; `mental-model-updater` ← `Commit range: <commit-range>` plus an output path only when one is defensible. If the task-specific input includes an output path, the delegate writes that file and returns a short completion summary naming it. If no output path is provided, the delegate returns the requested content in its final response.

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
Do not read the ticket directly. Do not read unlisted docs unless the brief or plan explicitly authorizes escalation.
Implement only the brief's scope boundary; leave later ticket phases untouched.

Acceptance criteria:
- Brief `## Contract Instructions` must be implemented or explicitly escalated.
- Brief `## Integration Test Instructions` must be satisfied.
- Test files: <integration test paths, or None with reason>
- Run: <verification command, or None with reason>

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
Diff range: <parent-of-first-commit>..<last-commit>
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

Send to the prior implementer when host resume is available. Otherwise dispatch
a fresh implementer using the rendered implementer playbook plus this
self-contained relay prompt; provide no prior transcript.

```text
Review cycle <N> (self-contained — rely only on this prompt and the paths it names, not on prior conversation).
Brief path: <brief-path>; implemented range: <commit-range>.
Non-clean review paths: <paths>. Read each file directly.
For each finding, return a disposition list with exact markers: [fixed], [won't fix: <reason>], or [deferred: <reason>].
Commit fixes, run the brief's verification command after fixes, and report commit hashes plus test results.
Won't-fix allowed: style suggestions conflicting with codebase patterns; scope expansion beyond brief.
Won't-fix not allowed: correctness, security, or contract violations.
```

### Re-review prompt

```text
Re-review (self-contained — rely only on this prompt and the paths it names, not on prior conversation).
Updated diff: <diff>
Prior findings and dispositions: <each finding with [fixed] / [won't fix: <reason>] / [deferred: <reason>]>
Findings path: <partition-output-path>
Verify whether each [fixed] item was actually addressed, and report any new issue the updated diff introduced, with severity. Do not classify findings as regression-vs-preexisting.
For each [won't fix] item: respond [accepted] or [maintained: <brief reason>]. A [maintained] response keeps that finding non-clean unless the lead adjudicates it as accepted. Preserve maintained items as unresolved disputes for the next relay or final report. [deferred] items need no response.
Write full findings to the findings path.
Return only the severity verdict: `clean`, `clean with N minor remaining`, or `non-clean: M critical/important`.
```

## Doctrine

Implement optimizes for **verified code reaching an implementation branch, then
the target branch after approved merge**. Route
decides delegation, plan depth, and branch mode upfront; subsequent stages
execute per those decisions without re-routing. Code quality lives in the Edit
stage (direct or implementer); review quality lives in the unified Review stage.
When ambiguous, keep routing out of execution and execution out of routing.
