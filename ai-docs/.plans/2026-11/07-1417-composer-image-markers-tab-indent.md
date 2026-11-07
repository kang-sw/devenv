# Plan: dashboard composer - inline image markers and Tab indent

## Outcome

Two composer changes on the dashboard session page, confirmed by the user
2026-11-07 (whiteboard items (69)-(74), all "ok").

### A. Image markers (items 70-73)

User request: "이미지 삽입 관련해서도, 본문에 [Image #1] 이런 느낌으로, 그리고
뒤에 Image #1: <path> 이런 식으로, 어떤 이미지가 본문의 어디에서
레퍼런스되는지 딱지? 같은 게 삽입되게 하면 좋겠어요."

1. (70) Pasting or dropping an image into the composer inserts the marker
   `[Image #N]` at the cursor (N = next number in this draft). On send, the
   delivered message text is the body followed by one `Image #N: <path>` line
   per image, replacing today's `[image: <path>]` lines
   (`wsinbox.ImageLine`, `internal/wsinbox/inbox.go`). The adapters pass the
   text through unchanged, so no adapter change. The server keeps parsing the
   old `[image: <path>]` form for entries already on disk.
2. (71) Marker and chip stay in sync both ways: deleting `[Image #N]` from the
   text drops that image chip; removing a chip deletes its marker from the
   text. Numbers are never renumbered within a draft (gaps are allowed),
   because rewriting numbers in the text the user is editing shakes it.
   Cancel of a held entry restores text with markers and chips together.
3. (72) When several mid-turn sends merge into one held entry (under the
   inbox lock, `d.inboxMu` / `wsinbox` merge path), the server renumbers the
   later message's markers and `Image #N:` lines to continue the earlier
   ones (e.g. the second message's #1 becomes #3), because the merged entry
   reaches the model as one message. The page must not pre-number across
   sends (concurrent sends would diverge). Steer copies the entry, so
   numbering carries over.
4. (73) Conversation rendering (finished user entries and held bubbles): the
   body's `[Image #N]` shows as a small tag; thumbnails carry their `#N`; the
   `Image #N: <path>` lines are removed from the body and shown as
   thumbnails (today `userImages` in `internal/wsdashboard/pageview_inbox.go`
   does this for the old form). A missing file shows the path as text, as
   today.

### B. Tab indent (item 69)

User: "채팅창 안에서 TAB 키라던지 이런 거 전부 인덴트라던지 그런 식으로
작동하게" - Tab moved focus out of the composer and caused a mis-send.

- Tab inserts two spaces at the cursor; with a multi-line selection it
  indents every selected line by two spaces. Shift+Tab outdents (removes up
  to two leading spaces) per line.
- Ignored while IME composition is active (`e.isComposing` / keyCode 229).
- Escape, then Tab, leaves the composer (keyboard escape hatch for
  accessibility).
- Keep the undo stack working where the browser allows (prefer
  `document.execCommand("insertText")` or `setRangeText` with an input event).

## Settled constraints

- Files expected: `internal/wsinbox/inbox.go` (+ tests),
  `internal/wsdashboard/` message/inbox routes, `pageview_inbox.go`,
  `assets/dashboard.js`, `dashboard.tmpl`, `dashboard.css` (+ tests). Do not
  touch `internal/wswhiteboard` or `internal/mcp/whiteboard_panels.go`
  (another delegate is editing them).
- The image cap, attachment storage, and the steer/cancel routes keep their
  current semantics apart from the numbering above.
- Update `ai-docs/manuals/ws-mcp.md` where it describes the message body or
  image lines (it currently says `{text, chips}` with no images; fix that
  line too).
- Shipped-surface boundary applies (`ai-docs/manuals/shipped-surface-boundary.md`).

## Boundary

Commit on branch `delegate/composer-image-markers-tab` only; do not merge or
push. Return to the lead if a change needs a protocol decision beyond the
above (e.g. an adapter must change).

## Verification

In agents-plugin-tool: `go build ./... && go vet ./internal/... ./cmd/...`,
`go test -count=1 ./...` (the `-race` timeout of
`TestMessageRouteRefusesBadImages` is pre-existing; ignore it), `-race` on
wsinbox and the wsdashboard message/merge/steer tests. Tests: new-format
lines emitted on send; old-format lines still parsed; merge renumbering
under concurrent sends; conversation render shows tags and numbered thumbs.
`node --check dashboard.js`. Headless Chromium (cached Playwright under
~/.npm/_npx/*/node_modules/playwright): paste/drop inserts a marker, deleting
the marker drops the chip and vice versa, Tab/Shift+Tab indent and outdent a
multi-line selection, Escape-then-Tab leaves the composer. Report what was not
checked.
