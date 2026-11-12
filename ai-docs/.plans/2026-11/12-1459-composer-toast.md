# Composer notes move to toasts; the composer note line goes away

## Goal

User feedback 2026-11-12 (board topic composer-note): after d4cd50a9a
dropped the "sent" note, the composer's note line (`#cnote`, set by
`syncNote` in `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`)
still shows failure and warning text under the input, growing the composer by
a row. The page already has a bottom-right toast surface (`WsToast.show` in
`assets/toast.js`); failures should be a warning toast there instead.

## Decisions

- Remove the `#cnote` element (template / markup and its CSS) and the
  `syncNote` note-line machinery, including the NOTE_OK TTL added in
  d4cd50a9a; the composer no longer reserves a row for notes.
- Every failure or warning that today goes through `syncNote` shows a warning
  toast instead: send failure, empty message, still-reading images, image
  read failure, image refused, cancel / interrupt / steer failure, draft
  images lost. Same i18n strings as today (keys keep their names).
- Successes that today show a note (cancel ok, interrupt ok, steer ok,
  interrupt restored) show a normal `info` toast. A successful send still
  shows nothing.
- Add a `warn` kind to `WsToast` (toast.js header comment documents it): a
  quiet warning look in the `--wb-*` tone (for example a left bar or icon in
  the existing warning/policy colour, readable in light and dark), duration
  about 6000 ms. Keep toast.js host-neutral (the tracker renderer shares it).
- Calls that only cleared the note (on input, chip changes) are dropped.
- Dashboard asset change only: compatibility tier T0.

## Scope and verification

- `assets/dashboard.js`, `assets/toast.js`, `assets/dashboard.css`, the
  page template / Go markup holding `#cnote` (find it), i18n tables only if a
  string becomes unused (remove it from every locale table, as d4cd50a9a did).
- Tests: replace `composer_note_test.go` (and adjust `railscript_test.go`'s
  send-path regex) with Go-driven node tests that a failure path calls
  `WsToast.show` with kind `warn`, a success path with the default kind, a
  successful send shows no toast, and the page has no `#cnote`.
- Headless check (playwright-core under the scratchpad's `pw/node_modules`;
  chromium `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`;
  scratchpad /tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad):
  the composer without the note row, and a warn toast next to an info toast,
  light and dark; screenshots into scratchpad/composer-toast/.

## Constraints

- Heavy Go test runs only through
  `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wsi18n/...`;
  never run go build/test/vet locally.
- Commit on branch `delegate/composer-toast` in this worktree; do not merge
  or push.
