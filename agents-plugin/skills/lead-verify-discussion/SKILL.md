---
name: lead-verify-discussion
description: Run a compact discussion verification and validation checkpoint using scoped subqueries.
---

Use this skill as a small explicit checkpoint during an active discussion. Verify whether the discussion is resting on sound assumptions, whether the direction still matches the evidence in the project, whether proposed items already exist and can be reused or merged to avoid duplication, and whether any proposed structure or implementation shape looks hygienic from a maintainability perspective.

Use multiple `ws/subquery` calls or equivalent scoped project investigation when useful, then validate the discussion by synthesizing corrected assumptions, concrete observations, reuse opportunities, and hygiene findings. Report the revised premise set and steer the conversation toward the direction that now looks best supported; keep it lightweight and do not edit files.
