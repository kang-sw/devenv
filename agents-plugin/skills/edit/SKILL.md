---
name: edit
description: Directly implement a narrow code change on the current branch, then verify it with one correctness-and-fit reviewer before updating specs and reporting completion.
---

# Edit

## Invariants

- The lead performs every source edit directly; delegates may review but never implement.
- Keep the scope narrow enough for one lead-owned change and one reviewer session.
- Escalate to the configured implementation workflow when scope becomes broad or cross-module without an established pattern.
- Load relevant mental-model documents before editing files they describe.
- Treat existing skeleton stubs and integration tests as binding acceptance criteria.
- Commit logical units with `git commit -F` and a detailed `## AI Context`.
- Run verification before review and after every review-driven fix.
- Cap reviewer relay at two non-clean cycles.
- Delete generated review files before returning.
- Call the `ws:update-spec` skill with the edit commit range before the completion report.
- End with the completion report format in `Templates / Completion Report`.

## On: Edit

1. Parse the request as either a ticket path, ticket stem, or inline brief.
2. Record `<start-commit>` with `git rev-parse HEAD`.
3. If ticket-driven, read the ticket and collect any skeleton references from frontmatter.
4. Apply `judge: skeleton-gate`; stop and suggest `ws:write-skeleton` when skeleton work is required first.
5. Identify target paths and call MCP tool `ws/mental_models.list` with those paths.
6. Read every listed mental-model document, loading parent domain indexes before child documents.
7. Call MCP tool `ws/infra.read` for `impl-playbook` when implementation or verification policy is uncertain.
8. Identify the build, syntax, or test command that proves the requested change.
9. Edit files directly, keeping each change within the request and existing project patterns.
10. Commit completed logical units with a detailed `## AI Context`.
11. Run the selected verification command and read enough output to defend the result.
12. Diagnose failures before editing tests; only change tests when the expected behavior or skeleton contract changed.
13. Call MCP tool `ws/path.generate` with `kind: "review"` and `stems: ["direct"]`; store the returned path as `<review-path>`.
14. Call MCP tool `ws/agents.register` with `name: "reviewer"`, `backend: "codex"`, `tier: "core"`, and `prompts: ["code-reviewer", "code-review-correctness", "code-review-fit"]`.
15. Call MCP tool `ws/agents.call_async` for `reviewer` using `Templates / Reviewer Prompt`.
16. Call MCP tool `ws/agents.wait` for `reviewer` when the reviewer result is needed.
17. Call MCP tool `ws/agents.print` for `reviewer` if the wait result does not include a usable summary.
18. If the reviewer reports clean, proceed to cleanup.
19. If the reviewer reports non-clean, read `<review-path>`, apply required fixes, verify again, and call `ws/agents.call_async` with `Templates / Re-Review Prompt`.
20. Repeat the relay until the reviewer reports clean or two non-clean cycles have completed.
21. Call MCP tool `ws/agents.erase` for `reviewer`.
22. Delete `<review-path>` after its findings are no longer needed.
23. Call the `ws:update-spec` skill with `<start-commit>..HEAD`.
24. Output `Templates / Completion Report`.

## Judgments

### judge: skeleton-gate

Proceed without a skeleton for a small isolated edit that does not create or change public contracts. Require `ws:write-skeleton` first when the change adds public interfaces, crosses module boundaries without an established pattern, or depends on tests that should lock a contract before implementation.

### judge: escalation

Stay in `edit` when the lead can hold the whole change in context, the verification target is clear, and one reviewer can cover correctness and fit. Escalate to the configured implementation workflow when the work needs a delegated implementer, multiple independent reviewers, branch routing, or a multi-step plan that would outgrow one direct-edit loop.

## Templates

### Reviewer Prompt

```markdown
Diff range: <start-commit>..HEAD
Scope: direct edit - <brief scope description>
Review path: <review-path>

Review for correctness and fit.
Write complete findings to the review path.
Return only: [clean|non-clean]: <one-line summary>
```

### Re-Review Prompt

```markdown
Re-review the updated direct edit.

Diff range: <start-commit>..HEAD
Review path: <review-path>

Focus on prior findings and any new regressions introduced by the fixes.
Write complete findings to the review path.
Return only: [clean|non-clean]: <one-line summary>
```

### Completion Report

```text
Edit complete.
Commit range: <start-commit>..HEAD
Test status: pass | fail | skipped
Review: clean | non-clean (<one-line summary>)
Spec: <N entries added, M planned markers stripped> | no changes
Open issues: <remaining issues after relay cap, or none>
```

## Doctrine

Edit optimizes for the lead's limited active implementation context during narrow code changes: the lead keeps source ownership, the reviewer spends independent review attention, and the relay cap prevents review negotiation from consuming the session. When a rule is ambiguous, apply whichever interpretation better preserves the lead's limited active implementation context during narrow code changes.
