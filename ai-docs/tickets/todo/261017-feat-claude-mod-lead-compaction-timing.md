---
title: Claude lead compaction timing through a ws plugin mod
related:
  261016-research-claude-mod-pi-like-compaction: source research; its Outcome Ledger Confirmed Decisions are this ticket's contract except where a Decision below records a reversal
  261017-feat-claude-mailbox-active-push-mod: first Claude mod; validated the mod path (hooks plus modules in one hooks.json, prompt.submit turns, $.store reload survival) and supplies the main-session-only tool precedent
  261006-feat-pi-compaction-advisory-standing-intent-milestones: pi advisory/milestone/latch semantics this ticket ports
  261004-feat-pi-explicit-post-compaction-continuation: pi post-compaction continuation this ticket ports
  261002-feat-pi-lead-ws-owned-compaction: pi lever and hard point this ticket maps onto Claude
---

# Claude lead compaction timing through a ws plugin mod

## Background

On pi, the adapter owns lead compaction timing: an advisory at a threshold
turns compaction into a standing intent, interim milestones keep it in view,
the lead compacts at a boundary of its own choosing through a ws lever, and
an explicit continuation resumes the work. Dogfooding found this timing the
pi feature with the largest observed effect.

Claude Code only has native auto compaction and the user's `/compact`; ws
merely observes compaction through the `SessionStart(compact)` revive nudge.
The research `261016-research-claude-mod-pi-like-compaction` settled that a
`ws`-only plugin mod reproduces pi's timing, not pi's summary ownership. The
mailbox push mod (`261017-feat-claude-mailbox-active-push-mod`) has since
validated the mod path live (2026-10-17 dogfood: hooks and modules in one
`hooks.json` both load, `$.prompt.submit` starts a turn in an idle session,
the binding survives `/reload-plugins` through `$.store`, the push loop stops
on `/clear`, and the revive nudge plus binding survive `/compact`).

Mod API facts used below are from the Claude Code 2.1.302 declaration file
(`plugin-authoring` skill, `types/claude-code.d.ts`):

- `session.measure` fires after each main-thread turn with
  `$.session.usage()`'s figures pushed (`context.tokens`, `context.window`,
  `context.percent`).
- `$.session.usage({ breakdown: "summary" })` estimates locally and itemizes
  rows by `kind`; the `buffer` row is the compaction reserve, so the native
  auto-compact point is the window minus that row.
- `$.session.compact({ instructions })` runs a `plugin`-trigger compaction
  between turns, resolves `{ skip }` on a hook veto, and rejects while a turn
  runs.
- A `tool.call` hook may await `next(e)` and return `{ result }` in its place,
  so text can be appended to a tool result the model reads mid-run.
- `$.tool.register` declares a model-callable tool served by the mod.

## Decisions

- **Ships in `ws` only** as a second module in `agents-plugin/hooks/hooks.json`,
  in its own directory under `agents-plugin/hooks/`, with no wsflow copy.
  `wsflow-mirroring.md`'s Claude Mod Mirror section gains one line naming it
  ws-only; `TestWsflowHooksMirrorUpToDate` is unchanged (it covers
  `mailbox-push/` only).
- **Advisory threshold is a fixed 50%** of the context window in this first
  step, with no config key (a config read would need an MCP tool call the mod
  otherwise does not make).
- **The hard point is Claude's native auto compaction**, never vetoed or
  preempted by the mod. Rejected: vetoing `auto` below a ceiling (research
  Rejected Alternatives).
- **The advisory is a dedicated wake turn.** When a `session.measure` after a
  main-thread turn shows the crossing, the mod `$.prompt.submit`s the
  advisory, which starts its own turn once idle (pi 261006's advisory
  delivery).
- **Milestones ride the next main-thread tool result.** A milestone is
  appended to the result of the next main-thread tool call through the mod's
  `tool.call` hook after `next(e)`; it is never a submitted prompt, and one
  crossed when no tool call follows stays pending until one does. This
  reverses the research Confirmed Decision (submit at `turn.complete`), whose
  premise "Pi's advisory and milestone nudges already cost a turn each" is
  wrong for milestones: pi 261006 steers them only at a tool-result
  `turn_end` and rejected adding a model turn. Rejected: submitting at
  `turn.complete` (an idle wake turn whose only correct response is to end
  without replying); dropping milestones in the first step.
- **Milestone points** sit at one and two thirds of the way from the advisory
  to the native auto-compact point, which the mod derives as the window minus
  the `buffer` row of `$.session.usage({ breakdown: "summary" })`. The row is
  read only when the window size changes and cached per size. Rejected: a
  breakdown call per measure (the plain usage call stays free).
- **Latch semantics port pi 261006:** one latch over the ordered thresholds
  (advisory, first milestone, second milestone); one message per observation,
  advisory before milestone; a milestone waits for the advisory; usage
  observed below the latch re-arms it; any landed compaction resets it; and
  session start or resume baselines it from current usage.
- **The lever.** A mod-registered tool named `request_compaction` (in prose,
  "the compaction request tool") taking `{ continuation: string }`
  (non-empty, one line) records a compaction request and returns at once; a
  call carrying `agentId` is refused (main session only, as the mailbox mod's
  `checkWatchCall`). At the next main-thread `turn.complete` the mod calls
  `$.session.compact()` with no instructions and, on success,
  `$.prompt.submit`s the continuation. Rejected: compacting inside the tool
  call (`compact()` rejects while a turn runs); naming the tool `compact`
  (invites expecting the context to be gone when the tool returns).
- **A compaction landing elsewhere clears the pending compact step.** The
  mod's own `session.compact` hook, when `next(e)` resolves without `skip` for
  a `manual` or `auto` trigger or a `plugin` call not made by this lever,
  clears a pending lever request's compact step, so the lever never compacts
  right after a native compaction. `precompute` is ignored: it is the one
  dispatch that installs nothing (2.1.302 `SessionCompactTrigger`). The same
  observation resets the latch. Rejected: inferring a landed compaction from a
  usage drop at `turn.complete` (indirect).
- **A pending request survives a rejected `compact()`** (a turn started first,
  for example a mailbox push) and retries at the next main-thread
  `turn.complete`. Rejected: a timer retry (turn boundaries are the only
  moments `compact()` can succeed; native auto compaction bounds starvation
  under steady mail).
- **The continuation is always submitted.** When another compaction landed
  first or a hook vetoed with `{ skip }`, the compact step is dropped, a
  user-visible `$.ui.log` line says why, and the continuation is still
  submitted at the next `turn.complete`; on a veto it is prefixed with a
  pinned one-line note that compaction was skipped. Rejected: dropping the
  whole request (leaves an idle lead with nothing to resume); keeping the
  continuation only when another compaction landed first.
- **Revive stays with the existing `SessionStart(compact)` nudge**; the
  continuation carries no revive instruction. Rejected: folding a revive into
  the continuation (double revive prompts).
- **No `$.session.version()` gate.** A broken mod API leaves no lever and
  native auto compaction untouched (as 261017-feat-claude-mailbox-active-push-mod).
- **The prose is pinned in this ticket** (advisory text, milestone line, lever
  tool description and result text, the veto note, the continuation framing)
  and asserted verbatim by tests, placeholders interpolated (261006
  precedent).

## Pinned Prose

Asserted verbatim by tests. `{percent}` and `{n}` are rounded current usage,
`{hard}` the rounded native auto-compact point, `{continuation}` the lever
argument, `{reason}` the vetoing hook's skip reason. Adapted from pi 261006's
pinned prose: the hard point is Claude's automatic compaction, and pi's
lead-written summary preparation becomes "update durable records, then call
the tool" because the engine writes the summary.

Advisory:

```text
Context usage is {percent}% of the window (advisory point: 50%). Claude Code compacts automatically at about {hard}%.

This is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.

From here on, look for a good moment to compact before {hard}%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, first bring durable records up to date with anything still needed that only this context holds, then call the compaction request tool with a one-line continuation and end your turn. Compacting at a moment you choose keeps the work coherent; at {hard}% it happens automatically, mid-work if need be. Brief context readings will follow on the way there.

If now is not such a moment, end this turn without replying and keep looking for one as you work.
```

Milestone line, appended to a tool result:

```text
Current context window: {n}% / {hard}% (automatic compaction point). Keep watching for a safe boundary to compact, as the advisory said; do not stop the current task for it.
```

Lever tool description:

```text
Request compaction of this conversation when the current turn ends. Before calling, bring durable records (tickets, notes, agenda) up to date with anything still needed that only this context holds. `continuation` is one line naming what to do next (for example "continue Phase 2 implementation"); it is sent as your next prompt after compaction. Main session only.
```

Lever tool result:

```text
Compaction requested. End your turn now without further tool calls; compaction runs when the turn ends, and your continuation resumes the work.
```

Continuation prompt, used whenever a compaction landed (the lever's own or
another that landed first):

```text
Continue after compaction: {continuation}
```

Continuation prompt after a `{ skip }` veto:

```text
Compaction did not run ({reason}); your context is unchanged.
Continue: {continuation}
```

The prefixes tell the lead the prompt is its own note, not a new user
instruction.

## Constraints

- Lead (main-thread) only: subagent turns neither advance thresholds nor may
  call the lever.
- The research's deferred Proposals (instructions injection on every trigger,
  a fully custom summary) are out of scope.

## Phases

### Phase 1: Advisory, milestones, lever, and continuation

Implement the Decisions in one landing in `agents-plugin/hooks/`.

Verification: `claude plugin test agents-plugin` passes with engine and unit
tests for each threshold, latch, lever, and retry behavior and with the pinned
prose asserted verbatim; then a live dogfood checks the advisory wake turn,
a milestone appended mid-run, a lever compaction followed by exactly one
continuation turn carrying the revive nudge, and lever retry while a mailbox
push holds the session.
