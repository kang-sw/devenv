---
title: Claude mod status band behind a config knob
related:
  261028-research-claude-mod-statusline-band: source research; its Outcome Ledger is the contract
---
# Claude mod status band behind a config knob

## Background

The research ticket `261028-research-claude-mod-statusline-band` showed that
the Claude mod can draw the shell statusline's figures as an `AbovePrompt`
band with width-aware layout, and a local spike settled the band's look with
the user (2026-10-28). This ticket ships that band in the ws plugin's Claude
mod, off by default and turned on through a config knob the user raises with
`lead-tune` / `config.tune`. The spike's rendering code is the starting point
for the implementation.

## Decisions

- **The band is an `AbovePrompt` `ui.render` hook in the Claude mod**, not a
  shell `statusLine.command` (research Confirmed Decision). Rejected:
  `$.ui.status(text)`, a single unstyled string.
- **The band is off by default and turned on by a boolean config knob** set
  through `lead-tune` / `config.tune`.
- **Frame: a hand-drawn rounded border around the band's rows, with the model
  as the border title**: the top edge is one text row
  `╭─ <model bold> [<effort> dim] ───╮` sized to `bodyColumns`, the sides are
  `│` columns beside the content rows, the bottom edge is `╰───╯`. Rejected:
  `Box` `borderStyle`, which draws a border but carries no title; an absolute
  child at `top={-1}` laid over the border row, which the spike showed is not
  drawn.
- **Row 1**: left, the mailbox self-address capsule, then the folder capsule
  spanning the row's spare width (folder name, the cwd relative to the
  project root dimmed); right, the branch capsule (ahead / behind) and the
  changes capsule (`+added -deleted ~modified ?untracked`, or `clean`).
- **Row 2**: left, the context progress bar capsule taking the row's spare
  width; right, the 5h and 7d rate-limit capsules (percent, reset hour / day,
  budget delta) and the cost capsule.
  - The rate-limit capsules carry no `5h` / `7d` label: the reset reads as
    an hour (`21H`) for the 5h window and a weekday (`Sun`) for the 7d one,
    which tells them apart.
  - The bar is statusline.sh's full-width bar (`1a68831e8`): 1/8-block fill
    in the percent's gradient color, with the label
    `<pct>% <tokens> / <window> ⚡<cache-hit>%` laid over it just past the
    fill edge and clamped to fit.
  - Each round cap of the bar capsule takes the fill color when the fill
    reaches it, so the capsule end stays round.
- **Row 3 (ws focus row)**: the session's in-progress todo, or its first
  pending todo when none is in progress, as `▶ <key>  <title>` bright on the
  left; a `todo <done>/<total>` capsule on the right.
- **Row 4 (ws rest row)**: the remaining pending todo keys as dim `○ <key>`
  chips on the left, folding into `+N`; on the right an `agenda` capsule
  followed by the agenda blob keys as plain dim text.
- **The ws rows appear only when they have content**: with no todos and no
  agenda the band is the two status rows inside the frame.
- **The model name is plain bold text in the border title, not a capsule.**
- **The mailbox capsule shows the key's own named-inbox address** and is
  absent when the key owns none.
- **The knob is `claude.status_band_enabled`**, boolean, default `false`, in
  `agents-plugin/config-manifest.json`, read through a keyless `config_get`
  as `claude.display_summary_enabled` is (`captions.tsx:52`,
  `register.ts:564`).
- **The knob is read at session start and again after each `config_tune`
  tool call in the conversation**, so a `lead-tune` on / off shows without a
  restart. Rejected: reading it once at session start as captions do.
- **The band is a new feature module (`band.tsx`) joined to `register.ts`'s
  shared lifecycle fan-out**; Claude Code accepts one hooks module per plugin
  and one unmatched hook per event.
- **The band runs only in the ws package** (`compactionEnabledFor`), as
  captions, compaction and manual do; wsflow draws nothing and ships no
  config manifest.
- **The ws rows read `workflow_state` for the key the manual feature already
  holds** (`manual.ts:450`), and the mailbox address comes from
  `mailbox_lookup_peers` `format: "json"` through the existing
  `selfAddress()` (`register.ts:149`). Rejected: a second key or address
  source.
- **Refresh is event-driven**: usage on `session.measure`, model and effort
  on main-loop `turn.step`, git on `tool.call` (2 s throttle) and
  `turn.complete`, the ws rows after a `todo_*` / `agenda_*` tool call, the
  mailbox address after a `mailbox_*` tool call, and a 10 s tick for the
  budget deltas. Rejected: an MCP call per redraw, which hits the server on
  every frame.
- **The side bars are built from the drawn content row count.** Rejected:
  the spike's fixed four, which breaks the frame when the ws rows are absent.
- **Narrow terminals degrade in order**: the spanning folder capsule
  truncates, the bar shrinks to a 10-cell floor, then row 2's right capsules
  drop in the order cost, 7d, 5h. Rejected: drawing nothing (`next(e)`)
  below the floors' width, since a band that vanishes and returns with the
  width is more jarring than a thinner one; letting rows wrap, which breaks
  the frame.
- **One phase.** Rejected: a status-rows phase then a ws-rows phase; the
  mailbox capsule sits on row 1, so the split separates nothing.

## Prior Art

- `ai-docs/ref/claude-mod-status-band-spike.tsx`: the spike's hooks module
  with its types inlined, the rendering to port (frame, capsules, spanning
  pill, `barRuns`, `cells`, git and usage reads). Its ws rows and mailbox
  address are fixed strings, and its comments name this repository's shell
  statusline, which shipped code must not.
- `agents-plugin/hooks/claude-mod/captions.tsx`: the config-gated feature
  pattern (`ENABLED_KEY`, `readSetting`, `redraw`).

## Constraints

- The Claude mod ships to every downstream project:
  `ai-docs/manuals/shipped-surface-boundary.md` and Architecture Rule 4
  apply. Nothing this repository alone has (its `shell/statusline.sh`, its
  maintainer's settings) may be named in shipped text or code.
- `ai-docs/manuals/wsflow-mirroring.md` applies to every edit under
  `agents-plugin/hooks/`.
- Mod authoring traps from the research ticket: a function receiving `$` is
  declared at module top level; a `turn.step` hook is
  `async function* ($, e, next)` returning `yield* next(e)`.
- `$.session.usage().context.tokens` counts the input side only; effort is
  known only from main-loop `turn.step` (`e.agentId === undefined`), so the
  title shows no effort until the first request.
- Cell widths: the spike counts emoji (`📁`, `🌿`, `⚡`) as two cells; the
  width math that sizes the bar must agree with what the terminal draws.
- The native status line still draws when its `statusLine.command` is set;
  this ticket does not change Claude Code settings.

## Phases

### Phase 1: Status band behind `claude.status_band_enabled`

Port the spike's band (`ai-docs/ref/claude-mod-status-band-spike.tsx`) into
the Claude mod per `## Decisions`, gated by the
knob, with live data for every row (the spike's ws rows and mailbox address
are fixed strings). Add the manifest key, resync the wsflow copy, and pin the
drawn tree with engine tests for: knob off (nothing drawn), status rows only,
status plus ws rows, no mailbox address, a `config_tune` toggle mid-session,
the narrow-width drop order, and the bar's cap colors at 0%, mid-range and
100%.
