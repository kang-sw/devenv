---
title: "Pi subagent interaction: persistent RPC children (send-message, transcript, resume)"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor; lists RPC/re-entry primitives as post-MVP expansion surface
  260902-feat-ws-pi-native-mvp: ships the one-shot `-p` spawner this ticket upgrades
  260903-research-ws-pi-adapter-npm-distribution: gap #6 (pi CLI resolution for spawned children) is a shared prerequisite
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
---

# Pi subagent interaction: persistent RPC children (send-message, transcript, resume)

## Background

The MVP spawner (`agents-plugin-pi/src/spawner.ts`, shipped by
`260902-feat-ws-pi-native-mvp`) dispatches every delegated agent as a **one-shot**
`pi -e <ext> -p "<prompt>" --no-session` subprocess with `stdin` ignored, parsing
streamed JSON events off stdout. This is the shape Pi's own subagent example
uses, and it is deliberately one-shot: the child runs one prompt and exits, so
the lead cannot send a follow-up into a running child, read a child's
transcript, or resume a child session by id.

The user wants Claude-CLI-level subagent ergonomics: **send additional messages
into a live child, and open a child's transcript.** The Pi API survey
(2026-09-03) confirmed Pi supports exactly this through **RPC mode** — the
capability is present in the installed build, so this is framework work, not a
Pi-core gap.

The golden rule holds: ws-mcp Go source is never modified; the dependency stays
one-directional (adapter -> ws-mcp).

## Feasibility (evidence, installed Pi build)

From `@earendil-works/pi-coding-agent` type defs (`dist/modes/rpc/`):

- **`RpcClient`** (package top-level export, `dist/modes/rpc/rpc-client.d.ts`):
  spawns a long-lived child (`start()`/`stop()`), and exposes
  - send-into-running-child: `prompt()`, `steer()` (interrupt mid-run),
    `followUp()` (queue after current run), `promptAndWait()`
  - transcript reads: `getMessages()`, `getEntries(since?)`,
    `getLastAssistantText()`, `getTree()`
  - lifecycle: `newSession()`, `switchSession(path)`, `fork(entryId)`,
    `clone()`, `compact()`, `abort()`, `setModel()`, `getSessionStats()`
  - `args?: string[]` construction option, so `--session <id>` can resume a
    specific session and then drive it live.
- **Wire protocol** alternative: `pi --mode rpc`, JSONL commands on stdin
  (`RpcCommand` union in `dist/modes/rpc/rpc-types.d.ts`: `prompt`/`steer`/
  `follow_up`/`get_messages`/`get_entries`/`get_tree`/`switch_session`/`fork`/
  `compact`/...).
- **Session resume** is a launch-time flag (`--session <path|id>`, accepts a
  partial id per `docs/sessions.md`); to keep driving the resumed session it
  must be launched in `--mode rpc` (a plain `-p` resume runs one prompt and
  exits — confirmed separately during the Phase 4 live-gate work).
- **In-process alternative:** the SDK `AgentSession` class
  (`dist/core/agent-session.d.ts`) offers `prompt()`/`sendUserMessage()`/
  `waitForIdle()`/`getContextUsage()` and its own event emitter, letting a child
  run inside the extension process with no subprocess — trades OS-process
  isolation for direct in-process message injection / transcript access.

## Proposed direction (idea — detailed UX TBD)

Replace (or augment) the one-shot `-p` spawner with a **persistent RPC child**
model so `ws-agent-spawn` yields a driveable handle, and add lead-facing
affordances to send a follow-up message into a running child and to open its
transcript. Whether the durable answer is out-of-process `RpcClient` (keeps the
MVP's context-isolation guarantee) or in-process `AgentSession` (simpler, no
subprocess, but shares the process) is an open fork below.

**Detailed UX is deliberately TBD** at idea stage: the exact ws tool/command
surface (how a follow-up is addressed to a specific child, how a transcript is
surfaced to the lead, how this maps onto the ws spawner tool names
`ws-agent-spawn`/`ws-agent-continue`/`ws-agent-wait`) is designed when this is
promoted, not fixed here.

## Open forks / questions

- **Fork — out-of-process `RpcClient` vs in-process `AgentSession`.** Isolation
  and parity with the current one-shot model (RpcClient) vs simplicity and
  direct transcript access (AgentSession). May be per-use-case rather than a
  single global choice.
- **cliPath resolution (shared with distribution gap #6).** `RpcClient` needs to
  locate the pi CLI entry (`cliPath`, defaults to searching `dist/cli.js`) to
  spawn children. From an *installed* extension this must resolve reliably; this
  is the same unknown as `260903-research-ws-pi-adapter-npm-distribution` gap #6
  and should be settled by that spike first.
- **`ws-agent-continue` mapping.** The MVP already exposes a continue tool; does
  it become a thin `RpcClient.followUp()`/`prompt()` call, and does that change
  its session-lineage/`session_key` semantics?
- **Lifecycle ownership.** A persistent child must be torn down on
  `session_shutdown` (the MVP already kills one-shot children there); RPC
  children add idle/abort lifecycle the extension must manage.

## Non-goals

- Building it in this ticket — capture + feasibility + fork framing only.
- Changing ws-mcp; the spawner upgrade is adapter-local.
