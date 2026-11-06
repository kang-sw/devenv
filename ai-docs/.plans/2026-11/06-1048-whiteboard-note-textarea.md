# whiteboard note textarea

User feedback (2026-11-06): the whiteboard note fields are single-line
`<input class="note">`, so long notes scroll sideways. Make them multiline
editors that grow with their content.

## Assignment

1. In `agents-plugin-tool/internal/wswhiteboard/assets/page.tmpl`, replace the
   three `<input class="note" ...>` fields (policy card, assumed card, ack row)
   with `<textarea class="note" ...>`, keeping class, placeholder, title, and
   `hidden`.
2. Auto-grow: in `assets/form.js`, size each note to its content on input
   (and when revealed by the note toggle), starting at one line. Style it in
   `assets/page.css` so it matches the current note field look, with no
   manual resize handle and no inner scrollbar while growing.
3. Enter inserts a newline (default textarea behavior). Newlines are kept as-is
   in the reply text that `build()` writes; do not collapse them.
4. Keep `live.js`'s `formBusy()` working: it already keys on the `note` class.
5. Check the ack row layout: a growing textarea in `.arow` must not break the
   row (it may need to wrap to its own line below the label).

## Verification

`go test ./internal/wswhiteboard -count=1` and the whiteboard tests in
`./internal/mcp` (`-run 'Whiteboard' -count=1`). Update any render test that
asserts the `<input class="note"` markup. If practical, render a sample full
page and check the note field in a headless browser; otherwise say it was not
visually checked.

## Return boundary

Stop and report before changing the reply grammar, the store, any tool schema,
or shipped playbook text.
