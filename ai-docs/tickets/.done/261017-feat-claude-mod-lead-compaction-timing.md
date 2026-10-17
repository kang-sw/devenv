---
title: Claude lead compaction timing through a ws plugin mod
related:
  261016-research-claude-mod-pi-like-compaction: source research; its Outcome Ledger Confirmed Decisions are this ticket's contract except where a Decision below records a reversal
  261017-feat-claude-mailbox-active-push-mod: first Claude mod; validated the mod path (hooks plus modules in one hooks.json, prompt.submit turns, $.store reload survival) and supplies the main-session-only tool precedent
  261006-feat-pi-compaction-advisory-standing-intent-milestones: pi advisory/milestone/latch semantics this ticket ports
  261007-feat-pi-compaction-milestone-wake-and-reread-lists: pi milestone wake-turn delivery and milestone prose this ticket ports
  261007-bug-pi-adapter-messages-read-as-user-text: pi precedent for labeling injected user-role messages
  261004-feat-pi-explicit-post-compaction-continuation: pi post-compaction continuation this ticket ports
  261002-feat-pi-lead-ws-owned-compaction: pi lever and hard point this ticket maps onto Claude
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 0dea3a7c6e3c4df2
sage-review-completeness-reviewed: 2d7c55d401b76e10
completed: 2026-10-18
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
plugin mod, with compaction running in `ws` only, reproduces pi's timing, not
pi's summary ownership. The mailbox push mod (`261017-feat-claude-mailbox-active-push-mod`) has since
validated the mod path live (2026-10-17 dogfood: hooks and modules in one
`hooks.json` both load, `$.prompt.submit` starts a turn in an idle session,
the binding survives `/reload-plugins` through `$.store`, the push loop stops
on `/clear`, and the revive nudge plus binding survive `/compact`).

Mod API facts used below are from the Claude Code 2.1.302 declaration file
(`plugin-authoring` skill, `types/claude-code.d.ts`):

- `session.measure` fires after each main-thread turn with
  `$.session.usage()`'s figures pushed (`context.tokens`, `context.window`,
  `context.percent`). `tokens` and `percent` are absent until the first
  response of the live window, and `percent` is against the model window.
- `$.session.usage({ breakdown: "summary" })` measures against the compaction
  window `rawMaxTokens`, which may be smaller than the model window
  (`CLAUDE_CODE_AUTO_COMPACT_WINDOW`, settings); it carries
  `isAutoCompactEnabled` and `autoCompactThreshold`, the token count at which
  auto compaction runs ("absent when it is off").
- `SessionCompactInput.agentId` is set when a subagent or fork compacts its
  own transcript and absent for the main conversation.
- A plugin's submitted prompt reaches the model framed as "The <name> plugin
  sent a message: ..." unless submitted `asUser` (`PromptOrigin`).
- `turn.complete`'s `reason` is `answer`, `aborted` (the user interrupted),
  `refusal`, or `error`.
- `$.session.compact({ instructions })` runs a `plugin`-trigger compaction
  between turns, resolves `{ skip }` on a hook veto, and rejects while a turn
  runs.
- `$.tool.register` declares a model-callable tool served by the mod.

## Decisions

- **One Claude mod source for both packages; compaction runs in `ws` only.**
  Claude Code 2.1.302 accepts one hooks module per plugin (`claude plugin
  validate`: "names one hooks module per plugin; a second entry is refused")
  and refuses an event registered twice without a matcher across the files
  one entry loads. So the mailbox push mod's directory becomes the single
  Claude mod `agents-plugin/hooks/claude-mod/`, holding the mailbox feature
  (moved from `mailbox-push/`, behavior unchanged) and the compaction feature
  (from pi's `src/lead-compaction.ts`), under one entry `register.ts` that
  registers each lifecycle event (`session.start`, `session.end`) once and
  starts each feature from it. Both packages' `hooks.json` name
  `./claude-mod/register.ts`, and `agents-plugin-wsflow/hooks/claude-mod/` is
  a byte-identical copy of the whole directory. The compaction feature is on
  only when `$.plugin.name === 'ws'` (the mailbox feature's `drainModeFor`
  precedent): in wsflow it registers no `request_compaction` tool, submits
  nothing, and never compacts, and any compaction hook it still registers
  passes through `next(e)` untouched. `register(on, options)` carries no
  plugin identity, so the gate is read inside each hook; it is a pure
  `compactionEnabledFor(name)` unit-tested the way `drainModeFor` is.
  Features are isolated on every shared hook: the two features' setups at
  `session.start` run independently (neither awaits the other, and the
  mailbox feature's early return when python is unavailable does not skip
  compaction setup), a throw in one is logged and leaves the other running,
  and at `session.end` a throw in the compaction reset never skips the
  mailbox feature's stop and `$.store` delete.
  This supersedes the research Confirmed Decision "The compaction mod ships
  in `ws` only, not in `wsflow`" (261016, 1f59a3a43): the compaction source now
  ships inert in wsflow inside the single Claude mod because Claude Code
  accepts one hooks module per plugin; compaction behavior stays `ws`-only
  through the gate (user decision, 2026-10-18).
  `TestWsflowHooksMirrorUpToDate` covers the whole `claude-mod/` directory, and
  `wsflow-mirroring.md`'s Claude Mod Mirror section names the directory and
  the `ws`-only compaction gate. Rejected: a second `modules` entry (refused);
  a ws-only entry file composing both mods with always-matching matchers on
  the duplicated lifecycle events (a validator workaround); a ws-only
  composer that records each mod's registrations and chains duplicates
  (machinery that only exists to keep two mods apart); a ws-only compaction
  module beside the mirrored mailbox module (needs one of the former).
- **Thresholds are measured against the compaction window.** The advisory is
  a fixed 50% of the breakdown's `rawMaxTokens` in this first step, with no
  config key (a config read would need an MCP tool call the mod otherwise
  does not make). Current usage is `context.tokens`, and every percentage in
  the pinned prose is a percentage of `rawMaxTokens`. Rejected: the model
  window (`context.percent`; with a smaller compaction window the advisory
  would never fire before auto compaction and the pinned hard point would be
  false); disabling the mod whenever the two windows differ.
- **Thresholds run only when auto compaction is on.** When
  `isAutoCompactEnabled` is false, `autoCompactThreshold` is absent, or it is
  not above the advisory point, the mod sends no advisory or milestone; the
  lever stays available. Rejected: a separate advisory text naming no hard point (more
  pinned prose for a rare setting).
- **The hard point is Claude's native auto compaction**, never vetoed or
  preempted by the mod. Rejected: vetoing `auto` below a ceiling (research
  Rejected Alternatives).
- **The advisory and both milestones are dedicated wake turns.** When a
  `session.measure` after a main-thread turn shows a crossing, the mod
  `$.prompt.submit`s that threshold's message, which starts its own turn once
  idle (the research Confirmed Decision; pi 261006's advisory delivery and pi
  261007's milestone delivery). Rejected: appending milestones to a
  main-thread tool result (pi 261006's mid-run steer, which pi 261007 dropped
  after dogfooding showed it never led to compaction); dropping milestones in
  the first step.
- **Milestone points** sit at one and two thirds of the way from the advisory
  to the native auto-compact point, `autoCompactThreshold`. The breakdown is
  read at session start, after a landed main-conversation compaction, and when
  `context.window` changes, and its figures are cached in between. Rejected: a
  breakdown call per measure (the plain usage call stays free).
- **No threshold message while a lever request is pending.** A threshold
  crossed while a request is pending is not submitted. Rejected: submitting
  it anyway (its turn can start first and make `compact()` reject, or it lands
  after compaction with stale percentages beside the continuation).
- **Latch semantics port pi 261006:** one latch over the ordered thresholds
  (advisory, first milestone, second milestone); one message per observation,
  advisory before milestone; a milestone waits for the advisory; usage
  observed below the latch re-arms it; any landed main-conversation
  compaction resets it; and on session start or resume the first
  `session.measure` that carries `tokens` baselines it from that usage (there
  is no usage figure at start itself).
- **The lever.** A mod-registered tool named `request_compaction` (in prose,
  "the compaction request tool") taking `{ continuation: string }`
  (non-empty, one line) records a compaction request and returns at once; a
  call carrying `agentId` is refused (main session only, as the mailbox mod's
  `checkWatchCall`). An empty or multi-line `continuation` is a tool error; a
  repeat call while a request is pending replaces its continuation (latest
  wins). At the next main-thread `turn.complete` whose `reason` is not
  `aborted`, the mod schedules `$.session.compact()` with no instructions to
  run after the hook has returned and, on success, `$.prompt.submit`s the
  continuation. Rejected: compacting inside the tool call (`compact()` rejects
  while a turn runs); awaiting `compact()` inside the `turn.complete` hook
  (the declarations do not say the turn has stopped running there, and if it
  has not, every retry would reject); compacting after an interrupted turn
  (the human is taking over); refusing a repeat call (the latest call states
  the current next step); naming the tool `compact` (invites expecting the
  context to be gone when the tool returns).
- **A compaction landing elsewhere clears the pending compact step.** The
  mod's own `session.compact` hook, when `next(e)` resolves without `skip` for
  a main-conversation compaction (`agentId` absent) with a `manual` or `auto`
  trigger or a `plugin` call not made by this lever,
  clears a pending lever request's compact step, so the lever never compacts
  right after a native compaction. `precompute` is ignored: it is the one
  dispatch that installs nothing (2.1.302 `SessionCompactTrigger`). The same
  observation resets the latch. Rejected: inferring a landed compaction from a
  usage drop at `turn.complete` (indirect). A subagent's or fork's own
  compaction is ignored.
- **A pending request survives a rejected `compact()`** (a turn started first,
  for example a mailbox push) and retries at the next main-thread
  `turn.complete`. Rejected: a timer retry (turn boundaries are the only
  moments `compact()` can succeed; native auto compaction bounds starvation
  under steady mail).
- **The continuation is always submitted.** When another compaction landed
  first or a hook vetoed with `{ skip }`, the compact step is dropped, a
  user-visible `$.ui.log` line says why, and the continuation is still
  submitted: after another compaction, at the next non-aborted main-thread
  `turn.complete` (where the compact step would have been scheduled); after a
  veto, as soon as `compact()` resolves `{ skip }`, as the success path
  submits, prefixed with a pinned one-line note that compaction was skipped. Rejected: dropping the
  whole request (leaves an idle lead with nothing to resume); keeping the
  continuation only when another compaction landed first.
- **Revive stays with the existing `SessionStart(compact)` nudge**; the
  continuation carries no revive instruction. Rejected: folding a revive into
  the continuation (double revive prompts).
- **No `$.session.version()` gate.** A broken mod API leaves no lever and
  native auto compaction untouched (as 261017-feat-claude-mailbox-active-push-mod).
- **Every submitted prompt is labeled.** The advisory, both milestones, and
  both continuation forms open with `[ws compaction] `: a submitted prompt is
  user-role text, and the label keeps it from reading as the human's (the
  mailbox mod's `[ws mailbox] ` prefix in `formatBatch`; pi
  261007-bug-pi-adapter-messages-read-as-user-text). Prompts are submitted
  without `asUser`, so the engine's plugin framing stays, as the mailbox mod
  submits.
- **The prose is pinned in this ticket** (advisory text, the two milestone
  texts, lever tool description and result text, the veto note, the
  continuation framing, the label) and asserted verbatim by tests,
  placeholders interpolated (261006 and 261007 precedent).

## Pinned Prose

Asserted verbatim by tests. Every percentage is of the compaction window
`rawMaxTokens`, rounded: `{percent}` and `{n}` current usage, `{hard}` the
native auto-compact point, `{continuation}` the lever
argument, `{reason}` the vetoing hook's skip reason, `{m1}` and `{m2}` the
milestone points. Adapted from pi's pinned prose (261006, as revised
by 261007): the hard point is Claude's automatic compaction, and pi's
lead-written summary preparation becomes "update durable records, then call
the tool" because the engine writes the summary. Each block below is the whole
submitted text, label included.

Advisory:

```text
[ws compaction] Context usage is {percent}% of the window (advisory point: 50%). Claude Code compacts automatically at about {hard}%.

This is not an instruction to compact right now. If you are in active discussion with the human, awaiting their answer or clarification, or holding working context that would be costly to rebuild (a half-applied change or a diagnosis in progress), carry on for now. A pause after asking the human a question is not a boundary.

From here on, look for a good moment to compact before {hard}%. A good moment is a natural boundary where most of what this context holds is no longer needed for the work ahead, and what is still needed can be restored cheaply after compaction from durable records (tickets, commits, notes, agenda) and the summary. Typical cases: work just landed, you are waiting only on background agents, or the next work is weakly related to the current context. When such a moment comes, first bring durable records up to date with anything still needed that only this context holds, then call the compaction request tool with a one-line continuation and end your turn. Compacting at a moment you choose keeps the work coherent; at {hard}% it happens automatically, mid-work if need be. Reminders follow at {m1}% and {m2}%.

If now is not such a moment, end this turn without replying and keep looking for one as you work.
```

Milestone 1:

```text
[ws compaction] Context window: {n}% / {hard}% (automatic compaction point). Past {hard}%, Claude Code compacts automatically, mid-work if need be. From here on, a good boundary is worth taking; if this is one, update durable records and call the compaction request tool, as the advisory describes. Otherwise end this turn without replying.
```

Milestone 2:

```text
[ws compaction] Context window: {n}% / {hard}% (automatic compaction point). This is the last reading before automatic compaction. Compacting at a boundary you choose keeps the work coherent; take this one if it fits, update durable records and call the compaction request tool, as the advisory describes. Otherwise end this turn without replying, and take the next good boundary.
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
[ws compaction] Continue after compaction: {continuation}
```

Continuation prompt after a `{ skip }` veto:

```text
[ws compaction] Compaction did not run ({reason}); your context is unchanged.
Continue: {continuation}
```

The label and the prefixes tell the lead the prompt is its own note, not a
new user instruction.

## Constraints

- Lead (main-thread) only: subagent turns neither advance thresholds nor may
  call the lever.
- The research's deferred Proposals (instructions injection on every trigger,
  a fully custom summary) are out of scope.
- The 261017 mailbox mod Decisions stay in force for the moved mailbox
  feature; in particular wsflow never drains or calls tools itself.
- Known limitation, accepted: in an organization-managed Claude Code, a
  person-installed plugin's `session.compact` hooks do not run (2.1.302
  `claude plugin validate` note). There the mod cannot see a compaction it did
  not start, so a pending request can compact again right after a native auto
  compaction, and the latch re-arms only when usage drops. No fallback is
  added. Rejected: inferring a landed compaction from a usage drop (indirect,
  and the cost is one redundant compaction, not lost data).
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/, agents-plugin-tool/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261016-research-claude-mod-pi-like-compaction (2026-10-17, Confirmed Decisions): "The compaction mod ships in `ws` only, not in `wsflow` (unlike the mailbox mod, which ships in both)." — bearing: constrains (superseded by the first Decision: the source ships inert in wsflow; behavior stays ws-only)
- 261016-research-claude-mod-pi-like-compaction (2026-10-16, Confirmed Decisions): "First-step scope mirrors pi's timing, not pi's summary ownership: - Advisory (50%) and milestone nudges are `$.prompt.submit`ted at `turn.complete`, driven from `$.session.usage()`." — bearing: constrains
- 261007-feat-pi-compaction-milestone-wake-and-reread-lists (2026-10-07, Decisions): "Move milestone delivery in `fireCompactionTriggers` from the tool-turn `steer` to `agent_end` (`followUp`, `triggerTurn: true`)" (the tool-turn steer was observed in dogfooding to never trigger compaction) — bearing: supports
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "A milestone is not deliverable while a lower advisory crossing is still undelivered: it stays pending" — bearing: supports
- 261006-feat-pi-compaction-advisory-standing-intent-milestones (2026-10-06, Decisions): "The trigger latch records the highest threshold already delivered. The `advisoryFired`/`hardFired` booleans are replaced by one latch over the ordered thresholds" — bearing: supports
- 261017-feat-claude-mailbox-active-push-mod (2026-10-17, Decisions): "A call carrying `agentId` (a subagent or teammate loop) is refused with an error naming the main-session-only rule, as the probe's `checkWatchCall`." — bearing: supports
- 99f22d9d (2026-10-17, commit): "The bound key persists in $.store keyed by session id, not $.state: a $.state ref must spell its owning plugin as a literal, which a source shared byte-identically by ws and wsflow cannot" — bearing: constrains
- 261017-bug-claude-mailbox-mod-post-drain-robustness (2026-10-17, Decisions): "When `$.prompt.submit` fails after a drain, the formatted batch is written to the transcript through `$.ui.log` at its default (user-visible) destination, as well as to the debug log." — bearing: supports
- 261004-feat-pi-explicit-post-compaction-continuation (2026-10-04, Decisions): "D4 - Caller intent, not delivery guarantees. The flag means 'I have work after compaction' ... Do not promise exactly one subsequent model run when child wake and resume are both owed." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/{hooks.json,claude-mod/ (from mailbox-push/)}, agents-plugin-wsflow/hooks/{hooks.json,claude-mod/}, agents-plugin/tests/test_claude_hooks_manifest.py, agents-plugin-wsflow/tests/test_wsflow_claude_hooks_manifest.py, agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go, ai-docs/manuals/wsflow-mirroring.md |
| scope.surface | public-interface | new model-callable request_compaction tool plus pinned model-facing prose; both packages' single hooks module moves to ./claude-mod/register.ts |
| scope.new_public_symbol | yes | request_compaction (mod-registered tool) |
| scope.new_type_contract | yes | request_compaction input { continuation: string } non-empty one line |
| scope.test_surface | new-files | engine-backed compaction tests beside the moved mailbox tests (agents-plugin/hooks/mailbox-push/engine.test.ts, watcher.test.ts); test_modules_name_existing_mod_sources in both packages' manifest tests and TestWsflowHooksMirrorUpToDate change to the claude-mod path |
| complexity.reuse_points | confirmed | agents-plugin/hooks/mailbox-push/register.ts#L63-L66 checkWatchCall main-session-only refusal, $.store usage, and engine test harness |
| complexity.side_effect_risk | high | mod submits prompts and compacts live sessions |
| risk.correctness | high | stateful latch, pending-request retry across rejected compact(), and races with native auto compaction and mailbox-push turns |
| risk.fit | moderate | early-access mod API (2.1.302 declarations); delivery and prose follow the research Confirmed Decisions and pi 261006/261007 |
| risk.test | high | session.measure, compact and prompt.submit ordering need an attended live session beyond engine tests |
| risk.security_or_contract | moderate | tool contract and prose are user-visible, and compaction drops context, but no auth or external surface |

## Phases

### Phase 1: Advisory, milestones, lever, and continuation

Implement the Decisions in one landing: move `mailbox-push/` into
`agents-plugin/hooks/claude-mod/` with the single entry, fold the draft
`agents-plugin/hooks/lead-compaction/` (52d7f6d0f) into it as the compaction
feature and delete the draft directory, and resync the wsflow copy
byte-identically. Reword text that names `mailbox-push/` or the single
package-name compare in the same landing: the `description` in
`agents-plugin/hooks/hooks.json`, the entry's header comment (two `'ws'`
compares now), the `test_wsflow_claude_hooks_manifest.py` docstring, the
`wsflow_hooks_mirror_test.go` comments and rsync hint, and the rsync command
in `wsflow-mirroring.md`.

Verification: `claude plugin validate` and `claude plugin test` pass for both
`agents-plugin` and `agents-plugin-wsflow`, with engine and unit tests for each
threshold, latch, lever, and retry behavior, the pinned prose asserted
verbatim, the existing mailbox tests passing in both packages (the two that
pin the exact registered-tool list narrowed to the mailbox tool, and their
test world answering every engine call the compaction setup makes), and
tests that under a non-`ws` plugin name no `request_compaction` tool is
registered and no prompt is submitted or compaction started (these run in
`claude plugin test agents-plugin-wsflow`, since the testing kit has no
plugin-name override; ws-behavior tests branch on the package identity); `python3 -m unittest discover
agents-plugin/tests` and `agents-plugin-wsflow/tests` pass with
`test_modules_name_existing_mod_sources` pinning `./claude-mod/register.ts`;
`go test -race ./internal/wsrsrc/ -run TestWsflowHooksMirrorUpToDate` passes
under `agents-plugin-tool/` over the whole `claude-mod/` directory; then a live
dogfood checks the advisory wake turn, a milestone wake
turn, a lever compaction followed by exactly one
continuation turn carrying the revive nudge, and lever retry while a mailbox
push holds the session.

### Result (2a4cfcf57) - 2026-10-18

Landed in 869003e09..2a4cfcf57 (implementation 42ab09388, review fix
2a4cfcf57):

- One mod `agents-plugin/hooks/claude-mod/`: `register.ts` (the single entry;
  `session.start` and `session.end` registered once, the mailbox and
  compaction setups independent and awaited together, per-feature `try`
  blocks at `session.end`), the moved mailbox `watcher.ts`, and the
  compaction feature in `compaction.ts` (state machine over bound engine
  edges, `compactionEnabledFor`) and `timing.ts` (thresholds, latch, lever
  check, pinned prose, moved from the 52d7f6d0f draft). The draft directory
  `agents-plugin/hooks/lead-compaction/` is deleted.
- Both `hooks.json` name `./claude-mod/register.ts`; the wsflow
  `claude-mod/` copy is byte-identical. Stale texts reworded: the ws
  `hooks.json` description, the entry header comment, the wsflow manifest
  test docstring, the Go mirror test comments and rsync hint, and the
  `wsflow-mirroring.md` Claude Mod Mirror section (now naming the ws-only
  compaction gate).
- Tests: `compaction.test.ts` (pure core, every pinned text verbatim,
  thresholds, baseline, re-arm, skipped thresholds, delivery blocked while a
  request is pending, lever refusal and errors, repeat call, rejection retry
  at the next turn end, veto note, another compaction dropping the compact
  step, subagent/precompute/veto ignored, session end, generation guard,
  dropped continuation recorded) and `compaction-engine.test.ts`
  (registration and pinned description, setups independent in both
  directions, advisory from `session.measure` with one summary breakdown
  read, lever after a main-thread turn end with subagent and aborted turn
  ends skipped, rejection retry, veto, manual compaction first, the lever's
  own landing resetting the latch, session end; under wsflow no
  `request_compaction`, no usage read, no prompt, no compaction, and
  compactions pass through untouched). The two mailbox engine tests that
  pinned the registered-tool list now assert the mailbox tools only.

Verification:

- `claude plugin validate agents-plugin`: passed;
  `claude plugin validate agents-plugin-wsflow`: passed (the validator notes
  the accepted managed-install `session.compact` limitation).
- `claude plugin test agents-plugin`: 103 pass, 0 fail;
  `claude plugin test agents-plugin-wsflow`: 103 pass, 0 fail.
- `python3 -m unittest discover agents-plugin/tests`: OK (78);
  `python3 -m unittest discover agents-plugin-wsflow/tests`: OK (15).
- `go test -race -count=1 ./internal/wsrsrc/ -run TestWsflowHooksMirrorUpToDate`
  (under `agents-plugin-tool/`): ok.
- Live dogfood (advisory and milestone wake turns, a lever compaction followed
  by one continuation turn with the revive nudge, lever retry while a mailbox
  push holds the session): not run by the worker; it needs an attended
  session that reaches the advisory point and is left to the lead.

Decisions:

- The validator forbids storing or passing `$`, so `compaction.ts` takes
  engine edges (submit, compact, after, log) bound as closures in
  `session.start`, and a usage-reader closure per measure.
- The summary breakdown is read lazily at the first measure after each
  invalidation (session start or end, a landed compaction, a changed model
  window) and cached by window; a generation counter drops a reading that
  awaited the read across an invalidation.
- The engine routes a plugin's `$.session.compact` past that plugin's own
  `session.compact` hook (an engine test confirmed it), so the lever step
  applies the landing reset itself; the hook ignores a landing while the
  lever's compaction runs, so an engine that does route it neither drops the
  lever's own step nor resets twice.
- A repeat lever call replaces the continuation and keeps the compact-step
  state; a lost continuation submit is recorded to the transcript (the only
  holder of that text), a lost threshold submit only to the debug log.
- The `session.compact` observer never vetoes: its `.catch` replays what
  settled beneath.
- Engine tests branch on the engine-reported package name, so the mirrored
  file runs ws behavior in ws and the inert path in wsflow.

## Sage Review Round 1 (2026-10-17)

### Design Reviewer — block

| # | Title | Severity | Resolution |
|---|-------|----------|------------|
| 1 | Hard point undefined when auto compaction is off or the compaction window is smaller than the model window | important | missing |
| 2 | No threshold nudge while a lever request is pending | important | autonomous |
| 3 | Subagent compactions must not count as a compaction landing elsewhere | important | autonomous |
| 4 | Hard-point derivation should use the breakdown's own figures | minor | autonomous |
| 5 | compact() called from inside a turn.complete hook may always reject | minor | autonomous |
| 6 | Session-start baseline has no usage figure | minor | autonomous |
| 7 | Label rationale overstates the risk; do not submit asUser | minor | autonomous |
| 8 | Pending request on an interrupted turn | minor | autonomous |

### Completeness Reviewer — pass

| # | Title | Severity |
|---|-------|----------|
| 1 | Repeat or invalid lever calls unspecified | minor |
| 2 | Milestone fallback when the hard point cannot be derived | minor |
| 3 | Latch re-arm and baseline levels left to the pi precedent | minor |
| 4 | Phase 1 verification names test categories rather than cases | minor |
