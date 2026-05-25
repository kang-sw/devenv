---
title: Daemon-owned harness control plane research
related:
  260525-research-codex-app-server-harness-adapter: lower-level evidence for using Codex app-server as the first backend adapter
  260521-research-libws-harness-mvp-planning: superseded JSONL-first MVP planning absorbed by this daemon-owned direction
  260521-research-libws-harness-agent-substrate: superseded custom-substrate research whose durable store, ToolHost, and event ideas are absorbed here
  260429-research-host-neutral-ws-plugin: host-neutral plugin and backend abstraction anchor
  260514-research-ws-web-dashboard-direction: dashboard daemon lifecycle and owner-auth control-plane context
  260524-epic-async-exec-job-surface: existing exec job surface that can become a brokered capability path
  260513-research-streamable-http-mcp-transport: adjacent transport and reconnect boundary research
related-mental-model:
  - named-agent-runtime
  - mcp-runtime
  - plugin-runtime
  - ws-web-dashboard
---

# Daemon-owned harness control plane research

## Background

Earlier libws-harness research assumed a JSONL-first custom run substrate would
be the initial path. Later Codex app-server investigation changed the likely
first backend: `codex app-server` already exposes persistent thread and turn
control, streaming events, steering, interrupt, account, model, and rate-limit
surfaces. That makes it a better first Codex-backed adapter than repeatedly
forking `codex exec`.

The settled direction is not to replace libws-harness with app-server. Instead,
use app-server as the first backend behind a ws-owned harness boundary, while
moving command execution, file IO, MCP calls, and approval policy into a
daemon-owned capability plane. This buys time for a custom harness backend
without making Codex protocol objects the ws public contract.

## Settled Macro Direction

The normal path should be daemon-owned:

```text
ws-dashboard
  -> ws-dashboard-daemon
      -> owns HarnessRuntime
      -> owns approval and capability brokers
      -> owns sustained sessions

ws-mcp / ws.agent
  -> ws-harness binary
      -> proxies to daemon when available
      -> falls back to local runtime when daemon is unavailable

libws-harness
  -> object-trait runtime boundary
  -> backend-neutral session, turn, approval, capability, and event contracts

agent backend
  -> Codex app-server first
  -> native/custom backends later
```

`ws-harness` should be a client or proxy in the primary path, not the canonical
session owner. The daemon composition root fills the concrete trait objects and
keeps sessions alive. Local fallback can still compose an in-process runtime for
diagnostics or environments without a daemon, but it should not be the steady
state that ws.agent stability depends on.

## Object-Trait Runtime Boundary

The public harness boundary should prefer dynamic trait objects over generic
runtime types. Turns are not performance sensitive enough to justify pushing
generic composition into the public surface, and runtime configuration needs to
select backends, capability brokers, and stores dynamically.

Conceptual trait roles:

```text
HarnessBackend
  Starts, resumes, and closes backend sessions.

HarnessSession
  Represents a sustained agent session until erase, retire, or backend loss.

TurnDriver
  Starts, steers, interrupts, and compacts turns.

HarnessEventStream
  Exposes normalized backend-neutral events.

CapabilityBroker
  Owns command, file, MCP, and other execution capabilities.

ApprovalBroker
  Generalizes backend-native and ws-native approval requests.

HarnessStore / HarnessEventLog
  Persist metadata, payload paths, and append-only event history for recovery
  and dashboard projection.
```

The concrete daemon-side implementation may look like:

```text
CodexAppServerBackend
DaemonCapabilityBroker
DaemonApprovalBroker
SqliteHarnessStore
FileBackedEventLog
```

The client/proxy side may use:

```text
RemoteHarnessBackend
RemoteCapabilityBroker
RemoteApprovalBroker
```

Trait objects should not cross the process boundary. RPC messages cross the
boundary; each process fills its own object graph at its composition root.

## Capability And Approval Plane

Codex app-server should be treated as the model/session plane. The ws daemon
should own the capability and authority plane:

```text
Codex app-server
  session and turn lifecycle
  assistant and reasoning event stream
  auth, model, and rate-limit state

ws daemon
  command execution
  file IO and patch application
  MCP tool calls
  approval queue
  durable output and event storage
  dashboard projection
```

The first implementation should avoid adopting app-server native command, fs,
process, MCP, or plugin surfaces as public ws contracts. If those native
surfaces cannot be fully disabled for a Codex-backed session, the adapter should
normalize their events and approval requests into the same ws approval model
instead of exposing raw app-server protocol objects.

`ApprovalBroker` should be an authority port, not a UI abstraction. It should
receive backend-neutral requests such as command execution, file writes, file
patches, MCP tool calls, network access, sandbox escapes, and backend-native
approval prompts. Decisions should be modeled as grants with explicit scope,
starting with per-request approval and leaving room for per-turn or per-session
grants later.

## ws-mcp Migration Direction

The existing ws-mcp surface already contains powerful local execution paths and
should not be rewritten as a prerequisite. The migration path is to add broker
hooks and runtime detection:

```text
direct-local
  Current ws-mcp behavior. Used when no daemon or broker context exists.

brokered-if-available
  Discover a dashboard or harness daemon and route dangerous exec, file, MCP,
  and agent actions through approval/capability RPC when possible.

brokered-required
  Future dashboard-first or remote mode. Dangerous actions fail when no broker
  is available.
```

Dashboard-context detection is a routing hint, not a security boundary. Actual
authority should come from daemon-issued session, actor, or capability context.
MCP profile filters remain containment hints and must not be treated as hard
approval boundaries.

The existing `exec.*` MCP tools are a useful starting point for brokered command
execution because they already have durable job records and file-backed output.
Future harness work should evaluate whether those internals can become the
daemon command capability implementation rather than creating a parallel exec
store.

## ws.agent Stability Goal

The named-agent runtime currently invokes backends such as `codex exec` per
call. Recursive use of ws-mcp through ws agents can therefore restart model
processes and MCP subprocesses repeatedly, causing fragile root, actor, session,
and result handling.

The target path is for ws.agent to gain a libws-harness backend that sustains
the same harness session until erase or retirement:

```text
agents.call #1 -> daemon session A, turn 1
agents.call #2 -> daemon session A, turn 2
agents.interrupt -> daemon session A, steer or interrupt
agents.erase -> daemon session A, retire or close
```

When a daemon is available, ws-mcp or ws.agent should invoke `ws-harness` as a
thin client. The client routes to the daemon-owned runtime, which holds the
Codex app-server thread, approval channel, actor/session metadata, and event
log. When no daemon is available, the client may fall back to local one-shot or
resume behavior with a local policy approval broker.

## Relationship To Codex app-server

Codex app-server remains the best first Codex backend candidate because it
already provides persistent thread/turn control, steering, interrupt, event
streaming, and subscription-backed account state. It should be treated as an
adapter implementation detail behind libws-harness traits.

The first spike must verify:

- whether app-server native command, file, process, MCP, and plugin surfaces can
  be disabled or narrowed for a ws-managed session;
- how server-initiated approval, user-input, MCP elicitation, and auth-refresh
  requests map into `ApprovalBroker` or adjacent request traits;
- how thread ids, session ids, turn ids, cwd, workspace roots, permission
  settings, rate-limit state, and app-server process loss are stored and
  recovered by the daemon;
- which normalized event categories are sufficient for dashboard projection and
  ws.agent result handling.

## Deferred Custom Harness Work

The earlier custom substrate ideas remain valuable, but they are no longer the
recommended first implementation order. Durable event logs, context ledgers,
compaction, ToolHost-style capability routing, READ directives, and custom
model loops should be reintroduced behind the same object-trait boundary after
the daemon-owned app-server adapter proves the session and authority contracts.

This means:

- JSONL remains useful as a transport or diagnostic format, not necessarily as
  the primary first-slice process protocol.
- ToolHost becomes part of the capability-broker direction rather than a
  separate first milestone.
- Dashboard integration starts from daemon-owned approval and projection rather
  than read-only replay of standalone harness JSONL files.
- A future native backend can replace Codex app-server without changing
  dashboard, ws-mcp, or ws.agent public contracts.

## Rejected Or Deferred Directions

Do not make Codex app-server protocol objects the ws public contract.

Do not build the first product path by driving the Codex TUI through a PTY.

Do not make `ws-harness` a stateful session owner in the normal path. It may
host a local fallback, but the stable path should proxy to a daemon-owned
runtime.

Do not rely on dashboard-context detection, MCP profiles, or prompt rules as
hard security boundaries.

Do not try to remove every dangerous ws-mcp local capability before adding the
broker path. Insert hooks first, then migrate call paths progressively.

Do not start by building the full custom run loop, compaction engine, and
ToolHost before proving daemon-owned session lifecycle and approval routing.

## Next Ticket Direction

Create actionable spikes only after selecting the owning package boundaries.
Likely children:

- Codex app-server adapter spike: thread/turn lifecycle, normalized events, and
  native capability disabling or containment.
- Daemon approval broker spike: pending request store, websocket projection, and
  request resolution protocol.
- `ws-harness` proxy spike: daemon discovery, remote backend client, and local
  fallback policy.
- ws-mcp broker hook spike: `brokered-if-available` detection around `exec.*`
  and agent call paths.
- ws.agent harness backend spike: sustained session reuse until erase or
  retirement.
