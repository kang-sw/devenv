---
title: agents-plugin async agent calls
parent: 260503-epic-agents-plugin-skill-porting
related:
  260503-feat-agents-plugin-agent-session-runtime: synchronous agent runtime baseline and write-skeleton consumer
  260503-feat-agents-plugin-runtime-boundary: ws-mcp retained process and plugin launcher boundary
  260503-epic-agents-plugin-skill-porting: parent roadmap; async calls should precede core implementation orchestration ports
---

# agents-plugin async agent calls

## Background

The minimum `ws.agents.call` path is synchronous: the MCP tool invocation waits
until the backend Codex turn completes and returns the final agent text. That is
acceptable for short delegate turns and the first `write-skeleton` path, but it
will be a poor fit for the core implementation track. `write-code`, `edit`,
`implement`, `proceed`, and `sprint` need long-running delegates, reviewers,
status inspection, cancellation, and progress recovery without blocking the lead's
entire Codex turn on one MCP request.

Codex does not currently expose a host-level "async wait for MCP result" surface
for plugin tools. The safer shared design is therefore to keep MCP calls
synchronous at the transport layer while making long-running agent calls
asynchronous inside the retained `ws-mcp` runtime.

## Decisions

- Keep `ws.agents.call` synchronous for compatibility and short delegate turns.
- Add `ws.agents.call_async` for asynchronous delegate calls that return a
  `run_id` immediately.
- Use `call_async` rather than `start`, `post`, `submit`, or `dispatch` because
  it preserves the semantic relationship to `call` and gives models the clearest
  tool-selection signal.
- Add `ws.agents.wait` to block until an async run completes, with timeout
  support.
- Add `ws.agents.status` and `ws.agents.tail` so the lead can inspect running
  delegates without waiting for final output.
- Add `ws.agents.cancel` before core implementation skills rely on long-running
  async calls.
- Persist run state to disk; do not rely only on in-memory process handles.

## Constraints

- The MCP server process is retained while the host session is alive, but it may
  restart. Async state must survive process restart well enough for status, tail,
  and print recovery.
- `ws.agents.call_async` must not write non-MCP diagnostics to stdout.
- `ws.agents.wait` must support bounded waits so skills can avoid unbounded host
  turns.
- The async layer must reuse the existing `wsstate` and `wsagent` path managers.
- The initial implementation may target Codex backend only, but schemas should
  leave room for Claude and future backends.

## Prior Art

Claude `ws-named-agent` accumulated practical behavior that should shape this
slice:

- named registry entries survive across calls
- outbox/interrupt delivery is file-backed
- output is persisted for `print`
- tail/list are operational debugging surfaces, not just convenience commands
- long-running orchestration needs cancellation and status visibility
- compression is backend-specific and should not block the basic async contract

The current Go runtime already stores `agent.json`, `output.md`, and
`events.jsonl` under the worktree-local state directory. This ticket should add a
run-level state layer under each agent directory.

## Phases

### Phase 1: Run state model

Add run-level state under each agent directory.

Suggested layout:

```text
agents/<agent-name>/
  runs/
    <run-id>.json
    <run-id>.stdout
    <run-id>.stderr
```

`run.json` should include at least:

- `schema_version`
- `run_id`
- `agent_name`
- `status`: `queued`, `running`, `completed`, `failed`, or `cancelled`
- `pid` when a local process is active
- `started_at`, `updated_at`, `finished_at`
- `prompt_path` or prompt snapshot metadata
- `stdout_path`, `stderr_path`
- `exit_code`
- `session_id` after completion
- `error` when failed

Success criteria:

- Unit tests cover run id generation, state persistence, status transitions, and
  recovery from existing run files.
- Existing synchronous `ws.agents.call` behavior remains unchanged.

### Phase 2: Async process execution

Implement `ws.agents.call_async` by launching the backend call in a child process
or goroutine-managed subprocess and returning `run_id` immediately.

Success criteria:

- `call_async` returns before the backend finishes.
- Backend stdout/stderr are captured to run files.
- Completion updates the agent `session_id`, `output.md`, `events.jsonl`, and
  run status.
- Failure marks the run failed and preserves diagnostics without corrupting MCP
  stdout.

### Phase 3: Wait, status, tail, and cancel tools

Expose the operational async surfaces:

- `ws.agents.wait`
- `ws.agents.status`
- `ws.agents.tail`
- `ws.agents.cancel`

Success criteria:

- `wait` returns final text for completed runs and supports timeout.
- `status` returns structured status text suitable for skill decisions.
- `tail` returns recent event/output/stderr snippets without invoking a backend.
- `cancel` terminates an active process when possible and marks the run
  cancelled.
- Process restart recovery is documented: which operations remain available and
  which require the process handle to still exist.

### Phase 4: Skill integration readiness

Update workflow/runtime documentation and prepare the core implementation track to
use async calls.

Success criteria:

- `agents-plugin/skills/workflow/SKILL.md` distinguishes `ws/agents.call` from
  `ws/agents.call_async`.
- `ai-docs/ref/ws-agent-runtime.md` documents async run state and tool contracts.
- The skill-porting epic records that core implementation orchestration can now
  choose between synchronous and async delegate calls.
