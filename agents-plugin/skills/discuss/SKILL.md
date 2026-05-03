---
name: discuss
description: Explore a workflow design, migration direction, ticket scope, or implementation approach before making code changes. Use when the user wants to reason through options, risks, or next steps rather than immediately edit files.
---

# Discuss

## Invariants

- Do not edit source files during discussion unless the user explicitly switches to implementation.
- Write documentation only when the user explicitly asks to capture or update the outcome.
- Treat unresolved risks as open questions instead of smoothing them into agreement.
- Surface existing patterns before proposing new abstractions.
- Keep Claude-specific commands and Codex-specific tooling out of shared conclusions.
- Use ws MCP tools when the discussion needs project memory or convention text.
- Keep all AI-authored captured artifacts in English.
- Do not read convention files from host-local plugin source paths.

## On: Start Discussion

1. Identify the concrete topic, ticket, skill, spec, or decision under discussion.
2. Call MCP tool `ws/project_tree` for current project memory.
3. Read any named ticket, spec, skill, or convention document before making claims about it.
4. Define the active boundary: design discussion, ticket shaping, skill porting, or implementation planning.
5. Ask at most one blocking question when the boundary cannot be inferred safely.
6. Enter the discussion loop.

## On: Discussion Loop

1. Restate the current decision in one or two sentences when the thread shifts.
2. Compare viable options with concrete trade-offs.
3. Mark assumptions, verification gaps, and compatibility risks explicitly.
4. Separate settled decisions from candidates and rejected alternatives.
5. When a claim depends on code or docs not yet read, inspect the artifact or label the claim as unverified.
6. When the user converges on a direction, summarize the capture target with `judge: capture-target`.
7. Continue until the user asks to implement, write a ticket, update docs, or stop.

## On: Capture Outcome

1. Choose the durable artifact with `judge: capture-target`.
2. Capture only decisions the user approved or clearly accepted.
3. Preserve rejected alternatives when they explain why the chosen direction matters.
4. Record remaining risks and follow-up questions separately from settled decisions.
5. If the capture target is a ticket, use the `write-ticket` skill.
6. If the capture target is a spec or convention, call `ws/convention.read` for the relevant convention before editing.
7. Commit only the captured documentation changes.

## Judgments

### judge: capture-target

Use a ticket when the outcome is scoped future work. Use a spec when the outcome changes caller-visible behavior. Use `ai-docs/_index.md` only for short-lived project memory needed at session start. Use a convention document when the outcome changes repeatable workflow rules.

### judge: needs-artifact

Create or edit an artifact only when the discussion produced a durable decision, actionable work item, or rule change. Do not create artifacts for unresolved brainstorming unless the user explicitly asks for a research note.

### judge: tooling-boundary

If an option requires named agents, hooks, or host-specific plugin behavior that is not part of the ws MCP contract, mark it as adapter work and keep it out of shared behavior.

## Templates

### Decision Summary

```markdown
Decision: <settled direction>
Rationale: <why this direction won>
Rejected: <important alternatives and why they were rejected>
Risks: <unresolved verification or compatibility risks>
Next: <ticket, spec, implementation, or no artifact>
```

## Doctrine

Discussion optimizes for the user's limited decision turns: each response should reduce ambiguity, expose real trade-offs, and preserve approved conclusions without forcing premature artifacts. When a rule is ambiguous, apply whichever interpretation better preserves the user's limited decision turns.
