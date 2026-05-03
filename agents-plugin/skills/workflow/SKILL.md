---
name: workflow
description: Load the current ws host-neutral workflow reference, including MCP tool notation and migration boundaries for agent orchestration primitives.
---

# Workflow

## Invariants

- Keep this skill's content active when writing or porting ws workflow skills.
- Re-invoke this skill after compaction when workflow primitive names matter.
- Treat this document as a migration reference, not as an operational parity claim.
- Do not name Claude PATH scripts as shared `agents-plugin` primitives.
- Do not invent host-qualified MCP names in shared skill text.
- Use `ws/<tool-name>` as the shared shorthand for MCP server `ws`, tool `<tool-name>`.
- Define the exact MCP server and tool separately when ambiguity would affect execution.
- Keep `ws:` reserved for plugin skill names such as `ws:write-ticket`.

## On: Invoke

1. Read this document as the session-resident reference for ws workflow notation.
2. Use `judge: mcp-reference-form` before adding MCP tool references to shared skill text.
3. Use `judge: migration-boundary` before porting any Claude orchestration primitive.

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

### judge: migration-boundary

Use existing MCP tools only when they are implemented by the current `ws-mcp`
runtime. Treat agent session tools, review-path allocation, message queues, and
runtime locks as planned surfaces until their tickets implement them. When a
skill needs those surfaces before implementation, state the required contract
instead of naming a Claude PATH script as the shared primitive.

## Doctrine

Workflow notation optimizes for the model's limited execution attention during
cross-host migration: references must be short enough to survive skill execution
while explicit enough to map to each host's actual tool display. When a rule is
ambiguous, apply whichever interpretation better preserves the model's limited
execution attention during cross-host migration.
