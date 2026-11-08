# Plan: tracker mockup layout - one Items panel, quiet rows, details on hover

Branch `delegate/tracker-mockup-layout` (worktree
`/Users/kang-sw/.ws-worktrees/devenv/declared-satiable-proximity`). Commit on
this branch (several commits are fine); do not merge or push.

Context: the reference mockup `ai-docs/ref/tracker-mockup/` (README is the
data-structure contract; `SCENARIOS.md` is the walk) of epic
`261107-epic-ws-personal-common-store`, just restyled in the dashboard tone
with a theme setting and a masonry packer (merge `9b35f65fd`). Screenshots of
the current state: `/tmp/tracker-mockup-styling/`. The Go port
`agents-plugin-tool/internal/wstracker` reads `engine.js` through
`testdata/gen-golden.js`; this change is presentation only and must leave the
engine, fixtures' meaning, and the view-state shape untouched.

## The user's design language (2026-11-08, verbatim intent)

"기본 상태에서 꼭 보여야 할 것만 그리고, 디테일은 마우스 호버에서." Today
there are too many badges and nothing stands out. Action buttons (Close,
Split, Hand off, Status, Accept, ...) appear only on hover, as icons. Status
is a coloured icon with its description on hover.

## Decisions (user, board (350)-(360), 2026-11-08)

1. **Row defaults** ((356) b). An item row shows by default: status icon,
   title, assignee initials, priority (always), due date (always), a warning
   icon for flagged or concurrent, and an unread dot where it applies. Due
   format: within 7 days `D-N`, today `D-day`, past due `D+N` in the danger
   colour, farther away a short date (`11-21`). Everything else (kind chip,
   source clone and event id, dates, roll-up counts, assignment path, notes,
   buttons) moves to hover.
2. **Hover has non-hover equivalents** ((354) ok). Keyboard focus on a row
   shows the same actions and details; on touch, tapping a row selects it and
   reveals its actions and details (one CSS state for hovered, focused, and
   selected).
3. **Status icons** ((355) ok). Each status differs by shape as well as
   colour (for example todo ○, doing ◐, blocked ⊘, done ✓, awaiting confirm
   ⧗, pending ?; pick a coherent set, inline SVG preferred). Descriptions use
   an immediate custom tooltip, not the browser's delayed `title`. Keep the
   icon set in one place so the dashboard can adopt it later.
4. **One Items panel** ((357) a). My assignments, Assigned by me, My team,
   Received requests, and Items merge into one Items panel with scope presets
   (mine / received requests / assigned by me / my team / all) crossed with
   the existing view modes (list, tree, kanban, timeline, calendar where they
   apply today). My team becomes a group-by-person view of the "my team"
   scope with each group header carrying that person's roll-up counts.
   Accept / Decline / Discuss / Confirm / Reject / Return / Close / Split /
   Hand off / Status become hover actions on the row they act on. The
   "for reference" notices My team showed today go to the Notifications card
   (it already lists them as reference notices). This reverses the earlier
   rule that Received requests and My team keep their buttons; update the
   README accordingly.
5. **Today stays** ((358) a, (352) a). A compact summary card (my open work /
   waiting for my answer / waiting for my confirm) that stays visible whatever
   scope the panel shows. Notifications also stays as is.
6. **Two-zone drawer** ((359) a). The wide Items panel (about two thirds of
   the width) is the main area; a side column stacks Notifications → Today →
   Support agent → Team → Activity (Data (debug) last). At narrow widths the
   side column moves above the panel. The masonry packer is no longer needed
   for this layout; remove it rather than keep dead code.
7. **Card height** ((350) a). Every card has a maximum height (about 70% of
   the viewport) with internal scroll; fixed-height cards (Support agent)
   stay fixed. The Items panel may use the full height of the main zone.
8. **Default scope** ((360) ok). The panel opens on "mine"; the last chosen
   scope and view mode are remembered per store as local UI state, like the
   existing per-card mode memory.
9. **Header** ((353) ok). At 1280px the gear must not wrap onto a second line:
   group the mockup-only controls (viewing as, clone, clock) compactly.

Keep the theme setting, the light and dark palettes, and every behaviour
`SCENARIOS.md` walks (notifications, mentions `@` picker, attachments, flagged
writes, kanban, timeline, calendar, dialogs, thread sheet).

## Verification

- `node --check` on every mockup script; no console errors in headless
  Chromium (cached Playwright under `~/.npm/_npx/*/node_modules/playwright`)
  for maya (lead), ada (admin), lena, and one follower.
- Screenshots to `/tmp/tracker-mockup-layout/`: each of those viewers at
  1280px and 1680px in light and dark; one row hovered showing its actions
  and tooltip; one row selected by a tap emulation (touch context); the
  panel in each scope preset; the my-team group view; a narrow width (~800px)
  showing the side column above the panel.
- Rerun SCENARIOS.md (at least scenarios 1, 4, and 10 and any scenario whose
  steps referenced a removed card) through the new panel and update the
  scenario text where a step named a removed card or button location.
- `cd agents-plugin-tool && go test ./internal/wstracker/` passes and
  `git diff develop -- ai-docs/ref/tracker-mockup/engine.js` is empty.
- README: the cards / drawer section describes the new layout, the panel
  scopes, hover actions and their non-hover equivalents, the row defaults and
  due format, and Settled rows for (350), (352)-(360) (with (357) noting it
  supersedes the buttons-stay-in-cards part of (331)-(334)).

## Return

Commits, changed files, screenshot paths, test output, SCENARIOS steps
changed, and anything left open (stop and report rather than inventing a
data-structure change).
