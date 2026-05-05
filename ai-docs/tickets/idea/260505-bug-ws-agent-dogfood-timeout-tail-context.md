---
title: ws agent dogfood timeout and tail context blowup
related:
  260503-epic-ws-agent-workflow-stability: parent area for named-agent runtime behavior
  260505-feat-ws-mcp-result-readiness-api: dogfood run that exposed blocking wait and tail output risks
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

## Fix Ideas

- Make workflow guidance prefer short polling with `agents.status` / bounded
  readiness checks over long blocking `agents.wait` calls when the host may
  impose a shorter tool-call timeout.
- Consider making `agents.wait` return promptly with readiness metadata instead
  of holding the MCP request for long-running agent work.
- Truncate or summarize large stdout/stderr JSONL `aggregated_output` fields in
  normal `agents.tail`.
- Keep raw unbounded stream inspection under `agents.debug.*` tools only.
- Add tests or smokes that cover a large stdout event and verify normal tail
  output stays context-bounded.

## Notes

This ticket captures dogfood failure evidence only. It should be promoted after
the current result/readiness API implementation is either completed or paused,
so the fix can align with the final `agents.wait` / `agents.result` contract.
