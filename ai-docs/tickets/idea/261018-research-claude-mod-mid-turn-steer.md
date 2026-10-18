---
title: Steer mod prompts into a running Claude Code turn instead of queuing a follow-up turn
related:
  261018-feat-claude-mod-mid-turn-steer-delivery: the actionable feature derived from this ledger
  261017-feat-claude-mailbox-active-push-mod: the mailbox push whose mid-turn mail waits for the turn to settle
  261017-feat-claude-mod-lead-compaction-timing: the compaction lever whose continuation queues behind a prompt typed during compaction
---

# Steer mod prompts into a running Claude Code turn instead of queuing a follow-up turn

## Background

The ws Claude mod (`agents-plugin/hooks/claude-mod/`) delivers every prompt
it originates through `$.prompt.submit`. The user observed (2026-10-18) two
symptoms of that choice:

- A compaction continuation lands behind a message the user typed during the
  compaction: the user's turn runs first and the continuation becomes a
  follow-up turn after it, so the continuation keeps getting pushed back.
- Mailbox mail arriving mid-turn waits for the whole turn to settle before
  the model sees it.

Claude Code's own mid-turn user input behaves as a steer: a message typed
during a turn is delivered inside that turn alongside the next tool result.
The question was whether a mod can do the same, at each tool finish rather
than at turn settle, and whether a mod's continuation can take precedence
over a user prompt queued during compaction.

## Probe

Two delegated probes on 2026-10-18, Claude Code 2.1.302, macOS, haiku: a
throwaway plugin holding one probe mod (plus a second "observer" plugin for
hook-visibility questions), headless `claude -p` runs and interactive tmux
sessions, and a logging reverse proxy behind `ANTHROPIC_BASE_URL` recording
each API request's messages as ground truth. The compaction cases fired
`$.session.compact()` from `turn.complete`, as the production lever does.
Mod API declarations: the bundled plugin-authoring skill's
`types/claude-code.d.ts` and `reference.md` (extracted under
`/private/tmp/claude-501/bundled-skills/2.1.302/`). Logs:
`/tmp/steerprobe/runs/` (volatile).

### Delivery paths

- `$.session.append({ message: { type: 'user', content: [{ type: 'text',
  text }] } })` steers. Called while a main-thread tool ran, the row
  reached the very next request of the same turn as an extra text block in
  the user message carrying the tool_result; no new turn started
  (reference.md: "The row is in the list at once and in a running turn's
  requests from the loop's next top"). There is no framing wrapper, so the
  text must say what it is. It worked from a `$.clock.after` timer started
  inside a still-running `tool.call` hook. Limits: it never wakes an idle
  session (the row waits and prepends to the next prompt), and a row
  appended during a text-only stretch with no later model request in the
  turn is not delivered in that turn and starts nothing at turn end.
- A `tool.call` hook returning `{ ...await next(e), context: [text] }`
  steers at the tool boundary only, framed `tool.call hook additional
  context: <text>`. haiku read it as hook output with "no instruction to
  follow" and took no action.
- `$.session.send` to the session's own id is refused ("no live session on
  this machine has id ...").
- `$.prompt.submit` is follow-up only. Its arguments are `{ text,
  attachments, asUser }`; there is no priority, mode, or timing field. A
  submit from a timer inside a still-running `tool.call` hook is rejected
  (it would wait on the turn the hook holds).
- `prompt.attachment` and `prompt.context` only rewrite text the engine
  raises or the once-per-conversation context; neither creates a message
  (declarations, not probed).

### Compaction ordering

- A prompt the user types during `$.session.compact()` is held and released
  10-20 ms after the compaction resolves; its `prompt.submit` event precedes
  the mod's continuation submitted at the same instant, so the user's turn
  runs first (6/6).
- `prompt.submit` hooks run when a queued prompt is dequeued (an observer
  plugin's hook ran right before the prompt's `turn.start`), but the calling
  plugin's own hook never sees its own `$.prompt.submit` (d.ts: "It goes
  through every hook but the calling one").
- A `prompt.submit` hook in another plugin drops a prompt with `return {
  drop: '<reason>' }`; the submitter's `$.prompt.submit` resolves to `{ drop
  }` and no turn starts.
- A mod `prompt.submit` hook can attach text to a user prompt with
  `next({ ...e, context: [...(e.context ?? []), text] })`; the text reached
  that turn's request after the user text.
- Deferred continuation: at compaction resolve, set a pending continuation
  and submit nothing; the mod's `prompt.submit` hook attaches it as
  `context` to the next composer-origin prompt and marks it consumed; a
  1500 ms timer submits it only when not consumed. User typed during
  compaction: 3/3 one turn carrying the user text plus the continuation, no
  extra turn. Nobody typed: 3/3 exactly one continuation turn.
- Immediate submit plus `$.turn.abort` of the mod's own turn when a user
  prompt already consumed the continuation worked once, but the aborted
  prompt still issued one model request and stays visible in the transcript
  as an unanswered prompt.

## Outcome Ledger

### Verified Findings

- `$.session.append` of a user text row during a running main-thread turn
  reaches that turn's next model request, with no new turn and no framing
  (probe r1, r4, interactive; reference.md line ~170).
- An appended row wakes nothing while idle, and is not delivered in the turn
  when no model request follows the append (interactive probe).
- `tool.call` result `context` steers at the tool boundary but reads as hook
  output; haiku did not act on it (r3, r4).
- `$.session.send` to self is refused (r2).
- `$.prompt.submit` takes only `{ text, attachments, asUser }`, is follow-up
  only, and is rejected from inside a running `tool.call` hook (d.ts ~3010,
  ~9369; probe).
- A prompt typed during compaction is released 10-20 ms after
  `session.compact` resolves and runs before a continuation submitted at
  resolve (6/6, trial-a1, trial-b1, defer runs).
- The calling plugin's own `prompt.submit` hook never sees its own
  `$.prompt.submit`; another plugin's hook sees it at dequeue and can drop
  it with `{ drop }` (trial-b1, trial-b2; d.ts ~3010).
- A mod `prompt.submit` hook can attach `context` to a user prompt (z-ctx).
- The deferred-continuation design gave one correct turn in 3/3 typed and
  3/3 idle runs (trial-d-*).

### Confirmed Decisions

- Mailbox mail arriving during a main-thread turn is delivered at the next
  main-thread tool boundary through `$.session.append`; mail still queued at
  turn end is submitted as today, and idle delivery stays `$.prompt.submit`
  (user, 2026-10-18).
- Probe the compaction-ordering mechanism, then derive one feat ticket and
  promote it to ready (user, 2026-10-18); its compaction design is settled in
  that ticket's Decisions.

### Proposals

- Deferred continuation with a short consume window (1500 ms in the probe)
  for the compaction lever; its worst case (a user prompt after the window)
  equals today's follow-up behavior.

### Open Questions

- Does an opus-class model weight a raw appended row like the engine's own
  steer wrapper?
- How does the deferred window behave under a long compaction or a user
  prompt landing near the window's end (not stress-tested)?
- Would a `session.compact` hook returning messages that carry the
  continuation be a race-free alternative (d.ts ~4494, not probed)?
- Does a prompt typed during an MCP reconnect hold and overtake the
  reconnect lever's continuation the same way (not probed)?
- Subagent targeting through `session.append`'s `agentId` (not probed).

### Rejected Alternatives

- Dropping the mod's own queued continuation from its own `prompt.submit`
  hook: impossible, the calling plugin's hook skips its own submits.
- Immediate submit plus `$.turn.abort`: wastes one model request and leaves
  a visible unanswered prompt.
- `$.session.send` to self: refused by the runtime.
