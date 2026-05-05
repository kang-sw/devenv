---
title: ws agent dogfood timeout and tail context blowup
related:
  260503-epic-ws-agent-workflow-stability: parent area for named-agent runtime behavior
  260505-feat-ws-mcp-result-readiness-api: dogfood run that exposed blocking wait and tail output risks
completed: 2026-05-05
---

# ws agent dogfood timeout and tail context blowup

## Background

While dogfooding the delegated implementation path for
`260505-feat-ws-mcp-result-readiness-api`, two runtime surprises consumed lead
attention and context.

First, `ws/agents.wait(timeout_seconds: 600)` was intended to allow a ten-minute
agent wait, but the host tool call failed around 120 seconds before the runtime
timeout could matter. Long blocking waits are therefore unsafe when the outer
MCP client has a shorter call deadline.

Second, `ws/agents.tail(lines: 20)` and even smaller tails can return very large
payloads because a single stdout JSONL event may contain a large
`aggregated_output` field. In the dogfood run, tail output repeated long file
reads, grep results, patch failures, and test output, causing abrupt context
growth.

## Decisions

- Long-running result reads are mitigated by the bundled Codex MCP
  `tool_timeout_sec: 600` setting, but normal callers should still avoid pulling
  unbounded diagnostic streams into the model context.
- `agents.wait` now returns readiness metadata rather than final output, and
  `agents.result` is the single result consumption surface.
- Normal `agents.tail` should be context-bounded by default. Raw stream
  inspection remains available under `agents.debug.*`.

## Phases

### Phase 1: Host MCP timeout and readiness split

Resolve the mismatch between long ws waits and shorter outer MCP client
deadlines by separating readiness from result consumption and by configuring the
bundled Codex MCP server with a ten-minute tool timeout.

### Result (4a3fc6e) - 2026-05-05

Implemented by `260505-feat-ws-mcp-result-readiness-api` and the follow-up
plugin config hotfix. `agents.wait` now returns readiness metadata,
`agents.result` consumes output, subquery result collection uses `agents.result`,
and the bundled `.mcp.json` sets `tool_timeout_sec: 600`.

### Phase 2: Context-bounded normal tail

Make normal `agents.tail` safe to call during dogfooding by truncating or
summarizing large stream fields and long lines before they enter the response.
The implementation should preserve enough recent diagnostic signal to identify
what happened, include an explicit truncation marker when content is shortened,
and leave `agents.debug.*` outputs raw for deep inspection.

Success criteria:

- A large stdout or stderr JSONL event containing `aggregated_output` does not
  cause normal `agents.tail` to return the full field.
- Normal tail output includes a visible truncation marker when stream content is
  shortened.
- Debug tail/stdout/stderr/runtime/event tools remain raw.
- Tests cover the large-event case.

### Result - 2026-05-05

Normal `agents.tail` now sanitizes recent diagnostic sections before returning
them to callers. Large JSON fields such as `aggregated_output`, `stdout`,
`stderr`, `output`, `content`, and `text` are shortened with a visible
`ws-tail truncated` marker, and very long non-JSON lines have a fallback line
limit. `agents.debug.tail` and stream-specific debug tools remain raw, and tests
cover both runtime and MCP tool behavior.

## Notes

Spec linkage is deferred while the spec directory is archived for forge-spec
reconstruction; this ticket should be linked once the replacement ws MCP/runtime
spec is restored.
