---
title: claude-mod in headless sessions - compaction lever rejected, captions run uselessly
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
Orca session the owner's own `/compact`, sent from Orca's chat, also failed
(error text not captured), so the engine's "a /compact prompt" hint is not yet
shown to work on this host; whether the failure is the SDK's or Orca's is
unknown. Lead-chosen
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

Separately, tool-row captions (`claude.display_summary_enabled`,
`agents-plugin/hooks/claude-mod/captions.tsx`) draw only in a terminal view,
but the knob is read at session start with no interactivity check, so a
headless session would still spend summary-model calls on captions nobody
sees. The Pi adapter already limits its display summaries to the TUI
(`agents-plugin-pi/src/display-summary.ts` header: `ctx.mode === "tui"`; the
widget gate at `index.ts`, `shouldArmAgentWidget`).

## Decisions

- **Headless lever uses `/compact`.** When the session started with
  `isInteractive: false`, the lever runs compaction through
  `$.command.run({ command: "compact" })` instead of `$.session.compact()`.
  Its landing still counts as the lever's own compaction, so the existing
  resume prompt (`continue` flag), landed notification, and manual re-key
  behave as in an interactive session.
- **Headless forces captions off at runtime.** With `isInteractive: false`,
  captions are off for that session regardless of
  `claude.display_summary_enabled`; the config value is never rewritten.
  Rejected: writing the knob (the config is shared with interactive sessions
  on the same machine). Because the knob is read once at session start, a
  surface attached later (e.g. Remote Control) does not turn captions on for
  that session.
- **Pi needs no change** beyond confirming that the display-summary pipeline
  itself is gated on `ctx.mode === "tui"` (its header says so; the gate line
  was not located).

## Constraints

- Manuals: `ai-docs/manuals/shipped-surface-boundary.md` (mod notices ship),
  `ai-docs/manuals/wsflow-mirroring.md` (`agents-plugin/hooks/`).

## Phases

### Phase 1: Headless compaction lever

Before coding, verify on a headless session that `$.command.run({ command:
"compact" })` is accepted from the lever's turn-end step and that its landing
is observable through the same `session.compact` path the lever watches; stop
back to the lead if it is rejected or lands unobservably.

Verification: compaction engine tests for the headless branch (command.run
used, landed and vetoed outcomes, resume prompt with and without `continue`);
then the lead's live check in a headless (Orca) session: a lever call
compacts at the turn end.

### Phase 2: Headless caption force-off

Gate captions on `isInteractive` per the decision; confirm the Pi display
summary gate line.

Verification: captions engine tests: headless start with the knob on draws
and requests nothing; interactive start unchanged.
