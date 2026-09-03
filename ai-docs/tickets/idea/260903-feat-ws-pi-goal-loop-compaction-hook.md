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

## Resolved design (2026-09-03 discussion)

**Signal shape — explicit skill calls, zero prose parsing.** State transitions
happen ONLY through model-invoked skills; the absence of any call means the loop
continues. No response-text marker parsing — a deliberate departure from the
opencode four-token design and a sharper contract than the Claude prose-judged
loop (which relied on the harness Stop-hook plus the agent simply stopping). The
three levers:

- `/goal-achieved <summary>` — terminal; goal met, end the run.
- `/goal-blocked <reason>` — terminal; end the run with a blocker report.
  (Whether the blocker is also written to durable ticket state — the Claude-native
  `260723` mechanism that survives compaction — is deferred to promotion.)
- `/goal-compact-and-continue <carry-forward-prose>` — non-terminal; the prose is
  passed as `ctx.compact({ customInstructions })`, then the loop re-enters the
  next goal turn.
- (no call) — default; `agent_settled` re-injects a continue turn and the agent
  keeps working.

**Arming (Claude-parity, minimal).** Entering goal mode injects an announcement
turn ("Goal settled: <goal>"), matching Claude's own `/goal` surface — no branch
or state substrate beyond an active-goal marker. The `agent_settled` handler is
armed ONLY while a goal is active; a settle outside goal mode is an ordinary stop
(this is what stops every normal Pi session from looping forever). Each loop
re-fire re-injects a reminder carrying the goal and the levers, e.g. "Goal yet
running … <goal> … call /goal-achieved | /goal-blocked |
/goal-compact-and-continue for a state transition."

**Runaway backstop.** Well-behaved agents self-terminate via achieved/blocked;
as a backstop, N consecutive re-fires with no tool call force-stop the goal.
Claude's own goal loop force-stops around ~10 consecutive no-tool-call re-fires —
mirror that threshold, config-tunable.

**Compaction ownership — model-driven.** The extension surfaces the current
`getContextUsage().percent` in the continue turn as information; the model decides
whether to call `/goal-compact-and-continue`. The extension does NOT autonomously
compact. Pi's own overflow auto-compaction stays as the last-resort backstop.
Config knobs: (a) the compaction advisory point (the `percent` at which the
reminder nudges the model to compact), and (b) a context-window / max-token
override. `session_before_compact` remains the companion surface for injecting ws
state into a compaction and detecting Pi's own `reason: "threshold"` signal.

## Remaining open questions (post-2026-09-03)

Resolved above: judgment signal shape (explicit skills, no prose parsing),
arming (active-goal announcement + armed-only-in-goal-mode), loop guard (N
consecutive no-tool-call re-fires force-stop), and compaction ownership
(model-driven with `percent` surfaced + config knobs + Pi overflow backstop).
Still open:

- **Durable goal state across compaction.** The Claude-native ancestor
  (`260723`) leaned on durable on-disk ticket state to survive compaction; how
  much of that carries over to the Pi loop, and whether `/goal-blocked` writes a
  durable blocker record.
- **Compression-safety heuristic placement.** In the model-driven design the
  "phase boundary / merge gate = safe to compact" heuristic becomes advisory
  prose the model weighs (not an extension gate) — confirm this is sufficient, or
  whether `session_before_compact` should still veto an unsafe compaction.
- **Interaction with subagent RPC children** (`260903-feat-ws-pi-subagent-rpc-ux`):
  does the goal-loop run only on the lead session, or also drive settle/compact
  on children?
- **Config surface.** Where the tunable knobs live (runaway threshold, compaction
  advisory point, context-window override) — ws config vs Pi settings vs
  extension constants.

## Non-goals

- Building it in this ticket — capture + feasibility + protocol framing only.
- Changing ws-mcp; the loop is adapter-local.
- Reproducing the opencode four-token marker verbatim (explicitly discarded).
