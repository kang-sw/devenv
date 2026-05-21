---
title: libws-harness agent substrate research
related:
  260429-research-host-neutral-ws-plugin: host-neutral plugin and backend abstraction anchor
  260514-research-ws-web-dashboard-direction: dashboard harness-library direction and owner-auth control-plane context
  260513-research-streamable-http-mcp-transport: adjacent long-running runtime transport and reconnect boundary research
  260512-research-claude-cli-stream-json: prior backend stream-json contract research for Claude runner compatibility
  260512-research-gemini-cli-stream-json: prior backend stream-json contract research for another one-shot backend
related-mental-model:
  - ws-web-dashboard
  - named-agent-runtime
  - mcp-runtime
  - plugin-runtime
---

# libws-harness agent substrate research

## Background

The proposed custom agent substrate would make a local harness the canonical
owner of agent runs while preserving the existing ws workflow boundaries. The
near-term loop can target local Ollama for cheap iteration, but the design
should not bake Ollama into the core contract. The same run substrate should be
able to drive one-shot backends such as `codex exec`, `claude -p`, future Gemini
CLI runs, and local model loops.

The user-facing dashboard should link directly to the harness for interactive
inspection and steering, while subagents should remain naturally compatible with
the existing ws named-agent pattern: one-shot model calls, filesystem-backed
context, durable run records, and explicit handoff through MCP or JSONL
transport. The dashboard must remain an owner-authenticated control plane, not
the canonical ws MCP root, harness detector, model backend, or named-agent
session owner.

The initial development loop is expected to run against a locally running
Ollama service. That is a pragmatic bootstrap path for cheap iteration and
inner-loop experiments, not a statement that Ollama is the default production
backend or the center of the architecture.

The substrate should explicitly support MCP-standard tool integration,
Agents-style skill artifacts, and Codex plugin compatibility. Those compatibility
targets should shape adapter boundaries and metadata, but they should not force
Claude, Codex, MCP stdio, or dashboard browser semantics into the core run
record.

## Ownership Direction

`libws-harness` should be a Rust core library and should strongly own the
canonical run record. That ownership includes the event log, context ledger,
compaction cache, backend session mapping, model-call lifecycle, tool-call
lifecycle, filesystem-backed state management, and interrupt/steering state.
CLI, dashboard, and MCP surfaces should compose the library rather than
reimplement filesystem state management.

The core library should provide the public extension traits and state machine:

```text
libws-harness
  RunLoop
  RunStore
  EventLog
  ContextLedger
  Compactor
  ModelBackend trait
  ToolHost trait
```

Concrete binaries and adapters should be thin:

```text
harness-cli
  JSONL transport over libws-harness, not a human-oriented CLI
  parent-mediated ToolHost for external execution

ws-mcp adapter
  MCP or ws-native ToolHost implementation
  root/session/tool authority stays with ws runtime boundaries

ws-dashboard daemon
  owner-authenticated observation, approval, steering, and control UI
  composes a ToolHost instead of becoming MCP/session authority
```

This keeps the run/context/session record in one place while allowing each host
surface to supply its own authority model.

## Interaction Model

The basic harness should not assume an interactive terminal chat mode. The
default subagent path should remain one-shot: a caller invokes a backend such as
`claude -p`, `codex exec`, a local Ollama turn, or another backend runner, then
records the result through the filesystem-backed run/context store.

Interactive behavior belongs at the server/dashboard layer. The dashboard can
link directly to `libws-harness` through a server process to observe run state,
show transcript and compaction state, and send steering or interrupt requests.
That browser-mediated interactivity should be an adapter over durable run
records, not a separate session model.

Subagent compatibility with existing ws-mcp patterns is a first-class goal.
Subagents should be able to run through one-shot backend calls and recover their
context from the filesystem-backed store without requiring a persistent terminal
conversation.

## JSONL Transport

All machine-facing input and output should be newline-delimited JSON. Newline is
the parse boundary. The CLI is not designed as a human pretty-text interface;
human-readable views can be built on top of JSONL events.

The JSON shapes should be typed Rust data, likely `serde` enums for input
commands and output events. The core library can expose channel-based APIs
internally, while `harness-cli` adapts those channels to stdin/stdout JSONL.
Input events should carry stable request ids when the caller expects
acknowledgement. Output should include system/control responses tied to the
input id so callers can detect whether a steering or interrupt request was
applied, queued, ignored, or failed.

`harness-cli` should treat stdin JSONL as a first-tier control surface. The
caller can write a newline-terminated control event to steer or interrupt a run.
The harness should parse at newline boundaries and report a correlated system
event even when the request loses a timing race and is queued, ignored, or
rejected. This prevents "silently ignored" steering inputs.

Example control input and acknowledgement:

```json
{"type":"control","id":"ctl_123","policy":"abort_model","payload":{"message":"change direction"}}
{"type":"system","in_reply_to":"ctl_123","status":"applied","at":"model_generation"}
```

Example race outcomes:

```json
{"type":"system","in_reply_to":"ctl_124","status":"queued","reason":"tool_batch_in_flight"}
{"type":"system","in_reply_to":"ctl_125","status":"ignored","reason":"run_already_terminal"}
{"type":"system","in_reply_to":"ctl_126","status":"failed","error":{"code":"unsupported_policy"}}
```

Interrupt semantics should be explicit in each input payload rather than hidden
behind a global default. Candidate policies include aborting model generation,
waiting for an in-flight tool batch, cancelling only when the current executor
supports cancellation, or enqueueing steering for the next model turn.

Initial event-shape sketch:

```text
RunInput
  UserMessage
  Control
  Interrupt
  Steering
  ToolResult
  CompactNow
  Shutdown

RunEvent
  RunStarted
  ModelDelta
  ToolCallRequested
  ToolCallCompleted
  ContextCompacted
  Interrupted
  RunCompleted
  RunFailed
  SystemAck
```

This sketch is not a final API. It records the intended level of structure:
every boundary is machine-readable, typed, and correlation-friendly.

## ToolHost Boundary

`ToolHost` should be a `libws-harness` extension point. The harness owns the
tool-call lifecycle state machine: model-requested tool calls, tool-call ids,
pending/running/completed/failed states, interrupt policy, result gating, and
event-log recording. The `ToolHost` implementation owns execution authority:
which tools exist, which calls are allowed, where MCP/plugin calls route, what
sandbox applies, whether approval is required, and how cancellation works.

The distinction should be:

```text
Harness owns:
  "The model requested a tool call; here is its lifecycle and required result."

ToolHost owns:
  "This authority boundary can or cannot execute that tool in this way."
```

Potential implementations:

- `ParentJsonlToolHost`: emits `ToolCallRequested` over stdout and waits for a
  matching `ToolResult` over stdin. This is the safest CLI default because the
  parent process decides how to execute tools.
- `McpToolHost`: routes tool calls to configured MCP servers through an MCP
  client layer.
- `WsNativeToolHost`: calls ws internal APIs directly where the runtime already
  owns the authority boundary.
- `TestToolHost`: provides deterministic fake tools for run-loop and compaction
  tests.
- `DashboardToolHost`: if needed, wraps another ToolHost with owner-auth
  approval, observation, and policy gates rather than becoming the tool registry
  itself.

This avoids inventing another harness layer while still letting `ws-dashboard`,
`harness-cli`, and `ws-mcp` compose different authority models.

The user concern behind this boundary is that if tool execution is fully pushed
outside the harness, MCP plugins and other tool systems may appear to require a
second harness layer. The settled direction is that `ToolHost` is itself part of
the `libws-harness` public contract. Hosts compose concrete ToolHost
implementations, while the harness still owns the lifecycle and event semantics
for tool calls.

## Context Compaction Direction

Context compression should be a first-tier feature rather than a cleanup pass
after the model has already drifted. The purpose is to let the harness clean up
exploration, build output, accidental verbosity, and transient tool noise while
preserving durable intent and evidence.

The original proposal was to request summary cleanup automatically between the
latest cached cursor and the new input after agent turns and relevant tool-call
events. The refined direction is to preserve that first-tier intent while
separating cheap deterministic cleanup from expensive model-assisted summaries.
The exact trigger policy remains open.

The design likely needs two levels:

- Deterministic cleanup for tool output chunking, stdout/stderr classification,
  exit-code capture, path/digest capture, and noisy output elision.
- Model-assisted summaries at turn boundaries or explicit compaction points,
  not necessarily after every raw tool event.

The compaction cursor algorithm must be precise enough to summarize from a
cached point to the current input without losing message boundaries, tool-call
ids, output digests, file snapshot digests, or decision markers. Token-window
thresholds alone are not sufficient because repeated file reads and stale
summaries can otherwise look current.

When total context exceeds a configured threshold, the harness can ask the
agent whether the current turn is a good compaction point. If the agent returns
yes, the harness performs a full compaction using the agent summary plus guarded
file materialization directives.

The compaction mechanism should support both incremental "desk cleanup" after
localized noisy work and periodic full-context compaction once total context
crosses configured thresholds.

## READ Directive

The compaction DSL should treat `{#READ:...}` as a harness directive, not a
natural-language reference. During materialization, the harness forcibly replaces
allowed READ directives with source text. A separate `{#REF:...}` directive is
not required because ordinary summaries can mention file paths when raw text is
not needed.

READ materialization must be guarded. The model may request READ directives, but
the harness decides whether to materialize them based on allowlists, root
policy, line or byte ranges, byte budgets, and file identity checks. The
materialized result should preserve enough digest or snapshot metadata for the
caller to detect stale compacted context.

Unresolved shape:

```text
{#READ:<file-path>#L<line-range>}
```

Possible later extensions include digest pins, byte caps, or named snapshot ids.
The first implementation should prefer a small, auditable directive grammar over
a general templating language.

## Filesystem-Backed State

Filesystem-backed state should live in the core library, not in the CLI. If the
CLI owns the state manager, dashboard, MCP, and tests will either depend on CLI
semantics or reimplement incompatible storage paths. The core store should own
append-only run events and recoverable indexes; adapters should only select the
root, run id, backend, and transport.

The exact resolution used by existing Claude and Codex session files should be
surveyed before finalizing the store layout. Initial direction:

```text
runs/<run-id>/
  events.jsonl              # canonical append-only record
  context-ledger.jsonl      # context spans, source references, cursor anchors
  compactions/<cursor>.json # derived cache, recoverable from events where possible
  backend/                  # backend session ids and adapter-private metadata
```

Only the append-only event stream should be treated as canonical at first.
Indexes and compaction caches should be rebuildable or explicitly versioned.

The core design should assume that every adapter uses the same state manager.
Putting state in `harness-cli` is rejected because the CLI is intended to be a
thin wrapper and because MCP/dashboard/test adapters need the same persistence
semantics without shelling out through the CLI.

## MCP And ws.explore Direction

`ws-mcp` should remain a first-tier integration surface. Over time it may absorb
sandboxed file IO or expose a dedicated `ws.explore` namespace for read-only
agents. That namespace should be a capability-specific API, not merely a hidden
profile over the full lead tool surface.

Candidate `ws.explore` tools could include constrained file reads, search,
symbol or document lookup, and documentation discovery. The goal is to provide
non-intrusive filesystem access to readonly agents without allowing them to
mutate workflow state or inspect arbitrary named-agent internals.

The existing MCP profile filter is not an authority boundary. Any sandboxed IO
direction must introduce real root, capability, and budget enforcement rather
than relying on prompt-level containment.

The broader compatibility target includes MCP tool standards, Agents skills
standards, and Codex plugin packaging/runtime compatibility. A future
implementation should keep shared artifacts host-neutral and place host-specific
behavior in adapters.

## Rejected Or Risky Directions

The dashboard should not become the canonical run owner. It can observe and
steer harness runs, but making it the session authority would conflict with the
existing dashboard boundary and make browser state too authoritative.

The CLI should not own filesystem state. A stateful CLI would make non-CLI
adapters second-class and create divergent behavior between dashboard, MCP, and
test execution.

Tool execution should not be hard-coded into the harness run loop. The harness
should own tool-call lifecycle, but concrete authority and routing belong to a
ToolHost implementation.

Interactive terminal chat should not be the baseline harness contract. It can
be provided by the dashboard/server adapter, while subagents and automation stay
one-shot and JSONL/file-backed by default.

Compaction should not blindly trust model-generated READ directives. Raw file
materialization needs harness-side validation and budget enforcement.

## Open Questions

- What exact persisted resolution do Claude and Codex use for sessions, turns,
  backend ids, tool outputs, and compaction or resume metadata?
- Should the first `ToolHost` implementation be parent-mediated JSONL, an MCP
  client host, or a ws-native host?
- Which interrupt policies are mandatory in the first slice, and which can be
  represented as queued steering until backend cancellation support is known?
- How should tool batch in-flight state interact with late control messages?
- What is the minimal READ directive grammar that still supports digest or
  snapshot validation?
- Which event enum variants are required for a useful first run loop, and which
  should stay adapter-private?
- How should local Ollama loops report model state, partial output, and
  cancellation compared with `codex exec` or `claude -p` one-shot backends?
- Where should dashboard approval state live when a dashboard wraps another
  ToolHost with owner-auth gates?
- Which MCP, Agents skill, and Codex plugin compatibility contracts must be
  treated as first-slice requirements versus later adapter validation?
- Should `harness-cli` expose only JSONL stdin/stdout, or should it also expose
  a non-interactive command API for smoke tests and local diagnostics?
