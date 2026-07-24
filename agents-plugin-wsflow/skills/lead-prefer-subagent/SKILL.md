---
name: lead-prefer-subagent
description: Switch to maximum-delegation posture. Delegate work — reads, searches, single-file edits — to subagents by default, keeping only durable-artifact authoring with the context-holder session. Use when the lead should stay thin and route rather than execute.
---

# Prefer Subagent

Maximum-delegation posture for this session: delegate all payload execution to a fresh subagent by default. The lead reads this playbook, chooses delegation strategy, writes delegate prompts, adjudicates results, asks the user for approval or judgment, and writes the final synthesis. The sole carve-out to full delegation: authoring or mutating a durable artifact (ticket, spec) stays with whichever session already holds the authoritative context for that decision — see the whitelist below. Outside that carve-out, no inline reads, searches, edits, tests, commits, or artifact writing to solve the task.

Keep workflow state-machine ownership with the lead. The lead follows the active skill to select the execution payload, record workflow state, and delegate only that selected payload. An execution payload is the scoped work item the delegate must perform, including artifact paths, constraints, and stop condition.

Route every delegated task to a fresh spawn built from named artifacts plus general constraints, never from a copy of this conversation. A standing role (implementer, reviewer, …) always takes a fresh spawn — this is unconditional — and captures the conversation's decisions into its spec so the fresh spawn stays self-contained.

Central authoring/mutation whitelist (owned here, not by individual skills): durable-artifact authoring or mutation stays with the session that already holds the authoritative context for the decision — lead-inline when the decision was settled in this conversation, or the delegated subagent's own continuing session when it was settled there. Never hand a durable artifact's authoring to a separate fresh spawn working only from an after-the-fact summary of a decision it did not make; a summary loses the reasoning a correct write depends on.

Keep the delegate prompt concise, usually under 300 words, since a fresh spawn starts with no other context. State the task, the artifacts and constraints it needs, permitted files/actions, and a concrete stop condition. Require this exact return format: `Outcome: ...`; `Files changed: ... or none`; `Verification: command/result summary, or not applicable`; `Blockers: ... or none`; `Commit: <hash> or none`.
