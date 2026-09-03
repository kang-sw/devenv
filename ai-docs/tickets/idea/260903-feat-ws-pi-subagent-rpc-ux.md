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

## Resolved design (2026-09-03 discussion)

Replace the one-shot `-p` spawner with **persistent `RpcClient` children** (mode
rpc, keeping the MVP's out-of-process context isolation) and expose this tool
surface:

| tool | backend | behavior |
| --- | --- | --- |
| `ws-agent-spawn(prompt, model_name?, model_effort?)` -> `{agent_id}` | `RpcClient.start()` | spawn a persistent, driveable child |
| `ws-agent-send(agent_id, message, interrupt?)` | `followUp()` (default, queue) / `steer()` (`interrupt: true`) | send into a live child; if `agent_id` is dormant, auto-resume via `--session` then deliver — **subsumes a separate resume tool** |
| `ws-agent-wait(agent_ids[], timeout?)` | select over the set | return the FIRST child to reach idle OR emit a report; carries `reason: idle\|report` and the child's last message auto-attached |
| `ws-agent-list()` | extension registry | live children, status, pending-report count |
| `ws-agent-stop(agent_id)` | `abort()` + `stop()` | teardown |
| `ws-agent-transcript(agent_id)` -> `{transcript_path}` | Pi session JSONL path | advanced/rare; lead greps/reads needed parts with fs tools — no content marshalling |
| `ws-report-to-lead(message)` (child-side) | RpcClient event stream | intermediate child->lead report before verdict; relayed to the parent, buffered per-agent, wakes a `ws-agent-wait` with `reason: report` |

Resolved forks:

- **send / resume unified** — one `ws-agent-send`; a dormant `agent_id`
  auto-resumes. No separate resume tool.
- **child->lead channel is asymmetric** — a dedicated no-arg `ws-report-to-lead`
  (a child has exactly one parent, so no addressing), NOT a symmetric `send`.
- **transcript is path-only** — return the session file path, not marshalled
  content; raw-transcript reads are rare and better served by the lead's own
  grep/fs tools.
- **wait is a select with report-wake** — an array of ids, first-finisher
  returns, and an in-flight `ws-report-to-lead` also wakes it (flagged via
  `reason`), so intermediate reports are not buried until the next poll.

## Remaining open questions (post-2026-09-03)

Resolved above: the full tool surface, send/resume unification, the asymmetric
`ws-report-to-lead` channel, path-only transcript, and wait-as-select with
report-wake. `ws-agent-continue` from the MVP folds into `ws-agent-send`. gap #6
(cliPath resolution) is settled by the distribution spike — `process.argv[1]`
already yields the correct installed CLI entry. Still open:

- **`RpcClient` vs in-process `AgentSession`.** Default is out-of-process
  `RpcClient` (isolation parity with the MVP); whether a lightweight in-process
  `AgentSession` variant is worth offering per-use-case is deferred.
- **session_key lineage.** How the extension maps a ws `session_key` <-> Pi
  session id in its registry so an auto-resume (`ws-agent-send` to a dormant id)
  restores the same ws lineage; and whether a resumed child keeps or re-mints its
  `session_key`.
- **Lifecycle / reaping.** Teardown on `session_shutdown` (the MVP already kills
  one-shot children); persistent children add idle-timeout auto-reap to prevent
  leaked processes, plus the abort/idle lifecycle the extension must own.
- **Report buffering semantics.** Per-agent report buffer bounds, ordering, and
  whether `ws-agent-wait` returns one report or drains all pending on wake.

## Non-goals

- Building it in this ticket — capture + feasibility + fork framing only.
- Changing ws-mcp; the spawner upgrade is adapter-local.
