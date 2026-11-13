---
title: "Dashboard search palette: four minor leftovers from the unified-search run"
related:
  261114-feat-ws-dashboard-unified-search-palette: "source: left open by its worker (merged b9f5086e4, 2026-11-14)"
---

# Dashboard search palette: four minor leftovers from the unified-search run

## Background

The unified-search palette worker left these unfixed. Each was rated Minor or
judged pre-existing.

- **Open toast under the palette.** The result toast of an `fs/act open` from
  the palette's file slot may render behind the modal palette, so the user
  does not see whether the open worked.
- **Truncated listing unmarked.** When `fs/list` answers with `more` (a
  truncated listing), the palette's file list shows the partial list with no
  mark that entries were cut.
- **Narrow tab overflow.** At the narrow layout step, the English tab labels
  overflow the tab row, which then scrolls sideways.
- **Shell search button overlap.** In the iframe shell at narrow width, the
  shell's search button can overlap the top-left corner of a tall palette.
  This looks pre-existing and may belong to the shell's layout, not the
  palette.

## Direction

- Raise the toast above the palette layer, or route palette-originated toasts
  into the palette itself.
- Show a "more entries not listed" row when `fs/list` sets `more`.
- Let the tab row fit at the narrow step: shorter labels, icon-only tabs, or
  a smaller font. Choose against the palette mockup.
- Check the shell button's stacking and position against the palette's top
  inset at narrow width, in the shell frame.
- Verify headless on an isolated daemon at 420 px and in the shell frame.
