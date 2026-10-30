---
title: claude-mod in headless sessions - compaction lever rejected, captions run uselessly
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 13292e025704603c
sage-review-completeness-reviewed: 13292e025704603c
---

# claude-mod in headless sessions - compaction lever rejected, captions run uselessly

## Background

In a headless Claude session (SDK host; observed in Orca on 2026-10-30) the
lead's `request_compaction` lever never compacts. The claude-mod's lever
calls `$.session.compact()` at the turn end
(`agents-plugin/hooks/claude-mod/compaction.ts`, `runLever`; bound in
`register.ts` as `compact: () => $.session.compact()`), and the engine
rejects it:

```text
ws compaction: the requested compaction was rejected (HooksError: ws: $.session.compact: not available in a headless (-p / SDK) session yet: compaction here runs inside a turn (a /compact prompt); catch it and carry on); retrying at the next turn end
```

The lever then retries at every turn end and fails each time. In the same
Orca session the owner's own `/compact`, sent from Orca's chat, was refused
by Orca itself ("wait for pending work and messages to finish before using
this command") while a background subagent ran, so Orca's composer offers no
workaround either. Lead-chosen
compaction is a primary working tool for the owner, so this blocks headless
use.

Verified API facts (`claude-code.d.ts`, mod API types for Claude Code
2.1.314):

- `SessionStartInput.isInteractive` is true under the REPL and false for a
  `-p` run or the SDK; `surface` is null there.
- `$.command.run({ command, args? })` "runs a slash command as if the person
  typed `/command args`: the event `command.run`, queued and run once the
  session is idle"; it rejects an unknown name, a call inside a hook the turn
  waits on, and a call past a Stop or a host's limit.

Spike result (2026-10-30, scratch mod in `/tmp/ws-compact-spike`, two
headless `claude -p --output-format stream-json` runs, Claude Code 2.1.314):

- VERIFIED: `$.command.run({ command: "compact" })`, scheduled with
  `$.clock.after(0, ...)` from the first main-thread `turn.complete`
  (`agentId === undefined`), compacts a headless session. Both runs emit a
  `compact_boundary` system message with `compact_metadata.trigger:
  "manual"` (pre/post tokens 22575/7783 and 24847/8308); the mod's own
  `session.compact` hook fires with `trigger=manual agentId=undefined` and
  `next(e)` returns no `skip`; `command.run` resolves `{}`.
- VERIFIED: it compacted at once while a background subagent was still
  running, and the subagent survived; the engine-side path does not share
  Orca's composer gate on pending work.
- Side effect: the queued command adds one extra empty `result` and `init`
  message to the stream.
- Not tested: the interactive (REPL) path, the real Orca host, a call made
  mid-turn or inside a hook the turn waits on, and passing instructions
  through `args`.

Because a `command.run` compaction carries `trigger: "manual"`, it is
indistinguishable by trigger from a compaction the user typed.

Separately, tool-row captions (`claude.display_summary_enabled`,
`agents-plugin/hooks/claude-mod/captions.tsx`) draw only in a terminal view,
but the knob is read at session start with no interactivity check, so a
headless session would still spend summary-model calls on captions nobody
sees. The Pi adapter already limits its display summaries to the TUI
(`agents-plugin-pi/src/display-summary.ts` header: `ctx.mode === "tui"`; the
widget gate at `index.ts`, `shouldArmAgentWidget`).

## Decisions

- **One lever route, chosen by the headless result.** The lever runs
  compaction through `$.command.run({ command: "compact" })`, the route the
  spike showed working headless. If the interactive check (Phase 1) shows it
  also works under the REPL, it replaces `$.session.compact()` everywhere and
  the lever keeps one code path; only if it fails interactively does the
  lever branch on `isInteractive`. Rejected: a headless-only branch kept
  beside the interactive one by default (two routes, two landing paths).
- **Lever versus user compaction.** A `command.run` compaction lands as
  `trigger: "manual"`, like a user-typed `/compact`. The lever's own
  compaction is the one that lands while its awaited `$.command.run({
  command: "compact" })` is pending: `leverCompacting` stays set from the
  call until it resolves, and the mod's own `session.compact` hook observed
  in that window supplies the landed or vetoed result that `runLever` acts
  on. In the spike `command.run` resolved 3 ms after the landing (run1
  ui_log 14:24:24.255, then 14:24:24.258). The lever's landing keeps the
  existing resume prompt (`continue` flag), landed notification, and manual
  re-key. A main-conversation compaction landing outside that window still
  drops the pending compact step and keeps its resume
  (261017-feat-claude-mod-lead-compaction-timing, "A compaction landing
  elsewhere clears the pending compact step"). Rejected: treating the first
  `manual` compaction after the request as the lever's (it reverses that
  261017 rule); an `args` marker read back from `instructions` (untested,
  and it rewrites what the summarizer is told).
- **Settled idle step; the lever never queues `/compact`.** After a
  non-aborted main-thread `turn.complete`, the compact step is scheduled with
  `$.clock.after(1000, ...)` instead of `after(0)`. When it fires, it first
  checks `turnRunning()`, before every other branch, including the `finish`
  of a step a landing elsewhere already dropped. If a turn is running, it
  does nothing and the next main-thread `turnComplete` schedules or settles
  the request as today. Otherwise it calls `command.run`. The 1 s settle lets
  a prompt already queued in the engine start its turn first, so the idle
  check sees it. A `/compact` queued behind a turn cannot be withdrawn: a
  native auto compaction in that turn would be credited to the lever and the
  queued command would compact a second time, breaking 261017's "never
  compacts right after a native compaction". This is compatible with
  261017's "Rejected: a timer retry", because the delay precedes the single
  attempt and retries stay at turn boundaries. Rejected: keeping the queue
  and vetoing later compactions in the window from the mod's hook (more
  moving parts); accepting a double compaction; checking `r.compact` before
  `turnRunning()` (that would submit the resume in the middle of the turn
  whose auto compaction dropped the step, contradicting the "resume follows
  at the next turn end" notice). If the `isInteractive` fallback branch exists,
  its `session.compact()` call uses the same settled, `turnRunning()`-first
  step. Accepted residuals: a prompt that arrives in the same tick as the
  idle check; a user-typed `/compact` that lands inside the awaited-command
  window, which is credited to the lever (the conversation is compacted once
  either way, and the lever's resume follows).
- **Split rejection reasons, by route.** A `command.run` rejection (unknown
  name, inside a hook the turn waits on, past a Stop or a host's limit) is
  permanent: the lever gives one notice, drops the compact step, and holds a
  requested resume with the vetoed note, as for a veto. The transient case
  is a turn running when the settled step fires: no call is made, and the
  retry comes at the next turn end. On the `isInteractive` fallback branch
  only, a `session.compact()` rejection stays transient and retries at the
  next main-thread turn end, as today. Rejected: retrying every rejection at
  every turn end (the observed headless loop); classifying by HooksError
  text (engine message text is not a contract).
- **No observed outcome counts as a veto.** If `command.run` resolves with
  no main-conversation `session.compact` landing or veto observed in the
  window, the lever treats it as a veto: one notice, the compact step is
  dropped, and a requested resume is held with the vetoed note. Rejected:
  treating it as a landing (a false re-key and resume); retrying (it
  recreates the turn-end loop).
- **Headless forces captions off at runtime.** With `isInteractive: false`,
  captions are off for that session regardless of
  `claude.display_summary_enabled`; the config value is never rewritten.
  Rejected: writing the knob (the config is shared with interactive sessions
  on the same machine). Because the knob is read once at session start, a
  surface attached later (e.g. Remote Control) does not turn captions on for
  that session.
- **Pi needs no change** beyond confirming that the display-summary pipeline
  itself is gated on `ctx.mode === "tui"` (its header says so, and the gate
  is `shouldRunDisplaySummary(mode, role)` returning `mode === "tui" && role === undefined`,
  `agents-plugin-pi/src/display-summary-session.ts#L33-L35`, applied at
  `agents-plugin-pi/src/display-summary-session.ts#L100`).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (mod notices ship),
  `ai-docs/manuals/wsflow-mirroring.md` (`agents-plugin/hooks/`).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/)

## Prior Decisions

- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "A compaction landing elsewhere clears the pending compact step. The mod's own `session.compact` hook, when `next(e)` resolves without `skip` for a main-conversation compaction ... with a `manual` or `auto` trigger ... clears a pending lever r..." — bearing: contradiction-candidate
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-17, Decisions): "A pending request survives a rejected `compact()` (a turn started first, for example a mailbox push) and retries at the next main-thread `turn.complete`. Rejected: a timer retry" — bearing: constrains
- 5ece662d (2026-10-18, commit): "A refused dispatch cannot be observed directly, so it is detected at the next main-thread turn end ... A token, not a boolean ... Rejected: a timer watchdog (more state, and contradicts the existing no-timer-retry contract)." — bearing: constrains
- 261030-feat-claude-mod-mcp-watchdog (2026-10-30, Decisions): "Reuse the lever's reconnect step ... (`/mcp reconnect <server>` through `$.command.run`, then the tool-list poll, then server re-resolution), not a second implementation." — bearing: supports
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "Independent of the compaction lever. A compaction whose `compact()` is rejected because the reconnect's continuation turn started first retries at the next turn end under its existing rule." — bearing: constrains
- 1bd9bf10 (2026-10-20, commit): "Guard implemented as "turn.start cancels the held resume" (compaction) ... A hold superseding a still-open window now replaces it instead of submitting the older text" — bearing: constrains
- 261022-feat-claude-mod-tool-call-captions (2026-10-22, Decisions): "Placement. A new feature file in `agents-plugin/hooks/claude-mod/`, wired from `register.ts`, like the lead compaction timing feature." — bearing: supports
- 261028-feat-claude-mod-status-band (2026-10-28, Decisions): "The band runs only in the ws package (`compactionEnabledFor`), as captions, compaction and manual do; wsflow draws nothing and ships no config manifest." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/compaction.ts, register.ts, captions.tsx, their tests, and the byte-identical agents-plugin-wsflow/hooks/claude-mod/ mirror |
| scope.surface | internal | mod-internal edges and session.start gating; no exported plugin API or tool schema change named |
| scope.new_public_symbol | no | none named |
| scope.new_type_contract | unknown | the `compact` edge (CompactionEdges, compaction.ts#L82) changes from `$.session.compact()` to an awaited `command.run` whose result comes from the mod's own session.compact hook; the signature is the worker's |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/compaction.test.ts, compaction-engine.test.ts, captions-engine.test.tsx |
| complexity.reuse_points | confirmed | $.command.run edge already used by the reconnect lever (agents-plugin/hooks/claude-mod/register.ts#L771) |
| complexity.side_effect_risk | moderate | the lever's compact moves from an awaited call to a queued command that lands later, and the interactive path is untested |
| risk.correctness | high | a manual trigger is indistinguishable from a user /compact; settled by the awaited-command window, which relies on command.run resolving after the landing (spike-observed, not a documented contract) |
| risk.fit | moderate | keeps the 261017 landing-elsewhere rule; the leverCompacting guard now spans a queued command that may wait behind a running turn |
| risk.test | moderate | engine tests exist but cannot observe the interactive or real Orca host; live checks remain with the lead |
| risk.security_or_contract | moderate | lever result text, resume prompt, and landed notification semantics must stay unchanged for a lever call |

## Phases

### Phase 1: Unified compaction lever

Order: the headless spike is done (Background). Before coding, check the
interactive path: in a REPL session, `$.command.run({ command: "compact" })`
from the lever's turn-end step compacts and lands through the same
`session.compact` hook. The worker runs this check by driving `claude` under
tmux with the scratch mod (`/tmp/ws-compact-spike/mod`, or a fresh copy)
loaded through `--plugin-dir`, reads the mod's `session.compact` hook log, and
records the result in this phase's Result, including whether the hook's
landing result is seen before `command.run` resolves (the window design
depends on that order; if `command.run` resolves at dispatch instead, stop
back to the lead). Also re-run the headless spike with the step scheduled at
`$.clock.after(1000, ...)` to confirm that a delayed call is not rejected in
a streaming-input SDK session. A worker that cannot drive a REPL stops back
to the lead. Implementation notes: the no-outcome veto needs its own skip
reason for `vetoedResumeText`/`vetoedResumeLine`; a settled step that finds
a turn running clears `r.scheduled`, so the next `turnComplete` does not log
a missed step; a `session.compact()` fallback branch still bypasses the
hook and keeps `leverLanded`. Then implement the decisions with
one route if it passes, or the `isInteractive` branch if it fails; stop back to
the lead if the landing is unobservable on either path. On the `command.run`
route the lever's compaction also reaches the `session.compact` observers
(`manual.compacted`, `captions.compacted`), which the current code assumes it
bypasses (`register.ts` comment near the `leverLanded` wiring,
`CompactionEdges.leverLanded`, the `captions.tsx` doc); keep exactly one
landing fan-out per lever compaction and update those comments in both
packages.

Verification: compaction engine tests for the chosen route (command.run used,
landed and vetoed outcomes, resume prompt with and without `continue`,
a compaction landing inside versus outside the awaited-command window, the
1 s settled step finding a turn running (no call, including for a dropped
step's resume), no observed outcome treated as a veto, permanent `command.run` rejection
fail-fast, and the fallback branch's transient retry if it exists);
then the lead's live checks: a lever call compacts at the turn end in a
headless (Orca) session and in an interactive session.

### Result (455e70820) - 2026-10-31

Interactive check passed, so the lever has one route (`$.command.run({
command: "compact" })`) and no `isInteractive` branch. The check used a fresh
scratch mod, `/tmp/ws-compact-spike2/mod`, that schedules the command with
`$.clock.after(1000, ...)` from the first main-thread `turn.complete`, on
Claude Code 2.1.314:

- REPL (`claude --model haiku --plugin-dir ...` under tmux,
  `isInteractive=true`): turn.complete 15:00:35.624, settled step 15:00:36.629,
  `session.compact` hook `trigger=manual agentId=undefined` 15:00:36.672,
  result `skip=none` 15:00:41.479, then `command.run resolved {}`
  15:00:41.481. The transcript showed "Prompt from the compact-spike2 plugin /
  /compact" and "Compacted".
- Headless streaming input (`claude -p --input-format stream-json
  --output-format stream-json`, stdin held open, `isInteractive=false`):
  the step fired 1 s after the turn end and was not rejected. The hook result
  `skip=none` came at 14:58:57.162 and `command.run resolved {}` at
  14:58:57.164. The stream had a `compact_boundary` with
  `trigger: "manual"` (pre/post 21992/7777).
- In both runs, the hook's landing settled about 2 ms before
  `command.run` resolved. Neither run raised a `turn.start` for the queued
  `/compact`, so a held resume is not cancelled by the command itself.

What landed (`compaction.ts`, `register.ts`, `captions.tsx`, `timing.ts`, and
the wsflow mirror):

- The `compact` edge now runs `/compact`.
- The compact step is `delay(SETTLE_MS = 1000)` after a non-aborted
  main-thread turn end. It checks `turnRunning()` first, clears
  `r.scheduled`, and makes no call while a turn runs.
- The outcome is the main-conversation compaction that `compacted()` sees
  while `leverCompacting` is set. A landing wins over a veto, and the window
  does not drop the step.
- A refused command and an unobserved outcome both settle through `vetoed()`
  with their own reason (`the compact command was refused: ...`,
  `UNOBSERVED_REASON`), with no retry.
- `landed()` moved into `compacted()`.
- The `leverLanded` edge and `captions.leverLanded` are removed. The lever's
  landing now reaches `manual.compacted` and `captions.compacted` through the
  mod's hook, which is the one fan-out.

Verification:
- `claude plugin test agents-plugin` gave 515 pass / 0 fail. The tests cover
  command use, landed and vetoed outcomes, resume with and without
  `continue`, landing inside versus outside the window, a turn running at the
  settled step (including a dropped step's resume), the unobserved veto,
  permanent refusal, and a headless start.
- `go test ./internal/wsrsrc -run TestWsflowHooksMirrorUpToDate` passed.
- `claude plugin validate` passed on both packages.
- The lead's live Orca and interactive checks remain.

Decisions:
- The no-fallback route was chosen from the check above.
- The engine-test worlds raise the command's compaction through the test
  body's engine, because the kit refuses `$.session.compact` from a test
  hook's own `$`.

### Phase 2: Headless caption force-off

Gate captions on `isInteractive` per the decision. `captions.isOn()` also
selects the mod's own-prompt split delivery (`register.ts`, the `own.isOn`
edge) and the mailbox recap requests, so a headless session also submits its
own prompts plainly and requests no recaps; that is intended.

Verification: captions engine tests: headless start with the knob on draws
nothing, requests no summaries or recaps, and delivers own prompts plainly;
interactive start unchanged.

### Result (392230dc6) - 2026-10-31

`CaptionEdges.interactive` is now required. `register.ts` passes
`e.isInteractive` to it. A non-interactive `captions.start` resets its state
and then returns before reading any setting, so captions, the split delivery
of the mod's own prompts, and mailbox recaps are all off in that session. The
knob is never written.

The Pi gate was confirmed: `shouldRunDisplaySummary` returns
`mode === "tui" && role === undefined`
(`agents-plugin-pi/src/display-summary-session.ts#L34-L35`). Pi is unchanged.

Verification:
- New engine tests:
  - A headless start with the switch on draws raw rows, sends no request,
    reads no caption key, and a later interactive start turns captions on.
  - A headless resume prompt is submitted whole.
  - A headless drained mailbox batch is submitted whole, with no recap.
- These tests fail with the gate disabled.
- `claude plugin test agents-plugin` gave 518 pass / 0 fail.
- The mirror drift guard and `claude plugin validate` passed.
