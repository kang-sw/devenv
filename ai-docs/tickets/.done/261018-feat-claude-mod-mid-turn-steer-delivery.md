---
title: Deliver Claude mod mail and compaction continuations into the running turn
related:
  261018-research-claude-mod-mid-turn-steer: Outcome Ledger this ticket derives from (evidence and confirmed decisions)
  261017-feat-claude-mailbox-active-push-mod: the mailbox push watcher whose delivery this changes
  261017-feat-claude-mod-lead-compaction-timing: the compaction lever whose continuation delivery this changes
  261018-feat-claude-mod-lead-key-adoption-mailbox-auto-bind: latest change to the same register.ts wiring (auto-bound push)
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: c7f9cf258963222e
sage-review-completeness-reviewed: c7f9cf258963222e
completed: 2026-10-18
---

# Deliver Claude mod mail and compaction continuations into the running turn

## Background

The ws Claude mod (`agents-plugin/hooks/claude-mod/`) delivers every prompt it
originates through `$.prompt.submit`, which only queues a turn of its own once
the session is idle. Two user-visible results (user, 2026-10-18): mailbox mail
arriving mid-turn waits for the whole turn to settle, and a lever compaction's
continuation runs behind a message the user typed during the compaction, as a
follow-up turn. `261018-research-claude-mod-mid-turn-steer` probed the mod
API: `$.session.append` reaches a running turn's next model request, and a mod
`prompt.submit` hook can attach `context` to a user prompt, while the calling
plugin's own hook never sees its own submit.

## Decisions

- **Mailbox mail steers at tool boundaries.** Mail the watcher would submit
  (the direct-mode batch or the notice-mode notice) while a main-thread turn
  runs is queued in the mod and flushed as one `$.session.append` user text row
  (`{ message: { type: 'user', content: [{ type: 'text', text }] } }`) at the
  next main-thread tool-call completion. Anything still queued at main-thread
  `turn.complete` goes through `$.prompt.submit` as today; idle delivery stays
  `$.prompt.submit`. Source: research Confirmed Decision (user, 2026-10-18).
- **Append, not `context`.** Rejected: a `tool.call` result `context` field,
  which the probe showed framed as hook output that haiku declined to act on.
- **Turn state from turn hooks.** "Main-thread turn running" is tracked from
  `turn.start` / `turn.complete` with `agentId === undefined`; mail arriving
  before the first tool call waits for the first boundary or turn end.
  Rejected: inferring it from in-flight tool calls, which leaves the gaps
  between calls unclassified.
- **Flush after the tool result.** The flush runs in a catch-all main-thread
  `tool.call` hook after `next(e)` resolves: a tool result is always followed
  by a model request in the same turn, so the row cannot strand the way a row
  appended during a text-only stretch does. A user interrupt right after the
  tool is the one leftover; the row then prepends to the next prompt.
  Rejected: flushing at tool start or on mail arrival.
- **Row text unchanged.** The appended row carries the existing `[ws mailbox]`
  batch or notice text, with no mid-turn marker. Rejected: an
  "arrived while you were working" prefix on the appended row only; the
  existing text already states source and handling, and one format keeps the
  watcher tests single-sourced.
- **Deferred compaction continuation.** When a lever compaction resolves (or
  is skipped), the continuation is held pending instead of submitted. The
  mod's `prompt.submit` hook attaches it as `context`
  (`next({ ...e, context: [...(e.context ?? []), text] })`) to the next
  composer-origin user prompt and marks it consumed; a 1500 ms timer submits
  it only if still unconsumed. A prompt arriving after the window behaves as
  today (the continuation runs as a follow-up turn). Rejected: the mod
  dropping its own queued continuation from its own hook (impossible: the
  calling plugin's `prompt.submit` hook never sees its own submit);
  immediate submit plus `$.turn.abort` (wastes a model request, leaves a
  visible unanswered prompt); today's immediate submit (user, 2026-10-18).
- **Flush only after model-issued calls.** `$.tool.call` runs through the
  plugin's other hooks, so the flush hook also sees the mod's own mid-turn
  `mailbox_recv` drain and slug lookup; those are not model tool results and
  no model request is guaranteed after them. The flush acts only on
  model-issued calls (`next.origin.plugin === 'engine'`, as
  `isModelRecvCompletion` already checks) with `agentId === undefined`.
- **One row per flush, no silent loss.** Several batches drained before one
  boundary join into the single appended row, never one row per batch (the
  per-envelope split 261009 fixed on pi). Mail the mod holds past the
  watcher's submit edge keeps the existing record path: a turn-end
  `$.prompt.submit` that rejects or resolves `{ drop }` (a failed submit, per
  261017-bug-claude-mailbox-mod-post-drain-robustness), a rejected append, and
  mail still queued at `session.end` or a watcher stop are recorded
  user-visibly as `keep` does today.
- **Continuation window details.** The 1500 ms timer starts when the lever
  compaction resolves or is skipped. A consuming prompt is one with
  `e.origin.kind === 'composer'` and no `e.command` (slash commands, which may
  never reach the model, do not consume). When `next` resolves `{ drop }` for
  the consuming prompt, the continuation is un-consumed and submitted, and the
  timer must not deliver it a second time. A deferred continuation lost to
  `session.end` inside the window or to a failed timer submit is recorded to
  the transcript as today (2a4cfcf57). The path where another compaction
  landed first (`!r.compact`, continuation at turn end) keeps its immediate
  submit.
- **Scope.** ws mod only, with the wsflow copy byte-identical and mirrors per
  `wsflow-mirroring.md`; push-unavailable and non-ws-project paths keep their
  behavior. One phase: both deliveries share the turn-state tracking.

## Constraints

- Do not touch the uncommitted change to
  `261009-bug-pi-mailbox-busy-drain-splits-into-per-envelope-steers`; do not
  stage `.codex`.
- Out of scope: the MCP reconnect lever's continuation keeps its immediate
  submit; whether a prompt typed during a reconnect overtakes it is unprobed
  (user, 2026-10-18).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261018-research-claude-mod-mid-turn-steer (2026-10-18, Confirmed Decisions): "Mailbox mail arriving during a main-thread turn is delivered at the next main-thread tool boundary through `$.session.append`; mail still queued at turn end is submitted as today, and idle delivery stays `$.prompt.submit` (user, 2026-10-18)." — bearing: supports
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, Result 2a4cfcf57): "A repeat lever call replaces the continuation and keeps the compact-step state; a lost continuation submit is recorded to the transcript (the only holder of that text), a lost threshold submit only to the debug log." — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "The advisory and both milestones are dedicated wake turns. ... the mod `$.prompt.submit`s that threshold's message, which starts its own turn once idle" — bearing: constrains
- 42ab0938 (2026-10-18, commit): "The validator forbids storing or passing `$`, so compaction.ts takes engine edges (submit, compact, after, log) bound as closures in session.start and a usage-reader closure per measure" — bearing: constrains
- 261017-bug-claude-mailbox-mod-post-drain-robustness (2026-10-17, Result 548f1c0cc): "A `drop` result from `$.prompt.submit` counts as a failed submit. The engine reports a refused prompt as a result, not a rejection" — bearing: constrains
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, commit 4d46a066): "dogfood (2026-10-17, auto mode) verified idle wake through a direct drain and mid-turn arrival delivered after the turn ended with no loss" — bearing: constrains
- 1ffd64e1 (2026-10-18, commit): "The lever's landing bypasses the mod's session.compact hook, so it reaches the manual feature through the new leverLanded edge." — bearing: constrains
- 261018-feat-claude-mod-ws-manual-injection (2026-10-18, commit 23cc9f5c): "Auto compaction leaving the flag unchanged stays as implemented; the live run confirmed no compose follows it." — bearing: supports

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/register.ts, watcher.ts, compaction.ts, their tests, and the byte-identical agents-plugin-wsflow/hooks/claude-mod/ copy |
| scope.surface | internal | mod-internal edges and hooks; no new model-facing tool and no exported plugin interface named |
| scope.new_public_symbol | no | none named; delivery edges and turn-state tracking are mod-internal |
| scope.new_type_contract | yes | watcher.ts edges type (submit edge at watcher.ts#L56) and compaction.ts edges (submit at compaction.ts#L43) gain a mid-turn append or deferred-continuation member |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/engine.test.ts, watcher.test.ts, compaction.test.ts, compaction-engine.test.ts; whether the testing kit models session.append and turn.start is unverified |
| complexity.reuse_points | confirmed | existing main-thread tool.call and turn.complete hooks and the submit edges in agents-plugin/hooks/claude-mod/register.ts#L417 and #L499 |
| complexity.side_effect_risk | moderate | changes when mailbox mail and the compaction continuation reach the model; races between the tool-boundary flush, turn end, and the 1500 ms window |
| risk.correctness | high | interleaving of turn state, flush at tool completion, turn-end fallback, and the consume timer; mail loss on a missed flush or interrupt |
| risk.fit | moderate | follows the existing hook and edge pattern but adds a prompt.submit hook and turn.start tracking the mod does not register today |
| risk.test | moderate | engine harness coverage of session.append, turn.start, prompt.submit hook and timers is not confirmed; live dogfood is left to the lead |
| risk.security_or_contract | low | appended row carries the same mailbox text as today and adds no tool, permission, or exported interface |

## Phases

### Phase 1: Steer mail at tool boundaries and defer the compaction continuation

Implement the settled Decisions in `agents-plugin/hooks/claude-mod/`
(`register.ts` wiring, `watcher.ts` delivery edge, `compaction.ts`
continuation), with unit coverage in the existing engine tests, and resync the
wsflow mirror.

Verification: `claude plugin validate` and `claude plugin test` for both
packages; engine tests cover queue-then-flush at a tool boundary, the
turn-end submit of leftovers, idle submit, consumed vs unconsumed
continuation, the window timer, a dropped consuming prompt, a slash command
not consuming, leftovers after an interrupt, and subagent or mod-issued tool
calls not flushing. The phase is complete when these pass. Live dogfood
(mid-turn mail from a peer during a long tool call; a prompt typed during a
lever compaction) is the lead's, after merge review, and is outside the
worker's boundary.

### Result (7fe04e8e6) - 2026-10-18

Landed in 7fe04e8e6 with review fixes in 720b51e37. The wsflow copy is byte-identical.

- `watcher.ts`: `WatcherEdges` gains `turnRunning` and `append`, and
  `Watcher` gains `toolCompleted` and `turnCompleted`. A batch or notice
  that becomes ready while a main-thread turn runs goes into a session-wide
  queue. The queue survives a rebind; a stop gives it up. At the next
  boundary the queue is appended as one row, and anything left at turn end
  is submitted as one prompt. A refused row, a failed turn-end submit, or a
  stop records each queued batch through the existing `keep` path. A lost
  queued notice backs off and re-notices. A queued notice is withdrawn once
  the model's own recv for that key completes.
- `register.ts`: `mainTurn` is set by `turn.start` and cleared at
  `turn.complete` (main thread only). The clear and the leftover flush run
  in a `finally`. A catch-all `tool.call` hook is registered first, so it
  wraps the mod-served tools. It awaits the flush before returning the
  result, and only for `next.origin.plugin === 'engine'` calls with no
  `agentId`. A new `prompt.submit` hook attaches the held continuation as
  `context` to a composer prompt that has no `command`. When that prompt is
  dropped or rejected, the hook releases the continuation.
- `compaction.ts`: `CONTINUATION_WINDOW_MS = 1500`. The new `delay` edge and
  `claim()` hold the continuation after the lever's own compaction, whether
  it landed or was vetoed. A release submits through the `after` edge,
  because the host refuses `$.prompt.submit` inside a `prompt.submit` hook
  (kit probe). A session end inside the window records the continuation to
  the transcript. The path where another compaction landed first keeps its
  immediate submit. Decision taken: threshold messages also wait while a
  continuation is held, since one sent then would run ahead of the
  continuation.
- Verification: `claude plugin test agents-plugin` and
  `claude plugin test agents-plugin-wsflow` each passed 258 tests with 0
  failures. `claude plugin validate` passed for both packages. `go test
  ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate` passed, and the
  hooks manifest and neutrality unittests passed. The engine tests cover
  every case named in the phase verification.
- Live dogfood is the lead's. It should include checking that mail still
  queued at turn end starts its follow-up turn: that submit runs inline in
  `turn.complete`, and the test kit does not model the host's turn-hold
  check there.

#### Edition (846c811df) - 2026-10-18

- Live dogfood found a gap. Mail was queued during a main-thread tool call,
  and the person interrupted that call with Esc. The flush still appended the
  mail at the call's boundary, and no model request followed in that turn.
  So the queue was empty at `turn.complete` (reason `aborted`). The model
  read the mail only with the next typed prompt.
- `register.ts`: the tool-boundary flush now also requires
  `!next.signal.aborted`. Mail queued during an interrupted call stays
  queued, and the turn end submits it as a follow-up turn.
- Engine test: the kit has no interrupt for a test's tool call. A
  prepend-tier plugin that settles the call first aborts the mod's
  `next.signal`, so the test uses it to stand in for Esc. The test fails
  without the guard.
- Verification: `claude plugin test agents-plugin` and
  `claude plugin test agents-plugin-wsflow` each passed 259 tests with 0
  failures. The wsflow mirror is byte-identical.

#### Edition (05ae99992) - 2026-10-18

- Live dogfood after 846c811df: the interrupted-call guard kept the mail
  queued. The host then dropped the turn-end submit, because a plugin
  `$.prompt.submit` is dropped after the person stops a turn until they send
  input. So the mail went to the loss record, which the model never reads.
- `watcher.ts`: one `deliver()` helper now backs every mailbox submit: the
  idle drained batch, the idle notice, and the turn-end leftovers. When the
  submit is rejected or dropped, it appends the same text as a row. A row
  appended while idle rides the person's next prompt. The text is recorded
  as lost only when the append fails too, and the loss reason names both
  failures. A notice delivered by the append suspends the loop as a
  submitted one does. The compaction and reconnect continuation submits are
  unchanged.
- Tests cover these cases:
  - An idle submit that is dropped is appended (unit and engine).
  - A turn-end submit dropped after an interrupt is appended and not
    recorded (engine, both packages).
  - A dropped notice is appended and the loop suspends (unit).
  - When both the submit and the append fail, the text is recorded as lost
    (unit and engine).
- Verification: `claude plugin test agents-plugin` and
  `claude plugin test agents-plugin-wsflow` each passed 264 tests with 0
  failures. `claude plugin validate` passed for both packages. `go test
  -count=1 ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate` passed.
