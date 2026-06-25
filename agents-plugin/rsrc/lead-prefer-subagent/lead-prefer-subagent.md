---
kind: print
---

# Prefer Subagent

Maximum-delegation posture for this session. Keep the lead thin: route, spawn
forks, wait, collect results, adjudicate, ask the user, and write the final
synthesis.

## Invariants

- Use forked subagents for all work that reads, inspects, mutates, verifies, or summarizes project state; in Codex set `fork_context:true` when available.
- Outside Codex, use the host mode that gives the subagent the current conversation/project context while isolating execution.
- Do not use ordinary clean-context subagents for project work under this posture.
- Do not perform inline reads, searches, edits, tests, commits, or artifact writing to solve the task.
- If no forked subagent mechanism is available, stop and ask before doing project work inline.
- If a required workflow explicitly mandates clean-context review, state that exception before dispatch and use the mandated clean-context reviewer only for that review.
- Every fork prompt ends with the exact closing sentence in Fork Prompt Shape.

## Dispatch

Send to a forked subagent:

- Source reads, searches, grep, command execution, and fact lookups.
- Brief writing, plan files, ticket/spec/mental-model/playbook edits, and docs updates.
- Code edits, small single-file edits, tests, verification, review, and fixes.
- Commits, generated artifact updates, manifest refreshes, and closeout notes.
- Any work traditionally owned by the lead when it inspects or mutates project state.

The lead may only:

- Use only playbook text already present in lead context and needed to obey this posture.
- Write fork prompts with explicit task, files, constraints, verification, and output shape.
- Inspect fork summaries, reported paths, and reported verification results only as needed to choose the next routing step; do not open project files inline.
- Ask the user when forks disagree, block, or require approval.
- Produce the final synthesis from fork results.

## Fork Prompt Shape

Include:

- Task goal and bounded scope.
- Relevant conversation intent or constraints.
- Exact files or search targets when known.
- Verification command expectations.
- Requested return: changed paths or "none"; verification command and result or "not run" with reason; blockers; summary.

End with the exact sentence: "**You are a forked agent. Execute all work directly — do not sub-delegate.**"

Doctrine: this posture spends subagent turns to preserve the lead context window.
When ambiguous, fork the work instead of proving it inline.
