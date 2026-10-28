# Status band: live context fill and the compaction window

## Outcome

Two user-requested fixes to the Claude mod status band's context bar
(`agents-plugin/hooks/claude-mod/band.tsx`, wiring in `register.ts`):

1. **Refresh the context fill after each settled tool call**, not only on
   `session.measure`. Per the mod typings (`claude-code.d.ts`,
   `'session.measure'`), that event fires "after each main-thread turn, and
   when a rate-limit window moves a whole point", so today the bar moves only
   when the agent settles. `$.session.usage().context.tokens` is the last API
   response's input side, so a read after a tool call shows the latest
   response's fill. Trigger a usage read from `band.toolCalled` (any tool
   call, the mod's own included or not: your call), throttled the way git is
   (`GIT_THROTTLE_MS`-style constant; one in flight at a time, a trailing read
   when calls arrive during one is fine). `session.measure` keeps reading as
   today.
2. **Show the compaction window as the bar's maximum**, not the model window.
   Today `usageFigures` takes `u.context.window` (the model's window, 1M on
   this user's model) while the session compacts at a smaller window set by
   `CLAUDE_CODE_AUTO_COMPACT_WINDOW` (300000 here). The breakdown carries it:
   `context.breakdown.rawMaxTokens` ("the model's limit, or a smaller
   compaction window (`autocompactSource` says which)"). Use
   `breakdown.rawMaxTokens` when the breakdown is present and positive, else
   `context.window`; the percent and the bar fill are `tokens` over that
   window. Do not read the environment variable directly.

## Constraints

- Read `ai-docs/manuals/shipped-surface-boundary.md` and
  `ai-docs/manuals/wsflow-mirroring.md` before editing; mirror every change to
  `agents-plugin-wsflow/hooks/claude-mod/` as that manual requires (the
  band files are mirrored copies today).
- The mod typings live at
  `/private/tmp/claude-501/bundled-skills/2.1.308/*/plugin-authoring/types/claude-code.d.ts`
  (`SessionContextUsage`, `SessionContextBreakdown`, `SessionMeasureInput`).
- Keep the per-redraw path free of engine calls (band's existing
  "event-driven, never a read per redraw" rule). If a `breakdown: 'summary'`
  read per tool call looks costly, you may cache the window from the last
  breakdown read and do lighter reads in between; state the choice in the
  commit's AI Context.
- Pin both behaviors in `band-engine.test.tsx`: a tool call triggers a
  throttled usage read and redraw; the window and percent come from
  `rawMaxTokens` when the breakdown carries it and fall back to
  `context.window` otherwise.
- Verification: the plugin's own test command for the claude-mod hooks (see
  how `band-engine.test.tsx` is run in the repo, both packages) and the
  wsflow mirror check (`go test ./...` under `agents-plugin-tool/` covers the
  mirror pin).

## Boundary

- Commit on `develop` in the root checkout. Stage only your files: leave the
  untracked `ai-docs/tickets/todo/261028-feat-pi-agent-widget-batch-row-redesign.md`,
  `latest`, and `debug.log.1` alone.
- Out of scope: any other band row, the Pi agent widget, plugin version bumps.
  Return to the lead if the change needs a new config knob or touches the
  compaction feature.
