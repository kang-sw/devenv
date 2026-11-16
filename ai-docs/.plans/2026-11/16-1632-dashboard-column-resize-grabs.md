# Plan: dashboard column resize grabs

Owner request 2026-11-16 16:28 KST, confirmed by board item (1251)=a with note
"the 'drop here to pin' placeholder already exists, so this should be doable
while keeping the current shape".

## Outcome

1. Session rail: unchanged (340px fixed, no grab).
2. New grab between the conversation panel (`.convo`) and the variable board
   (`.bcol`) in `.dgrid.live` wherever both are side by side (split/two-column
   modes and wider). Dragging sets the conversation/board split; double-click
   resets to the default ratio (today `minmax(320px,5fr) minmax(0,7fr)`).
   Keep the conversation minimum (320px) and a sane board minimum. Visual and
   interaction style match the existing `.bp-edge` grab (col-resize cursor,
   pill, primary colour on hover/drag, pointer capture, role=separator).
   Persist the same way the existing board width (`bw`) is persisted.
3. At the 4-panel width (>= 2200px, rail docked) the pinned-board column slot
   stays present even with zero pins: the existing "drop here to pin"
   placeholder (`.bpzone`) occupies that column, and the board/pinned-column
   grab (`.bp-edge`) is always visible and works there, so board width can be
   set before the first pin. Keep the current look; below 2200px behaviour is
   unchanged.

## Context (from read-only investigation; verify)

- Assets: `agents-plugin-tool/internal/wsdashboard/assets/` -
  `dashboard-layout.css:19` (`--dgrid-cols`), `dashboard-rail.css:46,63-90`
  (`.bpcol`, `.bp-edge`, `data-pincol` grids), `cards.js:64,1326-1360,536-540`
  (`BOARDW_MIN`, `PINW_MIN`, `edgeHandle`, `applyColumn`),
  `dashboard-board.js:276-279` (`pinColumnFits`), `.bpzone` rules in
  `dashboard.css`.
- `pincol_width_test.go` pins exact CSS strings; update it with the change.
- Shell iframe layout forwards `pincol` (`dashboard-lifecycle.js:483,618`,
  `shell.js`); keep framed pages consistent.

## Constraints

- No new config-manifest key (T3). localStorage/cards-record state only.
- Concurrent work on other branches: whiteboard panel-first Phase 2 (board
  stream rendering) and session iframe cache restore (shell/lifecycle). Keep
  the diff tight to layout/grab code.

## Verification

- Go tests on remote w5090 only (`internal/wsdashboard` package incl.
  pincol_width_test.go); Node tests locally if relevant.
- LOOK: capture the dashboard at ~1400px, ~1800px, ~2400px (with and without
  pins) and confirm both grabs render, drag, and reset.
