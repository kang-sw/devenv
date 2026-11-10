# Plan: tracker status picker on one horizontal row

## Request

User, 2026-11-10 whiteboard, with a screenshot of the tracker item menu's
status step (the popover opened from an item's status icon, `act: mstatus`
-> `itemMenu(..., { statusOnly: true })` in
`agents-plugin-tool/internal/wsdashboard/assets/tracker.js` ~:4299, and the
status step of the full item menu): the five status options (idea /
todo / in progress / blocked / done; Korean 구상 / 할 일 / 진행 중 / 막힘 /
완료) wrap onto a second row ("완료" alone on row 2), which reads awkwardly.
"그냥 가로로 길어지게 해주세요" - let the picker grow horizontally so all
options stay on one row.

## Changes

- Make the status option group a single non-wrapping row in every place
  that renders it (status-only popover and the full item menu's status
  step), letting the popover widen to fit. Keep the existing option look
  (icon above label, selected outline). The memo input row below may
  stretch to the new width.
- Keep the popover inside the viewport: the existing placement code
  (`placeMenu`) should still clamp it; check it does at narrow widths and
  near the right edge. If the row cannot fit the viewport at all (very
  narrow window), horizontal overflow inside the popover is acceptable;
  prefer that over wrapping only if trivial, otherwise allow wrap below a
  small breakpoint.
- Styles in `internal/wsdashboard/assets/tracker.css`; existing tokens only;
  light and dark.

## Verification

- `nice -n 19 node --check` on any touched JS.
- If cheap, a headless screenshot of the status popover (isolated dev
  daemon with its own WS_CACHE_HOME / WS_CONFIG_HOME and port, or a static
  stand-in page with tracker.css). Never touch processes or ports 29417 /
  45743.
- Go tests only remotely: `~/.local/bin/ws-rgo -C
  /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool
  -m 30 -- test ./internal/wsdashboard/ -count=1 -timeout 30m`. Never run go
  build/test/vet locally.
- Commit on branch `fix/tracker-status-picker-row`; do not push. No `git stash`.
