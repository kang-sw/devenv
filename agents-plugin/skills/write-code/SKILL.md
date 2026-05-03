---
name: write-code
description: Delegate a ticket or inline implementation target through a brief, optional plan, Codex implementer session, partitioned reviewer fanout, bounded relay loop, cleanup, and completion report.
---

# Write Code

## Invariants

- Operate on the current branch; branch creation, approval gates, merge, and spec updates belong to the caller.
- The lead writes the brief and optional plan before implementation starts.
- The implementer reads only the brief and optional plan; it must not read the ticket directly.
- The implementer may commit logical implementation checkpoints on the current branch.
- Register every named agent through `ws/agents.register` with embedded prompt stems.
- Use `ws/agents.call_async` for implementer and reviewer turns that may run long.
- Use `ws/path.generate` for reviewer finding files.
- Reviewers write complete findings to files and return only compact clean or non-clean summaries.
- Fit reviewers may consult the ticket for architectural headroom; correctness and test reviewers stay scoped to diff, brief, and tests.
- Relay review findings to the implementer by file path, not by copying full findings into the lead context.
- Cap review relays at three implementer fix cycles.
- Delete generated review files and erase named agents before returning.
- End with `Templates / Completion Report`.

## On: Write Code

1. Parse the request as either a ticket path, ticket stem, or inline implementation target.
2. Record `<start-commit>` from the current `HEAD`.
3. If ticket-driven, read the ticket and identify the target phase, scope, constraints, skeleton metadata, and ticket stem.
4. Call MCP tool `ws/agents.oneshot` with `prompts: ["project-survey"]` and `Templates / Project Survey Prompt`.
5. Write `<brief-path>` using `Templates / Brief` under `ai-docs/.plans/YYYY-MM/DD-<stem>.brief.md`.
6. Commit the brief as a lead-owned checkpoint before delegating implementation.
7. Apply `judge: plan-depth`; default to `survey` when uncertain between `as-is` and `survey`.
8. If plan depth is `survey`, call `ws/agents.oneshot` with `prompts: ["plan-populator-survey"]` and `Templates / Plan Population Prompt`.
9. If plan depth is `research`, call `ws/agents.oneshot` with `prompts: ["plan-populator-research"]` and `Templates / Plan Population Prompt`.
10. If a plan was written, review it and commit the plan as a lead-owned checkpoint before implementation.
11. Apply `judge: skeleton-gate`; stop and suggest `ws:write-skeleton` when skeleton work is required first.
12. Identify the verification commands or test targets the implementer must run, using existing project documentation when available.
13. Call MCP tool `ws/path.generate` with `kind: "review"` and `stems: ["correctness", "fit", "test"]`.
14. Store the returned paths as `<correctness-path>`, `<fit-path>`, and `<test-path>`.
15. Call MCP tool `ws/agents.register` for `implementer` with `backend: "codex"`, `tier: "core"`, and `prompts: ["implementer", "impl-playbook"]`.
16. Call MCP tool `ws/agents.register` for `reviewer-correctness` with `prompts: ["code-reviewer", "code-review-correctness"]`.
17. Call MCP tool `ws/agents.register` for `reviewer-fit` with `prompts: ["code-reviewer", "code-review-fit"]`.
18. Call MCP tool `ws/agents.register` for `reviewer-test` with `prompts: ["code-reviewer", "code-review-test"]`.
19. Call MCP tool `ws/agents.call_async` for `implementer` using `Templates / Implementer Prompt`.
20. Use `ws/agents.status` or `ws/agents.tail` while the implementer is running only when progress or diagnostics are needed.
21. Call MCP tool `ws/agents.wait` for `implementer` when final implementation output is needed.
22. If the wait result lacks a usable summary, call MCP tool `ws/agents.print` for `implementer`.
23. Determine `<implementation-range>` from commits created after `<start-commit>` and the implementer report.
24. Apply `judge: partition-allocation`; default to all three reviewer partitions for non-trivial implementation.
25. For every selected reviewer, call MCP tool `ws/agents.call_async` with the matching reviewer prompt template.
26. Use `ws/agents.wait` for each selected reviewer and call `ws/agents.print` for any reviewer whose wait output is incomplete.
27. If every reviewer summary is `[clean]`, proceed to cleanup.
28. If any reviewer summary is `[non-clean]`, increment `<relay-cycle>` and call `ws/agents.call_async` for `implementer` using `Templates / Fix Prompt`.
29. After each implementer fix turn, call `ws/agents.wait` for `implementer` and update `<implementation-range>` through `HEAD`.
30. Re-run selected reviewers with `Templates / Re-Review Prompts`; reviewers overwrite their existing finding files.
31. At relay cycle 2, read maintained disputes from review files and decide whether to accept the implementer disposition or override it.
32. At relay cycle 3, stop relaying and collect unresolved findings for the completion report.
33. Delete `<correctness-path>`, `<fit-path>`, and `<test-path>` after their findings are no longer needed.
34. Call MCP tool `ws/agents.erase` for `implementer`, `reviewer-correctness`, `reviewer-fit`, and `reviewer-test`.
35. Output `Templates / Completion Report`.

## Judgments

### judge: plan-depth

Use `as-is` when the brief names concrete change points and the implementation is single-file or single-function. Use `survey` when the work spans multiple modules, likely reuse points are unconfirmed, or the implementer would be cold on the codebase. Use `research` when multiple viable strategies exist or cross-module side effects are non-obvious.

### judge: skeleton-gate

Proceed without a skeleton for small isolated implementation that does not create public contracts. Require `ws:write-skeleton` first when the work adds public interfaces, crosses module boundaries without an established pattern, or needs integration tests to lock the contract before implementation.

### judge: partition-allocation

Use correctness when logic, contracts, errors, security, or data behavior changed. Use fit when the change reuses or extends existing architecture, introduces patterns others may follow, or affects future phase headroom. Use test when tests changed or new code paths lack obvious existing coverage. Use all three for non-trivial feature work. Use correctness only for purely mechanical changes.

### judge: relay-outcome

Treat `[clean]` from every selected reviewer as complete. Relay `[non-clean]` correctness, security, contract, or test-validity findings unless the lead proves they are outside the brief. Accept fit-only won't-fix dispositions when they cite established local patterns or explicit brief boundaries.

## Templates

### Project Survey Prompt

```markdown
Target:
<ticket path, ticket stem, or inline implementation target>

Return a `[Must]` and `[Maybe]` reference list for writing a brief.
Prioritize documents and source files the implementer must read before editing.
Do not implement.
```

### Brief

```markdown
# Brief: <stem-or-short-target>

## Intent
<what this achieves in one paragraph>

## Approach
- <implementation approach at ticket-level resolution>

## Constraints
- <must-hold condition>

## Out of scope
- <excluded behavior or follow-up>

## Details
<interface, data, migration, or contract details needed when no skeleton exists>

## References
<!-- Populated from the project-survey [Must]/[Maybe] output. -->
- `[Must]` `<path-or-stem>` - <why the implementer needs it>
- `[Maybe]` `<path-or-stem>` - <when to consult it>
```

### Plan Population Prompt

```markdown
Brief path: <brief-path>
Plan path: <plan-path>

Populate the plan path with a concise implementation plan derived from the brief.
Do not implement source changes.
```

### Implementer Prompt

```markdown
Brief path: <brief-path>
<if plan exists:> Plan path: <plan-path>

Read only the brief and optional plan. Do not read the ticket directly.

Acceptance criteria:
- Existing skeleton stubs and integration tests are binding when present.
- Verification targets: <commands or test targets>

Instructions:
- Implement on the current branch.
- Commit logical checkpoints with detailed `## AI Context`.
- Run the relevant verification before reporting completion.
- Report commit hashes, changed files, verification results, and unresolved risks.
- A later turn may send review finding file paths; read those files directly and respond with dispositions.
```

### Correctness Reviewer Prompt

```markdown
Diff range: <implementation-range>
Brief path: <brief-path>
Review path: <correctness-path>

Review correctness, contracts, error paths, security, and regressions.
Write complete findings to the review path.
Return only: [clean|non-clean]: <one-line summary>
```

### Fit Reviewer Prompt

```markdown
Diff range: <implementation-range>
Brief path: <brief-path>
<if ticket-driven:> Ticket path: <ticket-path>
Review path: <fit-path>

Review whether the implementation fits the brief, existing architecture, naming, reuse patterns, and future phase headroom.
You may consult the ticket only for architectural headroom checks.
Write complete findings to the review path.
Return only: [clean|non-clean]: <one-line summary>
```

### Test Reviewer Prompt

```markdown
Diff range: <implementation-range>
Brief path: <brief-path>
Review path: <test-path>

Review test validity, coverage, missing assertions, mock integrity, and whether verification supports the brief.
Write complete findings to the review path.
Return only: [clean|non-clean]: <one-line summary>
```

### Fix Prompt

```markdown
Review cycle: <relay-cycle>
Finding files:
- Correctness: <correctness-path>
- Fit: <fit-path>
- Test: <test-path>

Read the finding files directly.
For each finding, fix it or report one disposition:
- [fixed]
- [won't fix: <specific reason tied to codebase pattern or brief scope>]
- [deferred: <specific reason and required follow-up>]

Commit logical fixes and rerun relevant verification before reporting.
```

### Re-Review Prompts

```markdown
Re-review the updated implementation.

Diff range: <implementation-range>
Brief path: <brief-path>
Prior finding path: <partition-review-path>
Implementer dispositions: <short disposition list or path to implementer output>

Focus on whether prior findings were resolved and whether fixes introduced regressions.
For each implementer won't-fix item in your partition, respond `[accepted]` or `[maintained: <brief reason>]`.
Overwrite the review path with complete current findings.
Return only: [clean|non-clean]: <one-line summary>
```

### Completion Report

```text
Implementation complete.
Commit range: <start-commit>..HEAD
Brief: <brief-path>
Plan: <plan-path or none>
Test status: pass | fail | skipped
Review: clean | non-clean
Escalation: <unresolved findings after relay cap, or none>
Agents: erased | <remaining cleanup issue>
```

## Doctrine

Write-code optimizes for delegated implementation throughput within the caller's limited coordination budget: the lead serializes intent into a brief, delegates implementation and review through resumable agents, and keeps only summaries and unresolved decisions in active context. When a rule is ambiguous, apply whichever interpretation better preserves the caller's limited coordination budget during delegated implementation.
