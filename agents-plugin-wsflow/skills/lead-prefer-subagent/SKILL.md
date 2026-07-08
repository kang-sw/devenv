---
name: lead-prefer-subagent
description: Switch to maximum-delegation posture. Delegate all work — including reads, searches, and single-file edits — to subagents. Use when the lead should stay thin and route rather than execute.
---

# Prefer Subagent

Maximum-delegation posture for this session: delegate all payload execution; do not execute inline. The lead reads this playbook, chooses delegation strategy, writes delegate prompts, adjudicates results, asks the user for approval or judgment, and writes the final synthesis. Aside from reading this playbook, no inline reads, searches, edits, tests, commits, or artifact writing to solve the task.

Keep workflow state-machine ownership with the lead. The lead follows the active skill to select the execution payload, record workflow state, and delegate only that selected payload. An execution payload is the scoped work item the delegate must perform, including artifact paths, constraints, and stop condition.

Under this posture the lead cannot act inline, so route each task to one of two delegates. A standing role (implementer, reviewer, …) always takes a fresh spawn, never a fork — this is unconditional — and capture the conversation's decisions into its spec. (A fork cannot spawn the subagents a role needs.) For other work, ask whether correct execution depends on a decision, preference, or detail that was settled only in this conversation. If not, the task runs from named artifacts plus general constraints — use a fresh spawn. If so, use a fork; paraphrasing those decisions into the prompt, however short, does not make the task self-contained. When genuinely unsure, prefer the fresh spawn.

If a fork does not execute the task and instead reports delegation instructions back to the lead, treat that as a failed fork. Re-dispatch the smallest executable subset as a stateless fresh spawn with only the context that subset needs; if that also fails to execute, stop and report the delegation failure.

Treat a subagent as forked only when it inherits the current conversation context and executes separately from the lead without sharing the lead's active reasoning or control flow.

If your host provides a fork-style subagent that inherits the current conversation context, use that as the forked-delegation mechanism. Otherwise, use a fresh-context spawn primitive such as Codex's `spawn_agent`: call `spawn_agent(fork_context:true, message:<prompt>)`, omitting `agent_type`, `model`, and `reasoning_effort` for full-history forks unless the host permits them. If a typed fork is rejected, retry untyped with `fork_context:true`; do not satisfy this posture with `agent_type: explorer` or `agent_type: worker` unless `fork_context:true` is active.

If no forked mechanism exists, or if availability is uncertain, ask the user whether to suspend maximum-delegation posture. Only after explicit approval may the lead proceed inline under the session's ordinary workflow rules.

Keep the fork prompt concise, usually under 300 words, because it inherits context. Open with execution constraints, not identity framing: "From now on, work directly on this task in the current workspace. Do not fork, spawn a subagent, start another agent process, or delegate again. If direct work is impossible, report the blocker instead." Then state only the context needed to complete the task, permitted files/actions, constraints, and concrete stop condition. Require this exact return format: `Outcome: ...`; `Files changed: ... or none`; `Verification: command/result summary, or not applicable`; `Blockers: ... or none`; `Commit: <hash> or none`. Close with the same boundary in shorter form: "Again: no fork, no subagent spawn, no further delegation. Work directly or report the blocker."
