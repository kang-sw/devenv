---
title: Claude mod status band behind a config knob
related:
  261028-research-claude-mod-statusline-band: source research; its Outcome Ledger is the contract
sage-review-design: completed
sage-review-completeness: completed
sage-review-completeness-reviewed: 4b7b291a83cfc70a
sage-review-design-reviewed: 4b7b291a83cfc70a
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
  `agents-plugin/config-manifest.json`, read through `config_get` as
  `claude.display_summary_enabled` is (`captions.tsx:52`, `register.ts:564`),
  passing the held session key when there is one so a session-scope
  `lead-tune` is honored, and keyless otherwise. Rejected: a keyless-only
  read, which silently ignores a session-scope tune.
- **The knob is read at session start and again after each `config_tune`
  tool call in the conversation**, so a `lead-tune` on / off shows without a
  restart. Rejected: reading it once at session start as captions do.
- **The band is a new feature module (`band.tsx`) joined to `register.ts`'s
  shared lifecycle fan-out**; Claude Code accepts one hooks module per plugin
  and one unmatched hook per event.
- **The band runs only in the ws package** (`compactionEnabledFor`), as
  captions, compaction and manual do; wsflow draws nothing and ships no
  config manifest.
- **The ws rows read `todo_list(mode: "full")` and `agenda_list` for the key
  the manual feature already holds** (`switchKey`, `manual.ts:398`), and the
  mailbox address comes from `mailbox_lookup_peers` `format: "json"` through
  the existing `selfAddress()` (`register.ts:148`). Both ws reads are
  read-only and list every item. Rejected: `workflow_state`, which renders
  todos in summary mode and folds done items into one `...` line, so
  `todo <done>/<total>` would undercount with no sign
  (`workflow_manual.go:143`, `session_state.go:289`); dropping the
  done/total capsule; a JSON format for `workflow_state`, an MCP protocol
  change outside this ticket. Rejected: a second key or address source.
- **Reads fail closed**: a failed or denied knob read counts as off, a
  failed ws read hides the ws rows, and a failed mailbox read omits the
  mailbox capsule as when the key owns no address. The knob, the ws rows and
  the mailbox address are each read at session start, again whenever the
  held key changes (manual's `keyChanged`, which also covers a re-login or
  an adoption, since `setup` runs `ferrule` before `switchKey`), and when
  the knob turns on, besides their refresh triggers. A session-start read
  that runs before any key is held is keyless for the knob and skips the ws
  reads. Rejected: leaving the first read to
  the next matching tool call, which hides existing todos until one
  happens.
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
  drop in the order cost, 7d, 5h. Once that order is exhausted, every row
  truncates its flexible span (`truncate-end`); no row wraps. Rejected: drawing nothing (`next(e)`)
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
- Convention: ai-docs/manuals/shipped-surface-boundary.md (declared for `agents-plugin/`, `agents-plugin-wsflow/`, `agents-plugin-tool/`)
- Convention: ai-docs/manuals/wsflow-mirroring.md (declared for `agents-plugin/rsrc/`, `agents-plugin/skills/`, `agents-plugin/hooks/`, `agents-plugin-wsflow/`)

## Prior Decisions

- 261028-research-claude-mod-statusline-band (2026-10-28, Confirmed Decisions): "Fold the statusline into the claude mod as an `AbovePrompt` band, rather than keeping a shell `statusLine.command`." — bearing: supports
- 261028-research-claude-mod-statusline-band (2026-10-28, Confirmed Decisions): "The band's layout, frame, ws rows, mailbox capsule, knob and gating were settled with the user in the spike (2026-10-28) and are the contract of `261028-feat-claude-mod-status-band`" — bearing: supports
- cbe95447 (2026-10-28, commit): "agenda/todo content and the default-off gate stay Proposals." — bearing: supports
- 261018-feat-claude-mod-compaction-threshold-knob (2026-10-18, Decisions): "D1. Read at session start, fall back to 50%. The mod reads the knob once per session start through its existing MCP path; any failure ... leaves the advisory at 50%." — bearing: constrains
- 61e91ae6 (2026-10-21, commit): "loadDeclaredConfig rejects a bad manifest path silently instead of failing startup, so D12 and the live check now verify the key appears in config.list on both launches." — bearing: constrains
- 0acdd4a5 (2026-10-17, commit): "the knob read through $.mcp.call config_get is itself permission-gated (prompts unchecked, reads false for auto users when pre-checked)." — bearing: constrains
- ecb9b7e8 (2026-10-17, commit): "Knob manifest is set by the launcher only when WS_MCP_CONFIG_MANIFESTS is unset, because ws's .mcp.json is shared with Codex and pi's bridge sets the variable itself." — bearing: constrains
- 9007e572 (2026-10-23, Result): "Landed in `compaction.ts`, `timing.ts`, `register.ts` and both test files, mirrored byte-identical into `agents-plugin-wsflow/hooks/claude-mod/`." — bearing: constrains

## Route Facts

| fact | value | evidence |
|---|---|---|
| scope.span | multi-file | agents-plugin/hooks/claude-mod/band.tsx new, agents-plugin/hooks/claude-mod/register.ts, agents-plugin/config-manifest.json, agents-plugin-wsflow/hooks/claude-mod/ mirror, new engine test |
| scope.surface | public-interface | new user-facing config key claude.status_band_enabled in agents-plugin/config-manifest.json and a new AbovePrompt ui.render hook joined to register.ts lifecycle fan-out |
| scope.new_public_symbol | yes | config key claude.status_band_enabled; band.tsx feature exports for register.ts |
| scope.new_type_contract | yes | band feature edges interface analogous to CaptionEdges in agents-plugin/hooks/claude-mod/captions.tsx#L413-L418 |
| scope.test_surface | new-files | no band test exists; pattern agents-plugin/hooks/claude-mod/captions-engine.test.tsx; manifest keys pinned in agents-plugin-tool/internal/mcp/config_manifest_test.go#L529; mirror pinned by agents-plugin-tool/internal/wsrsrc/wsflow_hooks_mirror_test.go |
| complexity.reuse_points | confirmed | selfAddress register.ts#L148, config_get readSetting register.ts#L562-L568 (today keyless; the band passes the held key), compactionEnabledFor compaction.ts#L68, manual key switchKey manual.ts#L398, spike ai-docs/ref/claude-mod-status-band-spike.tsx barRuns L102 cells L91 |
| complexity.side_effect_risk | moderate | joins the shared register.ts fan-out and adds config_get, todo_list, agenda_list and mailbox_lookup_peers calls plus a git read and 10 s tick in every ws Claude session |
| risk.correctness | moderate | width-aware layout, emoji cell widths, degrade order and event-driven refresh with throttles have several edge cases |
| risk.fit | moderate | spike comments name shell/statusline.sh at spike L36, which shipped code must not; wsflow byte-identical mirror required |
| risk.test | moderate | engine tests can pin the drawn tree but not what the terminal actually draws for emoji widths |
| risk.security_or_contract | moderate | adds a public config key and consumes the mailbox_lookup_peers JSON self shape; config_get read is permission-gated per 0acdd4a5 |

## Phases

### Phase 1: Status band behind `claude.status_band_enabled`

Port the spike's band (`ai-docs/ref/claude-mod-status-band-spike.tsx`) into
the Claude mod per `## Decisions`, gated by the
knob, with live data for every row (the spike's ws rows and mailbox address
are fixed strings). Add the manifest key, resync the wsflow copy, and pin the
drawn tree with engine tests for: knob off (nothing drawn), status rows only,
status plus ws rows, no mailbox address, a `config_tune` toggle mid-session,
the narrow-width drop order and its end state below every floor (no row
wraps, the frame and its titled top edge stay intact), and the bar's cap colors at 0%, mid-range and
100%. Pin the new key's built-in value beside the caption keys in
`agents-plugin-tool/internal/mcp/config_manifest_test.go`
(`TestConfigGetReadsClaudeCaptionKeys`). Engine tests also pin a failed knob
read (nothing drawn), a failed ws read (status rows only), the reads that
follow a `keyChanged`, and a session-keyed knob read. `todo.list` has no
structured format, so the band parses its full-mode text; pin that parse
against the server's rendering of each todo status.

Then check a live session, which engine tests cannot see: the key appears in
`config.list`, a todo update under the default permission mode raises no
permission dialog for the band's reads, and the bar's width matches what the
terminal draws with the emoji cells. If a permission dialog does appear,
stop and report instead of redesigning the read path.
