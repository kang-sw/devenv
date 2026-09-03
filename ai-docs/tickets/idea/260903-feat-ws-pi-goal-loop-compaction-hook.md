---
title: "Pi goal-loop + turn-end compaction judgment hook"
parent: 260605-epic-ws-playbook-factory-pivot
related:
  260802-research-ws-pi-native-framework: research anchor; scopes goal-loop as a post-MVP expansion surface on Pi's re-entry primitives
  260731-research-ws-opencode-drop-in-package: origin design of the judgment-turn / compression-safety-heuristic protocol (opencode prior art)
  260902-feat-ws-pi-native-mvp: MVP that explicitly deferred goal-loop + compaction hooks to follow-up tickets
  260723-feat-goal-step-rename-and-goal-loop-completion: Claude-native goal-loop ancestor (durable ticket-state loop) this reproduces on Pi
related-mental-model:
  - plugin-runtime
spec:
  - pi-adapter-runtime
---

# Pi goal-loop + turn-end compaction judgment hook

## Background

The ws-pi-native framework vision (`260802-research-ws-pi-native-framework`)
includes a **goal-loop**: after an agent run settles, inject a judgment turn that
decides whether the goal is achieved, whether to keep working, and — the piece
the user specifically wants — **whether now is a safe time to compact**. The MVP
(`260902-feat-ws-pi-native-mvp`) shipped bridge + spawner + model-catalog +
`/ws-discuss` PoC and **explicitly deferred goal-loop + compaction hooks** to a
follow-up ticket under the epic. This is that ticket.

The origin design is the opencode judgment-turn protocol
(`260731-research-ws-opencode-drop-in-package`): on turn-end, inject a judgment
turn carrying the current ws goal plus a **compression-safety heuristic** (phase
boundaries / lead-proceed merge gates are normally safe to compact; a turn that
stopped for a non-phase reason is unsafe to compact) and ask the model to emit a
token (`achieved` / `next-step` / `keep-working` / `pause`). `next-step` accepts
compaction then re-enters the next goal turn; `keep-working` resumes without
compacting. The research anchor states the marker protocol is **redesigned from
scratch on Pi** (Pi's `agent_settled` is a stronger primitive than opencode's
`session.idle`); the opencode four-token marker is prior art, not the target
shape.

Golden rule holds: ws-mcp Go source untouched; adapter -> ws-mcp only.

## Feasibility (evidence, installed Pi build)

From `@earendil-works/pi-coding-agent` type defs
(`dist/core/extensions/types.d.ts`) — every piece the designed loop needs is a
real API:

- **`agent_settled`** event (`types.d.ts:559-562`): "Fired after an agent run has
  fully settled and no automatic retry, compaction, or queued continuation will
  run." The truest judgment-turn trigger. (`turn_end` fires per model turn;
  `agent_end` once per run — `agent_settled` is the post-drain boundary.)
- **`getContextUsage()`** (`types.d.ts:243`) -> `{ tokens, contextWindow,
  percent }`: read how near the window we are. (`percent`/`tokens` are `null`
  right after a compaction until the next LLM response.)
- **`ctx.compact({ customInstructions, onComplete, onError })`**
  (`types.d.ts:245`): programmatically trigger compaction (see
  `examples/extensions/trigger-compact.ts`).
- **`session_before_compact`** (`reason: "manual" | "threshold" | "overflow"`,
  `willRetry`; handler result can `cancel` or supply a custom compaction result):
  detect/customize/veto an imminent compaction. `reason === "threshold"` is Pi's
  own "near compaction" signal. Companion events `session_compact` /
  `session_compact_failed`.
- Auto-compaction toggle is RPC-only (`set_auto_compaction`), not on the `pi.*`
  extension API — a caveat for any design that wants to disable Pi's built-in
  auto-compaction while the ws loop owns compaction timing.

## Proposed direction (idea — detailed UX/protocol TBD)

Register an `agent_settled` handler that, **gated by `getContextUsage().percent`**
(so a judgment turn is not injected on every settle — a refinement over the raw
opencode design), injects a ws judgment turn carrying the current ws goal + the
compression-safety heuristic, and on a compact-safe verdict calls
`ctx.compact()` then re-enters the next goal turn. `session_before_compact` is
the companion surface for injecting ws state / custom summary into the
compaction, and for the "is this a phase boundary?" safety check.

**Detailed UX and the marker/judgment protocol are deliberately TBD** at idea
stage: the exact judgment signal (a registered judgment tool call vs a parsed
marker token vs a structured response), the percent threshold, how the ws goal
is sourced (todo/agenda state?), and the loop-guard against runaway re-entry are
designed when this is promoted — the research anchor already mandates the marker
protocol be redesigned from scratch on Pi.

## Open questions

- **Judgment signal shape.** Registered judgment tool call (extension-owned) vs
  parsed marker token vs structured message — the anchor leaves this to the
  expansion phase.
- **Percent threshold + heuristic source.** What `percent` gates the judgment
  turn, and where the "phase boundary / merge gate = safe" signal comes from
  (ws todo/agenda/workflow_state vs the model's own read).
- **Loop guard.** Preventing runaway re-entry (goal turn -> keep-working -> goal
  turn -> ...) and interaction with Pi's own auto-compaction (which is RPC-only
  to disable).
- **Interaction with subagent RPC children** (`260903-feat-ws-pi-subagent-rpc-ux`):
  does the goal-loop run only on the lead session, or also drive settle/compact
  on children?
- **Durable goal state across compaction.** The Claude-native ancestor
  (`260723`) leaned on durable on-disk ticket state to survive compaction; how
  much of that carries over to the Pi loop.

## Non-goals

- Building it in this ticket — capture + feasibility + protocol framing only.
- Changing ws-mcp; the loop is adapter-local.
- Reproducing the opencode four-token marker verbatim (explicitly discarded).
