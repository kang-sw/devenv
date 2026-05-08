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
  proj/
    <project-hash8>/
      project.json
      shared/
        locks/
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
    <project-hash8>@<worktree-hash8>/
      worktree.json
      agents/
      review-paths/
      sessions/
      tmp/
```

The project key is `sha256(canonical-root-path)[:8]`. The main worktree uses
that key directly. Linked worktrees share the common/root repository identity
and use `<project-hash8>@<worktree-hash8>`, where `worktree-hash8` is
`sha256(canonical-worktree-path)[:8]`. The shorter flat layout reduces prompt
and review-path context cost while preserving stable project/worktree lookup.

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

## Orchestration Authority

Agent orchestration tools are lead-owned. The runtime does not rely only on
`WS_MCP_TOOL_PROFILE`, because plugin-managed hosts may reuse or scope MCP server
processes differently from the nested agent subprocess environment.

`ws-mcp` defaults to the full `lead` tool surface. `WS_MCP_TOOL_PROFILE` is an
optional profile filter for hosts that propagate it reliably:

```text
lead > delegate > leaf
```

When profile propagation fails, delegated agents may see lead tools. Prompt-level
delegate orientation remains the durable containment mechanism.

When the profile filter is active, `delegate` and `leaf` cannot see or call
selected lead-owned orchestration or mutation tools. Delegate may use
`agents.wait/result/status/tail/cancel/print` only for generated `subquery-*`
agents; `leaf` also cannot see or call `subquery`. Explicit tool allowlists may
narrow the visible surface for tests, but cannot raise a filtered profile.

## Model Aliases

Shared skills use portable model aliases for routine delegate selection:

- `light` — lower-cost mechanical filtering, routing, small surveys, or narrow lookups.
- `core` — normal implementation, review, synthesis, and document updates.
- `deep` — broad architecture, contract design, reconstruction, or high-risk work.

Backend adapters map aliases to concrete models through defaults and user-local
configuration in `~/.cache/ws@kang-sw-devenv/config.json`. The default Codex
aliases are `light` → `gpt-5.4-mini`, `core` → `gpt-5.5`, and `deep` →
`gpt-5.5`; the default Claude aliases are `haiku`, `sonnet`, and `opus`.
Concrete model names override alias mapping for one registration. When
`backend` is omitted, ws infers the backend from recognizable model names
(`gpt-*`/`codex` → `codex`, `gemini*` → `gemini`,
`haiku`/`sonnet`/`opus`/`claude` → `claude`) and otherwise falls back to
`codex`.

## Tool Surface

MCP tools use server `ws` and the following tool names:

- `agents.register` — create or replace one named agent registry entry. Implemented.
- `agents.call` — start an async call for a registered agent and return follow-up state. Implemented.
- `agents.wait` — wait for one or more async calls to become ready and return metadata. Implemented.
- `agents.result` — return one completed result, optionally waiting first; successful ephemeral results are consumed and erased. Implemented.
- `agents.status` — report the current async call and agent registry state. Implemented.
- `agents.interrupt` — enqueue a lead-to-agent interrupt or redirect message. Implemented.
- `agents.print` — compatibility output reader that does not consume ephemeral agents. Implemented.
- `agents.tail` — summarize recent session/event state without invoking the backend. Implemented.
- `agents.cancel` — best-effort terminate the active worker pid and mark the current call cancelled. Implemented.
- `agents.erase` — remove or mark erased a named agent and clean backend session state where possible. Implemented.
- `agents.list` — list active agents for the current worktree or all cached worktrees. Planned.
- `config.show` — inspect the current user-local configuration and resolved config path without modifying it. Implemented.
- `config.agents_tier` — compatibility surface for configuring the user-local backend/model mapping for a model alias. Implemented.
- `path.generate` — allocate worktree-scoped writable workflow artifact paths. Implemented for `kind: "review"`.
- `runtime.info` — return runtime metadata, including embedded prompt bundle hash. Implemented.
- `api.list` — list existing API documentation cache domains. Implemented.
- `api.ask` — route an API documentation question to persistent `api-doc-<domain>` manager sessions. Implemented.

## CLI Prototype

The Phase 3 prototype also exposes the minimum runtime through `ws-mcp agents ...`
subcommands. The CLI remains useful for local smoke tests and fallback debugging
without a host MCP client.
The plugin launcher treats these subcommands as part of the runtime command
surface recorded in `agents-plugin/runtime.json`, so local plugin cache binaries
are repaired when the CLI surface drifts even if the MCP tool list is unchanged.

Implemented prototype commands:

```text
ws-mcp agents register --root <repo> --name <name> [--backend codex] [--harness codex|claude] [--model light|core|deep|<concrete-model>] [--tier light|core|deep] [--prompt <stem-or-absolute-path>] [--prompt-ref <logical-name>] [--system-prompt-file <path>]
ws-mcp agents call --root <repo> --name <name> <prompt>
ws-mcp agents call --root <repo> --name <name> --prompt-file -
ws-mcp agents run-current --root <repo> --name <name>
ws-mcp agents wait --root <repo> --name <name> [--name <name> ...] [--timeout 10m]
ws-mcp agents wait --root <repo> [--timeout 10m] <name> [<name> ...]
ws-mcp agents result --root <repo> --name <name> [--timeout 10m]
ws-mcp agents status --root <repo> --name <name>
ws-mcp agents interrupt --root <repo> --name <name> <message>
ws-mcp agents check-inbox --root <repo> --name <name>
ws-mcp agents tail --root <repo> --name <name> [--lines 3]
ws-mcp agents cancel --root <repo> --name <name>
ws-mcp agents print --root <repo> --name <name>
ws-mcp agents erase --root <repo> --name <name>
ws-mcp config show
ws-mcp config agents-tier --tier <light|core|deep> [--backend <backend>] [--model <concrete-model>]
ws-mcp path generate --root <repo> --kind review <stem> [<stem> ...]
ws-mcp runtime info
```

The CLI uses the same `agents/<agent-name>/agent.json`, `output.md`, and
`events.jsonl` layout described above. Shared skill text should prefer the MCP
tools with `ws/agents.register`, `ws/agents.call`, `ws/agents.wait`,
`ws/agents.result`, `ws/agents.status`, `ws/agents.interrupt`,
`ws/agents.tail`, `ws/agents.cancel`, `ws/agents.print`, `ws/agents.erase`,
`ws/config.show`, `ws/config.agents_tier`,
`ws/path.generate`, `ws/api.list`, `ws/api.ask`, and `ws/runtime.info`.

`agents.call` acquires a short-lived `current/setup.lock`, writes the prompt
snapshot to `current/prompt.md`, starts an internal `agents run-current` worker
process, records the worker pid in `current/state.json`, and returns before the
backend finishes. The setup lock serializes same-agent concurrent calls around
`BeginCurrentCall` and prompt snapshot creation; stale setup locks whose owner
pid is no longer alive may be recovered. The worker owns the actual backend
call, captures backend stdout and stderr to `current/stdout` and
`current/stderr`, persists streamed Codex `session_id` updates, writes the final
`output.md`, and transitions the current call to `completed` or `failed`. Only
one active call is allowed per named agent. `api.ask` composes over the same
agent runtime by registering or reusing `api-doc-<domain>` sessions and by
serializing same-domain calls before invoking `agents.call`/`agents.result`.

`agents.interrupt` appends a durable pending message to `inbox/<id>.json`.
Messages use a two-state contract: `pending` means the runtime has not injected
the message into a backend input path, and `delivered` means it has. `delivered`
does not claim that the model complied with the message.

Active Codex async workers configure a `PostToolUse` hook that runs
`agents check-inbox`. When pending inbox mail exists, the hook atomically marks
messages `delivered`, writes the lead-message feedback to stderr, and exits 2.
Codex CLI 0.128.0 injects that hook feedback into the next model step and
continues the turn; the runtime does not signal or kill Codex for normal
interrupt delivery. If no hook claims the messages during an active turn,
pending messages are delivered at the start of the next backend call by adding
them to the lead prompt. Delivery route details are recorded in event/runtime
logs such as `inbox.delivered_via_hook` and `inbox.delivered_via_resume`.

Use `agents.cancel` for urgent process termination. Cancellation may interrupt
or kill owned subprocesses and can require partial-output recovery; it is not
the normal `agents.interrupt` delivery path.

`agents.wait` polls `current/state.json` for one or more names and returns
readiness metadata when any named call is completed, failed, cancelled, or
otherwise no longer active. It never returns final output. If a timeout expires,
it returns `wait_timeout: true` plus ready/pending metadata for every requested
agent. Default timeout is 10 minutes. Explicit bounded waits for normal agent
work should use at least 10 minutes.

`agents.result` is the result-consumption surface. Without a timeout it reads an
already-completed result or returns a non-ready status immediately. With a
timeout it waits up to the bound. Successful result reads erase agents marked
ephemeral, such as generated subquery agents. Failed, cancelled, timed-out, and
still-running agents remain available for diagnostics.

`agents.tail` reads recent lines from `events.jsonl`, `current/stdout`,
`current/stderr`, and `output.md` without invoking a backend. Normal tail output
is context-bounded: large stream fields such as `aggregated_output` and long
lines are shortened with a visible `ws-tail truncated` marker. Raw stream
inspection belongs under `agents.debug.*`. Routine progress checks should
request `--lines 3`; larger tails are for concrete failure diagnosis.
`agents.cancel` uses the stored worker pid for a best-effort local process kill
and marks the current call `cancelled`. After process restart, `wait`, `result`,
`status`, `tail`, and `print` still work from disk state; `cancel` can only
terminate a process when the stored pid still refers to a live local worker, and
it does not yet provide backend-specific process-group cleanup.

## Prompt Resolution

Prompt references are logical names or absolute prompt paths, not
repository-local plugin source paths. Shared skill text must not point at
`claude-plugin/infra/prompts/`.

`agents.register` accepts `prompts` as the canonical prompt chain field.
`prompt_refs` remains a migration alias for older callers. Bare stems resolve
from the embedded runtime prompt bundle; absolute paths read the specified file
directly. Ambiguous relative paths are rejected until a later root-relative
contract exists.

Public `agents.register` calls prepend the embedded `delegate-orientation`
prompt before caller material. The orientation is a host-neutral role boundary
for lead-spawned delegates; it tells implementers and reviewers not to perform
lead-owned orchestration, reviewer fanout, or documentation lifecycle work
unless explicitly assigned. Internal helpers such as `subquery` suppress this
orientation and keep their scoped system prompt self-contained while still using
the named-agent async call path.

The runtime strips YAML frontmatter from each resolved prompt and concatenates
prompt bodies in caller order with `---` separators. `system_prompt_text`, when
provided, is appended after resolved prompt bodies so existing materialized
prompt callers remain compatible.

The first embedded bundle contains role prompts, review partition prompts,
`delegate-orientation`, and `impl-playbook`. Frontmatter `model: light`,
`model: core`, or `model: deep` supplies a portable alias when the caller did
not pass an explicit `model` or legacy `tier`. Compatibility frontmatter values
`haiku`, `sonnet`, and `opus` still map to the same aliases. Unknown frontmatter
model names become concrete backend model overrides only when no explicit model
was supplied.

Registration writes the materialized prompt to `system.md` in the agent
directory and stores the requested prompt chain in `agent.json` as `prompt_refs`
for compatibility with the current registry schema. `runtime.info` reports the
prompt bundle source commit, content SHA-256, and embedded prompt stem list so a
plugin launcher can detect runtime drift against `agents-plugin/runtime.json`.

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
