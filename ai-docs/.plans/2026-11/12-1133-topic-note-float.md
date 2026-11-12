# Plan: the open topic note floats under the topic's sticky header; SEND closes open notes

## Goal

User request (2026-11-12, board, with a screenshot of an open topic note
sitting right under the topic header): today the topic note editor (the pen
toggle on a topic header, `.tnote` / `.tnote-toggle`) opens attached in
the flow at the top of the topic, so it scrolls away while the user reads a
long topic. Instead:

1. An open topic note is a floating capsule that travels with the topic's
   sticky header, so the user can keep writing while scrolling through the
   topic body and its items.
2. Pressing SEND closes every open topic note editor.

## Scope

- Files: `agents-plugin-tool/internal/wsdashboard/assets/dashboard.css`,
  `dashboard.js` (topic note helpers ~L385-415 carry-over, ~L962-1010
  chips/`topicNote`/`focusTopicNote`, ~L1294-1345 `topicNotes` and the
  send path), and where the note markup is rendered
  (`wswhiteboard/assets/page.tmpl`, `page.css`, `form.js`,
  `wswhiteboard/render.go` - find which owns `.tnote`). Find the topic's
  sticky header rule (the board topic header; `.thead`-like) first.
- Floating capsule: make the open note `position: sticky` directly under the
  topic's sticky header (top = the header's height/offset, z-index above body
  and items, below the header), so at rest it sits in its current place and
  it sticks under the header while the topic scrolls, leaving with the topic
  when the topic scrolls out. Capsule look: rounded, card background, soft
  shadow, small inset from the topic edges, quiet `--wb-*` tokens, light and
  dark. The textarea keeps its auto-grow but caps its height (about 6 lines,
  then scrolls inside) so the capsule never covers most of the viewport.
  If sticky cannot work because of an ancestor's overflow, use the smallest
  JS fallback and say so.
- SEND: when a send succeeds, close every open topic note editor (the same
  state the pen toggle closes to). Keep the existing rule that a note edited
  during the send keeps its text for the next send; closing hides the editor
  only, and the note's chip/pen still reopens it with its text.
- Keep: note carry-over across board swaps (by topic name), the
  `focusTopicNote` chip jump (it must still bring the note into view and
  focus it), board morph/fold animations, the editing deferral, and both the
  board in the session page and the standalone whiteboard page if both render
  notes.

## Verification

- `~/.local/bin/ws-rgo -C agents-plugin-tool -m 8 -- test -count=1 ./internal/wsdashboard/... ./internal/wswhiteboard/...`
  (never run Go build/test/vet locally). Update markup assertions if a test
  pins the note's markup.
- Headless Chromium on the real assets (playwright-core at
  `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/pw/node_modules/playwright-core`, chromium at
  `~/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome`; a captured
  real page is `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/cur.html`, or build a fixture from the real
  templates): a long topic with the note open, scrolled to its top, middle
  (capsule stuck under the header), and past the topic (capsule gone with it);
  light and dark; then a send closing open notes. Screenshots to
  `/tmp/claude-1000/-home-swkang-devenv/6900e174-9e2f-4bdd-b25f-ae5a39a1cd42/scratchpad/note-float/`.

## Branch

Commit on `delegate/topic-note-float` in this worktree. Do not merge or
push.
