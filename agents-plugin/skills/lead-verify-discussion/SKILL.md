---
name: lead-verify-discussion
description: Run an explicit lightweight verification checkpoint during discussion; use when the user asks to verify the current discussion's assumptions, premise quality, code-hygiene fit, or structural fit with scoped subqueries.
---

# Verify Discussion

Target: current discussion

## Invariants

- Keep the check lightweight; verify discussion premises, not implementation.
- Use `ws/subquery` for each independent verification question.
- Ask about code hygiene only when the discussion touches source structure, module boundaries, maintainability, or implementation shape.
- Report findings as evidence, risks, and next discussion stance.
- Leave files unchanged.

## On: invoke

1. Restate the claims, assumptions, or structure choices being checked.
2. Call `ws/subquery(question: "<premise verification question>")`.
3. If code hygiene applies, call `ws/subquery(question: "<structure and maintainability question>")`.
4. Read each result with `ws/agents.result(name: "<subquery-key>", timeout_seconds: 600)`.
5. Return the verification report.

## Templates

### Verification report

```text
Verification:
- Premises: <supported | risky | unsupported> - <evidence>
- Code hygiene: <fit | concern | not applicable> - <evidence>
- Stance: <continue | adjust | stop and clarify>
```

## Doctrine

This skill optimizes for **explicit verification checkpoints**. Discussion
context decays under attention pressure; a small callable checkpoint refreshes
premises and structure fit before the conversation commits to a direction.
