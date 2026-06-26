---
name: lead-tune
description: Use when the user wants to tune or customize how the ws workflow runs — prompt overrides, lead delegation posture/eagerness, mercenary-vs-native delegation, or model tiers. Fires on standing preferences such as "delegate less", "stop spawning so many agents", or "use a cheaper model", and proposes the matching tune.
---

# Workflow Tuning

Call `ws/playbook.print(name: "lead-tune")` and execute the returned procedure
inline against the user request.
