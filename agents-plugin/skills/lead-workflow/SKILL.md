---
name: lead-workflow
description: Load before writing or executing ws workflow skills; keeps host-neutral MCP notation and orchestration primitive boundaries in context.
---

# Workflow

> **Session invariant:** Keep this reference active while writing or executing ws
> workflow skills. After compaction, re-invoke `ws:lead-workflow` when primitive
> names or orchestration boundaries matter.

## On: invoke

Reading this file is the invocation; it loads the primitive reference.

---

# WS Workflow Primitives

Host-neutral notation reference for ws plugin skill text.

Use `ws/<tool-name>` for MCP server `ws`, tool `<tool-name>`.
Use `ws:` only for plugin skills such as `ws:lead-write-ticket`.
Write MCP calls as `ws/tool.name(arg: value)`.
Show optional arguments only when the skill needs a non-default value.
Omit `root` when the current repository root is intended.
Use `prompt: <block below>` or `question: <block below>` for large text payloads.

When writing shared skill text, name only primitives that exist in the ws runtime.
If a workflow needs a surface that is still planned, state the required MCP
contract instead of naming a Claude helper command or another host-specific
fallback.

## How To Document

- Write compressed professional prose: short sentences, exact verbs, no filler.
- Prefer `Do X through Y` over `Do not do X` when a positive action exists.
- Use `Not:` / `Use:` examples only for recurring mistakes.
- Keep procedure text command-shaped; move rationale to one short Doctrine paragraph.
- Use full sentences when compression could blur order, ownership, or safety.

## Available

### One-turn query

`ws/subquery`

Use for scoped fact-finding, surveys, and one-turn answers. Set
`deep_research: true` only for broad tracing.

### Persistent agents

`ws/agents.register`
`ws/agents.call`
`ws/agents.wait`
`ws/agents.status`
`ws/agents.tail`
`ws/agents.print`
`ws/agents.cancel`
`ws/agents.erase`

Register a stable task name with prompt stems or a self-contained system prompt.
Call it for each continuity turn. `ws/agents.call` starts async and returns
promptly. Use `wait` for final output, `status` before waiting, `tail(lines: 3)`
for small diagnostics, `print` for last output, `cancel` to stop active work,
and `erase` when task-scoped state should be removed.

### Artifact paths

`ws/path.generate`

Use for generated workflow artifact paths. Capture returned paths. Relay paths,
not large findings, between lead, implementer, and reviewers.

### Runtime metadata

`ws/runtime.info`

Use for runtime compatibility checks and feature detection.

### Git

`ws/git.status`
`ws/git.diff`
`ws/git.log`
`ws/git.merge_base`
`ws/git.commit`

Use `ws/git.commit` for workflow commits when available. It stages explicit
paths, builds the `## AI Context` message, detects ticket moves and `### Result`
headings, and avoids shell quoting drift.

### API documentation

`ws/api.list`
`ws/api.ask`

Use `ws/api.ask(prompt: "<prose question>")` for external API documentation.
Pass the natural-language question directly. Use `ws/api.list` to inspect cached domains.
Use `domain_hint` only when the intended domain is known.
The runtime owns pre-routing, per-domain sessions, stale checks, and cache access.

## Planned Or Specialized

Treat active-agent listing and broad message-queue semantics as planned contract
surfaces unless the runtime exposes the exact tool. Basic async cancellation
exists through `ws/agents.cancel`; check runtime before assuming richer
interrupt behavior.

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
call `ws/api.ask(prompt: "<prose API documentation question>")` for external API lookup.
add `domain_hint: "<optional-domain>"` only when the intended domain is known.

Commit:
call `ws/git.commit(paths: ["<path>"], title: "<title>", ai_context: ["<bullet>"])`.
```

## Doctrine

Workflow notation optimizes for **limited execution attention** during cross-host
work. References must survive skill execution and map to each host's tool
display. When ambiguous, preserve execution attention.
