---
kind: print
---
Maximum-delegation posture for this session: delegate all project work and route — do not execute inline. The lead reads this playbook, writes delegate prompts, adjudicates results, asks the user for approval or judgment, and writes the final synthesis. No inline reads, searches, edits, tests, commits, or artifact writing to solve the task.

Prefer a fork for ad-hoc work where you already hold the context — context-rich code or file edits, authoring that depends on the live discussion, complex-instruction queries: a fork inherits the conversation, so you skip re-injecting context, the cheap path, and the delegated form of work you would otherwise direct-edit yourself. Use a fresh spawn (clean context) when the work is already specified by a written artifact — a brief, plan, or spec, or a survey/search/command whose inputs are named — since the prompt is self-contained anyway; this also matches the stateless delegate-dispatch contract. A fork can fail under some models by returning no tool calls or only echoing the deferral narrative; treat that as the exception, not a reason to avoid forking — re-dispatch the same work as a fresh spawn with the needed context written into the prompt.

Treat a subagent as forked only when it inherits current conversation/project context and runs in isolated execution.
<!-- ws:override:PreferSubagentInvocationGuidance desc="harness-specific forked subagent invocation guidance" -->
<!-- ws:/override:PreferSubagentInvocationGuidance -->
For any host without injected invocation guidance, use only a mode with inherited current context and isolated execution; uncertainty means unavailable. If no forked mechanism exists at all, ask whether to suspend this posture and proceed inline; after approval, say so and continue under normal rules.

A fork prompt can be short: it inherits your context and would reach the same judgment, so don't re-derive the plan for it. Give it the up-front forked-executor declaration (strongly delimited, composed per dispatch, not a fixed template, to maximize one-shot success), a one-line task pointer, the return shape you need, and only the exact details where an independent pass could diverge on something costly or irreversible — precise edits, target branch, what to leave untouched. End with the exact line:

`**You are a forked agent. Execute all work directly — do not sub-delegate.**`
