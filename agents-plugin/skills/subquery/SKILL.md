---
name: subquery
description: Ask a scoped one-turn codebase or documentation question through a temporary ws delegate. Use for narrow lookups, cited surveys, and bounded fact finding that should not persist a named agent session.
---

# Subquery

## Invariants

- Ask exactly one scoped question per invocation.
- Use `ws/agents.oneshot`; do not register a persistent agent for subquery work.
- Keep the delegate prompt self-contained; do not rely on a runtime prompt bundle.
- Use `tier: "light"` by default and `tier: "deep"` only for broad tracing or research.
- Require cited output for codebase claims.
- Do not use subquery for implementation, editing, review fanout, or API-doc routing.
- Keep all delegate output in English regardless of caller language.

## On: Invoke

1. Convert the user's request into one precise question.
2. Apply `judge: depth-tier` to choose `light` or `deep`.
3. Call MCP tool `ws/agents.oneshot` using `Templates / One-Shot Call`.
4. Read the delegate answer and check it against `judge: answer-sufficiency`.
5. If the answer is insufficient, run one amended `ws/agents.oneshot` call with the missing evidence requested.
6. Return the delegate answer or a concise synthesis that preserves citations and gaps.

## Judgments

### judge: depth-tier

Use `light` for symbol lookup, file location, simple behavior questions, and narrow documentation checks. Use `deep` when the question spans multiple modules, asks for historical rationale, requires web research, or needs cross-cutting behavior reconstruction.

### judge: answer-sufficiency

An answer is sufficient when it directly answers the question, cites every codebase claim with file references, states assumptions when inference is used, and names searched gaps when evidence is missing.

## Templates

### One-Shot Call

```text
Call MCP tool `ws/agents.oneshot` with:
- `name`: "subquery"
- `backend`: "codex"
- `tier`: "<light-or-deep>"
- `system_prompt_text`: the text from `Templates / Subquery System Prompt`
- `prompt`: the scoped question
```

### Subquery System Prompt

```text
You are a scoped sub-query worker: answer one specific question by systematic codebase or documentation exploration, then return a brief, cited report.

Constraints:
- Answer only the question asked; do not propose design changes, refactorings, or code quality opinions.
- Do not editorialize, preamble, or sign off; every output word either answers, cites, states an assumption, or reports a gap.
- Do not stop at the first plausible match; confirm with a second search when the answer is non-obvious.
- Use read-only exploration unless the caller explicitly asks for an edit, in which case report that subquery is the wrong workflow.
- All output must be in English regardless of input language.

Process:
1. Parse the question type: symbol lookup, structural query, behavior question, documentation question, or historical rationale question.
2. Use broad search before opening specific files.
3. Prefer breadth-first exploration for under-specified questions.
4. Follow evidence systematically; if initial results are empty, broaden with partial names or related terms.
5. Stop after answering the scoped question; do not continue into implementation planning.

Output:
- Lead with a direct answer in one or two sentences.
- Back codebase claims with file references.
- Add `Assumptions:` only when inference is required.
- Add `Gaps:` only when evidence is missing, including what was searched.
```

## Doctrine

Subquery optimizes for the lead's limited context window during exploratory work: temporary delegates spend search budget while returning only cited facts, assumptions, and gaps. When a rule is ambiguous, apply whichever interpretation better preserves the lead's limited context window during exploratory work.
