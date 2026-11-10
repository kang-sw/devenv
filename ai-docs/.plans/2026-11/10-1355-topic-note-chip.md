# Plan: composer chip for each non-empty topic note

## Request

User, 2026-11-10 whiteboard (598) ok: in the dashboard composer, a topic note
(the per-topic note pen, `.tnote[data-topic]` on the board) is currently
invisible in the composer even though a send carries it. Show each non-empty
topic note as a chip in the composer's chip row (`#cchips`), next to the
existing answer chips (`.achip`).

## Confirmed behavior

- One chip per non-empty topic note, reading `[<topic name>] <note text>`
  (text truncated with ellipsis by CSS like the existing achip), with a `×`
  button.
- `×` clears that topic note (empties the `.tnote` value and fires whatever
  the existing note-input path fires so state, drafts, and `holdsDraft()` stay
  consistent), then re-renders the chips.
- Clicking the chip body focuses that topic's note on the board (scroll the
  topic into view in `#bcol`, open the note if it is collapsed via the
  existing `.tnote-toggle` path, focus the textarea).
- Live update: typing in a topic note updates the chip row (input-driven, no
  polling); a board swap/morph re-renders the chips from the current DOM
  without accumulating listeners (use delegation on stable containers, as the
  existing code does at dashboard.js ~:1323 `data-unchip`).
- The send format is unchanged: `topicNotes()` (dashboard.js ~:1056) still
  produces the `[topic] text` lines exactly as today.
- Distinguish topic-note chips from answer chips visually only if existing
  tokens allow it cheaply (e.g. a muted variant); otherwise same achip look.
  Use only existing `--wb-*` tokens.

## Scope

- `agents-plugin-tool/internal/wsdashboard/assets/dashboard.js`
  (`renderChips` ~:746, `topicNotes` ~:1056, click delegation ~:1323, note
  input listeners ~:319/~:339), `dashboard.css` if needed, and i18n tables
  (`internal/wsi18n/tables/*.json`, all locales) only for a new tooltip/aria
  string such as the `×` label.
- Frontend only. No Go change beyond tests.

## Verification

- `nice -n 19 node --check` on touched JS.
- Go tests only remotely: `~/.local/bin/ws-rgo -C
  /home/swkang/.ws-worktrees/devenv/aim-hydration-granite/agents-plugin-tool
  -m 30 -- test ./internal/wsdashboard/ ./internal/wswhiteboard/
  ./internal/wsi18n/ -count=1 -timeout 30m`. Never run go build/test/vet
  locally (it freezes the machine).
- If an existing node harness covers composer chips (see
  `internal/wsdashboard/*_test.go` node harnesses), extend it for the new
  chip; otherwise add a small headless node check if cheap.
- Never touch processes or ports 29417 / 45743.
- Commit on branch `feat/topic-note-chip`; do not push. No `git stash`.
