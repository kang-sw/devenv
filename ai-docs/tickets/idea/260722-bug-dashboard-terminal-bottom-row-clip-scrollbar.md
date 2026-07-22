---
title: "ws-dashboard terminal bottom-row clipping and spurious scrollbar at fractional heights"
---

# ws-dashboard terminal bottom-row clipping and spurious scrollbar at fractional heights

## Background

Dogfood observation on 2026-07-22 against the ws-dashboard frontend terminal
pane (xterm-based terminal view). At certain container heights, the pane
misrenders in a way consistent with a row-height rounding mismatch between
the terminal's fit/rows calculation and the container's actual pixel height.

## Symptoms

- The bottom-most terminal row is clipped/cut off — partially hidden rather
  than fully visible.
- In that same state, a vertical scrollbar renders spuriously (appears when
  it should not), consistent with a fractional leftover row height that the
  container treats as overflow.

## Suspected Root Cause (unconfirmed)

Hypothesis only, not verified: the terminal viewport height is not an exact
multiple of the character/row cell height. FitAddon (or equivalent row-count
calculation) may floor/round the row count against a container pixel height
that doesn't divide evenly by the cell height, leaving a fractional partial
row. That partial row gets clipped, and the leftover overflow triggers the
scrollbar. Needs confirmation against the actual xterm/FitAddon integration
code and the container's CSS sizing (flex/grid height resolution, box-sizing,
any borders/padding eating into the computed height) before committing to a
fix approach.

## Notes

- Reproduce at "certain container heights" — exact trigger height(s) not yet
  pinned down; likely reproducible by resizing the terminal pane / browser
  window until the mismatch appears.
- Scope is frontend UI only (ws-dashboard terminal pane); no backend/agent
  behavior implicated.
