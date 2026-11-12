# Topic body frames survive a section move (Element.moveBefore)

## Goal

On the dashboard board, an open topic whose section moves during a board
swap (for example an updated topic rising from below to the head) must not
reload its body iframe blank. User approval: board item 365 a (2026-11-12).

## Background

- f62c48214 added the double buffer in `dashboard.js` `morphBoard`: a changed
  open topic whose section stays in place keeps its old section and swaps in
  the new body frame after load + two animation frames. `frameGrafts` replays
  the `cards.js` settle step (`settleMoves`) to decide whether a section would
  move; when it would, the board falls back to the old behaviour, because
  moving or re-inserting an iframe reloads it (also reloading unchanged
  topics' frames that get pushed around).
- `Element.prototype.moveBefore` (Chromium 133+) moves a node while keeping
  iframe state.

## Scope

- `agents-plugin-tool/internal/wsdashboard/assets/cards.js` settle step: use
  `parent.moveBefore(node, ref)` when available, `insertBefore` otherwise.
- `dashboard.js`: with `moveBefore` available, keep the old section (and its
  frame double buffer) for moved changed topics too, and let unchanged moved
  topics keep their frames; without it, the current fallback stays exactly
  as is. Keep `settleMoves` in agreement with the real settle (the existing
  500-board randomized test must keep passing).
- Tests: extend the Go-driven node tests (`board_frame_graft_test.go`,
  `cards_test.go`) for the moveBefore path and the fallback path (stub
  `moveBefore` presence).
- Headless check: `scratchpad/pw/dbuf.js` with `MOVE=1` (the bundled
  chromium-1223 supports moveBefore): a topic rising to the head shows no
  blank frame, and the other topics' frames keep identity (no reload).
  Light and dark sheets into `scratchpad/frame-move/`.
  Scratchpad: /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad
  (playwright-core under its `pw/node_modules`; chromium at
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`).

## Constraints

- Preserve: editing deferral, prefers-reduced-motion, new-topic slide-in,
  closing-topic fold, the topic-note float/carry-over (d9b292c82), theme
  posting, image relay, link menus, mermaid in the new frame.
- Dashboard asset change: compatibility tier T0.
- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/topic-frame-move` in this worktree; do not
  merge or push.
