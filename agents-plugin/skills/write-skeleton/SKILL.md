---
name: write-skeleton
description: Crystallize ticket contracts as public interface stubs and integration tests before implementation. Use after a ticket exists and public contracts need to be locked before implementation.
---

# Write Skeleton

## Invariants

- Skeleton means the first code change for a ticket: public stubs plus integration tests only.
- Keep implementation logic out of skeletons except placeholder bodies required for compilation.
- The lead owns ticket interpretation, contract directives, review, ticket metadata, and commits.
- The skeleton delegate owns codebase exploration, API shape selection, stub writing, and build cleanup.
- Do not modify existing public interfaces unless the ticket explicitly mandates it.
- Register `skeleton-writer` once per invocation and resume it for amendment rounds.
- Keep all AI-authored ticket and commit text in English.
- Use `git commit -F` for multi-paragraph commit messages.

## On: Write Skeleton

1. Read the target ticket and identify the implementation phase that needs a skeleton.
2. Inspect only the code and docs needed to form contract directives.
3. Write 2-5 contract directives covering choices the delegate cannot derive from the ticket and codebase.
4. Call MCP tool `ws.agents.register` with `root`, `name: "skeleton-writer"`, `backend: "codex"`, `tier: "deep"`, `prompt_refs: ["skeleton-writer"]`, and `system_prompt_text` from `Templates / Skeleton Writer System Prompt`.
5. Call MCP tool `ws.agents.call` with `name: "skeleton-writer"` and a prompt using `Templates / Delegate Prompt`.
6. Review `git diff HEAD` and `git status --short`; read changed files where contract or build correctness is uncertain.
7. Run the project build or syntax check required for compilation; do not run tests that are expected to fail against stubs.
8. Apply `judge: review-outcome`; fix minor issues directly and send structural amendments through `ws.agents.call`.
9. Commit stubs and tests together with a `feat(<scope>): skeleton - <contract>` message.
10. Add `## AI Context` with the key contract decisions and delegation amendments.
11. Add `## Ticket Updates` with the ticket stem and future implementation notes.
12. Update the ticket `skeletons:` frontmatter for the implemented phase after the commit hash exists.
13. Call MCP tool `ws.agents.erase` for `skeleton-writer` after the skeleton commit and ticket metadata are complete.
14. Suggest the next execution path using `judge: next-step`.

## Judgments

### judge: review-outcome

Minor issues are local naming, formatting, imports, or obvious compile fixes. Structural issues are wrong API shape, wrong test layer, missing contract coverage, or directive violations.

### judge: test-scope

Always include structural seam tests for cross-module boundaries. Include behavioral tests for behavior stated by the ticket. Include error and edge tests only when the ticket states those contracts.

### judge: stub-granularity

Use module-level stubs for most tickets. Stub all public methods only when the ticket specifies a detailed method surface or the language requires complete interface implementation.

### judge: next-step

Suggest `ws:implement` or a split when the remaining work spans independent modules. Suggest `ws:edit` when the remaining work is narrow and the lead is warm on the files. Suggest `ws:proceed` when routing is ambiguous. Name a skill only when it is available in the current host; otherwise name the equivalent configured workflow.

## Templates

### Skeleton Writer System Prompt

```text
You are the skeleton-writer delegate for a ws workflow.

Rules:
- Do not create commits, tags, or branches.
- Leave all changes unstaged for lead review.
- Treat the lead's contract directives as hard constraints.
- Make all unspecified design choices from the ticket and codebase.
- Write public interface stubs and integration tests only.
- Use placeholder bodies for unimplemented functions or methods.
- Do not add private helpers or implementation logic.
- Do not modify existing public interfaces unless a directive requires it.
- Make the skeleton compile or pass syntax checks.
- Do not run tests that are expected to fail against unimplemented stubs.

Process:
1. Read the ticket path from the prompt.
2. Read relevant project docs and nearby code.
3. Design the public contract and integration-test shape.
4. Write the stubs and tests.
5. Run build or syntax checks and fix compilation errors.
6. Report changed files, contract decisions, verification, and any deviations.
```

### Delegate Prompt

```markdown
Ticket: <ticket-path>
Phase: <phase-id-or-name>

## Contract directives
- <binding decision>
- <binding decision>

## Output required
- Files created or modified
- Key contract decisions
- Build or syntax verification performed
- Deviations or unresolved questions
```

### Amendment Prompt

```markdown
Amend skeleton output:

## Issues
- <contract, build, or directive issue>

## Revised directives
- <binding revision>
```

## Doctrine

Write-skeleton optimizes for contract stability before implementation consumes the context window: the lead serializes only binding directives, the delegate spends the exploration budget, and the commit records the public surface. When a rule is ambiguous, apply whichever interpretation better preserves contract stability before implementation consumes the context window.
