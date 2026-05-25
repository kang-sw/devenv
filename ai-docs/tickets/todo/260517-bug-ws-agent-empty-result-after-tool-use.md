---
title: ws named agent empty result after long tool-use run
related:
  260517-bug-ws-dashboard-editor-scroll-ime-verification: dogfood run where implementer edited files but produced no final output
  260512-research-claude-cli-stream-json: adjacent Claude backend stream/result contract research
related-mental-model:
  - named-agent-runtime
---

# ws named agent empty result after long tool-use run

## Background

During the dashboard editor scroll and terminal input fidelity implementation on
2026-05-17, the `implementer` named agent using the Claude Opus backend edited
source files but did not return a usable final result.

The first call ran for more than 20 minutes, modified dashboard frontend files,
and left no `output.md`. Runtime events showed `result.timeout`; after
cancellation, the captured backend stdout JSON reported `subtype: "success"`,
`stop_reason: "tool_use"`, `terminal_reason: "hook_stopped"`, and an empty
`result`. A recovery call resumed the same session, modified additional files,
again produced no stdout/stderr/output, and was cancelled after another timeout.

This made the lead recover by inspecting the dirty worktree, validating the
partial changes directly, committing the source result, and continuing the
review/doc pipeline manually.

## Notes

- This did not look like a project test deadlock: no long-running terminal
  command output was visible through the agent diagnostics, and the worktree
  contained plausible source edits.
- The strongest current hypothesis is a Claude backend / ws agent integration
  result-capture gap when a long-running session ends or stalls in a tool-use
  state without emitting a final assistant message.
- Recovery should distinguish model/tool loops, backend stream-json final-event
  semantics, hook interruption behavior, and ws runtime handling of empty
  successful backend results.
- A useful fix would expose a clearer status such as `waiting-for-tool`,
  `backend-ended-with-tool-use`, or `empty-final-result`, and preserve enough
  recent tool/output diagnostics for the lead to recover without guessing.
- Dogfood note 2026-05-23: a fresh-reader named-agent audit tried to register
  and wait on a nested named agent instead of auditing the supplied text, then
  produced repeated `getcwd` errors and no final output until cancelled. This is
  another recovery case for agent tool-use loops that do not yield a usable
  lead-facing result.
