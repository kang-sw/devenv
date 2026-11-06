---
title: ws dashboard Claude adapter - event probe, live event feed, and message submission
parent: 261106-epic-ws-dashboard-serve-daemon
related:
  261106-feat-ws-dashboard-daemon-core: its Phase 1 lifecycle may run alongside this ticket's Phase 1 probe; its Phase 2 onward waits for the probe (266)
  261106-feat-ws-dashboard-live-snapshot-pi: prerequisite for Phase 2; Phase 2 feeds its Phase 1 intake and produces its snapshot schema; this ticket's Phase 1 probe is in turn a prerequisite for all of that ticket (266)
  261106-feat-ws-dashboard-browser-interaction: prerequisite for Phase 3; the Claude drain and inject uses its board-answer inbox, drain tool, and `mailbox wait` inbox flag; this ticket's Phase 1 probe runs before that ticket (266)
---

# ws dashboard Claude adapter - event probe, live event feed, and message submission

## Background

The parent epic `261106-epic-ws-dashboard-serve-daemon` shows each session's
lead and subagent activity live and lets the browser send user messages to
the session through host adapters. On Claude the adapter is the Claude mod
(`agents-plugin/hooks/claude-mod/`). Whether the mod receives the events a
live view needs is not yet known, so Phase 1 probes it, and the probe runs
before any other child's work except the core's Phase 1 lifecycle (266).
Later phases build the adapter on that result: feeding the ws-mcp intake of
`261106-feat-ws-dashboard-live-snapshot-pi`, and the Claude side of message
submission (drain and inject) (213). Read the epic's Cross-Child Decisions
for shared context; the decisions below bind this child.

What is verified so far: subagent `turn.start` events carry an `agentId`,
and the main thread already delivers `turn.start`, `turn.step`,
`tool.call`, and `turn.complete`. The mod already calls the session's ws MCP
tools (`$.mcp.connect` and `$.tool.call` in `register.ts`) and already
spawns `mailbox wait` and flushes mail at turn end in `watcher.ts`.

## Decisions

- **Claude adapter probe first.** Phase 1 probes which events the Claude mod
  receives: subagent tool-call and generating-phase events (196), the main
  thread's generating-phase events (218), and the lead's reply text and
  tool-permission waits (241). It runs before any other child's work except
  the core child's Phase 1 lifecycle, which depends on no probe result and
  may run alongside it (266). If subagent events are unavailable, the Claude
  live view falls back to session-level state (193 a); if main-thread phase
  events are unavailable, the lead shows tool-call granularity only (218).
  The fallback for missing reply-text or permission-wait events is settled
  with the probe result, not before it. (196), (201), (290) A proposal to
  weigh then, not a decision: finished reply text shown when the message
  ends, and a running tool shown with no approval marker. A failed probe is not a drop trigger
  for the epic, because (190) and (196) define the fallback. (200)
  Rejected: holding the core lifecycle phase until the probe lands (266 b),
  which serializes a phase that reads no snapshot.
- **One snapshot schema.** The Claude adapter produces the same snapshot
  shape as Pi's batch model (`BatchView`, caps 5 batches, 5 tool names, 200
  preview characters). (195) The live content, lead root fields, tree
  retention, and native-agent-id keying are the live-snapshot child's
  decisions (193), (216), (217), (244), (245), (247); the adapter fills them
  as far as the probed events allow.
- **Adapters hand events to their session's ws-mcp.** The mod does not post
  to the daemon. Rejected: adapters posting to the daemon directly.
  Discovery, token, and restart handling would then be written twice in
  TypeScript instead of once in ws-mcp. (191)
- **Subagent tree keying.** The tree is keyed by the host's native agent id;
  a delegate holding a ws session key is labelled with its reply-id and the
  session key never reaches the browser. (192), (243)
- **Permission waits.** When the lead waits on a tool permission prompt, the
  conversation and the lead bar show that it is waiting for approval in the
  terminal, with the tool and its argument preview; the approval itself
  stays in the terminal. (241) Rejected: permission approval and turn
  interrupt from the browser (241 b); they would let the LAN token authorize
  tool execution.
- **Injection with user authority.** The adapter injects a drained message
  as the user's own message (`$.prompt.submit`) with a short label naming the
  whiteboard, so it carries user authority. (210) Rejected: an adapter
  notice without user authority (210 b), which makes the user confirm their
  own answer twice.
- **Hold until turn end.** A message that arrives while the turn runs is
  left undrained in the inbox and submitted by the watcher's turn-end flush,
  so merge and cancel act on it until then. Because `mailbox wait` only
  peeks, the adapter re-arms it without the inbox flag while an entry is
  held and adds the flag back after the turn-end drain. (212) Rejected:
  reusing the Claude watcher's tool-boundary mail flush, which injects
  mid-turn; injecting into a running turn.

- **A `claude.*` knob gates the dashboard feed and submission.** Like every
  Claude mod feature, the feed and the drain and inject are selected by a
  `claude.*` knob with per-package defaults: on in ws, off in wsflow. (291)
  Rejected: no knob, which would turn the feature on in the conservative
  distribution too.

## Constraints

- Shipped surface: the Claude mod ships downstream in both packages; no
  text in it may name this repository's tickets, commits, or layout.
- `agents-plugin/hooks/claude-mod/` is the one source;
  `agents-plugin-wsflow/hooks/claude-mod/` must stay a byte-identical copy
  (`TestWsflowHooksMirrorUpToDate`).
- An intake or drain failure never breaks the mod's existing features
  (mailbox push, compaction timing, status band) or the session.
- Out of scope: the intake, snapshot, and ring themselves (live-snapshot
  child); the inbox, drain tool, and wait flag (browser-interaction child);
  rendering (core child).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Phases

### Phase 1: Claude mod event probe

Probe, in a live Claude session, which events the Claude mod receives for:
subagent tool calls and generating phases; the main thread's generating
phases (thinking, replying, tool call streaming); the lead's reply text,
streamed or only when finished; tool-permission waits, with the tool and
its arguments; and what identifies a subagent across its events. The Result
lists each event class as available or not, with the event names and
payload fields observed, and states which fallback (193 a), (218) applies.
For missing reply-text or permission-wait events, the Result records what
the mod does receive so the fallback can be settled with it. Probe code need
not ship; any that does follows the mirror constraint.

Verification: a live-host probe, not a unit test. The lead runs a Claude
session with the probe mod loaded and drives one turn that spawns a
subagent making tool calls, streams a reply, and triggers a tool permission
prompt, then checks the probe's logged events against the Result's table.
If probe code is committed, `go test ./internal/wsrsrc/` in
`agents-plugin-tool` passes (hooks mirror).

### Phase 2: Claude live event feed

Build the adapter on the Phase 1 Result: the mod hands lead and subagent
events to its session's ws-mcp intake (live-snapshot child, Phase 1), in the
snapshot schema, with finished user messages (origin), lead replies, and
turn summaries for the conversation ring, applying the fallbacks the Phase 1
Result selects. The `claude.*` dashboard knob (291), declared in both
packages' manifests with ws on and wsflow off, gates the feed; the worker
records its name in the Result.

Verification: tests in `agents-plugin/hooks/claude-mod/` (extending
`engine.test.ts` and `watcher.test.ts` or a new test file beside them) cover
the mapping from each probed event to intake calls, subagents keyed by
native agent id, the fallback paths for unavailable event classes, and an
intake failure leaving the mod's other features working; the wsflow copy
updated and `go test ./internal/wsrsrc/` in `agents-plugin-tool` passing.
The lead checks by hand with a live Claude session and a running daemon that
the lead bar, subagent tree, and conversation follow the session at the
granularity the probe allows.

### Phase 3: Claude drain and inject

Arm the mod's existing `mailbox wait` child with the inbox flag. On an inbox
wake while no turn runs, drain through the drain tool and inject the message
with `$.prompt.submit` and the whiteboard label; while a turn runs, leave
the entry held, re-arm without the inbox flag, and at the watcher's
turn-end flush drain, inject, and re-arm with the flag. The same knob (291)
gates submission: when it is off, the wait is armed without the inbox flag.

Verification: tests in `agents-plugin/hooks/claude-mod/` (extending
`watcher.test.ts` and `engine.test.ts`) cover an idle wake injecting at once,
a mid-turn wake re-arming without the flag and injecting only at turn end,
no injection at a tool boundary, the flag restored after the drain, a
cancelled entry never injected, and a merged entry injected once; the wsflow
copy updated and `go test ./internal/wsrsrc/` in `agents-plugin-tool`
passing. The lead checks by hand with a live Claude session: a composer
message sent while idle starts a turn as user input with the whiteboard
label; one sent mid-turn shows as held, can be cancelled or merged, and is
submitted when the turn ends.
