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

The minimum `ws/agents.call` path is synchronous: the MCP tool invocation waits
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

- Keep `ws/agents.call` synchronous for compatibility and short delegate turns.
- Treat each named agent as a task-scoped session. Creating/registering the same
  name for a new task should reset the previous conversation state rather than
  imply a permanent worker identity.
- Add `ws/agents.call_async` for asynchronous delegate calls that return
  immediately with the agent name and current status. Do not require callers to
  track a public run handle for the normal case.
- Use `call_async` rather than `start`, `post`, `submit`, or `dispatch` because
  it preserves the semantic relationship to `call` and gives models the clearest
  tool-selection signal.
- Add `ws/agents.wait` to block until the named agent's current async call
  completes, with timeout support.
- Add `ws/agents.status` and `ws/agents.tail` so the lead can inspect running
  delegates without waiting for final output.
- Add `ws/agents.cancel` before core implementation skills rely on long-running
  async calls.
- Allow only one active async call per named agent. A second async call against
  the same agent should fail with an explicit busy status unless the caller
  cancels or resets the agent first.
- Persist current-call state to disk; do not rely only on in-memory process
  handles.

## Constraints

- The MCP server process is retained while the host session is alive, but it may
  restart. Async state must survive process restart well enough for status, tail,
  and print recovery.
- `ws/agents.call_async` must not write non-MCP diagnostics to stdout.
- `ws/agents.wait` must support bounded waits so skills can avoid unbounded host
  turns.
- Public async operations should be keyed by agent name. Internal execution ids,
  sequence numbers, or state file names may exist for crash recovery, but they
  should not become the primary user-facing contract.
- Codex assigns thread ids after session creation, but `codex exec --json`
  streams `thread.started` before the turn completes. The async implementation
  should parse JSONL incrementally and persist the thread id as soon as that
  event arrives.
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

Claude `ws-oneshot-agent` is only a routing composition: create a temporary named
agent, call it once, then erase it. The MCP `ws/agents.oneshot` surface should
keep that composition semantics rather than introduce a separate session model.

The API documentation workflow is more nuanced and should not be collapsed into
pure oneshot behavior. `ws-ask-api` uses a oneshot pre-router to select domains,
then delegates each domain query to a persistent, lock-protected
`api-doc-<domain>` named agent through `ws-ask-api-internal`. Future API-doc MCP
work should preserve that split: ephemeral routing for domain selection,
persistent per-domain sessions for cached documentation management.

The current Go runtime already stores `agent.json`, `output.md`, and
`events.jsonl` under the worktree-local state directory. This ticket should add a
current-call state layer under each agent directory.

## Phases

### Phase 0: Streaming thread-id registration

Prove and encode the Codex JSONL streaming assumption before building the async
state machine. Unlike Claude, Codex cannot accept a caller-chosen session UUID
before the first turn. The runtime must therefore treat `session_id` as initially
unknown, then update agent/current-call state as soon as the child Codex process
emits `thread.started`.

Success criteria:

- Add or update Codex backend tests around incremental JSONL event handling so
  `thread.started` updates session state before final `turn.completed`.
- Document that `status` may briefly show a running call with no `session_id`
  until the first streamed event arrives.
- Preserve final-output extraction from the last `agent_message`.
- Existing synchronous `ws/agents.call` behavior remains unchanged.

### Result (9e7420e) - 2026-05-03

Implemented incremental Codex JSONL parsing in the Go wsagent backend. The
Codex runner now uses `StdoutPipe`, starts the child process, scans JSONL while
the process is running, and invokes a session-id callback immediately when
`thread.started` arrives. `Manager.Call` uses that callback to persist
`agent.json.session_id` and append `call.session_started` before the final
agent message is available.

Added tests proving that the parser notifies `thread.started` before reading
later JSONL chunks, that `Manager.Call` persists the streamed session id while
the agent is still marked `running`, and that final output extraction from the
last `agent_message` remains intact. Updated the agent runtime reference to
document the Codex-specific window where async status may briefly show a running
call without a `session_id`.

### Phase 1: Current call state model

Add current async-call state under each agent directory. The public API should be
agent-name keyed, while the file format may keep an internal sequence or
execution id for diagnostics and recovery.

Suggested layout:

```text
agents/<agent-name>/
  current/
    state.json
    stdout
    stderr
```

`state.json` should include at least:

- `schema_version`
- `agent_name`
- internal `call_seq` or `execution_id` when useful for diagnostics
- `status`: `queued`, `running`, `completed`, `failed`, or `cancelled`
- `pid` when a local process is active
- `started_at`, `updated_at`, `finished_at`
- `prompt_path` or prompt snapshot metadata
- `stdout_path` and `stderr_path`
- `exit_code`
- `session_id` after completion
- `error` when failed

Success criteria:

- Unit tests cover state persistence, status transitions, busy-agent rejection,
  reset behavior, and recovery from existing current-call files.
- Existing synchronous `ws/agents.call` behavior remains unchanged.

### Result (429a718) - 2026-05-03

Added the current-call state model to the Go wsagent runtime. Agent layouts now
include `current/state.json`, `current/stdout`, and `current/stderr`, with a
schema-versioned `CurrentCall` record, status constants, atomic state writes,
state recovery, and helper transitions for queued, running, completed, failed,
and reset flows.

The public model remains agent-name keyed. `BeginCurrentCall` rejects a second
call when the named agent has `queued` or `running` current state, while
completed or failed current state can be replaced by the next call and keeps an
incremented internal sequence for diagnostics. Re-registering an existing agent
now resets the task-scoped session and clears previous output when no current
call is active; it refuses to reset an agent with active current-call state.

Added unit tests for state persistence, status transitions, busy-agent
rejection, reset behavior, recovery from an existing state file, and same-name
registration reset semantics. Updated the agent runtime reference with the
`current/` layout, schema, statuses, and active-call reset boundary.

### Phase 2: Async process execution

Implement `ws/agents.call_async` by launching the backend call in a child process
or goroutine-managed subprocess and returning immediately with the agent name and
status.

Success criteria:

- `call_async` returns before the backend finishes.
- A second `call_async` against an already-running agent fails with a clear busy
  response.
- Backend stdout/stderr are captured to run files.
- Completion updates the agent `session_id`, `output.md`, `events.jsonl`, and
  current-call status.
- Failure marks the current call failed and preserves diagnostics without
  corrupting MCP stdout.

### Result (b5cc3b1) - 2026-05-03

Implemented the first asynchronous execution path for Codex-backed ws agents.
The runtime now exposes `ws/agents.call_async` and CLI fallback
`ws-mcp agents call-async`. A call writes the prompt to `current/prompt.md`,
creates or updates `current/state.json`, starts a separate
`ws-mcp agents run-current` worker process, records the worker pid, and returns
immediately with the named agent and running status.

The worker owns the actual backend call. It reads the prompt snapshot, resumes
the registered agent session, captures Codex JSONL stdout and stderr into
`current/stdout` and `current/stderr`, persists streamed `session_id` updates,
writes the final `output.md`, appends lifecycle events, and transitions the
current call to `completed` or `failed`. Busy rejection remains agent-name keyed:
a second `call_async` against an agent whose current call is `queued` or
`running` fails with an active-call error.

The implementation deliberately keeps `wait`, `status`, `tail`, and `cancel` out
of this phase. Tests verify prompt snapshotting, worker launch, busy rejection,
worker completion, stream capture, and output persistence. A real Codex smoke
confirmed that `call-async` returned in zero seconds, then the worker completed
and `agents print` returned `WS_ASYNC_PHASE2_OK`.

### Phase 3: Wait, status, tail, and cancel tools

Expose the operational async surfaces:

- `ws/agents.wait`
- `ws/agents.status`
- `ws/agents.tail`
- `ws/agents.cancel`

Success criteria:

- `wait` returns final text for the named agent's completed current call and
  supports timeout.
- `status` returns structured status text suitable for skill decisions.
- `tail` returns recent event/output/stderr snippets without invoking a backend.
- `cancel` terminates an active process when possible and marks the current call
  cancelled.
- Process restart recovery is documented: which operations remain available and
  which require the process handle to still exist.

### Result (pending) - 2026-05-03

Implemented the operational async inspection surface. The runtime now exposes
`ws/agents.wait`, `ws/agents.status`, `ws/agents.tail`, and `ws/agents.cancel`,
with matching CLI fallback commands under `ws-mcp agents`.

`wait` polls the named agent's `current/state.json`, returns final `output.md`
for completed calls, and supports bounded waits through `timeout_seconds` in MCP
or `--timeout` in the CLI. Timeout, failed, and cancelled cases return
structured status text rather than requiring callers to inspect state files.
`status` reports stable line-oriented fields for the agent registry and current
call. `tail` reads recent lines from `events.jsonl`, `current/stdout`,
`current/stderr`, and `output.md` without invoking a backend. `cancel` attempts
to kill the stored worker pid, marks the current call `cancelled`, returns
status text, and records the cancellation event.

The process restart boundary is intentionally documented as best effort:
`wait`, `status`, `tail`, and `print` recover from disk state after MCP restart,
but `cancel` can only terminate a process when the stored pid still refers to a
live local worker. Backend-specific process-group cleanup remains future work.

### Phase 4: Skill integration readiness

Update workflow/runtime documentation and prepare the core implementation track to
use async calls.

Success criteria:

- `agents-plugin/skills/workflow/SKILL.md` distinguishes `ws/agents.call` from
  `ws/agents.call_async`.
- `ai-docs/ref/ws-agent-runtime.md` documents async run state and tool contracts.
- The skill-porting epic records that core implementation orchestration can now
  choose between synchronous and async delegate calls.
