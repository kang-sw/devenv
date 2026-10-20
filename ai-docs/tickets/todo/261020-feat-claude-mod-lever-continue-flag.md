---
title: Make the Claude mod lever continuation a flag that yields to an already-started turn
related:
  261017-feat-claude-mod-lead-compaction-timing: origin of the compaction lever and its continuation contract, which this ticket revises
  261018-feat-claude-mod-mcp-reconnect-lever: origin of the reconnect lever's continuation
  261004-feat-pi-explicit-post-compaction-continuation: Pi precedent for a boolean continuation with a fixed resume text
  261020-chore-retire-mcp-server-repair-skill: sibling; also edits reconnect.ts
  261020-bug-claude-mod-milestone-percent-stale: sibling; also edits compaction.ts and timing.ts
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
  since the step settled (register.ts feeds `turn.start` into the compaction
  module). The composer `claim` path and its `prompt.submit` hook are removed.
  Rejected: keeping `claim` (any started turn, typed or not, now disarms the
  resume, so carrying it as context into a typed prompt is no longer needed).
- **Reconnect.** The success resume (flag set) uses the same guard, measured
  from the reconnect step's start. A failure report is always submitted, with
  the resume line when the flag is set. Rejected: guarding the failure too
  (it carries information the lead needs whatever turn is running).
- **Another compaction first.** When another compaction lands before the
  lever's own (`compacted` path), the resume still follows at the next turn
  end, now with the fixed text; only the text changes.
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
