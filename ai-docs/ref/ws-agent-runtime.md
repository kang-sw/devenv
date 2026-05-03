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
    "interrupt": true,
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

## Workload Tiers

Shared skills use workload-depth tiers, not provider model names:

- `light` — mechanical filtering, routing, small surveys, or narrow lookups.
- `core` — normal implementation, review, synthesis, and document updates.
- `deep` — broad architecture, contract design, reconstruction, or high-risk work.

Backend adapters map tiers to concrete models. A concrete `model` may override the
tier mapping for one agent registration.

## Tool Surface

Planned MCP tools use server `ws` and the following tool names:

- `agents.register` — create or replace one named agent registry entry.
- `agents.call` — call a registered agent and resume existing session state when possible.
- `agents.oneshot` — register, call, and erase an ephemeral agent.
- `agents.interrupt` — enqueue a lead-to-agent message.
- `agents.print` — return `output.md` for a named agent.
- `agents.tail` — summarize recent session/event state without invoking the backend.
- `agents.erase` — remove or mark erased a named agent and clean backend session state where possible.
- `agents.list` — list active agents for the current worktree or all cached worktrees.

Phase 3 should implement only the smallest subset needed to prove backend resume
and support `write-skeleton`: `agents.register`, `agents.call`,
`agents.oneshot`, `agents.print`, and `agents.erase`. `agents.interrupt`,
`agents.tail`, and `agents.list` may be implemented when the queue and UI surfaces
need them.

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
- updating `session_id` when the host assigns one on first call
- writing the plain-text response to `output.md`
- appending lifecycle entries to `events.jsonl`
- reporting host errors to stderr and marking the agent `failed`

Plain text is the default caller-facing output. Backend JSON or event streams are
adapter internals unless a future tool explicitly exposes them.

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
