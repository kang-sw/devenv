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
minimum agent session tools `ws/agents.register`, `ws/agents.call`,
`ws/agents.call_async`, `ws/agents.oneshot`, `ws/agents.print`, and
`ws/agents.erase` are available. The async inspection tools `ws/agents.wait`,
`ws/agents.status`, `ws/agents.tail`, and `ws/agents.cancel` are available for
the current named agent call. Treat interrupts, active-agent listing,
review-path allocation, message queues, and runtime locks as planned surfaces
until the runtime implements them.
When a skill needs a planned surface, state the required server/tool contract
instead of naming a host-specific helper command as the shared primitive.

## Doctrine

Workflow notation optimizes for the model's limited execution attention during
cross-host execution: references must be short enough to survive skill execution
while explicit enough to map to each host's actual tool display. When a rule is
ambiguous, apply whichever interpretation better preserves the model's limited
execution attention during cross-host execution.
