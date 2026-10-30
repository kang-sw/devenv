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
  `trigger: "manual"`, like a user-typed `/compact`. The lever records its
  request and treats the first `manual` compaction that lands after it as
  its own; one landing detector serves the route. The lever's landing keeps
  the existing resume prompt (`continue` flag), landed notification, and
  manual re-key; a user compaction with no lever request pending is left as
  today.
- **Split rejection reasons.** A transient rejection (a turn is running, the
  session is not idle) retries at the next turn boundary, as today. A
  permanent rejection (the route is unavailable on this host, an unknown
  command) stops retrying and fails fast with one notice. Rejected: retrying
  every rejection at every turn end (the observed headless loop).
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

### Phase 1: Unified compaction lever

Order: the headless spike is done (Background). Before coding, check the
interactive path: in a REPL session, `$.command.run({ command: "compact" })`
from the lever's turn-end step compacts and lands through the same
`session.compact` hook. Record the result, then implement the decisions with
one route if it passes, or the `isInteractive` branch if it fails; stop back to
the lead if the landing is unobservable on either path.

Verification: compaction engine tests for the chosen route (command.run used,
landed and vetoed outcomes, resume prompt with and without `continue`,
lever-versus-user discrimination, transient retry versus permanent fail-fast);
then the lead's live checks: a lever call compacts at the turn end in a
headless (Orca) session and in an interactive session.

### Phase 2: Headless caption force-off

Gate captions on `isInteractive` per the decision; confirm the Pi display
summary gate line.

Verification: captions engine tests: headless start with the knob on draws
and requests nothing; interactive start unchanged.
