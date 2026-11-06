---
title: ws dashboard Claude adapter - event probe, live event feed, and message submission
parent: 261106-epic-ws-dashboard-serve-daemon
related:
  261106-feat-ws-dashboard-daemon-core: its Phase 1 lifecycle may run alongside this ticket's Phase 1 probe; its Phase 2 onward waits for the probe (266)
  261106-feat-ws-dashboard-live-snapshot-pi: prerequisite for Phase 2; Phase 2 feeds its Phase 1 intake and produces its snapshot schema; this ticket's Phase 1 probe is in turn a prerequisite for all of that ticket (266)
  261106-feat-ws-dashboard-browser-interaction: prerequisite for Phase 3; the Claude drain and inject uses its board-answer inbox, drain tool, and `mailbox wait` inbox flag; this ticket's Phase 1 probe runs before that ticket (266)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: ebe3e3a4870c3e0a
sage-review-completeness-reviewed: ebe3e3a4870c3e0a
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
for shared context; the decisions below bind this child. The `(nn)` numbers
are the parent epic's decision items.

What is verified so far: subagent `turn.start` events carry an `agentId`,
and the main thread already delivers `turn.start`, `turn.step`,
`tool.call`, and `turn.complete`. The mod already calls the session's ws MCP
tools (`$.mcp.connect` and `$.tool.call` in `register.ts`) and already
spawns `mailbox wait` (the watcher's `wait` edge, spawned through
`$.process.spawn` in `register.ts`,
`agents-plugin/hooks/claude-mod/register.ts#L655-L661`) and flushes mail at
turn end in `watcher.ts` (`turnCompleted`,
`agents-plugin/hooks/claude-mod/watcher.ts#L541-L544`).

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
  ends, and a running tool shown with no approval marker. After the Phase 1
  Result, the lead settles this fallback with the user and records it as an
  `#### Edition` on Phase 2; Phase 2 is not dispatched before that Edition
  exists, and the Phase 1 worker stops after Phase 1. A failed probe is not
  a drop trigger for the epic, because (190) and (196) define the fallback.
  (200) Rejected: holding the core lifecycle phase until the probe lands
  (266 b), which serializes a phase that reads no snapshot.
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

## Prior Decisions

- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, Decisions): "The intake is an adapter-only MCP tool. Adapters hand events to ws-mcp through an MCP tool whose short description marks it adapter-only, with compact text output. (289)" — bearing: supports
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, Decisions): "One snapshot schema. The snapshot carries agent activity in the shape of Pi's batch model (BatchView, caps AGENT_BATCH_CAP 5, BATCH_TOOL_NAME_CAP 5, BATCH_ARGS_PREVIEW_CAP 200). The Claude adapter produces the same shape." — bearing: constrains
- 261106-feat-ws-dashboard-live-snapshot-pi (2026-11-06, Decisions): "Three phases: Go intake, Pi producer, Pi submission. (294) The Claude adapter's feed phase waits only on the Go phase." — bearing: constrains
- fb514855 (2026-11-06, commit): "Children are captured later as separate lead-ticket invocations; the Claude mod event probe goes first." — bearing: supports
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, Decisions): "Mailbox mail steers at tool boundaries. Mail the watcher would submit while a main-thread turn runs is queued in the mod and flushed as one $.session.append user text row at the next tool boundary" — bearing: constrains
- 7fe04e8e (2026-10-18, commit): "the host refuses $.prompt.submit from inside a prompt.submit hook, and turn.complete is a permitted site, so the turn-end leftover submit runs inline." — bearing: constrains
- 261029-feat-claude-mod-package-parity-knob-defaults (2026-10-29, Decisions): "claude.* knobs carry per-package defaults: claude.manual_injection_enabled true in ws, false in wsflow; claude.mailbox_drain direct in ws, notice in wsflow" — bearing: supports
- 07976e68 (2026-10-31, commit): "close the post-drain crash gap: record-then-rethrow via the existing keep() over buffer-and-redeliver; out of scope: process/session death between recv and submit (needs a server-side peek/ack protocol)." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, watcher.ts, engine.test.ts, watcher.test.ts, agents-plugin/config-manifest.json, agents-plugin-wsflow/config-manifest.json, agents-plugin-wsflow/hooks/claude-mod/ mirror |
| scope.surface | cross-module | Claude mod calls the live-snapshot child's ws-mcp intake tool and the browser-interaction child's drain tool and inbox wait flag; new claude.* config key in both package manifests |
| scope.new_public_symbol | yes | a new claude.* dashboard knob in both config-manifest.json files, name left to the worker |
| scope.new_type_contract | unknown | the event-to-intake mapping depends on the Phase 1 probe result and the intake schema of 261106-feat-ws-dashboard-live-snapshot-pi Phase 1, neither landed |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/engine.test.ts, watcher.test.ts, agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | watcher wait edge and turnCompleted flush, $.mcp.connect and $.tool.call in register.ts, Pi BatchView caps in agents-plugin-pi/src/agent-batches.ts#L34-L40 |
| complexity.side_effect_risk | high | Phase 3 changes mailbox wait arming and turn-end prompt submission in a mod that also runs mailbox push, compaction, and captions |
| risk.correctness | high | hold-until-turn-end re-arming, merge and cancel of held entries, and once-only injection are race-prone around turn.complete |
| risk.fit | moderate | must not reuse the existing tool-boundary mail flush and must keep the wsflow mirror byte-identical |
| risk.test | moderate | Phase 1 and the hand checks are live-host only; unit tests cover only the mapping and watcher logic |
| risk.security_or_contract | high | $.prompt.submit injects browser-originated text with user authority, and the session key must never reach the browser |

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
not ship; any that does follows the mirror constraint. The worker stops
after this phase: the lead settles the reply-text and permission-wait
fallback with the user and records it as an `#### Edition` on Phase 2
before Phase 2 is dispatched.

Verification: a live-host probe, not a unit test. The lead runs a Claude
session with the probe mod loaded and drives one turn that spawns a
subagent making tool calls, streams a reply, and triggers a tool permission
prompt, then checks the probe's logged events against the Result's table.
If probe code is committed, `go test ./internal/wsrsrc/` in
`agents-plugin-tool` passes (hooks mirror).

### Phase 2: Claude live event feed

Build the adapter on the Phase 1 Result and the fallback Edition recorded
on this phase (not dispatched before that Edition exists): the mod hands
lead and subagent events to its session's ws-mcp intake (live-snapshot
child, Phase 1), in the snapshot schema, with finished user messages with
their origin tag (whiteboard or terminal), lead replies, and turn summaries
for the conversation ring, applying the fallbacks the Phase 1 Result and the
Edition select. The `claude.*` dashboard knob (291), declared in both
packages' manifests with ws on and wsflow off, gates the feed; the worker
records its name in the Result.

Verification: tests in `agents-plugin/hooks/claude-mod/` (extending
`engine.test.ts` and `watcher.test.ts` or a new test file beside them) cover
the mapping from each probed event to intake calls, subagents keyed by
native agent id, snapshot and ring fields that reach the browser never
carrying a ws session key, the intake payload carrying whatever identity the
live-snapshot child's Phase 1 Result specifies (which may be a session key
that ws-mcp relabels), the fallback paths for unavailable event classes, and an
intake failure leaving the mod's other features working; the wsflow copy
updated and `go test ./internal/wsrsrc/` in `agents-plugin-tool` passing.
The lead checks by hand with a live Claude session and a running daemon that
the lead bar, subagent tree, and conversation follow the session at the
granularity the probe allows. If the core's Phase 3 has not landed, the lead
inspects the snapshot and ring files directly instead, and the live-page
check happens once it lands.

### Phase 3: Claude drain and inject

Arm the mod's existing `mailbox wait` child with the inbox flag. On an inbox
wake while no turn runs, drain through the drain tool and inject the message
with `$.prompt.submit` and the whiteboard label; while a turn runs, leave
the entry held, re-arm without the inbox flag, and at the watcher's
turn-end flush drain, inject, and re-arm with the flag. The same knob (291)
gates submission: when it is off, the wait is armed without the inbox flag.
A crash between the drain's atomic claim and `$.prompt.submit` loses the
message, the same gap accepted for mail (07976e68).

The worker settles and records in the Result:

- The order or combination of a held board message and leftover mail that
  are both submitted at `turn.complete`, where the existing flush already
  submits mail inline. Constraint: both arrive; neither is dropped.

Verification: tests in `agents-plugin/hooks/claude-mod/` (extending
`watcher.test.ts` and `engine.test.ts`) cover an idle wake injecting at once,
a mid-turn wake re-arming without the flag and injecting only at turn end,
no injection at a tool boundary, the flag restored after the drain, a
cancelled entry never injected, a merged entry injected once, a held board
message and leftover mail both arriving at `turn.complete` in the settled
order, and a `$.prompt.submit`-injected message entering the ring tagged
`whiteboard`; the wsflow
copy updated and `go test ./internal/wsrsrc/` in `agents-plugin-tool`
passing. The lead checks by hand with a live Claude session: a composer
message sent while idle starts a turn as user input with the whiteboard
label; one sent mid-turn shows as held, can be cancelled or merged, and is
submitted when the turn ends.
