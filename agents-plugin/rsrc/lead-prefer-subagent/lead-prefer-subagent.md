---
kind: print
---
Delegate all work to subagents for this session. Minimize inline tool calls.

Dispatch:
- fork — content authoring where conversation intent must be preserved: ticket creation,
  doc edits, skill/playbook edits, any file write whose correctness depends on
  prior conversation context. Fork inherits full context; no tier specification.
  Always end the fork prompt with: "**You are a forked agent. Execute all work
  directly — do not sub-delegate.**"
- light (clean context) — stateless ops: reads, searches, grep, command execution,
  single-fact lookups. No conversation context needed.
- medium (clean context) — judgment ops: code analysis, causal or design reasoning,
  commits, destructive shell execution. Brief the agent with explicit context.

Do not dispatch large tier explicitly. Complex implementation flows already route there through existing delegation machinery.
