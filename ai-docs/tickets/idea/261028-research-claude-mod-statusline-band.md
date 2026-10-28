---
title: Claude mod status band replacing the shell statusline
---
# Claude mod status band replacing the shell statusline

## Background

`shell/statusline.sh` is wired by hand as the Claude Code `statusLine.command`
on the maintainer's machine (`~/.claude/settings.json`; `install.sh` does not
write it). Bash limits it: no right-aligned spans against the terminal width,
three natural-width rows only, and every computation squeezed into a few forks.
The user wants the statusline folded into the repository's Claude mod
(`agents-plugin/hooks/claude-mod/`) so the layout can use real width-aware
composition, and sees room in the freed space for more session context, such
as the ws session's agenda and todos.

A local spike (2026-10-28) drew the statusline's figures as an `AbovePrompt`
band from a throwaway mod. The user judged the result good and asked that the
method be captured here before the detailed UX is designed.

## Surface: why the AbovePrompt band

The mod API (Claude Code 2.1.310 types, marked early access) has no surface
that replaces the built-in status line row. The candidates it does offer:

- `ui.render` on `{ component: 'AbovePrompt' }`: a JSX tree drawn directly
  above the prompt, one instance, collapsible by the person (`[-]`, ctrl+x
  ctrl+a). Props carry `bodyColumns` (the usable width, the engine's five-cell
  mark excluded), `maxRows` (in fullscreen the bottom slot, capped at half the
  terminal's rows with the prompt included), `isWorking`, `hasSurvey`.
- `$.ui.status(text)`: one plain string per plugin under the prompt; no
  styling and no alignment, so a regression from the shell statusline.
- `PromptHint` (a rewrite or a dim tail on the hint line) and `$.ui.toast`:
  too small for this.

`Box` accepts an allowlisted flex subset (`flexDirection`, `justifyContent`
including `space-between`, `gap`, `width`, `flexShrink`, `backgroundColor`,
...) and `Text` takes `color` / `backgroundColor` as theme keys or raw hex, so
left and right spans per row are expressible. A prop outside the allowlist
fails the whole tree and the engine draws its own.

## Data sources the spike used

| statusline.sh figure | mod source |
|---|---|
| model display name | `$.session.model()` at start; `turn.step`'s `e.model` per main-loop request |
| effort | `turn.step`'s `e.effort` (main loop: `e.agentId === undefined`); no session-level getter, so it is blank until the first request |
| cwd / project dir | `$.session.cwd()` / `$.session.root()` |
| context tokens / window | `$.session.usage().context.tokens` / `.window`; `tokens` counts the input side only, where the shell summed output tokens too |
| cache-hit rate | `$.session.usage({ breakdown: 'summary' }).context.breakdown.apiUsage` cache read vs cache creation |
| 5h / 7d rate limit, reset | `usage().rateLimits` entries `five_hour` / `seven_day`: `percentUsed`, `resetsAt` (ISO string, not epoch) |
| cost | `usage().cost.usd` |
| git branch, ahead/behind, +/-/~/? | `$.process.run(['git', ...])` with `GIT_OPTIONAL_LOCKS=0` and `-c diff.autoRefreshIndex=false`, as the shell does; the mod has no fs |

Refresh triggers: `session.measure` (pushed when context, a rate-limit
window or cost moves), `turn.step`, `tool.call` (git, throttled to 2 s),
`turn.complete`, and a 10 s `$.clock.every` tick for the budget deltas.
Values live in one `$.state` atom so a write redraws the band without
`$.ui.invalidate`.

Mod authoring traps the spike hit:

- A function that receives `$` must be declared at the top level of the
  module (a function declaration or a top-level const); `claude plugin
  validate` refuses a closure inside `register` that takes `$`.
- A `turn.step` hook streams: it must be `async function* ($, e, next)` and
  forward with `return yield* next(e)`.
- The spike built pills with a plain helper call instead of a JSX function
  component; whether the tree accepts function components was not tested.

## Constraints

- The claude-mod ships in the `ws` plugin to every downstream project, and
  the `wsflow` copy is byte-identical (`ai-docs/manuals/wsflow-mirroring.md`).
  `ai-docs/manuals/shipped-surface-boundary.md` and Architecture Rule 4 apply:
  nothing this repository alone has may leak into the shipped band.
- Claude Code accepts one hooks module per plugin and one unmatched hook per
  event, so the band joins `register.ts`'s shared lifecycle fan-out rather
  than registering its own `session.start`.
- The existing `captions` and `manual` features gate on the plugin name and a
  ws project; a statusline band is useful outside ws projects too, so its
  gate is a separate question (see Open Questions).

## Outcome Ledger

### Verified Findings

- The mod API has no status line replacement; the `AbovePrompt` band is the
  only surface with a layout tree and width information (Claude Code 2.1.310
  types: `RenderComponent`, `AbovePrompt` props, `$.ui.status`).
- Every figure the shell statusline extracts is reachable from the mod, with
  the caveats in the data table: effort only from `turn.step`, context tokens
  counted on the input side only, reset times as ISO strings.
- A two-row band with `justifyContent: 'space-between'` per row, built from
  the sources above, passed `claude plugin validate` and `tsc`, loaded
  through mod hot reloading, and drew; the user judged it good (2026-10-28).
- The mod can reach the ws MCP server through `$.mcp.connect($.plugin.name)`
  (`agents-plugin/hooks/claude-mod/register.ts`, the mailbox feature), which
  is the path a band would read session state through.

### Confirmed Decisions

- Fold the statusline into the claude mod as an `AbovePrompt` band, rather
  than keeping a shell `statusLine.command`.
- The band's layout, frame, ws rows, mailbox capsule, knob and gating were
  settled with the user in the spike (2026-10-28) and are the contract of
  `261028-feat-claude-mod-status-band`; the spike source is kept at
  `ai-docs/ref/claude-mod-status-band-spike.tsx`.

### Proposals

- Use the band's spare rows for ws session context: the agenda and open
  todos of the session's lead key, read through the ws MCP server.
- Gate the band behind a config knob, default off, the way captions use
  `claude.display_summary_enabled`.

### Open Questions

- Band layout: which figures share a row, how many rows, what is packed and
  what gets its own row, and how the band degrades on a narrow terminal.
- Which ws session state earns band space (agenda, todos, mailbox peer state,
  others) and how it is read and refreshed without a call per redraw.
- Gating: on outside ws projects or not, default on or off, and whether the
  wsflow derivative carries it.
- Whether `shell/statusline.sh` is retired once the band lands, or kept for
  hosts without the mod.
- Unchecked spike details: whether nested `Text` keeps the pill background,
  whether `Date` in the mod sandbox formats reset hours in local time, and
  how the band coexists with engine content in the same slot.

### Rejected Alternatives

- `$.ui.status(text)`: a single unstyled string, a regression from the shell
  statusline.
