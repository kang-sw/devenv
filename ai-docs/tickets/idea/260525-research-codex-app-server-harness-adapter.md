---
title: Codex app-server harness adapter research
related:
  260525-research-daemon-owned-harness-control-plane: umbrella direction for daemon-owned harness runtime, capability broker, and approval control plane
  260521-research-libws-harness-mvp-planning: may change the recommended first MVP backend and child-ticket ordering
  260521-research-libws-harness-agent-substrate: prior JSONL-first harness substrate direction and backend abstraction constraints
  260429-research-host-neutral-ws-plugin: host-neutral plugin and backend abstraction anchor
  260514-research-ws-web-dashboard-direction: dashboard daemon lifecycle and harness-library direction
  260524-epic-async-exec-job-surface: adjacent persisted process output and activity projection model
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
  - plugin-runtime
  - ws-web-dashboard
---

# Codex app-server harness adapter research

## Background

The earlier libws-harness research favored a backend-agnostic run substrate with
one-shot backends such as `codex exec`, local model loops, or later custom
ToolHost implementations. That direction still protects the core harness
contract, but the Codex CLI now exposes a richer `codex app-server` protocol
that may be a better first integration layer than repeatedly forking
`codex exec`.

`260525-research-daemon-owned-harness-control-plane` is now the umbrella
direction. This ticket remains the lower-level Codex app-server evidence and
adapter investigation underneath that direction.

This ticket captures the initial investigation into using `codex app-server` as
the first Codex-backed harness adapter. It is research only. It should not make
the app-server protocol the ws public contract without a later actionable
ticket and spec review.

## Findings

`codex app-server` is not just a TUI control surface. In Codex CLI 0.133.0 it
exposes a newline-delimited JSON-RPC protocol over stdio and can generate
version-matched TypeScript and JSON Schema bundles through:

```text
codex app-server generate-ts --experimental --out <dir>
codex app-server generate-json-schema --experimental --out <dir>
```

Local smoke checks confirmed that a stdio app-server process responds to
`initialize`, `account/read`, `account/rateLimits/read`, and
`thread/loaded/list`. The local account surface reported ChatGPT-backed Codex
auth rather than API-key auth, which means app-server can reuse Codex product
entitlement instead of requiring separate OpenAI API billing.

The generated protocol includes the surfaces needed for a daemon-owned Codex
session adapter:

- `thread/start`, `thread/resume`, `thread/fork`, `thread/read`,
  `thread/list`, `thread/loaded/list`, and `thread/unsubscribe`.
- `turn/start`, `turn/steer`, and `turn/interrupt`.
- Streaming notifications such as `turn/started`, `turn/completed`,
  `item/started`, `item/completed`, `item/agentMessage/delta`,
  `item/mcpToolCall/progress`, command output deltas, process output deltas,
  token-usage updates, and error notifications.
- Manual compaction and history operations such as `thread/compact/start`,
  `thread/rollback`, and `thread/inject_items`.
- Built-in MCP, plugin, skill, fs, command, process, approval, rate-limit, and
  model-listing surfaces.

The protocol also has managed-daemon and proxy surfaces:

```text
codex app-server daemon start|restart|stop|version|bootstrap
codex app-server proxy
codex remote-control start|stop
```

The local managed-daemon `version` check failed only because no app-server
control socket was running. For an MVP adapter, starting
`codex app-server --listen stdio://` as a child process is likely simpler than
depending on the managed-daemon bootstrap path.

## Design Direction

Use a ws-owned interface layer in front of app-server rather than exposing the
Codex protocol directly.

Suggested boundary:

```text
ws-harness-session-api
  start_session
  resume_session
  start_turn
  steer_turn
  interrupt_turn
  compact_session
  read_thread_or_transcript
  normalize_events

CodexAppServerAdapter
  owns JSON-RPC transport, generated-schema compatibility, and app-server
  Thread/Turn/ThreadItem mapping

FutureCustomHarnessAdapter
  implements the same ws-harness-session-api using libws-harness RunStore,
  EventLog, ToolHost, and backend loops
```

This keeps `codex app-server` as the first implementation, not the permanent
public contract. Dashboard, MCP, and future harness callers consume normalized
ws events and commands. The adapter privately maps app-server `Thread`,
`Turn`, `ThreadItem`, and notification shapes.

The initial normalized event vocabulary should be deliberately smaller than
app-server's native protocol. Candidate categories:

- session started/resumed/forked/closed
- turn started/completed/failed/interrupted
- assistant output delta/completed
- tool call started/progress/completed/failed
- command output delta/completed
- file change summarized
- approval requested/resolved
- compaction started/completed
- token usage updated
- rate-limit or auth degraded

## Why This May Be A Better Initial Default

`codex exec` remains stable and simple, but it forces one process per turn. That
model preserves conversation state through `codex exec resume` while still
making MCP and tool processes restart repeatedly.

`codex app-server` better matches a daemon lifecycle:

- one long-running process can hold loaded threads and stream events;
- `turn/steer` provides a first-class live steering path;
- `turn/interrupt` provides a first-class cancellation path;
- app-server already normalizes Codex tool calls, command execution, file
  changes, assistant deltas, token usage, and rate-limit state;
- manual compaction can be triggered through the same thread protocol;
- generated schemas let ws detect drift instead of hand-maintaining guessed
  payload shapes.

Using app-server first could buy time before implementing a custom model loop,
ToolHost, compaction engine, and run store. The custom harness can later sit
behind the same ws-owned session API.

## Constraints

Do not expose raw app-server protocol objects as the ws dashboard, MCP, or
libws-harness public contract. App-server is an adapter detail.

Keep first-slice app-server usage narrow. Avoid depending on experimental
surfaces such as raw Responses API event injection, realtime audio, standalone
unsandboxed `process/spawn`, or broad fs mutation APIs until a ticket explicitly
accepts those boundaries.

Treat app-server command, fs, process, MCP, plugin, and approval features as
authority-sensitive. A ws dashboard route should not blindly proxy them as
host-control APIs.

Prefer a direct child-process stdio client for the first spike. Defer managed
daemon, remote-control, and proxy lifecycle integration until the basic session
adapter and event normalization are proven.

Persist enough ws-side metadata to reconnect after daemon restart. App-server
thread ids, session ids, thread path when present, cwd, model/provider,
permission settings, and latest known turn ids are adapter metadata, not the
whole ws run record.

## Risks

The app-server API is labeled experimental in some places and includes unstable
fields. Mitigate by allowlisting only the methods used by ws and by keeping
generated schema snapshots or smoke tests for those methods.

App-server is Codex-specific. If ws builds directly on it, later Ollama,
OpenAI API, Claude, or custom harness adapters become harder. The ws-owned
session API is the guardrail.

Authentication and rate-limit behavior differs from OpenAI API billing. That is
useful for subscription-backed local Codex workflows but cannot replace an API
backend for CI, service accounts, or predictable token billing.

Loaded-thread lifecycle is connection and server-process sensitive. The adapter
must define when it starts a fresh app-server process, resumes stored threads,
unsubscribes, kills the child process, or treats app-server loss as recoverable.

Approval requests are server-initiated JSON-RPC calls. The adapter must surface
them as explicit ws events and must not auto-approve by default merely because
the process is daemon-owned.

## Rejected Or Deferred Directions

Do not drive the Codex TUI through a PTY as the first harness control plane.
The TUI is human-oriented, harder to parse, and weaker than app-server's typed
JSON-RPC protocol.

Do not replace libws-harness with app-server. App-server can be the first Codex
adapter, while libws-harness still owns future backend-independent run records,
event logs, ToolHost lifecycle, compaction, and dashboard projection contracts.

Do not start by wrapping every app-server feature. The first adapter should
prove thread/turn lifecycle, streaming assistant output, steering, interrupt,
basic tool/command event normalization, and auth/rate-limit diagnostics.

## Next Investigation

Create an actionable spike ticket only after deciding the owning package and
test boundary. The spike should answer:

- What Rust client module owns app-server JSON-RPC transport?
- Should generated schemas be checked in, generated in tests, or represented by
  hand-written Rust structs for an allowlisted subset?
- Which exact app-server methods are in the MVP allowlist?
- How are server-initiated approval, user-input, MCP elicitation, and auth
  refresh requests surfaced to ws callers?
- What is the normalized event schema and which native `ThreadItem` variants
  are degraded or ignored?
- How does the adapter recover from child-process exit, lost loaded threads,
  app-server protocol errors, and rate-limit exhaustion?
- Which dashboard Activity Console contracts can consume the normalized event
  stream without becoming Codex session authority?
