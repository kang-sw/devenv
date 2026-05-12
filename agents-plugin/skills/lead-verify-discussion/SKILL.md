---
name: lead-verify-discussion
description: Run a compact discussion verification checkpoint using scoped subqueries.
---

Use this skill as a small explicit checkpoint during an active discussion. Verify whether the discussion is resting on sound assumptions, whether the direction still matches the evidence in the project, and whether any proposed structure or implementation shape looks hygienic from a maintainability perspective.

Use `ws/subquery` or equivalent scoped project investigation for the checks, then report the result qualitatively: what looks supported, what looks risky or under-evidenced, and whether the discussion should continue, adjust direction, or stop for clarification. Keep it lightweight and do not edit files.
