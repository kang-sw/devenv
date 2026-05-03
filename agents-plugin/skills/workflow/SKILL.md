---
name: workflow
description: Load the ws workflow reference for host-neutral MCP notation and orchestration primitive boundaries.
---

# Workflow

## Invariants

- Keep this skill's content active when writing or executing ws workflow skills.
- Re-invoke this skill after compaction when workflow primitive names matter.
- Treat available and planned workflow primitives as separate surfaces.
- Do not name host-specific helper commands as shared workflow primitives.
- Do not invent host-qualified MCP names in shared skill text.
- Use `ws/<tool-name>` as the shared shorthand for MCP server `ws`, tool `<tool-name>`.
- Define the exact MCP server and tool separately when ambiguity would affect execution.
- Keep `ws:` reserved for plugin skill names such as `ws:write-ticket`.

## On: Invoke

1. Read this document as the session-resident reference for ws workflow notation.
2. Use `judge: mcp-reference-form` before adding MCP tool references to shared skill text.
3. Use `judge: primitive-availability` before naming an orchestration primitive.
4. Use `judge: delegate-pattern` before selecting an agent orchestration shape.
5. Apply `judge: specialized-workflow-gap` when a workflow is not expressible with available primitives.

## Judgments

### judge: mcp-reference-form

Use `ws/<tool-name>` when a shared skill needs a compact MCP reference. Expand it
as "MCP server `ws`, tool `<tool-name>`" when teaching, debugging, or writing a
contract. Do not use `ws:<tool-name>` because `ws:` is the plugin skill prefix.
Do not assume the shorthand is the literal host-qualified tool name.

Examples:

- `ws/convention.read` means MCP server `ws`, tool `convention.read`.
- `ws/agents.call` means MCP server `ws`, tool `agents.call`.
- `ws/project_tree` means MCP server `ws`, tool `project_tree`.

### judge: primitive-availability

Use MCP tools only when they are available in the current `ws` runtime. The
minimum delegate tools `ws/subquery`, `ws/agents.register`, `ws/agents.call`,
`ws/agents.call_async`, `ws/agents.oneshot`, `ws/agents.print`, and
`ws/agents.erase` are available. The async inspection tools `ws/agents.wait`,
`ws/agents.status`, `ws/agents.tail`, and `ws/agents.cancel` are available for
the current named agent call. Treat interrupts, active-agent listing,
review-path allocation, message queues, and runtime locks as planned surfaces
until the runtime implements them.
When a skill needs a planned surface, state the required server/tool contract
instead of naming a host-specific helper command as the shared primitive.

### judge: delegate-pattern

Use `ws/agents.oneshot` for one-turn fact finding, scoped surveys, and subquery-style
answers where no future resume is needed. Use `ws/agents.register` plus
`ws/agents.call` when a named task agent needs conversational continuity and each
turn should complete before the lead proceeds. Use `ws/agents.register` plus
`ws/agents.call_async` when the delegate may run long enough that the lead needs
control back before completion. Use `ws/agents.wait` to collect final async
output, `ws/agents.status` to decide whether to wait or continue, `ws/agents.tail`
to inspect evidence and diagnostics, and `ws/agents.cancel` only when stopping the
current task is more valuable than preserving backend continuity.

### judge: specialized-workflow-gap

Treat `ws/subquery` as an available MCP tool composed over `ws/agents.oneshot`.
Treat API documentation routing as planned until the runtime provides the
pre-router, domain agent, stale-check, fetch, and lock contracts. Treat
interrupts, active agent listing, review-path allocation, message queues, and
runtime locks as planned even though basic async cancellation is available.

## Templates

### One-Shot Delegate

```text
Call MCP tool `ws/agents.oneshot` with:
- `root`: current repository root when the host does not supply it automatically
- `name`: optional descriptive temporary name
- `backend`: `codex`
- `tier`: `light` for narrow lookup, `core` for normal survey, `deep` for broad tracing
- `system_prompt_text`: self-contained delegate instructions
- `prompt`: the exact scoped question or task
```

### Persistent Synchronous Delegate

```text
1. Call MCP tool `ws/agents.register` with a stable task name and self-contained `system_prompt_text`.
2. Call MCP tool `ws/agents.call` for each turn that must complete before the lead proceeds.
3. Call MCP tool `ws/agents.print` when the latest output must be recovered.
4. Call MCP tool `ws/agents.erase` after the task-scoped session is no longer needed.
```

### Persistent Asynchronous Delegate

```text
1. Call MCP tool `ws/agents.register` with a stable task name and self-contained `system_prompt_text`.
2. Call MCP tool `ws/agents.call_async` for a long-running delegate turn.
3. Call MCP tool `ws/agents.status` to inspect current state without blocking.
4. Call MCP tool `ws/agents.tail` to inspect recent events, stdout, stderr, and output.
5. Call MCP tool `ws/agents.wait` with a bounded timeout when final output is needed.
6. Call MCP tool `ws/agents.cancel` only when the current async task should be stopped.
7. Call MCP tool `ws/agents.erase` after the task-scoped session is no longer needed.
```

## Doctrine

Workflow notation optimizes for the model's limited execution attention during
cross-host execution: references must be short enough to survive skill execution
while explicit enough to map to each host's actual tool display. When a rule is
ambiguous, apply whichever interpretation better preserves the model's limited
execution attention during cross-host execution.
