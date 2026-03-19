---
name: write-plan
description: Research the codebase and produce a self-contained implementation plan. Use before `/implement` for non-trivial changes.
argument-hint: [ticket-path or description]
---

# Write Plan

Target: $ARGUMENTS

## Goal

Produce a **self-contained plan** that survives context reset. The plan must
carry enough context — decisions, conventions, file roles, domain constraints —
for a fresh `/implement` invocation to execute without re-researching.

## Step 0: Understand

1. Read the ticket/description.
2. Read `ai-docs/_index.md` for project state.
3. Read `ai-docs/mental-model/overview.md`, then every mental-model doc that
   touches the change area. Include adjacent domains — cross-module coupling
   lives there.
4. Run `git log --oneline -10` for recent work context.

## Step 1: Research

Adapt depth to the change. Pick the appropriate level:

| Level | When | What to do |
|-------|------|------------|
| **Minimal** | Config tweak, typo, single-file mechanical change | Mental-model docs only |
| **Moderate** | Feature addition following existing patterns, 2-3 files | + Read target files and adjacent code for patterns |
| **Thorough** | New component, cross-module, unfamiliar area | + Search for similar implementations, extract concrete convention examples |

When uncertain, go one level deeper — over-researching costs less than a wrong
plan.

Use subagents for broad codebase searches. Keep the main context for synthesis.

## Step 2: Draft Plan

Write the plan in this format:

```markdown
> Execute this plan by invoking `/implement`.

## Context
- Ticket summary and background
- Decisions made during discussion, with reasoning
- Rejected alternatives and why

## Relevant Files
- `path/to/file` — role in this change, key types/functions to touch

## Conventions (verified from code)
- Naming, structure, error handling patterns observed
- Concrete examples from existing code where helpful

## Implementation Steps
1. Concrete action (which file, what change, why)
2. ...

## Testing Strategy
- What to test, approach (unit vs integration), key scenarios

## Success Criteria
- Observable conditions that mean "done"
```

**Self-containedness check.** Before moving on, ask: "Could an agent with no
prior context execute this plan correctly?" If not, add what's missing.

## Step 3: Verify Plan

Dispatch a **sonnet-level general-purpose subagent** to verify the plan against
the actual codebase:

> **Task:** Verify the following implementation plan against the actual source
> code and mental-model docs.
>
> **Plan:**
> [full plan text]
>
> **Check each item:**
> - Do referenced files, functions, and types actually exist?
> - Do described conventions match actual code patterns?
> - Are there missing considerations (error handling, edge cases, dependencies)?
> - Does the plan conflict with mental-model constraints or invariants?
> - Are implementation steps concrete enough to execute without ambiguity?
>
> **Be aggressive.** Flag anything suspicious — false positives are fine.
> Categorize as Critical / Important / Minor.

## Step 4: Triage & Revise

Read the verifier's report. For each flag:

- **Critical**: fix in the plan. These are factual errors.
- **Important**: assess whether the concern applies. Revise if it does.
- **Minor**: note if useful, skip if not.

The verifier is intentionally aggressive. Use your judgment on actual severity.

After revision, do a final read-through for coherence and completeness.

## Step 5: Finalize

Enter plan mode via `EnterPlanMode` with the finalized plan. The user will
invoke `/implement` in a fresh context to execute it.
