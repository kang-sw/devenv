---
kind: print
---
Maximum-delegation posture for this session: delegate all project work and route — do not execute inline. The lead reads this playbook, writes delegate prompts, adjudicates results, asks the user for approval or judgment, and writes the final synthesis. No inline reads, searches, edits, tests, commits, or artifact writing to solve the task.

Choose the delegation type by whether the work needs this conversation's context:
- fork (inherits full conversation/project context) — context-dependent authoring and mutation whose correctness depends on prior conversation not yet captured in a written artifact: brief/plan/ticket/spec/doc/skill authoring, commits, and conversation-dependent code edits.
- fresh spawn (clean context) — stateless work (survey, search, grep, reads, command runs, fact lookups) and plan/instruction-following implementation where a written brief, plan, or spec fully specifies the work.

Treat a subagent as forked only when it inherits current conversation/project context and runs in isolated execution.
<!-- ws:override:PreferSubagentInvocationGuidance desc="harness-specific forked subagent invocation guidance" -->
<!-- ws:/override:PreferSubagentInvocationGuidance -->
For any host without injected invocation guidance, use only a mode with inherited current context and isolated execution; uncertainty means unavailable. If no forked mechanism exists at all, ask whether to suspend this posture and proceed inline; after approval, say so and continue under normal rules.

A fork prompt states its task, scope, target files, verification expectation, and requested return shape. Open it with a strongly delimited up-front declaration that the recipient is a forked executor and this posture is suspended for it — composed per dispatch, not a fixed template — and end with the exact line:

`**You are a forked agent. Execute all work directly — do not sub-delegate.**`
