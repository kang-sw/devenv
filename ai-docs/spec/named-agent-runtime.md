---
title: Named Agent Runtime
summary: Durable ws named-agent sessions, asynchronous lifecycle control, subquery fan-out, diagnostics, and backend adapter behavior.
---

# Named Agent Runtime

The named-agent runtime lets ws workflows create durable delegate sessions that
can be called, resumed, interrupted, inspected, and collected through
host-neutral MCP tools. It stores agent and call state on disk so orchestration
can continue across MCP process restarts and host sessions.

## Named Agent Registry And State Layout {#260505-named-agent-registry-state-layout}

Each registered agent owns a worktree-local state directory keyed by its name.
The directory contains registry metadata, the materialized system prompt, inbox
messages, current-call state, diagnostic streams, an append-only event log, and
the last plain-text output.

Agent metadata records the backend, compatibility alias field, resolved model
when present, session id, status, prompt references, output path, capability
flags, and whether the agent is ephemeral. Re-registering an existing agent
replaces its directory only when it has no active current call.

## Prompt Registration And Model Alias Resolution {#260505-agent-prompt-registration-tier-resolution}

Agent registration accepts a prompt chain as logical prompt names or absolute
prompt paths. Bare logical names resolve from the embedded runtime prompt bundle;
ambiguous relative paths are rejected. Public delegate registrations prepend the
embedded delegate-orientation prompt unless the caller explicitly suppresses it
for an internal helper.

The runtime strips prompt frontmatter, concatenates prompt bodies in caller
order, and writes the resolved text to the agent's `system.md`. `light`,
`core`, and `deep` are portable model aliases. `model` may name one of those
aliases or a concrete backend model; concrete model names override alias and
harness defaults. Legacy `tier` inputs remain accepted as compatibility alias
selectors when `model` is absent. Resolved agent metadata reports the alias in
the compatibility `tier` field, plus the resolved backend and concrete model.
{#260508-harness-aware-model-aliases}

## Async Single-Call Lifecycle {#260505-agent-async-single-call-lifecycle}

`agents.call` starts one asynchronous current call for a registered agent and
returns before backend completion. The call snapshot records the prompt path,
execution id, worker pid, stream paths, status, timestamps, exit code, error,
and session id when known.

Only one active call may exist per named agent. The runtime serializes call setup
with a current-call lock, rejects concurrent active calls, writes the prompt
snapshot to disk, starts a worker process, captures backend streams, writes the
final output, and transitions the call to `completed`, `failed`, or `cancelled`.

## Readiness And Result Split {#260505-agent-readiness-result-split}

`agents.wait` waits for one or more named agents and returns readiness metadata
when any requested call is terminal. It does not return final output. If the
timeout expires, the response includes timeout and per-agent ready/pending
metadata. The default wait timeout is 10 minutes.

`agents.result` is the result-consumption surface for a single named agent. It
can read an already completed result or wait up to an explicit timeout. Running,
failed, cancelled, timed-out, and non-ready calls return status text rather than
successful output. Successful result reads erase agents marked ephemeral.

## Async Subquery Ephemeral Agents {#260505-async-subquery-ephemeral-agent}

`subquery` starts a scoped read-only query as an asynchronous named-agent call
and returns immediately with a generated subquery key. Deep-research requests use
the `deep` model alias; ordinary requests use the `light` model alias.

Generated subquery agents are marked ephemeral and suppress delegate orientation
because their system prompt is self-contained. Callers collect answers with
`agents.result(name: <subquery-key>, timeout_seconds: 600)` and can use
`agents.status`, `agents.tail`, or `agents.cancel` for diagnostics or recovery.

## Diagnostics, Tail, And Debug Streams {#260505-agent-diagnostics-tail-debug}

`agents.status` reports registry state and current-call state, including agent
status, backend, tier, model, session id, call status, execution id, pid,
timestamps, exit code, error text, cleanup flags, diagnostic stream paths, and
follow-up guidance.

`agents.tail` reads recent event, runtime, stdout, stderr, and output lines
without invoking the backend. Normal tail output is context-bounded: large JSON
fields and long lines are truncated with an explicit `ws-tail truncated` marker.
Raw inspection remains available through the `agents.debug.*` diagnostic tools.

## Inbox Interrupt Delivery {#260505-agent-inbox-interrupt-delivery}

`agents.interrupt` appends a durable pending message to the target agent's
inbox. Pending messages are marked delivered when the runtime injects them into
a backend input path; delivery does not claim model compliance.

Active Codex workers install a post-tool-use hook that checks the inbox. When
pending messages exist, the hook marks them delivered, writes feedback for Codex
to include in the next model step, and lets the turn continue. If a hook does
not deliver pending mail during the active turn, the runtime prepends the
messages to the next resumed backend call.

## Cancel And Disk-Backed Recovery {#260505-agent-cancel-recovery}

`agents.cancel` performs best-effort local process cancellation for the stored
worker pid and marks the current call cancelled. Cancellation is the urgent
termination path; normal redirects should use `agents.interrupt`.

After an MCP process restart, disk state remains sufficient for `agents.wait`,
`agents.result`, `agents.status`, `agents.tail`, and compatibility output reads.
If the stored worker pid for a running call is no longer alive, readiness and
result paths reconcile the call to a failed terminal state with diagnostic
information.

## Codex Session And JSONL Handling {#260505-codex-agent-session-jsonl-handling}

The Codex backend starts sessions with `codex exec --json` and resumes sessions
with `codex exec resume --json <thread-id>`. The adapter sets the subprocess
working directory for resumed calls and applies the resolved system prompt
through Codex configuration.

> [!note] Planned 🚧
> Codex-backed named-agent calls will deliver the user prompt through a prompt
> path that is reliable for multiline prompts on both first-call and resumed
> sessions, including Windows Codex execution. When Codex CLI supports stdin
> prompt input with `-`, the runtime may prefer stdin over positional argv
> prompt delivery while preserving session id, model, system prompt, stream
> capture, and hook behavior. {#260508-codex-stdin-prompt-delivery}

Codex assigns the thread id after startup. The runtime parses Codex JSONL output
incrementally, persists `thread.started.thread_id` as soon as it appears, and
stores the final plain-text agent message as the caller-facing result. The JSONL
reader accepts large single-line events so verbose tool output does not break
session parsing.

> [!note] Planned 🚧
> Codex-backed named-agent diagnostics will record bounded prompt-delivery
> metadata such as prompt byte size, resume state, Codex CLI version when
> available, and final stdout event shape so operators can distinguish backend
> success from prompt-delivery suspicion without logging prompt contents by
> default. {#260508-codex-prompt-delivery-diagnostics}

## Claude Agent Runner {#260505-claude-agent-runner}

Named-agent registrations with `backend: claude` execute through the same
agent lifecycle as Codex-backed agents. Callers will use the existing
`agents.register`, `agents.call`, `agents.wait`, `agents.result`,
`agents.status`, `agents.tail`, `agents.interrupt`, and diagnostics tools
without switching to a Claude-specific registry or output surface.

The Claude adapter starts first calls with a runtime-managed Claude session id,
resumes later calls with the stored session id, applies the resolved agent
system prompt, and parses Claude JSON output into the final plain-text result.
When a Claude hook delivery stops a turn with `terminal_reason: hook_stopped`,
the adapter resumes the same session so the delivered lead message can be
incorporated into a final result instead of completing with empty output.
Claude process failures will continue through the shared backend invocation
diagnostics path, preserving raw backend errors and reconfiguration hints.

Portable model aliases resolve through the detected MCP harness. A `core`
registration from a Codex MCP session resolves through Codex alias defaults,
while the same alias from a Claude MCP session resolves through Claude alias
defaults. Unknown harnesses use a deterministic configured default.
{#260508-mcp-harness-detection}

## Backend Invocation Failure Diagnostics {#260505-agent-backend-failure-diagnostics}

Backend invocation failures preserve the raw backend error and append a bounded
hint. The hint includes the configured agent name, compatibility alias field,
backend, and model;
PATH-detected backend binaries for known local backends; and explicit recovery
guidance for re-registering an existing agent or changing future alias defaults.

## Codex JSONL Trailing Noise Tolerance {#260505-codex-jsonl-trailing-noise-tolerance}

After the Codex backend has emitted both a session id and a final
`agent_message`, trailing non-JSONL process-control output on stdout does not
invalidate the completed agent result. Non-JSONL output before the result is
complete remains a parse failure.
