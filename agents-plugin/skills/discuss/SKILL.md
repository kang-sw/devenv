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
- Do not declare design closure while top-level design domains remain open.

## On: Start Discussion

1. Identify the concrete topic, ticket, skill, spec, or decision under discussion.
2. Call MCP tool `ws/project_tree` for current project memory.
3. Read any named ticket, spec, skill, or convention document before making claims about it.
4. Define the active boundary: design discussion, ticket shaping, skill porting, or implementation planning.
5. Ask at most one blocking question when the boundary cannot be inferred safely.
6. Enter the discussion loop.

## On: Design Discuss

Use this handler when the topic concerns workflow semantics, interface naming,
runtime behavior, migration policy, failure handling, or caller-visible
contracts.

1. Apply `judge: design-domains` and create a top-level checklist.
2. Mark each domain as `open`, `closed`, `delegated-detail`, `blocked`, or `irrelevant`.
3. Question `open` domains actively with concrete alternatives and trade-offs.
4. When a domain is too broad, split it into subdomains and repeat this handler.
5. When the user delegates implementation detail, mark only that subdomain `delegated-detail`.
6. After each answer, update the checklist before asking the next question.
7. Ask one to five high-leverage questions per turn; prefer fewer when questions are difficult.
8. Do not move to capture or implementation until `judge: design-closure` is not `open`.

## On: Discussion Loop

1. Restate the current decision in one or two sentences when the thread shifts.
2. Compare viable options with concrete trade-offs.
3. Mark assumptions, verification gaps, and compatibility risks explicitly.
4. Separate settled decisions from candidates and rejected alternatives.
5. When a claim depends on code or docs not yet read, inspect the artifact or label the claim as unverified.
6. Use `On: Design Discuss` before closure when the topic is a design discussion.
7. When the user converges on a direction, summarize the capture target with `judge: capture-target`.
8. Continue until the user asks to implement, write a ticket, update docs, or stop.

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

### judge: design-domains

Start with these top-level domains and drop irrelevant ones: interface and naming; state and lifecycle; failure and recovery; compatibility and migration; security and permissions; host and platform variance; testing and verification; rollout and deprecation; documentation and ticket/spec impact; user ergonomics.

### judge: design-closure

Return `open` while any relevant top-level domain is unresolved. Return `closed` when all relevant domains are decided. Return `delegated-detail` only for domains the user explicitly delegates to implementation judgment. Return `blocked` for domains that need external verification before closure.

### judge: question-pressure

Ask aggressively when an unresolved domain changes user-facing behavior, workflow safety, or migration cost. Offer delegation when a domain is internal implementation detail. Stop questioning when the remaining domains are all `closed`, `delegated-detail`, `blocked`, or `irrelevant`.

## Templates

### Decision Summary

```markdown
Decision: <settled direction>
Rationale: <why this direction won>
Rejected: <important alternatives and why they were rejected>
Risks: <unresolved verification or compatibility risks>
Next: <ticket, spec, implementation, or no artifact>
```

### Design Closure Checklist

```markdown
Design closure:
- Interface and naming: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- State and lifecycle: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Failure and recovery: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Compatibility and migration: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Security and permissions: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Host and platform variance: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Testing and verification: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Rollout and deprecation: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- Documentation and ticket/spec impact: <open|closed|delegated-detail|blocked|irrelevant> - <note>
- User ergonomics: <open|closed|delegated-detail|blocked|irrelevant> - <note>

Questions:
- <question tied to one open domain>
```

## Doctrine

Discussion optimizes for the user's limited decision turns: each response should reduce ambiguity, expose real trade-offs, and preserve approved conclusions without forcing premature artifacts. When a rule is ambiguous, apply whichever interpretation better preserves the user's limited decision turns.
