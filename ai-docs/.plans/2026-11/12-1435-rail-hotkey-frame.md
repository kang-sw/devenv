# Ctrl+` from inside topic body frames; manual session labels in the body font

## Goal

Two small dashboard fixes from user feedback 2026-11-12 (board topics
rail-hotkey and session-label).

1. Ctrl+` (toggle the overlay rail, `dashboard.js` ~4867) does nothing
   while focus is inside a topic body frame (`iframe.doc-frame`), because
   key events go only to the focused document. (401)=a: at 1600px and up
   (`railWide()`) it stays a no-op.
2. A manually set session display label (4b0a95db7: `.Label`, shown via
   `.rid` in the rail row `rrow`, the index `srow`, and the header `sname`)
   renders in the mono bold face, which looks bad for Korean text.

## Decisions (settled)

- (402) The topic body frame script
  (`agents-plugin-tool/internal/wswhiteboard/assets/doc-frame.js`) listens
  for Ctrl+` (same test as the host: `e.code === "Backquote"`, ctrlKey
  only, not composing) and posts a message to the parent (follow the
  existing `{wbDoc: ...}` message shape). The dashboard's frame message
  handler (~2620, which already resolves the sending frame) accepts it
  only from a real doc-frame and runs the same toggle path as the host
  keydown, skipping when `railWide()`. The HTML file preview / file view
  frames (user documents in a sandbox) are excluded: no script injection
  into user documents.
- (407) When a session has a manual label, the label text uses the normal
  body font (the page's sans `--wb-*` font token, not `--wb-mono`), weight
  600, same colour as today, in the header name, the rail row, and the
  sessions index row; an unlabeled reply ID keeps the mono face. Do it with
  a class set only when `.Label` is non-empty (template), plus the live
  rename path in dashboard.js that updates the label in place so the class
  follows a rename/reset without reload.
- Dashboard/whiteboard asset change only: compatibility tier T0.

## Verification

- Tests: extend the Go-driven node tests that cover doc-frame messages /
  the rail toggle (search `*_test.go` for `doc-frame`, `railOpen`,
  `Backquote`) for: a frame message toggles the rail below 1600px, is
  ignored at >=1600px, and is ignored from a non-doc-frame source; and a
  template/render test that a labeled session row carries the class and an
  unlabeled one does not.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  focus inside a doc-frame, press Ctrl+`, rail opens; a Korean label in
  header and rail, light and dark; shots into scratchpad/rail-hotkey-frame/.

## Constraints

- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wswhiteboard/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/rail-hotkey-frame` in this worktree; do not
  merge or push.
