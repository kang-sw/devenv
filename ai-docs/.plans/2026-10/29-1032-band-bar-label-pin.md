# Status band context bar: left-pinned label with per-cell contrast

## Goal

The status band's row 2 context bar label (`agents-plugin/hooks/claude-mod/band.tsx`,
`barRuns`) disappears too easily: an all-or-nothing `fits` check drops the
whole label (~26 cells) whenever the bar is narrower, and `row2Layout` shrinks
the bar to `BAR_FLOOR` (10) before dropping a right capsule. The label also
moves with the fill edge. Make the label stay readable and stable.

## Settled behavior (user-approved)

1. The label is pinned to the left: it starts at the bar's cell 0 at every
   percentage.
2. Per-cell coloring: a label cell lying over the filled part draws `C.onFill`
   on the fill color (`barColor`) as its background; a label cell over the
   unfilled part keeps its run's original color with no background.
3. The 1/8-block fractional fill glyph is drawn only where the fill edge lies
   past the label's end. When the fill edge cell sits under a label character,
   that cell counts as filled when the fractional part is >= 0.5.
4. Graceful truncation replaces the all-or-nothing drop: when the full label
   does not fit, drop label segments in this order — cache hit (`⚡..%`), then
   the `/ <window>` segment, then the token count — and always keep the
   percent. If even the percent does not fit the bar width, clip it to the
   width rather than dropping it.

## Scope

- `agents-plugin/hooks/claude-mod/band.tsx` (`barRuns`, and its call site in
  row 2 if the label segments must be passed in droppable form).
- `agents-plugin/hooks/claude-mod/band-engine.test.tsx` bar tests: update them
  to the new contract and cover the left pin, the split coloring at a mid
  percentage, the fractional rule, and each truncation step.
- Mirror `band.tsx` (and the test, if wsflow carries a copy) byte-identically
  into `agents-plugin-wsflow/hooks/claude-mod/`; read
  `ai-docs/manuals/wsflow-mirroring.md` first.

Out of scope: `barCaps` semantics, `row2Layout`, colors in `C`, other rows.

## Verification

Run the claude-mod test suite and whichever package test pins the wsflow
mirror (see the mirroring manual), and report the commands and results.

## Return boundary

Commit on this worktree's branch (`delegate/band-bar-label-pin`). Do not merge.
Stop and report if the change needs anything beyond the scope above.
