---
name: workflow
description: Load the ws workflow reference for host-neutral MCP notation and orchestration primitive boundaries.
---

# Workflow

> **Session invariant:** Keep this reference active while writing or executing ws
> workflow skills. After compaction, re-invoke `ws:workflow` if primitive names
> or orchestration boundaries matter.

## On: invoke

No action is required. Reading this file is the invocation; it loads the ws
workflow primitive reference into session context.

---

# WS Workflow Primitives

This is the host-neutral reference for ws plugin skill text. Use the compact form
`ws/<tool-name>` for MCP server `ws`, tool `<tool-name>`. Use `ws:` only for
plugin skills such as `ws:write-ticket`; do not use it for MCP tools.

Use pseudo-call notation when writing MCP calls in skills: `ws/tool.name(arg:
value)`. Show required arguments inline. Show optional arguments only when the
skill depends on a non-default value. Omit `root` when the current repository
root is intended. Prefer `prompt: <block below>` or `question: <block below>` for
large text payloads.

When writing shared skill text, name only primitives that exist in the ws runtime.
If a workflow needs a surface that is still planned, state the required MCP
contract instead of naming a Claude helper command or another host-specific
fallback.

## Available

### One-turn query

`ws/subquery`

Use for scoped fact-finding, surveys, and one-turn answers where no future resume
is needed. Set `deep_research: true` only for broad tracing.

### Persistent agents

`ws/agents.register`
`ws/agents.call`
`ws/agents.wait`
`ws/agents.status`
`ws/agents.tail`
`ws/agents.print`
`ws/agents.cancel`
`ws/agents.erase`

Register a stable task name with prompt stems or a self-contained system prompt,
then call it for each turn that needs continuity. `ws/agents.call` starts the
turn asynchronously and returns promptly. Use `wait` when final output is needed,
`status` to decide whether to wait or continue, `tail(lines: 3)` only for small
diagnostic snapshots, `print` for the last plain-text output, `cancel` only when
stopping the current task is more valuable than backend continuity, and `erase`
when the task-scoped session is no longer needed.

### Artifact paths

`ws/path.generate`

Use for generated workflow artifact paths, currently review files. Capture the
returned paths and pass file paths between lead, implementer, and reviewers
instead of copying large findings through the lead context.

### Runtime metadata

`ws/runtime.info`

Use for runtime compatibility checks and feature detection.

### API documentation

`ws/api.list`
`ws/api.ask`

Use `ws/api.list` to inspect available third-party API documentation domains and
`ws/api.ask(prompt: ..., domain_hint: ...)` to ask API documentation questions.
The runtime handles pre-routing, per-domain manager sessions, stale checks, and
cache access; shared skills should keep callers on the MCP tool surface instead
of local implementation details.

## Planned Or Specialized

Active-agent listing and broad message-queue semantics should be treated as
contract surfaces unless the current runtime exposes the exact MCP tool needed
by the skill. Basic async cancellation exists through `ws/agents.cancel`; do not
generalize that into a richer interrupt contract without checking the runtime.

## Usage Pattern

```text
One-turn survey:
call `ws/subquery(question: "<exact scoped question>")`.
call `ws/subquery(deep_research: true, question: <block below>)` only for broad tracing.

Persistent task:
call `ws/agents.register(name: "<agent-name>", prompts: ["<prompt-stem>"])`.
call `ws/agents.call(name: "<agent-name>", prompt: <block below>)`.
wait, inspect status, tail with `lines: 3`, or print output as needed.
erase the task-scoped agent when cleanup matters.

Review artifacts:
call `ws/path.generate(kind: "review", stems: ["<stem>"])`.
tell reviewers to write full findings to those paths.
relay file paths, not full findings, to the implementer.

API docs:
call `ws/api.list()` when choosing among cached domains matters.
call `ws/api.ask(prompt: "<API documentation question>", domain_hint: "<optional-domain>")`.
```

## Doctrine

Workflow notation optimizes for the model's limited execution attention during
cross-host execution: references must be short enough to survive skill execution
while explicit enough to map to each host's actual tool display. When a rule is
ambiguous, apply whichever interpretation better preserves that execution
attention.
