---
title: Make the Claude mod lever continuation a flag that yields to an already-started turn
related:
  261017-feat-claude-mod-lead-compaction-timing: origin of the compaction lever and its continuation contract, which this ticket revises
  261018-feat-claude-mod-mcp-reconnect-lever: origin of the reconnect lever's continuation
  261004-feat-pi-explicit-post-compaction-continuation: Pi precedent for a boolean continuation with a fixed resume text
  261020-chore-retire-mcp-server-repair-skill: sibling; also edits reconnect.ts
  261020-bug-claude-mod-milestone-percent-stale: sibling; also edits compaction.ts and timing.ts
sage-review-design: completed
sage-review-completeness: completed
sage-review-design-reviewed: 43672875b556f384
sage-review-completeness-reviewed: 43672875b556f384
completed: 2026-10-20
---

# Make the Claude mod lever continuation a flag that yields to an already-started turn

## Background

The Claude mod's `request_compaction` and `request_mcp_reconnect` tools take
an optional one-line `continuation` string that is submitted as the next
prompt after the step (`timing.ts` `continuationText`,
`reconnect.ts` `successText` / `failureText`).

For compaction, `runLever` holds the continuation for `CONTINUATION_WINDOW_MS`
(1500 ms) after the compaction lands and submits it with `$.prompt.submit` at
the window's end (`compaction.ts` `hold`). Only a prompt typed at the composer
within the window takes it instead (`register.ts` `prompt.submit` hook,
`compaction.claim`). Nothing checks whether a turn already started after the
landing.

Observed failure: the lead calls the lever with a continuation; a background
task finishes during the compaction and its notification starts the first
post-compaction turn; the lead resumes the work there; when that turn ends,
the continuation arrives as a stale, redundant prompt.

The user decided the per-call text is unnecessary (the summary and durable
records carry the next step) and that the resume prompt should fire only when
no other prompt has already started the first post-compaction turn.

## Decisions

- **Flag, not text.** Both tools replace the `continuation` string with a
  boolean flag and submit a fixed resume text. Rejected: keeping the
  free-text line (the user judged it unnecessary).
- **Yield to an already-started turn.** The resume prompt is submitted only
  when no main-thread turn has started since the step settled.
- **Both levers.** The reconnect tool gets the same flag and guard, keeping
  the two tools consistent.
- **Flag name.** `continue` (boolean, default false) on both tools.
  Rejected: Pi's `continue_after_compact` (one name fits both tools' schemas).
- **Fixed texts.** Landed: "[ws compaction] Compaction landed; resume the
  work in progress from the summary and your durable records." Vetoed: "[ws
  compaction] Compaction did not run (<reason>); your context is unchanged.
  Resume the work in progress." Reconnected: "[<plugin> mcp] The <plugin> MCP
  server reconnected; its tools are available again. Resume the interrupted
  work."
- **Guard mechanics.** The 1500 ms window after landing or veto stays; at its
  end the resume is submitted only when no main-thread `turn.start` occurred
  since the step settled (the register.ts `turn.start` hook feeds both the
  compaction and the reconnect modules). The composer `claim` path and its `prompt.submit` hook are removed.
  Rejected: keeping `claim` (any started turn, typed or not, now disarms the
  resume, so carrying it as context into a typed prompt is no longer needed).
- **Reconnect.** The reconnect step runs at turn end and has no window: on
  success with the flag set, the resume is submitted at once unless a
  main-thread `turn.start` occurred since the step started. A failure report
  is always submitted; with the flag set it ends with "Resume the interrupted
  work." (the success text's last sentence). Rejected: guarding the failure too
  (it carries information the lead needs whatever turn is running).
- **Flag input.** Only a literal boolean `true` sets the flag; absent or any
  other value (a non-boolean, or a leftover `continuation` argument) reads as
  unset, matching today's lenient read where a non-string continuation is
  omitted. Rejected: a tool error (no caller outside the shipped schema).
- **Lost-submit record.** The transcript record of a lost resume submit
  stays, recording the fixed text.
- **Repeat calls.** A repeat call while a request is pending replaces its
  flag (latest wins), so an omitted or false flag clears an earlier true,
  matching today's continuation-replacement rule.
- **Another compaction first.** When another compaction lands before the
  lever's own (`compacted` path), the resume still follows at the next turn
  end, now with the fixed text and without the `turn.start` guard; only the
  text changes.
- **Texts to update.** `LEVER_DESCRIPTION`, `LEVER_RESULT*`, the advisory and
  milestone prose ("a one-line continuation"), `reconnectDescription`,
  `RECONNECT_RESULT*`, and the `agents-plugin/hooks/hooks.json` description.
- **Milestone defect split out.** Tracked in
  `261020-bug-claude-mod-milestone-percent-stale`. Rejected: one combined
  ticket (that fix depends on a probe whose outcome is open).
- **wsflow mirror.** Every `agents-plugin/hooks/claude-mod/` edit is mirrored
  into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`
  (`ai-docs/manuals/wsflow-mirroring.md`).

## Constraints

- Pi (`agents-plugin-pi`) is out of scope: its `continue_after_compact` flag
  has shown no stale-resume problem in practice.
- Runs concurrently with `261020-chore-retire-mcp-server-repair-skill` (also
  rewrites `reconnectDescription` / `failureText` and their pinned tests) and
  `261020-bug-claude-mod-milestone-percent-stale` (edits `timing.ts`
  advisory/milestone heads and adds a `prompt.submit` path). Implement against
  this ticket alone; the lead merges the retirement first and composes the
  combined texts at merge.
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for agents-plugin/, agents-plugin-wsflow/)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for agents-plugin/hooks/, agents-plugin-wsflow/)

## Prior Decisions

- 261018-feat-claude-mod-optional-lever-continuation (2026-10-18, Decisions): "the lever descriptions (`LEVER_DESCRIPTION`, `reconnectDescription`), the lever results (`LEVER_RESULT`, `RECONNECT_RESULT`), and the compaction advisory and milestone texts say to pass a continuation only when work must resume by itself" — bearing: supports
- 261018-feat-claude-mod-optional-lever-continuation (2026-10-18, commit f87c093c): "A compaction landed elsewhere first with an omitted continuation keeps the request until the next turn end (existing shape) and submits nothing." — bearing: constrains
- 261018-feat-claude-mod-optional-lever-continuation (2026-10-18, commit f87c093c): "Rejected: submitting a bare "compaction done" notice, which starts the same idle turn." — bearing: constrains
- 261018-feat-claude-mod-mcp-reconnect-lever (2026-10-18, Decisions): "A call carrying `agentId` is refused (main session only); an empty or multi-line `continuation` is a tool error; a repeat call while a request is pending replaces its continuation (latest wins)" — bearing: constrains
- 261017-feat-claude-mod-lead-compaction-timing (2026-10-18, commit 42ab0938): "Repeat lever calls replace the continuation and keep the compact-step state; a lost continuation submit is recorded to the transcript (the only holder of that text)" — bearing: constrains
- 261018-feat-claude-mod-mid-turn-steer-delivery (2026-10-18, commit 7fe04e8e): "Holding threshold messages during the window extends the existing no-threshold-while-pending rule: an advisory submitted then would run ahead of the continuation." — bearing: constrains
- 261004-feat-pi-explicit-post-compaction-continuation (2026-10-04, Decisions): "Carry the explicit boolean on the compaction operation before ctx.compact aborts the invoking run. Use the existing resumeOwed/sendOwedResume completion path, not a second sender." — bearing: supports
- 261020-chore-retire-mcp-server-repair-skill (2026-10-20, Decisions): "Every `agents-plugin/hooks/claude-mod/` edit is mirrored into the byte-identical copy under `agents-plugin-wsflow/hooks/claude-mod/`" — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/compaction.ts, reconnect.ts, timing.ts, register.ts, five test files, agents-plugin/hooks/hooks.json, plus the byte-identical agents-plugin-wsflow/hooks/claude-mod/ copy |
| scope.surface | public-interface | model-callable request_compaction and request_mcp_reconnect input schemas (register.ts) and their model-facing descriptions change |
| scope.new_public_symbol | yes | tool input parameter continue (boolean) on both tools; replaces continuation |
| scope.new_type_contract | yes | boolean continue flag replacing the string continuation in checkLeverCall / checkReconnectCall and the request state; new turn.start edge into the compaction module |
| scope.test_surface | existing | agents-plugin/hooks/claude-mod/compaction.test.ts, compaction-engine.test.ts, reconnect.test.ts, reconnect-engine.test.ts, manual-engine.test.ts |
| complexity.reuse_points | confirmed | the 1500 ms hold window and edges.delay in compaction.ts, the mainTurn tracking on turn.start in register.ts, the reconnect request state in reconnect.ts |
| complexity.side_effect_risk | moderate | removes the prompt.submit claim hook and changes when a submit fires after a compaction or reconnect step |
| risk.correctness | moderate | guard races a started turn against the window end; the compacted-elsewhere and failure paths must keep their resume behavior |
| risk.fit | moderate | two tools, two byte-identical package copies, and fixed text strings must stay consistent |
| risk.test | moderate | engine-backed tests must drive a turn.start inside the window; manual-engine.test.ts uses the old continuation argument at line 345 |
| risk.security_or_contract | moderate | changes the model-facing tool schema and description texts; the parameter rename breaks callers passing continuation |

## Phases

### Phase 1: Flag and first-turn guard for both levers

Replace the `continuation` string with the flag in both tools' schemas,
checks, state and texts; add the `turn.start` guard; remove `claim` and its
`prompt.submit` hook. Update the claude-mod tests (`compaction.test.ts`,
`compaction-engine.test.ts`, `reconnect.test.ts`, `reconnect-engine.test.ts`,
`manual-engine.test.ts` where it drives the lever) in both copies.

Verification: tests cover - flag set, no turn started within the window ->
fixed resume submitted; flag set, a main-thread turn started within the
window (any origin) -> nothing submitted; flag unset -> nothing submitted;
veto with flag -> veto resume under the same guard; reconnect success with
flag and an intervening turn -> nothing; reconnect failure -> always
reported. The claude-mod test suite passes in both copies.

### Result (816801dbd) - 2026-10-20

Landed in 1bd9bf100 (source), a310c8f84 (tests), 816801dbd (wsflow resync).

- `request_compaction` and `request_mcp_reconnect` take `continue: boolean`
  (schema `{ continue: { type: 'boolean' } }`); only a literal `true` sets it,
  so a leftover `continuation` string reads as unset and the old multi-line
  tool error is gone (`LeverAnswer` / `ReconnectAnswer` lost their `isError`
  variant).
- Fixed texts per Decisions: `RESUME_TEXT`, `vetoedResumeText(reason)`,
  reconnect `successText(plugin)` ending in `RESUME_SENTENCE`, and
  `failureText(plugin, reason, resume)` appending that sentence with the flag.
  Lever/reconnect descriptions and results, advisory and milestone prose, and
  `agents-plugin/hooks/hooks.json` updated.
- Guard: `register.ts` `turn.start` feeds `compaction.turnStarted` and
  `reconnect.turnStarted`. A main-thread turn start cancels the held
  compaction resume (landed or vetoed); a reconnect success with the flag
  submits only when the main-thread turn counter has not moved since the step
  started. `claim` and its `prompt.submit` hook are removed. The
  compacted-elsewhere path submits `RESUME_TEXT` at the next turn end
  unguarded.
- Decision: a second hold while a window is still open replaces it instead of
  submitting the older text (the text is fixed, so one resume covers both).
- Verification: `claude plugin test agents-plugin` 302 pass / 0 fail;
  `claude plugin test agents-plugin-wsflow` 302 pass / 0 fail; `claude plugin
  validate` passes on both; `go test ./internal/wsrsrc -run
  TestWsflowHooksMirrorUpToDate` ok. Removing the `turn.start` feed fails six
  engine guard tests (mutation check, not committed).
- Lite review: no Critical or Important findings. Minor: a main-thread turn
  starting between turn.complete and the reconnect step's start is not
  counted (matches "since the step started"); `failureText` still names the
  repair skill, which `261020-chore-retire-mcp-server-repair-skill` rewrites
  at merge.
