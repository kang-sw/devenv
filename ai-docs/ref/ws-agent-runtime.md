# ws Agent Runtime Reference

Host-neutral contract for sustainable ws agent sessions. This document extracts
the shared behavior behind the current Claude `ws-named-agent` tools without
requiring the Claude registry paths, model names, hooks, or session file formats.

## Prior Art

The current Claude implementation is documented in `ai-docs/spec/workflow-skills.md`
under "Agent Orchestration Primitives" and implemented by
`claude-plugin/bin/ws-named-agent`, `ws-new-named-agent`,
`ws-call-named-agent`, and `ws-oneshot-agent`.

The shared contract keeps the durable pattern:

```text
<cli> <resume-command> <session-id> <new-prompt>
```

Backend adapters map that pattern to host-specific commands such as Claude resume,
Codex exec resume, or a future Gemini equivalent.

## Notation

Shared skill text refers to MCP tools with the `ws/<tool-name>` shorthand. This
means MCP server `ws`, tool `<tool-name>`. It is not the literal host-qualified
tool name.

Examples:

- `ws/agents.register`
- `ws/agents.call`
- `ws/agents.interrupt`

`ws:` remains reserved for plugin skill names such as `ws:workflow`.

## State Layout

Agent runtime state uses the project-state layout from
`agents-plugin-tool/internal/wsstate`.

```text
~/.cache/ws@kang-sw-devenv/
  projects/
    <project-key>/
      project.json
      shared/
        locks/
      worktrees/
        <worktree-key>/
          worktree.json
          agents/
            <agent-name>/
              agent.json
              inbox/
                000001.json
              outbox/
              current/
                state.json
                stdout
                stderr
              output.md
              events.jsonl
          review-paths/
          sessions/
          tmp/
```

The project key is `sha256(canonical-root-path)[:12]-<repo-basename>`. The
worktree key is `sha256(canonical-worktree-path)[:12]-<worktree-basename>`.
Linked worktrees share the common/root repository identity and keep separate
worktree-local agent state.

## Agent Directory

Each named agent owns one directory under the worktree-local `agents/` directory.
Agent names must be sanitized before path use and preserved in `agent.json` as
the display name.

Required files:

- `agent.json` — registry metadata for active-agent lookup and backend calls.
- `inbox/` — lead-to-agent queued messages such as interrupts and amendments.
- `outbox/` — agent-to-lead queued messages when a backend supports asynchronous requests.
- `current/` — current async-call state and captured stdout/stderr streams.
- `output.md` — last plain-text response returned to the caller.
- `events.jsonl` — append-only lifecycle and call log for debugging and UI state.

The runtime may accumulate text files. Cleanup is explicit through `agents.erase`
or a future garbage-collection command.

## Registry Schema

`agent.json` schema version 1:

```json
{
  "schema_version": 1,
  "name": "implementer",
  "backend": "codex",
  "tier": "core",
  "model": "",
  "session_id": "",
  "status": "idle",
  "created_at": "2026-05-03T00:00:00Z",
  "last_seen_at": "2026-05-03T00:00:00Z",
  "last_call_at": "",
  "last_output_path": "output.md",
  "prompt_refs": ["implementer"],
  "system_prompt_path": "system.md",
  "capabilities": {
    "resume": true,
    "interrupt": false,
    "compression": false
  }
}
```

Required status values:

- `idle` — registered and not currently known to be running.
- `running` — backend call is active or was last observed active.
- `blocked` — waiting on queued input, tool result, or recoverable host state.
- `failed` — last call failed.
- `erased` — registry retained only as historical event context, if retained.

The minimum active-agent listing surface scans `agents/*/agent.json` and reads
`name`, `backend`, `tier`, `model`, `session_id`, `status`, and `last_call_at`.

## Message Queue

Lead-to-agent messages are stored in `inbox/` as monotonic JSON files:

```text
inbox/000001.json
inbox/000002.json
```

Message schema version 1:

```json
{
  "schema_version": 1,
  "id": "000001",
  "kind": "interrupt",
  "created_at": "2026-05-03T00:00:00Z",
  "status": "pending",
  "text": "Scope reduced to src/foo.ts only."
}
```

Message kinds:

- `interrupt` — stop or redirect the current task at the next safe boundary.
- `amend` — continue the same task with additional instructions.
- `prompt` — queued user turn for the next call.

Message status values:

- `pending`
- `delivered`
- `failed`

The runtime drains pending inbox messages in creation order before or during the
next backend call, depending on host capability. Backends that cannot interrupt a
running process may still enqueue the message and deliver it on the next resume.

`outbox/` uses the same file shape for future agent-to-lead requests. It is a
planned surface; initial Codex backend work may leave it unused.

## Current Call State

Each named agent has at most one active asynchronous call. Public async tools are
keyed by agent name; internal execution identifiers are for diagnostics only.

`current/state.json` schema version 1:

```json
{
  "schema_version": 1,
  "agent_name": "implementer",
  "call_seq": 1,
  "execution_id": "000001",
  "status": "running",
  "pid": 1234,
  "started_at": "2026-05-03T00:00:00Z",
  "updated_at": "2026-05-03T00:00:01Z",
  "prompt_path": "",
  "stdout_path": "current/stdout",
  "stderr_path": "current/stderr",
  "session_id": "019..."
}
```

Current-call status values:

- `queued` — accepted by ws but not yet known to be running in a child process.
- `running` — child process is active or was last observed active.
- `completed` — child process exited successfully and final output was captured.
- `failed` — child process or backend parsing failed.
- `cancelled` — ws requested termination and marked the call cancelled.

Creating/registering an agent with an existing name resets the previous
task-scoped session when no current call is active. If `current/state.json` is
`queued` or `running`, registration fails until the call is cancelled, failed,
completed, or explicitly reset by a future operation that owns process cleanup.

## Workload Tiers

Shared skills use workload-depth tiers, not provider model names:

- `light` — mechanical filtering, routing, small surveys, or narrow lookups.
- `core` — normal implementation, review, synthesis, and document updates.
- `deep` — broad architecture, contract design, reconstruction, or high-risk work.

Backend adapters map tiers to concrete models. A concrete `model` may override the
tier mapping for one agent registration.

## Tool Surface

MCP tools use server `ws` and the following tool names:

- `agents.register` — create or replace one named agent registry entry. Implemented.
- `agents.call` — call a registered agent and resume existing session state when possible. Implemented.
- `agents.call_async` — start a current call for a registered agent and return immediately. Implemented.
- `agents.wait` — wait for the current async call to finish, with timeout support. Implemented.
- `agents.status` — report the current async call and agent registry state. Implemented.
- `agents.oneshot` — register, call, and erase an ephemeral agent. Implemented.
- `agents.interrupt` — enqueue a lead-to-agent message. Planned.
- `agents.print` — return `output.md` for a named agent. Implemented.
- `agents.tail` — summarize recent session/event state without invoking the backend. Implemented.
- `agents.cancel` — best-effort terminate the active worker pid and mark the current call cancelled. Implemented.
- `agents.erase` — remove or mark erased a named agent and clean backend session state where possible. Implemented.
- `agents.list` — list active agents for the current worktree or all cached worktrees. Planned.

## CLI Prototype

The Phase 3 prototype also exposes the minimum runtime through `ws-mcp agents ...`
subcommands. The CLI remains useful for local smoke tests and fallback debugging
without a host MCP client.
The plugin launcher treats these subcommands as part of the runtime command
surface recorded in `agents-plugin/runtime.json`, so local plugin cache binaries
are repaired when the CLI surface drifts even if the MCP tool list is unchanged.

Implemented prototype commands:

```text
ws-mcp agents register --root <repo> --name <name> [--backend codex] [--tier light|core|deep] [--model <model>] [--prompt-ref <logical-name>] [--system-prompt-file <path>]
ws-mcp agents call --root <repo> --name <name> <prompt>
ws-mcp agents call --root <repo> --name <name> --prompt-file -
ws-mcp agents call-async --root <repo> --name <name> <prompt>
ws-mcp agents call-async --root <repo> --name <name> --prompt-file -
ws-mcp agents run-current --root <repo> --name <name>
ws-mcp agents wait --root <repo> --name <name> [--timeout 30s]
ws-mcp agents status --root <repo> --name <name>
ws-mcp agents tail --root <repo> --name <name> [--lines 40]
ws-mcp agents cancel --root <repo> --name <name>
ws-mcp agents oneshot --root <repo> [--name <temporary-name>] <prompt>
ws-mcp agents print --root <repo> --name <name>
ws-mcp agents erase --root <repo> --name <name>
```

The CLI uses the same `agents/<agent-name>/agent.json`, `output.md`, and
`events.jsonl` layout described above. Shared skill text should prefer the MCP
tools with `ws/agents.register`, `ws/agents.call`, `ws/agents.call_async`,
`ws/agents.wait`, `ws/agents.status`, `ws/agents.tail`, `ws/agents.cancel`,
`ws/agents.oneshot`, `ws/agents.print`, and `ws/agents.erase`.

`agents.call_async` writes the prompt snapshot to `current/prompt.md`, starts an
internal `agents run-current` worker process, records the worker pid in
`current/state.json`, and returns before the backend finishes. The worker owns
the actual backend call, captures backend stdout and stderr to `current/stdout`
and `current/stderr`, persists streamed Codex `session_id` updates, writes the
final `output.md`, and transitions the current call to `completed` or `failed`.
Only one active call is allowed per named agent.

`agents.wait` polls `current/state.json` and returns `output.md` when the call is
completed. If a timeout expires, it returns `timeout` plus the same structured
status text produced by `agents.status`. Failed and cancelled calls also return
status text so workflow skills can branch without opening state files directly.

`agents.tail` reads recent lines from `events.jsonl`, `current/stdout`,
`current/stderr`, and `output.md` without invoking a backend. `agents.cancel`
uses the stored worker pid for a best-effort local process kill and marks the
current call `cancelled`. After process restart, `wait`, `status`, `tail`, and
`print` still work from disk state; `cancel` can only terminate a process when
the stored pid still refers to a live local worker, and it does not yet provide
backend-specific process-group cleanup.

## Prompt Resolution

Prompt references are logical names, not repository-local file paths. The runtime
should resolve role prompts from the installed plugin/runtime bundle. Shared skill
text must not point at `claude-plugin/infra/prompts/`.

Registration stores enough prompt metadata for backend re-entry and future
compression handoff, but the first Codex prototype may store a materialized system
prompt file in the agent directory.

## Backend Contract

Backends are responsible for:

- creating or resuming a host session from `session_id`
- applying the materialized system prompt or equivalent instruction file
- sending the new prompt plus pending inbox messages
- updating `session_id` as soon as the host assigns one on first call
- writing the plain-text response to `output.md`
- appending lifecycle entries to `events.jsonl`
- reporting host errors to stderr and marking the agent `failed`

Plain text is the default caller-facing output. Backend JSON or event streams are
adapter internals unless a future tool explicitly exposes them.

The Codex CLI prototype starts sessions with `codex exec --json` and resumes them
with `codex exec resume --json <thread-id>`. `codex exec resume` does not accept
the same `--cd` option as `codex exec`, so the adapter sets the subprocess working
directory instead of passing a command-line cwd flag.

Codex does not allow callers to preassign a thread id. The adapter therefore
parses `--json` stdout incrementally and persists `thread.started.thread_id` as
soon as it appears. Asynchronous status surfaces may briefly show a running call
with an empty `session_id` before the first streamed event arrives.

## Required For `write-skeleton`

The first consumer skill needs:

- named delegate registration for `skeleton-writer`
- resume-backed amendment rounds
- plain-text output for lead review
- file edits performed by the delegate in the shared workspace
- lead-owned final review and commit
- no dependency on Claude PATH scripts or Codex native subagent names

`write-skeleton` does not require background mode, active-agent UI listing,
compression, cross-agent mailboxes, or multi-reviewer fanout.

## Deferred Capabilities

- Transparent compression remains optional and backend-specific.
- Background execution is adapter-specific until a common process lifecycle is proven.
- `outbox/` delivery from agent to lead is reserved for future asynchronous workflows.
- Review path allocation should use the same project-state path manager but is a
  separate surface from agent sessions.
