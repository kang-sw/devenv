---
kind: print
---

# Prefer Subagent

Maximum-delegation posture. Applies from invocation until the user changes it or
the session ends. Keep the lead thin: route, spawn forks, wait, collect results,
adjudicate, ask the user, and write the final synthesis.

## Invariants

- Use forked subagents for all work that reads, inspects, mutates, verifies, or summarizes project state.
- Treat a subagent as forked only when it inherits current conversation/project context and runs in isolated execution.
<!-- ws:override:PreferSubagentInvocationGuidance desc="harness-specific forked subagent invocation guidance" -->
<!-- ws:/override:PreferSubagentInvocationGuidance -->
- For any host without injected invocation guidance, use only a mode with inherited current context and isolated execution; uncertainty means unavailable.
- Do not use ordinary clean-context subagents for project work under this posture.
- Do not perform inline reads, searches, edits, tests, commits, or artifact writing to solve the task.
- If no forked subagent mechanism is available, ask whether to proceed inline; after explicit approval, state that this posture cannot be fully applied.
- If a required workflow explicitly mandates clean-context review, state that exception before dispatch and use the mandated clean-context reviewer only for that review.
- End every fork prompt with the exact Markdown line in Fork Prompt Shape.

## Dispatch

Send to a forked subagent:

- Source reads, searches, grep, command execution, and fact lookups.
- Repository/workflow artifact writing: brief files, plans, tickets, specs, mental models, playbooks, and docs updates.
- Code edits, small single-file edits, tests, verification, review, and fixes.
- Commits, generated artifact updates, manifest refreshes, and closeout notes.
- Any work traditionally owned by the lead when it inspects or mutates project state.

The lead may only:

- Rely on this playbook text once loaded; do not open additional project files or workflow docs inline to solve the task.
- Write fork prompts with explicit task, files, constraints, verification, and output shape.
- Inspect fork summaries, reported paths, and reported verification results only as needed to route; if insufficient, dispatch a narrower verification/review fork.
- If a fork fails, returns partial output, or disagrees with another fork, dispatch a clarifying fork when progress is possible; ask the user only for approval or external judgment.
- Produce the final synthesis from fork results.
- Include outcome, changed paths or `none`, verification results or `not run` with reason, blockers, and user decisions needed.

## Fork Prompt Shape

Include:

- Task goal and bounded scope.
- Relevant conversation intent or constraints.
- Exact files or search targets when known.
- Verification command expectations.
- Requested return: changed paths or "none"; verification command and result or "not run" with reason; blockers; summary.

End with this exact Markdown line: `**You are a forked agent. Execute all work directly — do not sub-delegate.**`

Doctrine: this posture spends subagent turns to preserve the lead context window.
When ambiguous, fork the work instead of proving it inline.
